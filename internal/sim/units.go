package sim

import (
	"container/heap"
	"math"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
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

// railNodes — где можно сесть на поезд и где путь перерезан. board[i]: ж/д тайл вблизи работающей своей
// станции или узла; blocked[i]: ж/д тайл у станции или узла, которые не работают (разрушены).
func (w *World) railNodes(s int) (board, blocked []bool) {
	m := w.m
	board = make([]bool, m.W*m.H)
	blocked = make([]bool, m.W*m.H)
	r := w.cat.Rules.RailStationTiles
	for _, b := range w.Buildings {
		if b.Side != s || (b.Type != "rail_station" && b.Type != "rail_hub") {
			continue
		}
		ok := b.Operational()
		tx, ty := m.TileAt(b.X, b.Y)
		rr := r
		if !ok {
			rr = 1
		}
		for dy := -rr; dy <= rr; dy++ {
			for dx := -rr; dx <= rr; dx++ {
				if !m.In(tx+dx, ty+dy) {
					continue
				}
				j := m.Idx(tx+dx, ty+dy)
				if m.Flags[j]&4 == 0 || m.Terrain[j] != 1 {
					continue
				}
				if ok {
					board[j] = true
				} else {
					blocked[j] = true
				}
			}
		}
	}
	for j := range blocked {
		if blocked[j] {
			board[j] = false
		}
	}
	return
}

// railCutAt — перерезан ли путь на тайле (рядом разрушенная станция или узел).
func (w *World) railCutAt(s int, x, y float64) bool {
	m := w.m
	tx, ty := m.TileAt(x, y)
	for _, b := range w.Buildings {
		if b.Side != s || (b.Type != "rail_station" && b.Type != "rail_hub") || b.Operational() {
			continue
		}
		bx, by := m.TileAt(b.X, b.Y)
		if abs(bx-tx) <= 1 && abs(by-ty) <= 1 {
			return true
		}
	}
	return false
}

// findPath — путь по своей территории с учётом дорог и железной дороги (A*).
// Состояние поиска — тайл и «на поезде»: сесть и выйти можно только на ж/д тайле у работающей своей
// станции или узла, едет эшелон по соседним ж/д тайлам; поезд берётся, только если так быстрее.
// Возвращает точки пути, признак «участок по рельсам» и минуты ожидания перед участком.
func (w *World) findPath(s int, from, to Pt, roadKmh, offKmh float64, useRail bool) ([]Pt, []bool, []float64) {
	m := w.m
	start := w.tileOf(from.X, from.Y)
	goal := w.tileOf(to.X, to.Y)
	if start < 0 || goal < 0 || w.OwnerSide(goal) != s || m.Terrain[goal] != 1 {
		return nil, nil, nil
	}
	rules := w.cat.Rules
	var board, blocked []bool
	if useRail {
		board, blocked = w.railNodes(s)
		any := false
		for _, b := range board {
			if b {
				any = true
				break
			}
		}
		useRail = any
	}
	n := m.W * m.H
	nodes := n
	if useRail {
		nodes = 2 * n // второе полотно — состояние «на поезде»
	}
	g := make([]float32, nodes)
	prev := make([]int32, nodes)
	for i := range g {
		g[i] = float32(math.Inf(1))
		prev[i] = -1
	}
	fastest := roadKmh
	if useRail && rules.RailKmh > fastest {
		fastest = rules.RailKmh
	}
	g[start] = 0
	h := func(i int) float64 {
		cx, cy := m.TileCenter((i%n)%m.W, (i%n)/m.W)
		return dist(cx, cy, to.X, to.Y) / fastest
	}
	open := &pq{{start, h(start)}}
	closed := make([]bool, nodes)
	goalNode := goal
	// Открытые мосты с проездом: ребро между концами для пешей части поиска.
	bridgeTo := map[int]int{}
	for _, l := range w.bridgeLinks() {
		if w.bridgeOpen(s, l) {
			bridgeTo[l.a], bridgeTo[l.b] = l.b, l.a
		}
	}
	relax := func(from, to int, cost float64) {
		ng := g[from] + float32(cost)
		if ng < g[to] {
			g[to] = ng
			prev[to] = int32(from)
			heap.Push(open, pqItem{to, float64(ng) + h(to)})
		}
	}
	for open.Len() > 0 {
		it := heap.Pop(open).(pqItem)
		node := it.i
		if closed[node] {
			continue
		}
		closed[node] = true
		if node == goalNode {
			break
		}
		i := node % n
		train := node >= n
		x, y := i%m.W, i/m.W
		if useRail {
			// посадка и высадка
			if !train && board[i] && m.Flags[i]&4 != 0 {
				relax(node, i+n, rules.RailBoardMin/60)
			}
			if train && board[i] {
				relax(node, i, rules.RailAlightMin/60)
			}
		}
		if !train {
			if j, ok := bridgeTo[i]; ok && !closed[j] {
				bx, by := m.TileCenter(i%m.W, i/m.W)
				cx, cy := m.TileCenter(j%m.W, j/m.W)
				relax(node, j, dist(bx, by, cx, cy)/roadKmh)
			}
		}
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 || !m.In(x+dx, y+dy) {
					continue
				}
				j := m.Idx(x+dx, y+dy)
				if m.Terrain[j] != 1 || (w.OwnerSide(j) != s && j != start) {
					continue
				}
				diag := 1.0
				if dx != 0 && dy != 0 {
					diag = 1.414
				}
				if train {
					if m.Flags[j]&4 == 0 || blocked[j] || w.OwnerSide(j) != s {
						continue
					}
					relax(node, j+n, m.TileKm/rules.RailKmh*diag)
					continue
				}
				if closed[j] {
					continue
				}
				sp := offKmh
				if m.Flags[j]&2 != 0 && m.Flags[i]&2 != 0 {
					sp = roadKmh
				}
				relax(node, j, m.TileKm/sp*diag)
			}
		}
	}
	// Цель достигнута пешком или с поезда (на поезде в цель не приезжают: выход обязателен).
	if prev[goalNode] < 0 && goal != start {
		return nil, nil, nil
	}
	var rev []int
	for node := goalNode; node >= 0; node = int(prev[node]) {
		rev = append(rev, node)
		if node == start {
			break
		}
	}
	var path []Pt
	var rail []bool
	var wait []float64
	pending := 0.0
	for k := len(rev) - 2; k >= 0; k-- {
		a, b := rev[k+1], rev[k]
		ta, tb := a%n, b%n
		trainA, trainB := a >= n, b >= n
		if ta == tb && trainA != trainB {
			if trainB {
				pending += rules.RailBoardMin
			} else {
				pending += rules.RailAlightMin
			}
			continue
		}
		cx, cy := m.TileCenter(tb%m.W, tb/m.W)
		path = append(path, Pt{cx, cy})
		rail = append(rail, trainA && trainB)
		wait = append(wait, pending)
		pending = 0
	}
	if len(path) > 0 {
		path[len(path)-1] = to
	} else {
		path = append(path, to)
		rail = append(rail, false)
		wait = append(wait, pending)
		pending = 0
	}
	if pending > 0 { // выгрузка на последнем тайле
		path = append(path, to)
		rail = append(rail, false)
		wait = append(wait, pending)
	}
	return path, rail, wait
}

