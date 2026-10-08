package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Settings — настройки экрана; лежат в settings.json рядом с игрой.
type Settings struct {
	WinW       int     `json:"win_w"`
	WinH       int     `json:"win_h"`
	Fullscreen bool    `json:"fullscreen"`
	Scale      float64 `json:"scale"`   // масштаб интерфейса; 0 — автоматически по размеру окна
	Quality    float64 `json:"quality"` // качество отрисовки (доля физического разрешения); 0 — авто
	Glass      bool    `json:"glass"`
}

const (
	minWinW, minWinH = 1280, 720 // меньше окно не делаем
	// Интерфейс рассчитан минимум на такой логический экран (верхней строке нужно 1440 по ширине):
	// при больших масштабах он не помещается.
	minLogicalW, minLogicalH = 1440, 720
)

// scaleSteps — доступные масштабы интерфейса (0 — «Авто»).
var scaleSteps = []float64{0, 0.75, 1, 1.25, 1.5, 1.75, 2}

// qualitySteps — качество отрисовки (0 — «Авто»: внутренний кадр не больше 2560×1440).
var qualitySteps = []float64{0, 1, 0.75, 0.5}

// winPresets — размеры окна на выбор.
var winPresets = [][2]int{{1280, 720}, {1366, 768}, {1600, 900}, {1920, 1080}, {2560, 1440}, {3840, 2160}}

// DefaultSettings — настройки по умолчанию.
func DefaultSettings() Settings {
	return Settings{WinW: 1600, WinH: 900, Scale: 0, Glass: true}
}

// LoadSettings читает settings.json; при отсутствии или ошибке возвращает значения по умолчанию.
func LoadSettings(path string) Settings {
	s := DefaultSettings()
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &s) != nil {
			s = DefaultSettings()
		}
	}
	s.normalize()
	return s
}

// normalize отбрасывает недопустимые значения.
func (s *Settings) normalize() {
	d := DefaultSettings()
	if s.WinW < minWinW || s.WinH < minWinH || s.WinW > 7680 || s.WinH > 4320 {
		s.WinW, s.WinH = d.WinW, d.WinH
	}
	ok := false
	for _, v := range scaleSteps {
		if s.Scale == v {
			ok = true
		}
	}
	if !ok {
		s.Scale = 0
	}
	ok = false
	for _, v := range qualitySteps {
		if s.Quality == v {
			ok = true
		}
	}
	if !ok {
		s.Quality = 0
	}
}

// renderQuality — доля физического разрешения, в которой рисуется кадр.
func renderQuality(pw, ph, setting float64) float64 {
	if setting > 0 {
		return setting
	}
	return math.Min(1, math.Min(autoMaxW/pw, autoMaxH/ph))
}

// «Авто»: на экранах больше 2560×1440 кадр рисуется не крупнее этого и растягивается.
const autoMaxW, autoMaxH = 2560, 1440

// FitToMonitor уменьшает размер окна, если он не помещается на монитор (размеры в независимых от DPI пикселях).
func (s *Settings) FitToMonitor(mw, mh int) {
	if mw <= 0 || mh <= 0 {
		return
	}
	if s.WinW > mw {
		s.WinW = mw
	}
	if s.WinH > mh {
		s.WinH = mh
	}
	if s.WinW < minWinW || s.WinH < minWinH {
		s.WinW, s.WinH = minWinW, minWinH
	}
}

// windowChoices — размеры окна, которые помещаются на монитор вместе с рамкой и панелью задач.
func windowChoices(mw, mh int) [][2]int {
	var out [][2]int
	for _, p := range winPresets {
		if mw <= 0 || (p[0] <= mw && p[1] <= mh-70) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = append(out, winPresets[0])
	}
	return out
}

