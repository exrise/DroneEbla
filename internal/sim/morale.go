package sim

import "github.com/exrise/droneebla/internal/data"

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
