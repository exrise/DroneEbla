package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
)

// drawInfoPanel — панель выбранного объекта (справа внизу).
func (g *Game) drawInfoPanel() {
	u := &g.ui
	v := g.view
	if v.Placement {
		g.drawPlacementPanel()
		return
	}
	if g.mode == modeStrike {
		g.drawStrikePanel()
		return
	}
	if g.mode == modePalantir {
		g.drawPalantirPanel()
		return
	}
	if g.sel.Kind == "" && g.multi == nil {
		g.drawHelp()
		return
	}
	// Высота карточки — по содержимому (по данным прошлого кадра), не больше infoH.
	key := fmt.Sprintf("%s:%d:%d:%d", g.sel.Kind, g.sel.ID, g.sel.Idx, len(g.multi))
	if key != g.infoKey {
		g.infoKey, g.infoUsed = key, 0
	}
	ih := infoH
	if g.infoUsed > 0 {
		ih = min(infoH, max(130, g.infoUsed))
	}
	x, y := u.W-infoW-8, u.H-ih-8
	u.Panel(x, y, infoW, ih)
	old := u.screen
	u.screen = u.sub(x, y, infoW, ih)
	u.clip = image.Rect(x, y, x+infoW, y+ih)
	contentTrack, contentBottom = true, 0
	defer func() {
		contentTrack = false
		if contentBottom > 0 {
			g.infoUsed = int(contentBottom) - y + 14
		}
		u.screen = old
		u.clip = image.Rectangle{}
	}()
	px, py, pw := x+12, y+10, infoW-24
	if u.Button(x+infoW-34, y+6, 26, 26, "×") {
		g.sel = Selection{}
		g.multi = nil
		return
	}
	if ids := g.multiIDs(); ids != nil {
		g.multiInfo(ids, px, py, pw)
		return
	}
	switch g.sel.Kind {
	case "building":
		b := g.findBuilding(g.sel.ID)
		if b == nil {
			g.sel = Selection{}
			return
		}
		g.buildingInfo(b, px, py, pw)
	case "unit":
		un := g.findUnit(g.sel.ID)
		if un == nil {
			g.sel = Selection{}
			return
		}
		g.unitInfo(un, px, py, pw)
	case "contact":
		c := g.findContact(g.sel.ID)
		if c == nil {
			g.sel = Selection{}
			return
		}
		drawBold(u.screen, fitText(g.contactTitle(c), 16, float64(pw-30)), float64(px), float64(py), 16, sideColor(1-v.Side), 0)
		py += 28
		py = g.kv("Данные", ageText(v.Time, c.Seen), px, py, pw, colText)
		py = g.kv("Источник", c.Source, px, py, pw, colText)
		if c.HP >= 0 {
			py = g.kv("Оценка состояния", fmt.Sprintf("%.0f%%", c.HP*100), px, py, pw, colorForFrac(c.HP))
		} else {
			py = g.kv("Оценка состояния", "неизвестно", px, py, pw, colDim)
		}
		if ut := g.cat.UnitByID[c.Type]; ut != nil && ut.Kind == "ad" {
			py = g.kv("Дальность поражения", fmt.Sprintf("%.0f км", ut.RangeKm), px, py, pw, colText)
		}
		py += 6
		py = g.para("Мобильная техника могла уехать. Чтобы ударить, выберите свою пусковую или площадку и укажите эту цель на карте.", px, py, pw, colDim)
		if v.Palantir.Has {
			py += 4
			if u.ButtonState(px, py, pw, 26, "Удар через «Палантир»", false, v.Palantir.Active && v.War) {
				t := sim.Pt{X: c.X, Y: c.Y}
				g.startPalantir(&t)
			}
		}
	case "city":
		c := g.m.Cities[g.sel.Idx]
		drawBold(u.screen, c.Name, float64(px), float64(py), 16, colText, 0)
		py += 28
		py = g.kv("Население", fmt.Sprintf("%d тыс.", c.Pop/1000), px, py, pw, colText)
		if c.Region >= 0 {
			py = g.kv("Область", g.m.Regions[c.Region].Name, px, py, pw, colText)
		}
		tx, ty := g.m.TileAt(c.X, c.Y)
		if g.m.In(tx, ty) {
			o := int(v.Owner[g.m.Idx(tx, ty)]) - 1
			if o >= 0 {
				py = g.kv("Контроль", data.SideNames[o], px, py, pw, sideText(o))
			}
		}
	}
}

