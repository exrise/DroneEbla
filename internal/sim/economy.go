package sim

import (
	"fmt"
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
)

// baseTransfer — пропускная способность местных сетей области без подстанций, МВт.
const baseTransfer = 400.0

// updatePower считает энергобаланс по областям для обеих сторон.
func (w *World) updatePower() {
	r := w.cat.Rules
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		gen := map[int]float64{}
		use := map[int]float64{}
		capT := map[int]float64{}
		pop := map[int]float64{}
		for _, b := range w.Buildings {
			if b.Side != s || b.Region < 0 {
				continue
			}
			bt := w.cat.BuildingByID[b.Type]
			if bt.Power > 0 {
				gen[b.Region] += bt.Power * b.frac() * (1 + sd.eff("power_gen"))
			} else if bt.Power < 0 && b.Built >= 1 {
				use[b.Region] += -bt.Power
			}
			if bt.Transfer > 0 {
				capT[b.Region] += bt.Transfer * b.frac()
			}
		}
		for i, ci := range w.cityAt {
			if w.OwnerSide(i) != s {
				continue
			}
			c := w.m.Cities[ci]
			reg := int(w.m.Region[i])
			if reg < 0 {
				continue
			}
			p := float64(c.Pop) / 100000
			use[reg] += p * r.PowerPerCity
			pop[reg] += p
		}
		fr := regionFractions(gen, use, capT)
		totalGen, totalUse := 0.0, 0.0
		blackPop, allPop := 0.0, 0.0
		for k, f := range fr {
			totalGen += gen[k]
			totalUse += use[k]
			sd.RegionPower[k] = f
			allPop += pop[k]
			if f < 0.7 {
				blackPop += pop[k] * (1 - f)
			}
		}
		sd.Power = [2]float64{totalGen, totalUse}
		if allPop > 0 {
			sd.Blackout = blackPop / allPop
		} else {
			sd.Blackout = 0
		}
	}
}

// regionFractions — обеспеченность энергией по областям (0..1) при генерации gen, потреблении use
// и пропускной способности подстанций capT. Излишки областей идут в общий котёл, дефицитные
// области получают их пропорционально нехватке.
func regionFractions(gen, use, capT map[int]float64) map[int]float64 {
	regions := map[int]bool{}
	for k := range gen {
		regions[k] = true
	}
	for k := range use {
		regions[k] = true
	}
	pool, demand := 0.0, 0.0
	for k := range regions {
		bal := gen[k] - use[k]
		if bal > 0 {
			pool += math.Min(bal, capT[k]+baseTransfer)
		} else if bal < 0 {
			demand += math.Min(-bal, capT[k]+baseTransfer)
		}
	}
	share := 1.0
	if demand > pool && demand > 0 {
		share = pool / demand
	}
	out := map[int]float64{}
	for k := range regions {
		f := 1.0
		if use[k] > 0 {
			bal := gen[k] - use[k]
			got := gen[k]
			if bal < 0 {
				got += math.Min(-bal, capT[k]+baseTransfer) * share
			} else {
				got = use[k]
			}
			f = clamp(got/use[k], 0, 1)
		}
		out[k] = f
	}
	return out
}

// powerFactor — обеспеченность энергией области здания.
func (w *World) powerFactor(b *Building) float64 {
	bt := w.cat.BuildingByID[b.Type]
	if bt.Power >= 0 {
		return 1
	}
	f, ok := w.Sides[b.Side].RegionPower[b.Region]
	if !ok {
		return 1
	}
	return 0.25 + 0.75*f
}

// requirementFactor — выполнены ли требования к сырью (0..1), из кэша.
func (w *World) requirementFactor(b *Building) float64 {
	if v, ok := w.reqCache[b.ID]; ok {
		return v
	}
	v := w.calcRequirement(b)
	if w.reqCache != nil {
		w.reqCache[b.ID] = v
	}
	return v
}

