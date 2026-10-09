package sim

import (
	"fmt"
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
)

// lowThresholds — порог «мало ЗУР» по каждому виду ракет, которыми стреляют комплексы стороны:
// не меньше rules.low_interceptor и не меньше четырёх магазинов самого заряженного комплекса.
func (w *World) lowThresholds(s int) map[string]float64 {
	out := map[string]float64{}
	for _, u := range w.Units {
		if u.Side != s {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut == nil || ut.Kind != "ad" || ut.Interceptor == "" {
			continue
		}
		t := math.Max(w.cat.Rules.LowInterceptor, 4*float64(ut.Magazine))
		if t > out[ut.Interceptor] {
			out[ut.Interceptor] = t
		}
	}
	return out
}

// interceptorStocks следит за запасом ЗУР: предупреждает один раз, когда ракет мало и когда они кончились,
// и по желанию игрока заказывает партию в госзаказ (keep_stock).
func (w *World) interceptorStocks(s int) {
	sd := w.Sides[s]
	low := w.lowThresholds(s)
	ids := make([]string, 0, len(low))
	for id := range low {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		m := w.cat.MunitionByID[id]
		stock := sd.Stocks[id]
		units := 0
		for _, u := range w.Units {
			if u.Side == s && w.cat.UnitByID[u.Type].Interceptor == id {
				units++
			}
		}
		switch {
		case stock < 1 && sd.LowAlert[id] < 2:
			sd.LowAlert[id] = 2
			w.Log(s, 2, fmt.Sprintf("Кончились ракеты %s: %d комплексов без боезапаса. Закажите в госзаказе или включите автозаказ в «Арсенале»", m.Name, units))
		case stock <= low[id] && stock >= 1 && sd.LowAlert[id] < 1:
			sd.LowAlert[id] = 1
			w.Log(s, 2, fmt.Sprintf("Заканчиваются ракеты %s: осталось %.0f, комплексов %d", m.Name, stock, units))
		case stock > low[id]*1.5 && sd.LowAlert[id] != 0:
			sd.LowAlert[id] = 0
		}
		if sd.KeepStock[id] && stock <= low[id] && sd.Unlocked[id] {
			have := false
			for _, o := range sd.Orders {
				if o.Item == id {
					have = true
				}
			}
			if !have {
				sd.Orders = append(sd.Orders, Order{Item: id, Remaining: w.cat.Rules.KeepStockBatch})
				w.Log(s, 0, fmt.Sprintf("Автозаказ: %s ×%d", m.Name, w.cat.Rules.KeepStockBatch))
			}
		}
	}
}

// sell продаёт излишки топлива или стали за деньги (невыгодный курс); не трогает остаток rules.sell_keep_min.
// count: 0 — всё сверх остатка.
func (w *World) sell(s int, item string, count int) string {
	sd := w.Sides[s]
	r := w.cat.Rules
	idx := -1
	key := ""
	for i, k := range data.ResKeys {
		if item == "res:"+k {
			idx, key = i, k
		}
	}
	rate, ok := r.SellRate[key]
	if idx < 0 || !ok {
		return "Этот ресурс продавать нельзя"
	}
	room := sd.Res[idx] - r.SellKeepMin
	if room < 1 {
		return fmt.Sprintf("Продать нечего: остаток ниже %.0f", r.SellKeepMin)
	}
	n := room
	if count > 0 {
		n = math.Min(room, float64(count))
	}
	n = math.Floor(n)
	sd.Res[idx] -= n
	sd.Res[data.ResMoney] += n * rate
	w.Log(s, 0, fmt.Sprintf("Продано: %s %.0f → %.0f денег", data.ResNames[idx], n, n*rate))
	return ""
}

// heavyAD — сколько у стороны комплексов тяжёлой ПВО (дорогие зенитные комплексы с ракетами).
func (w *World) heavyAD(s int) int {
	n := 0
	for _, u := range w.Units {
		if u.Side != s {
			continue
		}
		if ut := w.cat.UnitByID[u.Type]; ut != nil && ut.Kind == "ad" && ut.Interceptor != "" && ut.Cost["money"] >= 100 {
			n++
		}
	}
	return n
}

// adLoss — доля потерянной тяжёлой ПВО относительно парка на начало войны.
func (w *World) adLoss(s int) float64 {
	st := w.Sides[s].StartAD
	if st <= 0 {
		return 0
	}
	return math.Max(0, 1-float64(w.heavyAD(s))/float64(st))
}
