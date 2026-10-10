package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

var tabNames = []string{"Фронт", "Госзаказ", "Арсенал", "Стройка", "Наука", "Импорт", "Разведка", "Журнал", "Задания"}

func (g *Game) drawLeftPanel() {
	u := &g.ui
	u.Panel(6, topH+4, leftW-6, u.H-topH-10)
	// Вкладки: две строки (5 и 4), обе от левого до правого края содержимого (x=22…leftW-26).
	const tx, tgap = 22, 4
	rowW := leftW - 48
	for i, name := range tabNames {
		// Одинаковая ширина вкладок в обоих рядах; неполный ряд — по центру.
		k := i
		bw := (rowW - 4*tgap) / 5
		off := 0
		if i >= 5 {
			k = i - 5
			off = (rowW - (len(tabNames)-5)*bw - (len(tabNames)-6)*tgap) / 2
		}
		x := tx + off + k*(bw+tgap)
		y := topH + 14 + (i/5)*30
		if i == 8 && g.view != nil && len(g.view.Sanctions) > 0 {
			name = "Санкции"
		}
		if u.ButtonState(x, y, bw, 26, name, g.tab == i, true) {
			g.tab = i
		}
		if i == 4 && g.view != nil {
			g.drawResearchBadge(x, y, bw)
		}
	}
	top := topH + 14 + 2*30 + 8
	area := image.Rect(10, top, leftW-4, u.H-14)
	key := tabNames[g.tab]
	sc := g.scroll[key]
	if image.Pt(u.in.mx, u.in.my).In(area) && u.in.wheel != 0 {
		sc -= u.in.wheel * 40
		u.in.wheel = 0
	}
	if sc < 0 {
		sc = 0
	}
	old := u.screen
	u.screen = u.sub(area.Min.X, area.Min.Y, area.Dx(), area.Dy())
	u.clip = area
	y0 := float64(top+8) - sc
	var bottom int
	x, w := 22, leftW-48
	switch g.tab {
	case 0:
		bottom = g.tabFront(x, int(y0), w)
	case 1:
		bottom = g.tabOrders(x, int(y0), w)
	case 2:
		bottom = g.tabArsenal(x, int(y0), w)
	case 3:
		bottom = g.tabBuild(x, int(y0), w)
	case 4:
		bottom = g.tabScience(x, int(y0), w)
	case 5:
		bottom = g.tabImport(x, int(y0), w)
	case 6:
		bottom = g.tabIntel(x, int(y0), w)
	case 7:
		bottom = g.tabLog(x, int(y0), w)
	case 8:
		bottom = g.tabMissions(x, int(y0), w)
	}
	u.screen = old
	u.clip = image.Rectangle{}
	content := float64(bottom) + sc - float64(top+8)
	maxSc := math.Max(0, content-float64(area.Dy())+30)
	g.scroll[key] = math.Min(sc, maxSc)
}

func (g *Game) header(s string, x, y int) int {
	drawBold(g.ui.screen, s, float64(x), float64(y), 16, colAccent, 0)
	return y + 26
}

func (g *Game) label(s string, x, y int, c color.Color) int {
	drawText(g.ui.screen, s, float64(x), float64(y), 14, c, 0)
	return y + 20
}

func (g *Game) para(s string, x, y, w int, c color.Color) int {
	for _, l := range wrap(s, 13, float64(w)) {
		drawText(g.ui.screen, l, float64(x), float64(y), 13, c, 0)
		y += 17
	}
	return y
}

func (g *Game) kv(k, v string, x, y, w int, c color.Color) int {
	drawText(g.ui.screen, k, float64(x), float64(y), 14, colDim, 0)
	drawText(g.ui.screen, v, float64(x+w), float64(y), 14, c, 2)
	return y + 19
}

func (g *Game) centerOn(x, y float64) {
	g.cam.CX, g.cam.CY = x, y
	if g.cam.Z < 1.5 {
		g.cam.Z = 1.5
	}
}

// ---------------------------------------------------------------------

const postureTip = "Оборона: меньше потерь и расход снарядов, фронт не продвигается. Активная оборона: локальные атаки. Наступление: больше потерь и расход, давление по всей линии направления."

