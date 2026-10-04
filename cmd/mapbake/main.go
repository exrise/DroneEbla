// mapbake — офлайн-утилита: превращает слои Natural Earth (GeoJSON) в
// тайловую карту internal/world/map.bin. Нужна только разработчику;
// игре для работы она не требуется.
//
//	go run ./cmd/mapbake -ne <папка с ne_10m_*.geojson>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/exrise/droneebla/internal/world"
)

const (
	lonMin, lonMax = 22.0, 41.6
	latMin, latMax = 44.2, 54.3
	tileKm         = 5.0
	refLat         = 49.0
)

type geom struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type feature struct {
	Properties map[string]any `json:"properties"`
	Geometry   geom           `json:"geometry"`
}

type fc struct {
	Features []feature `json:"features"`
}

type pt struct{ X, Y float64 }
type ring []pt
type poly []ring // первое кольцо — внешнее, остальные — дыры

var m *world.MapData

func load(dir, name string) fc {
	b, err := os.ReadFile(filepath.Join(dir, name+".geojson"))
	if err != nil {
		log.Fatal(err)
	}
	var f fc
	if err := json.Unmarshal(b, &f); err != nil {
		log.Fatal(name, err)
	}
	return f
}

func proj(c []float64) pt {
	x, y := m.Project(c[0], c[1])
	return pt{x, y}
}

func polys(g geom) []poly {
	var out []poly
	conv := func(rs [][][]float64) poly {
		var p poly
		for _, r := range rs {
			var rr ring
			for _, c := range r {
				rr = append(rr, proj(c))
			}
			p = append(p, rr)
		}
		return p
	}
	switch g.Type {
	case "Polygon":
		var c [][][]float64
		json.Unmarshal(g.Coordinates, &c)
		out = append(out, conv(c))
	case "MultiPolygon":
		var c [][][][]float64
		json.Unmarshal(g.Coordinates, &c)
		for _, p := range c {
			out = append(out, conv(p))
		}
	}
	return out
}

func lines(g geom) [][]pt {
	var out [][]pt
	conv := func(l [][]float64) []pt {
		var r []pt
		for _, c := range l {
			r = append(r, proj(c))
		}
		return r
	}
	switch g.Type {
	case "LineString":
		var c [][]float64
		json.Unmarshal(g.Coordinates, &c)
		out = append(out, conv(c))
	case "MultiLineString":
		var c [][][]float64
		json.Unmarshal(g.Coordinates, &c)
		for _, l := range c {
			out = append(out, conv(l))
		}
	}
	return out
}

func inRing(r ring, x, y float64) bool {
	in := false
	for i, j := 0, len(r)-1; i < len(r); j, i = i, i+1 {
		a, b := r[i], r[j]
		if (a.Y > y) != (b.Y > y) && x < (b.X-a.X)*(y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

func inPoly(p poly, x, y float64) bool {
	if len(p) == 0 || !inRing(p[0], x, y) {
		return false
	}
	for _, h := range p[1:] {
		if inRing(h, x, y) {
			return false
		}
	}
	return true
}

func bbox(p poly) (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, q := range p[0] {
		x0, y0 = math.Min(x0, q.X), math.Min(y0, q.Y)
		x1, y1 = math.Max(x1, q.X), math.Max(y1, q.Y)
	}
	return
}

// fill вызывает f для каждого тайла, центр которого внутри полигона.
func fill(p poly, f func(i int)) {
	x0, y0, x1, y1 := bbox(p)
	tx0, ty0 := int(math.Max(0, math.Floor(x0/tileKm))), int(math.Max(0, math.Floor(y0/tileKm)))
	tx1, ty1 := int(math.Min(float64(m.W-1), math.Ceil(x1/tileKm))), int(math.Min(float64(m.H-1), math.Ceil(y1/tileKm)))
	for ty := ty0; ty <= ty1; ty++ {
		for tx := tx0; tx <= tx1; tx++ {
			cx, cy := m.TileCenter(tx, ty)
			if inPoly(p, cx, cy) {
				f(m.Idx(tx, ty))
			}
		}
	}
}

// trace помечает тайлы вдоль ломаной.
func trace(l []pt, f func(i int)) {
	for k := 1; k < len(l); k++ {
		a, b := l[k-1], l[k]
		d := math.Hypot(b.X-a.X, b.Y-a.Y)
		n := int(d/(tileKm/3)) + 1
		for s := 0; s <= n; s++ {
			t := float64(s) / float64(n)
			tx, ty := m.TileAt(a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t)
			if m.In(tx, ty) {
				f(m.Idx(tx, ty))
			}
		}
	}
}

// simplify — алгоритм Дугласа—Пекера.
func simplify(l []pt, eps float64) []pt {
	if len(l) < 3 {
		return l
	}
	a, b := l[0], l[len(l)-1]
	idx, dmax := 0, 0.0
	for i := 1; i < len(l)-1; i++ {
		d := segDist(l[i], a, b)
		if d > dmax {
			idx, dmax = i, d
		}
	}
	if dmax <= eps {
		return []pt{a, b}
	}
	left := simplify(l[:idx+1], eps)
	right := simplify(l[idx:], eps)
	return append(left[:len(left)-1], right...)
}

func segDist(p, a, b pt) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/l2))
	return math.Hypot(p.X-a.X-t*dx, p.Y-a.Y-t*dy)
}

