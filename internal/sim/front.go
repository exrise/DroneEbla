package sim

import (
	"fmt"
	"math"

	"github.com/exrise/droneebla/internal/data"
)

// updateFrontTiles пересчитывает фронтовые тайлы обеих сторон.
func (w *World) updateFrontTiles() {
	if w.m == nil {
		return
	}
	w.frontT[0], w.frontT[1] = w.frontT[0][:0], w.frontT[1][:0]
	for i := range w.Owner {
		if !w.groundTile(i) {
			continue
		}
		s := w.OwnerSide(i)
		front := false
		w.neighbors4(i, func(j int) {
			if w.groundTile(j) && w.Owner[j] != w.Owner[i] {
				front = true
			}
		})
		if front {
			w.frontT[s] = append(w.frontT[s], i)
		}
	}
}

// FrontTiles — фронтовые тайлы стороны.
func (w *World) FrontTiles(s int) []int { return w.frontT[s] }

// dirPower — боевая мощь пула направления (без снабжения).
func (w *World) dirPower(s, d int) float64 {
	sd := w.Sides[s]
	f := sd.Front[d]
	art := f.Artillery * 0.05
	if sd.Res[data.ResAmmo] < 1 {
		art *= 0.2
	}
	armor := f.Armor * 0.03
	if sd.Res[data.ResFuel] < 1 {
		armor *= 0.3
	}
	fpv := f.FPV * 0.003 * (1 + sd.eff("fpv_power"))
	return f.Men + armor + art + fpv
}

// supply — снабжение направления: доля целых логистических узлов.
func (w *World) supply(s, d int) float64 {
	cur, max := 0.0, 0.0
	for _, b := range w.Buildings {
		if b.Side != s {
			continue
		}
		bt := w.cat.BuildingByID[b.Type]
		if bt.Supply <= 0 || b.Built < 1 {
			continue
		}
		bd := b.Dir
		if bd < 0 {
			bd = w.PointDir(b.X, b.Y)
		}
		if bd != d {
			continue
		}
		cur += bt.Supply * b.frac()
		max += bt.Supply
	}
	if max == 0 {
		return 0.25
	}
	return clamp(cur/max, 0.25, 1)
}

