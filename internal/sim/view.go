package sim

import (
	"sort"

	"github.com/exrise/droneebla/internal/data"
)

// ProjView — видимый летящий объект.
type ProjView struct {
	ID       uint32
	Munition string
	Side     int
	X, Y     float64
	Heading  float64
	Path     []Pt // только для своих
	Engaged  int
}

// SatView — спутник на карте.
type SatView struct {
	Name   string
	Side   int
	Sensor string
	Pass   SatPass
}

// View — всё, что видит сторона. Это единственное, что получает клиент.
type View struct {
	Side       int
	Sandbox    bool
	Solo       bool
	Time       float64
	PrepEnd    float64
	War        bool
	Speed      int
	MySpeed    int
	EnemySpeed int
	Paused     bool
	Pausing    bool
	PauseLeft  float64
	Winner     int
	WinReason  string

	Res         data.Res
	Rates       data.Res
	Income      float64
	Morale      float64
	People      float64
	LaborLoss   float64
	Blackout    float64
	Power       [2]float64
	Stocks      map[string]float64
	Storage     data.FrontPool
	Front       [3]Direction
	EnemyFront  [3]int // число фронтовых тайлов противника (видно по линии)
	Alloc       [3]float64
	Posture     int
	HasMain     bool
	MainX       float64
	MainY       float64
	Orders      []Order
	Capacity    map[string]float64
	Unlocked    map[string]bool
	Researched  map[string]bool
	Research    string
	Progress    map[string]float64
	Bonus       map[string]float64
	ResRate     float64
	ResFund     int
	AgentFund   bool
	Effects     map[string]float64
	Deliveries  []Delivery
	ImportCount map[string]int
	AidDone     map[string]bool
	MobUsed     map[string]int
	MobReady    map[string]float64
	PropReady   float64
	RegionPower map[int]float64
	Missions    []MissionView

	Buildings []Building
	Units     []Unit
	Contacts  []Contact
	Projs     []ProjView
	Sats      []SatView
	Events    []Event

	Owner    []uint8
	Fog      []uint8 // 2 — видно сейчас, 1 — недавно, 0 — давно/никогда
	Fort     []uint8 // только свои тайлы
	FortJobs []int
	Pressure []uint8 // давление: на своих тайлах — противника, на чужих — наше, 0..255
	Captures []Capture
}

