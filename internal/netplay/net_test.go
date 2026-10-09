package netplay

import (
	"encoding/json"
	"net"
	"strings"
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
	if cl.Side() != -1 {
		t.Fatal("до выбора стороны клиент нигде не играет")
	}
	cl.PickSide(data.UA)
	waitFor(t, "выбор стороны", func() bool { return cl.Side() == data.UA })
	h.StartGame()
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

// Одиночная игра за Украину: ИИ ведёт Россию, сторона человека запоминается.
func TestSoloUkraine(t *testing.T) {
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.Load()
	if err != nil {
		t.Fatal(err)
	}
	h := NewSolo(sim.New(cat, m, true), data.UA)
	defer h.Close()
	h.SetSide(data.RU)
	if h.Side() != data.UA {
		t.Fatal("в одиночной игре сторону менять нельзя")
	}
	time.Sleep(300 * time.Millisecond)
	var human int
	var aiReady bool
	h.Advance(0, func(w *sim.World) { human, aiReady = w.Human, w.Sides[data.RU].Ready })
	if human != data.UA || !aiReady {
		t.Fatalf("human=%d, ИИ России готов=%v", human, aiReady)
	}
	h.Advance(0, func(w *sim.World) { ai.New(w.Catalog(), data.UA).Place(w) })
	time.Sleep(700 * time.Millisecond)
	var research string
	h.Advance(0, func(w *sim.World) { research = w.Sides[data.RU].Research })
	if research == "" {
		t.Fatal("ИИ России не выбрал исследование")
	}
	if v := h.View(); v == nil || v.Side != data.UA || !v.Solo {
		t.Fatal("неверное представление игрока")
	}
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(30 * time.Millisecond)
	}
}

// До 6 игроков: до 3 на сторону, общий вид и приказы соратников, время — только у хоста.
func TestMultiplayer(t *testing.T) {
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
	h, err := NewHost(w, data.RU, 27998, hash)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	connect := func() *Client {
		c, err := Connect("127.0.0.1:27998", hash)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		return c
	}
	a, b, c, d := connect(), connect(), connect(), connect()
	a.PickSide(data.UA)
	b.PickSide(data.UA)
	c.PickSide(data.RU)
	d.PickSide(data.UA)
	waitFor(t, "лобби", func() bool {
		l := h.Lobby()
		return l.Count(data.UA) == 3 && l.Count(data.RU) == 2
	})
	// Четвёртый за Украину не помещается: он остаётся без стороны.
	e := connect()
	e.PickSide(data.UA)
	time.Sleep(300 * time.Millisecond)
	if e.Side() != -1 {
		t.Fatal("четвёртый игрок за сторону не должен помещаться")
	}
	// Седьмой игрок (хост + 5 клиентов уже в игре) отклоняется.
	if g, err := Connect("127.0.0.1:27998", hash); err == nil {
		g.Close()
		t.Fatal("в игре не может быть больше 6 игроков")
	}
	e.PickSide(data.RU)
	waitFor(t, "третий за Россию", func() bool { return h.Lobby().Count(data.RU) == 3 })
	// Партия ещё не началась: приказы не принимаются.
	a.Send(sim.Command{Kind: sim.CmdResearch, Item: "ua_fpv"})
	time.Sleep(200 * time.Millisecond)
	if w.Sides[data.UA].Research != "" {
		t.Fatal("до начала партии приказы принимать нельзя")
	}
	h.StartGame()
	waitFor(t, "представления", func() bool { return a.View() != nil && b.View() != nil && c.View() != nil })
	if a.View().Side != data.UA || c.View().Side != data.RU {
		t.Fatal("игроки получили представление не своей стороны")
	}
	if !a.View().TimeLocked {
		t.Fatal("у клиента время должно быть заблокировано")
	}
	// Приказ соратника действует на общую сторону.
	b.Send(sim.Command{Kind: sim.CmdResearch, Item: "ua_fpv"})
	waitFor(t, "приказ соратника", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return w.Sides[data.UA].Research == "ua_fpv"
	})
	// Скорость и пауза — только у хоста.
	before := w.Sides[data.UA].Speed
	a.Send(sim.Command{Kind: sim.CmdSpeed, Int: 5})
	a.Send(sim.Command{Kind: sim.CmdPause, Int: 1})
	time.Sleep(300 * time.Millisecond)
	h.mu.Lock()
	if w.Sides[data.UA].Speed != before || w.Sides[data.UA].Pausing {
		h.mu.Unlock()
		t.Fatal("клиент не должен управлять скоростью и паузой")
	}
	h.mu.Unlock()
	h.Send(sim.Command{Kind: sim.CmdSpeed, Int: 3})
	h.mu.Lock()
	sp := w.EffectiveSpeed()
	h.mu.Unlock()
	if sp != 3 {
		t.Fatalf("скорость задаёт хост: %d", sp)
	}
	// Отключение: слот освобождается; если сторона опустела — пауза.
	a.Close()
	b.Close()
	d.Close()
	waitFor(t, "освобождение слотов", func() bool { return h.Lobby().Count(data.UA) == 0 })
	if h.Status() == "" {
		t.Fatal("у стороны без игроков игра должна вставать на паузу")
	}
	// Возврат в свободный слот.
	r := connect()
	r.PickSide(data.UA)
	waitFor(t, "возврат", func() bool { return h.Lobby().Count(data.UA) == 1 })
	if h.Status() != "" {
		t.Fatal("после возвращения игрока игра должна продолжиться")
	}
	waitFor(t, "представление вернувшегося", func() bool { return r.View() != nil })
}

