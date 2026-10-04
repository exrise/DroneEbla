package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

const (
	topH  = 56
	leftW = 420
	infoW = 390
	infoH = 330
)

// hoverItem — объект под курсором.
type hoverItem struct {
	kind string
	id   uint32
	idx  int
	x, y float64 // км
	d    float64 // расстояние в пикселях
}

func (g *Game) gameKeys() {
	if g.sess == nil {
		return
	}
	pan := 600 / g.cam.Z / 60
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp) {
		g.cam.CY -= pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown) {
		g.cam.CY += pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft) {
		g.cam.CX -= pan
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight) {
		g.cam.CX += pan
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd) {
		g.zoomAt(1.25, float64(g.cam.X+g.cam.W/2), float64(g.cam.Y+g.cam.H/2))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract) {
		g.zoomAt(0.8, float64(g.cam.X+g.cam.W/2), float64(g.cam.Y+g.cam.H/2))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) && g.view != nil {
		p := 1
		if g.view.Pausing {
			p = 0
		}
		g.sess.Send(sim.Command{Kind: sim.CmdPause, Int: p})
	}
	for k := 1; k <= 5; k++ {
		if inpututil.IsKeyJustPressed(ebiten.Key0 + ebiten.Key(k)) {
			g.sess.Send(sim.Command{Kind: sim.CmdSpeed, Int: k})
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if g.mode != modeNone {
			g.mode = modeNone
			g.strike = strikePlan{}
		} else {
			g.sel = Selection{}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		g.quickSave()
	}
}

func (g *Game) zoomAt(k, sx, sy float64) {
	wx, wy := g.cam.ToWorld(sx, sy)
	g.cam.Z = math.Max(0.3, math.Min(14, g.cam.Z*k))
	nx, ny := g.cam.ToWorld(sx, sy)
	g.cam.CX += wx - nx
	g.cam.CY += wy - ny
}

func (g *Game) drawGame() {
	u := &g.ui
	if v := g.sess.View(); v != nil {
		g.view = v
	}
	for _, m := range g.sess.Messages() {
		g.toast(m)
	}
	g.cam.X, g.cam.Y = leftW, topH
	g.cam.W, g.cam.H = u.W-leftW, u.H-topH
	if g.view == nil {
		st := g.sess.Status()
		if st == "" {
			st = "Загрузка…"
		}
		drawText(u.screen, st, float64(u.W/2), float64(u.H/2), 20, colText, 1)
		if u.Button(u.W/2-90, u.H/2+50, 180, 36, "В главное меню") {
			g.leaveGame()
		}
		return
	}
	v := g.view
	// Новые события → всплывающие сообщения для важных.
	for _, e := range v.Events {
		if e.ID > g.evSeen {
			if e.Level >= 1 && g.evSeen > 0 {
				g.toast(e.Text)
			}
			g.evSeen = e.ID
		}
	}
	g.rend.update(v)

	// Карта.
	mapImg := u.sub(g.cam.X, g.cam.Y, g.cam.W, g.cam.H)
	mapImg.Fill(colSea)
	g.rend.drawBase(mapImg, &g.cam, g.layers, v)
	hov := g.drawEntities(mapImg)

	// Панели. Кнопка «Меню» может завершить игру посреди кадра.
	g.drawTopBar()
	if g.sess == nil {
		return
	}
	g.drawLeftPanel()
	g.drawInfoPanel()
	g.drawLayerButtons()
	g.drawToasts()
	if st := g.sess.Status(); st != "" {
		w := textWidth(st, 16) + 30
		fillRect(u.screen, float64(g.cam.X)+float64(g.cam.W)/2-w/2, float64(topH+10), w, 32, color.RGBA{120, 30, 20, 230})
		drawText(u.screen, st, float64(g.cam.X)+float64(g.cam.W)/2, float64(topH+16), 16, colText, 1)
	}
	if v.Winner >= 0 {
		g.drawVictory()
		if g.sess == nil {
			return
		}
	}

	// Ввод карты.
	g.mapInput(hov)
}

func (g *Game) leaveGame() {
	if g.sess != nil {
		g.sess.Close()
	}
	g.sess = nil
	g.view = nil
	g.scene = sceneMenu
}

// ---------------------------------------------------------------------
// Объекты на карте.

func (g *Game) bName(t string) string {
	if b := g.cat.BuildingByID[t]; b != nil {
		return b.Name
	}
	return t
}

func (g *Game) bShort(t string) string {
	if b := g.cat.BuildingByID[t]; b != nil {
		return b.Short
	}
	return "?"
}

func (g *Game) uName(t string) string {
	if u := g.cat.UnitByID[t]; u != nil {
		return u.Name
	}
	return t
}

func withAlpha(c color.RGBA, a uint8) color.RGBA {
	k := float64(a) / 255
	return color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), a}
}

