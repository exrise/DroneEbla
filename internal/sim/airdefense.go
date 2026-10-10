package sim

import (
	"fmt"
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
)

// detectRange — дальность обнаружения сенсором цели данного класса.
func (w *World) detectRange(s int, radarKm, radarLow float64, class string, stealth float64) float64 {
	r := radarKm
	if class == "low" {
		r *= math.Min(1, radarLow*(1+w.Sides[s].eff("radar_low")))
	}
	return r * (1 - stealth)
}

// tracked — видит ли сторона s летящий объект p.
func (w *World) tracked(s int, p *Projectile) bool {
	if p.Side == s {
		return true
	}
	if p.Delay > 0 {
		return false
	}
	m := w.cat.MunitionByID[p.Munition]
	if m.Kind == "ballistic" {
		return true // пуск баллистики фиксируется сразу
	}
	idx := w.sensors()
	for _, r := range idx.radars[s] {
		if dist(r.x, r.y, p.X, p.Y) <= w.detectRange(s, r.km, r.low, m.Class, m.Stealth) {
			return true
		}
	}
	// Визуальное и акустическое наблюдение над своей территорией.
	if m.Class == "low" && w.sideOfPoint(p.X, p.Y) == s {
		watch := w.cat.Rules.AirWatchKm + w.Sides[s].eff("air_watch")
		n := int(math.Ceil(watch / sensorCell))
		cx, cy := int(math.Floor(p.X/sensorCell)), int(math.Floor(p.Y/sensorCell))
		for dx := -n; dx <= n; dx++ {
			for dy := -n; dy <= n; dy++ {
				for _, q := range idx.posts[s][[2]int{cx + dx, cy + dy}] {
					if dist(q.X, q.Y, p.X, p.Y) <= watch {
						return true
					}
				}
			}
		}
	}
	return false
}

