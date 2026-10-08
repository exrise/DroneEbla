package ui

import (
	"image"
	"image/color"
	"math"
	"runtime"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	xvector "golang.org/x/image/vector"

	"github.com/exrise/droneebla/internal/world"
)

// Подложка карты строится плитками на нескольких масштабах (уровнях
// детализации). Нужный уровень выбирается по текущему зуму; плитки рисуются
// в фоновых потоках и загружаются в видеопамять по мере готовности. Пока
// плитка не готова, показывается более грубая.

const (
	tilePx     = 512 // сторона плитки в пикселях без полей
	tileMargin = 1   // поле вокруг плитки, чтобы не было швов при масштабировании
	maxTiles   = 120 // предел плиток в видеопамяти
)

// lodScales — пикселей на км для каждого уровня. Уровень выбирается так,
// чтобы плитка растягивалась не более чем в 1.3 раза.
var lodScales = []float64{1.0, 1.8, 3.2, 6.0, 14.0, 28.0}

type seg struct{ x0, y0, x1, y1 float32 } // километры

// Виды линий подложки в порядке отрисовки.
const (
	kRegion = iota
	kRoad
	kRiver
	kCoast
	kRail
	kBorder
	numKinds
)

var lineStyle = [numKinds]struct {
	c color.RGBA
	w float64
}{
	kRegion: {colRegion, 1.2},
	kRoad:   {colRoad, 1.0},
	kRiver:  {colRiver, 1.6},
	kCoast:  {colRiver, 1.0},
	kRail:   {colRail, 1.1},
	kBorder: {colBorder2, 2.2},
}

type lodKey struct{ level, tx, ty int16 }

type readyTile struct {
	key lodKey
	img *image.RGBA
}

type tileSet struct {
	m    *world.MapData
	segs [numKinds][]seg

	mu      sync.Mutex
	stack   []lodKey // самые свежие запросы обрабатываются первыми
	low     []lodKey // предзагрузка обзорного уровня
	pending map[lodKey]bool
	wake    chan struct{}
	ready   chan readyTile

	tiles map[lodKey]*ebiten.Image
	used  map[lodKey]int
	frame int
}

func newTileSet(m *world.MapData) *tileSet {
	t := &tileSet{
		m:       m,
		pending: map[lodKey]bool{},
		wake:    make(chan struct{}, 256),
		ready:   make(chan readyTile, 64),
		tiles:   map[lodKey]*ebiten.Image{},
		used:    map[lodKey]int{},
	}
	t.collectSegments()
	n := runtime.NumCPU() - 1
	n = max(1, min(3, n))
	for i := 0; i < n; i++ {
		go t.worker()
	}
	// Обзорный уровень готовим заранее.
	for ty := 0; ty < t.tilesY(0); ty++ {
		for tx := 0; tx < t.tilesX(0); tx++ {
			k := lodKey{0, int16(tx), int16(ty)}
			t.pending[k] = true
			t.low = append(t.low, k)
			t.wake <- struct{}{}
		}
	}
	return t
}

func (t *tileSet) tilesX(level int) int {
	return int(math.Ceil(t.m.WidthKm() * lodScales[level] / tilePx))
}

func (t *tileSet) tilesY(level int) int {
	return int(math.Ceil(t.m.HeightKm() * lodScales[level] / tilePx))
}

// collectSegments раскладывает линии карты по видам (в километрах).
func (t *tileSet) collectSegments() {
	m := t.m
	ts := float32(m.TileKm)
	for ty := 0; ty < m.H; ty++ {
		for tx := 0; tx < m.W; tx++ {
			i := m.Idx(tx, ty)
			a := m.Region[i]
			if a < 0 {
				continue
			}
			if tx+1 < m.W {
				if b := m.Region[i+1]; b >= 0 && b != a && m.Country[i] == m.Country[i+1] {
					t.segs[kRegion] = append(t.segs[kRegion], seg{float32(tx+1) * ts, float32(ty) * ts, float32(tx+1) * ts, float32(ty+1) * ts})
				}
			}
			if ty+1 < m.H {
				if b := m.Region[i+m.W]; b >= 0 && b != a && m.Country[i] == m.Country[i+m.W] {
					t.segs[kRegion] = append(t.segs[kRegion], seg{float32(tx) * ts, float32(ty+1) * ts, float32(tx+1) * ts, float32(ty+1) * ts})
				}
			}
		}
	}
	kind := map[int]int{world.LineRoad: kRoad, world.LineRail: kRail, world.LineRiver: kRiver, world.LineCoast: kCoast, world.LineCountryBorder: kBorder}
	for _, l := range m.Lines {
		k := kind[l.Kind]
		for j := 2; j+1 < len(l.Pts); j += 2 {
			t.segs[k] = append(t.segs[k], seg{l.Pts[j-2], l.Pts[j-1], l.Pts[j], l.Pts[j+1]})
		}
	}
}

