package ui

import (
	"fmt"
	"image/color"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/netplay"
)

// openLobby открывает лобби сетевой игры для хоста или клиента.
func (g *Game) openLobby(s netplay.Session) {
	g.sess = s
	g.scene = sceneLobby
	g.view = nil
	g.menuErr = ""
}

// leaveLobby закрывает соединение и возвращает в меню.
func (g *Game) leaveLobby() {
	if g.sess != nil {
		g.sess.Close()
	}
	g.sess = nil
	g.scene = sceneMenu
}

// drawLobby — лобби: два столбца по три слота, выбор стороны, начало партии.
func (g *Game) drawLobby() {
	u := &g.ui
	cx, y := g.menuFrame("Лобби: до 6 игроков, до 3 за сторону")
	if g.sess == nil {
		g.scene = sceneMenu
		return
	}
	lob := g.sess.Lobby()
	if lob == nil {
		g.leaveLobby()
		return
	}
	me := lob.Me()
	// Клиент входит в игру, как только пришло представление.
	if !g.sess.IsHost() && g.sess.View() != nil {
		g.startGame(g.sess)
		return
	}
	colW := 300
	for side := 0; side < 2; side++ {
		x := cx - colW - 20
		if side == 1 {
			x = cx + 20
		}
		drawBold(u.screen, fmt.Sprintf("%s (%d/%d)", data.SideNames[side], lob.Count(side), lob.Max), float64(x), float64(y), 18, sideColor(side), 0)
		row := 0
		for _, p := range lob.Players {
			if p.Side != side {
				continue
			}
			name := p.Name
			col := colText
			if p.ID == lob.You {
				name += " (вы)"
				col = colAccent
			}
			fillRect(u.screen, float64(x), float64(y+30+row*34), float64(colW), 30, color.RGBA{34, 40, 48, 255})
			drawText(u.screen, name, float64(x+10), float64(y+37+row*34), 16, col, 0)
			row++
		}
		for ; row < lob.Max; row++ {
			fillRect(u.screen, float64(x), float64(y+30+row*34), float64(colW), 30, color.RGBA{26, 30, 36, 255})
			drawText(u.screen, "свободно", float64(x+10), float64(y+37+row*34), 15, colDim, 0)
		}
		can := me.Side != side && lob.Count(side) < lob.Max && (!lob.Started || me.Side < 0) && !(me.Host && lob.Started)
		if u.ButtonState(x, y+30+lob.Max*34+6, colW, 32, "Играть за сторону: "+data.SideNames[side], me.Side == side, can || me.Side == side) && can {
			g.sess.PickSide(side)
		}
	}
	by := y + 30 + lob.Max*34 + 70
	switch {
	case g.sess.IsHost():
		ok := lob.Count(0) > 0 && lob.Count(1) > 0
		msg := "Выберите сторону и дождитесь игроков. Нужен хотя бы один игрок за каждую сторону."
		if ok {
			msg = "Всё готово. Когда все подключились — нажмите «Начать». Скоростью и паузой управляете только вы."
		}
		drawText(u.screen, msg, float64(cx), float64(by-26), 14, colDim, 1)
		if u.ButtonState(cx+10, by, 200, 40, "Начать", false, ok) {
			g.sess.StartGame()
			if l := g.sess.Lobby(); l != nil && l.Started {
				g.startGame(g.sess)
				return
			}
		}
	case lob.Started && me.Side < 0:
		drawText(u.screen, "Партия уже идёт — выберите сторону с свободным местом, чтобы войти.", float64(cx), float64(by-26), 14, colWarn, 1)
	case me.Side < 0:
		drawText(u.screen, "Выберите сторону. Затем хост начнёт партию.", float64(cx), float64(by-26), 14, colDim, 1)
	default:
		drawText(u.screen, "Вы в игре. Ждём, пока хост начнёт партию…", float64(cx), float64(by-26), 14, colDim, 1)
	}
	if u.Button(cx-210, by, 200, 40, "Выйти") {
		g.leaveLobby()
	}
	for _, m := range g.sess.Messages() {
		g.menuErr = m
	}
	if g.menuErr != "" {
		drawText(u.screen, g.menuErr, float64(cx), float64(by+56), 15, colWarn, 1)
	}
}
