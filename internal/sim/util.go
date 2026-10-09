package sim

import (
	"fmt"
	"math"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
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

// TileDir — направление фронта для тайла (Киев, Харьков, Донбасс, Крым): ближайшая опорная точка.
func (w *World) TileDir(i int) int {
	if w.dirMap == nil {
		w.buildDirMap()
	}
	return int(w.dirMap[i])
}

// buildDirMap раскладывает карту по направлениям (диаграмма Вороного по опорным точкам rules.directions).
func (w *World) buildDirMap() {
	anchors := w.cat.Rules.Directions
	pts := make([]Pt, len(anchors))
	for k, a := range anchors {
		pts[k].X, pts[k].Y = w.m.Project(a.Lon, a.Lat)
	}
	w.dirMap = make([]uint8, w.m.W*w.m.H)
	for i := range w.dirMap {
		cx, cy := w.m.TileCenter(i%w.m.W, i/w.m.W)
		w.dirMap[i] = uint8(nearestPt(pts, cx, cy))
	}
	w.dirPts = pts
}

func nearestPt(pts []Pt, x, y float64) int {
	best, bd := 0, math.Inf(1)
	for k, p := range pts {
		if d := dist(x, y, p.X, p.Y); d < bd {
			best, bd = k, d
		}
	}
	return best
}

// PointDir — направление для точки.
func (w *World) PointDir(x, y float64) int {
	if w.dirMap == nil {
		w.buildDirMap()
	}
	if i := w.tileOf(x, y); i >= 0 {
		return int(w.dirMap[i])
	}
	return nearestPt(w.dirPts, x, y)
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

// scale — множитель выпуска здания.
func (b *Building) scale() float64 {
	if b.Scale <= 0 {
		return 1
	}
	return b.Scale
}

// DirAt — направление фронта для точки по карте и опорным точкам (для интерфейса, не зависит от состояния мира).
func DirAt(m *world.MapData, anchors []data.DirAnchor, x, y float64) int {
	pts := make([]Pt, len(anchors))
	for k, a := range anchors {
		pts[k].X, pts[k].Y = m.Project(a.Lon, a.Lat)
	}
	return nearestPt(pts, x, y)
}
