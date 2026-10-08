package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/netplay"
	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
)

// Сцены.
const (
	sceneMenu = iota
	sceneHostSetup
	sceneConnect
	sceneSandbox
	sceneSolo
	sceneLoad
	sceneLobby
	sceneGame
)

// Режимы карты.
const (
	modeNone = iota
	modeBuild
	modeFort
	modeStrike
	modeMain
	modePlace // расстановка резерва перед стартом
)

// Selection — выбранный объект.
type Selection struct {
	Kind string // building, unit, contact, city
	ID   uint32
	Idx  int
}

type toast struct {
	text string
	at   time.Time
}

// Game — ebiten.Game.
type Game struct {
	ui       UI
	scene    int
	cat      *data.Catalog
	m        *world.MapData
	dataDir  string
	saveDir  string
	dataHash string

	sess           netplay.Session
	view           *sim.View
	rend           *MapRenderer
	cam            Camera
	tab            int
	glass          bool // стиль «жидкое стекло»
	fxFailed       bool
	menuCheat      bool // галочка «всё открыто» в настройке песочницы
	techOpen       bool // открыто окно исследований
	scroll         map[string]float64
	sel            Selection
	mode           int
	buildType      string
	placeType      string
	strike         strikePlan
	layers         map[string]bool
	toasts         []toast
	evSeen         uint64
	evInit         bool
	dragging       bool
	dragX, dragY   int
	dragCX, dragCY float64
	fortPainted    map[int]bool

	// меню
	menuSide     int
	portField    TextField
	ipField      TextField
	menuErr      string
	saves        []string
	lastW, lastH int
	auto         *autoShot
	notices      []string
	cycleSeen    map[string]map[uint32]bool
	labels       []image.Rectangle
	groups       [9][]Selection // контрольные группы (клавиши 1…9)
}

type strikePlan struct {
	Source   uint32
	Munition string
	Count    int
	Pts      []sim.Pt
	Delay    float64
}

// New создаёт игру.
func New(cat *data.Catalog, m *world.MapData, dataDir, saveDir string) *Game {
	netplay.LogDir = filepath.Join(filepath.Dir(dataDir), "logs")
	flat := os.Getenv("DRONEEBLA_FLAT") != "" // отладка: плоский стиль без шейдеров
	g := &Game{
		glass: !flat,
		cat:   cat, m: m, dataDir: dataDir, saveDir: saveDir,
		dataHash: netplay.DataHash(dataDir),
		rend:     newMapRenderer(m),
		scroll:   map[string]float64{},
		layers:   map[string]bool{"fog": true, "ad": true, "sats": false, "logistics": false, "energy": false, "deposits": false},
	}
	g.initAutoShot()
	g.portField.Text = strconv.Itoa(netplay.DefaultPort)
	g.portField.Max = 5
	g.portField.Allowed = func(r rune) bool { return r >= '0' && r <= '9' }
	g.ipField.Text = "26."
	g.ipField.Max = 40
	g.ipField.Allowed = func(r rune) bool { return r != ' ' }
	return g
}

// Notice — сообщение для главного меню.
func (g *Game) Notice(s string) { g.notices = append(g.notices, s) }

// Layout — логический размер равен размеру окна.
func (g *Game) Layout(w, h int) (int, int) {
	s := uiScale(w, h)
	return int(float64(w) / s), int(float64(h) / s)
}

// uiScale — масштаб интерфейса: образец — окно 1440×810; шаг 0.25, чтобы картинка оставалась чёткой.
func uiScale(w, h int) float64 {
	s := math.Min(float64(w)/1440, float64(h)/810)
	s = math.Floor(s*4) / 4
	return math.Max(0.85, math.Min(2.5, s))
}

// Update — сбор ввода.
func (g *Game) Update() error {
	in := &g.ui.in
	in.tick++
	in.mx, in.my = ebiten.CursorPosition()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		in.click = true
		if in.tick-in.lastClickT < 18 {
			in.dblclick = true
		}
		in.lastClickT = in.tick
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		in.rclick = true
	}
	in.down = ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	in.rdown = ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight)
	in.mdown = ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)
	_, wy := ebiten.Wheel()
	in.wheel += wy
	in.collectTextInput()
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) ||
		(inpututil.IsKeyJustPressed(ebiten.KeyEnter) && ebiten.IsKeyPressed(ebiten.KeyAlt)) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	g.glassKey()
	if g.scene == sceneGame {
		g.gameKeys()
	}
	return nil
}

