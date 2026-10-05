package ai

import (
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

// source — готовая к пуску пусковая или площадка.
type source struct {
	id    uint32
	x, y  float64
	cap   int      // сколько можно запустить сейчас
	munit []string // чем можно стрелять
}

// target — цель из данных разведки.
type target struct {
	c     sim.Contact
	score float64
	need  int // залп, который должен прорвать известное прикрытие
	got   int // уже назначено в этом тике
}

// plan — подготовленный пуск.
type plan struct {
	src   source
	tgt   *target
	m     *data.MunitionType
	count int
	eta   float64 // минут полёта
}

// launchers собирает готовые пусковые стороны по View.
func (a *AI) launchers(v *sim.View) []source {
	var out []source
	for _, u := range v.Units {
		ut := a.cat.UnitByID[u.Type]
		if ut == nil || ut.Kind != "launcher" || u.State != sim.UnitDeployed || u.Reload > 0.01 {
			continue
		}
		out = append(out, source{id: u.ID, x: u.X, y: u.Y, cap: ut.Salvo, munit: ut.Munitions})
	}
	for i := range v.Buildings {
		b := &v.Buildings[i]
		bt := a.cat.BuildingByID[b.Type]
		if bt == nil || len(bt.Launch) == 0 || !b.Operational() || math.Floor(b.Budget) < 1 {
			continue
		}
		s := source{id: b.ID, x: b.X, y: b.Y, cap: int(math.Floor(b.Budget))}
		for _, m := range a.cat.Munitions {
			if data.SideIndex(m.Side) != a.side || m.Kind == "interceptor" {
				continue
			}
			for _, p := range bt.Launch {
				if p == m.Platform {
					s.munit = append(s.munit, m.ID)
				}
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// adCover — сколько каналов ПВО противника известно в радиусе действия у точки.
func (a *AI) adCover(v *sim.View, x, y float64) int {
	n := 0
	for _, c := range v.Contacts {
		if c.Kind != 1 || c.Type == "" {
			continue
		}
		ut := a.cat.UnitByID[c.Type]
		if ut == nil || ut.Kind != "ad" {
			continue
		}
		if dist(c.X, c.Y, x, y) <= ut.RangeKm+5 {
			n += ut.Channels
		}
	}
	return n
}

// targets ранжирует известные здания противника.
func (a *AI) targets(v *sim.View) []*target {
	var out []*target
	for _, c := range v.Contacts {
		if c.Kind != 0 || c.Type == "" {
			continue
		}
		wgt := a.cfg.StrikeWeights[c.Type]
		if wgt <= 0 || (c.HP >= 0 && c.HP < 0.25) {
			continue
		}
		if t, ok := a.lastHit[c.ID]; ok && v.Time-t < a.cfg.TargetCooldownMin {
			continue
		}
		cover := a.adCover(v, c.X, c.Y)
		out = append(out, &target{
			c:     c,
			score: wgt / (1 + 0.15*float64(cover)),
			need:  int(math.Ceil(a.cfg.SalvoBase + a.cfg.SalvoPerChannel*float64(cover))),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].c.ID < out[j].c.ID
	})
	return out
}

// strikes планирует удары и разведку.
func (a *AI) strikes(w *sim.World, v *sim.View) {
	if a.due("recon", v.Time, a.cfg.ReconEveryMin) {
		a.recon(w, v)
	}
	if !a.due("strike", v.Time, a.cfg.StrikeEveryMin) {
		return
	}
	srcs := a.launchers(v)
	tgts := a.targets(v)
	if len(srcs) == 0 || len(tgts) == 0 {
		return
	}
	if len(tgts) > 12 {
		tgts = tgts[:12]
	}
	stock := map[string]float64{}
	for k, n := range v.Stocks {
		stock[k] = n
	}
	var plans []plan
	for _, s := range srcs {
		if p, ok := a.plan(w, s, tgts, stock); ok {
			plans = append(plans, p)
		}
	}
	// Прилёт по одной цели — одновременно: ранние пуски задерживаем.
	maxETA := map[*target]float64{}
	for _, p := range plans {
		maxETA[p.tgt] = math.Max(maxETA[p.tgt], p.eta)
	}
	for _, p := range plans {
		delay := math.Min(90, maxETA[p.tgt]-p.eta)
		err := a.cmd(w, sim.Command{
			Kind: sim.CmdStrike, ID: p.src.id, Item: p.m.ID, Count: p.count,
			X: p.tgt.c.X, Y: p.tgt.c.Y, Delay: math.Floor(delay),
		})
		if err == "" {
			a.lastHit[p.tgt.c.ID] = v.Time
		}
	}
}

// plan подбирает для пусковой цель и боеприпас.
func (a *AI) plan(w *sim.World, s source, tgts []*target, stock map[string]float64) (plan, bool) {
	opts := map[string]bool{}
	for _, m := range s.munit {
		opts[m] = true
	}
	for _, t := range tgts {
		if t.got >= t.need*3/2 {
			continue
		}
		for _, id := range a.cfg.StrikeMunitions {
			m := a.cat.MunitionByID[id]
			if m == nil || !opts[id] || m.Kind == "recon" || stock[id] < 1 {
				continue
			}
			count := int(math.Min(math.Min(stock[id], float64(s.cap)), float64(t.need-t.got)))
			if count < 1 {
				count = 1
			}
			if t.need > 1 && float64(count) < math.Ceil(a.cfg.MinSalvoFrac*math.Min(float64(t.need), float64(s.cap))) {
				continue
			}
			sp := sim.StrikePlan{Source: s.id, Munition: id, Count: count, Target: sim.Pt{X: t.c.X, Y: t.c.Y}}
			if w.ValidateStrike(a.side, sp) != "" {
				continue
			}
			stock[id] -= float64(count)
			t.got += count
			eta := 0.0
			if m.SpeedKmh > 0 {
				eta = dist(s.x, s.y, t.c.X, t.c.Y) / m.SpeedKmh * 60
			}
			return plan{src: s, tgt: t, m: m, count: count, eta: eta}, true
		}
	}
	return plan{}, false
}

// recon посылает разведывательный БПЛА к самой ценной цели с устаревшими данными.
func (a *AI) recon(w *sim.World, v *sim.View) {
	if v.Time < a.reconPause {
		return
	}
	srcs := a.launchers(v)
	var recon []string
	for _, m := range a.cat.Munitions {
		if data.SideIndex(m.Side) == a.side && m.Kind == "recon" && v.Stocks[m.ID] >= 1 {
			recon = append(recon, m.ID)
		}
	}
	if len(srcs) == 0 || len(recon) == 0 {
		return
	}
	var cands []sim.Contact
	for _, c := range v.Contacts {
		if c.Kind != 0 || c.Type == "" || a.cfg.StrikeWeights[c.Type] <= 0 {
			continue
		}
		if t, ok := a.lastRecon[c.ID]; ok && v.Time-t < a.cfg.ReconStaleMin {
			continue
		}
		if c.Seen < 0 || v.Time-c.Seen > a.cfg.ReconStaleMin {
			cands = append(cands, c)
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		wi, wj := a.cfg.StrikeWeights[cands[i].Type], a.cfg.StrikeWeights[cands[j].Type]
		if wi != wj {
			return wi > wj
		}
		return cands[i].ID < cands[j].ID
	})
	for _, c := range cands {
		for _, s := range srcs {
			for _, id := range recon {
				has := false
				for _, m := range s.munit {
					has = has || m == id
				}
				if !has {
					continue
				}
				sp := sim.StrikePlan{Source: s.id, Munition: id, Count: 1, Target: sim.Pt{X: c.X, Y: c.Y}}
				if w.ValidateStrike(a.side, sp) != "" {
					continue
				}
				if a.cmd(w, sim.Command{Kind: sim.CmdStrike, ID: s.id, Item: id, Count: 1, X: c.X, Y: c.Y}) == "" {
					a.lastRecon[c.ID] = v.Time
					return
				}
			}
		}
	}
}
