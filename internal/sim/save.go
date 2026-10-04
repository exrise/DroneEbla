package sim

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
)

// Save записывает партию в файл.
func (w *World) Save(path string) error {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(zw).Encode(w); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Load читает партию из файла.
func Load(path string, cat *data.Catalog, m *world.MapData) (*World, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	var w World
	if err := gob.NewDecoder(zr).Decode(&w); err != nil {
		return nil, err
	}
	if len(w.Owner) != len(m.Initial) {
		return nil, fmt.Errorf("сохранение сделано на другой карте (старая версия игры)")
	}
	for _, sd := range w.Sides {
		sd.ensure(len(m.Initial))
	}
	for id, b := range w.Buildings {
		if cat.BuildingByID[b.Type] == nil {
			delete(w.Buildings, id) // например, макеты из старых версий
		}
	}
	if w.FortJobs == nil {
		w.FortJobs = map[int]float64{}
	}
	if w.Groups == nil {
		w.Groups = map[uint32]*StrikeGroup{}
	}
	if w.Projs == nil {
		w.Projs = map[uint32]*Projectile{}
	}
	w.Attach(cat, m)
	return &w, nil
}

// ensure восстанавливает пустые карты после декодирования gob.
func (sd *Side) ensure(n int) {
	if sd.Stocks == nil {
		sd.Stocks = map[string]float64{}
	}
	if sd.Capacity == nil {
		sd.Capacity = map[string]float64{}
	}
	if sd.Unlocked == nil {
		sd.Unlocked = map[string]bool{}
	}
	if sd.Researched == nil {
		sd.Researched = map[string]bool{}
	}
	if sd.Progress == nil {
		sd.Progress = map[string]float64{}
	}
	if sd.Bonus == nil {
		sd.Bonus = map[string]float64{}
	}
	if sd.Effects == nil {
		sd.Effects = map[string]float64{}
	}
	if sd.ImportCount == nil {
		sd.ImportCount = map[string]int{}
	}
	if sd.AidDone == nil {
		sd.AidDone = map[string]bool{}
	}
	if sd.MobUsed == nil {
		sd.MobUsed = map[string]int{}
	}
	if sd.MobReady == nil {
		sd.MobReady = map[string]float64{}
	}
	if sd.Known == nil {
		sd.Known = map[uint32]*Contact{}
	}
	if sd.RegionPower == nil {
		sd.RegionPower = map[int]float64{}
	}
	if len(sd.SeenAt) != n {
		sd.SeenAt = make([]float32, n)
		for i := range sd.SeenAt {
			sd.SeenAt[i] = -1e9
		}
	}
}
