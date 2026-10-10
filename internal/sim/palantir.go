package sim

import (
	"fmt"
	"math"
	"sort"

	"github.com/exrise/droneebla/internal/data"
)

// «ИИ Палантир» (только Украина): платная подписка плюс ЦОД у воды с достаточным питанием. Планировщик по цели,
// боеприпасу и числу сам подбирает пусковые в досягаемости, строит маршруты в обход известной ПВО (и коридоры над
// открытым небом соседних стран) и выравнивает прилёт задержками.

// PalRequest — запрос игрока к «Палантиру».
type PalRequest struct {
	Item  string  // боеприпас
	Count int     // сколько всего
	X, Y  float64 // цель
	Valid bool
}

// PalLeg — один пуск плана: источник, число боеприпасов, маршрут и задержка.
type PalLeg struct {
	Source uint32
	SX, SY float64
	Count  int
	Wps    []Pt
	Delay  float64 // минут
	ETA    float64 // минут полёта
}

// PalPreview — рассчитанный план для интерфейса.
type PalPreview struct {
	Key     string // ключ запроса, на который отвечает план
	Err     string
	Legs    []PalLeg
	Total   int     // всего боеприпасов в плане
	Arrive  float64 // минут до прилёта всей волны (с учётом задержек)
	Cover   int     // каналов известной ПВО у цели
	Request int     // сколько просили
}

// PalProposal — предложение «Палантира»: цель, тип боеприпаса и количество; игрок только подтверждает.
type PalProposal struct {
	Item   string
	Count  int
	X, Y   float64
	Target string
	Total  int     // боеприпасов реально в плане (может быть меньше Count)
	Legs   int     // пусковых
	Arrive float64 // минут до прилёта волны
	Cover  int     // каналов известной ПВО у цели
	Age    float64 // минут с последней разведки цели (−1 — довоенные данные)
}

// PalantirView — состояние «Палантира» для интерфейса.
type PalantirView struct {
	Has       bool
	Sub       bool
	Active    bool
	Reason    string // почему не работает
	MoneyH    float64
	Building  string
	Preview   PalPreview
	Last      PalRequest
	Suggest   []PalProposal // предложения ударов
	SuggestAt float64       // игровое время расчёта предложений
}

// PalKey — ключ запроса для сопоставления плана с запросом на клиенте.
func PalKey(item string, count int, x, y float64) string {
	return fmt.Sprintf("%s|%d|%.1f|%.1f", item, count, x, y)
}

// palantirActive — работает ли «Палантир» стороны: подписка, целый ЦОД, энергия в его области.
func (w *World) palantirActive(s int) (bool, string) {
	def := w.cat.Sides[s].Palantir
	if def == nil {
		return false, "у этой стороны нет «Палантира»"
	}
	sd := w.Sides[s]
	if !sd.PalSub {
		return false, "подписка не оформлена"
	}
	found := false
	for _, b := range w.Buildings {
		if b.Side != s || b.Type != def.Building || !b.Operational() {
			continue
		}
		found = true
		if f, ok := sd.RegionPower[b.Region]; ok && f < def.MinPower {
			continue
		}
		return true, ""
	}
	if !found {
		return false, "нет работающего ЦОД"
	}
	return false, fmt.Sprintf("в области ЦОД не хватает энергии (нужно не меньше %.0f%%)", def.MinPower*100)
}

