package data

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed defaults/*.json
var defaults embed.FS

// Files — имена файлов каталога.
var Files = []string{"rules.json", "buildings.json", "units.json", "munitions.json", "front.json", "tech.json", "sides.json", "objects.json"}

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
		"sides.json":     &sides,
		"objects.json":   &c.Objects,
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
		list := c.Tech[SideKeys[s]]
		for i := range list {
			c.TechByID[s][list[i].ID] = &list[i]
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
				if !c.known(u) {
					return fmt.Errorf("технология %s: неизвестный предмет %s", t.ID, u)
				}
			}
			for _, r := range t.Requires {
				if c.TechByID[s][r] == nil {
					return fmt.Errorf("технология %s: неизвестное требование %s", t.ID, r)
				}
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
	}
	for _, o := range c.Objects {
		if c.BuildingByID[o.Type] == nil {
			return fmt.Errorf("объект %s: неизвестный тип %s", o.Name, o.Type)
		}
	}
	return nil
}
