package sim

import (
	"math"

	"github.com/exrise/droneebla/internal/data"
)

// Мораль влияет на саму игру плавными множителями; прямого проигрыша
// от нулевой морали нет — армия и тыл просто разваливаются.

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// moraleCurve: ниже bad — от low (при 0) к 1; между bad и good — 1;
// выше good — от 1 к high (при 100).
func moraleCurve(r data.Rules, m, low, high float64) float64 {
	switch {
	case m < r.MoraleBad:
		return lerp(low, 1, m/r.MoraleBad)
	case m > r.MoraleGood:
		return lerp(1, high, (m-r.MoraleGood)/(100-r.MoraleGood))
	}
	return 1
}

// MoraleFront — множитель силы фронта.
func MoraleFront(r data.Rules, m float64) float64 {
	return moraleCurve(r, m, r.MoraleFrontLow, r.MoraleFrontHigh)
}

// MoraleLosses — множитель потерь фронта (растут при низкой морали).
func MoraleLosses(r data.Rules, m float64) float64 {
	if m >= r.MoraleBad {
		return 1
	}
	return lerp(r.MoraleLossMax, 1, m/r.MoraleBad)
}

// MoraleProd — множитель выпуска заводов и налогов.
func MoraleProd(r data.Rules, m float64) float64 {
	return moraleCurve(r, m, r.MoraleProdLow, r.MoraleProdHigh)
}

// MoraleLevy — множитель притока людей на фронт (добровольцы и уклонисты).
func MoraleLevy(r data.Rules, m float64) float64 {
	return moraleCurve(r, m, r.MoraleLevyLow, r.MoraleLevyHigh)
}

// PropagandaCost — цена кампании в деньгах (дороже при высокой морали).
func PropagandaCost(r data.Rules, m float64) float64 {
	return r.PropagandaCost * (1 + m/100)
}

// PropagandaGain — прирост морали (меньше, чем выше мораль).
func PropagandaGain(r data.Rules, m float64) float64 {
	return r.PropagandaGain * (1 - m/100)
}

// moraleGain — прирост морали стороны s от успеха вида kind с убывающей
// отдачей: чем выше мораль, тем меньше прирост, и каждый недавний успех того
// же вида ослабляет следующий (нет «снежного кома»).
func (w *World) moraleGain(s int, kind string, base float64) float64 {
	r := w.cat.Rules
	sd := w.Sides[s]
	lvl := (100 - sd.Morale) / (100 - r.MoraleGainRef)
	lvl = math.Max(r.MoraleGainFloor, math.Min(1.5, lvl))
	keep := sd.MoraleHist[kind][:0]
	for _, t := range sd.MoraleHist[kind] {
		if w.Time-t < r.MoraleFatigueH*60 {
			keep = append(keep, t)
		}
	}
	n := float64(len(keep))
	sd.MoraleHist[kind] = append(keep, w.Time)
	return base * lvl / (1 + r.MoraleFatigue*n)
}

// addMorale прибавляет к морали стороны s (с ограничением 0..100).
func (w *World) addMorale(s int, d float64) {
	w.Sides[s].Morale = clamp(w.Sides[s].Morale+d, 0, 100)
}
