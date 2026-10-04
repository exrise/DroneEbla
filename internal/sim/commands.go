package sim

import (
	"fmt"
	"math"

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
	CmdMobilize   = "mobilize"
	CmdAlloc      = "alloc"
	CmdPosture    = "posture"
	CmdMainEffort = "main_effort"
	CmdSurrender  = "surrender"
)

// Apply выполняет приказ. Возвращает текст ошибки ("" — успех).
func (w *World) Apply(c Command) string {
	if c.Side < 0 || c.Side > 1 {
		return "неверная сторона"
	}
	if w.Winner >= 0 && c.Kind != CmdSpeed && c.Kind != CmdPause {
		return "Партия окончена"
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
		return w.buyImport(s, c.Item)
	case CmdMobilize:
		return w.mobilize(s, c.Item)
	case CmdAlloc:
		if len(c.Vals) != 3 {
			return "нужно три значения"
		}
		sum := 0.0
		for d := 0; d < 3; d++ {
			sd.Alloc[d] = math.Max(0, c.Vals[d])
			sum += sd.Alloc[d]
		}
		if sum <= 0 {
			sd.Alloc = [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
		} else {
			for d := 0; d < 3; d++ {
				sd.Alloc[d] /= sum
			}
		}
	case CmdPosture:
		sd.Posture = int(clamp(float64(c.Int), 0, 2))
	case CmdMainEffort:
		if c.Int == 0 {
			sd.HasMain = false
		} else {
			sd.HasMain, sd.MainX, sd.MainY = true, c.X, c.Y
		}
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
	if !w.Sides[s].Res.Covers(cost, 1) {
		return "Не хватает ресурсов: " + fmtRes(cost)
	}
	return ""
}

func (w *World) build(s int, typ string, x, y float64) string {
	if e := w.CanBuild(s, typ, x, y); e != "" {
		return e
	}
	bt := w.cat.BuildingByID[typ]
	w.Sides[s].Res.Add(data.ToRes(bt.Cost), -1)
	b := w.addBuilding(typ, s, x, y, 0)
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
	return im.Money * (1 - w.Sides[s].eff("import_discount"))
}

func (w *World) buyImport(s int, id string) string {
	sd := w.Sides[s]
	for _, im := range w.cat.Sides[s].Imports {
		if im.ID != id {
			continue
		}
		if !w.ImportAvailable(s, im) {
			return "Закупка недоступна"
		}
		price := w.ImportPrice(s, im)
		if sd.Res[data.ResMoney] < price {
			return fmt.Sprintf("Нужно %.0f денег", price)
		}
		sd.Res[data.ResMoney] -= price
		sd.ImportCount[im.ID]++
		sd.Deliveries = append(sd.Deliveries, Delivery{Name: im.Name, Item: im.Item, Amount: im.Amount, At: w.Time + im.DelayH*60})
		w.Log(s, 0, fmt.Sprintf("Заказано: %s, прибудет через %s", im.Name, fmtHours(im.DelayH)))
		return ""
	}
	return "Нет такой закупки"
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
	if sd.People < mb.Men {
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
		sd.People -= mb.Men
		sd.MobUsed[mb.ID]++
		sd.MobReady[mb.ID] = w.Time + mb.CooldownH*60
		sd.Morale = clamp(sd.Morale+mb.Morale, 0, 100)
		sd.LaborLoss = math.Min(0.5, sd.LaborLoss+mb.Labor)
		w.distributeFront(s, "men", mb.Men)
		w.Log(s, 1, fmt.Sprintf("%s: +%.0f тыс. человек на фронт", mb.Name, mb.Men))
		return ""
	}
	return "Нет такого варианта"
}