func (g *Game) tabFront(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Позиция: все направления", x, y)
	bw := (w - 8) / 3
	same := true
	for d := 1; d < sim.NumDir; d++ {
		same = same && v.Posture[d] == v.Posture[0]
	}
	u.rowSize = fitRow(sim.PostureNames[:], bw, 26)
	for p := 0; p < 3; p++ {
		if u.ButtonState(x+p*(bw+4), y, bw, 26, sim.PostureNames[p], same && v.Posture[0] == p, true) {
			g.sess.Send(sim.Command{Kind: sim.CmdPosture, Int: p})
		}
	}
	u.rowSize = 0
	u.Tooltip(x, y, w, 26, postureTip)
	y += 34
	y = g.header("Главный удар", x, y)
	if v.HasMain {
		y = g.label(fmt.Sprintf("Задан: %s направление", sim.DirNames[g.dirAt(v.MainX, v.MainY)]), x, y, colText)
	} else {
		y = g.label("Не задан — силы распределены равномерно", x, y, colDim)
	}
	if u.ButtonState(x, y, w/2-4, 26, "Указать на карте", g.mode == modeMain, true) {
		g.mode = modeMain
	}
	if u.ButtonState(x+w/2+4, y, w/2-4, 26, "Снять", false, v.HasMain) {
		g.sess.Send(sim.Command{Kind: sim.CmdMainEffort, Int: 0})
	}
	u.Tooltip(x, y, w, 26, fmt.Sprintf("В радиусе %.0f км от метки сила ×%.1f, остальная часть того же направления ослабевает.", g.cat.Rules.MainEffortKm, g.cat.Rules.MainEffortMult))
	y += 36
	y = g.header("Направления", x, y)
	y = g.frontTable(x, y, w)
	y = g.para("Позиция: О — оборона, А — активная оборона, Н — наступление.", x, y, w, colDim)
	y += 6
	if len(v.VictoryCities) > 0 {
		y = g.header("Победа России", x, y)
		y = g.para("Нужно одновременно удерживать все города:", x, y, w, colDim)
		for _, vc := range v.VictoryCities {
			who, c := "у Украины", colGood
			if vc.Owner == data.RU {
				who, c = "у России", colBad
			}
			if v.Side == data.RU && vc.Owner == data.RU {
				c = colGood
			} else if v.Side == data.RU {
				c = colText
			}
			y = g.kv(vc.Name, who, x, y, w, c)
		}
		y += 6
	}
	y = g.header("Мораль", x, y)
	y = g.para(g.moraleEffects(), x, y, w, colDim)
	{
		r := g.cat.Rules
		cost := sim.PropagandaCost(r, v.Morale)
		gain := sim.PropagandaGain(r, v.Morale)
		wait := v.PropReady - v.Time
		lbl := fmt.Sprintf("Информационная кампания: +%.1f за %.0f", gain, cost)
		ok := wait <= 0 && v.Res[data.ResMoney] >= cost
		if wait > 0 {
			lbl = "Кампания доступна через " + fmtMin(wait)
		}
		if u.ButtonState(x, y, w, 26, lbl, false, ok) {
			g.sess.Send(sim.Command{Kind: sim.CmdPropaganda})
		}
		u.Tooltip(x, y, w, 26, fmt.Sprintf("Деньги в обмен на мораль. Цена растёт с моралью, прирост убывает; перезарядка %.0f ч.", r.PropagandaCooldownH))
		y += 34
	}
	y = g.header("Пополнение людьми", x, y)
	y = g.label(fmt.Sprintf("Мобилизационный резерв: %.0f тыс.", v.People), x, y, colText)
	for _, mb := range g.cat.Sides[v.Side].Mobilization {
		reason := g.mobReady(mb)
		lbl := mb.Name
		if u.ButtonState(x, y, w, 26, lbl, false, reason == "") {
			g.sess.Send(sim.Command{Kind: sim.CmdMobilize, Item: mb.ID})
		}
		tip := fmt.Sprintf("+%.0f тыс. человек на фронт. Мораль %+.0f, выпуск заводов −%.0f%%.", mb.Men, mb.Morale, mb.Labor*100)
		if mb.Foreign {
			tip = fmt.Sprintf("+%.0f тыс. наёмников на фронт (распределяются по направлениям). Мобилизационный резерв, мораль и выпуск заводов не затрагиваются; платите деньгами. Можно нанять до %d раз.", mb.Men, mb.Limit)
		}
		if mb.Money > 0 {
			tip += fmt.Sprintf(" Стоимость %.0f.", mb.Money)
		}
		if reason != "" {
			tip += "\n" + reason
		}
		u.Tooltip(x, y, w, 26, tip)
		y += 30
	}
	if v.Storage.Armor > 0 || v.Storage.Artillery > 0 {
		y += 6
		y = g.header("Техника на хранении", x, y)
		y = g.kv("Бронетехника", fmt.Sprintf("%.0f", v.Storage.Armor), x, y, w, colText)
		y = g.kv("Артиллерия", fmt.Sprintf("%.0f", v.Storage.Artillery), x, y, w, colText)
		y = g.para("Расконсервация заказывается в госзаказе.", x, y, w, colDim)
	}
	y += 8
	y = g.header("Укрепления", x, y)
	y = g.para(fmt.Sprintf("Каждый уровень даёт +%.0f%% к обороне тайла (до 3 уровней). Стоимость тайла: %s, %.0f ч.", g.cat.Rules.FortPerLevel*100, resText(data.ToRes(g.cat.Rules.FortCost)), g.cat.Rules.FortHours), x, y, w, colDim)
	if u.ButtonState(x, y, w, 26, "Рисовать укрепления на карте", g.mode == modeFort, true) {
		g.mode = modeFort
	}
	return y + 34
}

func (g *Game) dirAt(x, y float64) int {
	return sim.DirAt(g.m, g.cat.Rules.Directions, x, y)
}

func (g *Game) alloc(d int, delta float64) {
	a := g.view.Alloc
	a[d] = math.Max(0, a[d]+delta)
	g.sess.Send(sim.Command{Kind: sim.CmdAlloc, Vals: a[:]})
}

func (g *Game) mobReady(mb data.Mobilization) string {
	v := g.view
	if mb.Limit > 0 && v.MobUsed[mb.ID] >= mb.Limit {
		return "Уже использовано"
	}
	if t := v.MobReady[mb.ID]; t > v.Time {
		return "Будет доступно через " + fmtMin(t-v.Time)
	}
	if !mb.Foreign && v.People < mb.Men {
		return "Исчерпан резерв"
	}
	if v.Res[data.ResMoney] < mb.Money {
		return "Не хватает денег"
	}
	return ""
}

func resText(r data.Res) string {
	s := ""
	for i, x := range r {
		if x > 0 {
			if s != "" {
				s += ", "
			}
			s += fmt.Sprintf("%s %g", data.ResNames[i], math.Round(x*10)/10)
		}
	}
	if s == "" {
		return "бесплатно"
	}
	return s
}

// ---------------------------------------------------------------------

var capNames = map[string]string{data.CapAir: "Ракеты и ЗУР", data.CapDrone: "Дроны", data.CapGround: "Техника и комплексы"}