func (w *World) calcRequirement(b *Building) float64 {
	bt := w.cat.BuildingByID[b.Type]
	f := 1.0
	for dep, km := range bt.NeedDeposit {
		kind := depositKind(dep)
		if kind == 0 {
			continue
		}
		ok := false
		for _, i := range w.depots[kind] {
			if w.OwnerSide(i) != b.Side {
				continue
			}
			if km <= 0 {
				ok = true
				break
			}
			cx, cy := w.m.TileCenter(i%w.m.W, i/w.m.W)
			if dist(cx, cy, b.X, b.Y) <= km {
				ok = true
				break
			}
		}
		if !ok {
			return 0
		}
	}
	if bt.NeedNear != "" {
		best := 0.0
		for _, o := range w.Buildings {
			if o.Side == b.Side && o.Type == bt.NeedNear && dist(o.X, o.Y, b.X, b.Y) <= bt.NearKm {
				best = math.Max(best, o.frac())
			}
		}
		f *= best
	}
	return f
}

func depositKind(s string) uint8 {
	switch s {
	case "oil":
		return 1
	case "gas":
		return 2
	case "coal":
		return 3
	case "ore":
		return 4
	}
	return 0
}

// output — итоговый коэффициент работы здания.
func (w *World) output(b *Building) float64 {
	if !b.Operational() {
		return 0
	}
	sd := w.Sides[b.Side]
	return Efficiency(w.cat.Rules, b.frac()) * MoraleProd(w.cat.Rules, sd.Morale) * w.powerFactor(b) * w.requirementFactor(b) * (1 - sd.LaborLoss)
}

// Efficiency — какая доля производительности остаётся у здания с долей HP h.
// Лёгкие повреждения почти не мешают, тяжёлые резко снижают выпуск, ниже
// порога здание стоит.
func Efficiency(r data.Rules, h float64) float64 {
	x := (h - r.DamageFloor) / (1 - r.DamageFloor)
	if x <= 0 {
		return 0
	}
	return math.Pow(math.Min(1, x), r.DamageCurve)
}

// economy — непрерывная экономика за dtH часов.
func (w *World) economy(dtH float64) {
	r := w.cat.Rules
	w.HourAcc += dtH * 60
	if w.reqCache == nil || w.HourAcc >= 10 {
		w.HourAcc = 0
		w.reqCache = map[uint32]float64{}
	}
	w.updatePower()
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		def := w.cat.Sides[s]
		var delta data.Res
		before := sd.Res

		// Доходы: база, города, экспорт.
		income := def.BaseIncome
		for i, ci := range w.cityAt {
			if w.OwnerSide(i) != s {
				continue
			}
			c := w.m.Cities[ci]
			pf := 1.0
			if reg := int(w.m.Region[i]); reg >= 0 {
				if f, ok := sd.RegionPower[reg]; ok {
					pf = 0.5 + 0.5*f
				}
			}
			income += float64(c.Pop) / 100000 * def.TaxPerCity * pf * MoraleProd(r, sd.Morale)
		}
		income *= 1 - w.Sanction(s, "tax")
		oil, grain := 0.0, 0.0
		portSum, portN := 0.0, 0
		for _, b := range w.Buildings {
			if b.Side != s {
				continue
			}
			bt := w.cat.BuildingByID[b.Type]
			if bt.OilPort {
				portSum += b.frac()
				portN++
			}
			if bt.Export > 0 {
				if b.Type == "grain_port" {
					grain += bt.Export * Efficiency(w.cat.Rules, b.frac())
				} else {
					oil += bt.Export * w.output(b)
				}
			}
		}
		if portN > 0 {
			oil *= 0.3 + 0.7*portSum/float64(portN)
		}
		income += (oil + grain) * (1 - w.Sanction(s, "export"))
		sd.Income = income
		delta[data.ResMoney] += income * dtH

		// Производство ресурсов.
		elecK := 1 - w.Sanction(s, "electronics")
		for _, b := range w.Buildings {
			if b.Side != s {
				continue
			}
			bt := w.cat.BuildingByID[b.Type]
			if len(bt.Produces) == 0 {
				continue
			}
			k := w.output(b) * dtH * b.scale()
			if k <= 0 {
				continue
			}
			cons := data.ToRes(bt.Consumes)
			// Если на потребление не хватает — работаем частично.
			avail := 1.0
			for i := range cons {
				if cons[i] > 0 {
					avail = math.Min(avail, (sd.Res[i]+delta[i])/(cons[i]*k))
				}
			}
			k *= clamp(avail, 0, 1)
			delta.Add(cons, -k)
			prod := data.ToRes(bt.Produces)
			prod[data.ResElectronics] *= elecK
			delta.Add(prod, k)
		}

		// Расход фронта: топливо и боеприпасы.
		postK := []float64{0.6, 1.0, 1.6}
		armor, art := 0.0, 0.0
		for d := 0; d < 3; d++ {
			k := postK[sd.PostureDir[d]]
			armor += sd.Front[d].Armor * k
			art += sd.Front[d].Artillery * k
		}
		if w.War() {
			delta[data.ResFuel] -= armor * r.FrontFuelUse * dtH
			delta[data.ResAmmo] -= art * r.FrontAmmoUse * dtH
		}

		sd.Res.Add(delta, 1)
		for i := range sd.Res {
			if sd.Res[i] < 0 {
				sd.Res[i] = 0
			}
		}
		// Людской поток на фронт.
		if w.War() && sd.People > 0 {
			men := math.Min(def.MenStream*(1+sd.eff("men_stream"))*dtH, sd.People)
			sd.People -= men
			// При низкой морали часть призванных не доходит до фронта.
			w.distributeFront(s, "men", men*MoraleLevy(r, sd.Morale))
		}

		w.production(s, dtH)
		w.construction(s, dtH)
		w.research(s, dtH)
		w.deliveries(s)
		w.morale(s, dtH)
		w.agentsCost(s, dtH)

		// Скорость изменения для интерфейса: фактический чистый поток за шаг (с госзаказом, ремонтом, наукой),
		// сглаженный по окну ~2 игровых часа.
		if dtH > 0 {
			a := math.Min(1, dtH/2)
			for i := range sd.Res {
				sd.Rates[i] += ((sd.Res[i]-before[i])/dtH - sd.Rates[i]) * a
			}
		}
	}
	w.forts(dtH)
}

