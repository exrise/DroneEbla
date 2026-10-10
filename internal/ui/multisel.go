package ui

import (
	"fmt"
	"image/color"
	"math"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/exrise/droneebla/internal/sim"
)

// Групповое выделение юнитов: Shift+щелчок добавляет или убирает юнит, Shift+рамка выделяет юниты в прямоугольнике,
// клавиши групп 1…9 выделяют всех живых членов группы (камера не двигается). ПКМ — марш всех выбранных.

// selRect — рамка выделения в экранных координатах.
type selRect struct{ x0, y0, x1, y1 int }

func shiftHeld() bool { return ebiten.IsKeyPressed(ebiten.KeyShift) }

// multiIDs — выбранные юниты по возрастанию ID (пусто, если выбран один объект или ничего).
func (g *Game) multiIDs() []uint32 {
	if len(g.multi) < 2 {
		return nil
	}
	ids := make([]uint32, 0, len(g.multi))
	for id := range g.multi {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return ids
}

// pruneMulti убирает погибших и отдавших юнитов; одиночный остаток становится обычным выбором.
func (g *Game) pruneMulti() {
	if len(g.multi) == 0 || g.view == nil {
		return
	}
	for id := range g.multi {
		if g.findUnit(id) == nil {
			delete(g.multi, id)
		}
	}
	switch len(g.multi) {
	case 0:
		g.multi = nil
	case 1:
		for id := range g.multi {
			g.sel = Selection{Kind: "unit", ID: id}
		}
		g.multi = nil
	}
}

// setMulti выделяет набор юнитов (меньше двух — обычный выбор).
func (g *Game) setMulti(ids []uint32) {
	g.multi = nil
	switch len(ids) {
	case 0:
		g.sel = Selection{}
	case 1:
		g.sel = Selection{Kind: "unit", ID: ids[0]}
	default:
		g.multi = map[uint32]bool{}
		for _, id := range ids {
			g.multi[id] = true
		}
		g.sel = Selection{Kind: "unit", ID: ids[0]}
	}
}

// toggleMulti добавляет юнит в выбор или убирает из него (Shift+щелчок).
func (g *Game) toggleMulti(id uint32) {
	cur := g.multiIDs()
	if cur == nil && g.sel.Kind == "unit" {
		cur = []uint32{g.sel.ID}
	}
	for i, c := range cur {
		if c == id {
			cur = append(cur[:i], cur[i+1:]...)
			g.setMulti(cur)
			return
		}
	}
	g.setMulti(append(cur, id))
}

// rectSelect выделяет свои юниты внутри рамки.
func (g *Game) rectSelect(r selRect) {
	x0, x1 := min(r.x0, r.x1), max(r.x0, r.x1)
	y0, y1 := min(r.y0, r.y1), max(r.y0, r.y1)
	if x1-x0 < 6 && y1-y0 < 6 {
		return
	}
	var ids []uint32
	for _, un := range g.view.Units {
		ux, uy := g.smooth.get(false, un.ID, un.X, un.Y, g.smooth.at)
		sx, sy := g.cam.ToScreen(ux, uy)
		if int(sx) >= x0 && int(sx) <= x1 && int(sy) >= y0 && int(sy) <= y1 {
			ids = append(ids, un.ID)
		}
	}
	if len(ids) == 0 {
		g.toast("В рамке нет ваших юнитов")
		return
	}
	g.setMulti(ids)
}

// moveMulti отправляет всех выбранных к точке: цели разносятся по спирали, чтобы юниты не встали друг на друга.
func (g *Game) moveMulti(ids []uint32, wx, wy float64) {
	for k, id := range ids {
		dx, dy := 0.0, 0.0
		if k > 0 {
			ring := 1 + (k-1)/6
			ang := float64((k-1)%6)*math.Pi/3 + float64(ring)*0.5
			dx, dy = math.Cos(ang)*3*float64(ring), math.Sin(ang)*3*float64(ring)
		}
		g.sess.Send(sim.Command{Kind: sim.CmdMove, ID: id, X: wx + dx, Y: wy + dy})
	}
}

// drawSelRect рисует рамку выделения.
func (g *Game) drawSelRect() {
	r := g.rect
	if r == nil {
		return
	}
	u := &g.ui
	x0, y0, x1, y1 := float64(min(r.x0, r.x1)), float64(min(r.y0, r.y1)), float64(max(r.x0, r.x1)), float64(max(r.y0, r.y1))
	fillRect(u.screen, x0, y0, x1-x0, y1-y0, color.RGBA{232, 182, 61, 40})
	strokeRect(u.screen, x0, y0, x1-x0, y1-y0, colAccent, 1.5)
}

// multiInfo — карточка группы выбранных юнитов.
func (g *Game) multiInfo(ids []uint32, x, y, w int) {
	u := &g.ui
	v := g.view
	drawBold(u.screen, fmt.Sprintf("Выбрано юнитов: %d", len(ids)), float64(x), float64(y), 16, sideText(v.Side), 0)
	y += 26
	count := map[string]int{}
	var ads []uint32
	adMode := -1
	var order []string
	for _, id := range ids {
		un := g.findUnit(id)
		if un == nil {
			continue
		}
		if count[un.Type] == 0 {
			order = append(order, un.Type)
		}
		count[un.Type]++
		if g.cat.UnitByID[un.Type].Kind == "ad" {
			ads = append(ads, id)
			if adMode < 0 {
				adMode = un.Fire
			} else if adMode != un.Fire {
				adMode = -2 // режимы разные
			}
		}
	}
	for _, t := range order {
		y = g.kv(g.uName(t), fmt.Sprintf("×%d", count[t]), x, y, w, colText)
	}
	y += 4
	y = g.para("ПКМ по карте — марш всех выбранных. Shift+щелчок — добавить или убрать юнит, Shift+рамка — выделить областью. Ctrl+1…9 — запомнить в группу.", x, y, w, colDim)
	if len(ads) > 0 {
		y += 4
		drawText(u.screen, "Режим огня ПВО:", float64(x), float64(y), 13, colDim, 0)
		y += 20
		y = g.fireModeButtons(ads, adMode, x, y, w)
	}
	y += 4
	if u.Button(x, y, w, 26, "Снять выбор") {
		g.multi = nil
		g.sel = Selection{}
	}
}
