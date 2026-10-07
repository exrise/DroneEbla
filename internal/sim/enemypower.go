package sim

import (
	"sort"
)

// EnemyRegionPower — оценка энергобаланса области противника по разведданным.
type EnemyRegionPower struct {
	Region  int
	Name    string
	Gen     float64 // МВт, по известным электростанциям и их известному состоянию
	Use     float64 // МВт, по известным городам и потребителям
	Frac    float64 // оценка обеспеченности, 0..1
	Sources int     // сколько известных объектов дали оценку
	Seen    float64 // время самой старой использованной метки, мин (-1 — довоенные данные)
}

// estimateEnemyPower оценивает энергобаланс противника стороны s только по тому, что она знает:
// метки станций, подстанций и потребителей с известным типом (и известной долей HP) плюс города
// на территории противника. Неизвестное в оценку не входит.
func (w *World) estimateEnemyPower(s int) (regs []EnemyRegionPower, gen, use float64) {
	sd := w.Sides[s]
	r := w.cat.Rules
	genM, useM, capM := map[int]float64{}, map[int]float64{}, map[int]float64{}
	src := map[int]int{}
	oldest := map[int]float64{}
	touch := func(reg int, seen float64) {
		src[reg]++
		if o, ok := oldest[reg]; !ok || seen < o {
			oldest[reg] = seen
		}
	}
	for _, c := range sd.Known {
		if c.Kind != 0 || c.Type == "" {
			continue
		}
		bt := w.cat.BuildingByID[c.Type]
		if bt == nil || (bt.Power == 0 && bt.Transfer == 0) {
			continue
		}
		i := w.tileOf(c.X, c.Y)
		if i < 0 || w.OwnerSide(i) != 1-s {
			continue
		}
		reg := int(w.m.Region[i])
		if reg < 0 {
			continue
		}
		hp := c.HP
		if hp < 0 {
			hp = 1
		}
		switch {
		case bt.Power > 0:
			genM[reg] += bt.Power * hp
		case bt.Power < 0:
			useM[reg] += -bt.Power
		}
		if bt.Transfer > 0 {
			capM[reg] += bt.Transfer * hp
		}
		touch(reg, c.Seen)
	}
	for i, ci := range w.cityAt {
		if w.OwnerSide(i) != 1-s {
			continue
		}
		reg := int(w.m.Region[i])
		if reg < 0 {
			continue
		}
		useM[reg] += float64(w.m.Cities[ci].Pop) / 100000 * r.PowerPerCity
	}
	fr := regionFractions(genM, useM, capM)
	for reg, f := range fr {
		if genM[reg] == 0 && useM[reg] == 0 {
			continue
		}
		gen += genM[reg]
		use += useM[reg]
		seen := -1.0
		if o, ok := oldest[reg]; ok {
			seen = o
		}
		regs = append(regs, EnemyRegionPower{Region: reg, Name: w.m.Regions[reg].Name, Gen: genM[reg], Use: useM[reg], Frac: f, Sources: src[reg], Seen: seen})
	}
	sort.Slice(regs, func(a, b int) bool { return regs[a].Name < regs[b].Name })
	return
}
