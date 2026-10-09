package sim

import (
	"math"
	"math/rand"
	"time"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
)

// New создаёт новую партию.
func New(cat *data.Catalog, m *world.MapData, sandbox bool) *World {
	w := &World{
		Owner:     make([]uint8, len(m.Initial)),
		Fort:      make([]uint8, len(m.Initial)),
		Pressure:  make([]float32, len(m.Initial)),
		FortJobs:  map[int]float64{},
		Buildings: map[uint32]*Building{},
		Units:     map[uint32]*Unit{},
		Projs:     map[uint32]*Projectile{},
		PrepEnd:   cat.Rules.PrepMinutes,
		Winner:    -1,
		Sandbox:   sandbox,
		Seed:      time.Now().UnixNano(),
	}
	copy(w.Owner, m.Initial)
	w.Attach(cat, m)

	for s := 0; s < 2; s++ {
		def := cat.Sides[s]
		sd := &Side{
			Res:         data.ToRes(def.Resources),
			Morale:      def.Morale,
			People:      def.People,
			Stocks:      map[string]float64{},
			Storage:     def.Storage,
			Alloc:       [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3},
			Posture:     PostureActive,
			PostureDir:  [3]int{PostureActive, PostureActive, PostureActive},
			PostureSet:  true,
			Capacity:    map[string]float64{},
			Unlocked:    map[string]bool{},
			Researched:  map[string]bool{},
			Progress:    map[string]float64{},
			Bonus:       map[string]float64{},
			Effects:     map[string]float64{},
			ImportCount: map[string]int{},
			AidDone:     map[string]bool{},
			MobUsed:     map[string]int{},
			MobReady:    map[string]float64{},
			Known:       map[uint32]*Contact{},
			SeenAt:      make([]float32, len(m.Initial)),
			Speed:       2,
			PauseLeft:   cat.Rules.PauseBudgetSec,
			RegionPower: map[int]float64{},
			MoraleHist:  map[string][]float64{},
			CitySeen:    map[int]int{},
			MissionSeen: map[string][]uint32{},
			MissionFail: map[string]bool{},
			AirOpen:     map[string]bool{},
			SanctionOn:  map[string]bool{},
			SanctionAt:  map[string]float64{},
			Reserve:     map[string]int{},
		}
		for i := range sd.SeenAt {
			sd.SeenAt[i] = -1e9
		}
		for k, v := range def.Stocks {
			sd.Stocks[k] = v
		}
		for d := 0; d < 3; d++ {
			f := def.Front[d]
			sd.Front[d] = Direction{Men: f.Men, Armor: f.Armor, Artillery: f.Artillery, FPV: f.FPV, FPVPow: f.FPV, Supply: 1}
		}
		for _, id := range def.Unlocked {
			sd.Unlocked[id] = true
		}
		w.Sides[s] = sd
	}

	// Реальные объекты.
	for _, o := range cat.Objects {
		x, y := m.Project(o.Lon, o.Lat)
		side := w.sideAtPoint(x, y)
		if side < 0 {
			continue
		}
		b := w.addBuilding(o.Type, side, x, y, 1)
		b.Name = o.Name
		b.Prewar = true
		b.Dir = o.Dir
		b.Scale = o.Scale
		for k, n := range o.Aircraft {
			if b.Aircraft == nil {
				b.Aircraft = map[string]float64{}
			}
			b.Aircraft[k] = float64(n)
		}
		if o.Type != "rail_hub" && o.Type != "bridge" && o.Type != "depot" {
			b.Dir = -1
		}
	}
	// Стартовые юниты.
	for s := 0; s < 2; s++ {
		for _, su := range cat.Sides[s].Units {
			x, y := m.Project(su.Lon, su.Lat)
			w.addUnit(su.Type, s, x, y)
		}
	}
	// Довоенная разведка: стационарные объекты противника известны.
	for _, b := range w.Buildings {
		e := 1 - b.Side
		w.Sides[e].Known[b.ID] = &Contact{ID: b.ID, Kind: 0, Type: b.Type, X: b.X, Y: b.Y, Seen: -1, HP: -1, Source: "довоенные данные"}
	}
	for s := 0; s < 2; s++ {
		w.Log(s, 1, "Подготовительная фаза: расставьте ПВО, распределите силы и задайте госзаказ. Война начнётся через "+fmtHours(w.PrepEnd/60)+".")
	}
	w.updateFrontTiles()
	w.FrontAcc = cat.Rules.FrontStepMin // первый шаг сразу посчитает статистику фронта
	w.front(0)
	return w
}