// airDefense — автоматический перехват.
func (w *World) airDefense(dtMin float64) {
	if len(w.Projs) == 0 && len(w.Engs) == 0 {
		return
	}
	// Завершение перехватов.
	keep := w.Engs[:0]
	for _, e := range w.Engs {
		e.T -= dtMin
		if e.T > 0 {
			keep = append(keep, e)
			continue
		}
		if e.Bld != 0 {
			if b, ok := w.Buildings[e.Bld]; ok {
				b.Busy--
				if b.Busy < 0 {
					b.Busy = 0
				}
				if n := b.Aircraft["fighter"]; n >= 1 && w.rng.Float64() < w.cat.Rules.AircraftLoss {
					b.Aircraft["fighter"] = n - 1
					w.LogAt(b.Side, 1, "Потерян истребитель-перехватчик: "+b.Name, b.X, b.Y)
				}
			}
		} else if u, ok := w.Units[e.AD]; ok {
			u.Busy--
		}
		p, ok := w.Projs[e.Proj]
		if !ok {
			continue
		}
		p.Engaged--
		if w.rng.Float64() < e.Pk {
			m := w.cat.MunitionByID[p.Munition]
			if w.sideOfPoint(p.X, p.Y) == e.Side && m.Kind != "decoy" {
				w.addBonus(e.Side, w.branchOf(p.Munition), w.cat.Rules.TrophyPoints)
			}
			w.addBonus(e.Side, "ad", w.cat.Rules.ExperiencePoints)
			w.downed(e.Side, p, m.Name)
			w.groupDone(p.Group, false)
			delete(w.Projs, p.ID)
		}
	}
	w.Engs = keep

	// Видимые цели для каждой стороны.
	var tracks [2][]*Projectile
	for _, p := range w.Projs {
		if p.Delay > 0 {
			continue
		}
		e := 1 - p.Side
		if w.tracked(e, p) {
			tracks[e] = append(tracks[e], p)
		}
	}
	for _, u := range sortedUnits(w.Units) {
		if u.State != UnitDeployed {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.Kind != "ad" || u.Busy >= ut.Channels {
			continue
		}
		sd := w.Sides[u.Side]
		if u.Ready < 1 {
			continue
		}
		var cands []*Projectile
		for _, p := range tracks[u.Side] {
			m := w.cat.MunitionByID[p.Munition]
			if m.Class == "low" && ut.PkLow <= 0 || m.Class == "high" && ut.PkHigh <= 0 {
				continue
			}
			if !fireAllowed(u.Fire, m) {
				continue
			}
			limit := 1
			if m.Class == "high" {
				limit = 2
			}
			if p.Engaged >= limit {
				continue
			}
			// Своя РЛС комплекса тоже должна видеть цель (или сеть передаёт её
			// в пределах дальности поражения).
			if dist(u.X, u.Y, p.X, p.Y) > ut.RangeKm {
				continue
			}
			cands = append(cands, p)
		}
		sort.Slice(cands, func(a, b int) bool {
			return dist(u.X, u.Y, cands[a].X, cands[a].Y) < dist(u.X, u.Y, cands[b].X, cands[b].Y)
		})
		for _, p := range cands {
			if u.Busy >= ut.Channels {
				break
			}
			// Стреляем только тем, что заряжено на пусковых.
			if u.Ready < 1 {
				break
			}
			u.Ready--
			m := w.cat.MunitionByID[p.Munition]
			pk := ut.PkHigh
			if m.Class == "low" {
				pk = ut.PkLow + sd.eff("ad_pk_low")
			}
			pk += sd.eff("ad_pk")
			pk *= 1 - m.Evasion
			u.Busy++
			p.Engaged++
			w.Engs = append(w.Engs, Engagement{AD: u.ID, Proj: p.ID, T: ut.EngageSec / 60, Pk: clamp(pk, 0, 0.98), Side: u.Side})
		}
	}
	w.fighterIntercepts(tracks)
}

// fighterIntercepts — перехват низких целей истребителями с аэродромов, которые
// не находятся в зоне поражения вражеской ПВО.
func (w *World) fighterIntercepts(tracks [2][]*Projectile) {
	ids := make([]uint32, 0, 16)
	for id, b := range w.Buildings {
		if bt := w.cat.BuildingByID[b.Type]; bt.Intercept != nil && b.Aircraft["fighter"] >= 1 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	for _, id := range ids {
		b := w.Buildings[id]
		bt := w.cat.BuildingByID[b.Type]
		ic := bt.Intercept
		if !b.Operational() {
			continue
		}
		channels := int(math.Min(float64(ic.Channels), math.Floor(b.Aircraft["fighter"]/3)))
		if channels < 1 || b.Busy >= channels {
			continue
		}
		sd := w.Sides[b.Side]
		aam := "m_aam_" + data.SideKeys[b.Side]
		if sd.Stocks[aam] < 1 || w.underEnemyAD(b) {
			continue
		}
		var cands []*Projectile
		for _, p := range tracks[b.Side] {
			m := w.cat.MunitionByID[p.Munition]
			if m.Class != "low" || p.Engaged >= 1 || dist(b.X, b.Y, p.X, p.Y) > ic.Km {
				continue
			}
			cands = append(cands, p)
		}
		sort.Slice(cands, func(a, c int) bool {
			da, dc := dist(b.X, b.Y, cands[a].X, cands[a].Y), dist(b.X, b.Y, cands[c].X, cands[c].Y)
			if da != dc {
				return da < dc
			}
			return cands[a].ID < cands[c].ID
		})
		for _, p := range cands {
			if b.Busy >= channels || sd.Stocks[aam] < 1 {
				break
			}
			sd.Stocks[aam]--
			b.Busy++
			p.Engaged++
			pk := ic.PkLow + sd.eff("intercept_pk")
			w.Engs = append(w.Engs, Engagement{Bld: b.ID, Proj: p.ID, T: ic.EngageSec / 60, Pk: clamp(pk, 0, 0.98), Side: b.Side})
		}
	}
}

// underEnemyAD — аэродром в зоне поражения развёрнутого вражеского ПВО.
func (w *World) underEnemyAD(b *Building) bool {
	for _, u := range w.Units {
		if u.Side == b.Side || u.State != UnitDeployed {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.Kind == "ad" && ut.RangeKm > 0 && dist(u.X, u.Y, b.X, b.Y) <= ut.RangeKm {
			return true
		}
	}
	return false
}

// downed — сообщение о сбитии.
func (w *World) downed(by int, p *Projectile, name string) {
	w.LogAt(by, 0, "Сбит: "+name, p.X, p.Y)
	if p.Side != by && w.cat.MunitionByID[p.Munition].Kind != "recon" {
		w.raidDowned(by, p.X, p.Y, name)
	}
	m := w.cat.MunitionByID[p.Munition]
	if m.Kind == "recon" {
		w.LogAt(p.Side, 1, fmt.Sprintf("Потерян разведчик: %s", name), p.X, p.Y)
	}
}

func sortedUnits(m map[uint32]*Unit) []*Unit {
	out := make([]*Unit, 0, len(m))
	for _, u := range m {
		out = append(out, u)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}

// sensorCell — размер ячейки сетки наблюдательных постов, км.
const sensorCell = 25.0

type radarSens struct{ x, y, km, low float64 }

// sensorIndex — сенсоры сторон для обнаружения целей: РЛС развёрнутых юнитов и сетка постов наблюдения
// (свои здания и города). Строится один раз и сбрасывается, когда юниты или приказы меняют картину.
type sensorIndex struct {
	valid  bool
	radars [2][]radarSens
	posts  [2]map[[2]int][]Pt
}

func (w *World) sensors() *sensorIndex {
	idx := &w.sens
	if idx.valid {
		return idx
	}
	*idx = sensorIndex{valid: true}
	for s := 0; s < 2; s++ {
		idx.posts[s] = map[[2]int][]Pt{}
	}
	for _, u := range w.Units {
		if u.State != UnitDeployed {
			continue
		}
		if ut := w.cat.UnitByID[u.Type]; ut.RadarKm > 0 {
			idx.radars[u.Side] = append(idx.radars[u.Side], radarSens{u.X, u.Y, ut.RadarKm, ut.RadarLow})
		}
	}
	add := func(s int, x, y float64) {
		k := [2]int{int(math.Floor(x / sensorCell)), int(math.Floor(y / sensorCell))}
		idx.posts[s][k] = append(idx.posts[s][k], Pt{x, y})
	}
	for _, b := range w.Buildings {
		add(b.Side, b.X, b.Y)
	}
	for i, ci := range w.cityAt {
		if s := w.OwnerSide(i); s >= 0 {
			c := w.m.Cities[ci]
			add(s, c.X, c.Y)
		}
	}
	return idx
}

// fireAllowed — разрешает ли режим огня комплекса стрелять по этой цели: 1 — без дронов, разведчиков и ложных целей
// (берегут дорогие ракеты), 2 — только баллистика.
func fireAllowed(mode int, m *data.MunitionType) bool {
	switch mode {
	case 1:
		return m.Kind != "drone" && m.Kind != "decoy" && m.Kind != "recon"
	case 2:
		return m.Kind == "ballistic"
	}
	return true
}
