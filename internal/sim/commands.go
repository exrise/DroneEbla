package sim

import (
	"fmt"
	"math"
	"strings"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
)

// Command — приказ игрока. Передаётся по сети.
type Command struct {
	Kind  string
	Side  int
	ID    uint32
	Item  string
	Text  string
	Count int
	Int   int
	X, Y  float64
	Pts   []Pt
	Vals  []float64
	Delay float64
	// Player — номер игрока в сетевой игре (0 — хост); заполняет хост, нужен журналу партий.
	Player int
}

// Виды приказов.
const (
	CmdSpeed      = "speed"
	CmdPause      = "pause"
	CmdBuild      = "build"
	CmdRepair     = "repair"
	CmdMask       = "mask"
	CmdFort       = "fort"
	CmdOrderAdd   = "order_add"
	CmdOrderDel   = "order_del"
	CmdOrderMove  = "order_move"
	CmdMove       = "move"
	CmdStrike     = "strike"
	CmdResearch   = "research"
	CmdResFund    = "research_fund"
	CmdAgentFund  = "agent_fund"
	CmdImport     = "import"
	CmdAutoImport = "auto_import"
	CmdKeepStock  = "keep_stock"
	CmdSell       = "sell"
	CmdMobilize   = "mobilize"
	CmdAlloc      = "alloc"
	CmdPosture    = "posture"
	CmdMainEffort = "main_effort"
	CmdSurrender  = "surrender"
	CmdPropaganda = "propaganda"
	CmdPlace      = "place"     // Item — тип из резерва, X, Y — точка
	CmdUnplace    = "unplace"   // ID — поставленный объект возвращается в резерв
	CmdFireMode   = "fire_mode" // ID — комплекс ПВО, Int — режим огня 0–2
	CmdReady      = "ready"     // Int 1/0 — готовность к старту
)

// Apply выполняет приказ. Возвращает текст ошибки ("" — успех).
func (w *World) Apply(c Command) string {
	w.sens.valid = false
	e := w.apply(c)
	w.sens.valid = false
	w.recCommand(c, e)
	return e
}

