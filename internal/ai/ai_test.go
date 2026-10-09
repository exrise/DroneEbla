package ai

import (
	"math"
	"testing"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
)

func newWorld(t *testing.T) *sim.World {
	t.Helper()
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := sim.New(cat, m, true)
	w.Solo = true
	return w
}

// ИИ за Украину сам ведёт экономику, фронт, удары и ПВО.
func TestAIPlays(t *testing.T) {
	w := newWorld(t)
	a := New(w.Catalog(), data.UA)
	ok := map[string]int{}
	var strikes int
	a.OnCommand = func(c sim.Command, err string) {
		if err == "" {
			ok[c.Kind]++
			if c.Kind == sim.CmdStrike {
				strikes++
			}
		}
	}
	for k := 0; k < 60*30; k++ {
		w.Step(1)
		a.Tick(w)
	}
	t.Logf("успешные приказы ИИ: %v", ok)
	for _, kind := range []string{sim.CmdResearch, sim.CmdOrderAdd, sim.CmdAlloc, sim.CmdPosture, sim.CmdStrike} {
		if ok[kind] == 0 {
			t.Errorf("ИИ ни разу не отдал приказ %q", kind)
		}
	}
	if w.Sides[data.UA].Research == "" && len(w.Sides[data.UA].Researched) == 0 {
		t.Error("ИИ не исследует")
	}
}

// ИИ бьёт только по тому, что известно его разведке.
func TestAIFogOfWar(t *testing.T) {
	w := newWorld(t)
	a := New(w.Catalog(), data.UA)
	checked := 0
	a.OnCommand = func(c sim.Command, err string) {
		if c.Kind != sim.CmdStrike || err != "" {
			return
		}
		v := w.BuildView(data.UA, 0)
		for _, ct := range v.Contacts {
			if ct.Kind == 0 && math.Hypot(ct.X-c.X, ct.Y-c.Y) < 1.5 {
				checked++
				return
			}
		}
		t.Errorf("удар по точке (%.0f, %.0f), о которой у ИИ нет разведданных", c.X, c.Y)
	}
	for k := 0; k < 60*20; k++ {
		w.Step(1)
		a.Tick(w)
	}
	t.Logf("проверено ударов: %d", checked)
	if checked == 0 {
		t.Error("ИИ не совершил ни одного удара")
	}
}

// ИИ за Россию (одиночная игра за Украину): расставляет резерв, исследует, заказывает и бьёт.
func TestAIPlaysRussia(t *testing.T) {
	w := newWorld(t)
	w.StartPlacement()
	a := New(w.Catalog(), data.RU)
	ok := map[string]int{}
	a.OnCommand = func(c sim.Command, err string) {
		if err == "" {
			ok[c.Kind]++
		}
	}
	a.Place(w)
	if !w.Sides[data.RU].Ready {
		t.Fatal("ИИ России не расставил резерв и не нажал «Готово»")
	}
	New(w.Catalog(), data.UA).Place(w)
	for k := 0; k < 60*36 && w.Winner < 0; k++ {
		w.Step(1)
		a.Tick(w)
	}
	t.Logf("успешные приказы ИИ России: %v", ok)
	for _, kind := range []string{sim.CmdResearch, sim.CmdOrderAdd, sim.CmdAlloc, sim.CmdPosture, sim.CmdStrike} {
		if ok[kind] == 0 {
			t.Errorf("ИИ России ни разу не отдал приказ %q", kind)
		}
	}
}

// Маршрут дронов огибает известную зону ПВО и не трогает зоны, накрывающие саму цель.
func TestRouteAvoidsAD(t *testing.T) {
	a := &AI{}
	if r := a.route(0, 0, 100, 0); r != nil {
		t.Fatalf("без известной ПВО маршрут прямой, а вышло %v", r)
	}
	a.zones = []zone{{x: 50, y: 0, r: 20}}
	r := a.route(0, 0, 100, 0)
	if len(r) == 0 {
		t.Fatal("маршрут через зону ПВО не обойдён")
	}
	prev := sim.Pt{X: 0, Y: 0}
	for _, p := range append(r, sim.Pt{X: 100, Y: 0}) {
		// ни один отрезок не должен заходить в зону
		for k := 0.0; k <= 1; k += 0.01 {
			x, y := prev.X+(p.X-prev.X)*k, prev.Y+(p.Y-prev.Y)*k
			if math.Hypot(x-50, y) < 20-0.001 {
				t.Fatalf("маршрут %v заходит в зону ПВО в точке (%.1f, %.1f)", r, x, y)
			}
		}
		prev = p
	}
	// Цель внутри зоны: обходить нечего.
	a.zones = []zone{{x: 100, y: 0, r: 20}}
	if r := a.route(0, 0, 100, 0); r != nil {
		t.Fatalf("зона накрывает цель — маршрут должен быть прямым, а вышло %v", r)
	}
}

