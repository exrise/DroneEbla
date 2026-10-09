// Package ai — компьютерный противник. Играет по тем же правилам, что и
// человек: видит только то, что видит его сторона (sim.View), и отдаёт те же
// приказы (sim.Command) через World.Apply. Все параметры — в ai.json.
package ai

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

// AI управляет одной стороной.
type AI struct {
	side int
	cfg  data.AISide
	cat  *data.Catalog
	rng  *rand.Rand

	// OnCommand вызывается после каждого приказа (для тестов и отладки).
	OnCommand func(c sim.Command, err string)
	// Debug получает пояснения решений (для отладки и стенда); nil — молчит.
	Debug func(format string, args ...any)

	next map[string]float64 // следующее время запуска подсистемы, игровые минуты

	ordersDone map[string]bool    // разовые позиции госзаказа уже заказаны
	lastHit    map[uint32]float64 // контакт → когда по нему били
	lastMove   map[uint32]float64 // юнит → когда его переставляли
	lastRecon  map[uint32]float64 // контакт → когда над ним летал разведчик
	repairOff  map[uint32]bool    // здания, у которых ИИ отключил ремонт
	buildAt    int                // счётчик опорных точек для стройки
	evSeen     uint64             // последнее разобранное событие журнала
	reconPause float64            // до этого времени разведчиков не посылаем (был сбит)

	// Состояние «умного» поведения.
	peakMen     float64             // максимум людей на фронте за партию
	trend       [sim.NumDir]float64 // сглаженный прирост тайлов по направлениям
	postureAt   [sim.NumDir]float64 // когда направление в последний раз меняло позицию
	noOffense   [sim.NumDir]float64 // до этого времени наступать на направлении нельзя (откатились)
	defending   [sim.NumDir]bool    // направление в глухой обороне (гистерезис)
	lastAlloc   []float64           // последнее отправленное распределение пополнений
	lastFort    string              // последний набор укрепляемых тайлов
	lastFortAt  float64             // когда его отправляли
	mainAt      float64             // когда пересматривали главный удар
	holdSince   map[uint32]float64  // цель → когда залп на неё начали копить
	reconFails  float64             // «штраф» разведки: растёт при потерях, спадает со временем
	zones       []zone              // известные зоны ПВО противника на время планирования ударов
	reachEMA    float64             // доля долетевших боеприпасов (скользящая)
	nowT        float64             // игровое время текущего решения
	lastLaunch  map[string]float64  // боеприпас → когда его запускали в последний раз
	mReach      map[string]*reachStat
	zeroHits    map[uint32]int // цель → сколько залпов подряд не долетело ни одного
	reconFailAt float64        // когда штраф обновляли
}

// New создаёт ИИ для стороны side. Если для стороны нет настроек, ИИ бездействует.
func New(cat *data.Catalog, side int) *AI {
	return &AI{
		side: side, cfg: cat.AI.Sides[data.SideKeys[side]], cat: cat,
		rng:  rand.New(rand.NewSource(int64(7919 + side))),
		next: map[string]float64{}, ordersDone: map[string]bool{},
		holdSince: map[uint32]float64{}, lastLaunch: map[string]float64{}, mReach: map[string]*reachStat{}, zeroHits: map[uint32]int{}, lastHit: map[uint32]float64{}, lastMove: map[uint32]float64{}, lastRecon: map[uint32]float64{}, repairOff: map[uint32]bool{},
	}
}

// Side — сторона, которой управляет ИИ.
func (a *AI) Side() int { return a.side }

// due — пора ли запускать подсистему name (период every минут).
func (a *AI) due(name string, now, every float64) bool {
	if now < a.next[name] {
		return false
	}
	if every <= 0 {
		every = a.cat.AI.ThinkMin
	}
	a.next[name] = now + every
	return true
}

// Tick принимает решения; вызывается из цикла хоста под его блокировкой.
func (a *AI) Tick(w *sim.World) {
	if w.Winner >= 0 || len(a.cat.AI.Sides) == 0 {
		return
	}
	if w.Placement {
		a.place(w)
		return
	}
	now := w.Time
	if now < a.next["think"] {
		return
	}
	a.next["think"] = now + a.cat.AI.ThinkMin
	v := w.BuildView(a.side, 0)
	a.nowT = now
	a.readEvents(v)
	a.economy(w, v)
	a.front(w, v)
	a.defense(w, v)
	if v.War {
		a.strikes(w, v)
	}
}

func (a *AI) debugf(format string, args ...any) {
	if a.Debug != nil {
		a.Debug(format, args...)
	}
}

// cmd отдаёт приказ от имени ИИ.
func (a *AI) cmd(w *sim.World, c sim.Command) string {
	c.Side = a.side
	err := w.Apply(c)
	if a.OnCommand != nil {
		a.OnCommand(c, err)
	}
	return err
}

// split разбирает "id" или "id:число".
func split(s string) (string, int) {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		n, _ := strconv.Atoi(s[i+1:])
		return s[:i], n
	}
	return s, 0
}

func dist(ax, ay, bx, by float64) float64 { return math.Hypot(ax-bx, ay-by) }

