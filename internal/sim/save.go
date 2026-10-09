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
	// Пишем во временный файл и переименовываем: сбой посреди записи не портит прежнее сохранение.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
	for s := 0; s < 2; s++ {
		w.Sides[s].migrateTech(cat, s)
	}
	for id, b := range w.Buildings {
		if cat.BuildingByID[b.Type] == nil {
			delete(w.Buildings, id) // например, макеты из старых версий
		}
	}
	for id, u := range w.Units {
		if cat.UnitByID[u.Type] == nil {
			delete(w.Units, id) // тип исчез из данных
		}
	}
	for _, sd := range w.Sides {
		keep := sd.Orders[:0]
		for _, o := range sd.Orders {
			if _, _, _, ok := cat.ItemCost(o.Item); ok {
				keep = append(keep, o)
			}
		}
		sd.Orders = keep
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
	if !sd.PostureSet { // старое сохранение: одна позиция на весь фронт
		sd.PostureDir = [3]int{sd.Posture, sd.Posture, sd.Posture}
		sd.PostureSet = true
	}
	for d := range sd.Front {
		// Старое сохранение: все FPV считались базовой версии.
		if sd.Front[d].FPVPow == 0 && sd.Front[d].FPV > 0 {
			sd.Front[d].FPVPow = sd.Front[d].FPV
		}
	}
	if !sd.DirsSet {
		sd.migrateDirs()
	}
	for d := range sd.Dirs {
		if sd.Dirs[d].FPVPow == 0 && sd.Dirs[d].FPV > 0 {
			sd.Dirs[d].FPVPow = sd.Dirs[d].FPV
		}
	}
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
	if sd.MoraleHist == nil {
		sd.MoraleHist = map[string][]float64{}
	}
	if sd.Reserve == nil {
		sd.Reserve = map[string]int{}
	}
	if sd.MissionSeen == nil {
		sd.MissionSeen = map[string][]uint32{}
	}
	if sd.MissionFail == nil {
		sd.MissionFail = map[string]bool{}
	}
	if sd.KeepStock == nil {
		sd.KeepStock = map[string]bool{}
	}
	if sd.LowAlert == nil {
		sd.LowAlert = map[string]int{}
	}
	if sd.AutoImport == nil {
		sd.AutoImport = map[string]bool{}
	}
	if sd.AutoImportAt == nil {
		sd.AutoImportAt = map[string]float64{}
	}
	if sd.AirOpen == nil {
		sd.AirOpen = map[string]bool{}
	}
	if sd.SanctionOn == nil {
		sd.SanctionOn = map[string]bool{}
	}
	if sd.SanctionAt == nil {
		sd.SanctionAt = map[string]float64{}
	}
	if sd.CitySeen == nil {
		sd.CitySeen = map[int]int{}
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

// oldTechIDs — исследования старого дерева, переехавшие в линейки версий.
var oldTechIDs = map[string]string{
	"ru_fiber": "ru_fpv_fiber", "ua_fiber": "ua_fpv_fiber", "ru_pantsir_smd": "ru_pantsir_s2",
}

// migrateTech приводит исследования старого сохранения к текущему дереву: переименованные
// переносятся, исчезнувшие отбрасываются, эффекты пересчитываются, а предметы изученного
// открываются (уже открытое не закрывается).
func (sd *Side) migrateTech(cat *data.Catalog, s int) {
	known := cat.TechByID[s]
	done := map[string]bool{}
	for id := range sd.Researched {
		if n, ok := oldTechIDs[id]; ok {
			id = n
		}
		if known[id] != nil {
			done[id] = true
		}
	}
	prog := map[string]float64{}
	for id, v := range sd.Progress {
		if n, ok := oldTechIDs[id]; ok {
			id = n
		}
		if known[id] != nil && !done[id] {
			prog[id] = v
		}
	}
	if n, ok := oldTechIDs[sd.Research]; ok {
		sd.Research = n
	}
	if known[sd.Research] == nil || done[sd.Research] {
		sd.Research = ""
	}
	sd.Researched, sd.Progress = done, prog
	sd.Effects = map[string]float64{}
	for id := range done {
		t := known[id]
		for _, u := range t.Unlocks {
			sd.Unlocked[u] = true
		}
		for k, v := range t.Effects {
			sd.Effects[k] += v
		}
	}
	// Стартовые предметы стороны открыты всегда.
	for _, id := range cat.Sides[s].Unlocked {
		sd.Unlocked[id] = true
	}
}

// migrateDirs переносит силы старого сохранения с трёх направлений (Север, Донбасс, Юг) на четыре:
// Север делится поровну между Киевом и Харьковом, Донбасс остаётся, Юг становится Крымом.
func (sd *Side) migrateDirs() {
	half := func(f Direction) Direction {
		f.Men, f.Armor, f.Artillery, f.FPV, f.FPVPow = f.Men/2, f.Armor/2, f.Artillery/2, f.FPV/2, f.FPVPow/2
		f.Losses /= 2
		return f
	}
	sd.Dirs = [NumDir]Direction{half(sd.Front[0]), half(sd.Front[0]), sd.Front[1], sd.Front[2]}
	sd.DirAlloc = [NumDir]float64{sd.Alloc[0] / 2, sd.Alloc[0] / 2, sd.Alloc[1], sd.Alloc[2]}
	sum := 0.0
	for _, a := range sd.DirAlloc {
		sum += a
	}
	if sum <= 0 {
		sd.DirAlloc = uniformAlloc()
	}
	sd.DirPosture = [NumDir]int{sd.PostureDir[0], sd.PostureDir[0], sd.PostureDir[1], sd.PostureDir[2]}
	sd.DirsSet = true
}
