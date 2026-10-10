package sim

import "math"

// Импорт идёт через пункты въезда стороны (sides.json: entries). Их проходимость — минимум из энергообеспеченности области
// пункта и доли целых железнодорожных узлов рядом; общая логистика — среднее по пунктам. От неё зависит срок поставок.

// importBlockK — ниже этой проходимости поставки не принимаются.
const importBlockK = 0.15

// importRailKm — радиус ж/д узлов, обслуживающих пункт въезда.
const importRailKm = 120.0

// importDelayK — во сколько раз растёт срок поставки при проходимости k.
func importDelayK(k float64) float64 { return 1 / (0.3 + 0.7*math.Max(0, math.Min(1, k))) }

// ImportLogistics — проходимость пунктов въезда стороны 0..1 (для интерфейса).
func (w *World) ImportLogistics(s int) float64 { return w.importLogistics(s) }

func (w *World) importLogistics(s int) float64 {
	def := w.cat.Sides[s]
	sd := w.Sides[s]
	type pt struct{ x, y float64 }
	var pts []pt
	for _, e := range def.Entries {
		x, y := w.m.Project(e.Lon, e.Lat)
		pts = append(pts, pt{x, y})
	}
	if len(pts) == 0 {
		x, y := w.m.Project(def.EntryLon, def.EntryLat)
		pts = append(pts, pt{x, y})
	}
	sum := 0.0
	for _, p := range pts {
		k := 1.0
		if reg := w.regionAt(p.x, p.y); reg >= 0 {
			if f, ok := sd.RegionPower[reg]; ok {
				k = f
			}
		}
		hubs, hubSum := 0, 0.0
		for _, b := range w.buildingsByID() {
			if b.Side != s || b.Built < 1 || b.Type != "rail_hub" || dist(b.X, b.Y, p.x, p.y) > importRailKm {
				continue
			}
			hubs++
			hubSum += b.frac()
		}
		if hubs > 0 {
			k = math.Min(k, hubSum/float64(hubs))
		}
		sum += k
	}
	return sum / float64(len(pts))
}
