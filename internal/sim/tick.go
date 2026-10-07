package sim

import "github.com/exrise/droneebla/internal/data"

// EffectiveSpeed — действующая скорость (меньшая из выбранных).
func (w *World) EffectiveSpeed() int {
	if w.Sandbox {
		return w.Sides[0].Speed
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
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		if sd.Pausing && (w.Sandbox || sd.PauseLeft > 0) {
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
		if !w.Sandbox {
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
	w.intel(dtMin)
	w.checkVictory()
}

func (w *World) checkVictory() {
	if w.Winner >= 0 || !w.War() {
		return
	}
	if w.kyiv >= 0 && w.OwnerSide(w.kyiv) == data.RU {
		w.Winner = data.RU
		w.WinReason = "Киев взят"
		return
	}
	// Украина: все территории, включая Крым.
	for i, c := range w.m.Country {
		if c == 1 && w.m.Terrain[i] == 1 && w.Owner[i] == 1 {
			return
		}
	}
	w.Winner = data.UA
	w.WinReason = "Все территории Украины, включая Крым, освобождены"
}
