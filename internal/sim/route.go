package sim

import (
	"math"

	"github.com/exrise/droneebla/internal/data"
)

// Zone — круг поражения известного комплекса ПВО противника.
type Zone struct{ X, Y, R float64 }

// RouteAround строит путевые точки в обход зон: прямой маршрут, пересекающий зону, отодвигается за её край
// (с запасом 12 км) на сторону с меньшим крюком. Зоны, накрывающие саму цель или старт, не обходятся — туда
// приходится лететь. Возвращает точки без старта и цели (nil — прямой маршрут).
func RouteAround(zones []Zone, sx, sy, tx, ty float64) []Pt {
	if len(zones) == 0 {
		return nil
	}
	var zs []Zone
	for _, z := range zones {
		if dist(z.X, z.Y, tx, ty) > z.R && dist(z.X, z.Y, sx, sy) > z.R {
			zs = append(zs, z)
		}
	}
	path := []Pt{{X: sx, Y: sy}, {X: tx, Y: ty}}
	for iter := 0; iter < 6; iter++ {
		inserted := false
		for seg := 0; seg+1 < len(path) && !inserted; seg++ {
			p, q := path[seg], path[seg+1]
			dx, dy := q.X-p.X, q.Y-p.Y
			l := math.Hypot(dx, dy)
			if l < 1 {
				continue
			}
			ux, uy := dx/l, dy/l
			for _, z := range zs {
				// ближайшая к центру зоны точка отрезка
				t := math.Max(0, math.Min(l, (z.X-p.X)*ux+(z.Y-p.Y)*uy))
				cx, cy := p.X+ux*t, p.Y+uy*t
				if dist(cx, cy, z.X, z.Y) >= z.R {
					continue
				}
				off := z.R + 12
				w1 := Pt{X: z.X - uy*off, Y: z.Y + ux*off}
				w2 := Pt{X: z.X + uy*off, Y: z.Y - ux*off}
				d1 := dist(p.X, p.Y, w1.X, w1.Y) + dist(w1.X, w1.Y, q.X, q.Y)
				d2 := dist(p.X, p.Y, w2.X, w2.Y) + dist(w2.X, w2.Y, q.X, q.Y)
				wp := w1
				if d2 < d1 {
					wp = w2
				}
				path = append(path[:seg+1], append([]Pt{wp}, path[seg+1:]...)...)
				inserted = true
				break
			}
		}
		if !inserted {
			break
		}
	}
	if len(path) <= 2 {
		return nil
	}
	return path[1 : len(path)-1]
}

// KnownADZones — зоны известной стороне s ПВО противника (разведка не старше 12 часов).
func (w *World) KnownADZones(s int) []Zone {
	var out []Zone
	for _, c := range w.Sides[s].Known {
		if c.Kind != 1 || c.Type == "" || c.Seen < 0 || w.Time-c.Seen > 720 {
			continue
		}
		ut := w.cat.UnitByID[c.Type]
		if ut == nil || ut.Kind != "ad" || ut.RangeKm < 5 {
			continue
		}
		out = append(out, Zone{c.X, c.Y, ut.RangeKm * 0.9})
	}
	return out
}

// CorridorRoute ищет самый короткий допустимый маршрут по цепочкам corridors (с любой точки цепочки до конца);
// nil, если небо закрыто или цель вне дальности.
func (w *World) CorridorRoute(s int, sp StrikePlan, sx, sy float64) []Pt {
	var best []Pt
	bestLen := math.MaxFloat64
	for _, chain := range w.cat.AI.Sides[data.SideKeys[s]].Corridors {
		for i := range chain {
			var r []Pt
			for _, c := range chain[i:] {
				x, y := w.m.Project(c[0], c[1])
				r = append(r, Pt{X: x, Y: y})
			}
			sp.Waypoints = r
			if w.ValidateStrike(s, sp) != "" {
				continue
			}
			if l := PathLength(Pt{X: sx, Y: sy}, append(append([]Pt{}, r...), sp.Target)); l < bestLen {
				best, bestLen = r, l
			}
		}
	}
	return best
}
