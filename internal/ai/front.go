package ai

import (
	"sort"

	"github.com/exrise/droneebla/internal/sim"
)

// front — распределение пополнений, позиция, укрепления.
func (a *AI) front(w *sim.World, v *sim.View) {
	if !a.due("front", v.Time, 30) {
		return
	}
	m := w.Map()
	// Угроза по направлениям: давление противника на наши фронтовые тайлы.
	var press [3]float64
	type tile struct {
		i int
		p float64
	}
	var hot []tile
	for _, i := range w.FrontTiles(a.side) {
		if i < 0 || i >= len(v.Pressure) {
			continue
		}
		p := float64(v.Pressure[i]) / 255
		press[w.TileDir(i)] += p
		if p > 0.05 && v.Fort[i] < 3 {
			hot = append(hot, tile{i, p})
		}
	}
	vals := make([]float64, 3)
	for d := 0; d < 3; d++ {
		vals[d] = 10 + 0.5*float64(v.EnemyFront[d]) + 6*press[d]
	}
	a.cmd(w, sim.Command{Kind: sim.CmdAlloc, Vals: vals})

	posture := a.cfg.WarPosture
	if !v.War {
		posture = a.cfg.PrepPosture
	}
	if v.Posture != posture {
		a.cmd(w, sim.Command{Kind: sim.CmdPosture, Int: posture})
	}

	// Укрепляем самые давимые тайлы, пока хватает ресурсов.
	if a.cfg.FortTiles > 0 && len(hot) > 0 {
		sort.Slice(hot, func(i, j int) bool {
			if hot[i].p != hot[j].p {
				return hot[i].p > hot[j].p
			}
			return hot[i].i < hot[j].i
		})
		var pts []sim.Pt
		for k := 0; k < len(hot) && k < a.cfg.FortTiles; k++ {
			i := hot[k].i
			pts = append(pts, sim.Pt{X: (float64(i%m.W) + 0.5) * m.TileKm, Y: (float64(i/m.W) + 0.5) * m.TileKm})
		}
		a.cmd(w, sim.Command{Kind: sim.CmdFort, Pts: pts})
	}
}