func (g *Game) resetInput() {
	in := &g.ui.in
	in.click, in.rclick, in.dblclick, in.consumed, in.overUI = false, false, false, false, false
	in.wheel = 0
	in.chars, in.backspace = in.chars[:0], 0
}

// Draw — отрисовка и обработка интерфейса (непосредственный режим).
func (g *Game) Draw(screen *ebiten.Image) {
	g.ui.screen = screen
	g.ui.W, g.ui.H = screen.Bounds().Dx(), screen.Bounds().Dy()
	g.fxInit()
	screen.Fill(colPanel2)
	if g.scene != sceneGame {
		g.drawBackdrop(screen)
		g.glassPrep(screen)
	}
	switch g.scene {
	case sceneMenu:
		g.drawMenu()
	case sceneHostSetup:
		g.drawHostSetup()
	case sceneConnect:
		g.drawConnect()
	case sceneSandbox:
		g.drawSandboxSetup()
	case sceneSolo:
		g.drawSoloSetup()
	case sceneLoad:
		g.drawLoad()
	case sceneLobby:
		g.drawLobby()
	case sceneGame:
		g.drawGame()
	}
	g.ui.drawTooltip()
	g.resetInput()
	if g.auto != nil {
		g.autoShotStep(screen)
	}
}

func (g *Game) toast(s string) {
	g.toasts = append(g.toasts, toast{text: s, at: time.Now()})
	if len(g.toasts) > 8 {
		g.toasts = g.toasts[len(g.toasts)-8:]
	}
}

// ---------------------------------------------------------------------
// Меню.

func (g *Game) menuFrame(title string) (int, int) {
	u := &g.ui
	cx := u.W / 2
	drawBold(u.screen, "DRONEEBLA", float64(cx), 48, 44, colAccent, 1)
	drawText(u.screen, "Война на истощение: экономика, дроны и ПВО", float64(cx), 104, 17, colDim, 1)
	ph := map[int]int{sceneMenu: 580, sceneHostSetup: 440, sceneConnect: 340, sceneSandbox: 400, sceneSolo: 340, sceneLoad: 640, sceneLobby: 520}[g.scene]
	if ph == 0 || ph > u.H-150-24 {
		ph = u.H - 150 - 24
	}
	pw := 700
	if pw > u.W-40 {
		pw = u.W - 40
	}
	u.Panel(cx-pw/2, 144, pw, ph)
	drawBold(u.screen, title, float64(cx), 164, 22, colText, 1)
	return cx, 214
}

// menuW — ширина содержимого меню.
const menuW = 560

// centerText рисует абзац по центру с переносом по ширине w и возвращает следующий y.
func (g *Game) centerText(text string, cx int, y float64, w float64, size float64, c color.Color) float64 {
	for _, l := range wrap(text, size, w) {
		drawText(g.ui.screen, l, float64(cx), y, size, c, 1)
		y += size + 7
	}
	return y
}

// menuButtons — пара кнопок «Назад» и основное действие в ряд.
func (g *Game) menuButtons(cx, y int, ok string) (back, act bool) {
	u := &g.ui
	back = u.Button(cx-200-8, y, 200, 42, "Назад")
	act = u.Button(cx+8, y, 200, 42, ok)
	return
}

func (g *Game) menuError(cx, y int) {
	if g.menuErr != "" {
		g.centerText(g.menuErr, cx, float64(y), menuW, 15, colBad)
	}
}

func (g *Game) drawMenu() {
	u := &g.ui
	cx, y := g.menuFrame("Главное меню")
	bw := menuW - 160
	items := []struct {
		label string
		scene int
	}{
		{"Создать сетевую игру (хост)", sceneHostSetup},
		{"Подключиться к игре", sceneConnect},
		{"Одиночная игра (против ИИ)", sceneSolo},
		{"Песочница (оба игрока вручную)", sceneSandbox},
		{"Загрузить сохранение", sceneLoad},
	}
	for _, it := range items {
		if u.Button(cx-bw/2, y, bw, 42, it.label) {
			g.scene, g.menuErr = it.scene, ""
			if it.scene == sceneLoad {
				g.saves = g.listSaves()
			}
		}
		y += 50
	}
	if u.Button(cx-bw/2, y, bw, 42, "Выход") {
		os.Exit(0)
	}
	y += 58
	gl := "Стиль «жидкое стекло»: вкл (F9)"
	if !g.glass {
		gl = "Стиль «жидкое стекло»: выкл (F9)"
	}
	if u.ButtonState(cx-bw/2, y, bw, 34, gl, g.glass, !g.fxFailed) {
		g.glass = !g.glass
	}
	y += 52
	fy := float64(y)
	fy = g.centerText("Сетевая игра до 6 игроков (до 3 за сторону) через Radmin VPN: хост создаёт игру, остальные вводят его IP из Radmin (26.x.x.x) и выбирают сторону в лобби.", cx, fy, menuW, 13, colDim)
	fy = g.centerText("Игровые цифры лежат в папке data рядом с игрой — их можно править без пересборки (у всех игроков файлы должны совпадать).", cx, fy+4, menuW, 13, colDim)
	for _, n := range g.notices {
		fy = g.centerText(n, cx, fy+4, menuW, 13, colWarn)
	}
}

