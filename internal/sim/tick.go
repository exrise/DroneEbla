package sim

import (
	"strings"

	"github.com/exrise/droneebla/internal/data"
)

// EffectiveSpeed — действующая скорость (меньшая из выбранных).
func (w *World) EffectiveSpeed() int {
	if w.NetHost {
		return w.Sides[w.HostSide].Speed
	}
	if w.Solo || w.Sandbox {
		return w.Sides[w.Human].Speed // скоростью управляет человек (в песочнице — текущая сторона), а не сторона 0
	}
	a, b := w.Sides[0].Speed, w.Sides[1].Speed
	if b < a {
		return b
	}
	return a
}

// Paused — стоит ли игра на паузе.
func (w *World) Paused() bool {
	if w.Winner >= 0 || w.Placement {
		return true
	}
	if w.NetHost {
		return w.Sides[w.HostSide].Pausing
	}
	if w.Sandbox {
		return w.Sides[w.Human].Pausing // пауза текущей стороны; после смены стороны старая пауза не держит игру
	}
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		if sd.Pausing && sd.PauseLeft > 0 {
			return true
		}
	}
	return false
}

// Update продвигает игру на realSec реальных секунд.
func (w *World) Update(realSec float64) {
	if w.Winner >= 0 {
		return
	}
	if w.Paused() {
		if !w.Sandbox && !w.NetHost {
			for s := 0; s < 2; s++ {
				sd := w.Sides[s]
				if sd.Pausing {
					sd.PauseLeft -= realSec
					if sd.PauseLeft <= 0 {
						sd.PauseLeft = 0
						sd.Pausing = false
						w.Log(s, 1, "Лимит паузы исчерпан")
					}
				}
			}
		}
		return
	}
	dt := realSec * w.cat.Rules.GameMinPerSec * SpeedMult[w.EffectiveSpeed()]
	// Шаги не длиннее 1 игровой минуты.
	for dt > 0 {
		step := dt
		if step > 1 {
			step = 1
		}
		w.Step(step)
		dt -= step
	}
}

// Step — один шаг симуляции длиной dtMin игровых минут.
func (w *World) Step(dtMin float64) {
	w.step(dtMin)
	w.recTick()
}

func (w *World) step(dtMin float64) {
	w.sens.valid = false
	wasWar := w.War()
	w.Time += dtMin
	if !wasWar && w.War() {
		for s := 0; s < 2; s++ {
			w.Log(s, 2, "Война началась! Удары и наступление разрешены.")
		}
	}
	// Мелкие подшаги для быстрых боеприпасов.
	sub := 1
	for _, p := range w.Projs {
		if m := w.cat.MunitionByID[p.Munition]; m != nil {
			if n := int(m.SpeedKmh/60*dtMin/4) + 1; n > sub {
				sub = n
			}
		}
	}
	if sub > 30 {
		sub = 30
	}
	for k := 0; k < sub; k++ {
		w.projectiles(dtMin / float64(sub))
		w.airDefense(dtMin / float64(sub))
	}
	w.units(dtMin)
	w.economy(dtMin / 60)
	w.front(dtMin)
	w.flushRaids()
	w.sens.valid = false // юниты двигались, тайлы и здания могли перейти к другой стороне
	w.intel(dtMin)
	// Условия победы не меняются без захватов тайлов: проверяем после захвата и не реже раза в 10 минут.
	if w.Time >= w.nextVictory {
		w.nextVictory = w.Time + 10
		w.checkVictory()
	}
}

func (w *World) checkVictory() {
	if w.Winner >= 0 || !w.War() {
		return
	}
	if cities := w.victoryCities(); len(cities) > 0 {
		all := true
		for _, vc := range cities {
			all = all && w.OwnerSide(vc.tile) == data.RU
		}
		if all {
			w.Winner = data.RU
			w.WinReason = "Взяты " + w.victoryNames()
			return
		}
	}
	// Украина: все территории, включая Крым. Тайлы суши, окружённые морем со всех четырёх сторон
	// (узкие косы вроде Арабатской стрелки в сетке карты), фронт захватить не может — они не мешают.
	for i, c := range w.m.Country {
		if c == 1 && w.m.Terrain[i] == 1 && w.Owner[i] == 1 && w.attackableByLand(i) {
			return
		}
	}
	w.Winner = data.UA
	w.WinReason = "Все территории Украины, включая Крым, освобождены"
}

// attackableByLand — есть ли у тайла сосед-суша (участвующий в войне), с которого его можно атаковать:
// фронт работает по четырём соседям, поэтому тайл, окружённый морем, захватить нельзя.
func (w *World) attackableByLand(i int) bool {
	ok := false
	w.neighbors4(i, func(j int) {
		if w.m.Terrain[j] == 1 && (w.m.Country[j] == 1 || w.m.Country[j] == 2) {
			ok = true
		}
	})
	return ok
}

type victoryCity struct {
	name string
	tile int
}

// victoryCities — города условия победы России (крупнейший город карты с таким именем).
func (w *World) victoryCities() []victoryCity {
	if w.vcOK {
		return w.vc
	}
	w.vc, w.vcOK = nil, true
	for _, name := range w.cat.Rules.RuVictoryCities {
		best := -1
		for ci, c := range w.m.Cities {
			if c.Name == name && (best < 0 || c.Pop > w.m.Cities[best].Pop) {
				best = ci
			}
		}
		if best < 0 {
			continue
		}
		c := w.m.Cities[best]
		if i := w.tileOf(c.X, c.Y); i >= 0 {
			w.vc = append(w.vc, victoryCity{name, i})
		}
	}
	return w.vc
}

func (w *World) victoryNames() string {
	var names []string
	for _, vc := range w.victoryCities() {
		names = append(names, vc.name)
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " и " + names[len(names)-1]
}

// VictoryCityView — город условия победы России и его владелец (для интерфейса).
type VictoryCityView struct {
	Name  string
	Owner int // -1 — ничей
}
