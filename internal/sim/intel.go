package sim

import (
	"math"

	"github.com/exrise/droneebla/internal/data"
)

// Типы сенсоров.
const (
	SensorOptical = "optical"
	SensorRadar   = "radar"
)

// SatPass — положение спутника.
type SatPass struct {
	Active bool
	X, Y   float64 // текущая точка (если активен)
	X0, Y0 float64 // начало трассы текущего/следующего пролёта
	DX, DY float64 // единичный вектор трассы
	L      float64 // длина трассы
	Start  float64 // время начала пролёта
	Swath  float64
	Index  int
}

const satSpeedKmMin = 420.0 // ~7 км/с

// SatAt — пролёт спутника, активный в момент t или ближайший следующий.
func (w *World) SatAt(sat data.Satellite, t float64) SatPass {
	period := sat.PeriodH * 60
	phase := sat.PhaseH * 60
	ang := sat.Angle * math.Pi / 180
	dx, dy := math.Sin(ang), math.Cos(ang)
	L := w.m.HeightKm() / dy
	dur := L / satSpeedKmMin
	k := int(math.Floor((t - phase) / period))
	if k < 0 {
		k = 0
	}
	for n := 0; n < 3; n++ {
		start := phase + float64(k)*period
		if t < start+dur {
			lon := sat.Tracks[k%len(sat.Tracks)]
			x0, _ := w.m.Project(lon, w.m.Lat0)
			p := SatPass{X0: x0, Y0: 0, DX: dx, DY: dy, L: L, Start: start, Swath: sat.SwathKm, Index: k}
			if t >= start {
				f := (t - start) * satSpeedKmMin
				p.Active = true
				p.X, p.Y = x0+dx*f, dy*f
			}
			return p
		}
		k++
	}
	return SatPass{}
}

// intel — работа всех сенсоров за шаг.
func (w *World) intel(dtMin float64) {
	for s := 0; s < 2; s++ {
		w.groundVision(s)
		w.satellites(s, dtMin)
		w.reconDrones(s)
		w.rtr(s)
		w.flashes(s)
		w.agents(s, dtMin)
	}
}

// observe обновляет контакт по данным сенсора.
func (w *World) observe(s int, id uint32, sensor, source string, errKm float64) {
	sd := w.Sides[s]
	c := sd.Known[id]
	if b, ok := w.Buildings[id]; ok {
		if b.Side == s {
			return
		}
		if c == nil {
			c = &Contact{ID: id, Kind: 0, HP: -1}
			sd.Known[id] = c
		}
		c.X, c.Y = b.X, b.Y
		switch sensor {
		case SensorOptical:
			c.Type, c.HP = b.Type, b.frac()
			if b.Built < 1 {
				c.HP = 0
			}
		case SensorRadar:
			if c.Type == "" {
				c.Class = "крупный объект"
			}
		case "agent":
			c.Type = b.Type
		}
		c.Seen, c.Source = w.Time, source
		if errKm > 0 {
			ox, oy := w.errOffset(id, sensor)
			c.X += ox * errKm
			c.Y += oy * errKm
		}
		return
	}
	if u, ok := w.Units[id]; ok {
		if u.Side == s {
			return
		}
		if c == nil {
			c = &Contact{ID: id, Kind: 1, HP: -1}
			sd.Known[id] = c
		}
		c.Kind = 1
		c.X, c.Y = u.X, u.Y
		switch sensor {
		case SensorOptical, "agent", "rtr", "flash":
			c.Type = u.Type
			c.Class = ""
		case SensorRadar:
			if c.Type == "" {
				c.Class = "техника"
			}
		}
		if sensor == SensorOptical {
			c.HP = u.HP / w.cat.UnitByID[u.Type].HP
		}
		c.Seen, c.Source = w.Time, source
		if errKm > 0 {
			ox, oy := w.errOffset(id, sensor)
			c.X += ox * errKm
			c.Y += oy * errKm
		}
	}
}

// errOffset — ошибка пеленга в [-1, 1]. Для РТР она постоянна в течение часа,
// чтобы отметка не дрожала; для агентуры — случайная.
func (w *World) errOffset(id uint32, sensor string) (float64, float64) {
	if sensor != "rtr" && sensor != "flash" {
		return w.rng.Float64()*2 - 1, w.rng.Float64()*2 - 1
	}
	h := uint64(id)*0x9E3779B97F4A7C15 ^ uint64(w.Time/60)*0xBF58476D1CE4E5B9
	h ^= h >> 31
	h *= 0x94D049BB133111EB
	h ^= h >> 29
	return float64(h&0xFFFF)/32767.5 - 1, float64((h>>16)&0xFFFF)/32767.5 - 1
}

