package sim

import (
	"fmt"
	"math"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
)

// StrikePlan — параметры удара.
type StrikePlan struct {
	Source    uint32
	Munition  string
	Count     int
	Waypoints []Pt
	Target    Pt
	Delay     float64 // минут
}

// sourcePos возвращает позицию и платформы источника пуска.
func (w *World) sourcePos(s int, id uint32) (x, y float64, ok bool) {
	if b, found := w.Buildings[id]; found && b.Side == s {
		return b.X, b.Y, true
	}
	if u, found := w.Units[id]; found && u.Side == s {
		return u.X, u.Y, true
	}
	return 0, 0, false
}

// LaunchOptions — какие боеприпасы можно запустить с источника.
func (w *World) LaunchOptions(s int, id uint32) []string {
	var out []string
	if b, ok := w.Buildings[id]; ok && b.Side == s {
		bt := w.cat.BuildingByID[b.Type]
		for _, m := range w.cat.Munitions {
			if data.SideIndex(m.Side) != s || m.Kind == "interceptor" {
				continue
			}
			for _, p := range bt.Launch {
				if p == m.Platform {
					out = append(out, m.ID)
				}
			}
		}
	}
	if u, ok := w.Units[id]; ok && u.Side == s {
		out = append(out, w.cat.UnitByID[u.Type].Munitions...)
	}
	return out
}

// PathLength — длина маршрута.
func PathLength(start Pt, pts []Pt) float64 {
	l := 0.0
	p := start
	for _, q := range pts {
		l += dist(p.X, p.Y, q.X, q.Y)
		p = q
	}
	return l
}

// AirspaceBlock — страна, чьё небо закрыто на отрезке a–b (0 — путь свободен).
// Открыты: свои страны, море и озёра, Беларусь при belarusAir и страны из open
// (открытые пакеты airspace); прочие государства закрыты. Функция не зависит
// от состояния мира, поэтому её вызывает и интерфейс (по карте и данным View).
func AirspaceBlock(m *world.MapData, belarusAir bool, open map[uint8]bool, a, b Pt) uint8 {
	d := dist(a.X, a.Y, b.X, b.Y)
	n := int(d/3) + 1
	for k := 0; k <= n; k++ {
		t := float64(k) / float64(n)
		x, y := a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t
		tx, ty := m.TileAt(x, y)
		if !m.In(tx, ty) {
			continue // за краем карты — допустимо
		}
		i := m.Idx(tx, ty)
		if m.Terrain[i] != world.TerrainLand {
			continue
		}
		c := m.Country[i]
		switch c {
		case world.CountryBelarus:
			if !belarusAir {
				return c
			}
		case world.CountryForeign, world.CountryPoland, world.CountryLithuania,
			world.CountryLatvia, world.CountryEstonia, world.CountryFinland:
			if !open[c] {
				return c
			}
		}
	}
	return 0
}

// airspaceOK — можно ли стороне пролететь по отрезку.
func (w *World) airspaceOK(s int, a, b Pt) bool {
	return w.airspaceBlock(s, a, b) == 0
}

func (w *World) airspaceBlock(s int, a, b Pt) uint8 {
	return AirspaceBlock(w.m, w.cat.Sides[s].BelarusAir, w.openCountries(s), a, b)
}

// ValidateStrike проверяет план удара. Возвращает текст ошибки или "".
func (w *World) ValidateStrike(s int, p StrikePlan) string {
	if !w.War() {
		return "Удары невозможны в подготовительной фазе"
	}
	m := w.cat.MunitionByID[p.Munition]
	if m == nil || data.SideIndex(m.Side) != s || m.Kind == "interceptor" {
		return "Неизвестный боеприпас"
	}
	x, y, ok := w.sourcePos(s, p.Source)
	if !ok {
		return "Нет источника пуска"
	}
	allowed := false
	for _, id := range w.LaunchOptions(s, p.Source) {
		if id == p.Munition {
			allowed = true
		}
	}
	if !allowed {
		return "Этот боеприпас нельзя запустить отсюда"
	}
	if p.Count < 1 {
		return "Количество должно быть больше нуля"
	}
	if w.Sides[s].Stocks[p.Munition] < float64(p.Count) {
		return fmt.Sprintf("Недостаточно: на складе %.0f", w.Sides[s].Stocks[p.Munition])
	}
	if u, ok := w.Units[p.Source]; ok {
		ut := w.cat.UnitByID[u.Type]
		if u.State != UnitDeployed {
			return "Пусковая не развёрнута"
		}
		if u.Reload > 0 {
			return fmt.Sprintf("Перезарядка: ещё %.0f мин", u.Reload)
		}
		if p.Count > ut.Salvo {
			return fmt.Sprintf("Максимум в залпе: %d", ut.Salvo)
		}
		if w.sideOfPoint(u.X, u.Y) != s {
			return "Пусковая должна стоять на своей территории"
		}
	}
	if b, ok := w.Buildings[p.Source]; ok {
		if !b.Operational() {
			return "Площадка не работает"
		}
		if float64(p.Count) > math.Floor(b.Budget) {
			return fmt.Sprintf("Площадка сейчас может запустить не более %.0f", math.Floor(b.Budget))
		}
	}
	start := Pt{x, y}
	pts := w.flightPath(m, p)
	l := PathLength(start, pts)
	if m.Kind == "recon" {
		l += dist(pts[len(pts)-1].X, pts[len(pts)-1].Y, x, y)
	}
	if l > m.RangeKm {
		return fmt.Sprintf("Цель вне досягаемости: %.0f км при дальности %.0f км", l, m.RangeKm)
	}
	prev := start
	for _, q := range pts {
		if c := w.airspaceBlock(s, prev, q); c != 0 {
			if name := world.CountryName(c); name != "" {
				return "Маршрут проходит через закрытое воздушное пространство: " + name
			}
			return "Маршрут проходит через закрытое воздушное пространство"
		}
		prev = q
	}
	if m.Kind != "recon" {
		if b := w.buildingNear(p.Target.X, p.Target.Y, 1.0); b != nil && b.Side == s {
			return "Это свой объект"
		}
		if b := w.buildingNear(p.Target.X, p.Target.Y, 1.0); b != nil && w.cat.BuildingByID[b.Type].Untargetable {
			return "Удары по АЭС запрещены"
		}
	}
	return ""
}