// airSupport — бонус тактической авиации и потери от ПВО.
func (w *World) airSupport(s, d int, cx, cy float64, dtH float64) float64 {
	r := w.cat.Rules
	sd := w.Sides[s]
	planes := 0.0
	var fields []*Building
	for _, b := range w.Buildings {
		if b.Side != s || b.Aircraft == nil || b.Aircraft["tactical"] <= 0 || !b.Operational() {
			continue
		}
		if dist(b.X, b.Y, cx, cy) <= r.AirBonusKm {
			planes += b.Aircraft["tactical"] * b.frac()
			fields = append(fields, b)
		}
	}
	// Вражеское ПВО у фронта.
	ad := 0
	for _, u := range w.Units {
		if u.Side == s || u.State != UnitDeployed {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.Kind == "ad" && ut.RangeKm >= 20 && dist(u.X, u.Y, cx, cy) <= 120 {
			ad++
		}
	}
	supp := math.Min(0.8, 0.1*float64(ad))
	bonus := planes * r.AirBonusPerPlane * (1 + sd.eff("air_bonus")) * (1 - supp)
	// Потери самолётов.
	if ad > 0 && planes > 0 && len(fields) > 0 && w.War() {
		loss := r.AirLossPerAD * float64(ad) * dtH
		for _, b := range fields {
			share := b.Aircraft["tactical"] * b.frac() / planes
			b.Aircraft["tactical"] = math.Max(0, b.Aircraft["tactical"]-loss*share)
		}
	}
	return 1 + bonus
}

// front — шаг наземной войны.
func (w *World) front(dtMin float64) {
	r := w.cat.Rules
	w.FrontAcc += dtMin
	if w.FrontAcc < r.FrontStepMin {
		return
	}
	step := w.FrontAcc
	w.FrontAcc = 0
	w.frontSummary(step)
	dtH := step / 60
	w.updateFrontTiles()

	// Центры направлений и плотность сил.
	var dens [2][3]float64
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		var cnt [3]int
		var sx, sy [3]float64
		for _, i := range w.frontT[s] {
			d := w.TileDir(i)
			cnt[d]++
			cx, cy := w.m.TileCenter(i%w.m.W, i/w.m.W)
			sx[d] += cx
			sy[d] += cy
		}
		for d := 0; d < 3; d++ {
			f := &sd.Front[d]
			f.Tiles = cnt[d]
			f.Supply = w.supply(s, d)
			if cnt[d] > 0 {
				f.Air = w.airSupport(s, d, sx[d]/float64(cnt[d]), sy[d]/float64(cnt[d]), dtH)
			} else {
				f.Air = 1
			}
			mor := MoraleFront(r, sd.Morale)
			f.Power = w.dirPower(s, d) * f.Supply * mor * f.Air
			if cnt[d] > 0 {
				dens[s][d] = f.Power / float64(cnt[d])
			}
		}
	}
	if !w.War() {
		return
	}

	// Атаки по тайлам: для каждого тайла противника берётся лучшее
	// соотношение сил среди соседних атакующих тайлов.
	thr := r.FrontThreshold
	if thr <= 0 {
		thr = 1.25
	}
	type cap struct{ tile, side int }
	var captures []cap
	best := map[int]float64{}
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		attK := []float64{0, 0.5, 1.0}[sd.Posture]
		if attK == 0 {
			continue
		}
		e := 1 - s
		ed := w.Sides[e]
		defK := []float64{1.3, 1.0, 0.85}[ed.Posture]
		for k := range best {
			delete(best, k)
		}
		for _, i := range w.frontT[s] {
			d := w.TileDir(i)
			att := dens[s][d] * attK * w.mainEffort(s, i) * (0.75 + 0.5*w.rng.Float64())
			w.neighbors4(i, func(j int) {
				if !w.groundTile(j) || w.OwnerSide(j) != e {
					return
				}
				dj := w.TileDir(j)
				def := dens[e][dj] * defK * w.mainEffort(e, j) * w.terrain(j)
				if def <= 0 {
					def = 0.01
				}
				if ratio := att / def; ratio > best[j] {
					best[j] = ratio
				}
			})
		}
		for j, ratio := range best {
			if ratio <= thr {
				continue
			}
			w.Pressure[j] += float32(math.Min(0.25, r.FrontAttack*(ratio-thr)) * step / r.FrontStepMin)
			if w.Pressure[j] >= float32(r.FrontCapture) {
				captures = append(captures, cap{j, s})
			}
		}
	}
	// Затухание давления.
	for i := range w.Pressure {
		if w.Pressure[i] > 0 {
			w.Pressure[i] = float32(math.Max(0, float64(w.Pressure[i])-0.002*step/r.FrontStepMin))
		}
	}
	for _, c := range captures {
		if w.OwnerSide(c.tile) != c.side {
			w.captureTile(c.tile, c.side)
		}
	}

	// Потери: пропорциональны силе противника на направлении.
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		e := 1 - s
		for d := 0; d < 3; d++ {
			f := &sd.Front[d]
			if f.Tiles == 0 {
				continue
			}
			ep := w.Sides[e].Front[d].Power
			ratio := 1.0
			if f.Power > 0 {
				ratio = clamp(ep/f.Power, 0.2, 5)
			}
			k := r.FrontLoss * ratio * step / r.FrontStepMin * MoraleLosses(r, sd.Morale)
			if sd.Posture == PostureOffense {
				k *= 1.6
			} else if sd.Posture == PostureDefense {
				k *= 0.7
			}
			if w.Sides[e].Posture == PostureOffense {
				k *= 1.3
			}
			men := f.Men * k
			f.Men -= men
			f.Losses += men
			sd.LossAcc += men
			f.Armor -= f.Armor * k * 0.8
			f.Artillery -= f.Artillery * k * 0.5
			f.FPV -= f.FPV * math.Min(0.5, k*8)
		}
	}
}

// mainEffort — множитель направления главного удара.
func (w *World) mainEffort(s, i int) float64 {
	sd := w.Sides[s]
	if !sd.HasMain {
		return 1
	}
	cx, cy := w.m.TileCenter(i%w.m.W, i/w.m.W)
	if dist(cx, cy, sd.MainX, sd.MainY) <= w.cat.Rules.MainEffortKm {
		return w.cat.Rules.MainEffortMult
	}
	if w.PointDir(sd.MainX, sd.MainY) == w.TileDir(i) {
		return 0.8
	}
	return 1
}