func (g *Game) buildingInfo(b *sim.Building, x, y, w int) {
	u := &g.ui
	v := g.view
	bt := g.cat.BuildingByID[b.Type]
	drawBold(u.screen, fitText(b.Name, 16, float64(w-30)), float64(x), float64(y), 16, sideText(v.Side), 0)
	y += 22
	drawText(u.screen, bt.Name, float64(x), float64(y), 13, colDim, 0)
	y += 20
	y = g.para(bt.Desc, x, y, w, colDim)
	y += 4
	if b.Built < 1 {
		y = g.kv("Строительство", fmt.Sprintf("%.0f%%", b.Built*100), x, y, w, colWarn)
	}
	f := b.HP / b.MaxHP
	y = g.kv("Состояние", fmt.Sprintf("%.0f%%", f*100), x, y, w, colorForFrac(f))
	if len(bt.Produces) > 0 || len(bt.Capacity) > 0 || bt.Research > 0 || bt.Export > 0 {
		ef := sim.Efficiency(g.cat.Rules, f)
		y = g.kv("Продуктивность", fmt.Sprintf("%.0f%%", ef*100), x, y, w, colorForFrac(ef))
	}
	if f < 0.999 && b.Built >= 1 {
		left := (1 - f) * bt.RepairHrs() / (1 + v.Effects["repair_speed"])
		switch {
		case !b.Repair:
			y = g.kv("Ремонт", "выключен", x, y, w, colDim)
		case b.Repairing:
			y = g.kv("Ремонт", fmt.Sprintf("идёт, ещё ~%.0f ч", left), x, y, w, colGood)
		default:
			y = g.kv("Ремонт", fmt.Sprintf("ждёт бригаду (~%.0f ч работы)", left), x, y, w, colWarn)
		}
	}
	if bt.Power > 0 {
		y = g.kv("Генерация", fmt.Sprintf("%.0f МВт", bt.Power*f), x, y, w, colText)
	} else if bt.Power < 0 {
		pf, ok := v.RegionPower[b.Region]
		if !ok {
			pf = 1
		}
		y = g.kv("Энергия в области", fmt.Sprintf("%.0f%%", pf*100), x, y, w, colorForFrac(pf))
	}
	if len(bt.Produces) > 0 {
		s := ""
		for i, k := range data.ResKeys {
			if q := bt.Produces[k]; q > 0 {
				s += fmt.Sprintf("%s %.0f/ч ", data.ResName(i, g.view.Side), q)
			}
		}
		y = g.kv("Выпуск (номинал)", s, x, y, w, colText)
	}
	for k, c := range bt.Capacity {
		y = g.kv("Мощность: "+capNames[k], fmt.Sprintf("%.0f очков/ч", c), x, y, w, colText)
	}
	if bt.NeedNear != "" || len(bt.NeedDeposit) > 0 {
		need := ""
		if bt.NeedNear != "" {
			need = fmt.Sprintf("%s в радиусе %.0f км", g.bName(bt.NeedNear), bt.NearKm)
		}
		for dep, km := range bt.NeedDeposit {
			if need != "" {
				need += "; "
			}
			n := map[string]string{"oil": "нефть", "gas": "газ", "coal": "уголь", "ore": "руда"}[dep]
			if km > 0 {
				need += fmt.Sprintf("%s в радиусе %.0f км", n, km)
			} else {
				need += n + " на своей территории"
			}
		}
		y = g.para("Требует: "+need, x, y, w, colDim)
	}
	if b.Aircraft != nil {
		for k, n := range b.Aircraft {
			name := map[string]string{"tactical": "Тактическая авиация", "strategic": "Дальняя авиация", "fighter": "Истребители-перехватчики"}[k]
			y = g.kv(name, fmt.Sprintf("%.0f", n), x, y, w, colText)
		}
	}
	if bt.Intercept != nil {
		n := v.Stocks["m_aam_"+data.SideKeys[v.Side]]
		c := colText
		if n < 20 {
			c = colBad
		}
		y = g.kv("Ракеты воздух—воздух", fmt.Sprintf("%.0f на складе", n), x, y, w, c)
		y = g.para(fmt.Sprintf("Перехват в радиусе %.0f км, каналов до %d. Не работает под вражеской ПВО.", bt.Intercept.Km, bt.Intercept.Channels), x, y, w, colDim)
	}
	if bt.Supply > 0 {
		d := b.Dir
		if d < 0 {
			d = g.dirAt(b.X, b.Y)
		}
		y = g.kv("Снабжает направление", sim.DirNames[d], x, y, w, colText)
	}
	if bt.ID == "bridge" {
		for _, o := range g.cat.Objects {
			if o.Type == "bridge" && o.Name == b.Name && len(o.Link) == 2 {
				open, c := "открыт", colGood
				if b.HP < b.MaxHP*g.cat.Rules.BridgePassFrac {
					open, c = "закрыт (разрушен)", colBad
				}
				y = g.kv("Проход для техники", open, x, y, w, c)
			}
		}
	}
	if bt.Untargetable {
		y = g.para("Объект защищён от ударов правилами игры.", x, y, w, colDim)
	}
	y += 6
	bw := (w - 8) / 2
	rl := "Ремонт: вкл"
	if !b.Repair {
		rl = "Ремонт: выкл"
	}
	if u.ButtonState(x, y, bw, 24, rl, b.Repair, true) {
		r := 1
		if b.Repair {
			r = 0
		}
		g.sess.Send(sim.Command{Kind: sim.CmdRepair, ID: b.ID, Int: r})
	}
	if u.ButtonState(x+bw+8, y, bw, 24, "Маскировать", b.Masked, !b.Masked) {
		g.sess.Send(sim.Command{Kind: sim.CmdMask, ID: b.ID})
	}
	u.Tooltip(x+bw+8, y, bw, 24, "Скрывает объект от оптической разведки (спутники, дроны). Радарные спутники видят. Стоимость: "+resText(g.view.Side, data.ToRes(g.cat.Rules.MaskCost)))
	y += 30
	if len(bt.Launch) > 0 {
		g.launchButtons(b.ID, x, y, w, fmt.Sprintf("Пусков доступно: %.0f", math.Floor(b.Budget)))
	}
}