// capacity — мощности стороны по категориям в час.
func (w *World) capacity(s int) map[string]float64 {
	sd := w.Sides[s]
	c := map[string]float64{}
	for _, b := range w.Buildings {
		if b.Side != s {
			continue
		}
		bt := w.cat.BuildingByID[b.Type]
		if len(bt.Capacity) == 0 {
			continue
		}
		o := w.output(b)
		for k, v := range bt.Capacity {
			c[k] += v * o
		}
	}
	for k := range c {
		c[k] *= 1 + sd.eff("cap_"+k) + sd.eff("cap_all")
	}
	return c
}

// production — госзаказ: мощность категории делится поровну между позициями.
func (w *World) production(s int, dtH float64) {
	sd := w.Sides[s]
	sd.Capacity = w.capacity(s)
	if len(sd.Orders) == 0 {
		return
	}
	count := map[string]int{}
	for _, o := range sd.Orders {
		_, cat, _, _ := w.cat.ItemCost(o.Item)
		count[cat]++
	}
	done := false
	for i := range sd.Orders {
		o := &sd.Orders[i]
		cost, cat, pts, ok := w.cat.ItemCost(o.Item)
		if !ok || pts <= 0 {
			continue
		}
		if w.Cheat {
			cost = data.Res{}
			o.Progress = pts * cheatBatch
		} else {
			o.Progress += sd.Capacity[cat] * dtH / float64(count[cat])
		}
		o.Stalled = false
		for o.Progress >= pts && o.Remaining != 0 {
			if !w.storageOK(s, o.Item) {
				o.Stalled = true
				o.Progress = pts
				break
			}
			if !sd.Res.Covers(cost, 1) {
				o.Stalled = true
				o.Progress = pts
				break
			}
			sd.Res.Add(cost, -1)
			o.Progress -= pts
			w.deliver(s, o.Item, 1, "")
			if o.Remaining > 0 {
				o.Remaining--
				if o.Remaining == 0 {
					done = true
				}
			}
		}
	}
	if done {
		keep := sd.Orders[:0]
		for _, o := range sd.Orders {
			if o.Remaining != 0 {
				keep = append(keep, o)
			}
		}
		sd.Orders = keep
	}
}

func (w *World) storageOK(s int, item string) bool {
	st := w.Sides[s].Storage
	switch item {
	case "armor_storage":
		return st.Armor >= 10
	case "artillery_storage":
		return st.Artillery >= 10
	}
	return true
}

