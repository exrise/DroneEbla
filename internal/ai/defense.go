package ai

import (
	"sort"

	"github.com/exrise/droneebla/internal/sim"
)

// defense — прикрытие ценных объектов ПВО и очерёдность ремонта.
func (a *AI) defense(w *sim.World, v *sim.View) {
	if a.due("defense", v.Time, a.cfg.AdEveryMin) {
		a.coverKeyObjects(w, v)
	}
	a.repairOrder(w, v)
}

func (a *AI) desiredCover(weight float64) int {
	if weight >= 8 {
		return 2
	}
	return 1
}

// coverKeyObjects переставляет ПВО к объектам, у которых прикрытия нет.
func (a *AI) coverKeyObjects(w *sim.World, v *sim.View) {
	type ad struct {
		u    sim.Unit
		x, y float64 // где юнит будет стоять (конец маршрута, если он в пути)
		rng  float64
	}
	var units []ad
	for _, u := range v.Units {
		ut := a.cat.UnitByID[u.Type]
		if ut != nil && ut.Kind == "ad" && ut.RangeKm > 0 {
			x, y := u.X, u.Y
			if n := len(u.Path); n > 0 {
				x, y = u.Path[n-1].X, u.Path[n-1].Y
			}
			units = append(units, ad{u, x, y, ut.RangeKm * 0.8})
		}
	}
	if len(units) == 0 {
		return
	}
	var keys []sim.Building
	for _, b := range v.Buildings {
		if b.Built >= 1 && a.cfg.Protect[b.Type] > 0 {
			keys = append(keys, b)
		}
	}
	covers := func(i int, b sim.Building) bool {
		return dist(units[i].x, units[i].y, b.X, b.Y) <= units[i].rng
	}
	cover := make([]int, len(keys))
	for k, b := range keys {
		for i := range units {
			if covers(i, b) {
				cover[k]++
			}
		}
	}
	// Самые ценные объекты без нужного прикрытия.
	var need []int
	for k, b := range keys {
		if cover[k] < a.desiredCover(a.cfg.Protect[b.Type]) {
			need = append(need, k)
		}
	}
	sort.Slice(need, func(i, j int) bool {
		wi, wj := a.cfg.Protect[keys[need[i]].Type], a.cfg.Protect[keys[need[j]].Type]
		if wi != wj {
			return wi > wj
		}
		return keys[need[i]].ID < keys[need[j]].ID
	})
	moves := 0
	used := map[int]bool{}
	for _, k := range need {
		if moves >= a.cfg.AdMovesMax {
			break
		}
		b := keys[k]
		best, bestD := -1, a.cfg.AdMoveMaxKm
		for i := range units {
			u := units[i].u
			if used[i] || u.State != sim.UnitDeployed || v.Time-a.lastMove[u.ID] < a.cfg.AdMoveCooldown {
				continue
			}
			if d := dist(u.X, u.Y, b.X, b.Y); d < bestD && !a.leavesGap(units[i].u, i, keys, cover, covers) {
				best, bestD = i, d
			}
		}
		if best < 0 {
			continue
		}
		u := units[best].u
		if a.cmd(w, sim.Command{Kind: sim.CmdMove, ID: u.ID, X: b.X, Y: b.Y}) == "" {
			a.lastMove[u.ID] = v.Time
			used[best] = true
			moves++
		}
	}
}

// leavesGap — оставит ли уход юнита i без нужного прикрытия объект, который он сейчас прикрывает.
func (a *AI) leavesGap(u sim.Unit, i int, keys []sim.Building, cover []int, covers func(int, sim.Building) bool) bool {
	for k, b := range keys {
		if covers(i, b) && cover[k]-1 < a.desiredCover(a.cfg.Protect[b.Type]) {
			return true
		}
	}
	return false
}

// repairOrder: пока ждут важные здания, малоценные не чинятся.
func (a *AI) repairOrder(w *sim.World, v *sim.View) {
	if a.cfg.RepairMinWeight <= 0 {
		return
	}
	keyWaiting := false
	for _, b := range v.Buildings {
		if b.Built >= 1 && b.Repair && !b.Repairing && b.HP < b.MaxHP-0.01 && a.cfg.Protect[b.Type] >= a.cfg.RepairMinWeight {
			keyWaiting = true
			break
		}
	}
	for _, b := range v.Buildings {
		if b.Built < 1 || b.HP >= b.MaxHP-0.01 {
			if a.repairOff[b.ID] {
				a.cmd(w, sim.Command{Kind: sim.CmdRepair, ID: b.ID, Int: 1})
				delete(a.repairOff, b.ID)
			}
			continue
		}
		low := a.cfg.Protect[b.Type] < a.cfg.RepairMinWeight
		switch {
		case keyWaiting && low && b.Repair && !b.Repairing:
			if a.cmd(w, sim.Command{Kind: sim.CmdRepair, ID: b.ID, Int: 0}) == "" {
				a.repairOff[b.ID] = true
			}
		case !keyWaiting && a.repairOff[b.ID]:
			a.cmd(w, sim.Command{Kind: sim.CmdRepair, ID: b.ID, Int: 1})
			delete(a.repairOff, b.ID)
		}
	}
}