// Attach подключает каталог и карту (после создания или загрузки).
func (w *World) Attach(cat *data.Catalog, m *world.MapData) {
	w.cat, w.m = cat, m
	w.rng = rand.New(rand.NewSource(w.Seed + int64(w.Time)))
	for i := range w.depots {
		w.depots[i] = nil
	}
	for i, d := range m.Deposit {
		if d != 0 {
			w.depots[d] = append(w.depots[d], i)
		}
	}
	w.cityAt = map[int]int{}
	w.kyiv = -1
	for ci, c := range m.Cities {
		tx, ty := m.TileAt(c.X, c.Y)
		if !m.In(tx, ty) {
			continue
		}
		i := m.Idx(tx, ty)
		if _, ok := w.cityAt[i]; !ok {
			w.cityAt[i] = ci
		}
		if c.Name == "Киев" && w.kyiv < 0 {
			w.kyiv = i
		}
	}
	for s := 0; s < 2; s++ {
		w.visible[s] = make([]bool, len(m.Initial))
	}
	w.updateFrontTiles()
}

// sideAtPoint — владелец ближайшего тайла суши.
func (w *World) sideAtPoint(x, y float64) int {
	tx, ty := w.m.TileAt(x, y)
	for r := 0; r <= 6; r++ {
		best := -1
		bd := math.Inf(1)
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if max(abs(dx), abs(dy)) != r || !w.m.In(tx+dx, ty+dy) {
					continue
				}
				i := w.m.Idx(tx+dx, ty+dy)
				if w.Owner[i] == 0 {
					continue
				}
				cx, cy := w.m.TileCenter(tx+dx, ty+dy)
				d := math.Hypot(cx-x, cy-y)
				if d < bd {
					bd, best = d, i
				}
			}
		}
		if best >= 0 {
			return w.OwnerSide(best)
		}
	}
	return -1
}

// regionAt — область ближайшего тайла суши.
func (w *World) regionAt(x, y float64) int {
	tx, ty := w.m.TileAt(x, y)
	for r := 0; r <= 6; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if w.m.In(tx+dx, ty+dy) {
					if g := w.m.Region[w.m.Idx(tx+dx, ty+dy)]; g >= 0 {
						return int(g)
					}
				}
			}
		}
	}
	return -1
}

func (w *World) addBuilding(typ string, side int, x, y float64, built float64) *Building {
	bt := w.cat.BuildingByID[typ]
	b := &Building{
		ID: w.newID(), Type: typ, Name: bt.Name, Side: side, X: x, Y: y,
		HP: bt.HP, MaxHP: bt.HP, Built: built, Dir: -1, Repair: true,
		Region: w.regionAt(x, y),
	}
	if len(bt.Aircraft) > 0 {
		b.Aircraft = map[string]float64{}
		for k, v := range bt.Aircraft {
			b.Aircraft[k] = float64(v)
		}
	}
	b.Budget = bt.LaunchRate
	w.Buildings[b.ID] = b
	return b
}

func (w *World) addUnit(typ string, side int, x, y float64) *Unit {
	ut := w.cat.UnitByID[typ]
	u := &Unit{ID: w.newID(), Type: typ, Side: side, X: x, Y: y, HP: ut.HP, State: UnitDeployed, Ready: float64(ut.Magazine)}
	// Ракеты на пусковых берутся из национального запаса.
	if ut.Interceptor != "" && w.Sides[side] != nil {
		st := w.Sides[side].Stocks
		u.Ready = math.Min(u.Ready, st[ut.Interceptor])
		st[ut.Interceptor] -= u.Ready
	}
	w.Units[u.ID] = u
	return u
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