func (g *Game) tabOrders(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Производственные мощности", x, y)
	for _, c := range []string{data.CapAir, data.CapDrone, data.CapGround} {
		y = g.kv(capNames[c], fmt.Sprintf("%.1f очков/ч", v.Capacity[c]), x, y, w, colText)
	}
	y = g.para("Мощность категории делится поровну между позициями госзаказа этой категории. Зависит от целостности заводов, энергии и рабочей силы.", x, y, w, colDim)
	y += 6
	y = g.header("Госзаказ", x, y)
	if len(v.Orders) == 0 {
		y = g.label("Пусто — добавьте позиции ниже", x, y, colDim)
	}
	for i, o := range v.Orders {
		cost, _, pts, _ := g.cat.ItemCost(o.Item)
		name := g.cat.ItemName(o.Item)
		rem := "∞"
		if o.Remaining >= 0 {
			rem = fmt.Sprintf("%d", o.Remaining)
		}
		drawText(u.screen, fitText(name, 14, float64(w-110)), float64(x), float64(y), 14, colText, 0)
		drawText(u.screen, rem, float64(x+w-92), float64(y), 14, colAccent, 2)
		if u.Button(x+w-86, y-2, 26, 20, "▲") {
			g.sess.Send(sim.Command{Kind: sim.CmdOrderMove, Int: i, Count: -1})
		}
		if u.Button(x+w-58, y-2, 26, 20, "▼") {
			g.sess.Send(sim.Command{Kind: sim.CmdOrderMove, Int: i, Count: 1})
		}
		if u.Button(x+w-30, y-2, 26, 20, "×") {
			g.sess.Send(sim.Command{Kind: sim.CmdOrderDel, Int: i})
		}
		y += 20
		c := colGood
		if o.Stalled {
			c = colBad
		}
		u.Bar(x, y, w-90, 8, o.Progress/math.Max(pts, 0.001), c)
		if o.Stalled {
			drawText(u.screen, "нет ресурсов", float64(x+w-86), float64(y-4), 12, colBad, 0)
		}
		u.Tooltip(x, y-20, w-90, 30, fmt.Sprintf("%s\nСтоимость: %s, мощность %.1f очков", name, resText(cost), pts))
		y += 16
	}
	y += 8
	y = g.header("Добавить в госзаказ", x, y)
	type item struct{ id, cat string }
	var items []item
	side := data.SideKeys[v.Side]
	for _, m := range g.cat.Munitions {
		if m.Side == side && v.Unlocked[m.ID] {
			items = append(items, item{m.ID, m.Cap})
		}
	}
	for _, un := range g.cat.Units {
		if un.Side == side && v.Unlocked[un.ID] {
			items = append(items, item{un.ID, un.Cap})
		}
	}
	for _, f := range g.cat.Front {
		if v.Unlocked[f.ID] {
			items = append(items, item{f.ID, f.Cap})
		}
	}
	for _, c := range []string{data.CapAir, data.CapDrone, data.CapGround} {
		drawBold(u.screen, capNames[c], float64(x), float64(y), 14, colText, 0)
		y += 22
		for _, it := range items {
			if it.cat != c {
				continue
			}
			cost, _, pts, _ := g.cat.ItemCost(it.id)
			drawText(u.screen, fitText(g.cat.ItemName(it.id), 13, float64(w-130)), float64(x+6), float64(y), 13, colText, 0)
			u.Tooltip(x, y, w-130, 18, fmt.Sprintf("%s\nСтоимость: %s\nМощность: %.1f очков\n%s", g.cat.ItemName(it.id), resText(cost), pts, g.itemDesc(it.id)))
			if u.Button(x+w-124, y-2, 38, 20, "+1") {
				g.sess.Send(sim.Command{Kind: sim.CmdOrderAdd, Item: it.id, Count: 1})
			}
			if u.Button(x+w-82, y-2, 40, 20, "+10") {
				g.sess.Send(sim.Command{Kind: sim.CmdOrderAdd, Item: it.id, Count: 10})
			}
			if u.Button(x+w-38, y-2, 38, 20, "∞") {
				g.sess.Send(sim.Command{Kind: sim.CmdOrderAdd, Item: it.id, Count: 0})
			}
			y += 22
		}
		y += 6
	}
	return y
}

func (g *Game) itemDesc(id string) string {
	if m := g.cat.MunitionByID[id]; m != nil {
		s := ""
		if m.Kind != "interceptor" {
			s = fmt.Sprintf("Дальность %.0f км, скорость %.0f км/ч, урон %.0f, точность %.0f%%", m.RangeKm, m.SpeedKmh, m.Damage, m.Accuracy*100)
			if m.Class == "high" {
				s += ", высокая/баллистическая цель"
			} else {
				s += ", низколетящая цель"
			}
			if m.GPS {
				s += ", уязвим к РЭБ"
			}
		}
		return s + " " + m.Desc
	}
	if un := g.cat.UnitByID[id]; un != nil {
		s := un.Desc
		if un.Kind == "ad" {
			s = fmt.Sprintf("Дальность %.0f км, РЛС %.0f км, каналов %d, вероятность поражения: низкие %.0f%%, высокие %.0f%%. ", un.RangeKm, un.RadarKm, un.Channels, un.PkLow*100, un.PkHigh*100) + s
		}
		return s
	}
	return ""
}

// ---------------------------------------------------------------------

