package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/exrise/droneebla/internal/ai"
	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/netplay"
	"github.com/exrise/droneebla/internal/sim"
)

// Отладка: DRONEEBLA_SHOT=<папка> — игра сама запускает песочницу,
// делает снимки экрана и выходит. Нужна для проверки интерфейса без монитора.

type autoShot struct {
	dir   string
	frame int
	step  int
	mark  int
}

func (g *Game) initAutoShot() {
	if d := os.Getenv("DRONEEBLA_SHOT"); d != "" {
		os.MkdirAll(d, 0o755)
		g.auto = &autoShot{dir: d}
	}
}

func (g *Game) save(screen *ebiten.Image, name string) {
	b := screen.Bounds()
	img := image.NewRGBA(b)
	screen.ReadPixels(img.Pix)
	f, err := os.Create(filepath.Join(g.auto.dir, name+".png"))
	if err != nil {
		return
	}
	png.Encode(f, img)
	f.Close()
}

// autoShotStep вызывается в конце Draw.
func (g *Game) autoShotStep(screen *ebiten.Image) {
	a := g.auto
	a.frame++
	if a.frame < 10 {
		return
	}
	if os.Getenv("DRONEEBLA_SOLO") != "" {
		g.autoSolo(screen)
		return
	}
	if os.Getenv("DRONEEBLA_PERF") != "" {
		g.autoPerf()
		return
	}
	if os.Getenv("DRONEEBLA_SETTINGS") != "" {
		g.autoSettings(screen)
		return
	}
	if os.Getenv("DRONEEBLA_LOBBY") != "" {
		g.autoLobby(screen)
		return
	}
	switch a.step {
	case 0:
		g.save(screen, "00_menu")
		g.menuSide = 0
		w := sim.New(g.cat, g.m, true)
		g.startGame(netplay.NewSandbox(w, 0))
	case 3:
		g.save(screen, "01_start")
		h := g.sess.(*netplay.Host)
		h.Advance(g.cat.Rules.PrepMinutes+30, func(w *sim.World) {
			w.Apply(sim.Command{Kind: sim.CmdResearch, Side: 0, Item: "ru_geran2"})
			w.Apply(sim.Command{Kind: sim.CmdOrderAdd, Side: 0, Item: "kalibr", Count: 10})
			w.Apply(sim.Command{Kind: sim.CmdOrderAdd, Side: 0, Item: "armor", Count: 0})
		})
		// Удар Калибрами по Киеву.
		h.Advance(1, func(w *sim.World) {
			var src, tgt *sim.Building
			for _, b := range w.Buildings {
				if b.Name == "Севастополь — база ЧФ" {
					src = b
				}
				if b.Name == "Трипольская ТЭС" {
					tgt = b
				}
			}
			mx, my := w.Map().Project(30.0, 47.5)
			e := w.Apply(sim.Command{Kind: sim.CmdStrike, Side: 0, ID: src.ID, Item: "kalibr", Count: 12, Pts: []sim.Pt{{X: mx, Y: my}}, X: tgt.X, Y: tgt.Y})
			if e != "" {
				fmt.Println("strike:", e)
			}
			w.Apply(sim.Command{Kind: sim.CmdPosture, Side: 0, Int: sim.PostureOffense})
			ex, ey := w.Map().Project(37.75, 48.14)
			w.Apply(sim.Command{Kind: sim.CmdMainEffort, Side: 0, Int: 1, X: ex, Y: ey})
		})
		h.Advance(25, nil)
		g.layers["sats"] = true
	case 6:
		g.save(screen, "02_war")
		x, y := g.m.Project(31.5, 48.0)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 1.3
	case 9:
		g.save(screen, "03_zoom_strike")
	case 10, 11, 12, 13, 14, 15, 16, 17, 18:
		if a.step > 10 {
			g.save(screen, fmt.Sprintf("1%d_tab", a.step-11))
		}
		g.tab = (a.step - 10) % 8
	case 19:
		// Выбор ПВО и планирование удара.
		for _, u := range g.view.Units {
			if u.Type == "iskander" {
				g.sel = Selection{Kind: "unit", ID: u.ID}
				g.centerOn(u.X, u.Y)
				break
			}
		}
	case 20:
		g.save(screen, "20_unit")
		g.mode = modeStrike
		g.strike = strikePlan{Source: g.sel.ID, Munition: "iskander_m", Count: 2}
		x, y := g.m.Project(36.25, 49.99)
		g.strike.Pts = []sim.Pt{{X: x, Y: y}}
		g.cam.Z = 1.0
	case 22:
		g.save(screen, "21_strike_plan")
		g.mode = modeNone
		g.sess.SetSide(1)
		g.view = nil
		g.evSeen, g.evInit = 0, false
	case 26:
		g.save(screen, "30_ukraine")
		g.sess.SetSide(0)
		h := g.sess.(*netplay.Host)
		h.Advance(300, nil)
		x, y := g.m.Project(37.6, 48.1)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 3.0
		g.tab = 0
	case 29:
		g.save(screen, "40_front")
		for _, u := range g.view.Units {
			if u.Type == "s400" {
				g.sel = Selection{Kind: "unit", ID: u.ID}
				break
			}
		}
	case 31:
		g.save(screen, "41_ad_info")
		g.sel = Selection{}
		g.sess.SetSide(0)
		x, y := g.m.Project(48.5, 55.2)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 1.4
	case 34:
		g.save(screen, "50_kazan")
		x, y := g.m.Project(38.5, 55.2)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 2.2
	case 36:
		g.save(screen, "51_moscow")
		x, y := g.m.Project(38.0, 52.5)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 0.35
	case 38:
		g.save(screen, "52_whole_map")
		x, y := g.m.Project(30.52, 50.45)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 12
	case 90:
		g.save(screen, "60_zoom12_kyiv")
		x, y := g.m.Project(36.25, 49.99)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 5
	case 140:
		g.save(screen, "61_zoom5_kharkiv")
		x, y := g.m.Project(33.0, 49.0)
		g.cam.CX, g.cam.CY, g.cam.Z = x, y, 0.8
	case 190:
		g.save(screen, "62_zoom08")
		g.sess.SetSide(1)
		g.view = nil
	case 200:
		g.tab = 2
		g.cycleSelect("unit:buk_ua", g.unitsOfType("buk_ua"))
	case 215:
		g.save(screen, "70_arsenal_click1")
		g.cycleSelect("unit:buk_ua", g.unitsOfType("buk_ua"))
	case 230:
		g.save(screen, "71_arsenal_click2")
		os.Exit(0)
	}
	a.step++
}