func (g *Game) unitInfo(un *sim.Unit, x, y, w int) {
	u := &g.ui
	v := g.view
	ut := g.cat.UnitByID[un.Type]
	drawBold(u.screen, fitText(ut.Name, 16, float64(w-30)), float64(x), float64(y), 16, sideText(v.Side), 0)
	y += 24
	y = g.para(ut.Desc, x, y, w, colDim)
	y += 4
	y = g.kv("Состояние", sim.UnitStateNames[un.State], x, y, w, colText)
	if un.State == sim.UnitPacking || un.State == sim.UnitDeploying {
		y = g.kv("Осталось", fmtMin(un.Timer), x, y, w, colText)
	}
	y = g.kv("Прочность", fmt.Sprintf("%.0f%%", un.HP/ut.HP*100), x, y, w, colorForFrac(un.HP/ut.HP))
	switch ut.Kind {
	case "ad":
		y = g.kv("Дальность / РЛС", fmt.Sprintf("%.0f / %.0f км", ut.RangeKm, ut.RadarKm), x, y, w, colText)
		y = g.kv("Каналы", fmt.Sprintf("%d занято из %d", un.Busy, ut.Channels), x, y, w, colText)
		rc := colText
		if un.Ready < 1 {
			rc = colBad
		} else if un.Ready < float64(ut.Magazine) {
			rc = colWarn
		}
		y = g.kv("На пусковых", fmt.Sprintf("%.0f / %d (перезарядка %.0f мин)", math.Floor(un.Ready), ut.Magazine, ut.ReloadMin), x, y, w, rc)
		if ut.Interceptor != "" {
			n := v.Stocks[ut.Interceptor]
			c := colText
			if thr := v.LowInterceptor[ut.Interceptor]; n <= thr {
				c = colBad
			}
			y = g.kv("Ракеты ("+g.cat.MunitionByID[ut.Interceptor].Short+")", fmt.Sprintf("%.0f на складе", n), x, y, w, c)
			if n < 1 && un.Ready < 1 {
				y = g.para("Нет ракет: комплекс не стреляет. Закажите ракеты в госзаказе или включите автозаказ в «Арсенале».", x, y, w, colBad)
			}
		} else {
			y = g.kv("Боеприпасы", "общий запас", x, y, w, colText)
		}
		y = g.kv("Поражение низких / высоких", fmt.Sprintf("%.0f%% / %.0f%%", ut.PkLow*100, ut.PkHigh*100), x, y, w, colText)
		y = g.fireModeButtons([]uint32{un.ID}, un.Fire, x, y, w)
	case "radar":
		y = g.kv("Дальность обнаружения", fmt.Sprintf("%.0f км", ut.RadarKm), x, y, w, colText)
	case "reb":
		y = g.kv("Радиус подавления", fmt.Sprintf("%.0f км", ut.RebKm), x, y, w, colText)
	case "rtr":
		y = g.kv("Дальность пеленгации", fmt.Sprintf("%.0f км", ut.RtrKm), x, y, w, colText)
	case "launcher":
		if un.Reload > 0 {
			y = g.kv("Перезарядка", fmtMin(un.Reload), x, y, w, colWarn)
		}
		y = g.kv("Залп", fmt.Sprintf("до %d", ut.Salvo), x, y, w, colText)
	}
	if len(un.Path) > 0 {
		eta := sim.PathETA(g.m, g.cat.Rules, un, ut)
		rail := 0
		for _, r := range un.PathRail {
			if r {
				rail++
			}
		}
		lbl := "В пути ещё"
		if len(un.PathRail) > 0 && un.PathRail[0] {
			lbl = "Едет эшелоном, ещё"
		} else if len(un.PathWait) > 0 && un.PathWait[0] > 0 {
			lbl = "Погрузка/выгрузка, всего ещё"
		}
		y = g.kv(lbl, fmtMin(eta), x, y, w, colText)
		if rail > 0 {
			y = g.kv("По железной дороге", fmt.Sprintf("%d уч.", rail), x, y, w, colAccent)
		}
	}
	y = g.kv("Свёртывание / развёртывание", fmt.Sprintf("%.0f / %.0f мин", ut.PackMin, ut.DeployMin), x, y, w, colDim)
	if ut.Emitter() {
		y = g.para("Излучает: видим вражеской РТР.", x, y, w, colDim)
	}
	y = g.para("ПКМ по карте — марш по своей территории.", x, y, w, colDim)
	if ut.Kind == "launcher" {
		g.launchButtons(un.ID, x, y+4, w, "")
	}
}

