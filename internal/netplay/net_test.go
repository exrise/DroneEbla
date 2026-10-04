package netplay

import (
	"testing"
	"time"

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
	if w.Time <= 0 {
		t.Fatal("время не идёт")
	}
}
