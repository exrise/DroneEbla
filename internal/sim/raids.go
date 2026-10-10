package sim

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Сводки налётов: попадания и сбитые цели по району собираются в одно сообщение через raidQuietMin минут тишины,
// вместо десятков отдельных «Попадание…» (они остаются в журнале на уровне 0).

const (
	raidCellKm   = 100.0
	raidQuietMin = 15.0
)

type raid struct {
	side   int
	cx, cy int
	sumX   float64
	sumY   float64
	n      int
	last   float64
	hits   map[string]int     // боеприпас → попаданий
	downed map[string]int     // боеприпас → сбито
	objs   map[string]float64 // здание → доля HP после последнего попадания
	dead   map[string]bool    // выведено из строя
}

func (w *World) raidAt(side int, x, y float64) *raid {
	k := [3]int{side, int(math.Floor(x / raidCellKm)), int(math.Floor(y / raidCellKm))}
	if w.raids == nil {
		w.raids = map[[3]int]*raid{}
	}
	r := w.raids[k]
	if r == nil {
		r = &raid{side: side, cx: k[1], cy: k[2], hits: map[string]int{}, downed: map[string]int{}, objs: map[string]float64{}, dead: map[string]bool{}}
		w.raids[k] = r
	}
	r.sumX += x
	r.sumY += y
	r.n++
	r.last = w.Time
	return r
}

func (w *World) raidHit(b *Building, munition string, frac float64, destroyed bool) {
	r := w.raidAt(b.Side, b.X, b.Y)
	r.hits[munition]++
	r.objs[b.Name] = frac
	if destroyed {
		r.dead[b.Name] = true
	}
}

func (w *World) raidDowned(side int, x, y float64, munition string) {
	r := w.raidAt(side, x, y)
	r.downed[munition]++
}

// flushRaids выдаёт сводки по налётам, в которых уже 15 минут тихо.
func (w *World) flushRaids() {
	if len(w.raids) == 0 {
		return
	}
	for k, r := range w.raids {
		if w.Time-r.last < raidQuietMin {
			continue
		}
		delete(w.raids, k)
		hits, downed := sum(r.hits), sum(r.downed)
		if hits == 0 && downed < 5 {
			continue // мелочь — достаточно записей журнала
		}
		x, y := r.sumX/float64(r.n), r.sumY/float64(r.n)
		w.LogAt(r.side, raidLevel(r), w.raidText(r, hits, downed, x, y), x, y)
	}
}

func raidLevel(r *raid) int {
	if len(r.dead) > 0 {
		return 2
	}
	return 1
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func joinCounts(m map[string]int) string {
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(a, b int) bool {
		if list[a].v != list[b].v {
			return list[a].v > list[b].v
		}
		return list[a].k < list[b].k
	})
	var parts []string
	for _, e := range list {
		parts = append(parts, fmt.Sprintf("%s ×%d", e.k, e.v))
	}
	return strings.Join(parts, ", ")
}

func (w *World) raidText(r *raid, hits, downed int, x, y float64) string {
	place := ""
	best := 80.0
	for _, c := range w.m.Cities {
		if d := dist(c.X, c.Y, x, y); d < best && c.Pop >= 100000 {
			best, place = d, c.Name
		}
	}
	head := "Налёт"
	if place != "" {
		head += " на район города " + place
	}
	var parts []string
	if hits > 0 {
		parts = append(parts, fmt.Sprintf("попаданий %d (%s)", hits, joinCounts(r.hits)))
	} else {
		parts = append(parts, "отбит")
	}
	if downed > 0 {
		parts = append(parts, fmt.Sprintf("сбито %d", downed))
	}
	text := head + ": " + strings.Join(parts, ", ")
	var dead, hurt []string
	for name, f := range r.objs {
		if r.dead[name] {
			dead = append(dead, name)
		} else {
			hurt = append(hurt, fmt.Sprintf("%s %.0f%%", name, f*100))
		}
	}
	sort.Strings(dead)
	sort.Strings(hurt)
	if len(dead) > 0 {
		text += "; выведено из строя: " + limitList(dead)
	}
	if len(hurt) > 0 {
		text += "; повреждены: " + limitList(hurt)
	}
	return text
}

func limitList(l []string) string {
	if len(l) <= 3 {
		return strings.Join(l, ", ")
	}
	return fmt.Sprintf("%s и ещё %d", strings.Join(l[:3], ", "), len(l)-3)
}