func (w *World) apply(c Command) string {
	if c.Side < 0 || c.Side > 1 {
		return "неверная сторона"
	}
	if w.Winner >= 0 && c.Kind != CmdSpeed && c.Kind != CmdPause {
		return "Партия окончена"
	}
	if !finite(c.X, c.Y, c.Delay) || !finite(c.Vals...) {
		return "неверные числа в приказе"
	}
	for _, p := range c.Pts {
		if !finite(p.X, p.Y) {
			return "неверные числа в приказе"
		}
	}
	s := c.Side
	sd := w.Sides[s]
	r := w.cat.Rules
	switch c.Kind {
	case CmdSpeed:
		sd.Speed = int(clamp(float64(c.Int), 1, 5))
	case CmdPause:
		sd.Pausing = c.Int != 0
	case CmdBuild:
		return w.build(s, c.Item, c.X, c.Y)
	case CmdPlace:
		return w.place(s, c.Item, c.X, c.Y)
	case CmdUnplace:
		return w.unplace(s, c.ID)
	case CmdReady:
		return w.ready(s, c.Int != 0)
	case CmdRepair:
		b, ok := w.Buildings[c.ID]
		if !ok || b.Side != s {
			return "Нет такого здания"
		}
		b.Repair = c.Int != 0
	case CmdMask:
		b, ok := w.Buildings[c.ID]
		if !ok || b.Side != s {
			return "Нет такого здания"
		}
		if b.Masked {
			return "Уже замаскировано"
		}
		cost := data.ToRes(r.MaskCost)
		if !sd.Res.Covers(cost, 1) {
			return "Не хватает ресурсов: " + fmtRes(cost)
		}
		sd.Res.Add(cost, -1)
		b.Masked = true
	case CmdFort:
		cost := data.ToRes(r.FortCost)
		n := 0
		for _, p := range c.Pts {
			i := w.tileOf(p.X, p.Y)
			if i < 0 || w.OwnerSide(i) != s || w.m.Terrain[i] != world.TerrainLand {
				continue
			}
			if _, busy := w.FortJobs[i]; busy || w.Fort[i] >= 3 {
				continue
			}
			if !sd.Res.Covers(cost, 1) {
				break
			}
			sd.Res.Add(cost, -1)
			w.FortJobs[i] = 0
			n++
		}
		if n == 0 {
			return "Нельзя укрепить выбранные тайлы (нужна своя суша и ресурсы)"
		}
	case CmdOrderAdd:
		if !sd.Unlocked[c.Item] {
			return "Не изучено"
		}
		if _, _, _, ok := w.cat.ItemCost(c.Item); !ok {
			return "Это нельзя производить"
		}
		n := c.Count
		if n == 0 {
			n = -1
		}
		for i := range sd.Orders {
			if sd.Orders[i].Item == c.Item {
				if sd.Orders[i].Remaining < 0 || n < 0 {
					sd.Orders[i].Remaining = -1
				} else {
					sd.Orders[i].Remaining += n
				}
				return ""
			}
		}
		sd.Orders = append(sd.Orders, Order{Item: c.Item, Remaining: n})
	case CmdOrderDel:
		if c.Int < 0 || c.Int >= len(sd.Orders) {
			return "Нет такой позиции"
		}
		sd.Orders = append(sd.Orders[:c.Int], sd.Orders[c.Int+1:]...)
	case CmdOrderMove:
		j := c.Int + c.Count
		if c.Int < 0 || c.Int >= len(sd.Orders) || j < 0 || j >= len(sd.Orders) {
			return ""
		}
		sd.Orders[c.Int], sd.Orders[j] = sd.Orders[j], sd.Orders[c.Int]
	case CmdMove:
		return w.MoveUnit(s, c.ID, Pt{c.X, c.Y})
	case CmdStrike:
		return w.Strike(s, StrikePlan{Source: c.ID, Munition: c.Item, Count: c.Count, Waypoints: c.Pts, Target: Pt{c.X, c.Y}, Delay: c.Delay})
	case CmdResearch:
		if c.Item == "" {
			sd.Research = ""
			return ""
		}
		if !w.TechAvailable(s, c.Item) {
			return "Исследование недоступно"
		}
		sd.Research = c.Item
	case CmdResFund:
		sd.ResFund = int(clamp(float64(c.Int), 0, 3))
	case CmdAgentFund:
		sd.AgentFund = c.Int != 0
	case CmdImport:
		return w.buyImport(s, c.Item, c.Count)
	case CmdKeepStock:
		m := w.cat.MunitionByID[c.Item]
		if m == nil || m.Kind != "interceptor" || data.SideIndex(m.Side) != s {
			return "Это не ваша зенитная ракета"
		}
		sd.KeepStock[c.Item] = c.Int != 0
	case CmdSell:
		return w.sell(s, c.Item, c.Count)
	case CmdAutoImport:
		return w.setAutoImport(s, c.Item, c.Int != 0)
	case CmdMobilize:
		return w.mobilize(s, c.Item)
	case CmdAlloc:
		if len(c.Vals) != NumDir {
			return "нужно четыре значения"
		}
		sum := 0.0
		for d := 0; d < NumDir; d++ {
			sd.DirAlloc[d] = math.Max(0, c.Vals[d])
			sum += sd.DirAlloc[d]
		}
		if sum <= 0 {
			sd.DirAlloc = uniformAlloc()
		} else {
			for d := 0; d < NumDir; d++ {
				sd.DirAlloc[d] /= sum
			}
		}
	case CmdPosture:
		// Count: 0 — все направления, 1–4 — одно направление.
		p := int(clamp(float64(c.Int), 0, 2))
		for d := 0; d < NumDir; d++ {
			if c.Count == 0 || c.Count == d+1 {
				sd.DirPosture[d] = p
			}
		}
	case CmdFireMode:
		u, ok := w.Units[c.ID]
		if !ok || u.Side != s || w.cat.UnitByID[u.Type].Kind != "ad" {
			return "Это не ваш комплекс ПВО"
		}
		u.Fire = int(clamp(float64(c.Int), 0, 2))
	case CmdMainEffort:
		if c.Int == 0 {
			sd.HasMain = false
		} else {
			sd.HasMain, sd.MainX, sd.MainY = true, c.X, c.Y
		}
	case CmdPropaganda:
		return w.propaganda(s)
	case CmdSurrender:
		w.Winner = 1 - s
		w.WinReason = data.SideNames[s] + " капитулировала"
	default:
		return "неизвестный приказ " + c.Kind
	}
	return ""
}