// sourceBusy — почему пусковая сейчас не может стрелять ("" — может).
func (g *Game) sourceBusy(src uint32) string {
	if un := g.findUnit(src); un != nil {
		switch {
		case un.Reload > 0:
			return fmt.Sprintf("Перезарядка: ещё %s", fmtMin(un.Reload))
		case un.State != sim.UnitDeployed:
			return "Не развёрнута: сначала дождитесь развёртывания"
		}
	}
	return ""
}

// launchButtons — выбор боеприпаса для удара.
func (g *Game) launchButtons(src uint32, x, y, w int, note string) {
	u := &g.ui
	v := g.view
	if note != "" {
		drawText(u.screen, note, float64(x), float64(y), 13, colDim, 0)
		y += 20
	}
	// Пусковая на перезарядке или не развёрнута: кнопки выключены, причина написана.
	busy := g.sourceBusy(src)
	if busy != "" {
		drawText(u.screen, busy, float64(x), float64(y), 13, colWarn, 0)
		y += 20
	}
	opts := g.launchOptions(src)
	if len(opts) == 0 {
		drawText(u.screen, "Нет доступных боеприпасов", float64(x), float64(y), 13, colDim, 0)
		return
	}
	for _, id := range opts {
		m := g.cat.MunitionByID[id]
		n := v.Stocks[id]
		lbl := fmt.Sprintf("%s (%.0f)", m.Name, n)
		verb := "Удар"
		if m.Kind == "recon" {
			verb = "Разведка"
		}
		if u.ButtonState(x, y, w, 24, verb+": "+lbl, false, n >= 1 && v.War && busy == "") {
			g.mode = modeStrike
			g.strike = strikePlan{Source: src, Munition: id, Count: 1}
		}
		if !v.War {
			u.Tooltip(x, y, w, 24, "Удары доступны после начала войны")
		} else {
			u.Tooltip(x, y, w, 24, g.itemDesc(id))
		}
		y += 28
	}
}