// request ставит плитку в очередь, если её ещё нет.
func (t *tileSet) request(k lodKey) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending[k] {
		return
	}
	t.pending[k] = true
	t.stack = append(t.stack, k)
	// Игрок быстро листает карту: самые старые запросы уже не нужны.
	if len(t.stack) > 48 {
		for _, old := range t.stack[:24] {
			delete(t.pending, old)
		}
		t.stack = append([]lodKey{}, t.stack[24:]...)
	}
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

func (t *tileSet) next() (lodKey, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n := len(t.stack); n > 0 {
		k := t.stack[n-1]
		t.stack = t.stack[:n-1]
		return k, true
	}
	if n := len(t.low); n > 0 {
		k := t.low[0]
		t.low = t.low[1:]
		return k, true
	}
	return lodKey{}, false
}

func (t *tileSet) worker() {
	sz := tilePx + 2*tileMargin
	ras := xvector.NewRasterizer(sz, sz)
	for {
		k, ok := t.next()
		if !ok {
			<-t.wake
			continue
		}
		img := t.render(k, ras)
		t.ready <- readyTile{k, img}
	}
}

// render рисует одну плитку на CPU.
func (t *tileSet) render(k lodKey, ras *xvector.Rasterizer) *image.RGBA {
	m := t.m
	S := lodScales[k.level]
	sz := tilePx + 2*tileMargin
	img := image.NewRGBA(image.Rect(0, 0, sz, sz))
	// пиксель (i, j) плитки соответствует точке карты ((tx*tilePx+i-margin)/S, ...)
	ox := float64(int(k.tx)*tilePx - tileMargin)
	oy := float64(int(k.ty)*tilePx - tileMargin)
	ts := m.TileKm
	edge := S * ts // пикселей на тайл: ширина полосы сглаживания берега

	land := func(tx, ty int) bool {
		return m.In(tx, ty) && m.Terrain[m.Idx(tx, ty)] == world.TerrainLand
	}
	landColor := func(tx, ty int) color.RGBA {
		if !m.In(tx, ty) {
			return colLand
		}
		switch m.Country[m.Idx(tx, ty)] {
		case world.CountryBelarus:
			return colBelarus
		case world.CountryForeign:
			return colForeign
		}
		return colLand
	}
	// Для каждого столбца и строки: индекс левого/верхнего тайла и вес.
	type axis struct {
		i    int
		f    float64
		near int
	}
	cols := make([]axis, sz)
	rows := make([]axis, sz)
	for i := 0; i < sz; i++ {
		g := (ox+float64(i)+0.5)/S/ts - 0.5
		ix := int(math.Floor(g))
		cols[i] = axis{ix, g - float64(ix), int(math.Floor(g + 0.5))}
		g = (oy+float64(i)+0.5)/S/ts - 0.5
		iy := int(math.Floor(g))
		rows[i] = axis{iy, g - float64(iy), int(math.Floor(g + 0.5))}
	}
	lerp := func(a, b uint8, w float64) uint8 { return uint8(float64(a)*(1-w) + float64(b)*w + 0.5) }
	for j := 0; j < sz; j++ {
		ry := rows[j]
		for i := 0; i < sz; i++ {
			cx := cols[i]
			var l00, l10, l01, l11 float64
			if land(cx.i, ry.i) {
				l00 = 1
			}
			if land(cx.i+1, ry.i) {
				l10 = 1
			}
			if land(cx.i, ry.i+1) {
				l01 = 1
			}
			if land(cx.i+1, ry.i+1) {
				l11 = 1
			}
			frac := (l00*(1-cx.f)+l10*cx.f)*(1-ry.f) + (l01*(1-cx.f)+l11*cx.f)*ry.f
			w := math.Max(0, math.Min(1, 0.5+(frac-0.5)*edge))
			lc := landColor(cx.near, ry.near)
			if !land(cx.near, ry.near) {
				// берег: берём цвет ближайшего соседнего тайла суши
				for _, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
					if land(cx.i+d[0], ry.i+d[1]) {
						lc = landColor(cx.i+d[0], ry.i+d[1])
						break
					}
				}
			}
			p := img.Pix[j*img.Stride+i*4:]
			p[0], p[1], p[2], p[3] = lerp(colSea.R, lc.R, w), lerp(colSea.G, lc.G, w), lerp(colSea.B, lc.B, w), 255
		}
	}
	// Линии: отбираем отрезки, попадающие в плитку.
	x0, y0 := ox/S-3, oy/S-3
	x1, y1 := (ox+float64(sz))/S+3, (oy+float64(sz))/S+3
	for kind := 0; kind < numKinds; kind++ {
		st := lineStyle[kind]
		h := float32(st.w / 2)
		ras.Reset(sz, sz)
		n := 0
		for _, s := range t.segs[kind] {
			if float64(max(s.x0, s.x1)) < x0 || float64(min(s.x0, s.x1)) > x1 || float64(max(s.y0, s.y1)) < y0 || float64(min(s.y0, s.y1)) > y1 {
				continue
			}
			ax, ay := float32(float64(s.x0)*S-ox), float32(float64(s.y0)*S-oy)
			bx, by := float32(float64(s.x1)*S-ox), float32(float64(s.y1)*S-oy)
			dx, dy := bx-ax, by-ay
			l := float32(math.Hypot(float64(dx), float64(dy)))
			if l < 0.01 {
				continue
			}
			ux, uy := dx/l*h, dy/l*h
			nx, ny := -uy, ux
			ras.MoveTo(ax-ux+nx, ay-uy+ny)
			ras.LineTo(bx+ux+nx, by+uy+ny)
			ras.LineTo(bx+ux-nx, by+uy-ny)
			ras.LineTo(ax-ux-nx, ay-uy-ny)
			ras.ClosePath()
			n++
		}
		if n > 0 {
			ras.Draw(img, img.Bounds(), image.NewUniform(st.c), image.Point{})
		}
	}
	return img
}

