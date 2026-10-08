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
	peakMen     float64            // максимум людей на фронте за партию
	trend       [3]float64         // сглаженный прирост тайлов по направлениям
	postureAt   [3]float64         // когда направление в последний раз меняло позицию
	noOffense   [3]float64         // до этого времени наступать на направлении нельзя (откатились)
	defending   [3]bool            // направление в глухой обороне (гистерезис)
	lastAlloc   []float64          // последнее отправленное распределение пополнений
	lastFort    string             // последний набор укрепляемых тайлов
	lastFortAt  float64            // когда его отправляли
	mainAt      float64            // когда пересматривали главный удар
	holdSince   map[uint32]float64 // цель → когда залп на неё начали копить
	reconFails  float64            // «штраф» разведки: растёт при потерях, спадает со временем
	reachEMA    float64            // доля долетевших боеприпасов (скользящая)
	reconFailAt float64            // когда штраф обновляли
}

// New создаёт ИИ для стороны side. Если для стороны нет настроек, ИИ бездействует.
func New(cat *data.Catalog, side int) *AI {
	return &AI{
		side: side, cfg: cat.AI.Sides[data.SideKeys[side]], cat: cat,
		rng:  rand.New(rand.NewSource(int64(7919 + side))),
		next: map[string]float64{}, ordersDone: map[string]bool{},
		holdSince: map[uint32]float64{}, lastHit: map[uint32]float64{}, lastMove: map[uint32]float64{}, lastRecon: map[uint32]float64{}, repairOff: map[uint32]bool{},
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