// launchOptions — аналог sim.LaunchOptions по данным представления.
func (g *Game) launchOptions(src uint32) []string {
	v := g.view
	side := data.SideKeys[v.Side]
	var out []string
	if b := g.findBuilding(src); b != nil {
		bt := g.cat.BuildingByID[b.Type]
		for _, m := range g.cat.Munitions {
			if m.Side != side || m.Kind == "interceptor" {
				continue
			}
			for _, p := range bt.Launch {
				if p == m.Platform && (v.Stocks[m.ID] > 0 || v.Unlocked[m.ID]) {
					out = append(out, m.ID)
				}
			}
		}
	}
	if un := g.findUnit(src); un != nil {
		for _, id := range g.cat.UnitByID[un.Type].Munitions {
			if v.Stocks[id] > 0 || v.Unlocked[id] {
				out = append(out, id)
			}
		}
	}
	return out
}

func (g *Game) sourcePos(src uint32) (float64, float64, bool) {
	if b := g.findBuilding(src); b != nil {
		return b.X, b.Y, true
	}
	if un := g.findUnit(src); un != nil {
		return un.X, un.Y, true
	}
	return 0, 0, false
}

func (g *Game) strikeLength() (float64, float64) {
	m := g.cat.MunitionByID[g.strike.Munition]
	sx, sy, ok := g.sourcePos(g.strike.Source)
	if !ok || m == nil {
		return 0, 0
	}
	pts := g.strike.Pts
	if m.Kind == "ballistic" || m.Kind == "rocket" || (m.Kind == "cruise" && m.Class == "high") {
		if len(pts) > 0 {
			pts = pts[len(pts)-1:]
		}
	}
	l := sim.PathLength(sim.Pt{X: sx, Y: sy}, pts)
	if m.Kind == "recon" && len(pts) > 0 {
		last := pts[len(pts)-1]
		l += math.Hypot(last.X-sx, last.Y-sy)
	}
	return l, m.RangeKm
}

// strikeBlocked — страна с закрытым небом на маршруте плана ("" — путь свободен);
// проверка та же, что на хосте (sim.AirspaceBlock), по открытым странам из View.
func (g *Game) strikeBlocked() string {
	m := g.cat.MunitionByID[g.strike.Munition]
	sx, sy, ok := g.sourcePos(g.strike.Source)
	if !ok || m == nil || g.view == nil {
		return ""
	}
	pts := g.strike.Pts
	if (m.Kind == "ballistic" || m.Kind == "rocket" || (m.Kind == "cruise" && m.Class == "high")) && len(pts) > 0 {
		pts = pts[len(pts)-1:]
	}
	prev := sim.Pt{X: sx, Y: sy}
	for _, q := range pts {
		if c := sim.AirspaceBlock(g.m, g.cat.Sides[g.view.Side].BelarusAir, g.view.AirOpen, prev, q); c != 0 {
			if n := world.CountryName(c); n != "" {
				return n
			}
			return "чужое государство"
		}
		prev = q
	}
	return ""
}

func (g *Game) drawStrikePlan(dst *ebiten.Image) {
	m := g.cat.MunitionByID[g.strike.Munition]
	sx, sy, ok := g.sourcePos(g.strike.Source)
	if !ok || m == nil {
		g.mode = modeNone
		return
	}
	px, py := g.cam.ToScreen(sx, sy)
	r := m.RangeKm
	if m.Kind == "recon" {
		r /= 2
	}
	circle(dst, px, py, r*g.cam.Z, color.RGBA{180, 120, 20, 150}, 1.5)
	l, rng := g.strikeLength()
	col := color.RGBA{200, 120, 0, 255}
	if l > rng {
		col = colBad
	}
	pts := g.strike.Pts
	direct := m.Kind == "ballistic" || m.Kind == "rocket" || (m.Kind == "cruise" && m.Class == "high")
	if direct && len(pts) > 0 {
		pts = pts[len(pts)-1:]
	}
	prev := sim.Pt{X: sx, Y: sy}
	bel := g.cat.Sides[g.view.Side].BelarusAir
	for i, p := range pts {
		qx, qy := g.cam.ToScreen(p.X, p.Y)
		segCol := col
		if sim.AirspaceBlock(g.m, bel, g.view.AirOpen, prev, p) != 0 {
			segCol = colBad
		}
		prev = p
		line(dst, px, py, qx, qy, segCol, 2)
		if i == len(pts)-1 && m.Kind != "recon" {
			circle(dst, qx, qy, 8, col, 2)
			line(dst, qx-11, qy, qx+11, qy, col, 1.5)
			line(dst, qx, qy-11, qx, qy+11, col, 1.5)
		} else {
			disc(dst, qx, qy, 4, col)
		}
		px, py = qx, qy
	}
	if m.Kind == "recon" && len(pts) > 0 {
		hx, hy := g.cam.ToScreen(sx, sy)
		line(dst, px, py, hx, hy, withAlpha(col, 120), 1)
	}
}