// CanBuild проверяет возможность строительства.
func (w *World) CanBuild(s int, typ string, x, y float64) string {
	bt := w.cat.BuildingByID[typ]
	if bt == nil {
		return "Неизвестный тип"
	}
	okSide := false
	for _, k := range bt.Buildable {
		if data.SideIndex(k) == s {
			okSide = true
		}
	}
	if !okSide {
		return "Этот объект нельзя построить"
	}
	i := w.tileOf(x, y)
	if i < 0 || w.OwnerSide(i) != s || w.m.Terrain[i] != world.TerrainLand {
		return "Строить можно только на своей суше"
	}
	for _, f := range w.frontT[s] {
		fx, fy := w.m.TileCenter(f%w.m.W, f/w.m.W)
		if dist(fx, fy, x, y) < 15 {
			return "Слишком близко к фронту (менее 15 км)"
		}
	}
	for dep, km := range bt.NeedDeposit {
		kind := depositKind(dep)
		found := false
		for _, t := range w.depots[kind] {
			if w.OwnerSide(t) != s {
				continue
			}
			cx, cy := w.m.TileCenter(t%w.m.W, t/w.m.W)
			if km <= 0 || dist(cx, cy, x, y) <= km {
				found = true
				break
			}
		}
		if !found {
			return "Нет нужного месторождения рядом"
		}
	}
	cost := data.ToRes(bt.Cost)
	if !w.Cheat && !w.Sides[s].Res.Covers(cost, 1) {
		return "Не хватает ресурсов: " + fmtRes(cost)
	}
	return ""
}

func (w *World) build(s int, typ string, x, y float64) string {
	if e := w.CanBuild(s, typ, x, y); e != "" {
		return e
	}
	bt := w.cat.BuildingByID[typ]
	built := 0.0
	if w.Cheat {
		built = 1
	} else {
		w.Sides[s].Res.Add(data.ToRes(bt.Cost), -1)
	}
	b := w.addBuilding(typ, s, x, y, built)
	if bt.Supply > 0 {
		b.Dir = w.PointDir(x, y)
	}
	w.LogAt(s, 0, "Начато строительство: "+b.Name, x, y)
	return ""
}

// ImportAvailable — доступна ли закупка.
func (w *World) ImportAvailable(s int, im data.ImportOffer) bool {
	sd := w.Sides[s]
	if im.Requires != "" && !sd.Researched[im.Requires] {
		return false
	}
	if im.Removes != "" && sd.Researched[im.Removes] {
		return false
	}
	if im.Limit > 0 && sd.ImportCount[im.ID] >= im.Limit {
		return false
	}
	return true
}

// ImportPrice — цена с учётом скидки.
func (w *World) ImportPrice(s int, im data.ImportOffer) float64 {
	return im.Money * (1 - w.Sides[s].eff("import_discount")) * (1 + w.Sanction(s, "import_cost"))
}

// buyImport заказывает закупку n раз (n < 1 — один раз); берёт столько, на сколько хватает денег и лимита.
func (w *World) buyImport(s int, id string, n int) string {
	sd := w.Sides[s]
	if n < 1 {
		n = 1
	}
	n = min(n, 50)
	for _, im := range w.cat.Sides[s].Imports {
		if im.ID != id {
			continue
		}
		if !w.ImportAvailable(s, im) {
			return "Закупка недоступна"
		}
		if w.Cheat {
			w.deliver(s, im.Item, im.Amount*float64(n), "")
			return ""
		}
		price := w.ImportPrice(s, im)
		bought := 0
		for bought < n && w.ImportAvailable(s, im) && sd.Res[data.ResMoney] >= price {
			sd.Res[data.ResMoney] -= price
			sd.ImportCount[im.ID]++
			sd.Deliveries = append(sd.Deliveries, Delivery{Name: im.Name, Item: im.Item, Amount: im.Amount, At: w.Time + im.DelayH*60})
			bought++
		}
		if bought == 0 {
			return fmt.Sprintf("Нужно %.0f денег", price)
		}
		if bought == 1 {
			w.Log(s, 0, fmt.Sprintf("Заказано: %s, прибудет через %s", im.Name, fmtHours(im.DelayH)))
		} else {
			w.Log(s, 0, fmt.Sprintf("Заказано %d×: %s, прибудет через %s", bought, im.Name, fmtHours(im.DelayH)))
		}
		return ""
	}
	return "Нет такой закупки"
}

// setAutoImport включает или выключает автозакупку предложения id (только ресурсы с порогом auto_below).
func (w *World) setAutoImport(s int, id string, on bool) string {
	for _, im := range w.cat.Sides[s].Imports {
		if im.ID != id {
			continue
		}
		if im.AutoBelow <= 0 {
			return "Для этой закупки автозакупка недоступна"
		}
		w.Sides[s].AutoImport[id] = on
		return ""
	}
	return "Нет такой закупки"
}