func (g *Game) visibleOnScreen(sx, sy float64) bool {
	return sx > float64(g.cam.X-40) && sy > float64(g.cam.Y-40) && sx < float64(g.cam.X+g.cam.W+40) && sy < float64(g.cam.Y+g.cam.H+40)
}

func (g *Game) drawEntities(dst *ebiten.Image) *hoverItem {
	v := g.view
	u := &g.ui
	mx, my := float64(u.in.mx), float64(u.in.my)
	var hov *hoverItem
	consider := func(kind string, id uint32, idx int, x, y float64, sx, sy float64) {
		d := math.Hypot(sx-mx, sy-my)
		if d < 12 && (hov == nil || d < hov.d) {
			hov = &hoverItem{kind: kind, id: id, idx: idx, x: x, y: y, d: d}
		}
	}
	z := g.cam.Z
	my2 := v.Side
	en := 1 - my2
	g.labels = g.labels[:0]

	// Недавно захваченные тайлы: яркая подсветка, гаснет за 3 игровых часа.
	for _, c := range v.Captures {
		i := int(c.Tile)
		age := v.Time - float64(c.Time)
		if age < 0 || age > 180 {
			continue
		}
		tx, ty := i%g.m.W, i/g.m.W
		x0, y0 := g.cam.ToScreen(float64(tx)*g.m.TileKm, float64(ty)*g.m.TileKm)
		ts := g.m.TileKm * z
		if !g.visibleOnScreen(x0, y0) {
			continue
		}
		k := 1 - age/180
		col := color.RGBA{255, 210, 40, 255}
		if int(c.Side) == my2 {
			col = color.RGBA{60, 220, 90, 255}
		} else {
			col = color.RGBA{255, 60, 40, 255}
		}
		fillRect(dst, x0, y0, ts, ts, withAlpha(col, uint8(30+120*k)))
		if age < 30 {
			strokeRect(dst, x0, y0, ts, ts, withAlpha(col, 230), 1.5)
		}
	}

	// Зоны ПВО.
	if g.layers["ad"] {
		for _, un := range v.Units {
			ut := g.cat.UnitByID[un.Type]
			if ut.Kind != "ad" && ut.Kind != "reb" {
				continue
			}
			sx, sy := g.cam.ToScreen(un.X, un.Y)
			r := ut.RangeKm
			c := withAlpha(sideColor(my2), 90)
			if ut.Kind == "reb" {
				r = ut.RebKm
				c = color.RGBA{120, 60, 140, 110}
			}
			if r*z < 3 {
				continue
			}
			if un.State != sim.UnitDeployed {
				c = color.RGBA{110, 110, 110, 90}
			}
			circle(dst, sx, sy, r*z, c, 1.2)
		}
		for _, c := range v.Contacts {
			ut := g.cat.UnitByID[c.Type]
			if c.Kind != 1 || ut == nil || ut.Kind != "ad" {
				continue
			}
			sx, sy := g.cam.ToScreen(c.X, c.Y)
			age := v.Time - c.Seen
			a := uint8(140)
			if age > 120 {
				a = 70
			}
			circle(dst, sx, sy, ut.RangeKm*z, withAlpha(sideColor(en), a), 1.2)
		}
	}
	// Логистика.
	if g.layers["logistics"] {
		for _, b := range v.Buildings {
			bt := g.cat.BuildingByID[b.Type]
			if bt.Supply <= 0 {
				continue
			}
			sx, sy := g.cam.ToScreen(b.X, b.Y)
			d := b.Dir
			if d < 0 {
				d = 1
			}
			drawTextHalo(dst, sim.DirNames[d], sx, sy+10, 12, colAccent, color.Black, 1)
			circle(dst, sx, sy, 9, colAccent, 2)
		}
	}
	// Укрепления.
	if z > 0.9 {
		for i, f := range v.Fort {
			if f == 0 {
				continue
			}
			cx, cy := g.m.TileCenter(i%g.m.W, i/g.m.W)
			sx, sy := g.cam.ToScreen(cx, cy)
			if !g.visibleOnScreen(sx, sy) {
				continue
			}
			s := g.m.TileKm * z * 0.3
			for k := 0; k < int(f); k++ {
				line(dst, sx-s, sy-s+float64(k)*4, sx+s, sy-s+float64(k)*4, color.RGBA{110, 80, 40, 220}, 1.5)
			}
		}
		for _, i := range v.FortJobs {
			cx, cy := g.m.TileCenter(i%g.m.W, i/g.m.W)
			sx, sy := g.cam.ToScreen(cx, cy)
			s := g.m.TileKm * z * 0.4
			strokeRect(dst, sx-s, sy-s, 2*s, 2*s, color.RGBA{150, 110, 50, 200}, 1)
		}
	}
	// Города.
	for ci, c := range g.m.Cities {
		need := 1000000
		switch {
		case z > 3:
			need = 0
		case z > 1.6:
			need = 80000
		case z > 0.9:
			need = 200000
		case z > 0.55:
			need = 400000
		}
		if c.Pop < need {
			continue
		}
		sx, sy := g.cam.ToScreen(c.X, c.Y)
		if !g.visibleOnScreen(sx, sy) {
			continue
		}
		r := 2.0 + math.Min(4, float64(c.Pop)/400000)
		disc(dst, sx, sy, r, colMapText)
		size := 12.0
		if c.Pop > 500000 {
			size = 14
		}
		drawTextHalo(dst, c.Name, sx+r+3, sy-size*0.65, size, colMapText, color.RGBA{240, 236, 222, 255}, 0)
		consider("city", 0, ci, c.X, c.Y, sx, sy)
	}
	// Контакты (разведданные о противнике).
	for _, c := range v.Contacts {
		sx, sy := g.cam.ToScreen(c.X, c.Y)
		if !g.visibleOnScreen(sx, sy) {
			continue
		}
		age := v.Time - c.Seen
		a := uint8(255)
		if c.Seen < 0 || age > 360 {
			a = 140
		} else if age > 60 {
			a = 200
		}
		col := withAlpha(sideColor(en), a)
		label := "?"
		if c.Kind == 0 {
			if c.Type != "" {
				label = g.bShort(c.Type)
			}
			s := math.Max(6, math.Min(13, z*4))
			strokeRect(dst, sx-s/2, sy-s/2, s, s, col, 2)
			if c.Decoy {
				line(dst, sx-s/2, sy-s/2, sx+s/2, sy+s/2, col, 1.5)
				label = "макет"
			}
			if c.HP >= 0 && c.HP < 0.99 {
				fillRect(dst, sx-s/2, sy+s/2+1, s*c.HP, 2, colBad)
			}
		} else {
			if ut := g.cat.UnitByID[c.Type]; ut != nil {
				label = ut.Short
			} else if c.Class != "" {
				label = "техн."
			}
			s := math.Max(6, math.Min(10, z*3))
			sx, sy = math.Round(sx), math.Round(sy)
			diamond(dst, sx, sy, s, col)
			diamond(dst, sx, sy, s-2.5, color.RGBA{245, 240, 225, a})
		}
		if z > 0.7 || (hov != nil && hov.id == c.ID) {
			g.mapLabel(dst, label, sx, sy-20, col, hov != nil && hov.id == c.ID)
		}
		consider("contact", c.ID, 0, c.X, c.Y, sx, sy)
	}
	// Свои здания.
	for _, b := range v.Buildings {
		sx, sy := g.cam.ToScreen(b.X, b.Y)
		if !g.visibleOnScreen(sx, sy) {
			continue
		}
		bt := g.cat.BuildingByID[b.Type]
		s := math.Max(6, math.Min(14, z*4))
		col := sideColor(my2)
		if b.Built < 1 {
			strokeRect(dst, sx-s/2, sy-s/2, s, s, col, 1.5)
			fillRect(dst, sx-s/2, sy+s/2-s*b.Built, s, s*b.Built, withAlpha(col, 150))
		} else {
			c := col
			if b.HP <= b.MaxHP*0.1 {
				c = color.RGBA{60, 60, 60, 255}
			}
			fillRect(dst, sx-s/2, sy-s/2, s, s, c)
			if b.Masked {
				strokeRect(dst, sx-s/2-2, sy-s/2-2, s+4, s+4, color.RGBA{60, 120, 50, 255}, 1.5)
			}
			if bt.Decoy {
				line(dst, sx-s/2, sy-s/2, sx+s/2, sy+s/2, color.White, 1)
			}
		}
		if f := b.HP / b.MaxHP; f < 0.99 && b.Built >= 1 {
			fillRect(dst, sx-s/2, sy+s/2+1, s, 3, color.RGBA{40, 40, 40, 200})
			fillRect(dst, sx-s/2, sy+s/2+1, s*f, 3, colorForFrac(f))
		}
		if z > 0.9 || (hov != nil && hov.id == b.ID) {
			g.mapLabel(dst, bt.Short, sx, sy-s/2-15, col, (hov != nil && hov.id == b.ID) || g.sel.ID == b.ID)
		}
		if g.sel.Kind == "building" && g.sel.ID == b.ID {
			strokeRect(dst, sx-s/2-4, sy-s/2-4, s+8, s+8, colAccent, 2)
		}
		consider("building", b.ID, 0, b.X, b.Y, sx, sy)
	}
	// Свои юниты.
	for _, un := range v.Units {
		sx, sy := g.cam.ToScreen(un.X, un.Y)
		if !g.visibleOnScreen(sx, sy) {
			continue
		}
		ut := g.cat.UnitByID[un.Type]
		col := sideColor(my2)
		w, h := math.Max(10, math.Min(22, z*6)), math.Max(7, math.Min(14, z*4))
		fillRect(dst, sx-w/2, sy-h/2, w, h, color.RGBA{245, 240, 225, 255})
		strokeRect(dst, sx-w/2, sy-h/2, w, h, col, 2)
		if un.State != sim.UnitDeployed {
			line(dst, sx-w/2, sy+h/2, sx+w/2, sy-h/2, col, 1.5)
		}
		if z > 0.8 || (hov != nil && hov.id == un.ID) {
			g.mapLabel(dst, ut.Short, sx, sy-h/2-15, col, (hov != nil && hov.id == un.ID) || g.sel.ID == un.ID)
		}
		if ut.Kind == "ad" && ut.Magazine > 0 && un.Ready < float64(ut.Magazine)-0.01 {
			f := un.Ready / float64(ut.Magazine)
			fillRect(dst, sx-w/2, sy+h/2+2, w, 3, color.RGBA{40, 40, 40, 220})
			c := colWarn
			if un.Ready < 1 {
				c = colBad
			}
			fillRect(dst, sx-w/2, sy+h/2+2, w*f, 3, c)
		}
		if len(un.Path) > 0 {
			px, py := sx, sy
			for _, p := range un.Path {
				qx, qy := g.cam.ToScreen(p.X, p.Y)
				line(dst, px, py, qx, qy, withAlpha(col, 140), 1)
				px, py = qx, qy
			}
		}
		if g.sel.Kind == "unit" && g.sel.ID == un.ID {
			strokeRect(dst, sx-w/2-4, sy-h/2-4, w+8, h+8, colAccent, 2)
		}
		consider("unit", un.ID, 0, un.X, un.Y, sx, sy)
	}
	// Главный удар.
	if v.HasMain {
		sx, sy := g.cam.ToScreen(v.MainX, v.MainY)
		circle(dst, sx, sy, g.cat.Rules.MainEffortKm*z, color.RGBA{200, 120, 20, 200}, 2)
		drawTextHalo(dst, "ГЛАВНЫЙ УДАР", sx, sy-8, 13, color.RGBA{180, 90, 10, 255}, color.White, 1)
	}
	// Летящие боеприпасы.
	for _, p := range v.Projs {
		sx, sy := g.cam.ToScreen(p.X, p.Y)
		if p.Side == my2 && len(p.Path) > 0 && p.Engaged >= 0 {
			px, py := sx, sy
			for _, q := range p.Path {
				qx, qy := g.cam.ToScreen(q.X, q.Y)
				line(dst, px, py, qx, qy, color.RGBA{60, 60, 60, 60}, 1)
				px, py = qx, qy
			}
		}
		if p.Engaged < 0 {
			continue // ещё не стартовал
		}
		col := sideColor(p.Side)
		s := 6.0
		h := p.Heading
		x1, y1 := sx+math.Cos(h)*s, sy+math.Sin(h)*s
		x2, y2 := sx+math.Cos(h+2.5)*s*0.8, sy+math.Sin(h+2.5)*s*0.8
		x3, y3 := sx+math.Cos(h-2.5)*s*0.8, sy+math.Sin(h-2.5)*s*0.8
		triangle(dst, x1, y1, x2, y2, x3, y3, col)
		if p.Engaged > 0 {
			circle(dst, sx, sy, 8, colWarn, 1)
		}
		consider("proj", p.ID, 0, p.X, p.Y, sx, sy)
	}
	// Спутники.
	if g.layers["sats"] {
		for i, s := range v.Sats {
			p := s.Pass
			band := withAlpha(sideColor(s.Side), 30)
			col := withAlpha(sideColor(s.Side), 160)
			x0, y0 := g.cam.ToScreen(p.X0, p.Y0)
			x1, y1 := g.cam.ToScreen(p.X0+p.DX*p.L, p.Y0+p.DY*p.L)
			line(dst, x0, y0, x1, y1, band, float32(math.Max(1, p.Swath*z)))
			line(dst, x0, y0, x1, y1, col, 1)
			lbl := s.Name
			if p.Active {
				sx, sy := g.cam.ToScreen(p.X, p.Y)
				disc(dst, sx, sy, 6, sideColor(s.Side))
				lbl += " — съёмка"
			} else {
				lbl += " — через " + fmtMin(p.Start-v.Time)
			}
			// подпись там, где трасса входит в видимую часть карты
			ly := float64(g.cam.Y) + 40 + float64(i%4)*15
			t := (ly - y0) / (y1 - y0)
			lx := x0 + (x1-x0)*t
			drawTextHalo(dst, lbl, lx+6, ly, 11, sideColor(s.Side), color.White, 0)
		}
	}
	// План удара.
	if g.mode == modeStrike {
		g.drawStrikePlan(dst)
	}
	// Призрак строительства.
	if g.mode == modeBuild && !u.in.overUI {
		wx, wy := g.cam.ToWorld(mx, my)
		_ = wx
		_ = wy
		s := math.Max(8, math.Min(14, z*4))
		fillRect(dst, mx-s/2, my-s/2, s, s, withAlpha(sideColor(my2), 140))
		drawTextHalo(dst, g.bName(g.buildType), mx+12, my-8, 13, colMapText, color.White, 0)
	}
	if g.mode == modeFort {
		drawTextHalo(dst, "Укрепления: зажмите ЛКМ и ведите по своим тайлам. ПКМ/Esc — выход", mx+14, my, 12, colMapText, color.White, 0)
	}
	if g.mode == modeMain {
		drawTextHalo(dst, "Укажите направление главного удара", mx+14, my, 13, colMapText, color.White, 0)
	}
	return hov
}