// imprecise — данные с погрешностью (их не опровергает отсутствие объекта
// в точке отметки).
func imprecise(c *Contact) bool {
	switch c.Source {
	case "РТР", "агентура/OSINT", "засветка пуска":
		return true
	}
	return false
}

// forget удаляет контакты в области, где объекта больше нет.
func (w *World) forget(s int, covered func(x, y float64) bool) {
	sd := w.Sides[s]
	for id, c := range sd.Known {
		if !covered(c.X, c.Y) {
			continue
		}
		if imprecise(c) {
			_, isB := w.Buildings[id]
			_, isU := w.Units[id]
			if isB || isU {
				continue
			}
		}
		if b, ok := w.Buildings[id]; ok && b.Side != s && dist(b.X, b.Y, c.X, c.Y) < 3 {
			continue
		}
		if u, ok := w.Units[id]; ok && u.Side != s && dist(u.X, u.Y, c.X, c.Y) < 3 {
			continue
		}
		delete(sd.Known, id)
	}
}

// groundVision — своя территория, фронтовая разведка, обзор юнитов и зданий.
func (w *World) groundVision(s int) {
	r := w.cat.Rules
	vis := w.visible[s]
	for i := range vis {
		vis[i] = w.OwnerSide(i) == s
	}
	rad := int(math.Ceil(r.FrontVisionKm / w.m.TileKm))
	for _, i := range w.frontT[s] {
		tx, ty := i%w.m.W, i/w.m.W
		for dy := -rad; dy <= rad; dy++ {
			for dx := -rad; dx <= rad; dx++ {
				if dx*dx+dy*dy <= rad*rad && w.m.In(tx+dx, ty+dy) {
					vis[w.m.Idx(tx+dx, ty+dy)] = true
				}
			}
		}
	}
	mark := func(x, y, km float64) {
		tx, ty := w.m.TileAt(x, y)
		rr := int(math.Ceil(km / w.m.TileKm))
		for dy := -rr; dy <= rr; dy++ {
			for dx := -rr; dx <= rr; dx++ {
				if w.m.In(tx+dx, ty+dy) {
					cx, cy := w.m.TileCenter(tx+dx, ty+dy)
					if dist(cx, cy, x, y) <= km+w.m.TileKm*0.7 {
						vis[w.m.Idx(tx+dx, ty+dy)] = true
					}
				}
			}
		}
	}
	for _, u := range w.Units {
		if u.Side == s {
			mark(u.X, u.Y, w.cat.UnitByID[u.Type].Vision)
		}
	}
	sd := w.Sides[s]
	for i, v := range vis {
		if v {
			sd.SeenAt[i] = float32(w.Time)
		}
	}
	for id, b := range w.Buildings {
		if b.Side != s && !b.Masked {
			if i := w.tileOf(b.X, b.Y); i >= 0 && vis[i] {
				w.observe(s, id, SensorOptical, "наблюдение", 0)
			}
		}
	}
	for id, u := range w.Units {
		if u.Side != s {
			if i := w.tileOf(u.X, u.Y); i >= 0 && vis[i] {
				w.observe(s, id, SensorOptical, "наблюдение", 0)
			}
		}
	}
	w.forget(s, func(x, y float64) bool {
		i := w.tileOf(x, y)
		return i >= 0 && vis[i] && w.OwnerSide(i) != s
	})
}

// satellites — пролёты спутников.
func (w *World) satellites(s int, dtMin float64) {
	for _, sat := range w.cat.Sides[s].Satellites {
		p := w.SatAt(sat, w.Time)
		if !p.Active {
			continue
		}
		f1 := (w.Time - p.Start) * satSpeedKmMin
		f0 := math.Max(0, f1-dtMin*satSpeedKmMin)
		w.sweep(s, p, f0, f1, sat.Sensor, sat.Name)
	}
}

// sweep — разведка полосы трассы спутника между f0 и f1 км.
func (w *World) sweep(s int, p SatPass, f0, f1 float64, sensor, name string) {
	half := p.Swath / 2
	in := func(x, y float64) bool {
		rx, ry := x-p.X0, y-p.Y0
		along := rx*p.DX + ry*p.DY
		if along < f0 || along > f1 {
			return false
		}
		perp := math.Abs(-rx*p.DY + ry*p.DX)
		return perp <= half
	}
	src := "спутник " + name
	for id, b := range w.Buildings {
		if b.Side == s || !in(b.X, b.Y) {
			continue
		}
		if sensor == SensorOptical && b.Masked {
			continue
		}
		w.observe(s, id, sensor, src, 0)
	}
	for id, u := range w.Units {
		if u.Side != s && in(u.X, u.Y) {
			w.observe(s, id, sensor, src, 0)
		}
	}
	w.forget(s, in)
	// Отметка наблюдённых тайлов.
	sd := w.Sides[s]
	for f := f0; f <= f1; f += w.m.TileKm {
		cx, cy := p.X0+p.DX*f, p.Y0+p.DY*f
		for o := -half; o <= half; o += w.m.TileKm {
			x, y := cx-p.DY*o, cy+p.DX*o
			if i := w.tileOf(x, y); i >= 0 {
				sd.SeenAt[i] = float32(w.Time)
			}
		}
	}
}

