package sim

import (
	"fmt"
	"math"
)

func dist(ax, ay, bx, by float64) float64 { return math.Hypot(ax-bx, ay-by) }

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func fmtHours(h float64) string {
	if h < 1 {
		return fmt.Sprintf("%.0f мин", h*60)
	}
	return fmt.Sprintf("%.1f ч", h)
}

// FmtTime форматирует игровое время.
func FmtTime(min float64) string {
	d := int(min) / 1440
	h := (int(min) % 1440) / 60
	mm := int(min) % 60
	return fmt.Sprintf("день %d, %02d:%02d", d+1, h, mm)
}

// eff возвращает значение эффекта технологий стороны.
func (s *Side) eff(k string) float64 { return s.Effects[k] }

// frac — доля работоспособности здания.
func (b *Building) frac() float64 {
	if b.Built < 1 || b.MaxHP <= 0 {
		return 0
	}
	return clamp(b.HP/b.MaxHP, 0, 1)
}

// Operational — здание достроено и не разрушено полностью.
func (b *Building) Operational() bool { return b.Built >= 1 && b.HP > b.MaxHP*0.1 }

// Frac — доля HP для интерфейса.
func (b *Building) Frac() float64 { return b.frac() }

// TileDir — направление фронта для тайла.
func (w *World) TileDir(i int) int {
	ty := i / w.m.W
	_, lat := w.m.Unproject(0, (float64(ty)+0.5)*w.m.TileKm)
	return latDir(lat)
}

func latDir(lat float64) int {
	switch {
	case lat >= 49.8:
		return 0
	case lat <= 47.6:
		return 2
	}
	return 1
}

// PointDir — направление для точки.
func (w *World) PointDir(x, y float64) int {
	_, lat := w.m.Unproject(x, y)
	return latDir(lat)
}

// tileOf — индекс тайла точки или -1.
func (w *World) tileOf(x, y float64) int {
	tx, ty := w.m.TileAt(x, y)
	if !w.m.In(tx, ty) {
		return -1
	}
	return w.m.Idx(tx, ty)
}

// sideOfPoint — владелец тайла под точкой или -1.
func (w *World) sideOfPoint(x, y float64) int {
	i := w.tileOf(x, y)
	if i < 0 {
		return -1
	}
	return w.OwnerSide(i)
}

// neighbors4 — соседние тайлы.
func (w *World) neighbors4(i int, f func(j int)) {
	W := w.m.W
	x, y := i%W, i/W
	if x > 0 {
		f(i - 1)
	}
	if x < W-1 {
		f(i + 1)
	}
	if y > 0 {
		f(i - W)
	}
	if y < w.m.H-1 {
		f(i + W)
	}
}

// groundTile — тайл, где может идти наземная война.
func (w *World) groundTile(i int) bool {
	c := w.m.Country[i]
	return w.m.Terrain[i] == 1 && (c == 1 || c == 2) && w.Owner[i] != 0
}
