package sim

import (
	"fmt"
	"sort"
	"strings"

	"github.com/exrise/droneebla/internal/data"
)

// MissionView — задание для интерфейса.
type MissionView struct {
	ID       string
	Title    string
	Hint     string
	Progress float64
	Target   float64
	Done     bool
	Failed   bool
	Reward   string
}

func missionCount(m *data.MissionDef) int {
	if m.Count < 1 {
		return 1
	}
	return m.Count
}

func missionFrac(m *data.MissionDef) float64 {
	if m.HpFrac <= 0 {
		return 0.1
	}
	return m.HpFrac
}

// matches — подходит ли здание под задание.
func (w *World) missionMatches(m *data.MissionDef, b *Building) bool {
	switch m.Kind {
	case "disable_building":
		return b.Name == m.Target
	case "disable_type":
		if b.Type != m.Target {
			return false
		}
	case "hit_region":
		if m.Target != "" && b.Type != m.Target {
			return false
		}
	default:
		return false
	}
	if m.RadiusKm > 0 {
		x, y := w.m.Project(m.Lon, m.Lat)
		return dist(b.X, b.Y, x, y) <= m.RadiusKm
	}
	return true
}

// missionBuildingHit вызывается, когда здание b противника получило урон.
func (w *World) missionBuildingHit(attacker int, b *Building, hpBefore float64) {
	sd := w.Sides[attacker]
	for _, a := range w.cat.Sides[attacker].Aid {
		m := a.Mission
		if m == nil || sd.AidDone[a.ID] || sd.MissionFail[a.ID] || m.Kind == "hold" {
			continue
		}
		frac := missionFrac(m)
		if !(hpBefore > b.MaxHP*frac && b.HP <= b.MaxHP*frac) || !w.missionMatches(m, b) {
			continue
		}
		seen := false
		for _, id := range sd.MissionSeen[a.ID] {
			seen = seen || id == b.ID
		}
		if seen {
			continue
		}
		sd.MissionSeen[a.ID] = append(sd.MissionSeen[a.ID], b.ID)
		if len(sd.MissionSeen[a.ID]) >= missionCount(m) {
			w.grantAid(attacker, a, "Задание выполнено: ")
		}
	}
}

// cityOwner — владелец города по названию (-1, если нет такого).
func (w *World) cityOwner(name string) int {
	for _, c := range w.m.Cities {
		if c.Name == name {
			if i := w.tileOf(c.X, c.Y); i >= 0 {
				return w.OwnerSide(i)
			}
		}
	}
	return -1
}

// missionTick проверяет задания на удержание.
func (w *World) missionTick(s int, a data.AidPackage) {
	sd := w.Sides[s]
	m := a.Mission
	if m.Kind != "hold" || sd.AidDone[a.ID] || sd.MissionFail[a.ID] || w.HoursSinceWar() < m.Hours {
		return
	}
	if w.cityOwner(m.Target) == s {
		w.grantAid(s, a, "Задание выполнено: ")
		return
	}
	sd.MissionFail[a.ID] = true
	w.Log(s, 2, "Задание провалено: "+missionTitle(a))
}

func missionTitle(a data.AidPackage) string {
	if a.Title != "" {
		return a.Title
	}
	return a.Name
}

func (w *World) rewardText(a data.AidPackage) string {
	var parts []string
	ids := make([]string, 0, len(a.Items))
	for id := range a.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%s ×%.0f", w.cat.ItemName(id), a.Items[id]))
	}
	if a.Morale != 0 {
		parts = append(parts, fmt.Sprintf("мораль %+.0f", a.Morale))
	}
	return strings.Join(parts, ", ")
}

// missionViews — задания стороны для интерфейса.
func (w *World) missionViews(s int) []MissionView {
	sd := w.Sides[s]
	var out []MissionView
	for _, a := range w.cat.Sides[s].Aid {
		m := a.Mission
		if m == nil {
			continue
		}
		mv := MissionView{ID: a.ID, Title: missionTitle(a), Hint: a.Hint, Done: sd.AidDone[a.ID], Failed: sd.MissionFail[a.ID], Reward: w.rewardText(a)}
		if m.Kind == "hold" {
			mv.Target = m.Hours
			mv.Progress = w.HoursSinceWar()
			if mv.Progress > mv.Target {
				mv.Progress = mv.Target
			}
		} else {
			mv.Target = float64(missionCount(m))
			mv.Progress = float64(len(sd.MissionSeen[a.ID]))
			if mv.Done {
				mv.Progress = mv.Target
			}
		}
		out = append(out, mv)
	}
	return out
}
