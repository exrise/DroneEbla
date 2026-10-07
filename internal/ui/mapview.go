package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
)

// Camera — камера карты.
type Camera struct {
	CX, CY float64 // центр, км
	Z      float64 // пикселей на км
	X, Y   int     // область карты на экране
	W, H   int
}

func (c *Camera) ToScreen(x, y float64) (float64, float64) {
	return (x-c.CX)*c.Z + float64(c.X) + float64(c.W)/2, (y-c.CY)*c.Z + float64(c.Y) + float64(c.H)/2
}

func (c *Camera) ToWorld(sx, sy float64) (float64, float64) {
	return (sx-float64(c.X)-float64(c.W)/2)/c.Z + c.CX, (sy-float64(c.Y)-float64(c.H)/2)/c.Z + c.CY
}

// MapRenderer — подложка и слои карты.
type MapRenderer struct {
	m        *world.MapData
	tiles    *tileSet
	ownerImg *ebiten.Image
	fogImg   *ebiten.Image
	ownerPix []byte
	fogPix   []byte
	lastOwn  []uint8
	lastFog  []uint8
	lastPres []uint8
	front    []float32 // отрезки линии фронта x0,y0,x1,y1 в км
	energy   *ebiten.Image
	depImg   *ebiten.Image
}

var (
	colSea     = color.RGBA{168, 196, 214, 255}
	colLand    = color.RGBA{236, 230, 212, 255}
	colBelarus = color.RGBA{222, 222, 206, 255}
	colForeign = color.RGBA{214, 212, 204, 255}
	colRiver   = color.RGBA{92, 140, 186, 255}
	colRail    = color.RGBA{70, 66, 62, 255}
	colRoad    = color.RGBA{196, 160, 120, 255}
	colRegion  = color.RGBA{176, 164, 140, 255}
	colBorder2 = color.RGBA{96, 88, 76, 255}
	colFront   = color.RGBA{150, 20, 20, 255}
)

func newMapRenderer(m *world.MapData) *MapRenderer {
	return &MapRenderer{m: m, tiles: newTileSet(m)}
}

// initLayers создаёт тайловые слои (нужна графика, главный поток).
func (r *MapRenderer) initLayers() {
	m := r.m
	// Месторождения — отдельный слой.
	dep := image.NewRGBA(image.Rect(0, 0, m.W, m.H))
	for i, d := range m.Deposit {
		var c color.RGBA
		switch d {
		case world.DepositOil:
			c = color.RGBA{30, 30, 30, 110}
		case world.DepositGas:
			c = color.RGBA{60, 160, 200, 110}
		case world.DepositCoal:
			c = color.RGBA{90, 70, 50, 110}
		case world.DepositOre:
			c = color.RGBA{170, 70, 40, 110}
		default:
			continue
		}
		dep.SetRGBA(i%m.W, i/m.W, c)
	}
	r.depImg = ebiten.NewImageFromImage(dep)
	r.ownerImg = ebiten.NewImage(m.W, m.H)
	r.fogImg = ebiten.NewImage(m.W, m.H)
	r.energy = ebiten.NewImage(m.W, m.H)
	r.ownerPix = make([]byte, m.W*m.H*4)
	r.fogPix = make([]byte, m.W*m.H*4)
}

