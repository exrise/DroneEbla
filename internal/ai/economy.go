package ai

import (
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

// economy — исследования, госзаказ, закупки, мобилизация, пропаганда, стройка.
func (a *AI) economy(w *sim.World, v *sim.View) {
	c := a.cfg
	money := v.Res[data.ResMoney]

	// Исследование: из списка приоритетов, затем самое дешёвое доступное.
	if v.Research == "" {
		if id := a.pickResearch(w, v); id != "" {
			a.cmd(w, sim.Command{Kind: sim.CmdResearch, Item: id})
		}
	}
	// Финансирование науки по уровню казны.
	lvl := 0
	for i, th := range c.FundLevels {
		if money >= th {
			lvl = i + 1
		}
	}
	if lvl != v.ResFund {
		a.cmd(w, sim.Command{Kind: sim.CmdResFund, Int: lvl})
	}

	// Госзаказ.
	have := map[string]bool{}
	for _, o := range v.Orders {
		have[o.Item] = true
	}
	for _, e := range c.Orders {
		id, n := split(e)
		if !v.Unlocked[id] || have[id] || (n > 0 && a.ordersDone[id]) {
			continue
		}
		if a.cmd(w, sim.Command{Kind: sim.CmdOrderAdd, Item: id, Count: n}) == "" {
			a.ordersDone[id] = true
		}
	}

	// Закупки за рубежом: одна за раз, если нужного не хватает.
	below := c.ImportBelow
	if below <= 0 {
		below = 300
	}
	pending := map[string]bool{}
	for _, d := range v.Deliveries {
		pending[d.Item] = true
	}
	for _, id := range c.Imports {
		for _, im := range a.cat.Sides[a.side].Imports {
			if im.ID != id || pending[im.Item] || !w.ImportAvailable(a.side, im) {
				continue
			}
			if !a.needItem(v, im.Item, below) || money-w.ImportPrice(a.side, im) < c.ImportReserve {
				continue
			}
			if a.cmd(w, sim.Command{Kind: sim.CmdImport, Item: im.ID}) == "" {
				money -= w.ImportPrice(a.side, im)
				pending[im.Item] = true
			}
		}
	}

	// Мобилизация, когда на фронте не хватает людей.
	men := 0.0
	for d := 0; d < 3; d++ {
		men += v.Front[d].Men
	}
	if c.MobilizeBelow > 0 && men < c.MobilizeBelow {
		for i, mb := range a.cat.Sides[a.side].Mobilization {
			// Дорогую крайнюю меру — только при сильной нехватке.
			if i > 0 && men > c.MobilizeBelow*0.6 {
				break
			}
			if w.MobilizationReady(a.side, mb) == "" {
				a.cmd(w, sim.Command{Kind: sim.CmdMobilize, Item: mb.ID})
				break
			}
		}
	}

	// Информационная кампания при низкой морали.
	if v.Morale < c.PropagandaBelow && v.PropReady <= v.Time &&
		money > sim.PropagandaCost(a.cat.Rules, v.Morale)+200 {
		a.cmd(w, sim.Command{Kind: sim.CmdPropaganda})
	}

	a.build(w, v, money)
}

// needItem — нужна ли закупка предмета.
func (a *AI) needItem(v *sim.View, item string, below float64) bool {
	if len(item) > 4 && item[:4] == "res:" {
		for i, k := range data.ResKeys {
			if item == "res:"+k {
				return v.Res[i] < below
			}
		}
		return false
	}
	own := v.Stocks[item]
	for _, u := range v.Units {
		if u.Type == item {
			own++
		}
	}
	return own < below/75 // для штучных предметов порог в десятки раз меньше
}

// pickResearch выбирает следующее исследование.
func (a *AI) pickResearch(w *sim.World, v *sim.View) string {
	for _, id := range a.cfg.Research {
		if w.TechAvailable(a.side, id) {
			return id
		}
	}
	var left []*data.Tech
	for i := range a.cat.Tech[data.SideKeys[a.side]] {
		t := &a.cat.Tech[data.SideKeys[a.side]][i]
		if w.TechAvailable(a.side, t.ID) {
			left = append(left, t)
		}
	}
	if len(left) == 0 {
		return ""
	}
	sort.Slice(left, func(i, j int) bool {
		if left[i].Cost != left[j].Cost {
			return left[i].Cost < left[j].Cost
		}
		return left[i].ID < left[j].ID
	})
	return left[0].ID
}

// build достраивает здания из списка конфига рядом с собственными объектами.
func (a *AI) build(w *sim.World, v *sim.View, money float64) {
	if len(a.cfg.Build) == 0 || !a.due("build", v.Time, a.cfg.BuildEveryMin) {
		return
	}
	var anchors []sim.Building
	count := map[string]int{}
	for _, b := range v.Buildings {
		count[b.Type]++
		if b.Built >= 1 && b.Type != "bridge" && b.Type != "oilfield" {
			anchors = append(anchors, b)
		}
	}
	if len(anchors) == 0 {
		return
	}
	for _, bd := range a.cfg.Build {
		if count[bd.Type] >= bd.Max || money < bd.MinMoney {
			continue
		}
		for try := 0; try < 24; try++ {
			an := anchors[(a.buildAt+try)%len(anchors)]
			ang := a.rng.Float64() * 2 * math.Pi
			r := 6 + a.rng.Float64()*18
			x, y := an.X+math.Cos(ang)*r, an.Y+math.Sin(ang)*r
			if w.CanBuild(a.side, bd.Type, x, y) != "" {
				continue
			}
			if a.cmd(w, sim.Command{Kind: sim.CmdBuild, Item: bd.Type, X: x, Y: y}) == "" {
				a.buildAt += try + 1
				return
			}
		}
	}
}
