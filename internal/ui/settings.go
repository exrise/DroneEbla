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
	WinW         int     `json:"win_w"`
	WinH         int     `json:"win_h"`
	Fullscreen   bool    `json:"fullscreen"`
	Scale        float64 `json:"scale"`         // масштаб интерфейса; 0 — автоматически по размеру окна
	Quality      float64 `json:"quality"`       // качество отрисовки (доля физического разрешения); 0 — авто
	GlassOpacity float64 `json:"glass_opacity"` // прозрачность стекла: 0 — почти прозрачное, 1 — плотное; 0,5 — по умолчанию
	Glass        bool    `json:"glass"`
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
	return Settings{WinW: 1600, WinH: 900, Scale: 0, Glass: true, GlassOpacity: 0.5}
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
	if !(s.GlassOpacity >= 0 && s.GlassOpacity <= 1) { // в том числе NaN
		s.GlassOpacity = d.GlassOpacity
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

func (g *Game) setGlassOpacity(v float64) {
	g.set.GlassOpacity = math.Round(math.Max(0, math.Min(1, v))*100) / 100
	g.saveSettings()
}

// glassTintK — множитель плотности подложки стеклянных панелей по ползунку (0,5 → 1, как раньше).
func glassTintK(opacity float64) float32 {
	return float32(0.25 + 1.5*math.Max(0, math.Min(1, opacity)))
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

// drawSettingsBody рисует настройки экрана; возвращает y под последним элементом.
// Сетка одна на все разделы: подпись 20, ряд кнопок 32 (шаг 40), пояснение 18, отступ между разделами 14.
func (g *Game) drawSettingsBody(cx, y int) int {
	u := &g.ui
	left := float64(cx - menuW/2)
	gap := 14
	if u.H < 780 { // минимальный логический экран: плотнее
		gap = 8
	}
	section := func(title string) {
		drawText(u.screen, title, left, float64(y), 14, colDim, 0)
		y += 20
	}
	note := func(s string, c color.Color) {
		drawText(u.screen, s, float64(cx), float64(y), 13, c, 1)
		y += 18
	}
	// row рисует ряд одинаковых кнопок по центру; click получает индекс нажатой.
	row := func(n, bw, g int, btn func(i, x int) bool) {
		x := cx - (n*bw+(n-1)*g)/2
		for i := 0; i < n; i++ {
			btn(i, x)
			x += bw + g
		}
		y += 40
	}

	section("Режим экрана")
	row(2, 200, 8, func(i, x int) bool {
		if i == 0 {
			if u.ButtonState(x, y, 200, 32, "Окно", !g.set.Fullscreen, true) {
				g.setFullscreen(false)
			}
		} else if u.ButtonState(x, y, 200, 32, "Полный экран (F11)", g.set.Fullscreen, true) {
			g.setFullscreen(true)
		}
		return false
	})
	y += gap - 8

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
	const perRow = 4
	for i := 0; i < len(choices); i += perRow {
		r := choices[i:min(i+perRow, len(choices))]
		row(len(r), 130, 8, func(k, x int) bool {
			c := r[k]
			on := !g.set.Fullscreen && g.set.WinW == c[0] && g.set.WinH == c[1]
			if u.ButtonState(x, y, 130, 32, fmt.Sprintf("%d×%d", c[0], c[1]), on, !g.set.Fullscreen) {
				g.setWindowSize(c[0], c[1])
			}
			return false
		})
	}
	if g.set.Fullscreen {
		note("В полном экране размер окна не используется.", colDim)
	} else {
		note("Размер окна можно менять и мышью — потянув за край.", colDim)
	}
	y += gap - 4

	section("Масштаб интерфейса")
	row(len(scaleSteps), 72, 6, func(i, x int) bool {
		s := scaleSteps[i]
		lbl := "Авто"
		if s > 0 {
			lbl = fmt.Sprintf("%.0f%%", s*100)
		}
		ww, wh := ebiten.WindowSize()
		fits := s == 0 || layoutScale(float64(ww), float64(wh), g.dsf, s) >= s*g.dsf-1e-9
		if u.ButtonState(x, y, 72, 32, lbl, g.set.Scale == s, fits) {
			g.setScale(s)
		}
		return false
	})
	switch {
	case g.capped:
		note(fmt.Sprintf("Выбрано %.0f%%, но при таком размере окна интерфейс не помещается — показано %.0f%%.", g.set.Scale*100, g.effScale*100), colWarn)
	case g.set.Scale == 0:
		note(fmt.Sprintf("Авто: по размеру окна, сейчас %.0f%%. Ctrl+«+» / Ctrl+«−» — вручную.", g.effScale*100), colDim)
	default:
		note(fmt.Sprintf("Сейчас %.0f%%. Ctrl+«+» / Ctrl+«−» — шаг, Ctrl+0 — авто.", g.effScale*100), colDim)
	}
	y += gap - 4

	section("Качество отрисовки")
	row(len(qualitySteps), 72, 6, func(i, x int) bool {
		q := qualitySteps[i]
		lbl := "Авто"
		if q > 0 {
			lbl = fmt.Sprintf("%.0f%%", q*100)
		}
		if u.ButtonState(x, y, 72, 32, lbl, g.set.Quality == q, true) {
			g.setQuality(q)
		}
		return false
	})
	note(fmt.Sprintf("Кадр %d×%d. Меньше — быстрее, но картинка мягче.", u.screen.Bounds().Dx(), u.screen.Bounds().Dy()), colDim)
	y += gap - 4

	section("Жидкое стекло")
	gl := "Включено (F9)"
	if !g.glass {
		gl = "Выключено (F9)"
	}
	x0 := cx - menuW/2 + 6
	if u.ButtonState(x0, y, 200, 32, gl, g.glass, !g.fxFailed) {
		g.setGlass(!g.glass)
	}
	drawText(u.screen, "Прозрачность", float64(x0+200+16), float64(y+9), 13, colDim, 0)
	sx := x0 + 200 + 16 + int(textWidth("Прозрачность", 13)) + 10
	sw := cx + menuW/2 - 6 - 44 - sx
	if v, ch := u.Slider("glass", sx, y, sw, 32, g.set.GlassOpacity); ch {
		g.setGlassOpacity(v)
	}
	drawText(u.screen, fmt.Sprintf("%.0f%%", g.set.GlassOpacity*100), float64(cx+menuW/2-6), float64(y+9), 13, colText, 2)
	return y + 32 + gap + 4
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