// MoveUnit отдаёт приказ на марш.
func (w *World) MoveUnit(s int, id uint32, to Pt) string {
	u, ok := w.Units[id]
	if !ok || u.Side != s {
		return "Нет такого юнита"
	}
	ut := w.cat.UnitByID[u.Type]
	path, rail, wait := w.findPath(s, Pt{u.X, u.Y}, to, ut.SpeedRoad, ut.SpeedOff, true)
	if path == nil {
		return "Нет пути по своей территории"
	}
	u.Path, u.PathRail, u.PathWait = path, rail, wait
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

// popLeg снимает пройденную точку пути.
func (u *Unit) popLeg() {
	u.Path = u.Path[1:]
	if len(u.PathRail) > 0 {
		u.PathRail = u.PathRail[1:]
	}
	if len(u.PathWait) > 0 {
		u.PathWait = u.PathWait[1:]
	}
}

// PathETA — сколько минут осталось ехать по пути юнита (рельсы, ожидание на станциях учитываются).
func PathETA(m *world.MapData, r data.Rules, u *Unit, ut *data.UnitType) float64 {
	x, y := u.X, u.Y
	t := 0.0
	for i, p := range u.Path {
		sp := ut.SpeedOff
		railLeg := i < len(u.PathRail) && u.PathRail[i]
		if railLeg {
			sp = r.RailKmh
		} else if tx, ty := m.TileAt(p.X, p.Y); m.In(tx, ty) && m.Flags[m.Idx(tx, ty)]&2 != 0 {
			sp = ut.SpeedRoad
		}
		t += dist(x, y, p.X, p.Y) / sp * 60
		if i < len(u.PathWait) {
			t += u.PathWait[i]
		}
		x, y = p.X, p.Y
	}
	return t
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
			if w.deadBridgeAhead(u) {
				w.bridgeCut(u, ut)
			}
			left := dtMin
			for left > 0 && len(u.Path) > 0 {
				t := u.Path[0]
				// Погрузка и выгрузка: стоим, пока не выйдет время ожидания.
				if len(u.PathWait) > 0 && u.PathWait[0] > 0 {
					d := math.Min(left, u.PathWait[0])
					u.PathWait[0] -= d
					left -= d
					continue
				}
				rail := len(u.PathRail) > 0 && u.PathRail[0]
				// Станцию или узел разбили: поезд дальше не идёт, остаток пути — пешим маршем.
				if rail && w.railCutAt(u.Side, t.X, t.Y) {
					w.railCut(u, ut)
					continue
				}
				i := w.tileOf(u.X, u.Y)
				sp := ut.SpeedOff
				if rail {
					sp = w.cat.Rules.RailKmh
				} else if i >= 0 && w.m.Flags[i]&2 != 0 {
					sp = ut.SpeedRoad
				}
				step := sp / 60 * left
				d := dist(u.X, u.Y, t.X, t.Y)
				if d <= step {
					u.X, u.Y = t.X, t.Y
					left -= d / (sp / 60)
					u.popLeg()
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

// railCut заменяет оставшийся путь юнита обычным маршем (рельсы перерезаны ударом).
func (w *World) railCut(u *Unit, ut *data.UnitType) {
	dest := u.Path[len(u.Path)-1]
	path, rail, wait := w.findPath(u.Side, Pt{u.X, u.Y}, dest, ut.SpeedRoad, ut.SpeedOff, false)
	w.Log(u.Side, 2, "Путь по железной дороге перерезан: "+ut.Name+" продолжает марш по дорогам")
	if path == nil {
		u.Path, u.PathRail, u.PathWait = nil, nil, nil
		return
	}
	u.Path, u.PathRail, u.PathWait = path, rail, wait
}

// bridgeCut пересчитывает путь юнита, когда мост впереди разрушен; нет другого пути — юнит останавливается у берега.
func (w *World) bridgeCut(u *Unit, ut *data.UnitType) {
	dest := u.Path[len(u.Path)-1]
	path, rail, wait := w.findPath(u.Side, Pt{u.X, u.Y}, dest, ut.SpeedRoad, ut.SpeedOff, true)
	if path == nil {
		u.Path, u.PathRail, u.PathWait = nil, nil, nil
		w.LogAt(u.Side, 2, "Мост разрушен, пути нет: "+ut.Name+" остановлена у берега", u.X, u.Y)
		return
	}
	u.Path, u.PathRail, u.PathWait = path, rail, wait
	w.LogAt(u.Side, 2, "Мост разрушен: "+ut.Name+" идёт в обход", u.X, u.Y)
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
