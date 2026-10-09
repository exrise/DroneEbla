package sim

import (
	"fmt"
	"strings"

	"github.com/exrise/droneebla/internal/data"
)

// sanctionCap — предел суммарного штрафа одного вида.
const sanctionCap = 0.75

// SanctionView — пакет санкций для интерфейса.
type SanctionView struct {
	ID       string
	Name     string
	Hint     string
	Effects  string // «экспорт −10%, импорт +15%»
	On       bool
	At       float64 // когда введён (игровые минуты)
	Trigger  bool    // вводится в ответ на действия
	Progress float64
	Target   float64
	AtHour   float64 // для пакетов по таймеру
	Cond     string  // условия по таймеру
}

// Sanction — суммарный штраф вида k (доля), действующий против стороны s.
func (w *World) Sanction(s int, k string) float64 {
	sd := w.Sides[s]
	t := 0.0
	for _, p := range w.cat.Sides[s].Sanctions {
		if sd.SanctionOn[p.ID] {
			t += p.Effects[k]
		}
	}
	return clamp(t, 0, sanctionCap)
}

// imposeSanction вводит пакет санкций против стороны s.
func (w *World) imposeSanction(s int, p data.SanctionPackage) {
	sd := w.Sides[s]
	if sd.SanctionOn[p.ID] {
		return
	}
	sd.SanctionOn[p.ID] = true
	sd.SanctionAt[p.ID] = w.Time
	if p.Morale != 0 {
		w.addMorale(s, p.Morale)
	}
	eff := sanctionEffectsText(p.Effects)
	w.Log(s, 2, "Санкции: "+p.Name+" ("+eff+")")
	w.Log(1-s, 1, "Запад ввёл санкции против противника: "+p.Name)
}

// sanctionsTick вводит пакеты по таймеру и проверяет захват городов.
func (w *World) sanctionsTick(s int) {
	if !w.War() {
		return
	}
	sd := w.Sides[s]
	h := w.HoursSinceWar()
	for _, p := range w.cat.Sides[s].Sanctions {
		if sd.SanctionOn[p.ID] {
			continue
		}
		if t := p.Trigger; t != nil {
			if t.Kind == "capture" && w.cityOwner(t.Target) == s {
				w.imposeSanction(s, p)
			}
			continue
		}
		if h < p.AtHour || !w.sanctionCondOK(s, p) {
			continue
		}
		w.imposeSanction(s, p)
	}
}

func (w *World) sanctionCondOK(s int, p data.SanctionPackage) bool {
	if p.NeedKyiv && w.kyiv >= 0 && w.OwnerSide(w.kyiv) == s {
		return false
	}
	return w.Sides[1-s].Morale >= p.EnemyMorale
}

// sanctionBuildingHit — удар стороны attacker по зданию противника может
// вызвать пакет санкций против неё.
func (w *World) sanctionBuildingHit(attacker int, b *Building, hpBefore float64) {
	sd := w.Sides[attacker]
	for _, p := range w.cat.Sides[attacker].Sanctions {
		m := p.Trigger
		if m == nil || m.Kind == "capture" || sd.SanctionOn[p.ID] {
			continue
		}
		if w.countHit(sd, p.ID, m, b, hpBefore) {
			w.imposeSanction(attacker, p)
		}
	}
}

// countHit засчитывает здание в прогресс условия key; true — набрано нужное число.
func (w *World) countHit(sd *Side, key string, m *data.MissionDef, b *Building, hpBefore float64) bool {
	frac := missionFrac(m)
	if !(hpBefore > b.MaxHP*frac && b.HP <= b.MaxHP*frac) || !w.missionMatches(m, b) {
		return false
	}
	for _, id := range sd.MissionSeen[key] {
		if id == b.ID {
			return false
		}
	}
	sd.MissionSeen[key] = append(sd.MissionSeen[key], b.ID)
	return len(sd.MissionSeen[key]) >= missionCount(m)
}

func sanctionEffectsText(e map[string]float64) string {
	var parts []string
	for _, k := range SanctionOrder {
		if v, ok := e[k]; ok {
			parts = append(parts, sanctionLine(k, v))
		}
	}
	return strings.Join(parts, ", ")
}

// sanctionLine — «экспорт −10%».
func sanctionLine(k string, v float64) string {
	sign := "−"
	if k == "import_cost" {
		sign = "+"
	}
	return fmt.Sprintf("%s %s%.0f%%", sanctionShort[k], sign, v*100)
}

// SanctionOrder — порядок видов штрафа в текстах.
var SanctionOrder = []string{"tax", "export", "electronics", "import_cost"}

var sanctionShort = map[string]string{
	"tax":         "налоги",
	"export":      "экспорт",
	"electronics": "электроника",
	"import_cost": "импорт",
}

// SanctionTotalsText — сводка действующих санкций («» — санкций нет).
func SanctionTotalsText(t map[string]float64) string {
	e := map[string]float64{}
	for k, v := range t {
		if v > 0 {
			e[k] = v
		}
	}
	return sanctionEffectsText(e)
}

// sanctionViews — санкции против стороны для интерфейса.
func (w *World) sanctionViews(s int) ([]SanctionView, map[string]float64) {
	sd := w.Sides[s]
	var out []SanctionView
	for _, p := range w.cat.Sides[s].Sanctions {
		sv := SanctionView{ID: p.ID, Name: p.Name, Hint: p.Hint, Effects: sanctionEffectsText(p.Effects),
			On: sd.SanctionOn[p.ID], At: sd.SanctionAt[p.ID], AtHour: p.AtHour}
		if m := p.Trigger; m != nil {
			sv.Trigger = true
			sv.Target = float64(missionCount(m))
			sv.Progress = float64(len(sd.MissionSeen[p.ID]))
			if m.Kind == "capture" {
				sv.Target = 1
				sv.Progress = 0
			}
			if sv.On {
				sv.Progress = sv.Target
			}
		} else {
			var cond []string
			if p.NeedKyiv {
				cond = append(cond, "если Киев держится")
			}
			if p.EnemyMorale > 0 {
				cond = append(cond, fmt.Sprintf("если мораль противника не ниже %.0f", p.EnemyMorale))
			}
			sv.Cond = strings.Join(cond, ", ")
		}
		out = append(out, sv)
	}
	tot := map[string]float64{}
	for k := range data.SanctionKeys {
		if v := w.Sanction(s, k); v > 0 {
			tot[k] = v
		}
	}
	return out, tot
}
