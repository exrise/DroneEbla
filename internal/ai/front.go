package ai

import (
	"fmt"
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

// front — распределение пополнений, позиция, укрепления.
func (a *AI) front(w *sim.World, v *sim.View) {
	if !a.due("front", v.Time, 30) {
		return
	}
	if a.cfg.Smart {
		a.frontSmart(w, v)
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

	base := a.cfg.WarPosture
	if !v.War {
		base = a.cfg.PrepPosture
	}
	for d := 0; d < 3; d++ {
		posture := base
		// Под сильным давлением направление уходит в глухую оборону.
		if v.War && a.cfg.DefensePressure > 0 && press[d] > a.cfg.DefensePressure {
			posture = 0
		}
		if v.Posture[d] != posture {
			a.cmd(w, sim.Command{Kind: sim.CmdPosture, Int: posture, Count: d + 1})
		}
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

// frontSmart — позиции по направлениям с гистерезисом и выдержкой, одно направление наступает, остальные держат
// линию; главный удар — по самому давимому нами тайлу на направлении наступления; приказы повторяются только при изменении.
func (a *AI) frontSmart(w *sim.World, v *sim.View) {
	c := a.cfg
	m := w.Map()
	now := v.Time
	var press [3]float64
	var cnt [3]int
	type tile struct {
		i int
		p float64
	}
	var hot []tile
	for _, i := range w.FrontTiles(a.side) {
		if i < 0 || i >= len(v.Pressure) {
			continue
		}
		d := w.TileDir(i)
		p := float64(v.Pressure[i]) / 255
		cnt[d]++
		press[d] += p
		if p > 0.05 && v.Fort[i] < 3 {
			hot = append(hot, tile{i, p})
		}
	}
	var pn [3]float64 // среднее давление на фронтовой тайл
	for d := 0; d < 3; d++ {
		if cnt[d] > 0 {
			pn[d] = press[d] / float64(cnt[d])
		}
		a.trend[d] = 0.7*a.trend[d] + 0.3*float64(v.Front[d].GainedH-v.Front[d].LostH)
	}

	// Пополнения идут туда, где враг сильнее давит и длиннее линия соприкосновения.
	vals := make([]float64, 3)
	for d := 0; d < 3; d++ {
		vals[d] = 10 + 0.5*float64(v.EnemyFront[d]) + 6*press[d]
	}
	if a.allocChanged(vals) {
		a.cmd(w, sim.Command{Kind: sim.CmdAlloc, Vals: vals})
		a.lastAlloc = append(a.lastAlloc[:0], vals...)
	}

	a.debugf("%s t=%.0f фронт: давление/тайл %.3f %.3f %.3f, тайлов %v, сила %.0f %.0f %.0f, тренд %.1f %.1f %.1f, позиции %v", data.SideKeys[a.side], now, pn[0], pn[1], pn[2], cnt, v.Front[0].Power, v.Front[1].Power, v.Front[2].Power, a.trend[0], a.trend[1], a.trend[2], v.Posture)
	// Позиции.
	dwell := c.PostureDwellMin
	if dwell <= 0 {
		dwell = 180
	}
	hi := c.DefenseTilePressure
	if hi <= 0 {
		hi = 0.12
	}
	want := [3]int{}
	if !v.War {
		for d := range want {
			want[d] = c.PrepPosture
		}
	} else {
		ceil := c.WarPosture // потолок: 0 — только оборона, 1 — без наступления, 2 — одно направление наступает
		best, bestDens := -1, 0.0
		for d := 0; d < 3; d++ {
			if pn[d] > hi {
				a.defending[d] = true
			} else if pn[d] < hi*0.6 {
				a.defending[d] = false
			}
			if a.defending[d] || now < a.noOffense[d] || cnt[d] == 0 || v.Front[d].Power <= 0 {
				continue
			}
			if dens := v.Front[d].Power / float64(cnt[d]); best < 0 || dens > bestDens {
				best, bestDens = d, dens
			}
		}
		for d := 0; d < 3; d++ {
			want[d] = sim.PostureActive
			switch {
			case ceil <= sim.PostureDefense || a.defending[d]:
				want[d] = sim.PostureDefense
			case d == best && ceil >= sim.PostureOffense:
				want[d] = sim.PostureOffense
			}
			// Наступление, которое не даёт тайлов, отменяется надолго: оно стоит +60% потерь.
			if v.Posture[d] == sim.PostureOffense && now-a.postureAt[d] > 360 && a.trend[d] < 0.2 {
				want[d] = sim.PostureActive
				a.noOffense[d] = now + 720
			}
			if v.Posture[d] == sim.PostureOffense && a.trend[d] < -1.5 {
				want[d] = sim.PostureActive
				a.noOffense[d] = now + 720
			}
		}
	}
	for d := 0; d < 3; d++ {
		if v.Posture[d] == want[d] {
			continue
		}
		// Уход в оборону под давлением — сразу; остальное не чаще, чем раз в dwell минут.
		urgent := want[d] == sim.PostureDefense && a.defending[d]
		if !urgent && now-a.postureAt[d] < dwell {
			continue
		}
		a.cmd(w, sim.Command{Kind: sim.CmdPosture, Int: want[d], Count: d + 1})
		a.postureAt[d] = now
	}

	a.mainEffort(w, v)

	// Укрепляем самые давимые тайлы; набор повторно отправляется только при изменении или раз в 2 часа.
	if c.FortTiles > 0 && len(hot) > 0 {
		sort.Slice(hot, func(i, j int) bool {
			if hot[i].p != hot[j].p {
				return hot[i].p > hot[j].p
			}
			return hot[i].i < hot[j].i
		})
		var pts []sim.Pt
		ids := make([]int, 0, c.FortTiles)
		for k := 0; k < len(hot) && k < c.FortTiles; k++ {
			i := hot[k].i
			ids = append(ids, i)
			pts = append(pts, sim.Pt{X: (float64(i%m.W) + 0.5) * m.TileKm, Y: (float64(i/m.W) + 0.5) * m.TileKm})
		}
		sort.Ints(ids)
		key := fmt.Sprint(ids)
		if key != a.lastFort || now-a.lastFortAt >= 120 {
			a.cmd(w, sim.Command{Kind: sim.CmdFort, Pts: pts})
			a.lastFort, a.lastFortAt = key, now
		}
	}
}

// allocChanged — заметно ли новое распределение отличается от отправленного.
func (a *AI) allocChanged(vals []float64) bool {
	if len(a.lastAlloc) != len(vals) {
		return true
	}
	sum, old := 0.0, 0.0
	for i := range vals {
		sum += vals[i]
		old += a.lastAlloc[i]
	}
	for i := range vals {
		if math.Abs(vals[i]/sum-a.lastAlloc[i]/old) > 0.03 {
			return true
		}
	}
	return false
}

// mainEffort ставит главный удар на самый давимый нами вражеский тайл направления наступления
// и снимает его, когда наступления нет.
func (a *AI) mainEffort(w *sim.World, v *sim.View) {
	every := a.cfg.MainEveryMin
	if every <= 0 {
		every = 240
	}
	off := -1
	for d := 0; d < 3; d++ {
		if v.Posture[d] == sim.PostureOffense {
			off = d
		}
	}
	if off < 0 {
		if v.HasMain {
			a.cmd(w, sim.Command{Kind: sim.CmdMainEffort, Int: 0})
		}
		return
	}
	if v.HasMain && v.Time-a.mainAt < every {
		return
	}
	enemy := uint8(2 - a.side)
	m := w.Map()
	bestI, bestP := -1, uint8(8)
	for i, o := range v.Owner {
		if o != enemy || v.Pressure[i] <= bestP || w.TileDir(i) != off {
			continue
		}
		bestI, bestP = i, v.Pressure[i]
	}
	if bestI < 0 {
		return
	}
	a.cmd(w, sim.Command{Kind: sim.CmdMainEffort, Int: 1,
		X: (float64(bestI%m.W) + 0.5) * m.TileKm, Y: (float64(bestI/m.W) + 0.5) * m.TileKm})
	a.mainAt = v.Time
}
