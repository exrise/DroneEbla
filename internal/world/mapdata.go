// Package world содержит статическую карту (тайлы, области, города, линии)
// и вспомогательную геометрию. Карта готовится утилитой cmd/mapbake из
// данных Natural Earth и встраивается в исполняемый файл.
package world

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/gob"
	"math"
)

// Страны на карте.
const (
	CountryNone    uint8 = iota // море или иностранное государство
	CountryUkraine              // Украина (включая Крым)
	CountryRussia               // Россия
	CountryBelarus              // Беларусь: только воздушное пространство для РФ
	CountryForeign              // прочие государства: закрыты для всех
)

// Типы местности тайла.
const (
	TerrainSea uint8 = iota
	TerrainLand
	TerrainLake
)

// Месторождения (свойство тайла).
const (
	DepositNone uint8 = iota
	DepositOil
	DepositGas
	DepositCoal
	DepositOre
)

// Флаги тайла.
const (
	FlagRiver uint8 = 1 << iota // крупная река: штраф наступающему
	FlagRoad                    // дорога: быстрое передвижение
	FlagRail                    // железная дорога
	FlagUrban                   // городская застройка
)

// Region — административная область.
type Region struct {
	Name    string
	Country uint8
	CX, CY  float64 // центр, км
}

// City — населённый пункт.
type City struct {
	Name    string
	X, Y    float64
	Pop     int
	Region  int
	Capital bool
}

// Line — ломаная для отрисовки (реки, дороги, ЖД, границы).
type Line struct {
	Kind int // LineRiver, LineRoad, LineRail, LineCoast, LineBorder
	Pts  []float32
}

const (
	LineRiver = iota
	LineRoad
	LineRail
	LineCoast
	LineCountryBorder
)

// MapData — всё статическое описание карты.
type MapData struct {
	W, H    int
	TileKm  float64
	Lon0    float64 // долгота левого края
	Lat0    float64 // широта верхнего края
	KX, KY  float64 // км на градус
	Terrain []uint8
	Country []uint8
	Region  []int16 // -1 если нет
	Deposit []uint8
	Flags   []uint8
	Initial []uint8 // исходный владелец: 0 никто, 1 РФ, 2 Украина (см. sim)
	Regions []Region
	Cities  []City
	Lines   []Line
}

//go:embed map.bin
var embeddedMap []byte

// Load распаковывает встроенную карту.
func Load() (*MapData, error) {
	return Decode(embeddedMap)
}

// Decode распаковывает карту из gzip+gob.
func Decode(b []byte) (*MapData, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var m MapData
	if err := gob.NewDecoder(zr).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Encode упаковывает карту.
func (m *MapData) Encode() ([]byte, error) {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err := gob.NewEncoder(zw).Encode(m); err != nil {
		return nil, err
	}
	zw.Close()
	return buf.Bytes(), nil
}

// Project переводит долготу/широту в км карты.
func (m *MapData) Project(lon, lat float64) (x, y float64) {
	return (lon - m.Lon0) * m.KX, (m.Lat0 - lat) * m.KY
}

// Unproject — обратное преобразование.
func (m *MapData) Unproject(x, y float64) (lon, lat float64) {
	return m.Lon0 + x/m.KX, m.Lat0 - y/m.KY
}

// Idx возвращает индекс тайла.
func (m *MapData) Idx(tx, ty int) int { return ty*m.W + tx }

// In проверяет попадание тайла в карту.
func (m *MapData) In(tx, ty int) bool { return tx >= 0 && ty >= 0 && tx < m.W && ty < m.H }

// TileAt возвращает тайл под точкой (км).
func (m *MapData) TileAt(x, y float64) (int, int) {
	return int(math.Floor(x / m.TileKm)), int(math.Floor(y / m.TileKm))
}

// TileCenter — центр тайла в км.
func (m *MapData) TileCenter(tx, ty int) (float64, float64) {
	return (float64(tx) + 0.5) * m.TileKm, (float64(ty) + 0.5) * m.TileKm
}

// WidthKm/HeightKm — размеры карты.
func (m *MapData) WidthKm() float64  { return float64(m.W) * m.TileKm }
func (m *MapData) HeightKm() float64 { return float64(m.H) * m.TileKm }

// IsLand — суша (не море и не озеро).
func (m *MapData) IsLand(i int) bool { return m.Terrain[i] == TerrainLand }