// sidePicker — «Ваша сторона» и две кнопки в одной строке по центру.
func (g *Game) sidePicker(cx, y int) {
	u := &g.ui
	drawText(u.screen, "Ваша сторона:", float64(cx-menuW/2+20), float64(y+9), 16, colText, 0)
	if u.ButtonState(cx-30, y, 130, 36, "Россия", g.menuSide == data.RU, true) {
		g.menuSide = data.RU
	}
	if u.ButtonState(cx+110, y, 130, 36, "Украина", g.menuSide == data.UA, true) {
		g.menuSide = data.UA
	}
}

func (g *Game) drawHostSetup() {
	u := &g.ui
	cx, y := g.menuFrame("Создание сетевой игры")
	g.sidePicker(cx, y)
	y += 56
	drawText(u.screen, "Порт:", float64(cx-menuW/2+20), float64(y+9), 16, colText, 0)
	g.portField.Draw(u, cx-30, y, 130, 36)
	y += 60
	fy := g.centerText("Ваши IP-адреса: сообщите игрокам адрес Radmin VPN (обычно 26.x.x.x).", cx, float64(y), menuW, 13, colDim) + 4
	for i, ip := range netplay.LocalIPs() {
		if i > 5 {
			break
		}
		drawText(u.screen, ip, float64(cx), fy, 17, colAccent, 1)
		fy += 24
	}
	by := int(fy) + 22
	back, act := g.menuButtons(cx, by, "Создать")
	if back {
		g.scene = sceneMenu
	}
	if act {
		port, err := strconv.Atoi(strings.TrimSpace(g.portField.Text))
		if err != nil {
			g.menuErr = "Неверный порт"
		} else {
			w := sim.New(g.cat, g.m, false)
			h, err := netplay.NewHost(w, g.menuSide, port, g.dataHash)
			if err != nil {
				g.menuErr = "Не удалось открыть порт: " + err.Error()
			} else {
				g.openLobby(h)
			}
		}
	}
	g.menuError(cx, by+60)
}

func (g *Game) drawConnect() {
	u := &g.ui
	cx, y := g.menuFrame("Подключение к игре")
	y0 := g.centerText("IP хоста (Radmin VPN); порт можно указать через двоеточие.", cx, float64(y), menuW, 14, colDim)
	g.ipField.Draw(u, cx-170, int(y0)+8, 340, 38)
	by := int(y0) + 74
	back, act := g.menuButtons(cx, by, "Подключиться")
	if back {
		g.scene = sceneMenu
	}
	if act {
		cl, err := netplay.Connect(strings.TrimSpace(g.ipField.Text), g.dataHash)
		if err != nil {
			g.menuErr = "Ошибка: " + err.Error()
		} else {
			g.openLobby(cl)
		}
	}
	g.menuError(cx, by+60)
}

func (g *Game) drawSandboxSetup() {
	u := &g.ui
	cx, y := g.menuFrame("Песочница")
	g.sidePicker(cx, y)
	y += 62
	ty := g.centerText("Противник не управляется. Сторону можно переключать во время игры кнопкой в верхней панели.", cx, float64(y), menuW, 14, colDim)
	mark := "[ ]"
	if g.menuCheat {
		mark = "[x]"
	}
	if u.ButtonState(cx-menuW/2+20, int(ty)+10, menuW-40, 36, mark+" Всё открыто: всё изучено, производство и стройка мгновенно и бесплатно", g.menuCheat, true) {
		g.menuCheat = !g.menuCheat
	}
	by := int(ty) + 80
	back, act := g.menuButtons(cx, by, "Начать")
	if back {
		g.scene = sceneMenu
	}
	if act {
		w := sim.New(g.cat, g.m, true)
		if g.menuCheat {
			w.EnableCheat()
		}
		g.startGame(netplay.NewSandbox(w, g.menuSide))
	}
}