// «Умный» ИИ держит парк ПВО: разбитые комплексы заказываются заново, электроника закупается пачками.
func TestSmartFleetAndImports(t *testing.T) {
	w := newWorld(t)
	w.Solo = false
	w.StartPlacement()
	ru, ua := New(w.Catalog(), data.RU), New(w.Catalog(), data.UA)
	if !ru.cfg.Smart || !ua.cfg.Smart {
		t.Skip("в ai.json выключен умный режим")
	}
	ru.Place(w)
	ua.Place(w)
	orders := map[string]int{}
	imports := 0
	ru.OnCommand = func(c sim.Command, err string) {
		if err != "" {
			return
		}
		switch c.Kind {
		case sim.CmdOrderAdd:
			orders[c.Item]++
		case sim.CmdImport:
			imports++
		}
	}
	// Сутки игры, затем уничтожаем всю ПВО России и даём ещё несколько часов.
	for k := 0; k < 24*60; k++ {
		w.Step(1)
		ru.Tick(w)
		ua.Tick(w)
	}
	adBefore := 0
	for id, u := range w.Units {
		if u.Side == data.RU {
			if ut := w.Catalog().UnitByID[u.Type]; ut != nil && ut.Kind == "ad" && ut.Magazine > 0 && ut.RangeKm >= 100 {
				delete(w.Units, id)
				adBefore++
			}
		}
	}
	if adBefore == 0 {
		t.Skip("у России нет дальней ПВО для проверки")
	}
	for k := 0; k < 6*60; k++ {
		w.Step(1)
		ru.Tick(w)
	}
	got := 0
	for _, item := range []string{"s400", "s300", "buk_ru", "buk_m3", "pantsir", "pantsir_s2", "tor"} {
		got += orders[item]
	}
	if got == 0 {
		t.Errorf("уничтожено %d комплексов ПВО, а ИИ не заказал ни одного (заказы: %v)", adBefore, orders)
	}
	if imports < 2 {
		t.Errorf("ИИ почти не закупает (%d закупок за сутки и 6 часов) — деньги должны тратиться", imports)
	}
}

// Короткая дуэль двух «умных» ботов не падает и обе стороны действуют.
func TestDuelSmoke(t *testing.T) {
	w := newWorld(t)
	w.Solo = false
	w.StartPlacement()
	bots := [2]*AI{New(w.Catalog(), data.RU), New(w.Catalog(), data.UA)}
	n := [2]int{}
	for s := range bots {
		s := s
		bots[s].OnCommand = func(c sim.Command, err string) {
			if err == "" {
				n[s]++
			}
		}
		bots[s].Place(w)
	}
	for k := 0; k < 12*60; k++ {
		w.Step(1)
		for _, b := range bots {
			b.Tick(w)
		}
	}
	if n[0] < 20 || n[1] < 20 {
		t.Errorf("слишком мало приказов за 12 часов: %v", n)
	}
}

// Коридор ИИ Украины над Польшей и Прибалтикой проходим, когда небо открыто, и закрыт, пока нет.
func TestCorridorAirspace(t *testing.T) {
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.Load()
	if err != nil {
		t.Fatal(err)
	}
	chains := cat.AI.Sides["ua"].Corridors
	if len(chains) == 0 {
		t.Fatal("у ИИ Украины нет коридоров")
	}
	open := map[uint8]bool{}
	for _, c := range []uint8{world.CountryPoland, world.CountryLithuania, world.CountryLatvia, world.CountryEstonia, world.CountryFinland} {
		open[c] = true
	}
	for _, chain := range chains {
		var prev sim.Pt
		for i, c := range chain {
			x, y := m.Project(c[0], c[1])
			p := sim.Pt{X: x, Y: y}
			if i == 0 {
				prev = p
				continue
			}
			if got := sim.AirspaceBlock(m, false, open, prev, p); got != 0 {
				t.Fatalf("сегмент %d коридора закрыт страной %d даже при открытом небе", i, got)
			}
			if got := sim.AirspaceBlock(m, false, nil, prev, p); got == 0 && i > 1 {
				t.Fatalf("сегмент %d коридора свободен при закрытом небе — коридор не идёт над Европой", i)
			}
			prev = p
		}
	}
}