// flightPath — фактический маршрут: баллистика летит напрямую.
func (w *World) flightPath(m *data.MunitionType, p StrikePlan) []Pt {
	if m.Kind == "ballistic" || m.Kind == "rocket" || (m.Kind == "cruise" && m.Class == "high") {
		return []Pt{p.Target}
	}
	pts := append([]Pt{}, p.Waypoints...)
	if m.Kind != "recon" || len(pts) == 0 {
		pts = append(pts, p.Target)
	}
	return pts
}

// Strike запускает удар.
func (w *World) Strike(s int, p StrikePlan) string {
	if e := w.ValidateStrike(s, p); e != "" {
		return e
	}
	m := w.cat.MunitionByID[p.Munition]
	x, y, _ := w.sourcePos(s, p.Source)
	sd := w.Sides[s]
	sd.Stocks[p.Munition] -= float64(p.Count)
	pts := w.flightPath(m, p)
	group := w.newID()
	if m.Kind != "recon" {
		if w.Groups == nil {
			w.Groups = map[uint32]*StrikeGroup{}
		}
		w.Groups[group] = &StrikeGroup{Side: s, Munition: p.Munition, Total: p.Count, X: p.Target.X, Y: p.Target.Y}
	}
	gap := 0.5
	if m.SpeedKmh > 1000 {
		gap = 0.2
	}
	for k := 0; k < p.Count; k++ {
		pr := &Projectile{
			ID: w.newID(), Munition: p.Munition, Side: s, X: x, Y: y,
			Path: append([]Pt{}, pts...), Home: Pt{x, y},
			Delay: p.Delay + float64(k)*gap, Source: p.Source, Group: group,
		}
		w.Projs[pr.ID] = pr
	}
	if u, ok := w.Units[p.Source]; ok {
		ut := w.cat.UnitByID[u.Type]
		u.Reload = ut.ReloadMin
		u.FlashTill = w.Time + p.Delay + w.cat.Rules.FlashMin
	}
	if b, ok := w.Buildings[p.Source]; ok {
		b.Budget -= float64(p.Count)
		if m.Platform == "strategic" && w.cat.Sides[1-s].BomberWarning &&
			(w.BomberWarnAt[1-s] <= 0 || w.Time-w.BomberWarnAt[1-s] >= w.cat.Rules.BomberWarnMin) {
			w.BomberWarnAt[1-s] = w.Time
			w.LogAt(1-s, 2, "Разведка партнёров: взлёт стратегической авиации — "+b.Name+". Ожидайте пусков крылатых ракет.", b.X, b.Y)
		}
	}
	verb := "Удар"
	if m.Kind == "recon" {
		verb = "Разведвылет"
	}
	w.LogAt(s, 0, fmt.Sprintf("%s: %s ×%d", verb, m.Name, p.Count), p.Target.X, p.Target.Y)
	return ""
}

// buildingNear — ближайшее здание в радиусе.
func (w *World) buildingNear(x, y, r float64) *Building {
	var best *Building
	bd := r
	for _, b := range w.Buildings {
		if d := dist(b.X, b.Y, x, y); d <= bd {
			bd, best = d, b
		}
	}
	return best
}

// projectiles двигает боеприпасы.
func (w *World) projectiles(dtMin float64) {
	for id, p := range w.Projs {
		if p.Delay > 0 {
			p.Delay -= dtMin
			if p.Delay > 0 {
				continue
			}
		}
		m := w.cat.MunitionByID[p.Munition]
		move := m.SpeedKmh / 60 * dtMin
		for move > 0 && len(p.Path) > 0 {
			t := p.Path[0]
			d := dist(p.X, p.Y, t.X, t.Y)
			if d > 0 {
				p.Heading = math.Atan2(t.Y-p.Y, t.X-p.X)
			}
			if d <= move {
				p.X, p.Y = t.X, t.Y
				p.Traveled += d
				move -= d
				p.Path = p.Path[1:]
			} else {
				p.X += (t.X - p.X) / d * move
				p.Y += (t.Y - p.Y) / d * move
				p.Traveled += move
				move = 0
			}
		}
		if len(p.Path) > 0 {
			continue
		}
		if m.Kind == "recon" {
			if !p.Returning {
				p.Returning = true
				p.Path = []Pt{p.Home}
				continue
			}
			w.Sides[p.Side].Stocks[p.Munition]++
			delete(w.Projs, id)
			continue
		}
		w.impact(p, m)
		w.groupDone(p.Group, true)
		delete(w.Projs, id)
	}
}

