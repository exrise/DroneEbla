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
