package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// perfStats — счётчики для оверлея F3 и отладочного сценария DRONEEBLA_PERF.
type perfStats struct {
	show   bool
	drawMs float64 // скользящее среднее времени Draw на стороне процессора, мс
	updMs  float64
	ms     runtime.MemStats
	msAt   int
}

func ema(prev, v float64) float64 {
	if prev == 0 {
		return v
	}
	return prev*0.95 + v*0.05
}

// StartProfiling пишет cpu.pprof (60 секунд) и затем heap.pprof в папку dir.
func StartProfiling(dir string) {
	f, err := os.Create(filepath.Join(dir, "cpu.pprof"))
	if err != nil {
		return
	}
	if pprof.StartCPUProfile(f) != nil {
		f.Close()
		return
	}
	go func() {
		time.Sleep(60 * time.Second)
		pprof.StopCPUProfile()
		f.Close()
		if h, err := os.Create(filepath.Join(dir, "heap.pprof")); err == nil {
			runtime.GC()
			pprof.WriteHeapProfile(h)
			h.Close()
		}
	}()
}

// perfKey переключает оверлей по F3.
func (g *Game) perfKey() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF3) {
		g.perf.show = !g.perf.show
	}
}

func (g *Game) memStats() *runtime.MemStats {
	if g.ui.in.tick-g.perf.msAt > 30 || g.perf.msAt == 0 {
		runtime.ReadMemStats(&g.perf.ms)
		g.perf.msAt = g.ui.in.tick
	}
	return &g.perf.ms
}

// drawPerf рисует оверлей F3.
func (g *Game) drawPerf() {
	if !g.perf.show {
		return
	}
	u := &g.ui
	ms := g.memStats()
	b := u.screen.Bounds()
	lines := []string{
		fmt.Sprintf("FPS %.0f  TPS %.0f   Draw %.1f мс  Update %.1f мс", ebiten.ActualFPS(), ebiten.ActualTPS(), g.perf.drawMs, g.perf.updMs),
		fmt.Sprintf("кадр %d×%d  интерфейс %.2f×  DPI %.2f", b.Dx(), b.Dy(), rs, g.dsf),
		fmt.Sprintf("куча Go %d МБ  выделено у ОС %d МБ  сборок мусора %d", ms.HeapAlloc>>20, ms.Sys>>20, ms.NumGC),
		fmt.Sprintf("шрифтов в кэше %d+%d  плиток карты %d", len(faces), len(boldFaces), g.rend.tiles.count()),
	}
	var di ebiten.DebugInfo
	ebiten.ReadDebugInfo(&di)
	lines = append(lines, fmt.Sprintf("видеопамять под картинки %d МБ  графика: %s", di.TotalGPUImageMemoryUsageInBytes>>20, graphicsName(di.GraphicsLibrary)))
	w := 0.0
	for _, l := range lines {
		w = max(w, textWidth(l, 13))
	}
	x, y := 10.0, 70.0
	fillRect(u.screen, x-6, y-6, w+12, float64(len(lines))*17+8, color.RGBA{0, 0, 0, 200})
	for i, l := range lines {
		drawText(u.screen, l, x, y+float64(i)*17, 13, colText, 0)
	}
}

func graphicsName(l ebiten.GraphicsLibrary) string {
	switch l {
	case ebiten.GraphicsLibraryDirectX:
		return "DirectX"
	case ebiten.GraphicsLibraryOpenGL:
		return "OpenGL"
	case ebiten.GraphicsLibraryMetal:
		return "Metal"
	case ebiten.GraphicsLibraryPlayStation5:
		return "PS5"
	}
	return "?"
}
