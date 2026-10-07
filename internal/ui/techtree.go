package ui

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

// Окно исследований: линейки версий образцов (Герань-2 → 3 → 4 → 5), у каждой
// версии — карточка с цифрами и изменением относительно предыдущей.

const (
	cardW   = 208
	cardH   = 184
	cardGap = 8
)

// groupNames — группы линеек в окне исследований.
var groupNames = map[string]string{
	"strike": "Ударные средства", "ad": "ПВО", "intel": "Разведка и РЭБ",
	"front": "Фронт", "caps": "Возможности",
}

// techGroups — порядок групп.
var techGroups = []string{"strike", "ad", "intel", "front", "caps"}

// stat — одна строка характеристик на карточке.
type stat struct {
	label string
	v     float64
	unit  string
	up    bool // true — больше значит лучше
}

func moneyCost(c map[string]float64) stat { return stat{"Цена", c["money"], "", false} }

// itemStats — характеристики предмета для карточки. id: боеприпас, юнит, снаряжение фронта или «sat:Имя».
func (g *Game) itemStats(side int, id string) []stat {
	cat := g.cat
	if strings.HasPrefix(id, "sat:") {
		for _, s := range cat.Sides[side].Satellites {
			if s.Name == id[4:] {
				sensor := "оптика"
				if s.Sensor == "radar" {
					sensor = "радар"
				}
				return []stat{{"Полоса съёмки", s.SwathKm, " км", true}, {"Период", s.PeriodH, " ч", false}, {"Датчик: " + sensor, 0, "", true}}
			}
		}
		return nil
	}
	if m := cat.MunitionByID[id]; m != nil {
		switch m.Kind {
		case "recon":
			return []stat{{"Обзор", m.Vision, " км", true}, {"Дальность", m.RangeKm, " км", true}, {"Скорость", m.SpeedKmh, " км/ч", true}, moneyCost(m.Cost)}
		case "interceptor":
			return []stat{moneyCost(m.Cost)}
		}
		st := []stat{
			{"Дальность", m.RangeKm, " км", true}, {"Скорость", m.SpeedKmh, " км/ч", true},
			{"Боевая часть", m.Damage, "", true}, {"Точность", m.Accuracy * 100, "%", true},
		}
		if m.Evasion > 0 {
			st = append(st, stat{"Против ПВО", m.Evasion * 100, "%", true})
		} else if m.Stealth > 0 {
			st = append(st, stat{"Скрытность", m.Stealth * 100, "%", true})
		}
		return append(st, moneyCost(m.Cost))
	}
	if u := cat.UnitByID[id]; u != nil {
		switch u.Kind {
		case "ad":
			st := []stat{
				{"Дальность", u.RangeKm, " км", true}, {"Обзор РЛС", u.RadarKm, " км", true},
				{"Каналы", float64(u.Channels), "", true}, {"По низким", u.PkLow * 100, "%", true},
			}
			if u.PkHigh > 0 {
				st = append(st, stat{"По баллистике", u.PkHigh * 100, "%", true})
			}
			return append(st, moneyCost(u.Cost))
		case "radar":
			return []stat{{"Дальность", u.RadarKm, " км", true}, {"По низким", u.RadarLow * 100, "%", true}, moneyCost(u.Cost)}
		case "reb":
			return []stat{{"Радиус", u.RebKm, " км", true}, {"Подавление", u.RebPower * 100, "%", true}, moneyCost(u.Cost)}
		case "rtr":
			return []stat{{"Дальность", u.RtrKm, " км", true}, moneyCost(u.Cost)}
		case "launcher":
			return []stat{{"Залп", float64(u.Salvo), "", true}, {"Перезарядка", u.ReloadMin, " мин", false}, moneyCost(u.Cost)}
		}
		return []stat{moneyCost(u.Cost)}
	}
	if f := cat.FrontByID[id]; f != nil {
		p := f.Power
		if p <= 0 {
			p = 1
		}
		return []stat{{"Сила на фронте", p, "×", true}, {"Партия", f.Batch, " шт", true}, moneyCost(f.Cost)}
	}
	return nil
}

