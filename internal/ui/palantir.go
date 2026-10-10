package ui

import (
	"fmt"
	"image/color"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/exrise/droneebla/internal/sim"
)

// «ИИ Палантир» (Украина): подписка и ЦОД, затем удар одной кнопкой — боеприпас, число и цель выбирает игрок,
// пусковые, маршруты в обход известной ПВО и синхронизацию прилёта подбирает хост.

type palState struct {
	Munition string
	Count    int
	Target   sim.Pt
	HasT     bool
	manual   bool // ручное планирование (иначе показываются предложения «Палантира»)
	sentKey  string
	sentAt   time.Time
}

const palRefreshEvery = 250 * time.Millisecond

// palMunitions — боеприпасы стороны, которыми можно бить через «Палантир» (есть на складе).
func (g *Game) palMunitions() []string {
	v := g.view
	var ids []string
	for _, m := range g.cat.Munitions {
		if m.Side != "" && sideIdx(m.Side) == v.Side && m.Kind != "interceptor" && m.Kind != "decoy" && m.Kind != "recon" && v.Stocks[m.ID] >= 1 {
			ids = append(ids, m.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

func sideIdx(k string) int {
	if k == "ru" {
		return 0
	}
	return 1
}

func (g *Game) palKey() string {
	p := &g.pal
	return sim.PalKey(p.Munition, p.Count, p.Target.X, p.Target.Y)
}

// startPalantir входит в режим планирования; target — готовая цель (контакт) или nil.
func (g *Game) startPalantir(target *sim.Pt) {
	g.mode = modePalantir
	g.pal.manual = true
	g.strike = strikePlan{}
	ms := g.palMunitions()
	if g.pal.Munition == "" || g.view.Stocks[g.pal.Munition] < 1 {
		g.pal.Munition = ""
		if len(ms) > 0 {
			g.pal.Munition = ms[0]
		}
	}
	if g.pal.Count < 1 {
		g.pal.Count = 4
	}
	if target != nil {
		g.pal.Target, g.pal.HasT = *target, true
	}
	g.pal.sentKey = ""
}

// startPalantirSuggest открывает предложения «Палантира»: он сам выбирает цели, боеприпас и количество.
func (g *Game) startPalantirSuggest() {
	g.mode = modePalantir
	g.pal.manual = false
	g.strike = strikePlan{}
	g.sess.Send(sim.Command{Kind: sim.CmdPalantirSuggest})
	g.pal.sentAt = time.Now()
}

// drawPalantirSuggest — панель предложений: игрок только подтверждает.
func (g *Game) drawPalantirSuggest() {
	u := &g.ui
	v := g.view
	pl := v.Palantir
	rows := len(pl.Suggest)
	h := 150
	if pl.Active && rows > 0 {
		h = 120 + rows*66
	}
	x, y := u.W-infoW-8, u.H-h-8
	u.Panel(x, y, infoW, h)
	px, py, pw := x+12, y+10, infoW-24
	drawBold(u.screen, "ИИ «Палантир» предлагает", float64(px), float64(py), 16, colAccent, 0)
	py += 26
	by := y + h - 40
	if !pl.Active {
		g.para("«Палантир» не работает: "+pl.Reason+".", px, py, pw, colBad)
		if u.Button(px, by, pw, 30, "Закрыть") {
			g.mode = modeNone
		}
		return
	}
	if rows == 0 {
		g.para("Подходящих целей нет: нужны известные объекты противника в досягаемости готовых пусковых. Запустите разведку или дождитесь перезарядки.", px, py, pw, colDim)
	}
	for i, p := range pl.Suggest {
		m := g.cat.MunitionByID[p.Item]
		if m == nil {
			continue
		}
		ry := py + i*66
		fillRect(u.screen, float64(px-4), float64(ry-3), float64(pw+8), 62, color.RGBA{255, 255, 255, 14})
		drawBold(u.screen, fmt.Sprintf("%d. %s", i+1, fitText(p.Target, 14, float64(pw-100))), float64(px), float64(ry), 14, colText, 0)
		drawText(u.screen, fmt.Sprintf("%s ×%d (в плане %d, пусковых %d)", m.Name, p.Count, p.Total, p.Legs), float64(px), float64(ry+19), 13, colText, 0)
		cc := colGood
		if p.Cover > 0 {
			cc = colWarn
		}
		info := fmt.Sprintf("прилёт через %s · ПВО у цели: %d кан.", fmtMin(p.Arrive), p.Cover)
		if p.Age >= 0 {
			info += " · данные " + fmtMin(p.Age) + " назад"
		}
		drawText(u.screen, fitText(info, 12, float64(pw-90)), float64(px), float64(ry+38), 12, cc, 0)
		if u.Button(px+pw-84, ry+4, 84, 28, "Пуск") {
			g.sess.Send(sim.Command{Kind: sim.CmdPalantirStrike, Item: p.Item, Count: p.Count, X: p.X, Y: p.Y})
			g.sess.Send(sim.Command{Kind: sim.CmdPalantirSuggest})
		}
		u.Tooltip(px-4, ry-3, pw+8, 62, fmt.Sprintf("Подтвердите — «Палантир» подберёт пусковые, обойдёт известную ПВО и синхронизирует прилёт.\n%s: дальность %.0f км, на складе %.0f.", m.Name, m.RangeKm, v.Stocks[m.ID]))
	}
	bw := (pw - 8) / 3
	if u.Button(px, by, bw, 30, "Обновить") {
		g.sess.Send(sim.Command{Kind: sim.CmdPalantirSuggest})
	}
	if u.Button(px+bw+4, by, bw, 30, "Вручную") {
		g.startPalantir(nil)
	}
	if u.Button(px+2*(bw+4), by, bw, 30, "Закрыть") {
		g.mode = modeNone
	}
}

func (g *Game) palConfirm() {
	if !g.pal.manual {
		// F в режиме предложений подтверждает первое из них.
		if sg := g.view.Palantir.Suggest; len(sg) > 0 && g.view.Palantir.Active {
			g.sess.Send(sim.Command{Kind: sim.CmdPalantirStrike, Item: sg[0].Item, Count: sg[0].Count, X: sg[0].X, Y: sg[0].Y})
			g.sess.Send(sim.Command{Kind: sim.CmdPalantirSuggest})
		}
		return
	}
	p := &g.pal
	pv := g.view.Palantir.Preview
	if !p.HasT || pv.Key != g.palKey() || pv.Err != "" {
		g.toast("План ещё не рассчитан или невозможен")
		return
	}
	g.sess.Send(sim.Command{Kind: sim.CmdPalantirStrike, Item: p.Munition, Count: p.Count, X: p.Target.X, Y: p.Target.Y})
	p.sentKey = ""
	g.mode = modeNone
}

// drawPalantirPlan рисует рассчитанные маршруты на карте.
func (g *Game) drawPalantirPlan(dst *ebiten.Image) {
	p := &g.pal
	if !p.manual {
		for i, sg := range g.view.Palantir.Suggest {
			sx, sy := g.cam.ToScreen(sg.X, sg.Y)
			col := color.RGBA{60, 140, 220, 255}
			circle(dst, sx, sy, 10, col, 2)
			drawTextHalo(dst, fmt.Sprintf("%d", i+1), sx+12, sy-12, 14, col, color.White, 0)
		}
		return
	}
	if !p.HasT {
		return
	}
	tx, ty := g.cam.ToScreen(p.Target.X, p.Target.Y)
	col := color.RGBA{60, 140, 220, 255}
	circle(dst, tx, ty, 9, col, 2)
	line(dst, tx-12, ty, tx+12, ty, col, 1.5)
	line(dst, tx, ty-12, tx, ty+12, col, 1.5)
	pv := g.view.Palantir.Preview
	if pv.Key != g.palKey() || pv.Err != "" {
		return
	}
	for _, l := range pv.Legs {
		px, py := g.cam.ToScreen(l.SX, l.SY)
		disc(dst, px, py, 4, col)
		for _, w := range l.Wps {
			qx, qy := g.cam.ToScreen(w.X, w.Y)
			line(dst, px, py, qx, qy, withAlpha(col, 200), 2)
			disc(dst, qx, qy, 3, col)
			px, py = qx, qy
		}
		line(dst, px, py, tx, ty, withAlpha(col, 200), 2)
	}
}

// drawPalantirPanel — панель планирования удара через «Палантир».
func (g *Game) drawPalantirPanel() {
	if !g.pal.manual {
		g.drawPalantirSuggest()
		return
	}
	u := &g.ui
	v := g.view
	pl := v.Palantir
	p := &g.pal
	m := g.cat.MunitionByID[p.Munition]
	h := 330
	if !pl.Active || len(g.palMunitions()) == 0 {
		h = 150
	}
	x, y := u.W-infoW-8, u.H-h-8
	u.Panel(x, y, infoW, h)
	px, py, pw := x+12, y+10, infoW-24
	drawBold(u.screen, "ИИ «Палантир» — удар", float64(px), float64(py), 16, colAccent, 0)
	py += 26
	if !pl.Active {
		py = g.para("«Палантир» не работает: "+pl.Reason+".", px, py, pw, colBad)
		if u.Button(px, y+h-40, pw, 30, "Закрыть") {
			g.mode = modeNone
		}
		return
	}
	ms := g.palMunitions()
	if m == nil || v.Stocks[p.Munition] < 1 {
		if len(ms) == 0 {
			py = g.para("На складе нет боеприпасов для ударов.", px, py, pw, colBad)
			if u.Button(px, y+h-40, pw, 30, "Закрыть") {
				g.mode = modeNone
			}
			return
		}
		p.Munition = ms[0]
		m = g.cat.MunitionByID[p.Munition]
	}
	idx := 0
	for i, id := range ms {
		if id == p.Munition {
			idx = i
		}
	}
	// Боеприпас: ◀ название ▶.
	if u.Button(px, py, 30, 26, "◀") && len(ms) > 0 {
		p.Munition = ms[(idx+len(ms)-1)%len(ms)]
	}
	if u.Button(px+pw-30, py, 30, 26, "▶") && len(ms) > 0 {
		p.Munition = ms[(idx+1)%len(ms)]
	}
	drawBold(u.screen, fitText(m.Name, 14, float64(pw-80)), float64(px+pw/2), float64(py+5), 14, colText, 1)
	py += 32
	drawText(u.screen, fmt.Sprintf("На складе: %.0f · дальность %.0f км", v.Stocks[m.ID], m.RangeKm), float64(px), float64(py), 13, colDim, 0)
	py += 22
	drawText(u.screen, "Количество:", float64(px), float64(py+3), 14, colDim, 0)
	mx := int(v.Stocks[m.ID])
	for i, d := range []int{-10, -1, 1, 10} {
		if u.Button(px+strikeBtnX+i*42, py, 38, 24, fmt.Sprintf("%+d", d)) {
			p.Count = max(1, min(mx, p.Count+d))
		}
	}
	if u.Button(px+strikeBtnX+4*42, py, pw-strikeBtnX-4*42, 24, "Макс") {
		p.Count = max(1, mx)
	}
	drawBold(u.screen, fmt.Sprintf("%d", p.Count), float64(px+strikeBtnX-8), float64(py+3), 14, colText, 2)
	py += 32
	if !p.HasT {
		py = g.para("Укажите цель на карте: щёлкните по разведанному объекту или точке.", px, py, pw, colWarn)
	} else {
		py = g.kv("Цель", fmt.Sprintf("%.0f, %.0f км", p.Target.X, p.Target.Y), px, py, pw, colText)
	}
	// План: запрос уходит хосту при изменении параметров, ответ приходит в представлении.
	if p.HasT {
		key := g.palKey()
		if key != p.sentKey && time.Since(p.sentAt) > palRefreshEvery {
			g.sess.Send(sim.Command{Kind: sim.CmdPalantirPlan, Item: p.Munition, Count: p.Count, X: p.Target.X, Y: p.Target.Y})
			p.sentKey, p.sentAt = key, time.Now()
		}
	}
	pv := pl.Preview
	ready := false
	switch {
	case !p.HasT:
	case pv.Key != g.palKey():
		py = g.label("Расчёт плана…", px, py, colDim)
	case pv.Err != "":
		py = g.para(pv.Err, px, py, pw, colBad)
	default:
		ready = true
		py = g.kv("Пусковых в плане", fmt.Sprintf("%d", len(pv.Legs)), px, py, pw, colText)
		tc := colText
		if pv.Total < p.Count {
			tc = colWarn
		}
		py = g.kv("Боеприпасов", fmt.Sprintf("%d из %d", pv.Total, p.Count), px, py, pw, tc)
		py = g.kv("Прилёт всей волны", fmtMin(pv.Arrive), px, py, pw, colText)
		cc := colGood
		if pv.Cover > 0 {
			cc = colWarn
		}
		py = g.kv("Известная ПВО у цели", fmt.Sprintf("%d каналов", pv.Cover), px, py, pw, cc)
	}
	by := y + h - 40
	if u.Button(px, by, pw/2-4, 30, "Отмена") {
		g.mode = modeNone
		return
	}
	if u.ButtonState(px+pw/2+4, by, pw/2-4, 30, "Пуск! (F)", false, ready) {
		g.palConfirm()
	}
}

// palantirBlock — блок «ИИ Палантир» во вкладке «Разведка».
func (g *Game) palantirBlock(x, y, w int) int {
	u := &g.ui
	v := g.view
	pl := v.Palantir
	if !pl.Has {
		return y
	}
	y = g.header("ИИ «Палантир»", x, y)
	bname := pl.Building
	if bt := g.cat.BuildingByID[pl.Building]; bt != nil {
		bname = bt.Name
	}
	y = g.para(fmt.Sprintf("Подбирает пусковые, обходит известную ПВО и синхронизирует прилёт: достаточно указать цель, боеприпас и число. Подписка %.0f денег в час; нужен достроенный %s у воды с достаточной энергией (вкладка «Стройка»).", pl.MoneyH, bname), x, y, w, colDim)
	switch {
	case pl.Active:
		y = g.kv("Состояние", "работает", x, y, w, colGood)
	case pl.Sub:
		y = g.kv("Состояние", "не работает", x, y, w, colBad)
		y = g.para(pl.Reason, x, y, w, colBad)
	default:
		y = g.kv("Состояние", "нет подписки", x, y, w, colDim)
	}
	lbl := fmt.Sprintf("Оформить подписку (%.0f/ч)", pl.MoneyH)
	if pl.Sub {
		lbl = "Отменить подписку"
	}
	if u.ButtonState(x, y, w, 26, lbl, pl.Sub, true) {
		n := 1
		if pl.Sub {
			n = 0
		}
		g.sess.Send(sim.Command{Kind: sim.CmdPalantir, Int: n})
	}
	y += 32
	if u.ButtonState(x, y, w/2-4, 26, "Предложить удары", g.mode == modePalantir && !g.pal.manual, pl.Active && v.War) {
		g.startPalantirSuggest()
	}
	u.Tooltip(x, y, w/2-4, 26, "«Палантир» сам выберет цели, тип дронов или ракет и количество; вы только подтверждаете. Ручное планирование — кнопка «Вручную» в панели.")
	last := pl.Last
	ok := pl.Active && v.War && last.Valid
	if u.ButtonState(x+w/2+4, y, w/2-4, 26, "Повторить удар", false, ok) {
		g.sess.Send(sim.Command{Kind: sim.CmdPalantirRepeat})
	}
	if last.Valid {
		if m := g.cat.MunitionByID[last.Item]; m != nil {
			u.Tooltip(x+w/2+4, y, w/2-4, 26, fmt.Sprintf("Последний: %s ×%d", m.Name, last.Count))
		}
	}
	return y + 38
}