func (g *Game) drawSoloSetup() {
	cx, y := g.menuFrame("Одиночная игра")
	g.sidePicker(cx, y)
	ty := float64(y + 66)
	ty = g.centerText("Противоположной стороной управляет компьютер.", cx, ty, menuW, 14, colText)
	ty = g.centerText("ИИ играет по тем же правилам: видит только то, что видит его разведка, и отдаёт те же приказы. Его настройки — файл ai.json в папке data рядом с игрой.", cx, ty+4, menuW, 13, colDim)
	by := int(ty) + 30
	back, act := g.menuButtons(cx, by, "Начать")
	if back {
		g.scene = sceneMenu
	}
	if act {
		w := sim.New(g.cat, g.m, true)
		g.startGame(netplay.NewSolo(w, g.menuSide))
	}
}

func (g *Game) listSaves() []string {
	files, _ := filepath.Glob(filepath.Join(g.saveDir, "*.sav"))
	sort.Slice(files, func(a, b int) bool {
		ia, _ := os.Stat(files[a])
		ib, _ := os.Stat(files[b])
		if ia == nil || ib == nil {
			return files[a] > files[b]
		}
		return ia.ModTime().After(ib.ModTime())
	})
	return files
}

func (g *Game) drawLoad() {
	u := &g.ui
	cx, y := g.menuFrame("Загрузка сохранения")
	g.sidePicker(cx, y)
	y += 52
	drawText(u.screen, "Порт:", float64(cx-menuW/2+20), float64(y+9), 16, colText, 0)
	g.portField.Draw(u, cx-30, y, 130, 36)
	ly := y + 54
	if len(g.saves) == 0 {
		g.centerText("Сохранений нет (папка saves рядом с игрой)", cx, float64(ly), menuW, 14, colDim)
	}
	rows := (u.H - ly - 150) / 38
	if rows > 10 {
		rows = 10
	}
	if rows < 3 {
		rows = 3
	}
	for i, f := range g.saves {
		if i >= rows {
			break
		}
		name := filepath.Base(f)
		bw := menuW/2 - 24
		if u.Button(cx-bw-6, ly+i*38, bw, 32, name) {
			g.loadSave(f, false)
		}
		if u.Button(cx+6, ly+i*38, bw, 32, "без сети (песочница / одиночная)") {
			g.loadSave(f, true)
		}
	}
	by := ly + rows*38 + 14
	if u.Button(cx-100, by, 200, 42, "Назад") {
		g.scene = sceneMenu
	}
	g.menuError(cx, by+58)
}

func (g *Game) loadSave(path string, sandbox bool) {
	w, err := sim.Load(path, g.cat, g.m)
	if err != nil {
		g.menuErr = "Не удалось загрузить: " + err.Error()
		return
	}
	if sandbox && w.Solo {
		g.startGame(netplay.NewSolo(w, w.Human))
		return
	}
	w.Sandbox, w.Solo = sandbox, false
	if sandbox {
		g.startGame(netplay.NewSandbox(w, g.menuSide))
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(g.portField.Text))
	if err != nil {
		g.menuErr = "Неверный порт"
		return
	}
	h, err := netplay.NewHost(w, g.menuSide, port, g.dataHash)
	if err != nil {
		g.menuErr = "Не удалось открыть порт: " + err.Error()
		return
	}
	g.openLobby(h)
}

func (g *Game) startGame(s netplay.Session) {
	g.sess = s
	g.scene = sceneGame
	g.view = nil
	g.sel = Selection{}
	g.mode = modeNone
	g.evSeen, g.evInit = 0, false
	g.menuErr = ""
	// Камера на центр карты.
	x, y := g.m.Project(32.5, 48.8)
	g.cam = Camera{CX: x, CY: y, Z: 0.8}
}

// QuickSave — сохранение (только хост).
func (g *Game) quickSave() {
	if g.sess == nil || !g.sess.IsHost() {
		g.toast("Сохранять может только хост")
		return
	}
	os.MkdirAll(g.saveDir, 0o755)
	name := fmt.Sprintf("%s.sav", time.Now().Format("2006-01-02_15-04-05"))
	p := filepath.Join(g.saveDir, name)
	if err := g.sess.Save(p); err != nil {
		g.toast("Ошибка сохранения: " + err.Error())
	} else {
		g.toast("Сохранено: " + name)
	}
}