func (g *Game) drawStrikePanel() {
	u := &g.ui
	v := g.view
	m := g.cat.MunitionByID[g.strike.Munition]
	if m == nil {
		g.mode = modeNone
		return
	}
	h := 270
	blocked := g.strikeBlocked()
	if blocked != "" {
		h += 19
	}
	x, y := u.W-infoW-8, u.H-h-8
	u.Panel(x, y, infoW, h)
	px, py, pw := x+12, y+10, infoW-24
	title := "Планирование удара"
	if m.Kind == "recon" {
		title = "Разведывательный вылет"
	}
	drawBold(u.screen, title, float64(px), float64(py), 16, colAccent, 0)
	py += 24
	drawText(u.screen, fmt.Sprintf("%s — на складе %.0f", m.Name, v.Stocks[m.ID]), float64(px), float64(py), 14, colText, 0)
	py += 22
	direct := m.Kind == "ballistic" || m.Kind == "rocket" || (m.Kind == "cruise" && m.Class == "high")
	help := "ЛКМ — точки маршрута в обход ПВО, последняя точка — цель (щелчок по разведанному объекту ставит точку точно на него). ПКМ — убрать точку."
	if direct {
		help = "Баллистика летит напрямую: щёлкните по цели. ПКМ — убрать точку."
	}
	if m.Kind == "recon" {
		help = "ЛКМ — точки облёта. После последней точки разведчик вернётся на базу."
	}
	py = g.para(help, px, py, pw, colDim)
	py += 4
	l, rng := g.strikeLength()
	lc := colText
	if l > rng {
		lc = colBad
	}
	py = g.kv("Длина маршрута / дальность", fmt.Sprintf("%.0f / %.0f км", l, rng), px, py, pw, lc)
	if l > 0 && m.SpeedKmh > 0 {
		py = g.kv("Время полёта", fmtMin(l/m.SpeedKmh*60), px, py, pw, colText)
	}
	if blocked != "" {
		py = g.kv("Воздушное пространство", "закрыто: "+blocked, px, py, pw, colBad)
	}
	if m.Kind != "recon" {
		// Колонки: подпись слева, значение прижато вправо к кнопкам, кнопки с одной и той же x у обеих строк.
		drawText(u.screen, "Количество:", float64(px), float64(py+3), 14, colDim, 0)
		mx := g.maxSalvo()
		for i, d := range []int{-10, -1, 1, 10} {
			lbl := fmt.Sprintf("%+d", d)
			if u.Button(px+strikeBtnX+i*42, py, 38, 24, lbl) {
				g.strike.Count = max(1, min(mx, g.strike.Count+d))
			}
		}
		if u.Button(px+strikeBtnX+4*42, py, pw-strikeBtnX-4*42, 24, fmt.Sprintf("Макс %d", mx)) {
			g.strike.Count = max(1, mx)
		}
		u.Tooltip(px+strikeBtnX+4*42, py, pw-strikeBtnX-4*42, 24, "Максимальный залп: ограничен запасом, залпом пусковой или пропускной способностью площадки")
		drawBold(u.screen, fmt.Sprintf("%d", g.strike.Count), float64(px+strikeBtnX-8), float64(py+3), 14, colText, 2)
		py += 28
	}
	drawText(u.screen, "Задержка:", float64(px), float64(py+3), 14, colDim, 0)
	for i, d := range []float64{-30, -5, 5, 30} {
		if u.Button(px+strikeBtnX+i*42, py, 38, 24, fmt.Sprintf("%+.0f", d)) {
			g.strike.Delay = math.Max(0, g.strike.Delay+d)
		}
	}
	drawBold(u.screen, fmt.Sprintf("%.0f мин", g.strike.Delay), float64(px+strikeBtnX-8), float64(py+3), 14, colText, 2)
	u.Tooltip(px, py, pw, 24, "Задержка позволяет синхронизировать несколько групп: например, пустить ложные цели раньше ракет.")
	py += 32
	if u.Button(px, py, pw/2-4, 30, "Отмена") {
		g.mode = modeNone
		g.strike = strikePlan{}
		return
	}
	busy := g.sourceBusy(g.strike.Source)
	if busy != "" {
		drawText(u.screen, busy, float64(px), float64(py-18), 13, colWarn, 2)
	}
	can := len(g.strike.Pts) > 0 && l <= rng && blocked == "" && busy == ""
	if u.ButtonState(px+pw/2+4, py, pw/2-4, 30, "Пуск! (F)", false, can) {
		g.confirmStrike()
	}
}