func equalBytes(a, b []uint8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// update обновляет слои по представлению.
func (r *MapRenderer) update(v *sim.View) {
	if r.ownerImg == nil {
		r.initLayers()
	}
	m := r.m
	if !equalBytes(v.Owner, r.lastOwn) || !equalBytes(v.Pressure, r.lastPres) {
		for i, o := range v.Owner {
			var c color.RGBA
			switch o {
			case 1:
				c = color.RGBA{200, 60, 50, 46}
			case 2:
				c = color.RGBA{50, 90, 200, 40}
			}
			if m.Terrain[i] != world.TerrainLand {
				c = color.RGBA{}
			}
			if p := v.Pressure[i]; p > 8 && m.Terrain[i] == world.TerrainLand {
				k := float64(p) / 255
				if o == uint8(v.Side+1) {
					// противник давит на наш тайл
					c = color.RGBA{240, 110, 0, uint8(50 + 150*k)}
				} else {
					// наше наступление на тайл противника
					c = color.RGBA{90, 200, 60, uint8(50 + 150*k)}
				}
			}
			// premultiplied alpha
			a := float64(c.A) / 255
			r.ownerPix[i*4] = uint8(float64(c.R) * a)
			r.ownerPix[i*4+1] = uint8(float64(c.G) * a)
			r.ownerPix[i*4+2] = uint8(float64(c.B) * a)
			r.ownerPix[i*4+3] = c.A
		}
		r.ownerImg.WritePixels(r.ownerPix)
		if !equalBytes(v.Owner, r.lastOwn) {
			r.buildFront(v.Owner)
		}
		r.lastOwn = append(r.lastOwn[:0], v.Owner...)
		r.lastPres = append(r.lastPres[:0], v.Pressure...)
	}
	if !equalBytes(v.Fog, r.lastFog) {
		my := uint8(v.Side + 1)
		for i, f := range v.Fog {
			var a uint8
			if v.Owner[i] != my {
				switch f {
				case 0:
					a = 70
				case 1:
					a = 30
				}
			}
			r.fogPix[i*4] = uint8(int(a) * 30 / 255)
			r.fogPix[i*4+1] = uint8(int(a) * 32 / 255)
			r.fogPix[i*4+2] = uint8(int(a) * 40 / 255)
			r.fogPix[i*4+3] = a
		}
		r.fogImg.WritePixels(r.fogPix)
		r.lastFog = append(r.lastFog[:0], v.Fog...)
	}
}

// buildFront строит линию фронта по краям тайлов.
func (r *MapRenderer) buildFront(owner []uint8) {
	m := r.m
	r.front = r.front[:0]
	ground := func(i int) bool {
		c := m.Country[i]
		return m.Terrain[i] == world.TerrainLand && (c == world.CountryUkraine || c == world.CountryRussia) && owner[i] != 0
	}
	ts := float32(m.TileKm)
	for ty := 0; ty < m.H; ty++ {
		for tx := 0; tx < m.W; tx++ {
			i := m.Idx(tx, ty)
			if !ground(i) {
				continue
			}
			if tx+1 < m.W && ground(i+1) && owner[i+1] != owner[i] {
				r.front = append(r.front, float32(tx+1)*ts, float32(ty)*ts, float32(tx+1)*ts, float32(ty+1)*ts)
			}
			if ty+1 < m.H && ground(i+m.W) && owner[i+m.W] != owner[i] {
				r.front = append(r.front, float32(tx)*ts, float32(ty+1)*ts, float32(tx+1)*ts, float32(ty+1)*ts)
			}
		}
	}
}

// drawBase рисует подложку и тайловые слои.
func (r *MapRenderer) drawBase(dst *ebiten.Image, cam *Camera, layers map[string]bool, v *sim.View) {
	if r.ownerImg == nil {
		r.initLayers()
	}
	ox, oy := cam.ToScreen(0, 0)
	r.tiles.draw(dst, cam)
	tile := func(img *ebiten.Image) {
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
		op.GeoM.Scale(cam.Z*r.m.TileKm, cam.Z*r.m.TileKm)
		op.GeoM.Translate(ox, oy)
		dst.DrawImage(img, op)
	}
	if layers["deposits"] {
		tile(r.depImg)
	}
	tile(r.ownerImg)
	if layers["energy"] && v != nil {
		r.drawEnergy(v)
		tile(r.energy)
	}
	if layers["fog"] {
		tile(r.fogImg)
	}
	// Линия фронта.
	w := float32(math.Max(2, math.Min(4, cam.Z*1.2)))
	for k := 0; k+3 < len(r.front); k += 4 {
		x0, y0 := cam.ToScreen(float64(r.front[k]), float64(r.front[k+1]))
		x1, y1 := cam.ToScreen(float64(r.front[k+2]), float64(r.front[k+3]))
		if math.Max(x0, x1) < float64(cam.X) || math.Min(x0, x1) > float64(cam.X+cam.W) || math.Max(y0, y1) < float64(cam.Y) || math.Min(y0, y1) > float64(cam.Y+cam.H) {
			continue
		}
		// удлиняем на половину толщины, чтобы углы смыкались
		dx, dy := x1-x0, y1-y0
		l := math.Hypot(dx, dy)
		if l > 0 {
			ex, ey := dx/l*float64(w)/2, dy/l*float64(w)/2
			x0, y0, x1, y1 = x0-ex, y0-ey, x1+ex, y1+ey
		}
		vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), w, colFront, false)
	}
}

// drawEnergy — обеспеченность энергией своих областей.
func (r *MapRenderer) drawEnergy(v *sim.View) {
	pix := make([]byte, r.m.W*r.m.H*4)
	my := uint8(v.Side + 1)
	est := map[int]float64{}
	for _, e := range v.EnemyPower {
		est[e.Region] = e.Frac
	}
	for i, reg := range r.m.Region {
		if reg < 0 {
			continue
		}
		var f float64
		var ok, enemy bool
		switch v.Owner[i] {
		case my:
			f, ok = v.RegionPower[int(reg)]
		case 3 - my:
			f, ok = est[int(reg)]
			enemy = true
		}
		if !ok {
			continue
		}
		var c color.RGBA
		switch {
		case f < 0.7:
			c = color.RGBA{230, 40, 30, 110}
		case f < 0.99:
			c = color.RGBA{240, 180, 30, 90}
		default:
			c = color.RGBA{60, 180, 80, 50}
		}
		if enemy { // оценка по разведданным — бледнее и с фиолетовым оттенком
			c.R = uint8((int(c.R) + 120) / 2)
			c.B = 150
			c.A = c.A * 3 / 4
		}
		a := float64(c.A) / 255
		pix[i*4] = uint8(float64(c.R) * a)
		pix[i*4+1] = uint8(float64(c.G) * a)
		pix[i*4+2] = uint8(float64(c.B) * a)
		pix[i*4+3] = c.A
	}
	r.energy.WritePixels(pix)
}

// ageText — сколько назад.
func ageText(now, seen float64) string {
	if seen < 0 {
		return "довоенные данные"
	}
	d := now - seen
	switch {
	case d < 2:
		return "сейчас"
	case d < 60:
		return fmt.Sprintf("%.0f мин назад", d)
	default:
		return fmt.Sprintf("%.1f ч назад", d/60)
	}
}