// Объявление хоста, пришедшее по UDP, попадает в список найденных игр.
func TestDiscovery(t *testing.T) {
	d := NewDiscovery("h1")
	defer d.Close()
	if d.Err() != nil {
		t.Skip("порт поиска занят:", d.Err())
	}
	c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: DiscoveryPort})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	send := func(a announce) {
		b, _ := json.Marshal(a)
		c.Write(b)
	}
	send(announce{Magic: announceMagic, Version: Version, Hash: "h1", Port: 27015, Name: "Хост", Players: 2})
	send(announce{Magic: announceMagic, Version: Version, Hash: "other", Port: 27099, Name: "Чужой", Players: 1})
	send(announce{Magic: "мусор", Port: 1})
	var list []Found
	for i := 0; i < 50 && len(list) < 2; i++ {
		time.Sleep(20 * time.Millisecond)
		list = d.List()
	}
	if len(list) != 2 {
		t.Fatalf("найдено %d игр, ожидалось 2: %+v", len(list), list)
	}
	for _, f := range list {
		if f.Name == "Хост" && (!f.Compatible || f.Port != 27015 || f.Players != 2) {
			t.Fatalf("хост: %+v", f)
		}
		if f.Name == "Чужой" && f.Compatible {
			t.Fatal("игра с другими данными не должна быть совместима")
		}
	}
}

// Хост может добавить бота за сторону: партия стартует без второго игрока, бот расставляет резерв и играет.
func TestHostWithBot(t *testing.T) {
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := sim.New(cat, m, false)
	h, err := NewHost(w, data.RU, 27998, DataHash(""))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.StartGame()
	if h.Lobby().Started {
		t.Fatal("без второй стороны партия начаться не должна")
	}
	h.ToggleBot(data.UA)
	if !h.Lobby().Bots[data.UA] {
		t.Fatal("бот не добавлен")
	}
	h.ToggleBot(data.RU) // и за сторону хоста тоже можно
	h.ToggleBot(data.RU)
	h.StartGame()
	if !h.Lobby().Started {
		t.Fatal("с ботом партия должна начаться")
	}
	h.ToggleBot(data.UA) // после начала партии убрать бота нельзя
	if !h.Lobby().Bots[data.UA] {
		t.Fatal("бота убрали во время партии")
	}
	if s := h.Status(); s != "" {
		t.Fatalf("за стороной с ботом пауза не нужна: %q", s)
	}
	// Бот расставляет резерв и нажимает «Готово».
	waitFor(t, "бот готов", func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return w.Sides[data.UA].Ready
	})
}

// Обрыв связи: клиент сам переподключается, возвращается на свою сторону, а хост пишет причину.
func TestClientReconnect(t *testing.T) {
	cat, _ := data.Load("")
	m, _ := world.Load()
	w := sim.New(cat, m, false)
	hash := DataHash("")
	h, err := NewHost(w, data.RU, 27997, hash)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	cl, err := Connect("127.0.0.1:27997", hash)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	cl.PickSide(data.UA)
	waitFor(t, "выбор стороны", func() bool { return cl.Side() == data.UA })
	h.StartGame()
	waitFor(t, "первое представление", func() bool { return cl.View() != nil })
	oldID := cl.id
	cl.k.c.Close() // обрыв
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		cl.mu.Lock()
		id := cl.id
		cl.mu.Unlock()
		if id != oldID && cl.Side() == data.UA {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if cl.Side() != data.UA {
		t.Fatalf("после обрыва клиент не вернулся на сторону; статус: %q", cl.Status())
	}
	found := false
	for _, s := range h.Messages() {
		if strings.Contains(s, "отключился") {
			found = true
		}
	}
	if !found {
		t.Fatal("хост не сообщил об отключении с причиной")
	}
}