// layoutScale — во сколько раз растягивается логический экран на физические пиксели.
// ow, oh — размер окна в независимых пикселях, dsf — масштаб системы (DPI), manual — выбор игрока (0 — авто).
func layoutScale(ow, oh, dsf, manual float64) float64 {
	if dsf < 1 {
		dsf = 1
	}
	pw, ph := ow*dsf, oh*dsf
	if manual <= 0 {
		return uiScale(int(pw), int(ph))
	}
	s := manual * dsf
	// Интерфейс обязан помещаться: больше, чем нужно для минимального логического экрана, не берём.
	if limit := math.Min(pw/minLogicalW, ph/minLogicalH); s > limit {
		s = math.Max(0.5, math.Floor(limit*20+1e-9)/20)
	}
	return s
}

// stepScale — следующий ручной масштаб вверх или вниз от текущего (в долях единицы).
func stepScale(cur float64, up bool) float64 {
	manual := scaleSteps[1:]
	if up {
		for _, s := range manual {
			if s > cur+0.001 {
				return s
			}
		}
		return manual[len(manual)-1]
	}
	for i := len(manual) - 1; i >= 0; i-- {
		if manual[i] < cur-0.001 {
			return manual[i]
		}
	}
	return manual[0]
}

// SetSettings передаёт игре загруженные настройки; окно к этому моменту уже создано с нужным размером.
func (g *Game) SetSettings(s Settings, path string) {
	g.set, g.setPath = s, path
	if os.Getenv("DRONEEBLA_FLAT") == "" {
		g.glass = s.Glass
	}
	g.winSeen = [2]int{s.WinW, s.WinH}
}

func (g *Game) saveSettings() {
	if g.setPath == "" {
		return
	}
	b, err := json.MarshalIndent(g.set, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(g.setPath, b, 0o644)
}

func (g *Game) setFullscreen(on bool) {
	g.set.Fullscreen = on
	ebiten.SetFullscreen(on)
	if !on {
		ebiten.SetWindowSize(g.set.WinW, g.set.WinH)
		g.resetWinWatch()
	}
	g.saveSettings()
}

func (g *Game) setWindowSize(w, h int) {
	g.set.WinW, g.set.WinH = w, h
	if g.set.Fullscreen {
		g.set.Fullscreen = false
		ebiten.SetFullscreen(false)
	}
	ebiten.SetWindowSize(w, h)
	g.resetWinWatch()
	g.saveSettings()
}

func (g *Game) setQuality(q float64) {
	g.set.Quality = q
	g.saveSettings()
}

func (g *Game) setScale(s float64) {
	g.set.Scale = s
	g.saveSettings()
}

func (g *Game) setGlass(on bool) {
	g.glass, g.set.Glass = on, on
	g.saveSettings()
}

func (g *Game) resetWinWatch() {
	g.winSeen = [2]int{-1, -1}
	g.winSeenAt = g.ui.in.tick
}

// settingsKeys — F11 / Alt+Enter, Ctrl+«+» / Ctrl+«−» / Ctrl+0.
func (g *Game) settingsKeys() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) ||
		(inpututil.IsKeyJustPressed(ebiten.KeyEnter) && ebiten.IsKeyPressed(ebiten.KeyAlt)) {
		g.setFullscreen(!ebiten.IsFullscreen())
	}
	if !ctrlHeld() {
		return
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyEqual) || inpututil.IsKeyJustPressed(ebiten.KeyKPAdd):
		g.setScale(stepScale(g.effScale, true))
	case inpututil.IsKeyJustPressed(ebiten.KeyMinus) || inpututil.IsKeyJustPressed(ebiten.KeyKPSubtract):
		g.setScale(stepScale(g.effScale, false))
	case inpututil.IsKeyJustPressed(ebiten.Key0) || inpututil.IsKeyJustPressed(ebiten.KeyKP0):
		g.setScale(0)
	default:
		return
	}
	if g.set.Scale == 0 {
		g.toast("Масштаб интерфейса: авто")
	} else {
		g.toast(fmt.Sprintf("Масштаб интерфейса: %.0f%%", g.set.Scale*100))
	}
}

func ctrlHeld() bool {
	return ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
}

