package sim

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/exrise/droneebla/internal/data"
)

// Склады: вместимость по ресурсам (fuel, steel, ammo, electronics, единицы) и «арсенал» для ракет, дронов и ЗУР
// (очки мощности: размер боеприпаса — его cap_points). Вместимость — сумма целых складов (с учётом повреждений) плюс
// небольшая полевая база; запас сверх вместимости ресурсов сгорает, заказы на боеприпасы при полном арсенале стоят,
// а при падении вместимости (удар по складу) излишек арсенала сгорает.

// storageRes — ключ вместимости для ресурса; пусто — ресурс не хранится на складах (деньги).
func storageRes(i int) string {
	switch i {
	case data.ResFuel:
		return "fuel"
	case data.ResSteel:
		return "steel"
	case data.ResAmmo:
		return "ammo"
	case data.ResElectronics:
		return "electronics"
	}
	return ""
}

// storageCaps — вместимость складов стороны по ключам.
func (w *World) storageCaps(s int) map[string]float64 {
	sd := w.Sides[s]
	out := map[string]float64{}
	for k, v := range w.cat.Rules.StorageBase {
		out[k] = v
	}
	for _, b := range w.buildingsByID() {
		if b.Side != s || b.Built < 1 {
			continue
		}
		bt := w.cat.BuildingByID[b.Type]
		if len(bt.Storage) == 0 {
			continue
		}
		e := Efficiency(w.cat.Rules, b.frac()) * b.scale()
		for k, v := range bt.Storage {
			out[k] += v * e
		}
	}
	if k := sd.eff("storage_cap"); k != 0 {
		for key := range out {
			out[key] *= 1 + k
		}
	}
	return out
}

// munSize — сколько места в арсенале занимает один боеприпас.
func (w *World) munSize(id string) float64 {
	m := w.cat.MunitionByID[id]
	if m == nil {
		return 0
	}
	return math.Max(m.CapPoints, 0.2)
}

// ArsenalUsed — занято в арсенале стороны.
func (w *World) ArsenalUsed(s int) float64 {
	used := 0.0
	for id, n := range w.Sides[s].Stocks {
		used += n * w.munSize(id)
	}
	return used
}

// arsenalBlocks — полный ли арсенал для выпуска очередной партии боеприпаса item.
func (w *World) arsenalBlocks(s int, item string) bool {
	if w.Cheat || w.cat.MunitionByID[item] == nil {
		return false
	}
	capA := w.Sides[s].StorageCap["arsenal"]
	return w.ArsenalUsed(s)+w.cat.ItemBatch(item)*w.munSize(item) > capA
}

// clampStorage сжигает запас ресурсов сверх вместимости; потери копятся и раз в 6 игровых часов попадают в журнал.
func (w *World) clampStorage(s int) {
	sd := w.Sides[s]
	for i := range sd.Res {
		key := storageRes(i)
		if key == "" {
			continue
		}
		if capK := sd.StorageCap[key]; sd.Res[i] > capK {
			sd.StorageLost[i] += sd.Res[i] - capK
			sd.Res[i] = capK
		}
	}
	if w.Time-sd.StorageLogAt < 360 {
		return
	}
	var parts []string
	for i, v := range sd.StorageLost {
		if v >= 20 {
			parts = append(parts, fmt.Sprintf("%s %.0f", strings.ToLower(data.ResNames[i]), v))
		}
	}
	if len(parts) > 0 {
		w.Log(s, 1, "Склады переполнены, запас сгорает: "+strings.Join(parts, ", ")+". Постройте склады или расходуйте запасы")
		sd.StorageLost = data.Res{}
	}
	sd.StorageLogAt = w.Time
}

// enforceArsenal: если вместимость арсенала упала (удар по складу), излишек боеприпасов пропорционально сгорает.
func (w *World) enforceArsenal(s int) {
	sd := w.Sides[s]
	capA := sd.StorageCap["arsenal"]
	prev := sd.ArsenalCapSeen
	sd.ArsenalCapSeen = capA
	if prev <= 0 || capA >= prev-1e-6 {
		return
	}
	used := w.ArsenalUsed(s)
	if used <= capA || used <= 0 {
		return
	}
	k := capA / used
	ids := make([]string, 0, len(sd.Stocks))
	for id := range sd.Stocks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lost := 0.0
	for _, id := range ids {
		n := sd.Stocks[id]
		keep := math.Floor(n * k)
		lost += (n - keep) * w.munSize(id)
		sd.Stocks[id] = keep
	}
	w.Log(s, 2, fmt.Sprintf("Удар по арсеналам: сгорело боеприпасов на %.0f очков мощности из %.0f", lost, used))
}

// importFits — помещается ли партия ресурса im на склады с учётом заказанного, но ещё не прибывшего.
func (w *World) importFits(s int, im data.ImportOffer) (bool, string) {
	if len(im.Item) <= 4 || im.Item[:4] != "res:" {
		return true, ""
	}
	sd := w.Sides[s]
	for i, k := range data.ResKeys {
		if im.Item != "res:"+k {
			continue
		}
		key := storageRes(i)
		if key == "" {
			return true, ""
		}
		have := sd.Res[i]
		for _, d := range sd.Deliveries {
			if d.Item == im.Item {
				have += d.Amount
			}
		}
		if capK := sd.StorageCap[key]; have+im.Amount > capK {
			return false, fmt.Sprintf("Склады заполнены: вместимость %.0f, с заказанным %.0f. Постройте склад или расходуйте запас", capK, have)
		}
	}
	return true, ""
}