func (g *Game) tabArsenal(x, y, w int) int {
	v := g.view
	side := data.SideKeys[v.Side]
	y = g.header("Ударные средства", x, y)
	for _, m := range g.cat.Munitions {
		if m.Side != side || m.Kind == "interceptor" {
			continue
		}
		n := v.Stocks[m.ID]
		if n <= 0 && !v.Unlocked[m.ID] {
			continue
		}
		y = g.clickRow(m.Name, fmt.Sprintf("%.0f", n), x, y, w, colText, "mun:"+m.ID, func() []cand { return g.munitionSources(m.ID) })
		g.ui.Tooltip(x, y-19, w, 19, g.itemDesc(m.ID)+"\nКлик — выбрать следующий источник пуска, готовый к залпу.")
	}
	y += 8
	y = g.header("Зенитные ракеты", x, y)
	for _, m := range g.cat.Munitions {
		if m.Side != side || m.Kind != "interceptor" {
			continue
		}
		n := v.Stocks[m.ID]
		c := colText
		thr := v.LowInterceptor[m.ID]
		val := fmt.Sprintf("%.0f", n)
		switch {
		case thr > 0 && n < 1:
			c, val = colBad, "нет"
		case thr > 0 && n <= thr:
			c = colBad
		case n < 20:
			c = colWarn
		}
		y = g.clickRow(m.Name, val, x, y, w, c, "int:"+m.ID, func() []cand { return g.interceptorUnits(m.ID) })
	}
	// Автозаказ ЗУР: когда запас ниже порога, партия сама встаёт в госзаказ.
	keys := make([]string, 0, len(v.LowInterceptor))
	for id := range v.LowInterceptor {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		y += 4
		y = g.para("Автозаказ: когда запас ЗУР упадёт до порога, партия встанет в госзаказ сама.", x, y, w, colDim)
		for _, id := range keys {
			m := g.cat.MunitionByID[id]
			on := v.KeepStock[id]
			lbl := fmt.Sprintf("Держать запас %s (≤ %.0f)", m.Short, v.LowInterceptor[id])
			if g.ui.ButtonState(x, y, w, 26, lbl, on, true) {
				k := 1
				if on {
					k = 0
				}
				g.sess.Send(sim.Command{Kind: sim.CmdKeepStock, Item: id, Int: k})
			}
			g.ui.Tooltip(x, y, w, 26, m.Name+": при запасе не больше порога в госзаказ добавляется партия ЗУР.")
			y += 30
		}
	}
	y += 8
	y = g.header("Авиация", x, y)
	tac, str := 0.0, 0.0
	for _, b := range v.Buildings {
		tac += b.Aircraft["tactical"]
		str += b.Aircraft["strategic"]
	}
	y = g.clickRow("Тактическая авиация", fmt.Sprintf("%.0f", tac), x, y, w, colText, "air:tactical", func() []cand { return g.airfields("tactical") })
	y = g.clickRow("Дальняя авиация", fmt.Sprintf("%.0f", str), x, y, w, colText, "air:strategic", func() []cand { return g.airfields("strategic") })
	y += 8
	y = g.header("Мобильные комплексы", x, y)
	count := map[string]int{}
	for _, un := range v.Units {
		count[un.Type]++
	}
	var ids []string
	for id := range count {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		y = g.clickRow(g.uName(id), fmt.Sprintf("%d", count[id]), x, y, w, colText, "unit:"+id, func() []cand { return g.unitsOfType(id) })
	}
	return y
}

// cand — кандидат для выбора по клику в арсенале.
type cand struct {
	sel  Selection
	x, y float64
}

// clickRow — строка арсенала: клик выбирает следующий юнит или источник пуска
// из списка (ближайший к центру экрана из ещё не показанных).
func (g *Game) clickRow(k, val string, x, y, w int, c color.Color, key string, list func() []cand) int {
	u := &g.ui
	if u.mouseIn(x-2, y-2, w+4, 20) {
		u.rowHi(float64(x-4), float64(y-2), float64(w+8), 20)
	}
	if u.clicked(x-2, y-2, w+4, 20) {
		g.cycleSelect(key, list(), true)
	}
	return g.kv(k, val, x, y, w, c)
}

// cycleSelect выбирает следующего кандидата и наводит на него камеру.
func (g *Game) cycleSelect(key string, cs []cand, center bool) {
	if len(cs) == 0 {
		g.toast("Нет подходящих юнитов или источников пуска, готовых к действию")
		return
	}
	if g.cycleSeen == nil {
		g.cycleSeen = map[string]map[uint32]bool{}
	}
	seen := g.cycleSeen[key]
	if seen == nil {
		seen = map[uint32]bool{}
		g.cycleSeen[key] = seen
	}
	cx, cy := g.cam.ToWorld(float64(g.cam.X+g.cam.W/2), float64(g.cam.Y+g.cam.H/2))
	pick := func() int {
		best, bd := -1, math.Inf(1)
		for i, c := range cs {
			if seen[c.sel.ID] {
				continue
			}
			if d := math.Hypot(c.x-cx, c.y-cy); d < bd {
				best, bd = i, d
			}
		}
		return best
	}
	i := pick()
	if i < 0 { // показаны все — начинаем новый круг
		for k := range seen {
			delete(seen, k)
		}
		i = pick()
	}
	seen[cs[i].sel.ID] = true
	g.multi = nil
	g.sel = cs[i].sel
	if center {
		g.centerOn(cs[i].x, cs[i].y)
	}
}

func (g *Game) unitsOfType(id string) []cand {
	var out []cand
	for _, un := range g.view.Units {
		if un.Type == id {
			out = append(out, cand{Selection{Kind: "unit", ID: un.ID}, un.X, un.Y})
		}
	}
	return out
}

func (g *Game) interceptorUnits(id string) []cand {
	var out []cand
	for _, un := range g.view.Units {
		if g.cat.UnitByID[un.Type].Interceptor == id {
			out = append(out, cand{Selection{Kind: "unit", ID: un.ID}, un.X, un.Y})
		}
	}
	return out
}

func (g *Game) airfields(kind string) []cand {
	var out []cand
	for _, b := range g.view.Buildings {
		if b.Aircraft[kind] >= 1 && b.Operational() {
			out = append(out, cand{Selection{Kind: "building", ID: b.ID}, b.X, b.Y})
		}
	}
	return out
}

