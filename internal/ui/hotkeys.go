package ui

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/exrise/droneebla/internal/sim"
)

// Горячие клавиши (необязательные, всё доступно мышью):
//   - Ctrl+1…9 — запомнить выбранный объект в группу (Ctrl+Shift+1…9 — добавить);
//   - 1…9 — выбрать следующий объект группы и перейти к нему;
//   - F — режим пуска для выбранной пусковой, повторно — пуск;
//   - [ и ] (или цифры numpad 1–5) — скорость игры;
//   - F11 или Alt+Enter — полный экран.

// speedKeys переключает скорость игры.
func (g *Game) helpKey() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.helpOpen, g.helpPinned = !g.helpOpen, true
	}
}

func (g *Game) speedKeys() {
	if g.view == nil || g.view.TimeLocked {
		return
	}
	cur := g.view.MySpeed
	for k := 1; k <= 5; k++ {
		if inpututil.IsKeyJustPressed(ebiten.KeyNumpad0 + ebiten.Key(k)) {
			g.sess.Send(sim.Command{Kind: sim.CmdSpeed, Int: k})
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) && cur > 1 {
		g.sess.Send(sim.Command{Kind: sim.CmdSpeed, Int: cur - 1})
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) && cur < 5 {
		g.sess.Send(sim.Command{Kind: sim.CmdSpeed, Int: cur + 1})
	}
}

// groupKeys — контрольные группы 1…9.
func (g *Game) groupKeys() {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	for n := 1; n <= 9; n++ {
		if !inpututil.IsKeyJustPressed(ebiten.Key0 + ebiten.Key(n)) {
			continue
		}
		if ctrl {
			g.setGroup(n, shift)
		} else {
			g.selectGroup(n)
		}
	}
}

// groupable — можно ли записать объект в группу.
func (g *Game) groupable(s Selection) bool {
	return s.Kind == "unit" || s.Kind == "building"
}

func (g *Game) setGroup(n int, add bool) {
	if !g.groupable(g.sel) {
		g.toast("Выберите свой юнит или здание, чтобы запомнить его в группе")
		return
	}
	grp := g.groups[n-1]
	if !add {
		grp = nil
	}
	for _, m := range grp {
		if m == g.sel {
			g.groups[n-1] = grp
			g.toast(fmt.Sprintf("Группа %d: %d", n, len(grp)))
			return
		}
	}
	g.groups[n-1] = append(grp, g.sel)
	g.toast(fmt.Sprintf("Группа %d: %d", n, len(g.groups[n-1])))
}

// groupMembers — живые члены группы (погибшие отсеиваются).
func (g *Game) groupMembers(n int) []cand {
	var out, keep = []cand(nil), []Selection(nil)
	for _, m := range g.groups[n-1] {
		switch m.Kind {
		case "unit":
			if un := g.findUnit(m.ID); un != nil {
				out = append(out, cand{sel: m, x: un.X, y: un.Y})
				keep = append(keep, m)
			}
		case "building":
			if b := g.findBuilding(m.ID); b != nil {
				out = append(out, cand{sel: m, x: b.X, y: b.Y})
				keep = append(keep, m)
			}
		}
	}
	g.groups[n-1] = keep
	return out
}

func (g *Game) selectGroup(n int) {
	cs := g.groupMembers(n)
	if len(cs) == 0 {
		g.toast(fmt.Sprintf("Группа %d пуста: выберите объект и нажмите Ctrl+%d", n, n))
		return
	}
	g.cycleSelect(fmt.Sprintf("group:%d", n), cs)
}

// strikeHotkey — F: открыть режим пуска для выбранной пусковой или подтвердить удар.
func (g *Game) strikeHotkey() {
	v := g.view
	if v == nil {
		return
	}
	if g.mode == modeStrike {
		g.confirmStrike()
		return
	}
	if g.sel.Kind != "unit" && g.sel.Kind != "building" {
		return
	}
	if !v.War {
		g.toast("Удары доступны после начала войны")
		return
	}
	for _, id := range g.launchOptions(g.sel.ID) {
		if v.Stocks[id] >= 1 {
			g.mode = modeStrike
			g.strike = strikePlan{Source: g.sel.ID, Munition: id, Count: 1}
			return
		}
	}
	g.toast("У выбранного объекта нет боеприпасов для пуска")
}