// clip оставляет только участки ломаной внутри карты (с запасом).
func clip(l []pt) [][]pt {
	var out [][]pt
	var cur []pt
	W, H := m.WidthKm(), m.HeightKm()
	for _, p := range l {
		in := p.X > -20 && p.Y > -20 && p.X < W+20 && p.Y < H+20
		if in {
			cur = append(cur, p)
		} else if len(cur) > 0 {
			cur = append(cur, p)
			out = append(out, cur)
			cur = nil
		}
	}
	if len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

func addLine(kind int, l []pt, eps float64) {
	for _, c := range clip(l) {
		s := simplify(c, eps)
		if len(s) < 2 {
			continue
		}
		fl := make([]float32, 0, len(s)*2)
		for _, p := range s {
			fl = append(fl, float32(p.X), float32(p.Y))
		}
		m.Lines = append(m.Lines, world.Line{Kind: kind, Pts: fl})
	}
}

func str(p map[string]any, k string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}

func num(p map[string]any, k string) float64 {
	if v, ok := p[k].(float64); ok {
		return v
	}
	return 0
}

// Линия соприкосновения на 23.02.2022: районы Донецкой и Луганской
// областей восточнее этой линии принадлежат РФ на старте.
var ordlo = []pt{
	{39.48, 48.63}, {39.23, 48.70}, {38.90, 48.68}, {38.55, 48.65}, {38.45, 48.42}, {38.28, 48.39}, {38.10, 48.36},
	{37.93, 48.30}, {37.82, 48.15}, {37.58, 47.99}, {37.50, 47.86},
	{37.70, 47.66}, {37.86, 47.40}, {37.80, 47.09}, {37.75, 46.90}, {39.00, 46.80},
	{40.50, 47.40}, {40.50, 49.10}, {39.90, 49.00},
}

type deposit struct {
	lon, lat, r float64
	kind        uint8
}

// Крупные месторождения (упрощённо, круги).
var deposits = []deposit{
	{37.9, 48.1, 70, world.DepositCoal}, // Донбасс
	{39.3, 48.4, 50, world.DepositCoal}, // Луганский угольный район
	{40.2, 47.7, 40, world.DepositCoal}, // Восточный Донбасс (Шахты)
	{24.3, 50.4, 35, world.DepositCoal}, // Львовско-Волынский бассейн
	{33.4, 47.9, 30, world.DepositOre},  // Кривой Рог
	{33.6, 49.1, 20, world.DepositOre},  // Горишние Плавни
	{37.7, 51.2, 35, world.DepositOre},  // КМА: Старый Оскол, Губкин
	{35.4, 52.3, 25, world.DepositOre},  // КМА: Железногорск
	{34.9, 50.3, 35, world.DepositOil},  // Охтырка
	{33.0, 50.6, 30, world.DepositOil},  // Прилуки / Гнединцы
	{23.4, 49.3, 25, world.DepositOil},  // Борислав
	{38.6, 45.0, 45, world.DepositOil},  // Кубань (Абинск, Крымск)
	{39.8, 44.8, 30, world.DepositOil},  // Майкоп
	{36.6, 49.5, 30, world.DepositGas},  // Шебелинка
	{34.3, 49.4, 30, world.DepositGas},  // Полтавские месторождения
	{40.4, 45.8, 30, world.DepositGas},  // Кубанские газовые
	{24.0, 49.6, 20, world.DepositGas},  // Прикарпатье
}

func main() {
	dir := flag.String("ne", "", "папка с GeoJSON Natural Earth")
	out := flag.String("o", "internal/world/map.bin", "выходной файл")
	flag.Parse()
	if *dir == "" {
		log.Fatal("укажите -ne")
	}
	m = &world.MapData{
		TileKm: tileKm, Lon0: lonMin, Lat0: latMax,
		KX: 111.32 * math.Cos(refLat*math.Pi/180), KY: 110.57,
	}
	m.W = int(math.Ceil((lonMax - lonMin) * m.KX / tileKm))
	m.H = int(math.Ceil((latMax - latMin) * m.KY / tileKm))
	n := m.W * m.H
	m.Terrain = make([]uint8, n)
	m.Country = make([]uint8, n)
	m.Region = make([]int16, n)
	m.Deposit = make([]uint8, n)
	m.Flags = make([]uint8, n)
	m.Initial = make([]uint8, n)
	for i := range m.Region {
		m.Region[i] = -1
	}
	log.Printf("сетка %dx%d (%d тайлов)", m.W, m.H, n)

	// Страны.
	countries := load(*dir, "ne_10m_admin_0_countries")
	for _, f := range countries.Features {
		a3 := str(f.Properties, "ADM0_A3")
		var c uint8
		switch a3 {
		case "UKR":
			c = world.CountryUkraine
		case "RUS":
			c = world.CountryRussia
		case "BLR":
			c = world.CountryBelarus
		default:
			c = world.CountryForeign
		}
		for _, p := range polys(f.Geometry) {
			x0, y0, x1, y1 := bbox(p)
			if x1 < -50 || y1 < -50 || x0 > m.WidthKm()+50 || y0 > m.HeightKm()+50 {
				continue
			}
			fill(p, func(i int) {
				m.Terrain[i] = world.TerrainLand
				m.Country[i] = c
			})
			if c == world.CountryUkraine || c == world.CountryRussia || c == world.CountryBelarus {
				for _, r := range p {
					l := make([]pt, len(r))
					copy(l, r)
					addLine(world.LineCountryBorder, l, 1.0)
				}
			}
		}
	}

	// Области.
	provinces := load(*dir, "ne_10m_admin_1_states_provinces")
	regionIdx := map[string]int{}
	for _, f := range provinces.Features {
		a3 := str(f.Properties, "adm0_a3")
		if a3 != "UKR" && a3 != "RUS" {
			continue
		}
		iso := str(f.Properties, "iso_3166_2")
		name := str(f.Properties, "name_ru")
		ps := polys(f.Geometry)
		any := false
		for _, p := range ps {
			fill(p, func(i int) {
				if m.Terrain[i] != world.TerrainLand {
					return
				}
				id, ok := regionIdx[iso]
				if !ok {
					id = len(m.Regions)
					regionIdx[iso] = id
					c := world.CountryRussia
					if a3 == "UKR" || iso == "UA-43" || iso == "UA-40" {
						c = world.CountryUkraine
					}
					m.Regions = append(m.Regions, world.Region{Name: name, Country: c})
				}
				m.Region[i] = int16(id)
				any = true
			})
		}
		_ = any
	}
	// Крым и Севастополь в Natural Earth отнесены к РФ; для правил игры
	// это территория Украины, занятая РФ на старте.
	for i := range m.Region {
		r := m.Region[i]
		if r >= 0 && m.Regions[r].Country == world.CountryUkraine {
			m.Country[i] = world.CountryUkraine
		}
	}
	// Центры областей.
	sx := make([]float64, len(m.Regions))
	sy := make([]float64, len(m.Regions))
	cnt := make([]float64, len(m.Regions))
	for ty := 0; ty < m.H; ty++ {
		for tx := 0; tx < m.W; tx++ {
			r := m.Region[m.Idx(tx, ty)]
			if r >= 0 {
				cx, cy := m.TileCenter(tx, ty)
				sx[r] += cx
				sy[r] += cy
				cnt[r]++
			}
		}
	}
	for i := range m.Regions {
		if cnt[i] > 0 {
			m.Regions[i].CX, m.Regions[i].CY = sx[i]/cnt[i], sy[i]/cnt[i]
		}
	}

	// Озёра и водохранилища.
	lakes := load(*dir, "ne_10m_lakes")
	for _, f := range lakes.Features {
		for _, p := range polys(f.Geometry) {
			fill(p, func(i int) { m.Terrain[i] = world.TerrainLake })
			for _, r := range p {
				addLine(world.LineCoast, r, 0.8)
			}
		}
	}

	// Береговая линия: из границ суши.
	// Рисуется клиентом по маске тайлов, поэтому отдельно не храним.

	// Реки.
	rivers := load(*dir, "ne_10m_rivers_lake_centerlines")
	major := map[string]bool{"Dnipro": true, "Desna": true, "Donets": true, "Dniester": true,
		"Southern Bug": true, "Don": true, "Pripyat": true, "Seym": true, "Kuban": true, "Prut": true, "Danube": true}
	for _, f := range rivers.Features {
		name := str(f.Properties, "name")
		sr := num(f.Properties, "scalerank")
		if sr > 9 {
			continue
		}
		for _, l := range lines(f.Geometry) {
			addLine(world.LineRiver, l, 0.8)
			if major[name] {
				trace(l, func(i int) { m.Flags[i] |= world.FlagRiver })
			}
		}
	}

	// Железные дороги.
	rails := load(*dir, "ne_10m_railroads")
	for _, f := range rails.Features {
		for _, l := range lines(f.Geometry) {
			ok := false
			for _, p := range l {
				if p.X > -20 && p.Y > -20 && p.X < m.WidthKm()+20 && p.Y < m.HeightKm()+20 {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
			addLine(world.LineRail, l, 0.8)
			trace(l, func(i int) { m.Flags[i] |= world.FlagRail })
		}
	}

	// Дороги.
	roads := load(*dir, "ne_10m_roads")
	for _, f := range roads.Features {
		t := str(f.Properties, "type")
		if t != "Major Highway" && t != "Secondary Highway" && t != "Road" {
			continue
		}
		for _, l := range lines(f.Geometry) {
			ok := false
			for _, p := range l {
				if p.X > -20 && p.Y > -20 && p.X < m.WidthKm()+20 && p.Y < m.HeightKm()+20 {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
			addLine(world.LineRoad, l, 1.0)
			trace(l, func(i int) { m.Flags[i] |= world.FlagRoad })
		}
	}

	// Города.
	places := load(*dir, "ne_10m_populated_places")
	for _, f := range places.Features {
		a3 := str(f.Properties, "ADM0_A3")
		if a3 != "UKR" && a3 != "RUS" {
			continue
		}
		var c []float64
		json.Unmarshal(f.Geometry.Coordinates, &c)
		if c[0] < lonMin || c[0] > lonMax || c[1] < latMin || c[1] > latMax {
			continue
		}
		x, y := m.Project(c[0], c[1])
		tx, ty := m.TileAt(x, y)
		if !m.In(tx, ty) {
			continue
		}
		i := m.Idx(tx, ty)
		name := str(f.Properties, "NAME_RU")
		if name == "" {
			name = str(f.Properties, "NAME")
		}
		pop := int(num(f.Properties, "POP_MAX"))
		m.Cities = append(m.Cities, world.City{
			Name: name, X: x, Y: y, Pop: pop, Region: int(m.Region[i]),
			Capital: num(f.Properties, "ADM0CAP") == 1,
		})
		m.Flags[i] |= world.FlagUrban
		if pop > 400000 {
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if m.In(tx+dx, ty+dy) {
						m.Flags[m.Idx(tx+dx, ty+dy)] |= world.FlagUrban
					}
				}
			}
		}
	}
	sort.Slice(m.Cities, func(a, b int) bool { return m.Cities[a].Pop > m.Cities[b].Pop })

	// Месторождения.
	for _, d := range deposits {
		cx, cy := m.Project(d.lon, d.lat)
		for ty := 0; ty < m.H; ty++ {
			for tx := 0; tx < m.W; tx++ {
				x, y := m.TileCenter(tx, ty)
				i := m.Idx(tx, ty)
				if m.Terrain[i] == world.TerrainLand && math.Hypot(x-cx, y-cy) <= d.r {
					m.Deposit[i] = d.kind
				}
			}
		}
	}

	// Исходные владельцы: 1 = РФ, 2 = Украина.
	var ord ring
	for _, p := range ordlo {
		ord = append(ord, proj([]float64{p.X, p.Y}))
	}
	crimea := map[int16]bool{}
	for id, r := range m.Regions {
		if r.Name == "Автономная Республика Крым" || r.Name == "Севастополь" {
			crimea[int16(id)] = true
		}
	}
	for ty := 0; ty < m.H; ty++ {
		for tx := 0; tx < m.W; tx++ {
			i := m.Idx(tx, ty)
			if m.Terrain[i] != world.TerrainLand {
				continue
			}
			cx, cy := m.TileCenter(tx, ty)
			switch m.Country[i] {
			case world.CountryRussia:
				m.Initial[i] = 1
			case world.CountryUkraine:
				if crimea[m.Region[i]] || inRing(ord, cx, cy) {
					m.Initial[i] = 1
				} else {
					m.Initial[i] = 2
				}
			}
		}
	}

	b, err := m.Encode()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("регионов %d, городов %d, линий %d, файл %d КБ\n", len(m.Regions), len(m.Cities), len(m.Lines), len(b)/1024)
}