// terrain — оборонительный множитель тайла.
func (w *World) terrain(i int) float64 {
	r := w.cat.Rules
	k := 1.0
	if w.m.Flags[i]&1 != 0 {
		k *= r.RiverDefense
	}
	if w.m.Flags[i]&8 != 0 {
		k *= r.UrbanDefense
	}
	if f := w.Fort[i]; f > 0 {
		s := w.OwnerSide(i)
		k *= 1 + float64(f)*(r.FortPerLevel+w.Sides[s].eff("fort_bonus"))
	}
	return k
}

// captureTile передаёт тайл стороне s.
func (w *World) captureTile(i, s int) {
	r := w.cat.Rules
	old := w.OwnerSide(i)
	w.Owner[i] = uint8(s + 1)
	d := w.TileDir(i)
	w.Sides[s].Front[d].Gained++
	if old >= 0 {
		w.Sides[old].Front[d].Lost++
	}
	w.Captures = append(w.Captures, Capture{Tile: int32(i), Side: int8(s), Time: float32(w.Time)})
	w.Pressure[i] = 0
	w.Fort[i] = 0
	delete(w.FortJobs, i)
	cx, cy := w.m.TileCenter(i%w.m.W, i/w.m.W)
	half := w.m.TileKm / 2
	for _, b := range w.Buildings {
		if math.Abs(b.X-cx) <= half && math.Abs(b.Y-cy) <= half && b.Side != s {
			b.HP *= 1 - r.CaptureDamage
			b.Side = s
			b.Masked = false
			w.LogAt(s, 1, "Захвачен объект: "+b.Name, b.X, b.Y)
			w.LogAt(old, 2, "Потерян объект: "+b.Name, b.X, b.Y)
			delete(w.Sides[s].Known, b.ID)
			w.Sides[old].Known[b.ID] = &Contact{ID: b.ID, Type: b.Type, X: b.X, Y: b.Y, Seen: w.Time, HP: b.frac(), Source: "потерян"}
		}
	}
	for id, u := range w.Units {
		if u.Side != s && w.tileOf(u.X, u.Y) == i {
			w.LogAt(u.Side, 2, "Юнит уничтожен при отступлении: "+w.cat.UnitByID[u.Type].Name, u.X, u.Y)
			delete(w.Units, id)
			delete(w.Sides[s].Known, id) // старая метка на захваченной территории не нужна
		}
	}
	if ci, ok := w.cityAt[i]; ok {
		c := w.m.Cities[ci]
		if c.Pop >= 20000 {
			k := r.MoraleCity * math.Max(0.3, float64(c.Pop)/500000)
			w.Sides[s].Morale = clamp(w.Sides[s].Morale+k, 0, 100)
			if old >= 0 {
				w.Sides[old].Morale = clamp(w.Sides[old].Morale-k, 0, 100)
				w.LogAt(old, 2, fmt.Sprintf("Потерян город: %s", c.Name), c.X, c.Y)
			}
			w.LogAt(s, 1, fmt.Sprintf("Взят город: %s", c.Name), c.X, c.Y)
		}
	}
}

// frontSummary раз в игровой час подводит итоги по направлениям.
func (w *World) frontSummary(step float64) {
	// Подсветка захватов живёт 3 игровых часа.
	keep := w.Captures[:0]
	for _, c := range w.Captures {
		if w.Time-float64(c.Time) < 180 {
			keep = append(keep, c)
		}
	}
	w.Captures = keep
	w.FrontHour += step
	if w.FrontHour < 60 {
		return
	}
	w.FrontHour = 0
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		text := ""
		lvl := 0
		for d := 0; d < 3; d++ {
			f := &sd.Front[d]
			f.GainedH, f.LostH = f.Gained, f.Lost
			f.Gained, f.Lost = 0, 0
			if f.GainedH == 0 && f.LostH == 0 {
				continue
			}
			if text != "" {
				text += "; "
			}
			text += fmt.Sprintf("%s: +%d / −%d км²", DirNames[d], f.GainedH*25, f.LostH*25)
			if f.LostH > f.GainedH {
				lvl = 1
			}
		}
		if text != "" {
			w.Log(s, lvl, "Фронт за час — "+text)
		}
	}
}
