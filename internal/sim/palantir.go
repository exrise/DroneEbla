package sim

import (
	"fmt"
	"math"
	"sort"
	"strings"

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
	Force bool // игрок подтвердил неполный залп (повтор удара наследует подтверждение)
}

// PalLeg — один пуск плана: источник, число боеприпасов, маршрут и задержка.
type PalLeg struct {
	Source uint32
	SX, SY float64
	Count  int
	Wps    []Pt
	Delay  float64 // минут
	ETA    float64 // минут полёта
	// Exposed — каналов известной ПВО на трассе плеча (не считая прикрывающей цель); Zones — сколько зон пересекает трасса.
	Exposed int
	Zones   int
}

// palGap — интервал между снарядами одного плеча, мин: волна должна приходить плотно, иначе ПВО бьёт её по очереди.
const palGap = 0.1

// PalPreview — рассчитанный план для интерфейса.
type PalPreview struct {
	Key     string // ключ запроса, на который отвечает план
	Err     string
	Legs    []PalLeg
	Total   int     // всего боеприпасов в плане
	Arrive  float64 // минут до прилёта всей волны (с учётом задержек)
	Cover   int     // каналов известной ПВО у цели
	Request int     // сколько просили
	Route   int     // каналов известной ПВО на трассах плеч (в среднем по боеприпасам)
	Exposed int     // плеч, идущих через зону известной ПВО
	Warn    string  // предупреждение: неполный залп, прохождение через ПВО, рассинхрон
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
	Route  int     // каналов известной ПВО на трассе
	Age    float64 // минут с последней разведки цели (−1 — довоенные данные)
	Warn   string
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
	if sd.PalPaused {
		return false, "подписка приостановлена: не хватает денег на плату"
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

// palSourcesAll — готовые к пуску источники по боеприпасам: юниты без перезарядки и площадки с накопленными пусками.
func (w *World) palSourcesAll(s int) map[string][]palSource {
	out := map[string][]palSource{}
	for _, u := range sortedUnits(w.Units) {
		if u.Side != s || u.State != UnitDeployed || u.Reload > 0 {
			continue
		}
		ut := w.cat.UnitByID[u.Type]
		if ut.Kind != "launcher" {
			continue
		}
		for _, id := range ut.Munitions {
			out[id] = append(out[id], palSource{u.ID, u.X, u.Y, ut.Salvo})
		}
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
		for _, item := range w.LaunchOptions(s, id) {
			out[item] = append(out[item], palSource{id, b.X, b.Y, int(math.Floor(b.Budget))})
		}
	}
	return out
}

// palSources — готовые к пуску источники одного боеприпаса.
func (w *World) palSources(s int, item string) []palSource { return w.palSourcesAll(s)[item] }

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
	return w.palPlanWith(s, req, w.palSourcesAll(s), w.KnownADZones(s))
}

// palPlanWith — план по готовым спискам источников и зон (их считает вызывающий один раз на много планов).
func (w *World) palPlanWith(s int, req PalRequest, all map[string][]palSource, zones []Zone) PalPreview {
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
	srcs := append([]palSource(nil), all[req.Item]...)
	sort.SliceStable(srcs, func(a, b int) bool {
		return dist(srcs[a].x, srcs[a].y, target.X, target.Y) < dist(srcs[b].x, srcs[b].y, target.X, target.Y)
	})
	lowRoute := m.Kind == "drone" || (m.Kind == "cruise" && m.Class != "high")
	firstErr := ""
	maxEnd := 0.0
	routeCh, routeN := 0, 0
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
		leg := PalLeg{Source: src.id, SX: src.x, SY: src.y, Count: n, Wps: wps, ETA: eta}
		if lowRoute {
			// Трасса, не обошедшая известную ПВО (обход не уложился в дальность), честно помечается.
			leg.Exposed, leg.Zones = RouteChannels(zones, Pt{src.x, src.y}, flightWps(m, wps), target)
		}
		maxEnd = math.Max(maxEnd, eta+float64(n-1)*palGap)
		pv.Legs = append(pv.Legs, leg)
		pv.Total += n
		routeCh += leg.Exposed * n
		if leg.Zones > 0 {
			pv.Exposed++
		}
		routeN += n
		left -= n
	}
	if len(pv.Legs) == 0 {
		pv.Err = firstErr
		if pv.Err == "" {
			pv.Err = "Нет готовых пусковых для этого боеприпаса"
		}
		return pv
	}
	// Волна приходит плотно: конец окна прилёта каждого плеча совпадает с самым поздним.
	desync := false
	for i := range pv.Legs {
		l := &pv.Legs[i]
		d := maxEnd - (l.ETA + float64(l.Count-1)*palGap)
		if d > 360 {
			d, desync = 360, true
		}
		l.Delay = math.Floor(d)
		pv.Arrive = math.Max(pv.Arrive, l.Delay+l.ETA+float64(l.Count-1)*palGap)
	}
	if routeN > 0 {
		pv.Route = int(math.Round(float64(routeCh) / float64(routeN)))
	}
	for _, z := range zones {
		if dist(z.X, z.Y, req.X, req.Y) <= z.R/0.9+5 {
			pv.Cover += z.Ch
		}
	}
	var warn []string
	if pv.Total < pv.Request {
		warn = append(warn, fmt.Sprintf("в плане %d из %d", pv.Total, pv.Request))
	}
	if pv.Exposed > 0 {
		warn = append(warn, fmt.Sprintf("%d пусков. идут через известную ПВО (обход не уложился в дальность)", pv.Exposed))
	}
	if desync {
		warn = append(warn, "разница прилёта больше 6 ч — волна не будет синхронной")
	}
	pv.Warn = strings.Join(warn, "; ")
	return pv
}

