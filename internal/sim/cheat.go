package sim

import "github.com/exrise/droneebla/internal/data"

// cheatBatch — сколько единиц одной позиции госзаказа выпускается за шаг в режиме «всё открыто».
const cheatBatch = 50

// EnableCheat включает режим песочницы «всё открыто»: изучено всё, открыты все предметы
// обеих сторон, производство, стройка и закупки мгновенные и бесплатные.
func (w *World) EnableCheat() {
	w.Cheat = true
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		for i := range w.cat.Tech[data.SideKeys[s]] {
			t := &w.cat.Tech[data.SideKeys[s]][i]
			if !sd.Researched[t.ID] {
				w.completeTech(s, t)
			}
		}
		sd.Research = ""
		for _, m := range w.cat.Munitions {
			if data.SideIndex(m.Side) == s {
				sd.Unlocked[m.ID] = true
			}
		}
		for _, u := range w.cat.Units {
			if data.SideIndex(u.Side) == s {
				sd.Unlocked[u.ID] = true
			}
		}
		for _, f := range w.cat.Front {
			sd.Unlocked[f.ID] = true
		}
	}
}