func colorForFrac(f float64) color.RGBA {
	switch {
	case f > 0.7:
		return colGood
	case f > 0.35:
		return colWarn
	}
	return colBad
}

func fmtMin(m float64) string {
	if m < 60 {
		return fmt.Sprintf("%.0f мин", m)
	}
	return fmt.Sprintf("%.1f ч", m/60)
}

// mapInput — клики по карте.
func (g *Game) mapInput(hov *hoverItem) {
	u := &g.ui
	in := &u.in
	overMap := u.mouseIn(g.cam.X, g.cam.Y, g.cam.W, g.cam.H) && !in.overUI
	if overMap && in.wheel != 0 {
		g.zoomAt(math.Pow(1.15, in.wheel), float64(in.mx), float64(in.my))
	}
	// Подсказка.
	if overMap && hov != nil && !g.dragging {
		u.tip = g.hoverText(hov)
	}
	// Перетаскивание карты.
	if g.dragging {
		if in.down || in.mdown {
			g.cam.CX = g.dragCX - float64(in.mx-g.dragX)/g.cam.Z
			g.cam.CY = g.dragCY - float64(in.my-g.dragY)/g.cam.Z
			return
		}
		g.dragging = false
		return
	}
	if in.mdown && overMap {
		g.dragging, g.dragX, g.dragY, g.dragCX, g.dragCY = true, in.mx, in.my, g.cam.CX, g.cam.CY
		return
	}
	wx, wy := g.cam.ToWorld(float64(in.mx), float64(in.my))
	switch g.mode {
	case modeFort:
		if in.rclick {
			g.mode = modeNone
			return
		}
		if in.down && overMap {
			if g.fortPainted == nil {
				g.fortPainted = map[int]bool{}
			}
			tx, ty := g.m.TileAt(wx, wy)
			if g.m.In(tx, ty) {
				g.fortPainted[g.m.Idx(tx, ty)] = true
			}
			in.consumed = true
			return
		}
		if !in.down && len(g.fortPainted) > 0 {
			var pts []sim.Pt
			for i := range g.fortPainted {
				cx, cy := g.m.TileCenter(i%g.m.W, i/g.m.W)
				pts = append(pts, sim.Pt{X: cx, Y: cy})
			}
			g.sess.Send(sim.Command{Kind: sim.CmdFort, Pts: pts})
			g.fortPainted = nil
		}
		return
	}
	if !overMap || in.consumed {
		return
	}
	if in.click {
		switch g.mode {
		case modeBuild:
			g.sess.Send(sim.Command{Kind: sim.CmdBuild, Item: g.buildType, X: wx, Y: wy, Text: g.mimic})
			if !ebiten.IsKeyPressed(ebiten.KeyShift) {
				g.mode = modeNone
			}
			return
		case modeMain:
			g.sess.Send(sim.Command{Kind: sim.CmdMainEffort, Int: 1, X: wx, Y: wy})
			g.mode = modeNone
			return
		case modeStrike:
			p := sim.Pt{X: wx, Y: wy}
			if hov != nil && (hov.kind == "contact" || hov.kind == "city") {
				p = sim.Pt{X: hov.x, Y: hov.y}
			}
			g.strike.Pts = append(g.strike.Pts, p)
			return
		}
		if hov != nil {
			g.sel = Selection{Kind: hov.kind, ID: hov.id, Idx: hov.idx}
		} else {
			// Начинаем перетаскивание пустой карты.
			g.dragging, g.dragX, g.dragY, g.dragCX, g.dragCY = true, in.mx, in.my, g.cam.CX, g.cam.CY
			g.sel = Selection{}
		}
		return
	}
	if in.rclick {
		switch g.mode {
		case modeStrike:
			if n := len(g.strike.Pts); n > 0 {
				g.strike.Pts = g.strike.Pts[:n-1]
			}
			return
		case modeBuild, modeMain:
			g.mode = modeNone
			return
		}
		if g.sel.Kind == "unit" {
			g.sess.Send(sim.Command{Kind: sim.CmdMove, ID: g.sel.ID, X: wx, Y: wy})
		}
	}
}