// watchWindow запоминает размер окна после того, как игрок перестал тянуть его мышью.
func (g *Game) watchWindow() {
	if ebiten.IsFullscreen() || ebiten.IsWindowMaximized() || ebiten.IsWindowMinimized() {
		return
	}
	w, h := ebiten.WindowSize()
	if w < 1000 || h < 560 {
		return
	}
	cur := [2]int{w, h}
	if cur != g.winSeen {
		g.winSeen, g.winSeenAt = cur, g.ui.in.tick
		return
	}
	if (w != g.set.WinW || h != g.set.WinH) && g.ui.in.tick-g.winSeenAt > 45 {
		g.set.WinW, g.set.WinH = w, h
		g.saveSettings()
	}
}

// monitorDIP — размер монитора в независимых пикселях (0, 0, если неизвестен).
func monitorDIP() (int, int) {
	if m := ebiten.Monitor(); m != nil {
		return m.Size()
	}
	return 0, 0
}

// drawSettingsBody рисует выбор режима, размера окна и масштаба; возвращает y под последним элементом.
func (g *Game) drawSettingsBody(cx, y int) int {
	u := &g.ui
	left := float64(cx - menuW/2)
	section := func(title string) {
		drawText(u.screen, title, left, float64(y), 14, colDim, 0)
		y += 22
	}

	section("Режим экрана")
	if u.ButtonState(cx-204, y, 200, 36, "Окно", !g.set.Fullscreen, true) {
		g.setFullscreen(false)
	}
	if u.ButtonState(cx+4, y, 200, 36, "Полный экран (F11)", g.set.Fullscreen, true) {
		g.setFullscreen(true)
	}
	y += 52

	section("Размер окна")
	mw, mh := monitorDIP()
	choices := windowChoices(mw, mh)
	cur := [2]int{g.set.WinW, g.set.WinH}
	have := false
	for _, c := range choices {
		have = have || c == cur
	}
	if !have && (mw <= 0 || cur[0] <= mw) {
		choices = append(append([][2]int{}, choices...), cur) // текущий размер (например, подобранный мышью)
	}
	const perRow, bw, gap = 4, 130, 8
	for i := 0; i < len(choices); i += perRow {
		row := choices[i:min(i+perRow, len(choices))]
		x := cx - (len(row)*bw+(len(row)-1)*gap)/2
		for _, c := range row {
			on := !g.set.Fullscreen && g.set.WinW == c[0] && g.set.WinH == c[1]
			if u.ButtonState(x, y, bw, 34, fmt.Sprintf("%d×%d", c[0], c[1]), on, !g.set.Fullscreen) {
				g.setWindowSize(c[0], c[1])
			}
			x += bw + gap
		}
		y += 42
	}
	note := "Размер окна можно менять и мышью — потянув за край."
	if g.set.Fullscreen {
		note = "В полном экране размер окна не используется."
	}
	drawText(u.screen, note, float64(cx), float64(y), 13, colDim, 1)
	y += 30

	section("Масштаб интерфейса")
	const sbw, sgap = 72, 6
	ww, wh := ebiten.WindowSize()
	x := cx - (len(scaleSteps)*sbw+(len(scaleSteps)-1)*sgap)/2
	for _, s := range scaleSteps {
		lbl := "Авто"
		if s > 0 {
			lbl = fmt.Sprintf("%.0f%%", s*100)
		}
		fits := s == 0 || layoutScale(float64(ww), float64(wh), g.dsf, s) >= s*g.dsf-1e-9
		if u.ButtonState(x, y, sbw, 34, lbl, g.set.Scale == s, fits) {
			g.setScale(s)
		}
		x += sbw + sgap
	}
	y += 42
	note = fmt.Sprintf("Сейчас %.0f%%. Ctrl+«+» / Ctrl+«−» — шаг масштаба, Ctrl+0 — авто.", g.effScale*100)
	if g.set.Scale == 0 {
		note = fmt.Sprintf("Авто: подбирается по размеру окна, сейчас %.0f%%. Ctrl+«+» / Ctrl+«−» — вручную.", g.effScale*100)
	}
	c := color.Color(colDim)
	if g.capped {
		note = fmt.Sprintf("Выбрано %.0f%%, но при таком размере окна интерфейс не помещается — показано %.0f%%.", g.set.Scale*100, g.effScale*100)
		c = colWarn
	}
	drawText(u.screen, note, float64(cx), float64(y), 13, c, 1)
	y += 30

	section("Качество отрисовки")
	x = cx - (len(qualitySteps)*sbw+(len(qualitySteps)-1)*sgap)/2
	for _, q := range qualitySteps {
		lbl := "Авто"
		if q > 0 {
			lbl = fmt.Sprintf("%.0f%%", q*100)
		}
		if u.ButtonState(x, y, sbw, 34, lbl, g.set.Quality == q, true) {
			g.setQuality(q)
		}
		x += sbw + sgap
	}
	y += 42
	note = fmt.Sprintf("Кадр %d×%d. Меньше — быстрее, но картинка мягче (если лагает на 4K, выберите 75%% или 50%%).", u.screen.Bounds().Dx(), u.screen.Bounds().Dy())
	drawText(u.screen, note, float64(cx), float64(y), 13, colDim, 1)
	y += 30

	gl := "Стиль «жидкое стекло»: вкл (F9)"
	if !g.glass {
		gl = "Стиль «жидкое стекло»: выкл (F9)"
	}
	if u.ButtonState(cx-150, y, 300, 34, gl, g.glass, !g.fxFailed) {
		g.setGlass(!g.glass)
	}
	return y + 46
}