// autoImportTick — автозакупка: ресурс ниже порога, партии этого вида в пути нет, деньги выше резерва.
func (w *World) autoImportTick(s int) {
	sd := w.Sides[s]
	r := w.cat.Rules
	for _, im := range w.cat.Sides[s].Imports {
		if !sd.AutoImport[im.ID] || im.AutoBelow <= 0 || !strings.HasPrefix(im.Item, "res:") {
			continue
		}
		if w.Time-sd.AutoImportAt[im.ID] < r.AutoImportEveryMin || !w.ImportAvailable(s, im) {
			continue
		}
		have := 0.0
		for i, k := range data.ResKeys {
			if im.Item == "res:"+k {
				have = sd.Res[i]
			}
		}
		pending := false
		for _, d := range sd.Deliveries {
			if d.Item == im.Item {
				pending = true
			}
		}
		price := w.ImportPrice(s, im)
		if pending || have >= im.AutoBelow || sd.Res[data.ResMoney] < price+r.AutoImportReserve {
			continue
		}
		sd.AutoImportAt[im.ID] = w.Time
		w.buyImport(s, im.ID, 1)
	}
}

// MobilizationReady — можно ли объявить.
func (w *World) MobilizationReady(s int, mb data.Mobilization) string {
	sd := w.Sides[s]
	if mb.Limit > 0 && sd.MobUsed[mb.ID] >= mb.Limit {
		return "Уже использовано"
	}
	if t := sd.MobReady[mb.ID]; t > w.Time {
		return "Будет доступно через " + fmtHours((t-w.Time)/60)
	}
	if !mb.Foreign && sd.People < mb.Men {
		return "Исчерпан мобилизационный резерв"
	}
	if sd.Res[data.ResMoney] < mb.Money {
		return fmt.Sprintf("Нужно %.0f денег", mb.Money)
	}
	return ""
}

func (w *World) mobilize(s int, id string) string {
	sd := w.Sides[s]
	for _, mb := range w.cat.Sides[s].Mobilization {
		if mb.ID != id {
			continue
		}
		if e := w.MobilizationReady(s, mb); e != "" {
			return e
		}
		sd.Res[data.ResMoney] -= mb.Money
		if !mb.Foreign {
			sd.People -= mb.Men
		}
		sd.MobUsed[mb.ID]++
		sd.MobReady[mb.ID] = w.Time + mb.CooldownH*60
		r := w.cat.Rules
		// Чем ниже мораль, тем сильнее удар по ней и больше уклонистов.
		pen := mb.Morale
		if pen < 0 {
			pen *= lerp(r.MoraleMobPenalty, 1, sd.Morale/100)
		}
		men := mb.Men
		if !mb.Foreign {
			men *= MoraleLevy(r, sd.Morale)
		}
		sd.Morale = clamp(sd.Morale+pen, 0, 100)
		if !mb.Foreign {
			sd.LaborLoss = math.Min(0.5, sd.LaborLoss+mb.Labor)
		}
		w.distributeFront(s, "men", men)
		if mb.Foreign {
			w.Log(s, 1, fmt.Sprintf("%s: +%.0f тыс. бойцов на фронт", mb.Name, mb.Men))
		} else {
			w.Log(s, 1, fmt.Sprintf("%s: +%.0f тыс. человек на фронт (призвано %.0f тыс.)", mb.Name, men, mb.Men))
		}
		return ""
	}
	return "Нет такого варианта"
}

// propaganda — информационная кампания: деньги в обмен на мораль.
func (w *World) propaganda(s int) string {
	sd := w.Sides[s]
	r := w.cat.Rules
	if sd.PropReady > w.Time {
		return "Кампания будет доступна через " + fmtHours((sd.PropReady-w.Time)/60)
	}
	cost := PropagandaCost(r, sd.Morale)
	if sd.Res[data.ResMoney] < cost {
		return fmt.Sprintf("Нужно %.0f денег", cost)
	}
	gain := PropagandaGain(r, sd.Morale)
	sd.Res[data.ResMoney] -= cost
	sd.Morale = clamp(sd.Morale+gain, 0, 100)
	sd.PropReady = w.Time + r.PropagandaCooldownH*60
	w.Log(s, 0, fmt.Sprintf("Информационная кампания: мораль +%.1f", gain))
	return ""
}

// finite — все числа конечны (приказы приходят по сети: NaN и Inf ломают расчёты).
func finite(v ...float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return false
		}
	}
	return true
}

// uniformAlloc — поровну между направлениями.
func uniformAlloc() [NumDir]float64 {
	var a [NumDir]float64
	for d := range a {
		a[d] = 1.0 / NumDir
	}
	return a
}