func (g *Game) hoverText(h *hoverItem) string {
	v := g.view
	switch h.kind {
	case "city":
		c := g.m.Cities[h.idx]
		return fmt.Sprintf("%s — %d тыс. жителей", c.Name, c.Pop/1000)
	case "building":
		if b := g.findBuilding(h.id); b != nil {
			return fmt.Sprintf("%s\nСостояние %.0f%%", b.Name, b.HP/b.MaxHP*100)
		}
	case "unit":
		if un := g.findUnit(h.id); un != nil {
			return fmt.Sprintf("%s — %s", g.uName(un.Type), sim.UnitStateNames[un.State])
		}
	case "contact":
		if c := g.findContact(h.id); c != nil {
			return g.contactTitle(c) + "\n" + ageText(v.Time, c.Seen) + " · " + c.Source
		}
	case "proj":
		for _, p := range v.Projs {
			if p.ID == h.id {
				m := g.cat.MunitionByID[p.Munition]
				s := m.Name
				if p.Side != v.Side {
					s = "Цель: " + s
				}
				return s
			}
		}
	}
	return ""
}

func (g *Game) contactTitle(c *sim.Contact) string {
	name := "Неопознанный объект"
	if c.Kind == 1 {
		name = "Неопознанная техника"
	}
	if c.Type != "" {
		if c.Kind == 0 {
			name = g.bName(c.Type)
		} else {
			name = g.uName(c.Type)
		}
	} else if c.Class != "" {
		name = capFirst(c.Class)
	}
	if c.Decoy {
		name = "Макет (" + name + ")"
	}
	return name
}