// autoSolo — сценарий одиночной игры (DRONEEBLA_SOLO=1): меню, начало, 12 игровых часов, журнал.
func (g *Game) autoSolo(screen *ebiten.Image) {
	a := g.auto
	switch a.step {
	case 0:
		g.save(screen, "s0_menu")
		g.scene = sceneSolo
	case 2:
		g.save(screen, "s1_setup")
		g.startGame(netplay.NewSolo(sim.New(g.cat, g.m, true), 0))
	case 4:
		g.save(screen, "s1b_placement")
	case 6:
		g.save(screen, "s2_start")
		h := g.sess.(*netplay.Host)
		h.Advance(0, func(w *sim.World) { ai.New(g.cat, 0).Place(w) })
		h.Advance(g.cat.Rules.PrepMinutes+720, func(w *sim.World) {
			w.Apply(sim.Command{Kind: sim.CmdPosture, Side: 0, Int: sim.PostureOffense})
		})
		g.tab = 7
	case 9:
		g.save(screen, "s3_journal")
		g.tab = 8
	case 12:
		g.save(screen, "s4_missions")
		g.sess.SetSide(0)
		g.view = nil
		h := g.sess.(*netplay.Host)
		h.Advance(0, func(w *sim.World) {
			w.Apply(sim.Command{Kind: sim.CmdResFund, Side: 0, Int: 3})
			w.Apply(sim.Command{Kind: sim.CmdResearch, Side: 0, Item: "ru_geran2"})
		})
		h.Advance(1500, nil)
		g.tab, g.techOpen = 4, true
	case 15:
		g.save(screen, "s5_tech")
		g.scroll["tech"] = 520
	case 17:
		g.save(screen, "s6_tech_scroll")
		g.scroll["tech"] = 1500
	case 19:
		g.save(screen, "s7_tech_scroll2")
		g.sess.SetSide(1)
		g.view = nil
	case 21:
		g.save(screen, "s8_tech_ua")
		os.Exit(0)
	}
	a.step++
}

