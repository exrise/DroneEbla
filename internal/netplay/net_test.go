package netplay

import (
	"testing"
	"time"

	"github.com/exrise/droneebla/internal/ai"
	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
	"github.com/exrise/droneebla/internal/world"
)

func TestHostClient(t *testing.T) {
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := sim.New(cat, m, false)
	hash := DataHash("")
	h, err := NewHost(w, data.RU, 27999, hash)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := Connect("127.0.0.1:27999", "другой"); err == nil {
		t.Fatal("клиент с другими данными должен быть отклонён")
	}
	cl, err := Connect("127.0.0.1:27999", hash)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if cl.Side() != data.UA {
		t.Fatal("клиент должен играть за Украину")
	}
	deadline := time.Now().Add(5 * time.Second)
	for cl.View() == nil && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	v := cl.View()
	if v == nil {
		t.Fatal("клиент не получил представление")
	}
	if v.Side != data.UA {
		t.Fatal("клиент получил чужое представление")
	}
	for _, u := range v.Units {
		if u.Side != data.UA {
			t.Fatal("утечка чужих юнитов")
		}
	}
	cl.Send(sim.Command{Kind: sim.CmdPosture, Int: sim.PostureDefense})
	cl.Send(sim.Command{Kind: sim.CmdResearch, Item: "ua_fpv"})
	time.Sleep(500 * time.Millisecond)
	if w.Sides[data.UA].Research != "ua_fpv" {
		t.Fatal("приказ клиента не дошёл")
	}
	// Сначала расстановка: время стоит, у клиента есть свой резерв и нет резерва хоста.
	if w.Time != 0 || !v.Placement {
		t.Fatalf("до готовности обеих сторон время должно стоять (t=%.1f, placement=%v)", w.Time, v.Placement)
	}
	if len(v.Reserve) == 0 || v.Reserve["s400"] > 0 {
		t.Fatalf("резерв клиента: %v", v.Reserve)
	}
	h.Advance(0, func(w *sim.World) {
		ai.New(w.Catalog(), data.RU).Place(w)
		ai.New(w.Catalog(), data.UA).Place(w)
	})
	time.Sleep(800 * time.Millisecond)
	if w.Time <= 0 {
		t.Fatal("время не идёт после готовности обеих сторон")
	}
}

// В одиночной игре ИИ управляет Украиной, а смена стороны заблокирована.
func TestSolo(t *testing.T) {
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.Load()
	if err != nil {
		t.Fatal(err)
	}
	h := NewSolo(sim.New(cat, m, true), data.RU)
	defer h.Close()
	h.SetSide(data.UA)
	if h.Side() != data.RU {
		t.Fatal("в одиночной игре сторону менять нельзя")
	}
	time.Sleep(300 * time.Millisecond)
	// Сначала расстановка: время стоит, ИИ расставил резерв и готов.
	var placing, aiReady bool
	h.Advance(0, func(w *sim.World) { placing, aiReady = w.Placement, w.Sides[data.UA].Ready })
	if !placing || !aiReady {
		t.Fatalf("ожидалась расстановка и готовность ИИ: placement=%v ready=%v", placing, aiReady)
	}
	if v := h.View(); v == nil || !v.Placement || len(v.Reserve) == 0 {
		t.Fatal("игрок должен получить резерв для расстановки")
	}
	h.Advance(0, func(w *sim.World) {
		ai.New(w.Catalog(), data.RU).Place(w) // человек расставляет (здесь — автоматически)
	})
	time.Sleep(700 * time.Millisecond)
	var research string
	var solo bool
	h.Advance(0, func(w *sim.World) { research, solo = w.Sides[data.UA].Research, w.Solo })
	if !solo {
		t.Fatal("не выставлен флаг одиночной игры")
	}
	if research == "" {
		t.Fatal("ИИ не выбрал исследование")
	}
	if v := h.View(); v == nil || v.Side != data.RU || !v.Solo {
		t.Fatal("неверное представление игрока")
	}
}
