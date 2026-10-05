package ui

import (
	"fmt"
	"image"
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
	sceneLoad
	sceneGame
)

// Режимы карты.
const (
	modeNone = iota
	modeBuild
	modeFort
	modeStrike
	modeMain
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
	scroll         map[string]float64
	sel            Selection
	mode           int
	buildType      string
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
	g := &Game{
		cat: cat, m: m, dataDir: dataDir, saveDir: saveDir,
		dataHash: netplay.DataHash(dataDir),
		rend:     newMapRenderer(m),
		scroll:   map[string]float64{},
		layers:   map[string]bool{"fog": true, "ad": true, "sats": false, "logistics": false, "energy": false, "deposits": false},
	}
	g.initAutoShot()
	g.portField.Text = strconv.Itoa(netplay.DefaultPort)
	g.portField.Max = 5
	g.ipField.Text = "26."
	g.ipField.Max = 40
	return g
}

// Notice — сообщение для главного меню.
func (g *Game) Notice(s string) { g.notices = append(g.notices, s) }

// Layout — логический размер равен размеру окна.
func (g *Game) Layout(w, h int) (int, int) {
	return w, h
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
	if g.scene == sceneGame {
		g.gameKeys()
	}
	return nil
}

func (g *Game) resetInput() {
	in := &g.ui.in
	in.click, in.rclick, in.dblclick, in.consumed, in.overUI = false, false, false, false, false
	in.wheel = 0
}

