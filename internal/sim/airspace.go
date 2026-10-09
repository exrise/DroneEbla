package sim

import (
	"fmt"
	"strings"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
)

// AirspaceView — пакет открытия воздушного пространства для интерфейса.
type AirspaceView struct {
	ID        string
	Name      string
	Countries string // «Литва, Латвия»
	On        bool
	AtHour    float64
	Cond      string // условия открытия
}

// openCountries — страны, небо которых открыто для ударов стороны s.
func (w *World) openCountries(s int) map[uint8]bool {
	sd := w.Sides[s]
	var out map[uint8]bool
	for _, p := range w.cat.Sides[s].Airspace {
		if !sd.AirOpen[p.ID] {
			continue
		}
		if out == nil {
			out = map[uint8]bool{}
		}
		for _, k := range p.Countries {
			out[world.CountryCodes[k]] = true
		}
	}
	return out
}

func airspaceCountries(p data.AirspacePackage) string {
	var names []string
	for _, k := range p.Countries {
		names = append(names, world.CountryName(world.CountryCodes[k]))
	}
	return strings.Join(names, ", ")
}

// airspaceTick открывает небо по таймеру и условиям. Открытое небо остаётся
// открытым, даже если условие потом пропало (маршруты не рвутся на лету).
func (w *World) airspaceTick(s int) {
	sd := w.Sides[s]
	h := w.HoursSinceWar()
	for _, p := range w.cat.Sides[s].Airspace {
		if sd.AirOpen[p.ID] || h < p.AtHour || sd.Morale < p.MinMorale {
			continue
		}
		if p.NeedKyiv && w.kyiv >= 0 && w.OwnerSide(w.kyiv) != s {
			continue
		}
		sd.AirOpen[p.ID] = true
		c := airspaceCountries(p)
		w.Log(s, 2, "Воздушное пространство открыто для ударов: "+c)
		w.Log(1-s, 1, "Противнику открыли небо: "+c)
	}
}

// airspaceViews — открытые страны и пакеты для интерфейса (nil, если у стороны их нет).
func (w *World) airspaceViews(s int) (map[uint8]bool, []AirspaceView) {
	def := w.cat.Sides[s].Airspace
	if len(def) == 0 {
		return nil, nil
	}
	sd := w.Sides[s]
	var out []AirspaceView
	for _, p := range def {
		av := AirspaceView{ID: p.ID, Name: p.Name, Countries: airspaceCountries(p), On: sd.AirOpen[p.ID], AtHour: p.AtHour}
		var cond []string
		if p.NeedKyiv {
			cond = append(cond, "если Киев держится")
		}
		if p.MinMorale > 0 {
			cond = append(cond, fmt.Sprintf("если ваша мораль не ниже %.0f", p.MinMorale))
		}
		av.Cond = strings.Join(cond, ", ")
		out = append(out, av)
	}
	return w.openCountries(s), out
}