// deliver выдаёт стороне предмет (производство, импорт, помощь).
func (w *World) deliver(s int, item string, amount float64, via string) {
	sd := w.Sides[s]
	if len(item) > 4 && item[:4] == "res:" {
		for i, k := range data.ResKeys {
			if item == "res:"+k {
				sd.Res[i] += amount
			}
		}
		return
	}
	if _, ok := w.cat.MunitionByID[item]; ok {
		sd.Stocks[item] += amount
		return
	}
	if ut, ok := w.cat.UnitByID[item]; ok {
		for n := 0; n < int(amount+0.5); n++ {
			x, y := w.spawnUnit(s, ut, via)
			u := w.addUnit(item, s, x+w.rng.Float64()*4-2, y+w.rng.Float64()*4-2)
			w.LogAt(s, 0, "Поступил на вооружение: "+ut.Name, u.X, u.Y)
		}
		return
	}
	if f, ok := w.cat.FrontByID[item]; ok {
		n := f.Batch * amount
		switch item {
		case "armor":
			w.distributeFront(s, "armor", n)
		case "artillery":
			w.distributeFront(s, "artillery", n)
		case "fpv", "fpv_fiber", "fpv_ai":
			pw := f.Power
			if pw <= 0 {
				pw = 1
			}
			w.distributeFPV(s, n, pw)
		case "armor_storage":
			sd.Storage.Armor -= n
			w.distributeFront(s, "armor", n)
		case "artillery_storage":
			sd.Storage.Artillery -= n
			w.distributeFront(s, "artillery", n)
		}
	}
}