// confirmStrike отправляет запланированный удар (кнопка «Пуск!» и клавиша F).
func (g *Game) confirmStrike() {
	m := g.cat.MunitionByID[g.strike.Munition]
	pts := g.strike.Pts
	if m == nil || len(pts) == 0 {
		return
	}
	l, rng := g.strikeLength()
	if l > rng {
		g.toast("Цель вне досягаемости")
		return
	}
	if c := g.strikeBlocked(); c != "" {
		g.toast("Маршрут проходит через закрытое воздушное пространство: " + c)
		return
	}
	if b := g.sourceBusy(g.strike.Source); b != "" {
		g.toast(b)
		return
	}
	target := pts[len(pts)-1]
	var wps []sim.Pt
	if m.Kind == "recon" {
		wps = append([]sim.Pt{}, pts...)
	} else {
		wps = append([]sim.Pt{}, pts[:len(pts)-1]...)
	}
	g.sess.Send(sim.Command{Kind: sim.CmdStrike, ID: g.strike.Source, Item: g.strike.Munition, Count: g.strike.Count, Pts: wps, X: target.X, Y: target.Y, Delay: g.strike.Delay})
	g.mode = modeNone
	g.strike = strikePlan{}
}

// strikeBtnX — x кнопок «Количество» и «Задержка» относительно левого края панели планирования удара.
const strikeBtnX = 146

func (g *Game) drawHelp() {
	u := &g.ui
	v := g.view
	lines := []string{
		"ЛКМ — выбрать, перетаскивание — сдвиг карты",
		"Колесо — масштаб, WASD/стрелки — прокрутка",
		"ПКМ — марш юнита, F — пуск (пусковая выбрана)",
		"Shift+щелчок или Shift+рамка — выбрать несколько юнитов",
		"Ctrl+1…9 — в группу, 1…9 — выбрать группу",
		"Пробел — пауза, [ ] — скорость, F5 — сохранить",
		"Esc — меню, F11 — экран, Ctrl+«+»/«−» — масштаб",
		"F1 — показать или скрыть эту подсказку",
	}
	if !v.War {
		lines = append([]string{fmt.Sprintf("Подготовка: до войны %s", fmtMin(v.PrepEnd-v.Time))}, lines...)
	}
	// Строки длиннее плашки переносятся, а не вылезают за рамку.
	type hl struct {
		text string
		warn bool
	}
	var out []hl
	for i, l := range lines {
		for _, w := range wrap(l, 13, infoW-32) {
			out = append(out, hl{w, i == 0 && !v.War})
		}
	}
	// Через 90 секунд (если игрок сам не трогал) плашка сворачивается в маленькую кнопку; F1 — показать снова.
	if !g.helpPinned && g.helpOpen && time.Since(g.helpStart) > 90*time.Second {
		g.helpOpen = false
	}
	if !g.helpOpen {
		bw, bh := 150, 28
		bx, by := u.W-bw-8, u.H-bh-8
		if u.Button(bx, by, bw, bh, "F1 — подсказки") {
			g.helpOpen, g.helpPinned = true, true
		}
		return
	}
	h := len(out)*18 + 20
	x, y := u.W-infoW-8, u.H-h-8
	u.Panel(x, y, infoW, h)
	if u.Button(x+infoW-34, y+6, 26, 26, "×") {
		g.helpOpen, g.helpPinned = false, true
		return
	}
	for i, l := range out {
		c := color.RGBA{196, 202, 210, 255}
		if l.warn {
			c = colWarn
		}
		drawText(u.screen, l.text, float64(x+16), float64(y+9+i*18), 13, c, 0)
	}
}

