package ai

import (
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/sim"
)

// Place — расстановка резерва (для автоматической расстановки и тестов).
func (a *AI) Place(w *sim.World) { a.place(w) }

// place расставляет весь резерв и нажимает «Готово». Стартовые точки из данных
// стороны используются в первую очередь; если там нельзя ставить (близко к
// фронту), ИИ ищет место рядом со своими ценными объектами.
func (a *AI) place(w *sim.World) {
	v := w.BuildView(a.side, 0)
	if v.Ready {
		return
	}
	left := map[string]int{}
	for k, n := range v.Reserve {
		left[k] = n
	}
	try := func(typ string, x, y float64) bool {
		if left[typ] <= 0 || w.CanPlace(a.side, typ, x, y) != "" {
			return false
		}
		if a.cmd(w, sim.Command{Kind: sim.CmdPlace, Item: typ, X: x, Y: y}) != "" {
			return false
		}
		left[typ]--
		return true
	}
	for _, h := range v.Hints {
		try(h.Type, h.X, h.Y)
	}
	// Остальное — рядом со своими объектами по убыванию их ценности.
	var anchors []sim.Building
	for _, b := range v.Buildings {
		if b.Built >= 1 && a.cfg.Protect[b.Type] > 0 {
			anchors = append(anchors, b)
		}
	}
	sort.Slice(anchors, func(i, j int) bool {
		wi, wj := a.cfg.Protect[anchors[i].Type], a.cfg.Protect[anchors[j].Type]
		if wi != wj {
			return wi > wj
		}
		return anchors[i].ID < anchors[j].ID
	})
	types := make([]string, 0, len(left))
	for k := range left {
		types = append(types, k)
	}
	sort.Strings(types)
	if len(anchors) > 0 {
		for _, typ := range types {
			for i := 0; left[typ] > 0 && i < 400; i++ {
				an := anchors[i%len(anchors)]
				ang := a.rng.Float64() * 2 * math.Pi
				r := 6 + a.rng.Float64()*25
				try(typ, an.X+math.Cos(ang)*r, an.Y+math.Sin(ang)*r)
			}
		}
	}
	a.cmd(w, sim.Command{Kind: sim.CmdReady, Int: 1})
}