// BuildView собирает представление для стороны s.
func (w *World) BuildView(s int, eventsSince uint64) *View {
	sd := w.Sides[s]
	v := &View{
		Side: s, Sandbox: w.Sandbox, Solo: w.Solo, Time: w.Time, PrepEnd: w.PrepEnd, War: w.War(),
		Speed: w.EffectiveSpeed(), MySpeed: sd.Speed, EnemySpeed: w.Sides[1-s].Speed,
		Paused: w.Paused(), Pausing: sd.Pausing, PauseLeft: sd.PauseLeft,
		Winner: w.Winner, WinReason: w.WinReason,
		Res: sd.Res, Rates: sd.Rates, Income: sd.Income, Morale: sd.Morale, People: sd.People,
		LaborLoss: sd.LaborLoss, Blackout: sd.Blackout, Power: sd.Power,
		Stocks: copyMap(sd.Stocks), Storage: sd.Storage, Front: sd.Front, Alloc: sd.Alloc,
		Posture: sd.Posture, HasMain: sd.HasMain, MainX: sd.MainX, MainY: sd.MainY,
		Orders: append([]Order{}, sd.Orders...), Capacity: copyMap(sd.Capacity),
		Unlocked: copyBool(sd.Unlocked), Researched: copyBool(sd.Researched),
		Research: sd.Research, Progress: copyMap(sd.Progress), Bonus: copyMap(sd.Bonus),
		ResRate: sd.ResRate, ResFund: sd.ResFund, AgentFund: sd.AgentFund,
		Effects: copyMap(sd.Effects), Deliveries: append([]Delivery{}, sd.Deliveries...),
		ImportCount: copyInt(sd.ImportCount), AidDone: copyBool(sd.AidDone),
		MobUsed: copyInt(sd.MobUsed), MobReady: copyMap(sd.MobReady), PropReady: sd.PropReady,
		RegionPower: map[int]float64{},
	}
	for k, x := range sd.RegionPower {
		v.RegionPower[k] = x
	}
	v.Missions = w.missionViews(s)
	for d := 0; d < 3; d++ {
		v.EnemyFront[d] = w.Sides[1-s].Front[d].Tiles
	}
	for _, b := range w.Buildings {
		if b.Side == s {
			cp := *b
			if b.Aircraft != nil {
				cp.Aircraft = copyMap(b.Aircraft)
			}
			v.Buildings = append(v.Buildings, cp)
		}
	}
	sort.Slice(v.Buildings, func(a, b int) bool { return v.Buildings[a].ID < v.Buildings[b].ID })
	for _, u := range w.Units {
		if u.Side == s {
			cp := *u
			cp.Path = append([]Pt{}, u.Path...)
			v.Units = append(v.Units, cp)
		}
	}
	sort.Slice(v.Units, func(a, b int) bool { return v.Units[a].ID < v.Units[b].ID })
	for _, c := range sd.Known {
		v.Contacts = append(v.Contacts, *c)
	}
	sort.Slice(v.Contacts, func(a, b int) bool { return v.Contacts[a].ID < v.Contacts[b].ID })
	for _, p := range w.Projs {
		if p.Delay > 0 && p.Side != s {
			continue
		}
		if p.Side == s || w.tracked(s, p) {
			pv := ProjView{ID: p.ID, Munition: p.Munition, Side: p.Side, X: p.X, Y: p.Y, Heading: p.Heading, Engaged: p.Engaged}
			if p.Side == s {
				pv.Path = append([]Pt{}, p.Path...)
				if p.Delay > 0 {
					pv.Engaged = -1
				}
			}
			v.Projs = append(v.Projs, pv)
		}
	}
	for side := 0; side < 2; side++ {
		for _, sat := range w.cat.Sides[side].Satellites {
			v.Sats = append(v.Sats, SatView{Name: sat.Name, Side: side, Sensor: sat.Sensor, Pass: w.SatAt(sat, w.Time)})
		}
	}
	for _, e := range sd.Events {
		if e.ID > eventsSince {
			v.Events = append(v.Events, e)
		}
	}
	n := len(w.Owner)
	v.Owner = append([]uint8{}, w.Owner...)
	v.Fog = make([]uint8, n)
	v.Fort = make([]uint8, n)
	v.Pressure = make([]uint8, n)
	for i := 0; i < n; i++ {
		age := w.Time - float64(sd.SeenAt[i])
		switch {
		case w.visible[s][i] || age < 3:
			v.Fog[i] = 2
		case age < 360:
			v.Fog[i] = 1
		}
		if w.OwnerSide(i) == s {
			v.Fort[i] = w.Fort[i]
		}
		if p := w.Pressure[i]; p > 0 {
			v.Pressure[i] = uint8(clamp(float64(p)*255, 0, 255))
		}
	}
	v.Captures = append([]Capture{}, w.Captures...)
	for i := range w.FortJobs {
		if w.OwnerSide(i) == s {
			v.FortJobs = append(v.FortJobs, i)
		}
	}
	return v
}

func copyMap(m map[string]float64) map[string]float64 {
	o := make(map[string]float64, len(m))
	for k, v := range m {
		o[k] = v
	}
	return o
}

func copyBool(m map[string]bool) map[string]bool {
	o := make(map[string]bool, len(m))
	for k, v := range m {
		o[k] = v
	}
	return o
}

func copyInt(m map[string]int) map[string]int {
	o := make(map[string]int, len(m))
	for k, v := range m {
		o[k] = v
	}
	return o
}