// autoLobby — сценарий лобби (DRONEEBLA_LOBBY=1): хост и несколько клиентов.
func (g *Game) autoLobby(screen *ebiten.Image) {
	a := g.auto
	switch a.step {
	case 0:
		w := sim.New(g.cat, g.m, false)
		h, err := netplay.NewHost(w, data.RU, 27990, g.dataHash)
		if err != nil {
			os.Exit(1)
		}
		g.openLobby(h)
		for i, side := range []int{data.UA, data.UA, data.RU} {
			cl, err := netplay.Connect("127.0.0.1:27990", g.dataHash)
			if err != nil {
				os.Exit(1)
			}
			cl.PickSide(side)
			_ = i
		}
	case 6:
		g.save(screen, "l0_lobby")
		os.Exit(0)
	}
	a.step++
}

// autoSettings — снимки экрана настроек и меню в партии при разных масштабах.
func (g *Game) autoSettings(screen *ebiten.Image) {
	a := g.auto
	switch a.step {
	case 0:
		g.save(screen, "c0_menu")
		g.scene = sceneSettings
	case 3:
		g.save(screen, "c1_settings")
		g.setScale(0.75)
	case 6:
		g.save(screen, "c2_settings_75")
		g.setScale(2)
	case 9:
		g.save(screen, "c3_settings_200")
		g.setScale(1)
		g.startGame(netplay.NewSandbox(sim.New(g.cat, g.m, true), 0))
	case 14:
		g.save(screen, "c4_game_100")
		g.menuOpen = true
	case 17:
		g.save(screen, "c5_gamemenu")
		g.menuPage = 1
	case 20:
		g.save(screen, "c6_gamemenu_settings")
		g.menuOpen = false
		g.setScale(0.75)
	case 24:
		g.save(screen, "c7_game_75")
		g.setScale(1.5)
	case 28:
		g.save(screen, "c8_game_150")
		os.Exit(0)
	}
	a.step++
}

// autoPerf — замер: меню, затем партия; в конце каждого этапа печатает расход памяти и время кадра.
func (g *Game) autoPerf() {
	a := g.auto
	report := func(name string) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		var di ebiten.DebugInfo
		ebiten.ReadDebugInfo(&di)
		fmt.Printf("%s: кадр %.1f мс, update %.1f мс | куча %d МБ, ОС %d МБ, сборок %d, шрифтов %d+%d, плиток %d, видеопамять %d МБ | rs %.2f\n",
			name, g.perf.drawMs, g.perf.updMs, ms.HeapAlloc>>20, ms.Sys>>20, ms.NumGC, len(faces), len(boldFaces), g.rend.tiles.count(), di.TotalGPUImageMemoryUsageInBytes>>20, rs)
	}
	switch a.step {
	case 0:
		a.mark = a.frame
		a.step = 1
	case 1:
		if a.frame-a.mark == 120 {
			report("меню, 120 кадров")
		}
		if a.frame-a.mark == 300 {
			report("меню, 300 кадров")
			g.startGame(netplay.NewSandbox(sim.New(g.cat, g.m, true), 0))
			a.mark = a.frame
			a.step = 2
		}
	case 2:
		if a.frame-a.mark == 120 {
			report("партия, 120 кадров")
			x, y := g.m.Project(31.5, 48.0)
			g.cam.CX, g.cam.CY, g.cam.Z = x, y, 3
		}
		if a.frame-a.mark == 240 {
			report("партия, приближение, 240 кадров")
		}
		if a.frame-a.mark == 400 {
			report("партия, 400 кадров")
			os.Exit(0)
		}
	}
}
