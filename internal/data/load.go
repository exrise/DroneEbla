package data

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/exrise/droneebla/internal/world"
)

//go:embed defaults/*.json
var defaults embed.FS

// Files — имена файлов каталога.
var Files = []string{"rules.json", "buildings.json", "units.json", "munitions.json", "front.json", "tech.json", "lines.json", "sides.json", "objects.json", "ai.json"}

// readFile берёт файл из папки override (если есть), иначе встроенный.
func readFile(override, name string) ([]byte, error) {
	if override != "" {
		if b, err := os.ReadFile(filepath.Join(override, name)); err == nil {
			return b, nil
		}
	}
	return defaults.ReadFile("defaults/" + name)
}

// Load загружает каталог. override — папка с пользовательскими JSON
// (может быть пустой строкой).
func Load(override string) (*Catalog, error) {
	c := &Catalog{}
	var sides struct {
		RU SideDef `json:"ru"`
		UA SideDef `json:"ua"`
	}
	targets := map[string]any{
		"rules.json":     &c.Rules,
		"buildings.json": &c.Buildings,
		"units.json":     &c.Units,
		"munitions.json": &c.Munitions,
		"front.json":     &c.Front,
		"tech.json":      &c.Tech,
		"lines.json":     &c.Lines,
		"sides.json":     &sides,
		"objects.json":   &c.Objects,
		"ai.json":        &c.AI,
	}
	for _, f := range Files {
		b, err := readFile(override, f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if err := json.Unmarshal(b, targets[f]); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
	}
	c.Sides = [2]SideDef{sides.RU, sides.UA}
	// Старые файлы данных без магазинов ПВО: разумные значения по умолчанию.
	for i := range c.Units {
		u := &c.Units[i]
		if u.Kind == "ad" && u.Magazine <= 0 {
			u.Magazine = 8
			if u.ReloadMin <= 0 {
				u.ReloadMin = 30
			}
		}
	}
	// Старые файлы правил без новых параметров ремонта.
	r := &c.Rules
	if r.RepairCrews <= 0 {
		r.RepairCrews = 6
	}
	if r.MoraleGood <= 0 {
		r.MoraleGood = 70
		r.MoraleBad = 30
		r.MoraleFrontHigh, r.MoraleFrontLow, r.MoraleLossMax = 1.10, 0.60, 1.5
		r.MoraleProdHigh, r.MoraleProdLow = 1.05, 0.70
		r.MoraleLevyHigh, r.MoraleLevyLow, r.MoraleMobPenalty = 1.20, 0.50, 2.0
		r.PropagandaCost, r.PropagandaGain, r.PropagandaCooldownH = 100, 10, 6
		r.MoraleKeyHit, r.MoraleKeyLoss, r.MoraleRepel, r.MoraleRepelMin = 1.5, 1.0, 2.0, 10
	}
	if r.MoraleGainRef <= 0 {
		r.MoraleGainRef, r.MoraleGainFloor, r.MoraleFatigue, r.MoraleFatigueH, r.MoraleCityRepeat = 50, 0.15, 0.35, 6, 0.25
	}
	if len(r.PlacementKinds) == 0 {
		r.PlacementKinds = []string{"ad", "radar", "reb", "rtr", "launcher"}
	}
	if r.LowInterceptor <= 0 {
		r.LowInterceptor, r.KeepStockBatch, r.SellKeepMin = 20, 20, 1000
	}
	if len(r.SellRate) == 0 {
		r.SellRate = map[string]float64{"fuel": 0.04, "steel": 0.06}
	}
	if len(r.Directions) != 4 {
		r.Directions = []DirAnchor{{"Киев", 30.8, 51.0}, {"Харьков", 36.5, 50.0}, {"Донбасс", 38.2, 48.2}, {"Крым", 35.0, 47.0}}
	}
	if len(r.RuVictoryCities) == 0 {
		r.RuVictoryCities = []string{"Киев", "Харьков", "Одесса", "Днепр"}
	}
	if r.RetreatTiles <= 0 {
		r.RetreatTiles, r.RetreatDamage = 4, 0.25
	}
	if r.CityMinPop <= 0 {
		r.CityMinPop = 100000
	}
	if r.BridgePassFrac <= 0 {
		r.BridgePassFrac = 0.5
	}
	if r.BomberWarnMin <= 0 {
		r.BomberWarnMin = 30
	}
	if r.AutoImportReserve <= 0 {
		r.AutoImportReserve, r.AutoImportEveryMin = 300, 30
	}
	if r.RailKmh <= 0 {
		r.RailKmh, r.RailBoardMin, r.RailAlightMin, r.RailStationTiles = 110, 20, 10, 2
	}
	if len(r.PlacementBuildings) == 0 {
		r.PlacementBuildings = []string{"training_center"}
	}
	if r.SocialMinPop <= 0 {
		r.SocialMinPop, r.SocialCityKm, r.SocialDelayMin, r.SocialDelayMax = 20000, 8, 15, 60
		r.IntelPoints, r.ForcesPoints = 0.3, 0.5
	}
	if r.AircraftLoss <= 0 {
		r.AircraftLoss = 0.03
	}
	if r.StrikeDroneVisionKm <= 0 {
		r.StrikeDroneVisionKm = 5
	}
	if r.RepairPerType <= 0 {
		r.RepairPerType = 2
	}
	if r.RepairRuinMult <= 0 {
		r.RepairRuinMult = 0.5
	}
	if r.DamageFloor <= 0 {
		r.DamageFloor = 0.15
	}
	if r.DamageCurve <= 0 {
		r.DamageCurve = 1.3
	}
	if c.AI.ThinkMin <= 0 {
		c.AI.ThinkMin = 3
	}
	c.index()
	return c, c.validate()
}

// DefaultFile отдаёт встроенный файл (для выгрузки шаблонов рядом с .exe).
func DefaultFile(name string) ([]byte, error) { return defaults.ReadFile("defaults/" + name) }

func (c *Catalog) index() {
	c.BuildingByID = map[string]*BuildingType{}
	for i := range c.Buildings {
		c.BuildingByID[c.Buildings[i].ID] = &c.Buildings[i]
	}
	c.UnitByID = map[string]*UnitType{}
	for i := range c.Units {
		c.UnitByID[c.Units[i].ID] = &c.Units[i]
	}
	c.MunitionByID = map[string]*MunitionType{}
	for i := range c.Munitions {
		c.MunitionByID[c.Munitions[i].ID] = &c.Munitions[i]
	}
	c.FrontByID = map[string]*FrontType{}
	for i := range c.Front {
		c.FrontByID[c.Front[i].ID] = &c.Front[i]
	}
	for s := 0; s < 2; s++ {
		c.TechByID[s] = map[string]*Tech{}
		c.LineByID[s] = map[string]*TechLine{}
		c.LineSteps[s] = map[string][]*Tech{}
		list := c.Tech[SideKeys[s]]
		for i := range list {
			c.TechByID[s][list[i].ID] = &list[i]
		}
		lines := c.Lines[SideKeys[s]]
		for i := range lines {
			c.LineByID[s][lines[i].ID] = &lines[i]
		}
		for i := range list {
			t := &list[i]
			if t.Line != "" {
				c.LineSteps[s][t.Line] = append(c.LineSteps[s][t.Line], t)
			}
		}
		for id, steps := range c.LineSteps[s] {
			sort.SliceStable(steps, func(a, b int) bool { return steps[a].Step < steps[b].Step })
			// Следующая ступень требует предыдущую.
			for k := 1; k < len(steps); k++ {
				prev := steps[k-1].ID
				have := false
				for _, r := range steps[k].Requires {
					have = have || r == prev
				}
				if !have {
					steps[k].Requires = append(steps[k].Requires, prev)
				}
			}
			c.LineSteps[s][id] = steps
		}
	}
}

// ItemName возвращает название любого производимого предмета.
func (c *Catalog) ItemName(id string) string {
	if m, ok := c.MunitionByID[id]; ok {
		return m.Name
	}
	if u, ok := c.UnitByID[id]; ok {
		return u.Name
	}
	if f, ok := c.FrontByID[id]; ok {
		return f.Name
	}
	if b, ok := c.BuildingByID[id]; ok {
		return b.Name
	}
	for i, k := range ResKeys {
		if id == "res:"+k {
			return ResNames[i]
		}
	}
	return id
}

// ItemCost возвращает стоимость, категорию и очки мощности предмета.
func (c *Catalog) ItemCost(id string) (Res, string, float64, bool) {
	if m, ok := c.MunitionByID[id]; ok {
		return ToRes(m.Cost), m.Cap, m.CapPoints, true
	}
	if u, ok := c.UnitByID[id]; ok {
		return ToRes(u.Cost), u.Cap, u.CapPoints, true
	}
	if f, ok := c.FrontByID[id]; ok {
		return ToRes(f.Cost), f.Cap, f.CapPoints, true
	}
	return Res{}, "", 0, false
}

// ItemBatch — сколько штук предмета выпускается за один цикл производства (партия снаряжения; по умолчанию 1).
func (c *Catalog) ItemBatch(id string) float64 {
	if m, ok := c.MunitionByID[id]; ok && m.Batch > 1 {
		return m.Batch
	}
	return 1
}

// satTech — есть ли у стороны s спутник name, включаемый исследованием tech.
func (c *Catalog) satTech(s int, name, tech string) bool {
	for _, sat := range c.Sides[s].Satellites {
		if sat.Name == name && sat.Tech == tech {
			return true
		}
	}
	return false
}

func (c *Catalog) known(id string) bool {
	if _, ok := c.MunitionByID[id]; ok {
		return true
	}
	if _, ok := c.UnitByID[id]; ok {
		return true
	}
	if _, ok := c.FrontByID[id]; ok {
		return true
	}
	if _, ok := c.BuildingByID[id]; ok {
		return true
	}
	for _, k := range ResKeys {
		if id == "res:"+k {
			return true
		}
	}
	return false
}

func (c *Catalog) validate() error {
	for _, u := range c.Units {
		if u.Interceptor != "" && c.MunitionByID[u.Interceptor] == nil {
			return fmt.Errorf("юнит %s: неизвестная зенитная ракета %s", u.ID, u.Interceptor)
		}
		for _, m := range u.Munitions {
			if c.MunitionByID[m] == nil {
				return fmt.Errorf("юнит %s: неизвестный боеприпас %s", u.ID, m)
			}
		}
	}
	for s := 0; s < 2; s++ {
		for _, t := range c.Tech[SideKeys[s]] {
			for _, u := range t.Unlocks {
				if strings.HasPrefix(u, "sat:") {
					if !c.satTech(s, u[4:], t.ID) {
						return fmt.Errorf("технология %s: нет спутника %s с tech=%s", t.ID, u[4:], t.ID)
					}
				} else if !c.known(u) {
					return fmt.Errorf("технология %s: неизвестный предмет %s", t.ID, u)
				}
			}
			if t.Line != "" && c.LineByID[s][t.Line] == nil {
				return fmt.Errorf("технология %s: неизвестная линейка %s", t.ID, t.Line)
			}
			for _, r := range t.Requires {
				if c.TechByID[s][r] == nil {
					return fmt.Errorf("технология %s: неизвестное требование %s", t.ID, r)
				}
			}
		}
		for _, l := range c.Lines[SideKeys[s]] {
			for _, id := range l.Start {
				if !c.known(id) {
					return fmt.Errorf("линейка %s: неизвестная стартовая версия %s", l.ID, id)
				}
			}
			prev := 0.0
			for k, t := range c.LineSteps[s][l.ID] {
				if t.Step != k+1 {
					return fmt.Errorf("линейка %s: ступени должны идти подряд с 1 (у %s — %d)", l.ID, t.ID, t.Step)
				}
				if t.Cost <= prev {
					return fmt.Errorf("линейка %s: ступень %s должна быть дороже предыдущей", l.ID, t.ID)
				}
				prev = t.Cost
			}
		}
		for _, sat := range c.Sides[s].Satellites {
			if sat.Tech != "" && c.TechByID[s][sat.Tech] == nil {
				return fmt.Errorf("спутник %s: неизвестное исследование %s", sat.Name, sat.Tech)
			}
		}
		sd := c.Sides[s]
		for _, id := range sd.Unlocked {
			if !c.known(id) {
				return fmt.Errorf("%s: неизвестный стартовый предмет %s", SideKeys[s], id)
			}
		}
		for id := range sd.Stocks {
			if c.MunitionByID[id] == nil {
				return fmt.Errorf("%s: неизвестный запас %s", SideKeys[s], id)
			}
		}
		for _, u := range sd.Units {
			if c.UnitByID[u.Type] == nil {
				return fmt.Errorf("%s: неизвестный стартовый юнит %s", SideKeys[s], u.Type)
			}
		}
		for _, im := range sd.Imports {
			if !c.known(im.Item) {
				return fmt.Errorf("%s: импорт %s: неизвестный предмет %s", SideKeys[s], im.ID, im.Item)
			}
		}
		for _, a := range sd.Aid {
			for id := range a.Items {
				if !c.known(id) {
					return fmt.Errorf("%s: помощь %s: неизвестный предмет %s", SideKeys[s], a.ID, id)
				}
			}
		}
		for _, p := range sd.Airspace {
			if len(p.Countries) == 0 {
				return fmt.Errorf("%s: небо %s: не указаны страны", SideKeys[s], p.ID)
			}
			for _, k := range p.Countries {
				if _, ok := world.CountryCodes[k]; !ok {
					return fmt.Errorf("%s: небо %s: неизвестная страна %s", SideKeys[s], p.ID, k)
				}
			}
		}
		for _, p := range sd.Sanctions {
			for k := range p.Effects {
				if _, ok := SanctionKeys[k]; !ok {
					return fmt.Errorf("%s: санкции %s: неизвестный штраф %s", SideKeys[s], p.ID, k)
				}
			}
			if t := p.Trigger; t != nil {
				switch t.Kind {
				case "disable_building", "disable_type", "hit_region", "capture":
				default:
					return fmt.Errorf("%s: санкции %s: неизвестное условие %s", SideKeys[s], p.ID, t.Kind)
				}
			}
		}
	}
	for _, o := range c.Objects {
		if c.BuildingByID[o.Type] == nil {
			return fmt.Errorf("объект %s: неизвестный тип %s", o.Name, o.Type)
		}
	}
	return nil
}