// primaryItem — предмет, цифры которого показываются на карточке исследования.
func (g *Game) primaryItem(t *data.Tech) string {
	for _, id := range t.Unlocks {
		if m := g.cat.MunitionByID[id]; m != nil && m.Kind == "interceptor" {
			continue
		}
		return id
	}
	if len(t.Unlocks) > 0 {
		return t.Unlocks[0]
	}
	return ""
}

func fmtStat(v float64, unit string) string {
	switch {
	case unit == "×":
		return fmt.Sprintf("×%.1f", v)
	case v >= 100 || v == math.Trunc(v):
		return fmt.Sprintf("%.0f%s", v, unit)
	}
	return fmt.Sprintf("%.1f%s", v, unit)
}

// techCard — карточка версии в линейке.
type techCard struct {
	item  string // предмет для цифр
	title string
	tech  *data.Tech // nil — версия доступна с начала
}

// lineCards — карточки линейки по порядку.
func (g *Game) lineCards(side int, l *data.TechLine) []techCard {
	var cs []techCard
	for _, id := range l.Start {
		cs = append(cs, techCard{item: id, title: g.cat.ItemName(id)})
	}
	for _, t := range g.cat.LineSteps[side][l.ID] {
		cs = append(cs, techCard{item: g.primaryItem(t), title: t.Name, tech: t})
	}
	return cs
}

// drawTechTree рисует окно исследований поверх карты.
func (g *Game) drawTechTree() {
	if !g.techOpen || g.view == nil {
		return
	}
	u := &g.ui
	v := g.view
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.techOpen = false
		return
	}
	px, py := leftW, topH
	pw, ph := u.W-leftW, u.H-topH
	u.Panel(px, py, pw, ph)
	solid := colPanel
	solid.A = 255
	fillRect(u.screen, float64(px+1), float64(py+1), float64(pw-2), float64(ph-2), solid)
	// Шапка: текущее исследование и кнопка закрытия.
	drawBold(u.screen, "Исследования: "+data.SideNames[v.Side], float64(px+14), float64(py+10), 18, colAccent, 0)
	if u.Button(px+pw-130, py+8, 116, 26, "Закрыть (T)") {
		g.techOpen = false
		return
	}
	if v.Research != "" {
		t := g.cat.TechByID[v.Side][v.Research]
		eta := (t.Cost - v.Progress[t.ID]) / math.Max(v.ResRate, 0.01)
		txt := fmt.Sprintf("Сейчас: %s — %.0f / %.0f, ~%s", t.Name, v.Progress[t.ID], t.Cost, fmtMin(eta*60))
		drawText(u.screen, txt, float64(px+14), float64(py+38), 14, colText, 0)
		u.Bar(px+14, py+58, pw-28, 8, v.Progress[t.ID]/t.Cost, colAccent)
	} else {
		drawText(u.screen, "Исследование не выбрано — нажмите «Изучить» на карточке", float64(px+14), float64(py+38), 14, colWarn, 0)
	}
	drawText(u.screen, fmt.Sprintf("Скорость: %.1f очков/ч", v.ResRate), float64(px+pw-16), float64(py+38), 14, colDim, 2)

	area := image.Rect(px, py+74, px+pw, py+ph)
	sc := g.scroll["tech"]
	if image.Pt(u.in.mx, u.in.my).In(area) && u.in.wheel != 0 {
		sc -= u.in.wheel * 50
		u.in.wheel = 0
	}
	if sc < 0 {
		sc = 0
	}
	old := u.screen
	u.screen = u.sub(area.Min.X, area.Min.Y, area.Dx(), area.Dy())
	u.clip = area
	ox, oy := px, area.Min.Y-int(sc) // подизображение сохраняет экранные координаты
	y := oy + 6
	for _, grp := range techGroups {
		var lines []*data.TechLine
		for i := range g.cat.Lines[data.SideKeys[v.Side]] {
			l := &g.cat.Lines[data.SideKeys[v.Side]][i]
			if l.Group == grp && (len(g.cat.LineSteps[v.Side][l.ID]) > 0 || len(l.Start) > 0) {
				lines = append(lines, l)
			}
		}
		var caps []*data.Tech
		if grp == "caps" {
			for i := range g.cat.Tech[data.SideKeys[v.Side]] {
				if t := &g.cat.Tech[data.SideKeys[v.Side]][i]; t.Line == "" {
					caps = append(caps, t)
				}
			}
		}
		if len(lines) == 0 && len(caps) == 0 {
			continue
		}
		drawBold(u.screen, groupNames[grp], float64(ox+14), float64(y), 16, colAccent, 0)
		y += 26
		for _, l := range lines {
			y = g.drawLine(v, l, ox+14, y, pw-28, area)
		}
		if len(caps) > 0 {
			cs := make([]techCard, len(caps))
			for i, t := range caps {
				cs[i] = techCard{title: t.Name, tech: t}
			}
			y = g.drawCardRow(v, "", cs, ox+14, y, pw-28, area)
		}
		y += 6
	}
	u.screen = old
	u.clip = image.Rectangle{}
	content := float64(y - oy)
	g.scroll["tech"] = math.Min(sc, math.Max(0, content-float64(area.Dy())+20))
}