// reconDrones — обзор разведывательных БПЛА (оптика).
func (w *World) reconDrones(s int) {
	sd := w.Sides[s]
	for _, p := range w.Projs {
		if p.Side != s || p.Delay > 0 {
			continue
		}
		m := w.cat.MunitionByID[p.Munition]
		v := m.Vision
		switch {
		case m.Kind == "recon":
		case m.Kind == "drone" && sd.eff("drone_recon") > 0:
			// После исследования ударные дроны ведут съёмку по пути к цели.
			v = w.cat.Rules.StrikeDroneVisionKm
		default:
			continue
		}
		for id, b := range w.Buildings {
			if b.Side != s && !b.Masked && dist(b.X, b.Y, p.X, p.Y) <= v {
				w.observe(s, id, SensorOptical, m.Name, 0)
			}
		}
		for id, u := range w.Units {
			if u.Side != s && dist(u.X, u.Y, p.X, p.Y) <= v {
				w.observe(s, id, SensorOptical, m.Name, 0)
			}
		}
		px, py := p.X, p.Y
		w.forget(s, func(x, y float64) bool { return dist(x, y, px, py) <= v })
		tx, ty := w.m.TileAt(p.X, p.Y)
		rr := int(math.Ceil(v / w.m.TileKm))
		for dy := -rr; dy <= rr; dy++ {
			for dx := -rr; dx <= rr; dx++ {
				if w.m.In(tx+dx, ty+dy) {
					sd.SeenAt[w.m.Idx(tx+dx, ty+dy)] = float32(w.Time)
				}
			}
		}
	}
}

// rtr — пеленгация излучателей.
func (w *World) rtr(s int) {
	for _, u := range w.Units {
		if u.Side != s || u.State != UnitDeployed {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.RtrKm <= 0 {
			continue
		}
		for id, e := range w.Units {
			if e.Side == s || e.State != UnitDeployed {
				continue
			}
			et := w.cat.UnitByID[e.Type]
			if !et.Emitter() {
				continue
			}
			d := dist(u.X, u.Y, e.X, e.Y)
			if d > ut.RtrKm {
				continue
			}
			// Свежие данные (любые) не перебиваем: иначе отметка прыгает
			// между точной позицией и пеленгом.
			if c := w.Sides[s].Known[id]; c != nil && w.Time-c.Seen < 10 {
				continue
			}
			w.observe(s, id, "rtr", "РТР", math.Min(15, d*0.04))
		}
	}
}

// flashes — засветка пусковых после залпа.
func (w *World) flashes(s int) {
	for id, u := range w.Units {
		if u.Side != s && u.FlashTill > w.Time && w.Time > u.FlashTill-w.cat.Rules.FlashMin {
			if c := w.Sides[s].Known[id]; c == nil || w.Time-c.Seen > 5 {
				w.observe(s, id, "flash", "засветка пуска", 1)
			}
		}
	}
}

// agents — донесения агентуры и OSINT.
func (w *World) agents(s int, dtMin float64) {
	if !w.War() {
		return
	}
	sd := w.Sides[s]
	r := w.cat.Rules
	k := 1.0
	if sd.AgentFund {
		k = 2.5
	}
	sd.AgentTimer += dtMin * k
	if sd.AgentTimer < r.AgentEveryH*60 {
		return
	}
	sd.AgentTimer = 0
	var ids []uint32
	for id, b := range w.Buildings {
		if b.Side != s {
			ids = append(ids, id)
		}
	}
	for id, u := range w.Units {
		if u.Side != s {
			ids = append(ids, id, id) // юниты интереснее
		}
	}
	if len(ids) == 0 {
		return
	}
	id := ids[w.rng.Intn(len(ids))]
	w.observe(s, id, "agent", "агентура/OSINT", r.AgentErrorKm)
	if c := sd.Known[id]; c != nil {
		name := c.Type
		if b := w.cat.BuildingByID[c.Type]; b != nil {
			name = b.Name
		} else if u := w.cat.UnitByID[c.Type]; u != nil {
			name = u.Name
		}
		w.LogAt(s, 0, "Донесение агентуры: замечен объект «"+name+"» (точность ±8 км)", c.X, c.Y)
	}
}