// Draw — отрисовка и обработка интерфейса (непосредственный режим).
func (g *Game) Draw(screen *ebiten.Image) {
	g.ui.screen = screen
	g.ui.W, g.ui.H = screen.Bounds().Dx(), screen.Bounds().Dy()
	screen.Fill(colPanel2)
	switch g.scene {
	case sceneMenu:
		g.drawMenu()
	case sceneHostSetup:
		g.drawHostSetup()
	case sceneConnect:
		g.drawConnect()
	case sceneSandbox:
		g.drawSandboxSetup()
	case sceneLoad:
		g.drawLoad()
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
	drawBold(u.screen, "DRONEEBLA", float64(cx), 60, 44, colAccent, 1)
	drawText(u.screen, "Война на истощение: экономика, дроны и ПВО", float64(cx), 116, 18, colDim, 1)
	drawBold(u.screen, title, float64(cx), 170, 22, colText, 1)
	return cx, 220
}

func (g *Game) drawMenu() {
	u := &g.ui
	cx, y := g.menuFrame("Главное меню")
	bw := 360
	if u.Button(cx-bw/2, y, bw, 44, "Создать сетевую игру (хост)") {
		g.scene, g.menuErr = sceneHostSetup, ""
	}
	if u.Button(cx-bw/2, y+56, bw, 44, "Подключиться к игре") {
		g.scene, g.menuErr = sceneConnect, ""
	}
	if u.Button(cx-bw/2, y+112, bw, 44, "Песочница (один игрок)") {
		g.scene, g.menuErr = sceneSandbox, ""
	}
	if u.Button(cx-bw/2, y+168, bw, 44, "Загрузить сохранение (хост)") {
		g.scene, g.menuErr = sceneLoad, ""
		g.saves = g.listSaves()
	}
	if u.Button(cx-bw/2, y+224, bw, 44, "Выход") {
		os.Exit(0)
	}
	lines := []string{
		"Сетевая игра на двоих через Radmin VPN: хост создаёт игру, второй игрок вводит его IP из Radmin (26.x.x.x).",
		"Игровые цифры лежат в папке data рядом с игрой — их можно править без пересборки (у обоих игроков файлы должны совпадать).",
	}
	for i, l := range lines {
		drawText(u.screen, l, float64(cx), float64(y+300+i*22), 14, colDim, 1)
	}
	ny := float64(y + 360)
	for _, n := range g.notices {
		for _, l := range wrap(n, 14, float64(u.W)-200) {
			drawText(u.screen, l, float64(cx), ny, 14, colWarn, 1)
			ny += 20
		}
	}
}

func (g *Game) sidePicker(cx, y int) {
	u := &g.ui
	drawText(u.screen, "Ваша сторона:", float64(cx-180), float64(y+8), 16, colText, 0)
	if u.ButtonState(cx-40, y, 110, 34, "Россия", g.menuSide == data.RU, true) {
		g.menuSide = data.RU
	}
	if u.ButtonState(cx+80, y, 110, 34, "Украина", g.menuSide == data.UA, true) {
		g.menuSide = data.UA
	}
}

func (g *Game) drawHostSetup() {
	u := &g.ui
	cx, y := g.menuFrame("Создание сетевой игры")
	g.sidePicker(cx, y)
	drawText(u.screen, "Порт:", float64(cx-180), float64(y+60), 16, colText, 0)
	g.portField.Draw(u, cx-40, y+52, 110, 32)
	ips := netplay.LocalIPs()
	drawText(u.screen, "Ваши IP-адреса (сообщите второму игроку адрес Radmin VPN, обычно 26.x.x.x):", float64(cx), float64(y+104), 14, colDim, 1)
	for i, ip := range ips {
		if i > 5 {
			break
		}
		drawText(u.screen, ip, float64(cx), float64(y+128+i*20), 16, colAccent, 1)
	}
	by := y + 260
	if u.Button(cx-180, by, 170, 40, "Назад") {
		g.scene = sceneMenu
	}
	if u.Button(cx+10, by, 170, 40, "Создать") {
		port, err := strconv.Atoi(strings.TrimSpace(g.portField.Text))
		if err != nil {
			g.menuErr = "Неверный порт"
		} else {
			w := sim.New(g.cat, g.m, false)
			h, err := netplay.NewHost(w, g.menuSide, port, g.dataHash)
			if err != nil {
				g.menuErr = "Не удалось открыть порт: " + err.Error()
			} else {
				g.startGame(h)
			}
		}
	}
	if g.menuErr != "" {
		drawText(u.screen, g.menuErr, float64(cx), float64(by+56), 15, colBad, 1)
	}
}

func (g *Game) drawConnect() {
	u := &g.ui
	cx, y := g.menuFrame("Подключение к игре")
	drawText(u.screen, "IP хоста (Radmin VPN), можно с портом через двоеточие:", float64(cx), float64(y), 15, colDim, 1)
	g.ipField.Draw(u, cx-160, y+24, 320, 34)
	by := y + 90
	if u.Button(cx-180, by, 170, 40, "Назад") {
		g.scene = sceneMenu
	}
	if u.Button(cx+10, by, 170, 40, "Подключиться") {
		cl, err := netplay.Connect(strings.TrimSpace(g.ipField.Text), g.dataHash)
		if err != nil {
			g.menuErr = "Ошибка: " + err.Error()
		} else {
			g.startGame(cl)
		}
	}
	if g.menuErr != "" {
		drawText(u.screen, g.menuErr, float64(cx), float64(by+56), 15, colBad, 1)
	}
}

func (g *Game) drawSandboxSetup() {
	u := &g.ui
	cx, y := g.menuFrame("Песочница")
	g.sidePicker(cx, y)
	drawText(u.screen, "Противник не управляется. Сторону можно переключать во время игры кнопкой в верхней панели.", float64(cx), float64(y+64), 14, colDim, 1)
	by := y + 120
	if u.Button(cx-180, by, 170, 40, "Назад") {
		g.scene = sceneMenu
	}
	if u.Button(cx+10, by, 170, 40, "Начать") {
		w := sim.New(g.cat, g.m, true)
		g.startGame(netplay.NewSandbox(w, g.menuSide))
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
	drawText(u.screen, "Порт:", float64(cx-180), float64(y+52), 16, colText, 0)
	g.portField.Draw(u, cx-40, y+44, 110, 32)
	ly := y + 96
	if len(g.saves) == 0 {
		drawText(u.screen, "Сохранений нет (папка saves рядом с игрой)", float64(cx), float64(ly), 15, colDim, 1)
	}
	for i, f := range g.saves {
		if i >= 10 {
			break
		}
		name := filepath.Base(f)
		bw := 280
		if u.Button(cx-bw-5, ly+i*38, bw, 32, name) {
			g.loadSave(f, false)
		}
		if u.Button(cx+5, ly+i*38, bw, 32, "в песочнице") {
			g.loadSave(f, true)
		}
	}
	by := ly + 10*38 + 10
	if u.Button(cx-85, by, 170, 40, "Назад") {
		g.scene = sceneMenu
	}
	if g.menuErr != "" {
		drawText(u.screen, g.menuErr, float64(cx), float64(by+56), 15, colBad, 1)
	}
}

func (g *Game) loadSave(path string, sandbox bool) {
	w, err := sim.Load(path, g.cat, g.m)
	if err != nil {
		g.menuErr = "Не удалось загрузить: " + err.Error()
		return
	}
	w.Sandbox = sandbox
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
	g.startGame(h)
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
