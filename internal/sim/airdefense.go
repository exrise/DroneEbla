package sim

import (
	"fmt"
	"math"
	"sort"
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
	for _, u := range w.Units {
		if u.Side != s || u.State != UnitDeployed {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.RadarKm <= 0 {
			continue
		}
		if dist(u.X, u.Y, p.X, p.Y) <= w.detectRange(s, ut.RadarKm, ut.RadarLow, m.Class, m.Stealth) {
			return true
		}
	}
	// Визуальное и акустическое наблюдение над своей территорией.
	if m.Class == "low" && w.sideOfPoint(p.X, p.Y) == s {
		watch := w.cat.Rules.AirWatchKm + w.Sides[s].eff("air_watch")
		for _, b := range w.Buildings {
			if b.Side == s && dist(b.X, b.Y, p.X, p.Y) <= watch {
				return true
			}
		}
		for i, ci := range w.cityAt {
			if w.OwnerSide(i) == s {
				c := w.m.Cities[ci]
				if dist(c.X, c.Y, p.X, p.Y) <= watch {
					return true
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
		if u, ok := w.Units[e.AD]; ok {
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
			if m.Kind == "ballistic" {
				pk *= 1 - w.Sides[p.Side].eff("ballistic_evasion")
			}
			u.Busy++
			p.Engaged++
			w.Engs = append(w.Engs, Engagement{AD: u.ID, Proj: p.ID, T: ut.EngageSec / 60, Pk: clamp(pk, 0, 0.98), Side: u.Side})
		}
	}
}

// downed — сообщение о сбитии.
func (w *World) downed(by int, p *Projectile, name string) {
	w.LogAt(by, 0, "Сбит: "+name, p.X, p.Y)
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
