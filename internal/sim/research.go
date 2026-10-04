package sim

import "github.com/exrise/droneebla/internal/data"

// research — исследования за dtH часов.
func (w *World) research(s int, dtH float64) {
	sd := w.Sides[s]
	r := w.cat.Rules
	rate := r.ResearchBase + float64(sd.ResFund)*1.5
	for _, b := range w.Buildings {
		if b.Side != s {
			continue
		}
		if bt := w.cat.BuildingByID[b.Type]; bt.Research > 0 {
			rate += bt.Research * w.output(b)
		}
	}
	rate *= 1 + sd.eff("research_speed")
	sd.ResRate = rate
	if sd.Research == "" {
		return
	}
	t := w.cat.TechByID[s][sd.Research]
	if t == nil {
		sd.Research = ""
		return
	}
	add := rate * dtH
	// Трофеи и опыт ускоряют исследования своей ветки (до +100%).
	if b := sd.Bonus[t.Branch]; b > 0 {
		extra := minf(b, add)
		sd.Bonus[t.Branch] -= extra
		add += extra
	}
	sd.Progress[t.ID] += add
	if sd.Progress[t.ID] >= t.Cost {
		w.completeTech(s, t)
	}
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (w *World) completeTech(s int, t *data.Tech) {
	sd := w.Sides[s]
	sd.Researched[t.ID] = true
	sd.Research = ""
	for _, u := range t.Unlocks {
		sd.Unlocked[u] = true
	}
	for k, v := range t.Effects {
		sd.Effects[k] += v
	}
	w.Log(s, 1, "Исследование завершено: "+t.Name)
}

// TechAvailable — можно ли начать исследование.
func (w *World) TechAvailable(s int, id string) bool {
	t := w.cat.TechByID[s][id]
	if t == nil || w.Sides[s].Researched[id] {
		return false
	}
	for _, r := range t.Requires {
		if !w.Sides[s].Researched[r] {
			return false
		}
	}
	return true
}

// addBonus — очки трофеев/опыта в ветку.
func (w *World) addBonus(s int, branch string, pts float64) {
	w.Sides[s].Bonus[branch] += pts
}

// branchOf — ветка для боеприпаса.
func (w *World) branchOf(munition string) string {
	m := w.cat.MunitionByID[munition]
	if m == nil {
		return "strike"
	}
	switch m.Kind {
	case "drone", "decoy", "recon":
		return "drones"
	}
	return "strike"
}