// palSources — готовые к пуску источники боеприпаса: юниты без перезарядки и площадки с накопленными пусками.
func (w *World) palSources(s int, item string) []palSource {
	var out []palSource
	for _, u := range sortedUnits(w.Units) {
		if u.Side != s || u.State != UnitDeployed || u.Reload > 0 {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.Kind != "launcher" || !contains(ut.Munitions, item) {
			continue
		}
		out = append(out, palSource{u.ID, u.X, u.Y, ut.Salvo})
	}
	ids := make([]uint32, 0, len(w.Buildings))
	for id := range w.Buildings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	for _, id := range ids {
		b := w.Buildings[id]
		if b.Side != s || !b.Operational() || math.Floor(b.Budget) < 1 {
			continue
		}
		if contains(w.LaunchOptions(s, id), item) {
			out = append(out, palSource{id, b.X, b.Y, int(math.Floor(b.Budget))})
		}
	}
	return out
}

type palSource struct {
	id   uint32
	x, y float64
	cap  int
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// palPlan рассчитывает план по запросу.
func (w *World) palPlan(s int, req PalRequest) PalPreview {
	pv := PalPreview{Key: PalKey(req.Item, req.Count, req.X, req.Y), Request: req.Count}
	m := w.cat.MunitionByID[req.Item]
	if m == nil || data.SideIndex(m.Side) != s || m.Kind == "interceptor" || m.Kind == "decoy" {
		pv.Err = "Неизвестный боеприпас"
		return pv
	}
	if !w.War() {
		pv.Err = "Удары невозможны в подготовительной фазе"
		return pv
	}
	left := req.Count
	if left < 1 {
		left = 1
	}
	stock := int(w.Sides[s].Stocks[req.Item])
	if stock < 1 {
		pv.Err = "Нет на складе"
		return pv
	}
	left = min(left, stock)
	target := Pt{req.X, req.Y}
	srcs := w.palSources(s, req.Item)
	sort.SliceStable(srcs, func(a, b int) bool {
		return dist(srcs[a].x, srcs[a].y, target.X, target.Y) < dist(srcs[b].x, srcs[b].y, target.X, target.Y)
	})
	zones := w.KnownADZones(s)
	lowRoute := m.Kind == "drone" || (m.Kind == "cruise" && m.Class != "high")
	firstErr := ""
	maxETA := 0.0
	for _, src := range srcs {
		if left <= 0 {
			break
		}
		n := min(src.cap, left)
		sp := StrikePlan{Source: src.id, Munition: req.Item, Count: n, Target: target}
		var wps []Pt
		if lowRoute {
			if r := RouteAround(zones, src.x, src.y, target.X, target.Y); len(r) > 0 {
				sp.Waypoints = r
				if w.ValidateStrike(s, sp) == "" {
					wps = r
				} else {
					sp.Waypoints = nil
				}
			}
		}
		err := w.ValidateStrike(s, sp)
		if err != "" && lowRoute {
			if r := w.CorridorRoute(s, sp, src.x, src.y); r != nil {
				sp.Waypoints, wps, err = r, r, ""
			}
		}
		if err != "" {
			if firstErr == "" {
				firstErr = err
			}
			continue
		}
		eta := 0.0
		if m.SpeedKmh > 0 {
			eta = PathLength(Pt{src.x, src.y}, append(append([]Pt{}, wps...), target)) / m.SpeedKmh * 60
		}
		maxETA = math.Max(maxETA, eta)
		pv.Legs = append(pv.Legs, PalLeg{Source: src.id, SX: src.x, SY: src.y, Count: n, Wps: wps, ETA: eta})
		pv.Total += n
		left -= n
	}
	if len(pv.Legs) == 0 {
		pv.Err = firstErr
		if pv.Err == "" {
			pv.Err = "Нет готовых пусковых для этого боеприпаса"
		}
		return pv
	}
	for i := range pv.Legs {
		pv.Legs[i].Delay = math.Floor(math.Min(360, maxETA-pv.Legs[i].ETA))
		pv.Arrive = math.Max(pv.Arrive, pv.Legs[i].Delay+pv.Legs[i].ETA)
	}
	for _, c := range w.Sides[s].Known {
		if c.Kind != 1 || c.Type == "" || w.Time-c.Seen > 720 {
			continue
		}
		if ut := w.cat.UnitByID[c.Type]; ut != nil && ut.Kind == "ad" && dist(c.X, c.Y, req.X, req.Y) <= ut.RangeKm+5 {
			pv.Cover += ut.Channels
		}
	}
	return pv
}

// palCommand выполняет команды «Палантира» стороны s.
func (w *World) palCommand(s int, c Command) string {
	def := w.cat.Sides[s].Palantir
	if def == nil {
		return "У вашей стороны нет «Палантира»"
	}
	sd := w.Sides[s]
	switch c.Kind {
	case CmdPalantir:
		sd.PalSub = c.Int != 0
		if sd.PalSub {
			w.Log(s, 1, fmt.Sprintf("«Палантир»: подписка оформлена (%.0f %s в час). Нужен ЦОД у воды с достаточной энергией", def.MoneyH, data.MoneyUnit[s]))
		} else {
			w.Log(s, 1, "«Палантир»: подписка отменена")
		}
		return ""
	}
	if ok, why := w.palantirActive(s); !ok {
		return "«Палантир» не работает: " + why
	}
	if c.Kind == CmdPalantirSuggest {
		sd.PalSuggest, sd.PalSuggestAt = w.palSuggest(s), w.Time
		return ""
	}
	req := PalRequest{Item: c.Item, Count: c.Count, X: c.X, Y: c.Y, Valid: true}
	if c.Kind == CmdPalantirRepeat {
		if !sd.PalLast.Valid {
			return "Нет предыдущего удара «Палантира»"
		}
		req = sd.PalLast
	}
	pv := w.palPlan(s, req)
	if c.Kind == CmdPalantirPlan {
		sd.PalPreview = pv
		return ""
	}
	if pv.Err != "" {
		return pv.Err
	}
	launched, sent := 0, 0
	var firstErr string
	for _, l := range pv.Legs {
		if e := w.Strike(s, StrikePlan{Source: l.Source, Munition: req.Item, Count: l.Count, Waypoints: l.Wps, Target: Pt{req.X, req.Y}, Delay: l.Delay}); e != "" {
			if firstErr == "" {
				firstErr = e
			}
			continue
		}
		launched++
		sent += l.Count
	}
	if launched == 0 {
		return firstErr
	}
	sd.PalLast = req
	name := w.cat.MunitionByID[req.Item].Name
	w.LogAt(s, 1, fmt.Sprintf("«Палантир»: залп %s ×%d с %d пусковых, прилёт через %s", name, sent, launched, fmtHours(pv.Arrive/60)), req.X, req.Y)
	return ""
}

// palantirView — состояние «Палантира» для представления стороны.
func (w *World) palantirView(s int) PalantirView {
	def := w.cat.Sides[s].Palantir
	if def == nil {
		return PalantirView{}
	}
	sd := w.Sides[s]
	ok, why := w.palantirActive(s)
	return PalantirView{Has: true, Sub: sd.PalSub, Active: ok, Reason: why, MoneyH: def.MoneyH, Building: def.Building, Preview: sd.PalPreview, Last: sd.PalLast, Suggest: sd.PalSuggest, SuggestAt: sd.PalSuggestAt}
}

// palantirCost списывает плату за подписку; при нехватке денег подписка отменяется.
func (w *World) palantirCost(s int, dtH float64) {
	def := w.cat.Sides[s].Palantir
	sd := w.Sides[s]
	if def == nil || !sd.PalSub {
		return
	}
	c := def.MoneyH * dtH
	if sd.Res[data.ResMoney] >= c {
		sd.Res[data.ResMoney] -= c
		return
	}
	sd.PalSub = false
	w.Log(s, 2, "«Палантир»: подписка отменена — не хватает денег")
}

// palSuggest предлагает до четырёх ударов: по самым ценным известным целям с наименьшим прикрытием, боеприпасом с лучшим
// отношением поражающего действия к цене из тех, что реально долетают с готовых пусковых, и числом, достаточным для
// прикрытия цели (для дронов и крылатых ракет — с запасом на потери).
func (w *World) palSuggest(s int) []PalProposal {
	sd := w.Sides[s]
	cfg := w.cat.AI.Sides[data.SideKeys[s]]
	type cand struct {
		c     *Contact
		name  string
		cover int
		score float64
	}
	var cands []cand
	for _, c := range sd.Known {
		if c.Type == "" {
			continue
		}
		var weight float64
		name := ""
		if c.Kind == 0 {
			weight = cfg.StrikeWeights[c.Type]
			if bt := w.cat.BuildingByID[c.Type]; bt != nil {
				name = bt.Name
			}
			if c.HP >= 0 && c.HP < 0.25 {
				continue
			}
		} else {
			ut := w.cat.UnitByID[c.Type]
			if ut == nil || c.Seen < 0 || w.Time-c.Seen > 360 {
				continue
			}
			weight = cfg.UnitStrikeWeights[ut.Kind]
			name = ut.Name
		}
		if weight <= 0 {
			continue
		}
		cover := 0
		for _, o := range sd.Known {
			if o.Kind != 1 || o.Type == "" || o.Seen < 0 || w.Time-o.Seen > 720 {
				continue
			}
			if ut := w.cat.UnitByID[o.Type]; ut != nil && ut.Kind == "ad" && dist(o.X, o.Y, c.X, c.Y) <= ut.RangeKm+5 {
				cover += ut.Channels
			}
		}
		cands = append(cands, cand{c, name, cover, weight / (1 + 0.3*float64(cover))})
	}
	sort.Slice(cands, func(a, b int) bool {
		if cands[a].score != cands[b].score {
			return cands[a].score > cands[b].score
		}
		return cands[a].c.ID < cands[b].c.ID
	})
	if len(cands) > 40 {
		cands = cands[:40]
	}
	var avail []*data.MunitionType
	for i := range w.cat.Munitions {
		m := &w.cat.Munitions[i]
		if data.SideIndex(m.Side) != s || m.Kind == "interceptor" || m.Kind == "decoy" || m.Kind == "recon" || sd.Stocks[m.ID] < 1 {
			continue
		}
		if len(w.palSources(s, m.ID)) > 0 {
			avail = append(avail, m)
		}
	}
	base, per := cfg.SalvoBase, cfg.SalvoPerChannel
	if base <= 0 {
		base = 3
	}
	if per <= 0 {
		per = 2.5
	}
	var out []PalProposal
	for _, cd := range cands {
		need := math.Ceil(base + per*float64(cd.cover))
		bestVal := -1.0
		var best PalProposal
		for _, m := range avail {
			n := need
			if m.Kind == "drone" || (m.Kind == "cruise" && m.Class != "high") {
				n = math.Ceil(need * 1.6) // часть собьёт ПВО
			}
			n = math.Min(n, sd.Stocks[m.ID])
			pv := w.palPlan(s, PalRequest{Item: m.ID, Count: int(n), X: cd.c.X, Y: cd.c.Y, Valid: true})
			if pv.Err != "" || float64(pv.Total) < n*0.5 {
				continue
			}
			if val := m.Damage * m.Accuracy / (m.Cost["money"] + 1); val > bestVal {
				bestVal = val
				age := -1.0
				if cd.c.Seen >= 0 {
					age = w.Time - cd.c.Seen
				}
				best = PalProposal{Item: m.ID, Count: int(n), X: cd.c.X, Y: cd.c.Y, Target: cd.name, Total: pv.Total, Legs: len(pv.Legs), Arrive: pv.Arrive, Cover: pv.Cover, Age: age}
			}
		}
		if bestVal >= 0 {
			out = append(out, best)
		}
		if len(out) == 4 {
			break
		}
	}
	return out
}