// rebAt — сила РЭБ противника стороны s в точке.
func (w *World) rebAt(s int, x, y float64) float64 {
	best := 0.0
	for _, u := range w.Units {
		if u.Side == s || u.State != UnitDeployed {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.RebKm > 0 && dist(u.X, u.Y, x, y) <= ut.RebKm {
			best = math.Max(best, ut.RebPower*(1+w.Sides[u.Side].eff("reb_power")))
		}
	}
	return math.Min(0.9, best*(1-w.Sides[s].eff("reb_resist")))
}

// impact — попадание.
func (w *World) impact(p *Projectile, m *data.MunitionType) {
	if m.Kind == "decoy" {
		return
	}
	acc := m.Accuracy
	jam := 0.0
	if m.GPS {
		jam = w.rebAt(p.Side, p.X, p.Y)
		acc *= 1 - jam
	}
	if w.rng.Float64() > acc {
		if jam > 0 {
			w.addBonus(p.Side, "drones", w.cat.Rules.ExperiencePoints)
		}
		return
	}
	hit := false
	for _, b := range w.Buildings {
		if b.Side == p.Side {
			continue
		}
		if dist(b.X, b.Y, p.X, p.Y) > m.BlastKm+0.8 {
			continue
		}
		bt := w.cat.BuildingByID[b.Type]
		if bt.Untargetable {
			continue
		}
		hit = true
		before := b.HP
		b.HP = math.Max(0, b.HP-m.Damage)
		w.missionBuildingHit(p.Side, b, before)
		w.socialPost(p.Side, b)
		// Вывод из строя ключевого объекта поднимает мораль атакующего и
		// бьёт по морали владельца.
		if bt.Key && before > b.MaxHP*0.1 && b.HP <= b.MaxHP*0.1 && !b.KeyHit {
			r := w.cat.Rules
			b.KeyHit = true // пока объект не починен, повторный удар мораль не меняет
			w.addMorale(p.Side, w.moraleGain(p.Side, "key", r.MoraleKeyHit))
			w.addMorale(b.Side, -r.MoraleKeyLoss)
		}
		if bt.Aircraft != nil && b.Aircraft != nil {
			for k, n := range b.Aircraft {
				b.Aircraft[k] = math.Max(0, n-n*m.Damage/b.MaxHP*0.6)
			}
		}
		lvl := 1
		if b.HP <= 0 && before > 0 {
			lvl = 2
		}
		w.LogAt(b.Side, lvl, fmt.Sprintf("Попадание (%s): %s — %.0f%%", m.Name, b.Name, b.frac()*100), b.X, b.Y)
	}
	for id, u := range w.Units {
		if u.Side == p.Side || dist(u.X, u.Y, p.X, p.Y) > m.BlastKm+0.3 {
			continue
		}
		hit = true
		u.HP -= m.Damage
		ut := w.cat.UnitByID[u.Type]
		if u.HP <= 0 {
			w.LogAt(u.Side, 2, "Уничтожен: "+ut.Name, u.X, u.Y)
			delete(w.Units, id)
			w.sens.valid = false
		} else {
			w.LogAt(u.Side, 1, "Повреждён: "+ut.Name, u.X, u.Y)
		}
	}
	_ = hit
}

// groupDone учитывает судьбу боеприпаса и по завершении удара сообщает
// атакующему, сколько долетело (ущерб он узнает только разведкой).
func (w *World) groupDone(id uint32, arrived bool) {
	g := w.Groups[id]
	if g == nil {
		return
	}
	if arrived {
		g.Arrived++
	} else {
		g.Downed++
	}
	if g.Arrived+g.Downed < g.Total {
		return
	}
	delete(w.Groups, id)
	if r := w.cat.Rules; g.Arrived == 0 && g.Total >= r.MoraleRepelMin {
		// Полностью отбитый массированный удар поднимает мораль обороны.
		w.addMorale(1-g.Side, w.moraleGain(1-g.Side, "repel", r.MoraleRepel))
		w.addMorale(g.Side, -r.MoraleRepel/2)
		w.Log(1-g.Side, 1, "Массированный удар полностью отбит: мораль растёт")
	}
	name := w.cat.MunitionByID[g.Munition].Name
	lvl := 1 // итог удара всегда всплывает на экране
	if g.Arrived == 0 {
		lvl = 2
	}
	w.LogAt(g.Side, lvl, fmt.Sprintf("Итог удара (%s): долетело %d из %d. Оценка ущерба — по данным разведки.", name, g.Arrived, g.Total), g.X, g.Y)
}