// pump забирает готовые плитки и загружает их в видеопамять.
func (t *tileSet) pump() {
	for n := 0; n < 4; n++ {
		select {
		case r := <-t.ready:
			t.mu.Lock()
			delete(t.pending, r.key)
			t.mu.Unlock()
			t.tiles[r.key] = ebiten.NewImageFromImage(r.img)
			t.used[r.key] = t.frame
		default:
			n = 4
		}
	}
	if len(t.tiles) > maxTiles {
		t.evict()
	}
}

// evict выгружает давно не нужные плитки (обзорный уровень не трогаем).
func (t *tileSet) evict() {
	for len(t.tiles) > maxTiles-10 {
		var oldest lodKey
		best := math.MaxInt
		for k := range t.tiles {
			if k.level == 0 || t.frame-t.used[k] < 3 {
				continue
			}
			if t.used[k] < best {
				best, oldest = t.used[k], k
			}
		}
		if best == math.MaxInt {
			return
		}
		t.tiles[oldest].Deallocate()
		delete(t.tiles, oldest)
		delete(t.used, oldest)
	}
}

// levelFor выбирает уровень детализации по зуму.
func levelFor(z float64) int {
	for i, s := range lodScales {
		if s >= z/1.3 {
			return i
		}
	}
	return len(lodScales) - 1
}

func (t *tileSet) drawTile(dst *ebiten.Image, cam *Camera, k lodKey, img *ebiten.Image) {
	S := lodScales[k.level]
	sub := img.SubImage(image.Rect(tileMargin, tileMargin, tileMargin+tilePx, tileMargin+tilePx)).(*ebiten.Image)
	sx, sy := cam.ToScreen(float64(int(k.tx)*tilePx)/S, float64(int(k.ty)*tilePx)/S)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	k2 := cam.Z / S
	op.GeoM.Scale(k2, k2)
	op.GeoM.Translate(sx, sy)
	op.GeoM.Scale(rs, rs)
	dst.DrawImage(sub, op)
}

// draw рисует видимую часть подложки.
func (t *tileSet) draw(dst *ebiten.Image, cam *Camera) {
	t.frame++
	t.pump()
	level := levelFor(cam.Z * rs) // детализация по физическим пикселям
	S := lodScales[level]
	wx0, wy0 := cam.ToWorld(float64(cam.X), float64(cam.Y))
	wx1, wy1 := cam.ToWorld(float64(cam.X+cam.W), float64(cam.Y+cam.H))
	tx0 := max(0, int(math.Floor(wx0*S/tilePx)))
	ty0 := max(0, int(math.Floor(wy0*S/tilePx)))
	tx1 := min(t.tilesX(level)-1, int(math.Floor(wx1*S/tilePx)))
	ty1 := min(t.tilesY(level)-1, int(math.Floor(wy1*S/tilePx)))

	var missing, have []lodKey
	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			k := lodKey{int16(level), int16(tx), int16(ty)}
			if _, ok := t.tiles[k]; ok {
				have = append(have, k)
			} else {
				missing = append(missing, k)
			}
		}
	}
	// Сначала грубые подмены для ещё не готовых плиток, поверх — готовые.
	drawn := map[lodKey]bool{}
	for _, k := range missing {
		t.request(k)
		cx := (float64(int(k.tx)*tilePx) + tilePx/2) / S
		cy := (float64(int(k.ty)*tilePx) + tilePx/2) / S
		for lv := level - 1; lv >= 0; lv-- {
			s2 := lodScales[lv]
			fk := lodKey{int16(lv), int16(math.Floor(cx * s2 / tilePx)), int16(math.Floor(cy * s2 / tilePx))}
			if img, ok := t.tiles[fk]; ok {
				if !drawn[fk] {
					drawn[fk] = true
					t.drawTile(dst, cam, fk, img)
				}
				t.used[fk] = t.frame
				break
			}
		}
	}
	for _, k := range have {
		t.used[k] = t.frame
		t.drawTile(dst, cam, k, t.tiles[k])
	}
}