// distributeFront раздаёт пополнение по направлениям согласно долям.
func (w *World) distributeFront(s int, what string, n float64) {
	sd := w.Sides[s]
	sum := sd.Alloc[0] + sd.Alloc[1] + sd.Alloc[2]
	if sum <= 0 {
		sum = 1
		sd.Alloc = [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	}
	for d := 0; d < 3; d++ {
		k := n * sd.Alloc[d] / sum
		f := &sd.Front[d]
		switch what {
		case "men":
			f.Men += k
		case "armor":
			f.Armor += k
		case "artillery":
			f.Artillery += k
		}
	}
}

// distributeFPV раздаёт FPV-дроны по направлениям; power — сила версии (радио 1, оптоволокно, ИИ…).
func (w *World) distributeFPV(s int, n, power float64) {
	sd := w.Sides[s]
	sum := sd.Alloc[0] + sd.Alloc[1] + sd.Alloc[2]
	if sum <= 0 {
		sum = 1
		sd.Alloc = [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	}
	for d := 0; d < 3; d++ {
		k := n * sd.Alloc[d] / sum
		sd.Front[d].FPV += k
		sd.Front[d].FPVPow += k * power
	}
}

// spawnPoint — где появляется новый юнит. Заводы нужного типа и пункты
// въезда чередуются по кругу, чтобы техника не скапливалась в одном месте.
func (w *World) spawnPoint(s int, cat, via string) (float64, float64) {
	if via == "entry" {
		return w.entryPoint(s)
	}
	want := "armor_plant"
	if cat == "air" {
		want = "missile_plant"
	} else if cat == "drone" {
		want = "drone_workshop"
	}
	if x, y, ok := w.spawnAtBuilding(s, want); ok {
		return x, y
	}
	return w.entryPoint(s)
}

// spawnUnit — точка появления юнита: у здания из spawn_at (например, центра подготовки),
// а если такого нет — как у остальных юнитов его категории.
func (w *World) spawnUnit(s int, ut *data.UnitType, via string) (float64, float64) {
	if via != "entry" && ut.SpawnAt != "" {
		if x, y, ok := w.spawnAtBuilding(s, ut.SpawnAt); ok {
			return x, y
		}
	}
	return w.spawnPoint(s, ut.Cap, via)
}

func (w *World) entryPoint(s int) (float64, float64) {
	def := w.cat.Sides[s]
	n := w.nextSpawn(s)
	if len(def.Entries) > 0 {
		e := def.Entries[n%len(def.Entries)]
		return w.m.Project(e.Lon, e.Lat)
	}
	return w.m.Project(def.EntryLon, def.EntryLat)
}

// spawnAtBuilding — по очереди работающие здания типа want на своей территории.
func (w *World) spawnAtBuilding(s int, want string) (float64, float64, bool) {
	var plants []*Building
	for _, b := range w.Buildings {
		if b.Side == s && b.Type == want && b.Operational() && w.sideOfPoint(b.X, b.Y) == s {
			plants = append(plants, b)
		}
	}
	if len(plants) == 0 {
		return 0, 0, false
	}
	sort.Slice(plants, func(a, b int) bool { return plants[a].ID < plants[b].ID })
	b := plants[w.nextSpawn(s)%len(plants)]
	return b.X, b.Y, true
}

func (w *World) nextSpawn(s int) int {
	n := w.SpawnN[s]
	w.SpawnN[s]++
	return n
}

// construction — стройка, ремонт, пополнение пусков, авиация.
func (w *World) construction(s int, dtH float64) {
	w.repairs(s, dtH)
	for _, b := range w.Buildings {
		if b.Side != s {
			continue
		}
		bt := w.cat.BuildingByID[b.Type]
		if b.Built < 1 {
			if bt.BuildHours > 0 {
				b.Built = math.Min(1, b.Built+dtH/bt.BuildHours)
			} else {
				b.Built = 1
			}
			if b.Built >= 1 {
				w.LogAt(s, 0, "Строительство завершено: "+b.Name, b.X, b.Y)
			}
			continue
		}
		if bt.LaunchRate > 0 {
			b.Budget = math.Min(w.launchCap(b), b.Budget+bt.LaunchRate*dtH)
		}
	}
}

// repairs — ремонт повреждённых зданий: ограниченное число бригад, у каждого
// типа свой срок; сначала чинятся менее повреждённые (быстрее вернуть в строй).
func (w *World) repairs(s int, dtH float64) {
	sd := w.Sides[s]
	r := w.cat.Rules
	var list []*Building
	for _, b := range w.Buildings {
		if b.Side != s {
			continue
		}
		b.Repairing = false
		if b.Built >= 1 && b.Repair && b.HP < b.MaxHP-0.01 {
			list = append(list, b)
		}
	}
	sort.Slice(list, func(a, c int) bool {
		fa, fc := list[a].frac(), list[c].frac()
		if fa != fc {
			return fa > fc
		}
		return list[a].ID < list[c].ID
	})
	crews := r.RepairCrews
	perType := map[string]int{}
	for _, b := range list {
		if crews == 0 {
			break
		}
		// Однотипные здания делят специалистов и оборудование: одновременно
		// чинится не больше repair_per_type зданий каждого типа.
		if perType[b.Type] >= r.RepairPerType {
			continue
		}
		bt := w.cat.BuildingByID[b.Type]
		rate := b.MaxHP / bt.RepairHrs() * (1 + sd.eff("repair_speed"))
		if b.frac() < r.DamageFloor {
			rate *= r.RepairRuinMult
		}
		hp := math.Min(b.MaxHP-b.HP, rate*dtH)
		cost := repairCost(bt, hp/b.MaxHP, r.RepairCostK)
		if !sd.Res.Covers(cost, 1) {
			continue
		}
		sd.Res.Add(cost, -1)
		b.HP += hp
		if b.KeyHit && b.HP > b.MaxHP*0.5 {
			b.KeyHit = false // починен достаточно: новый вывод из строя снова считается
		}
		b.Repairing = true
		perType[b.Type]++
		crews--
	}
}

// repairCost — стоимость восстановления доли frac HP.
func repairCost(bt *data.BuildingType, frac, k float64) data.Res {
	c := data.ToRes(bt.Cost)
	if c[data.ResMoney] == 0 && c[data.ResSteel] == 0 {
		c = data.Res{100, 0, 60, 5, 0}
	}
	var out data.Res
	out.Add(c, frac*k)
	return out
}

// launchCap — сколько пусков можно накопить.
func (w *World) launchCap(b *Building) float64 {
	bt := w.cat.BuildingByID[b.Type]
	c := bt.LaunchRate
	if n := b.Aircraft["strategic"]; bt.Aircraft["strategic"] > 0 {
		c = math.Min(c, n*4)
	}
	return c * b.frac()
}

// forts — постройка укреплений.
func (w *World) forts(dtH float64) {
	r := w.cat.Rules
	for i, p := range w.FortJobs {
		p += dtH / r.FortHours
		if p >= 1 {
			if w.Fort[i] < 3 {
				w.Fort[i]++
			}
			delete(w.FortJobs, i)
			continue
		}
		w.FortJobs[i] = p
	}
}

// deliveries — импорт, помощь.
func (w *World) deliveries(s int) {
	sd := w.Sides[s]
	keep := sd.Deliveries[:0]
	for _, d := range sd.Deliveries {
		if w.Time >= d.At {
			w.deliver(s, d.Item, d.Amount, "entry")
			w.Log(s, 1, "Поставка прибыла: "+d.Name)
		} else {
			keep = append(keep, d)
		}
	}
	sd.Deliveries = keep

	if !w.War() {
		return
	}
	if sd.StartAD == 0 {
		sd.StartAD = w.heavyAD(s)
	}
	w.sanctionsTick(s)
	w.airspaceTick(s)
	w.autoImportTick(s)
	w.interceptorStocks(s)
	h := w.HoursSinceWar()
	for _, a := range w.cat.Sides[s].Aid {
		if a.Mission != nil {
			w.missionTick(s, a) // выдаётся только за выполнение задания
			continue
		}
		if sd.AidDone[a.ID] || h < a.AtHour {
			continue
		}
		if sd.Morale < a.MinMorale {
			continue
		}
		if a.NeedKyiv && w.kyiv >= 0 && w.OwnerSide(w.kyiv) != s {
			continue
		}
		if a.MinADLoss > 0 && w.adLoss(s) < a.MinADLoss {
			continue
		}
		w.grantAid(s, a, "Пакет помощи: ")
	}
}

// grantAid выдаёт пакет помощи.
func (w *World) grantAid(s int, a data.AidPackage, prefix string) {
	sd := w.Sides[s]
	sd.AidDone[a.ID] = true
	for id, n := range a.Items {
		w.deliver(s, id, n, "entry")
	}
	if a.Morale > 0 {
		w.addMorale(s, w.moraleGain(s, "aid", a.Morale))
	} else {
		w.addMorale(s, a.Morale)
	}
	name := a.Name
	if a.Title != "" {
		name = a.Title
	}
	w.Log(s, 1, prefix+name)
}

// morale — изменение морали.
func (w *World) morale(s int, dtH float64) {
	sd := w.Sides[s]
	r := w.cat.Rules
	if !w.War() {
		return
	}
	d := 0.0
	if sd.Blackout > 0.05 {
		d -= r.MoraleBlackout * sd.Blackout * 10
	} else {
		d += r.MoraleRecover
	}
	if sd.Res[data.ResMoney] < 1 {
		d -= r.MoraleDebt
	}
	sd.Morale = clamp(sd.Morale+d*dtH, 0, 100)
	// потери людей
	if sd.LossAcc > 0 {
		sd.Morale = clamp(sd.Morale-sd.LossAcc/10*r.MoraleLossPer10k, 0, 100)
		sd.LossAcc = 0
	}
}

func (w *World) agentsCost(s int, dtH float64) {
	sd := w.Sides[s]
	if sd.AgentFund {
		c := w.cat.Rules.AgentFundCost * dtH
		if sd.Res[data.ResMoney] >= c {
			sd.Res[data.ResMoney] -= c
		} else {
			sd.AgentFund = false
		}
	}
	if sd.ResFund > 0 {
		c := w.cat.Rules.ResearchFundCost * float64(sd.ResFund) * dtH
		if sd.Res[data.ResMoney] >= c {
			sd.Res[data.ResMoney] -= c
		} else {
			sd.ResFund = 0
			w.Log(s, 1, "Не хватает денег на финансирование исследований")
		}
	}
}

// ItemAvailable — доступен ли предмет для госзаказа.
func (w *World) ItemAvailable(s int, id string) bool { return w.Sides[s].Unlocked[id] }

func fmtRes(r data.Res) string {
	out := ""
	for i, v := range r {
		if v > 0 {
			if out != "" {
				out += ", "
			}
			out += fmt.Sprintf("%s %.0f", data.ResNames[i], v)
		}
	}
	return out
}