// readEvents учится на журнале: после потери разведчика делает паузу в разведке.
func (a *AI) readEvents(v *sim.View) {
	for _, e := range v.Events {
		if e.ID <= a.evSeen {
			continue
		}
		a.evSeen = e.ID
		if a.cfg.Smart && strings.HasPrefix(e.Text, "Итог удара (") && !strings.Contains(e.Text, "ложная") {
			var got, sent int
			if i := strings.Index(e.Text, "долетело "); i >= 0 {
				if n, _ := fmt.Sscanf(e.Text[i:], "долетело %d из %d", &got, &sent); n == 2 && sent > 0 {
					k := math.Min(0.3, float64(sent)/60)
					a.reachEMA = a.reachEMA*(1-k) + float64(got)/float64(sent)*k
					a.noteReach(e.Text, got, sent, v.Time)
					a.noteResult(v, e, got)
				}
			}
		}
		if strings.HasPrefix(e.Text, "Потерян разведчик") {
			if a.cfg.Smart {
				// Каждая потеря удлиняет паузу: 6 ч, 12 ч, 24 ч… (штраф спадает со временем).
				a.reconFails++
				a.reconFailAt = v.Time
				a.reconPause = v.Time + a.cfg.ReconStaleMin/2*math.Pow(2, math.Min(a.reconFails-1, 3))
			} else {
				a.reconPause = v.Time + a.cfg.ReconStaleMin/2
			}
		}
	}
}

// noteResult запоминает исход залпа по цели у точки события: подряд провалы удлиняют паузу до следующего удара по ней.
func (a *AI) noteResult(v *sim.View, e sim.Event, got int) {
	if !e.HasPos {
		return
	}
	var best *sim.Contact
	bd := 25.0
	for i := range v.Contacts {
		c := &v.Contacts[i]
		if c.Kind != 0 {
			continue
		}
		if d := dist(c.X, c.Y, e.X, e.Y); d < bd {
			best, bd = c, d
		}
	}
	if best == nil {
		return
	}
	if got > 0 {
		delete(a.zeroHits, best.ID)
		return
	}
	a.zeroHits[best.ID]++
}

// reachStat — сколько боеприпасов одного вида выпущено и долетело (по итогам залпов).
type reachStat struct {
	sent, got, at float64
	fails         int // залпов подряд, из которых не долетело ни одного
}

// noteReach учитывает итог залпа «Итог удара (Название): долетело N из M» для боеприпаса с этим названием.
func (a *AI) noteReach(text string, got, sent int, now float64) {
	i, j := strings.Index(text, "("), strings.Index(text, "): долетело")
	if i < 0 || j <= i {
		return
	}
	name := text[i+1 : j]
	for _, m := range a.cat.Munitions {
		if m.Name != name || data.SideIndex(m.Side) != a.side {
			continue
		}
		st := a.mReach[m.ID]
		if st == nil {
			st = &reachStat{}
			a.mReach[m.ID] = st
		}
		st.sent += float64(sent)
		st.got += float64(got)
		st.at = now
		if got == 0 {
			st.fails++
		} else {
			st.fails = 0
		}
		if st.sent > 100 {
			st.sent, st.got = st.sent/2, st.got/2 // старые залпы забываются
		}
		return
	}
}

// hopeless — этот боеприпас почти не долетал (меньше reach_floor от выпущенных) и давно не пробовали заново.
func (a *AI) hopeless(id string) bool {
	st := a.mReach[id]
	if st == nil || st.sent < 20 || a.nowT-st.at > 2*a.hopelessPause(id) {
		return false
	}
	floor := a.cfg.ReachFloor
	if floor <= 0 {
		floor = 0.05
	}
	return st.got/st.sent < floor
}

// probing — медленный дрон, чьи залпы ещё не разрешились: итоги приходят через часы полёта, и без этого правила бот
// успевает выпустить десятки дронов, не узнав, что они не долетают.
func (a *AI) probing(m *data.MunitionType) bool {
	if m.Kind != "drone" || m.SpeedKmh <= 0 || m.SpeedKmh > 300 {
		return false
	}
	last, ok := a.lastLaunch[m.ID]
	if !ok {
		return false
	}
	if st := a.mReach[m.ID]; st != nil && st.sent >= 20 {
		return false
	}
	return a.nowT-last < 1440
}

// hopelessPause — пауза между волнами безнадёжным боеприпасом: сутки, после каждых двух провальных залпов подряд вдвое дольше (до 8 суток).
func (a *AI) hopelessPause(id string) float64 {
	return 1440 * math.Pow(2, math.Min(3, float64(a.failStreak(id)/2)))
}

func (a *AI) failStreak(id string) int {
	if st := a.mReach[id]; st != nil {
		return st.fails
	}
	return 0
}

// hopelessSalvo — минимальный размер волны для боеприпаса, который почти не долетал: масса, способная насытить ПВО;
// после каждых двух провальных залпов подряд требование удваивается (до ×8).
func (a *AI) hopelessSalvo(id string) float64 {
	base := a.cfg.HopelessSalvo
	if base <= 0 {
		base = 24
	}
	return base * math.Pow(2, math.Min(3, float64(a.failStreak(id)/2)))
}