func (g *Game) findBuilding(id uint32) *sim.Building {
	for i := range g.view.Buildings {
		if g.view.Buildings[i].ID == id {
			return &g.view.Buildings[i]
		}
	}
	return nil
}

func (g *Game) findUnit(id uint32) *sim.Unit {
	for i := range g.view.Units {
		if g.view.Units[i].ID == id {
			return &g.view.Units[i]
		}
	}
	return nil
}

func (g *Game) findContact(id uint32) *sim.Contact {
	for i := range g.view.Contacts {
		if g.view.Contacts[i].ID == id {
			return &g.view.Contacts[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Верхняя панель.

func fmtNum(v float64) string {
	a := math.Abs(v)
	switch {
	case a >= 100000:
		return fmt.Sprintf("%.0fk", v/1000)
	case a >= 10000:
		return fmt.Sprintf("%.1fk", v/1000)
	}
	return fmt.Sprintf("%.0f", v)
}

func fmtRate(v float64) string {
	if v >= 0 {
		return "+" + fmtNum(v)
	}
	return fmtNum(v)
}

func (g *Game) drawTopBar() {
	u := &g.ui
	v := g.view
	fillRect(u.screen, 0, 0, float64(u.W), topH, colPanel)
	line(u.screen, 0, topH, float64(u.W), topH, colBorder, 1)
	u.blockUI(0, 0, u.W, topH)
	x := 10.0
	drawBold(u.screen, data.SideNames[v.Side], x, 6, 16, sideColor(v.Side), 0)
	if v.Sandbox {
		if u.Button(int(x), 30, 110, 20, "Сменить сторону") {
			// Новое представление придёт со следующим кадром.
			g.sess.SetSide(1 - v.Side)
			g.sel = Selection{}
			g.mode = modeNone
			g.evSeen = 0
			g.rend.lastFog = nil
		}
	}
	x = 130
	for i := 0; i < data.NumRes; i++ {
		rc := colGood
		if v.Rates[i] < -0.05 {
			rc = colBad
		}
		drawText(u.screen, data.ResNames[i], x, 6, 12, colDim, 0)
		drawBold(u.screen, fmtNum(v.Res[i]), x, 22, 16, colText, 0)
		drawText(u.screen, fmtRate(v.Rates[i])+"/ч", x+textWidth(fmtNum(v.Res[i]), 16)+6, 26, 12, rc, 0)
		tip := fmt.Sprintf("%s: %.0f, изменение %.1f в час", data.ResNames[i], v.Res[i], v.Rates[i])
		if i == data.ResMoney {
			tip += fmt.Sprintf("\nДоход (налоги, экспорт): %.0f в час", v.Income)
		}
		u.Tooltip(int(x), 0, 120, topH, tip)
		x += 128
	}
	// Мораль, люди, энергия.
	mc := colorForFrac(v.Morale / 100)
	drawText(u.screen, "Мораль", x, 6, 12, colDim, 0)
	drawBold(u.screen, fmt.Sprintf("%.0f", v.Morale), x, 22, 16, mc, 0)
	u.Tooltip(int(x), 0, 70, topH, "Воля к сопротивлению. При нуле — поражение. Падает от потерь, блэкаутов, потери городов и мобилизации.")
	x += 72
	drawText(u.screen, "Резерв", x, 6, 12, colDim, 0)
	drawBold(u.screen, fmt.Sprintf("%.0fk", v.People), x, 22, 16, colText, 0)
	u.Tooltip(int(x), 0, 70, topH, fmt.Sprintf("Мобилизационный резерв, тыс. человек. Потеря рабочей силы: %.0f%%", v.LaborLoss*100))
	x += 72
	ec := colGood
	if v.Blackout > 0.05 {
		ec = colBad
	}
	drawText(u.screen, "Энергия, МВт", x, 6, 12, colDim, 0)
	drawBold(u.screen, fmt.Sprintf("%.0f/%.0f", v.Power[0], v.Power[1]), x, 22, 15, ec, 0)
	u.Tooltip(int(x), 0, 110, topH, fmt.Sprintf("Генерация / потребление. Без света: %.0f%% населения", v.Blackout*100))
	x += 118

	// Время и скорость (справа).
	rx := float64(u.W) - 10
	if g.sess.IsHost() {
		if u.Button(int(rx)-70, 6, 70, 20, "Сохранить") {
			g.quickSave()
		}
	}
	if u.Button(int(rx)-70, 30, 70, 20, "Меню") {
		g.leaveGame()
		return
	}
	rx -= 80
	for k := 5; k >= 1; k-- {
		lbl := strings.Repeat(">", min(k, 3))
		if k > 3 {
			lbl = fmt.Sprintf("×%d", int(sim.SpeedMult[k]))
		}
		if u.ButtonState(int(rx)-30, 30, 30, 20, lbl, v.MySpeed == k, true) {
			g.sess.Send(sim.Command{Kind: sim.CmdSpeed, Int: k})
		}
		u.Tooltip(int(rx)-30, 30, 30, 20, fmt.Sprintf("Скорость %d (×%.0f). Действует меньшая из выбранных игроками. Клавиши 1–5.", k, sim.SpeedMult[k]))
		rx -= 32
	}
	pl := "Пауза"
	if v.Pausing {
		pl = "Продолжить"
	}
	if u.ButtonState(int(rx)-84, 30, 84, 20, pl, v.Pausing, true) {
		p := 1
		if v.Pausing {
			p = 0
		}
		g.sess.Send(sim.Command{Kind: sim.CmdPause, Int: p})
	}
	if !v.Sandbox {
		u.Tooltip(int(rx)-84, 30, 84, 20, fmt.Sprintf("Пауза может поставить любой игрок. Осталось: %.0f с. Пробел.", v.PauseLeft))
	}
	status := sim.FmtTime(v.Time)
	if !v.War {
		status += fmt.Sprintf(" · подготовка: до войны %s", fmtMin(v.PrepEnd-v.Time))
	}
	if v.Paused {
		status += " · ПАУЗА"
	}
	drawText(u.screen, status, float64(u.W)-90, 8, 14, colText, 2)
	if !v.Sandbox {
		drawText(u.screen, fmt.Sprintf("скорость: %d (вы %d, противник %d)", v.Speed, v.MySpeed, v.EnemySpeed), rx-92, 33, 11, colDim, 2)
	}
}

func (g *Game) drawLayerButtons() {
	u := &g.ui
	layers := []struct{ key, name, tip string }{
		{"fog", "Туман", "Затемнение неразведанных районов"},
		{"ad", "ПВО", "Зоны поражения своих ПВО, РЭБ и известных ПВО противника"},
		{"sats", "Спутники", "Трассы ближайших пролётов спутников обеих сторон"},
		{"logistics", "Логистика", "Логистические узлы и их направления"},
		{"energy", "Энергия", "Энергобаланс своих областей"},
		{"deposits", "Ресурсы", "Месторождения: нефть (тёмные), газ (голубые), уголь (коричневые), руда (рыжие)"},
	}
	x := u.W - infoW - 10 - len(layers)*84
	y := topH + 8
	for _, l := range layers {
		if u.ButtonState(x, y, 80, 22, l.name, g.layers[l.key], true) {
			g.layers[l.key] = !g.layers[l.key]
		}
		u.Tooltip(x, y, 80, 22, l.tip)
		u.blockUI(x, y, 80, 22)
		x += 84
	}
}

func (g *Game) drawToasts() {
	u := &g.ui
	y := float64(u.H - 40)
	now := time.Now()
	for i := len(g.toasts) - 1; i >= 0; i-- {
		t := g.toasts[i]
		age := now.Sub(t.at).Seconds()
		if age > 7 {
			continue
		}
		w := textWidth(t.text, 14) + 20
		x := float64(leftW) + 12
		fillRect(u.screen, x, y, w, 26, color.RGBA{20, 22, 26, 220})
		strokeRect(u.screen, x, y, w, 26, colAccent, 1)
		drawText(u.screen, t.text, x+10, y+5, 14, colText, 0)
		y -= 30
	}
}

func (g *Game) drawVictory() {
	u := &g.ui
	v := g.view
	w, h := 560.0, 200.0
	x, y := float64(u.W)/2-w/2, float64(u.H)/2-h/2
	fillRect(u.screen, x, y, w, h, color.RGBA{16, 18, 22, 245})
	strokeRect(u.screen, x, y, w, h, colAccent, 2)
	title := "ПОРАЖЕНИЕ"
	c := colBad
	if v.Winner == v.Side {
		title, c = "ПОБЕДА", colGood
	}
	drawBold(u.screen, title, x+w/2, y+30, 34, c, 1)
	for i, l := range wrap(v.WinReason, 16, w-40) {
		drawText(u.screen, l, x+w/2, y+90+float64(i)*22, 16, colText, 1)
	}
	if u.Button(int(x+w/2-90), int(y+h-50), 180, 34, "В главное меню") {
		g.leaveGame()
	}
	u.blockUI(int(x), int(y), int(w), int(h))
}

func capFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// diamond — ромб (значок техники).
func diamond(dst *ebiten.Image, x, y, s float64, c color.Color) {
	triangle(dst, x, y-s, x+s, y, x, y+s, c)
	triangle(dst, x, y-s, x-s, y, x, y+s, c)
}

// mapLabel — подпись на карте без наложения на уже нарисованные.
func (g *Game) mapLabel(dst *ebiten.Image, text string, x, y float64, col color.RGBA, force bool) {
	w := textWidth(text, 11)
	r := image.Rect(int(x-w/2)-1, int(y), int(x+w/2)+1, int(y+13))
	if !force {
		for _, o := range g.labels {
			if r.Overlaps(o) {
				return
			}
		}
	}
	g.labels = append(g.labels, r)
	drawTextHalo(dst, text, x, y, 11, col, color.RGBA{240, 236, 222, 210}, 1)
}
