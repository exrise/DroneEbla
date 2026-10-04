package sim

import (
	"container/heap"
	"math"

	"github.com/exrise/droneebla/internal/data"
)

type pqItem struct {
	i int
	f float64
}
type pq []pqItem

func (p pq) Len() int           { return len(p) }
func (p pq) Less(a, b int) bool { return p[a].f < p[b].f }
func (p pq) Swap(a, b int)      { p[a], p[b] = p[b], p[a] }
func (p *pq) Push(x any)        { *p = append(*p, x.(pqItem)) }
func (p *pq) Pop() any          { o := *p; it := o[len(o)-1]; *p = o[:len(o)-1]; return it }

// findPath — путь по своей территории с учётом дорог (A*).
func (w *World) findPath(s int, from, to Pt, roadKmh, offKmh float64) []Pt {
	m := w.m
	start := w.tileOf(from.X, from.Y)
	goal := w.tileOf(to.X, to.Y)
	if start < 0 || goal < 0 || w.OwnerSide(goal) != s || m.Terrain[goal] != 1 {
		return nil
	}
	n := m.W * m.H
	g := make([]float32, n)
	prev := make([]int32, n)
	for i := range g {
		g[i] = float32(math.Inf(1))
		prev[i] = -1
	}
	g[start] = 0
	h := func(i int) float64 {
		cx, cy := m.TileCenter(i%m.W, i/m.W)
		return dist(cx, cy, to.X, to.Y) / roadKmh
	}
	open := &pq{{start, h(start)}}
	closed := make([]bool, n)
	for open.Len() > 0 {
		it := heap.Pop(open).(pqItem)
		i := it.i
		if closed[i] {
			continue
		}
		closed[i] = true
		if i == goal {
			break
		}
		x, y := i%m.W, i/m.W
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 || !m.In(x+dx, y+dy) {
					continue
				}
				j := m.Idx(x+dx, y+dy)
				if closed[j] || m.Terrain[j] != 1 || (w.OwnerSide(j) != s && j != start) {
					continue
				}
				sp := offKmh
				if m.Flags[j]&2 != 0 && m.Flags[i]&2 != 0 {
					sp = roadKmh
				}
				step := m.TileKm / sp
				if dx != 0 && dy != 0 {
					step *= 1.414
				}
				ng := g[i] + float32(step)
				if ng < g[j] {
					g[j] = ng
					prev[j] = int32(i)
					heap.Push(open, pqItem{j, float64(ng) + h(j)})
				}
			}
		}
	}
	if prev[goal] < 0 && goal != start {
		return nil
	}
	var rev []Pt
	for i := goal; i != start && i >= 0; i = int(prev[i]) {
		cx, cy := m.TileCenter(i%m.W, i/m.W)
		rev = append(rev, Pt{cx, cy})
	}
	path := make([]Pt, 0, len(rev)+1)
	for k := len(rev) - 1; k >= 0; k-- {
		path = append(path, rev[k])
	}
	if len(path) > 0 {
		path[len(path)-1] = to
	} else {
		path = append(path, to)
	}
	return path
}

// MoveUnit отдаёт приказ на марш.
func (w *World) MoveUnit(s int, id uint32, to Pt) string {
	u, ok := w.Units[id]
	if !ok || u.Side != s {
		return "Нет такого юнита"
	}
	ut := w.cat.UnitByID[u.Type]
	path := w.findPath(s, Pt{u.X, u.Y}, to, ut.SpeedRoad, ut.SpeedOff)
	if path == nil {
		return "Нет пути по своей территории"
	}
	u.Path = path
	switch u.State {
	case UnitDeployed:
		u.State = UnitPacking
		u.Timer = ut.PackMin
	case UnitDeploying:
		u.State = UnitPacking
		u.Timer = ut.PackMin * 0.5
	}
	return ""
}

// units — марш, свёртывание, перезарядка.
func (w *World) units(dtMin float64) {
	for _, u := range w.Units {
		ut := w.cat.UnitByID[u.Type]
		if u.Reload > 0 {
			u.Reload = math.Max(0, u.Reload-dtMin)
		}
		w.reloadAD(u, ut, dtMin)
		switch u.State {
		case UnitPacking:
			u.Timer -= dtMin
			if u.Timer <= 0 {
				u.State = UnitMoving
			}
		case UnitDeploying:
			u.Timer -= dtMin
			if u.Timer <= 0 {
				u.State = UnitDeployed
			}
		case UnitMoving:
			left := dtMin
			for left > 0 && len(u.Path) > 0 {
				t := u.Path[0]
				i := w.tileOf(u.X, u.Y)
				sp := ut.SpeedOff
				if i >= 0 && w.m.Flags[i]&2 != 0 {
					sp = ut.SpeedRoad
				}
				step := sp / 60 * left
				d := dist(u.X, u.Y, t.X, t.Y)
				if d <= step {
					u.X, u.Y = t.X, t.Y
					left -= d / (sp / 60)
					u.Path = u.Path[1:]
				} else {
					u.X += (t.X - u.X) / d * step
					u.Y += (t.Y - u.Y) / d * step
					left = 0
				}
			}
			if len(u.Path) == 0 {
				u.State = UnitDeploying
				u.Timer = ut.DeployMin
			}
		}
	}
}

// reloadAD — перезарядка пусковых ПВО из национального запаса.
// Полный магазин перезаряжается за reload_min минут; на марше — нет.
func (w *World) reloadAD(u *Unit, ut *data.UnitType, dtMin float64) {
	if ut.Kind != "ad" || ut.Magazine <= 0 || ut.ReloadMin <= 0 {
		return
	}
	if u.State == UnitMoving || u.Ready >= float64(ut.Magazine) {
		return
	}
	sd := w.Sides[u.Side]
	n := math.Min(float64(ut.Magazine)/ut.ReloadMin*dtMin, float64(ut.Magazine)-u.Ready)
	if ut.Interceptor != "" {
		n = math.Min(n, sd.Stocks[ut.Interceptor])
		sd.Stocks[ut.Interceptor] -= n
	} else {
		// пушки и пулемёты: 0.2 боеприпаса на очередь
		n = math.Min(n, sd.Res[data.ResAmmo]/0.2)
		sd.Res[data.ResAmmo] -= n * 0.2
	}
	if n > 0 {
		u.Ready += n
	}
}