// drawLine рисует название линейки и ряд карточек её версий.
func (g *Game) drawLine(v *sim.View, l *data.TechLine, x, y, w int, area image.Rectangle) int {
	return g.drawCardRow(v, l.Name, g.lineCards(v.Side, l), x, y, w, area)
}

// drawCardRow рисует ряд карточек (с переносом) и возвращает новый y.
func (g *Game) drawCardRow(v *sim.View, title string, cs []techCard, x, y, w int, area image.Rectangle) int {
	u := &g.ui
	if title != "" {
		drawBold(u.screen, title, float64(x), float64(y), 14, colText, 0)
		y += 22
	}
	per := (w + cardGap) / (cardW + cardGap)
	if per < 1 {
		per = 1
	}
	var prev []stat
	for i, c := range cs {
		col, row := i%per, i/per
		cx, cy := x+col*(cardW+cardGap), y+row*(cardH+cardGap)
		if col == 0 {
			prev = nil
		}
		st := g.itemStats(v.Side, c.item)
		g.drawCard(v, c, st, prev, cx, cy, area)
		prev = st
	}
	rows := (len(cs) + per - 1) / per
	return y + rows*(cardH+cardGap) + 4
}

// drawCard рисует одну карточку (экранные координаты).
func (g *Game) drawCard(v *sim.View, c techCard, st, prev []stat, x, y int, area image.Rectangle) {
	u := &g.ui
	status, by := sim.TechOpen, ""
	if c.tech != nil {
		status, by = sim.TechStatus(v.Researched, g.cat, v.Side, c.tech.ID)
	} else {
		status = sim.TechDone
	}
	active := c.tech != nil && v.Research == c.tech.ID
	border, bg := colBorder, colPanel
	switch {
	case active:
		border = colAccent
	case status == sim.TechDone:
		border = colGood
	case status == sim.TechOpen:
		border = colWarn
	}
	fillRect(u.screen, float64(x), float64(y), cardW, cardH, bg)
	strokeRect(u.screen, float64(x), float64(y), cardW, cardH, border, 1.5)

	// Название (до двух строк).
	nameCol := colText
	if status == sim.TechNeeds {
		nameCol = colDim
	}
	lines := wrap(c.title, 14, cardW-14)
	for k := 0; k < len(lines) && k < 2; k++ {
		drawBold(u.screen, lines[k], float64(x+7), float64(y+6+k*17), 14, nameCol, 0)
	}
	ty := y + 42
	if len(st) == 0 && c.tech != nil {
		for _, l := range wrap(c.tech.Desc, 12, cardW-14) {
			if ty > y+cardH-50 {
				break
			}
			drawText(u.screen, l, float64(x+7), float64(ty), 12, colDim, 0)
			ty += 15
		}
	}
	for _, s := range st {
		if s.v == 0 && s.unit != "" {
			continue // у ложных целей нет боевой части и т. п.
		}
		drawText(u.screen, s.label, float64(x+7), float64(ty), 12, colDim, 0)
		val := fmtStat(s.v, s.unit)
		if s.v == 0 && s.unit == "" {
			val = ""
		}
		drawText(u.screen, val, float64(x+cardW-7), float64(ty), 12, colText, 2)
		for _, p := range prev {
			if p.label != s.label || p.v == s.v {
				continue
			}
			d := s.v - p.v
			col := colGood
			if (d > 0) != s.up {
				col = colBad
			}
			dt := fmtStat(math.Abs(d), s.unit)
			sign := "+"
			if d < 0 {
				sign = "−"
			}
			drawText(u.screen, "("+sign+dt+")", float64(x+cardW-12)-textWidth(val, 12), float64(ty), 11, col, 2)
		}
		ty += 16
	}

	// Подвал: состояние или кнопка.
	fy := y + cardH - 30
	switch {
	case c.tech == nil:
		drawText(u.screen, "Есть с начала", float64(x+cardW/2), float64(fy+6), 12, colGood, 1)
	case status == sim.TechDone:
		drawText(u.screen, "Изучено", float64(x+cardW/2), float64(fy+6), 13, colGood, 1)
	case active:
		u.Bar(x+7, fy+4, cardW-14, 8, v.Progress[c.tech.ID]/c.tech.Cost, colAccent)
		drawText(u.screen, "Исследуется", float64(x+cardW/2), float64(fy+14), 11, colAccent, 1)
	case status == sim.TechOpen:
		eta := c.tech.Cost / math.Max(v.ResRate, 0.01)
		lbl := fmt.Sprintf("Изучить · %.0f · ~%s", c.tech.Cost, fmtMin(eta*60))
		if u.ButtonState(x+7, fy, cardW-14, 22, lbl, false, true) {
			g.sess.Send(sim.Command{Kind: sim.CmdResearch, Item: c.tech.ID})
		}
	default:
		need := ""
		if t := g.cat.TechByID[v.Side][by]; t != nil {
			need = t.Name
		}
		drawText(u.screen, fitText("Сначала: "+need, 12, cardW-14), float64(x+cardW/2), float64(fy+6), 12, colDim, 1)
	}

	// Подсказка: описание и что открывает.
	if c.tech != nil {
		tip := c.tech.Name
		if c.tech.Desc != "" {
			tip += "\n" + c.tech.Desc
		}
		if len(c.tech.Unlocks) > 0 {
			var names []string
			for _, id := range c.tech.Unlocks {
				if strings.HasPrefix(id, "sat:") {
					names = append(names, id[4:])
				} else {
					names = append(names, g.cat.ItemName(id))
				}
			}
			tip += "\nОткрывает: " + strings.Join(names, ", ")
		}
		tip += fmt.Sprintf("\nСтоимость: %.0f очков", c.tech.Cost)
		g.techTip(x, y, tip)
	} else if m := g.cat.MunitionByID[c.item]; m != nil && m.Desc != "" {
		g.techTip(x, y, m.Desc)
	} else if un := g.cat.UnitByID[c.item]; un != nil && un.Desc != "" {
		g.techTip(x, y, un.Desc)
	}
}

// techTip показывает подсказку над карточкой.
func (g *Game) techTip(x, y int, tip string) {
	g.ui.Tooltip(x, y, cardW, cardH, tip)
}