func (g *Game) drawSettings() {
	u := &g.ui
	cx, y := g.menuFrame("Настройки")
	y = g.drawSettingsBody(cx, y)
	if u.Button(cx-100, y+4, 200, 42, "Назад") {
		g.scene = sceneMenu
	}
}

// ---------------------------------------------------------------------
// Меню внутри партии.

// freezeInput отключает ввод для всего, что рисуется следом: пока открыто окно меню, панели игры не реагируют.
func (u *UI) freezeInput() input {
	saved := u.in
	u.in.click, u.in.rclick, u.in.dblclick = false, false, false
	u.in.down, u.in.rdown, u.in.mdown = false, false, false
	u.in.wheel = 0
	u.in.mx, u.in.my = -10000, -10000
	return saved
}

func (g *Game) toggleGameMenu() {
	g.menuOpen = !g.menuOpen
	g.menuPage = 0
}

// drawGameMenu рисует окно меню поверх партии.
func (g *Game) drawGameMenu() {
	u := &g.ui
	g.glassPrep(u.screen)
	fillRect(u.screen, 0, 0, float64(u.W), float64(u.H), color.RGBA{0, 0, 0, 120})
	host := g.sess != nil && g.sess.IsHost()
	pw, ph := 640, 330
	if g.menuPage == 1 {
		ph = 560
	}
	ph = min(ph, u.H-24)
	px, py := (u.W-pw)/2, (u.H-ph)/2
	u.PanelT(px, py, pw, ph, 0.85)
	u.blockUI(px, py, pw, ph)
	cx := u.W / 2
	title := "Меню"
	if g.menuPage == 1 {
		title = "Настройки"
	}
	drawBold(u.screen, title, float64(cx), float64(py+22), 22, colText, 1)
	y := py + 70
	if g.menuPage == 1 {
		y = g.drawSettingsBody(cx, y)
		if u.Button(cx-100, y+4, 200, 40, "Назад") {
			g.menuPage = 0
		}
		return
	}
	const bw = 300
	if u.Button(cx-bw/2, y, bw, 42, "Продолжить (Esc)") {
		g.menuOpen = false
	}
	y += 50
	if u.Button(cx-bw/2, y, bw, 42, "Настройки") {
		g.menuPage = 1
	}
	y += 50
	if host {
		if u.Button(cx-bw/2, y, bw, 42, "Сохранить игру (F5)") {
			g.quickSave()
		}
		y += 50
	}
	if u.Button(cx-bw/2, y, bw, 42, "Выйти в главное меню") {
		g.leaveGame()
	}
}