// munitionSources — источники пуска боеприпаса, у которых сейчас есть запас
// и готовность к залпу.
func (g *Game) munitionSources(id string) []cand {
	v := g.view
	m := g.cat.MunitionByID[id]
	if v.Stocks[id] < 1 {
		return nil
	}
	var out []cand
	for _, un := range v.Units {
		ut := g.cat.UnitByID[un.Type]
		if un.State != sim.UnitDeployed || un.Reload > 0 {
			continue
		}
		for _, mid := range ut.Munitions {
			if mid == id {
				out = append(out, cand{Selection{Kind: "unit", ID: un.ID}, un.X, un.Y})
			}
		}
	}
	for _, b := range v.Buildings {
		if !b.Operational() || b.Budget < 1 {
			continue
		}
		for _, p := range g.cat.BuildingByID[b.Type].Launch {
			if p == m.Platform {
				out = append(out, cand{Selection{Kind: "building", ID: b.ID}, b.X, b.Y})
				break
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------

func (g *Game) tabBuild(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Строительство", x, y)
	y = g.para("Выберите объект и щёлкните по своей территории (не ближе 15 км к фронту). Shift — строить несколько.", x, y, w, colDim)
	y += 4
	for _, bt := range g.cat.Buildings {
		ok := false
		for _, s := range bt.Buildable {
			if data.SideIndex(s) == v.Side {
				ok = true
			}
		}
		if !ok {
			continue
		}
		cost := data.ToRes(bt.Cost)
		afford := v.Res.Covers(cost, 1)
		fillRect(u.screen, float64(x-4), float64(y-4), float64(w+8), 1, color.RGBA{255, 255, 255, 40})
		drawBold(u.screen, bt.Name, float64(x), float64(y), 14, colText, 0)
		y += 20
		y = g.para(fmt.Sprintf("%s · %.0f ч", resText(cost), bt.BuildHours), x, y, w, colDim)
		y = g.para(bt.Desc, x, y, w, colDim)
		if u.ButtonState(x, y, 140, 24, "Строить", g.mode == modeBuild && g.buildType == bt.ID, afford) {
			g.mode, g.buildType = modeBuild, bt.ID
		}
		y += 32
	}
	return y
}

// ---------------------------------------------------------------------

var branchNames = map[string]string{"drones": "Дроны", "strike": "Удар", "ad": "ПВО", "intel": "Разведка и РЭБ"}

// techBranches — ветки, дающие бонусные очки трофеев и опыта.
var techBranches = []string{"drones", "strike", "ad", "intel"}

func (g *Game) tabScience(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Исследования", x, y)
	if v.Research != "" {
		t := g.cat.TechByID[v.Side][v.Research]
		y = g.label("Сейчас: "+t.Name, x, y, colText)
		u.Bar(x, y, w, 10, v.Progress[t.ID]/t.Cost, colAccent)
		y += 14
		eta := (t.Cost - v.Progress[t.ID]) / math.Max(v.ResRate, 0.01)
		y = g.label(fmt.Sprintf("%.0f / %.0f очков, ~%s", v.Progress[t.ID], t.Cost, fmtMin(eta*60)), x, y, colDim)
	} else {
		y = g.label("Исследование не выбрано", x, y, colWarn)
	}
	y = g.kv("Скорость", fmt.Sprintf("%.1f очков/ч", v.ResRate), x, y, w, colText)
	drawText(u.screen, "Финансирование:", float64(x), float64(y+3), 14, colDim, 0)
	for k := 0; k <= 3; k++ {
		if u.ButtonState(x+130+k*44, y, 40, 22, fmt.Sprintf("%d", k), v.ResFund == k, true) {
			g.sess.Send(sim.Command{Kind: sim.CmdResFund, Int: k})
		}
	}
	u.Tooltip(x, y, w, 22, fmt.Sprintf("Каждый уровень: +1.5 очка/ч за %.0f денег/ч", g.cat.Rules.ResearchFundCost))
	y += 30
	y = g.para("Трофеи (сбитые над своей территорией боеприпасы) и боевой опыт дают бонусные очки своей ветке:", x, y, w, colDim)
	for _, b := range techBranches {
		y = g.kv("  "+branchNames[b], fmt.Sprintf("%.0f", v.Bonus[b]), x, y, w, colText)
	}
	y += 6
	if u.ButtonState(x, y, w, 30, "Открыть окно исследований (T)", g.techOpen, true) {
		g.techOpen = !g.techOpen
	}
	y += 40
	y = g.para("В окне — линейки версий образцов: Герань-2 → 3 → 4 → 5, С-300 → С-400 → С-350 и т. д. У каждой версии свои цифры; старые версии остаются доступны, новые юниты и ракеты производятся уже новой версии. Каждая следующая ступень дороже и дольше.", x, y, w, colDim)
	// Изученное за эту партию.
	var done []string
	for _, t := range g.cat.Tech[data.SideKeys[v.Side]] {
		if v.Researched[t.ID] {
			done = append(done, t.Name)
		}
	}
	y += 4
	y = g.header(fmt.Sprintf("Изучено: %d из %d", len(done), len(g.cat.Tech[data.SideKeys[v.Side]])), x, y)
	for _, n := range done {
		y = g.label("• "+n, x, y, colGood)
	}
	return y
}

// ---------------------------------------------------------------------

func (g *Game) tabImport(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Закупки за рубежом", x, y)
	if k := v.SanctionTotal["import_cost"]; k > 0 {
		y = g.para(fmt.Sprintf("Из-за санкций импорт дороже на %.0f%%.", k*100), x, y, w, colWarn)
		y += 4
	}
	for _, im := range g.cat.Sides[v.Side].Imports {
		ok := true
		if im.Requires != "" && !v.Researched[im.Requires] {
			ok = false
		}
		if im.Removes != "" && v.Researched[im.Removes] {
			ok = false
		}
		if im.Limit > 0 && v.ImportCount[im.ID] >= im.Limit {
			ok = false
		}
		price := im.Money * (1 - v.Effects["import_discount"]) * (1 + v.SanctionTotal["import_cost"])
		drawText(u.screen, fitText(im.Name, 13, float64(w-100)), float64(x), float64(y), 13, colText, 0)
		y += 18
		info := fmt.Sprintf("%.0f денег, доставка %.0f ч", price, im.DelayH)
		if im.Limit > 0 {
			info += fmt.Sprintf(", осталось %d", im.Limit-v.ImportCount[im.ID])
		}
		if im.Requires != "" && !v.Researched[im.Requires] {
			info = "требует: " + g.cat.TechByID[v.Side][im.Requires].Name
		}
		drawText(u.screen, info, float64(x), float64(y), 13, colDim, 0)
		y += 20
		// Купить один раз, пачкой ×5 и ×10 (покупается столько, на сколько хватает денег) и автозакупка по порогу.
		afford := ok && v.Res[data.ResMoney] >= price
		bx := x
		for _, n := range []int{1, 5, 10} {
			lbl := "Купить"
			bw := 84
			if n > 1 {
				lbl, bw = fmt.Sprintf("×%d", n), 52
			}
			if u.ButtonState(bx, y, bw, 26, lbl, false, afford) {
				g.sess.Send(sim.Command{Kind: sim.CmdImport, Item: im.ID, Count: n})
			}
			bx += bw + 6
		}
		if im.AutoBelow > 0 {
			if u.ButtonState(bx, y, w-(bx-x), 26, fmt.Sprintf("Авто: ниже %.0f", im.AutoBelow), v.AutoImport[im.ID], ok) {
				on := 1
				if v.AutoImport[im.ID] {
					on = 0
				}
				g.sess.Send(sim.Command{Kind: sim.CmdAutoImport, Item: im.ID, Int: on})
			}
			u.Tooltip(bx, y, w-(bx-x), 26, fmt.Sprintf("Автозакупка: когда запас ниже %.0f, партия заказывается сама (если нет такой же в пути и денег больше %.0f). Включено — повторное нажатие выключает.", im.AutoBelow, g.cat.Rules.AutoImportReserve+price))
		}
		y += 34
	}
	y += 6
	y = g.header("Продажа излишков", x, y)
	y = g.para(fmt.Sprintf("Топливо и сталь можно продать за деньги по невыгодному курсу; остаток %.0f не трогается.", g.cat.Rules.SellKeepMin), x, y, w, colDim)
	for _, k := range []string{"fuel", "steel"} {
		rate := g.cat.Rules.SellRate[k]
		idx := 0
		for i, rk := range data.ResKeys {
			if rk == k {
				idx = i
			}
		}
		have := v.Res[idx]
		room := math.Max(0, have-g.cat.Rules.SellKeepMin)
		drawText(u.screen, fmt.Sprintf("%s: %.0f (можно продать %.0f по %.2f)", data.ResNames[idx], have, room, rate), float64(x), float64(y), 13, colText, 0)
		y += 20
		bx := x
		for _, n := range []int{500, 2000, 0} {
			lbl := fmt.Sprintf("%d", n)
			if n == 0 {
				lbl = "Всё лишнее"
			}
			bw := 70
			if n == 0 {
				bw = 110
			}
			if u.ButtonState(bx, y, bw, 26, lbl, false, room >= 1) {
				g.sess.Send(sim.Command{Kind: sim.CmdSell, Item: "res:" + k, Count: n})
			}
			bx += bw + 6
		}
		y += 34
	}
	y += 6
	y = g.header("В пути", x, y)
	if len(v.Deliveries) == 0 {
		y = g.label("Нет ожидаемых поставок", x, y, colDim)
	}
	for _, d := range v.Deliveries {
		y = g.kv(fitText(d.Name, 13, float64(w-80)), fmtMin(d.At-v.Time), x, y, w, colText)
	}
	if len(g.cat.Sides[v.Side].Aid) > 0 {
		y += 6
		y = g.header("Помощь партнёров", x, y)
		y = g.para("Пакеты помощи приходят со временем; их размер и сроки зависят от морали и удержания Киева.", x, y, w, colDim)
		for _, a := range g.cat.Sides[v.Side].Aid {
			if v.AidDone[a.ID] {
				y = g.label("• "+a.Name, x, y, colGood)
			}
		}
	}
	return y
}

// ---------------------------------------------------------------------

func (g *Game) tabIntel(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Агентура и OSINT", x, y)
	y = g.para(fmt.Sprintf("Случайные донесения о вражеских объектах с точными координатами (данные стареют). Финансирование ускоряет их в 2.5 раза за %.0f денег/ч.", g.cat.Rules.AgentFundCost), x, y, w, colDim)
	lbl := "Финансировать"
	if v.AgentFund {
		lbl = "Финансирование включено"
	}
	if u.ButtonState(x, y, w, 26, lbl, v.AgentFund, true) {
		f := 1
		if v.AgentFund {
			f = 0
		}
		g.sess.Send(sim.Command{Kind: sim.CmdAgentFund, Int: f})
	}
	y += 34
	y = g.header("Спутники", x, y)
	y = g.para("Оптика определяет тип объекта и повреждения, но не видит замаскированное. Радар видит замаскированное, но тип не определяет.", x, y, w, colDim)
	for _, s := range v.Sats {
		c := sideText(s.Side)
		sensor := "оптика"
		if s.Sensor == "radar" {
			sensor = "радар"
		}
		st := "через " + fmtMin(s.Pass.Start-v.Time)
		if s.Pass.Active {
			st = "съёмка"
		}
		drawText(u.screen, fmt.Sprintf("%s (%s)", s.Name, sensor), float64(x), float64(y), 13, c, 0)
		drawText(u.screen, st, float64(x+w-60), float64(y), 13, colText, 2)
		if u.Button(x+w-54, y-2, 54, 20, "трасса") {
			g.layers["sats"] = true
			g.centerOn(s.Pass.X0+s.Pass.DX*s.Pass.L/2, s.Pass.Y0+s.Pass.DY*s.Pass.L/2)
			g.cam.Z = 0.8
		}
		y += 22
	}
	y += 8
	y = g.header("Энергия противника (оценка)", x, y)
	if len(v.EnemyPower) == 0 {
		y = g.para("Данных нет: нужны известные электростанции и города противника.", x, y, w, colDim)
	} else {
		y = g.para("Считается только по известным объектам и их известному состоянию; неизвестные станции и потребители в оценку не входят, данные могут устареть.", x, y, w, colDim)
		y = g.kv("Всего ≈ генерация / потребление", fmt.Sprintf("%.0f / %.0f МВт", v.EnemyGen, v.EnemyUse), x, y, w, colText)
		for _, e := range v.EnemyPower {
			c := colGood
			switch {
			case e.Frac < 0.7:
				c = colBad
			case e.Frac < 0.99:
				c = colWarn
			}
			drawText(u.screen, fitText(e.Name, 13, float64(w-150)), float64(x), float64(y), 13, colText, 0)
			drawText(u.screen, fmt.Sprintf("≈%.0f/%.0f  %.0f%%", e.Gen, e.Use, e.Frac*100), float64(x+w), float64(y), 12, c, 2)
			u.Tooltip(x, y-2, w, 20, fmt.Sprintf("%s: по %d известным объектам, самая старая метка — %s", e.Name, e.Sources, ageText(v.Time, e.Seen)))
			y += 20
		}
	}
	y += 8
	y = g.header("Разведданные", x, y)
	cs := append([]sim.Contact{}, v.Contacts...)
	sort.Slice(cs, func(a, b int) bool { return cs[a].Seen > cs[b].Seen })
	nb, nu := 0, 0
	for _, c := range cs {
		if c.Kind == 0 {
			nb++
		} else {
			nu++
		}
	}
	y = g.label(fmt.Sprintf("Известно объектов: %d, техники: %d", nb, nu), x, y, colText)
	for i, c := range cs {
		if i >= 40 || c.Seen < 0 {
			break
		}
		t := fitText(g.contactTitle(&c), 13, float64(w-110))
		if u.mouseIn(x, y-2, w, 20) {
			u.rowHi(float64(x-4), float64(y-2), float64(w+8), 20)
		}
		drawText(u.screen, t, float64(x), float64(y), 13, colText, 0)
		drawText(u.screen, ageText(v.Time, c.Seen), float64(x+w), float64(y), 12, colDim, 2)
		if u.clicked(x, y-2, w, 20) {
			g.centerOn(c.X, c.Y)
			g.sel = Selection{Kind: "contact", ID: c.ID}
		}
		y += 20
	}
	return y
}

// ---------------------------------------------------------------------

func (g *Game) tabLog(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Журнал событий", x, y)
	for i := len(v.Events) - 1; i >= 0; i-- {
		e := v.Events[i]
		c := colText
		switch e.Level {
		case 1:
			c = colWarn
		case 2:
			c = colBad
		}
		lines := wrap(e.Text, 13, float64(w-60))
		h := len(lines)*17 + 4
		if e.HasPos && u.mouseIn(x, y-2, w, h) {
			u.rowHi(float64(x-4), float64(y-2), float64(w+8), float64(h))
		}
		drawText(u.screen, fmt.Sprintf("%02d:%02d", (int(e.Time)%1440)/60, int(e.Time)%60), float64(x), float64(y), 12, colDim, 0)
		for k, l := range lines {
			drawText(u.screen, l, float64(x+50), float64(y+k*17), 13, c, 0)
		}
		if e.HasPos && u.clicked(x, y-2, w, h) {
			g.centerOn(e.X, e.Y)
		}
		y += h
	}
	return y
}

// ---------------------------------------------------------------------

func (g *Game) tabMissions(x, y, w int) int {
	u := &g.ui
	v := g.view
	if len(v.Sanctions) > 0 {
		return g.tabSanctions(x, y, w)
	}
	if len(v.Airspace) > 0 {
		y = g.tabAirspace(x, y, w)
		y += 8
	}
	y = g.header("Задания", x, y)
	if len(v.Missions) == 0 {
		return g.para("Для вашей стороны заданий нет: помощь приходит только по таймеру.", x, y, w, colDim)
	}
	y = g.para("За выполнение задания приходит крупный пакет помощи. Прогресс засчитывается по данным о поражении объектов.", x, y, w, colDim)
	y += 4
	for _, m := range v.Missions {
		c := colText
		status := fmt.Sprintf("%.0f / %.0f", m.Progress, m.Target)
		switch {
		case m.Done:
			c, status = colGood, "выполнено"
		case m.Failed:
			c, status = colBad, "провалено"
		}
		drawBold(u.screen, m.Title, float64(x), float64(y), 14, c, 0)
		drawText(u.screen, status, float64(x+w), float64(y+1), 13, c, 2)
		y += 22
		if !m.Done && !m.Failed {
			u.Bar(x, y, w, 8, m.Progress/math.Max(m.Target, 0.001), colAccent)
			y += 14
		}
		y = g.para(m.Hint, x, y, w, colDim)
		y = g.para("Награда: "+m.Reward, x, y, w, colText)
		y += 10
	}
	return y
}

// tabSanctions — санкции против стороны (аналог помощи и заданий у противника).
func (g *Game) tabSanctions(x, y, w int) int {
	u := &g.ui
	v := g.view
	y = g.header("Санкции", x, y)
	y = g.para("Запад вводит пакеты санкций по графику и в ответ на ваши действия. Штрафы действуют до конца войны и складываются (не больше 75% по каждому виду).", x, y, w, colDim)
	y += 4
	y = g.header("Действуют сейчас", x, y)
	if len(v.SanctionTotal) == 0 {
		y = g.label("Санкций пока нет", x, y, colDim)
	}
	for _, k := range sim.SanctionOrder {
		if t := v.SanctionTotal[k]; t > 0 {
			sign := "−"
			if k == "import_cost" {
				sign = "+"
			}
			y = g.kv(data.SanctionKeys[k], fmt.Sprintf("%s%.0f%%", sign, t*100), x, y, w, colBad)
		}
	}
	h := 0.0
	if v.War {
		h = (v.Time - v.PrepEnd) / 60
	}
	var on, timed, trig []sim.SanctionView
	for _, s := range v.Sanctions {
		switch {
		case s.On:
			on = append(on, s)
		case s.Trigger:
			trig = append(trig, s)
		default:
			timed = append(timed, s)
		}
	}
	if len(on) > 0 {
		y += 6
		y = g.header(fmt.Sprintf("Введены: %d из %d", len(on), len(v.Sanctions)), x, y)
		for _, s := range on {
			drawBold(u.screen, fitText(s.Name, 14, float64(w)), float64(x), float64(y), 14, colBad, 0)
			y += 19
			y = g.para(s.Effects, x, y, w, colText)
			y += 4
		}
	}
	if len(timed) > 0 {
		y += 6
		y = g.header("Ожидаются", x, y)
		for _, s := range timed {
			left := "скоро"
			if d := s.AtHour - h; d > 0 {
				left = fmt.Sprintf("через %.0f ч", math.Ceil(d))
			}
			drawBold(u.screen, fitText(s.Name, 14, float64(w-90)), float64(x), float64(y), 14, colText, 0)
			drawText(u.screen, left, float64(x+w), float64(y+1), 13, colWarn, 2)
			y += 19
			txt := s.Effects
			if s.Cond != "" {
				txt += "; " + s.Cond
			}
			y = g.para(txt, x, y, w, colDim)
			y += 4
		}
	}
	if len(trig) > 0 {
		y += 6
		y = g.header("В ответ на ваши действия", x, y)
		for _, s := range trig {
			drawBold(u.screen, fitText(s.Name, 14, float64(w-60)), float64(x), float64(y), 14, colText, 0)
			if s.Target > 1 {
				drawText(u.screen, fmt.Sprintf("%.0f / %.0f", s.Progress, s.Target), float64(x+w), float64(y+1), 13, colWarn, 2)
			}
			y += 20
			if s.Target > 1 {
				u.Bar(x, y, w, 8, s.Progress/s.Target, colBad)
				y += 14
			}
			y = g.para(s.Hint, x, y, w, colDim)
			y = g.para("Штраф: "+s.Effects, x, y, w, colText)
			y += 6
		}
	}
	return y
}

// tabAirspace — пакеты открытия неба соседних стран для ударов (у Украины).
func (g *Game) tabAirspace(x, y, w int) int {
	u := &g.ui
	v := g.view
	h := 0.0
	if v.War {
		h = (v.Time - v.PrepEnd) / 60
	}
	y = g.header("Воздушное пространство", x, y)
	y = g.para("Запуски идут только с вашей земли, но маршрут можно вести над странами, которые открыли небо (на карте они подкрашены): так проще обойти российскую ПВО и достать север. Над остальными странами и над Беларусью лететь нельзя. Открытое небо не закрывается.", x, y, w, colDim)
	y += 4
	for _, a := range v.Airspace {
		c, status := colText, ""
		if a.On {
			c, status = colGood, "открыто"
		} else if d := a.AtHour - h; d > 0 {
			status = fmt.Sprintf("через %.0f ч", math.Ceil(d))
		} else {
			status = "ждёт условий"
		}
		drawBold(u.screen, fitText(a.Countries, 14, float64(w-110)), float64(x), float64(y), 14, c, 0)
		drawText(u.screen, status, float64(x+w), float64(y+1), 13, map[bool]color.Color{true: colGood, false: colWarn}[a.On], 2)
		y += 20
		if !a.On && a.Cond != "" {
			y = g.para("Условие: "+a.Cond, x, y, w, colDim)
		}
	}
	return y
}

// frontTable — направления одной таблицей: строки — показатели, столбцы — Киев, Харьков, Донбасс, Крым.
func (g *Game) frontTable(x, y, w int) int {
	u := &g.ui
	v := g.view
	lw := 100
	nd := sim.NumDir
	cw := (w - lw) / nd
	col := func(d int) float64 { return float64(x + lw + d*cw + cw/2) }
	u.card(float64(x-6), float64(y-6), float64(w+12), 298, color.RGBA{255, 255, 255, 90}, 0)
	for d := 0; d < nd; d++ {
		drawBold(u.screen, sim.DirNames[d], col(d), float64(y), 12, colText, 1)
	}
	y += 24
	// Позиция направления: О — оборона, А — активная оборона, Н — наступление.
	drawText(u.screen, "Позиция", float64(x), float64(y+5), 13, colDim, 0)
	short := [3]string{"О", "А", "Н"}
	const pb, pg = 22, 2
	for d := 0; d < nd; d++ {
		for p := 0; p < 3; p++ {
			bx := x + lw + d*cw + (cw-3*pb-2*pg)/2 + p*(pb+pg)
			if u.ButtonState(bx, y, pb, 26, short[p], v.Posture[d] == p, true) {
				g.sess.Send(sim.Command{Kind: sim.CmdPosture, Int: p, Count: d + 1})
			}
			u.Tooltip(bx, y, pb, 26, sim.DirNames[d]+": "+sim.PostureNames[p]+". "+postureTip)
		}
	}
	y += 32
	drawText(u.screen, "Пополнение", float64(x), float64(y+5), 13, colDim, 0)
	for d := 0; d < nd; d++ {
		bx := x + lw + d*cw + (cw-22)/2
		drawText(u.screen, fmt.Sprintf("%.0f%%", v.Alloc[d]*100), col(d), float64(y+5), 13, colText, 1)
		if u.Button(bx-12, y+24, 22, 22, "−") {
			g.alloc(d, -0.1)
		}
		if u.Button(bx+12, y+24, 22, 22, "+") {
			g.alloc(d, 0.1)
		}
	}
	y += 54
	row := func(label string, val func(f sim.Direction) (string, color.Color)) {
		drawText(u.screen, label, float64(x), float64(y), 13, colDim, 0)
		for d := 0; d < nd; d++ {
			t, c := val(v.Front[d])
			drawText(u.screen, t, col(d), float64(y), 14, c, 1)
		}
		y += 20
	}
	row("Люди, тыс.", func(f sim.Direction) (string, color.Color) { return fmt.Sprintf("%.1f", f.Men), colText })
	row("Бронетехника", func(f sim.Direction) (string, color.Color) { return fmt.Sprintf("%.0f", f.Armor), colText })
	row("Артиллерия", func(f sim.Direction) (string, color.Color) { return fmt.Sprintf("%.0f", f.Artillery), colText })
	row("FPV-дроны", func(f sim.Direction) (string, color.Color) { return fmt.Sprintf("%.0f", f.FPV), colText })
	row("Снабжение", func(f sim.Direction) (string, color.Color) {
		return fmt.Sprintf("%.0f%%", f.Supply*100), colorForFrac(f.Supply)
	})
	row("Авиация", func(f sim.Direction) (string, color.Color) { return fmt.Sprintf("×%.2f", f.Air), colText })
	row("Мощь", func(f sim.Direction) (string, color.Color) { return fmt.Sprintf("%.0f", f.Power), colText })
	row("Тайлов фронта", func(f sim.Direction) (string, color.Color) { return fmt.Sprintf("%d", f.Tiles), colText })
	row("За час, км²", func(f sim.Direction) (string, color.Color) {
		c := colText
		if f.LostH > f.GainedH {
			c = colBad
		} else if f.GainedH > f.LostH {
			c = colGood
		}
		return fmt.Sprintf("%+d", (f.GainedH-f.LostH)*25), c
	})
	return y + 12
}