// flightWps — путевые точки, по которым снаряд реально летит (баллистика и высотные ракеты — напрямую).
func flightWps(m *data.MunitionType, wps []Pt) []Pt {
	if m.Kind == "ballistic" || m.Kind == "rocket" || (m.Kind == "cruise" && m.Class == "high") {
		return nil
	}
	return wps
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
		sd.PalPaused, sd.PalWarned = false, false
		if sd.PalSub {
			w.Log(s, 1, fmt.Sprintf("«Палантир»: подписка оформлена (%s в час). Нужен ЦОД у воды с достаточной энергией", data.MoneyText(s, def.MoneyH)))
		} else {
			w.Log(s, 1, "«Палантир»: подписка отменена")
		}
		return ""
	}
	if ok, why := w.palantirActive(s); !ok {
		return "«Палантир» не работает: " + why
	}
	if c.Kind == CmdPalantirSuggest {
		// Повторный запрос в тот же момент без пусков в промежутке даёт тот же результат — считать не нужно.
		if sd.palCacheAt != w.Time || sd.palCacheEp != w.strikeEpoch || sd.PalSuggestAt != w.Time {
			sd.PalSuggest, sd.PalSuggestAt = w.palSuggest(s), w.Time
			sd.palCacheAt, sd.palCacheEp = w.Time, w.strikeEpoch
		}
		return ""
	}
	req := PalRequest{Item: c.Item, Count: c.Count, X: c.X, Y: c.Y, Valid: true, Force: c.Force}
	if c.Kind == CmdPalantirRepeat {
		if !sd.PalLast.Valid {
			return "Нет предыдущего удара «Палантира»"
		}
		req = sd.PalLast
	}
	if c.Kind == CmdPalantirPlan {
		key := PalKey(req.Item, req.Count, req.X, req.Y)
		if sd.PalPreview.Key != key || sd.palPlanAt != w.Time || sd.palPlanEp != w.strikeEpoch {
			sd.PalPreview = w.palPlan(s, req)
			sd.palPlanAt, sd.palPlanEp = w.Time, w.strikeEpoch
		}
		return ""
	}
	pv := w.palPlan(s, req)
	if pv.Err != "" {
		return pv.Err
	}
	// Сильно неполный залп (мало пусковых или запаса) без подтверждения не уходит: игрок должен увидеть, что получилось.
	want := min(max(req.Count, 1), int(w.Sides[s].Stocks[req.Item]))
	if !req.Force && want > 1 && float64(pv.Total) < 0.5*float64(want) {
		return fmt.Sprintf("«Палантир»: в плане только %d из %d — подтвердите неполный залп", pv.Total, want)
	}
	launched, sent := 0, 0
	var firstErr string
	for _, l := range pv.Legs {
		if e := w.Strike(s, StrikePlan{Source: l.Source, Munition: req.Item, Count: l.Count, Waypoints: l.Wps, Target: Pt{req.X, req.Y}, Delay: l.Delay, Gap: palGap}); e != "" {
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

// palantirCost списывает плату за подписку. Если денег на очередной шаг не хватает, подписка приостанавливается
// и возобновляется сама, как только деньги появятся; за 3 часа до исчерпания игрок получает предупреждение.
func (w *World) palantirCost(s int, dtH float64) {
	def := w.cat.Sides[s].Palantir
	sd := w.Sides[s]
	if def == nil || !sd.PalSub {
		return
	}
	c := def.MoneyH * dtH
	if sd.Res[data.ResMoney] >= c {
		sd.Res[data.ResMoney] -= c
		w.spend(s, SpendOther, c)
		if sd.PalPaused {
			sd.PalPaused = false
			w.Log(s, 1, "«Палантир»: подписка возобновлена")
		}
		if sd.Res[data.ResMoney] < 3*def.MoneyH {
			if !sd.PalWarned {
				sd.PalWarned = true
				w.Log(s, 2, "«Палантир»: денег хватит меньше чем на 3 часа подписки")
			}
		} else {
			sd.PalWarned = false
		}
		return
	}
	if !sd.PalPaused {
		sd.PalPaused = true
		w.Log(s, 2, "«Палантир»: подписка приостановлена — не хватает денег")
	}
}

// munCost — условная цена одного боеприпаса в млн $ с учётом сырья (для сравнения «ущерб за деньги»).
func munCost(m *data.MunitionType) float64 {
	c := m.Cost["money"] + 0.15*m.Cost["electronics"] + 0.07*m.Cost["steel"] + 0.1*m.Cost["ammo"]
	return math.Max(c, 0.02)
}

// palAttrition — доля боеприпасов, которую сбивает один канал известной ПВО на пути.
func palAttrition(m *data.MunitionType) float64 {
	a := 0.02 // баллистика и высотные крылатые
	switch {
	case m.Kind == "drone":
		a = 0.05
	case m.Kind == "cruise" && m.Class != "high":
		a = 0.04
	}
	return a * (1 - 0.5*m.Stealth)
}

// palSuggest предлагает до четырёх ударов: по самым ценным известным целям с наименьшим прикрытием, боеприпасом с лучшим
// отношением ожидаемого ущерба к цене из тех, что реально долетают с готовых пусковых, и числом, достаточным для прикрытия
// цели с учётом потерь от ПВО у цели и на трассе. Цели, по которым уже летит залп, и цели рядом с уже предложенными пропускаются.
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
		busy := false
		for _, g := range w.Groups {
			if g.Side == s && dist(g.X, g.Y, c.X, c.Y) < palSpacing {
				busy = true
				break
			}
		}
		if busy {
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
	all := w.palSourcesAll(s)
	zones := w.KnownADZones(s)
	var avail []*data.MunitionType
	for i := range w.cat.Munitions {
		m := &w.cat.Munitions[i]
		if data.SideIndex(m.Side) != s || m.Kind == "interceptor" || m.Kind == "decoy" || m.Kind == "recon" || sd.Stocks[m.ID] < 1 {
			continue
		}
		if len(all[m.ID]) > 0 {
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
		near := false
		for _, o := range out {
			if dist(o.X, o.Y, cd.c.X, cd.c.Y) < palSpacing {
				near = true
				break
			}
		}
		if near {
			continue
		}
		need := math.Ceil(base + per*float64(cd.cover))
		bestVal := -1.0
		var best PalProposal
		for _, m := range avail {
			stock := sd.Stocks[m.ID]
			req := PalRequest{Item: m.ID, Count: int(math.Min(need, stock)), X: cd.c.X, Y: cd.c.Y, Valid: true}
			pv := w.palPlanWith(s, req, all, zones)
			if pv.Err != "" {
				continue
			}
			// Потери от ПВО у цели и на трассе определяют, сколько боеприпасов нужно послать, чтобы дошло need.
			loss := math.Min(0.8, palAttrition(m)*float64(cd.cover+pv.Route))
			n := math.Min(math.Ceil(need/(1-loss)), stock)
			if int(n) != req.Count {
				req.Count = int(n)
				if pv = w.palPlanWith(s, req, all, zones); pv.Err != "" {
					continue
				}
			}
			if float64(pv.Total) < n*0.5 {
				continue
			}
			arrive := float64(pv.Total) * (1 - loss)
			val := (1 - loss) * m.Damage * m.Accuracy / munCost(m) * math.Min(1, arrive/need)
			if val > bestVal {
				bestVal = val
				age := -1.0
				if cd.c.Seen >= 0 {
					age = w.Time - cd.c.Seen
				}
				best = PalProposal{Item: m.ID, Count: int(n), X: cd.c.X, Y: cd.c.Y, Target: cd.name, Total: pv.Total, Legs: len(pv.Legs),
					Arrive: pv.Arrive, Cover: pv.Cover, Route: pv.Route, Age: age, Warn: pv.Warn}
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

// palSpacing — не предлагать цели ближе этого расстояния (км) друг к другу и к целям летящих залпов.
const palSpacing = 12.0
