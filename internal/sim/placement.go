package sim

import (
	"fmt"
	"sort"

	"github.com/exrise/droneebla/internal/world"
)

// Расстановка перед стартом: стартовые комплексы ПВО, РЛС, РЭБ, РТР и
// пусковые (виды из rules.placement_kinds) плюс резерв площадок БПЛА выдаются
// игроку, который сам ставит их на своей территории. Пока идёт расстановка,
// время стоит; партия стартует, когда обе стороны нажали «Готово».

const placeFrontKm = 15 // не ближе к фронту

// StartPlacement переводит мир в режим расстановки (вызывается до первого шага).
func (w *World) StartPlacement() {
	if w.Placement || w.PlacementDone || w.Time > 0 {
		return
	}
	kinds := map[string]bool{}
	for _, k := range w.cat.Rules.PlacementKinds {
		kinds[k] = true
	}
	var ids []uint32
	for id, u := range w.Units {
		if kinds[w.cat.UnitByID[u.Type].Kind] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	for _, id := range ids {
		u := w.Units[id]
		sd := w.Sides[u.Side]
		sd.Reserve[u.Type]++
		sd.Hints = append(sd.Hints, PlaceHint{Type: u.Type, X: u.X, Y: u.Y})
		if ut := w.cat.UnitByID[u.Type]; ut.Interceptor != "" {
			sd.Stocks[ut.Interceptor] += u.Ready // ракеты вернулись на склад
		}
		delete(w.Units, id)
	}
	// Стартовые здания выбранных типов (центры подготовки) тоже ставит игрок:
	// настоящие места остаются подсказками для ИИ.
	bkinds := map[string]bool{}
	for _, k := range w.cat.Rules.PlacementBuildings {
		bkinds[k] = true
	}
	var bids []uint32
	for id, b := range w.Buildings {
		if bkinds[b.Type] && !b.Placed {
			bids = append(bids, id)
		}
	}
	sort.Slice(bids, func(a, b int) bool { return bids[a] < bids[b] })
	for _, id := range bids {
		b := w.Buildings[id]
		sd := w.Sides[b.Side]
		sd.Reserve[b.Type]++
		sd.Hints = append(sd.Hints, PlaceHint{Type: b.Type, X: b.X, Y: b.Y})
		delete(w.Buildings, id)
	}
	for s := 0; s < 2; s++ {
		for t, n := range w.cat.Sides[s].ReserveBuildings {
			w.Sides[s].Reserve[t] += n
		}
	}
	w.Placement = true
	for s := 0; s < 2; s++ {
		w.Log(s, 1, "Расстановка: поставьте выданные комплексы ПВО и площадки на своей территории и нажмите «Готово». Война начнётся после готовности обеих сторон.")
	}
}

func (w *World) isBuildingType(id string) bool { _, ok := w.cat.BuildingByID[id]; return ok }

// CanPlace проверяет, можно ли поставить объект из резерва в точку.
func (w *World) CanPlace(s int, typ string, x, y float64) string {
	if !w.Placement {
		return "Расстановка уже завершена"
	}
	sd := w.Sides[s]
	if sd.Reserve[typ] <= 0 {
		return "Этого нет в резерве"
	}
	i := w.tileOf(x, y)
	if i < 0 || w.OwnerSide(i) != s || w.m.Terrain[i] != world.TerrainLand {
		return "Ставить можно только на своей суше"
	}
	for _, f := range w.frontT[s] {
		fx, fy := w.m.TileCenter(f%w.m.W, f/w.m.W)
		if dist(fx, fy, x, y) < placeFrontKm {
			return fmt.Sprintf("Слишком близко к фронту (менее %d км)", placeFrontKm)
		}
	}
	return ""
}

func (w *World) place(s int, typ string, x, y float64) string {
	if w.Sides[s].Ready {
		return "Вы уже нажали «Готово»"
	}
	if e := w.CanPlace(s, typ, x, y); e != "" {
		return e
	}
	sd := w.Sides[s]
	sd.Reserve[typ]--
	if w.isBuildingType(typ) {
		b := w.addBuilding(typ, s, x, y, 1)
		b.Placed = true
		return ""
	}
	u := w.addUnit(typ, s, x, y)
	u.Placed = true
	return ""
}

func (w *World) unplace(s int, id uint32) string {
	if !w.Placement || w.Sides[s].Ready {
		return "Сейчас нельзя убрать объект"
	}
	sd := w.Sides[s]
	if u, ok := w.Units[id]; ok && u.Side == s && u.Placed {
		if ut := w.cat.UnitByID[u.Type]; ut.Interceptor != "" {
			sd.Stocks[ut.Interceptor] += u.Ready
		}
		sd.Reserve[u.Type]++
		delete(w.Units, id)
		return ""
	}
	if b, ok := w.Buildings[id]; ok && b.Side == s && b.Placed {
		sd.Reserve[b.Type]++
		delete(w.Buildings, id)
		return ""
	}
	return "Этот объект убрать нельзя"
}

// ready — готовность стороны к старту; когда готовы обе, расстановка заканчивается.
func (w *World) ready(s int, on bool) string {
	if !w.Placement {
		return ""
	}
	sd := w.Sides[s]
	if on {
		left := 0
		for _, n := range sd.Reserve {
			left += n
		}
		if left > 0 {
			return fmt.Sprintf("Расставьте весь резерв (осталось %d)", left)
		}
	}
	sd.Ready = on
	if w.Sides[0].Ready && w.Sides[1].Ready {
		w.Placement, w.PlacementDone = false, true
		for k := 0; k < 2; k++ {
			w.Sides[k].Ready = false
			w.Log(k, 1, "Расстановка завершена. Подготовительная фаза началась.")
		}
		w.updateFrontTiles()
	}
	return ""
}

// PlacementLeft — сколько объектов осталось расставить у стороны.
func (w *World) PlacementLeft(s int) int {
	n := 0
	for _, c := range w.Sides[s].Reserve {
		n += c
	}
	return n
}