// maxSalvo — максимальный залп для текущего плана.
func (g *Game) maxSalvo() int {
	v := g.view
	n := int(v.Stocks[g.strike.Munition])
	if un := g.findUnit(g.strike.Source); un != nil {
		n = min(n, g.cat.UnitByID[un.Type].Salvo)
	}
	if b := g.findBuilding(g.strike.Source); b != nil {
		n = min(n, int(math.Floor(b.Budget)))
	}
	if m := g.cat.MunitionByID[g.strike.Munition]; m != nil && m.Kind == "recon" {
		n = min(n, 1)
	}
	return max(n, 1)
}

// drawPlacementPanel — расстановка резерва перед стартом партии.
func (g *Game) drawPlacementPanel() {
	u := &g.ui
	v := g.view
	var types []string
	total := 0
	for t, n := range v.Reserve {
		types = append(types, t)
		total += n
	}
	sort.Strings(types)
	h := 190 + 28*len(types)
	if h > u.H-topH-40 {
		h = u.H - topH - 40
	}
	x, y := u.W-infoW-8, u.H-h-8
	u.Panel(x, y, infoW, h)
	px, py, pw := x+12, y+10, infoW-24
	drawBold(u.screen, "Расстановка перед стартом", float64(px), float64(py), 16, colAccent, 0)
	py += 26
	py = g.para("Поставьте выданные комплексы ПВО, РЛС, пусковые, площадки и центры подготовки на своей территории (не ближе 15 км к фронту). ПКМ по поставленному — вернуть в резерв. Время стоит, пока обе стороны не нажмут «Готово». Противник не видит, что и где вы поставили.", px, py, pw, colDim)
	py += 4
	if g.mode == modePlace && v.Reserve[g.placeType] == 0 {
		g.mode, g.placeType = modeNone, ""
	}
	for _, t := range types {
		name := g.bName(t)
		if un := g.cat.UnitByID[t]; un != nil {
			name = un.Name
		}
		sel := g.mode == modePlace && g.placeType == t
		if u.ButtonState(px, py, pw, 24, fmt.Sprintf("%s — осталось %d", name, v.Reserve[t]), sel, true) {
			g.mode, g.placeType = modePlace, t
		}
		if un := g.cat.UnitByID[t]; un != nil && un.Desc != "" {
			u.Tooltip(px, py, pw, 24, un.Desc)
		} else if bt := g.cat.BuildingByID[t]; bt != nil {
			u.Tooltip(px, py, pw, 24, bt.Desc)
		}
		py += 28
	}
	if len(types) == 0 {
		drawText(u.screen, "Всё расставлено", float64(px), float64(py), 14, colGood, 0)
		py += 24
	}
	label := "Готово"
	if v.Ready {
		label = "Готово (отменить)"
	}
	if u.ButtonState(px, py+4, pw, 30, label, v.Ready, total == 0 || v.Ready) {
		r := 1
		if v.Ready {
			r = 0
		}
		g.sess.Send(sim.Command{Kind: sim.CmdReady, Int: r})
	}
	st := "Противник расставляет"
	c := colWarn
	if v.EnemyReady {
		st, c = "Противник готов", colGood
	}
	drawText(u.screen, st, float64(px), float64(py+42), 13, c, 0)
}

// fireModeNames — режимы огня ПВО.
var fireModeNames = [3]string{"Все цели", "Без дронов", "Баллистика"}

const fireModeTip = "Режим огня комплекса ПВО. «Все цели» — по всему, что видит. «Без дронов» — не тратить ракеты на дроны, разведчиков и ложные цели (дорогие ракеты берегутся для ракет и авиации). «Баллистика» — только по баллистическим ракетам."

// fireModeButtons рисует переключатель режима огня для группы комплексов ПВО; cur — текущий режим (первого).
func (g *Game) fireModeButtons(ids []uint32, cur int, x, y, w int) int {
	u := &g.ui
	bw := (w - 8) / 3
	u.rowSize = fitRow(fireModeNames[:], bw, 24)
	for m := 0; m < 3; m++ {
		if u.ButtonState(x+m*(bw+4), y, bw, 24, fireModeNames[m], cur == m, true) {
			for _, id := range ids {
				g.sess.Send(sim.Command{Kind: sim.CmdFireMode, ID: id, Int: m})
			}
		}
	}
	u.rowSize = 0
	u.Tooltip(x, y, w, 24, fireModeTip)
	return y + 30
}
