package ai

import (
	"math"
	"sort"
	"strings"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
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

	// Госзаказ. Запись вида "новая|старая|самая старая[:N]" — семейство версий:
	// заказывается лучшая из открытых, заказы худших версий снимаются.
	orders := append([]sim.Order(nil), v.Orders...)
	for _, e := range c.Orders {
		spec, n := split(e)
		alts := strings.Split(spec, "|")
		best := -1
		for i, id := range alts {
			if v.Unlocked[id] {
				best = i
				break
			}
		}
		if best < 0 {
			continue
		}
		for i := len(orders) - 1; i >= 0; i-- {
			for _, old := range alts[best+1:] {
				if orders[i].Item == old && a.cmd(w, sim.Command{Kind: sim.CmdOrderDel, Int: i}) == "" {
					orders = append(orders[:i], orders[i+1:]...)
					break
				}
			}
		}
		id := alts[best]
		if limit := c.StockCap[id]; c.Smart && limit > 0 {
			// Дешёвые позиции не копятся сверх колпака: заказ снимается, пока запас не упадёт ниже 60% колпака,
			// и мощность идёт на ЗУР и ракеты (она делится между позициями поровну).
			held := v.Stocks[id]
			ordered := false
			for i := len(orders) - 1; i >= 0; i-- {
				if orders[i].Item != id {
					continue
				}
				ordered = true
				if held >= limit && a.cmd(w, sim.Command{Kind: sim.CmdOrderDel, Int: i}) == "" {
					orders = append(orders[:i], orders[i+1:]...)
				}
			}
			if held >= limit || (!ordered && held >= limit*0.6) {
				continue
			}
		}
		have := false
		for _, o := range orders {
			have = have || o.Item == id
		}
		if have || (n > 0 && a.ordersDone[id]) {
			continue
		}
		if a.cmd(w, sim.Command{Kind: sim.CmdOrderAdd, Item: id, Count: n}) == "" {
			a.ordersDone[id] = true
			orders = append(orders, sim.Order{Item: id})
		}
	}

	// Закупки за рубежом: одна за раз, если нужного не хватает.
	below := c.ImportBelow
	if below <= 0 {
		below = 300
	}
	pending := map[string]int{}
	for _, d := range v.Deliveries {
		pending[d.Item]++
	}
	par := 1
	if c.Smart && c.ImportParallel > 1 {
		par = c.ImportParallel // деньги есть — закупки идут пачками, а не по одной в полдня
	}
	for _, id := range c.Imports {
		for _, im := range a.cat.Sides[a.side].Imports {
			if im.ID != id {
				continue
			}
			reserve, limit := c.ImportReserve, below
			if c.Smart {
				reserve, limit = a.smartImportLimits(v, im, money, reserve, below)
			}
			maxPar := par
			if c.Smart && im.Item == "res:electronics" && c.RichMoney > 0 && money > c.RichMoney {
				maxPar *= 3 // денег много, а электроники нет — берём втрое больше партий в пути
			}
			for pending[im.Item] < maxPar && w.ImportAvailable(a.side, im) {
				price := w.ImportPrice(a.side, im)
				if !a.needItem(v, im, limit, pending[im.Item]) || money-price < reserve {
					break
				}
				if a.cmd(w, sim.Command{Kind: sim.CmdImport, Item: im.ID}) != "" {
					break
				}
				money -= price
				pending[im.Item]++
			}
		}
	}

	// Излишки топлива и стали (умный режим): продаём за деньги, пока не остался разумный запас.
	if c.Smart {
		for k, above := range c.SellAbove {
			for i, rk := range data.ResKeys {
				if rk == k && v.Res[i] > above {
					a.cmd(w, sim.Command{Kind: sim.CmdSell, Item: "res:" + k})
				}
			}
		}
	}

	// Мобилизация, когда на фронте не хватает людей.
	men := 0.0
	for d := 0; d < sim.NumDir; d++ {
		men += v.Front[d].Men
	}
	a.peakMen = math.Max(a.peakMen, men)
	if c.Smart {
		a.mobilizeSmart(w, v, men, money)
	} else if c.MobilizeBelow > 0 && men < c.MobilizeBelow {
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

	if c.Smart {
		a.fleet(w, v, money)
	}
	a.build(w, v, money)
}

// needItem — нужна ли закупка предмета (с учётом уже заказанных, но не прибывших партий).
func (a *AI) needItem(v *sim.View, im data.ImportOffer, below float64, pending int) bool {
	item := im.Item
	incoming := float64(pending) * im.Amount
	if len(item) > 4 && item[:4] == "res:" {
		for i, k := range data.ResKeys {
			if item == "res:"+k {
				return v.Res[i]+incoming < below
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
	return own+incoming < below/75 // для штучных предметов порог в десятки раз меньше
}

// mobilizeSmart: дешёвые меры без потерь морали (контрактники, наёмники) используются, пока есть деньги и
// фронт слабее своего максимума; тяжёлые — только при сильной нехватке людей.
func (a *AI) mobilizeSmart(w *sim.World, v *sim.View, men, money float64) {
	c := a.cfg
	frac := c.PeakMenFrac
	if frac <= 0 {
		frac = 0.9
	}
	low := (c.MobilizeBelow > 0 && men < c.MobilizeBelow) || men < a.peakMen*frac
	if !low {
		return
	}
	for _, mb := range a.cat.Sides[a.side].Mobilization {
		if w.MobilizationReady(a.side, mb) != "" {
			continue
		}
		cheap := mb.Morale >= 0 && mb.Labor <= 0.001
		switch {
		case cheap:
			if money-mb.Money < c.ImportReserve || (mb.Money > 0 && money < c.ContractMoney) {
				continue
			}
		default:
			// Крайние меры (удар по морали и выпуску): люди упали ниже 60% от максимума или ниже порога,
			// либо фронт отступает, а людей меньше максимума.
			losing := a.trend[0]+a.trend[1]+a.trend[2] < -3
			heavy := c.HeavyMenFrac
			if heavy <= 0 {
				heavy = 0.6
			}
			if !(men < a.peakMen*heavy || (c.MobilizeBelow > 0 && men < c.MobilizeBelow*0.6) || (losing && men < a.peakMen*0.95)) {
				continue
			}
		}
		if a.cmd(w, sim.Command{Kind: sim.CmdMobilize, Item: mb.ID}) == "" && !cheap {
			return // тяжёлая мера — не больше одной за заход
		}
	}
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

// fleet держит парк юнитов (ПВО, РЭБ, РЛС…) на целевом уровне: разбитые комплексы заказываются заново.
// Запись "новый|старый:N": N штук всех версий вместе (живых и ещё не поставленных).
func (a *AI) fleet(w *sim.World, v *sim.View, money float64) {
	if len(a.cfg.Fleet) == 0 || !a.due("fleet", v.Time, 30) {
		return
	}
	have := map[string]int{}
	for _, u := range v.Units {
		have[u.Type]++
	}
	ordered := map[string]int{}
	for _, o := range v.Orders {
		if o.Remaining > 0 {
			ordered[o.Item] += o.Remaining
		} else if o.Remaining < 0 {
			ordered[o.Item]++
		}
	}
	batch := a.cfg.FleetBatch
	if batch <= 0 {
		batch = 2
	}
	for _, e := range a.cfg.Fleet {
		spec, want := split(e)
		alts := strings.Split(spec, "|")
		best := ""
		total := 0
		for _, id := range alts {
			total += have[id] + ordered[id]
			if best == "" && v.Unlocked[id] {
				best = id
			}
		}
		if best == "" || total >= want || ordered[best] > 0 {
			continue
		}
		a.cmd(w, sim.Command{Kind: sim.CmdOrderAdd, Item: best, Count: min(want-total, batch)})
	}
}

// buildWhen — выполнено ли условие стройки.
func (a *AI) buildWhen(v *sim.View, when string) bool {
	switch when {
	case "power_low":
		return v.Power[0] < v.Power[1]*1.3 || v.Blackout > 0.03
	}
	return true
}

// build достраивает здания из списка конфига рядом с собственными объектами.
func (a *AI) build(w *sim.World, v *sim.View, money float64) {
	if len(a.cfg.Build) == 0 || !a.due("build", v.Time, a.cfg.BuildEveryMin) {
		return
	}
	var anchors []sim.Building
	count := map[string]int{}
	healthy := map[string]int{}
	for _, b := range v.Buildings {
		count[b.Type]++
		if b.Built < 1 || b.HP >= b.MaxHP*0.5 {
			healthy[b.Type]++ // целое или строящееся
		}
		if b.Built >= 1 && b.Type != "bridge" && b.Type != "oilfield" {
			anchors = append(anchors, b)
		}
	}
	if len(anchors) == 0 {
		return
	}
	per := 1
	if a.cfg.Smart && a.cfg.BuildPerCycle > 1 {
		per = a.cfg.BuildPerCycle
	}
	built := 0
	for _, bd := range a.cfg.Build {
		if !a.cfg.Smart && (bd.Healthy || bd.When != "") {
			continue
		}
		have := count[bd.Type]
		if bd.Healthy {
			have = healthy[bd.Type]
		}
		if have >= bd.Max || money < bd.MinMoney || !a.buildWhen(v, bd.When) {
			continue
		}
		pool := anchors
		maxR := 18.0
		if bt := a.cat.BuildingByID[bd.Type]; bt != nil && bt.NeedCityKm > 0 {
			// Заводы строят только возле своих городов: опорные точки — крупные города стороны.
			pool = a.cityAnchors(w, v)
			maxR = math.Max(1, bt.NeedCityKm-8)
			if len(pool) == 0 {
				continue
			}
		}
		for try := 0; try < 24; try++ {
			an := pool[(a.buildAt+try)%len(pool)]
			ang := a.rng.Float64() * 2 * math.Pi
			r := 6 + a.rng.Float64()*maxR
			x, y := an.X+math.Cos(ang)*r, an.Y+math.Sin(ang)*r
			if w.CanBuild(a.side, bd.Type, x, y) != "" {
				continue
			}
			if a.cmd(w, sim.Command{Kind: sim.CmdBuild, Item: bd.Type, X: x, Y: y}) == "" {
				a.buildAt += try + 1
				built++
				count[bd.Type]++
				healthy[bd.Type]++
				break
			}
		}
		if built >= per {
			return
		}
	}
}

// smartImportLimits — резерв денег и порог докупки для предложения im в умном режиме: когда ресурса почти нет,
// резерв снижается до критического, а при избытке денег запас электроники поднимается, чтобы деньги не лежали мёртвым грузом.
func (a *AI) smartImportLimits(v *sim.View, im data.ImportOffer, money, reserve, below float64) (float64, float64) {
	c := a.cfg
	if len(im.Item) <= 4 || im.Item[:4] != "res:" {
		return reserve, below
	}
	have := 0.0
	for i, k := range data.ResKeys {
		if im.Item == "res:"+k {
			have = v.Res[i]
		}
	}
	crit := c.ImportReserveCritical
	if crit <= 0 {
		crit = 150
	}
	if have < 10 && crit < reserve {
		reserve = crit
	}
	if c.RichMoney > 0 && money > c.RichMoney && im.Item == "res:electronics" {
		reserve = 0
		below = math.Max(below, c.RichElecBelow)
	}
	return reserve, below
}

// cityAnchors — крупные города стороны (опорные точки для заводов), в порядке убывания населения.
func (a *AI) cityAnchors(w *sim.World, v *sim.View) []sim.Building {
	m := w.Map()
	var out []sim.Building
	cities := append([]world.City(nil), m.Cities...)
	sort.SliceStable(cities, func(i, j int) bool { return cities[i].Pop > cities[j].Pop })
	for _, c := range cities {
		if c.Pop < a.cat.Rules.CityMinPop {
			continue
		}
		if tx, ty := m.TileAt(c.X, c.Y); m.In(tx, ty) && int(v.Owner[m.Idx(tx, ty)])-1 == a.side {
			out = append(out, sim.Building{X: c.X, Y: c.Y})
		}
	}
	return out
}
