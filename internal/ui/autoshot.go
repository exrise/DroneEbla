package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/exrise/droneebla/internal/ai"
	"github.com/exrise/droneebla/internal/netplay"
	"github.com/exrise/droneebla/internal/sim"
)

// Отладка: DRONEEBLA_SHOT=<папка> — игра сама запускает песочницу,
// делает снимки экрана и выходит. Нужна для проверки интерфейса без монитора.

type autoShot struct {
	dir   string
	frame int
	step  int
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
		g.sess.SetSide(1)
		g.tab = 8
	case 15:
		os.Exit(0)
	}
	a.step++
}
