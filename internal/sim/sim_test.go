package sim

import (
	"bytes"
	"encoding/json"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/world"
)

func newTestWorld(t *testing.T) *World {
	t.Helper()
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := world.Load()
	if err != nil {
		t.Fatal(err)
	}
	w := New(cat, m, false)
	w.rng.Seed(1)
	return w
}

func findBuilding(w *World, name string) *Building {
	for _, b := range w.Buildings {
		if b.Name == name {
			return b
		}
	}
	return nil
}

func run(w *World, minutes float64) {
	for k := 0.0; k < minutes; k++ {
		w.Step(1)
	}
}

func TestInit(t *testing.T) {
	w := newTestWorld(t)
	if len(w.Buildings) < 150 {
		t.Fatalf("мало зданий: %d", len(w.Buildings))
	}
	for s := 0; s < 2; s++ {
		n := 0
		for _, b := range w.Buildings {
			if b.Side == s {
				n++
			}
		}
		if n < 50 {
			t.Fatalf("сторона %d: мало зданий %d", s, n)
		}
		if len(w.Sides[s].Known) < 50 {
			t.Fatalf("сторона %d: нет довоенной разведки", s)
		}
		if len(w.FrontTiles(s)) < 100 {
			t.Fatalf("сторона %d: мало фронтовых тайлов %d", s, len(w.FrontTiles(s)))
		}
	}
	if b := findBuilding(w, "Кременчугский НПЗ"); b == nil || b.Side != data.UA {
		t.Fatal("Кременчугский НПЗ должен быть у Украины")
	}
	if b := findBuilding(w, "Крымский мост"); b == nil || b.Side != data.RU {
		t.Fatal("Крымский мост должен быть у РФ")
	}
	if b := findBuilding(w, "Старобешевская ТЭС"); b == nil || b.Side != data.RU {
		t.Fatal("Старобешевская ТЭС (ОРДЛО) должна быть у РФ")
	}
}

func TestEconomyRuns(t *testing.T) {
	w := newTestWorld(t)
	start := time.Now()
	run(w, 600)
	t.Logf("10 игровых часов за %v", time.Since(start))
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		t.Logf("%s: деньги %.0f топливо %.0f сталь %.0f электроника %.0f боеприпасы %.0f, доход %.0f/ч, мораль %.1f, энергия %.0f/%.0f",
			data.SideNames[s], sd.Res[0], sd.Res[1], sd.Res[2], sd.Res[3], sd.Res[4], sd.Income, sd.Morale, sd.Power[0], sd.Power[1])
		if sd.Res[data.ResMoney] <= 0 || sd.Income <= 0 {
			t.Fatalf("%s: нет денег", data.SideNames[s])
		}
		if sd.Res[data.ResFuel] <= 0 || sd.Res[data.ResSteel] <= 0 {
			t.Fatalf("%s: экономика не производит", data.SideNames[s])
		}
		if sd.Blackout > 0.01 {
			t.Fatalf("%s: блэкаут в мирное время %.2f", data.SideNames[s], sd.Blackout)
		}
	}
}

func TestPowerStrike(t *testing.T) {
	w := newTestWorld(t)
	run(w, 5)
	ua := w.Sides[data.UA]
	// Уничтожаем всю генерацию Украины (кроме АЭС) — должен быть блэкаут.
	for _, b := range w.Buildings {
		if b.Side == data.UA && (b.Type == "tpp" || b.Type == "hpp") {
			b.HP = 0
		}
		if b.Side == data.UA && b.Type == "substation" {
			b.HP = 0
		}
	}
	run(w, 2)
	if ua.Blackout < 0.05 {
		t.Fatalf("ожидался блэкаут, доля %.2f", ua.Blackout)
	}
}

func TestStrikeAndIntercept(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ru := w.Sides[data.RU]
	sev := findBuilding(w, "Севастополь — база ЧФ")
	tgt := findBuilding(w, "Трипольская ТЭС")
	before := ru.Stocks["kalibr"]
	e := w.Strike(data.RU, StrikePlan{Source: sev.ID, Munition: "kalibr", Count: 10, Target: Pt{tgt.X, tgt.Y}})
	if e != "" {
		t.Fatal(e)
	}
	if ru.Stocks["kalibr"] != before-10 {
		t.Fatal("запас не списан")
	}
	ua := w.Sides[data.UA]
	ints := ua.Stocks["m_5v55"] + ua.Stocks["m_9m38"] + ua.Stocks["m_9m33"]
	hp := tgt.HP
	run(w, 120)
	if len(w.Projs) != 0 {
		t.Fatalf("ракеты всё ещё летят: %d", len(w.Projs))
	}
	used := ints - (ua.Stocks["m_5v55"] + ua.Stocks["m_9m38"] + ua.Stocks["m_9m33"])
	t.Logf("потрачено зенитных ракет: %.0f, HP цели %.0f → %.0f", used, hp, tgt.HP)
	if used == 0 {
		t.Fatal("ПВО не стреляло")
	}
	found := false
	for _, e := range ru.Events {
		if len(e.Text) > 10 && e.Text[:len("Итог удара")] == "Итог удара" {
			found = true
			t.Log(e.Text)
		}
	}
	if !found {
		t.Fatal("атакующий не получил итог удара")
	}
}

func TestStrikeValidation(t *testing.T) {
	w := newTestWorld(t)
	sev := findBuilding(w, "Севастополь — база ЧФ")
	tgt := findBuilding(w, "Трипольская ТЭС")
	if e := w.Strike(data.RU, StrikePlan{Source: sev.ID, Munition: "kalibr", Count: 1, Target: Pt{tgt.X, tgt.Y}}); e == "" {
		t.Fatal("удар в подготовительной фазе должен быть запрещён")
	}
	run(w, w.PrepEnd+1)
	npp := findBuilding(w, "Южно-Украинская АЭС")
	if e := w.Strike(data.RU, StrikePlan{Source: sev.ID, Munition: "kalibr", Count: 1, Target: Pt{npp.X, npp.Y}}); e == "" {
		t.Fatal("удар по АЭС должен быть запрещён")
	}
	// Точка-У не долетит до Москвы-края карты.
	var tochka *Unit
	for _, u := range w.Units {
		if u.Type == "tochka" {
			tochka = u
		}
	}
	far := findBuilding(w, "НЛМК (Липецк)")
	if e := w.Strike(data.UA, StrikePlan{Source: tochka.ID, Munition: "tochka_u", Count: 1, Target: Pt{far.X, far.Y}}); e == "" {
		t.Fatal("удар вне дальности должен быть запрещён")
	}
}

func TestFogFilter(t *testing.T) {
	w := newTestWorld(t)
	run(w, 3)
	v := w.BuildView(data.UA, 0)
	for _, u := range v.Units {
		if u.Side != data.UA {
			t.Fatal("в представлении чужой юнит")
		}
	}
	for _, b := range v.Buildings {
		if b.Side != data.UA {
			t.Fatal("в представлении чужое здание")
		}
	}
	// Мобильные юниты РФ в глубине не должны быть известны.
	for _, c := range v.Contacts {
		if c.Kind == 1 {
			if u, ok := w.Units[c.ID]; ok {
				i := w.tileOf(u.X, u.Y)
				if !w.visible[data.UA][i] && c.Source == "" {
					t.Fatalf("юнит %s известен без источника", u.Type)
				}
			}
		}
	}
}

func TestFrontMoves(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	// Лишаем Украину снабжения на Донбассе и ставим РФ в наступление.
	for _, b := range w.Buildings {
		if b.Side == data.UA && w.cat.BuildingByID[b.Type].Supply > 0 {
			b.HP = 0
		}
	}
	w.Sides[data.RU].PostureDir = [3]int{PostureOffense, PostureOffense, PostureOffense}
	w.Sides[data.UA].Front[1].Men = 5
	ownedBefore := 0
	for _, o := range w.Owner {
		if o == 1 {
			ownedBefore++
		}
	}
	run(w, 600)
	owned := 0
	for _, o := range w.Owner {
		if o == 1 {
			owned++
		}
	}
	t.Logf("тайлов у РФ: %d → %d", ownedBefore, owned)
	if owned <= ownedBefore {
		t.Fatal("фронт не сдвинулся")
	}
}

func TestOrdersAndResearch(t *testing.T) {
	w := newTestWorld(t)
	if e := w.Apply(Command{Kind: CmdOrderAdd, Side: data.RU, Item: "kalibr", Count: 5}); e != "" {
		t.Fatal(e)
	}
	if e := w.Apply(Command{Kind: CmdResearch, Side: data.RU, Item: "ru_geran2"}); e != "" {
		t.Fatal(e)
	}
	before := w.Sides[data.RU].Stocks["kalibr"]
	run(w, 60*40)
	if w.Sides[data.RU].Stocks["kalibr"] < before+5-0.01 {
		t.Fatalf("Калибры не произведены: %.0f → %.0f", before, w.Sides[data.RU].Stocks["kalibr"])
	}
	if !w.Sides[data.RU].Researched["ru_geran2"] {
		t.Fatalf("исследование не завершено: %.0f", w.Sides[data.RU].Progress["ru_geran2"])
	}
	if !w.Sides[data.RU].Unlocked["geran2"] {
		t.Fatal("Герань-2 не открыта")
	}
}

func TestMoveUnit(t *testing.T) {
	w := newTestWorld(t)
	var u *Unit
	for _, x := range w.Units {
		if x.Type == "buk_ua" {
			u = x
			break
		}
	}
	dst := findBuilding(w, "Кременчугский НПЗ")
	if e := w.MoveUnit(data.UA, u.ID, Pt{dst.X + 2, dst.Y + 2}); e != "" {
		t.Fatal(e)
	}
	run(w, 60*24)
	if u.State != UnitDeployed || dist(u.X, u.Y, dst.X+2, dst.Y+2) > 1 {
		t.Fatalf("юнит не дошёл: состояние %d, расстояние %.1f", u.State, dist(u.X, u.Y, dst.X+2, dst.Y+2))
	}
}

func TestSaveLoad(t *testing.T) {
	w := newTestWorld(t)
	w.Solo = true
	run(w, 30)
	p := filepath.Join(t.TempDir(), "save.gob")
	if err := w.Save(p); err != nil {
		t.Fatal(err)
	}
	w2, err := Load(p, w.cat, w.m)
	if err != nil {
		t.Fatal(err)
	}
	if w2.Time != w.Time || len(w2.Buildings) != len(w.Buildings) || len(w2.Units) != len(w.Units) {
		t.Fatal("состояние не совпало")
	}
	if !w2.Solo {
		t.Fatal("флаг одиночной игры потерялся при сохранении")
	}
	run(w2, 10)
}

func TestADOverload(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ru := w.Sides[data.RU]
	src := findBuilding(w, "Севастополь — база ЧФ")
	tgt := findBuilding(w, "Трипольская ТЭС")
	src.Budget = 200
	ru.Stocks["kalibr"] = 200
	if e := w.Strike(data.RU, StrikePlan{Source: src.ID, Munition: "kalibr", Count: 120, Target: Pt{tgt.X, tgt.Y}}); e != "" {
		t.Fatal(e)
	}
	empty := false
	for k := 0; k < 180; k++ {
		w.Step(1)
		for _, u := range w.Units {
			if u.Side == data.UA && w.cat.UnitByID[u.Type].Kind == "ad" && u.Ready < 1 {
				empty = true
			}
		}
	}
	if !empty {
		t.Fatal("ни один комплекс ПВО не расстрелял магазин")
	}
	var res string
	for _, e := range ru.Events {
		if len(e.Text) > 20 && e.Text[:len("Итог удара")] == "Итог удара" {
			res = e.Text
		}
	}
	t.Log(res, "; HP цели:", tgt.HP)
	if tgt.HP >= tgt.MaxHP {
		t.Fatal("массированный удар не прорвал ПВО")
	}
}

func TestSpawnSpread(t *testing.T) {
	for s := 0; s < 2; s++ {
		w := newTestWorld(t)
		pts := map[[2]int]bool{}
		for i := 0; i < 12; i++ {
			x, y := w.spawnPoint(s, "ground", "")
			pts[[2]int{int(x / 20), int(y / 20)}] = true
		}
		ents := map[[2]int]bool{}
		for i := 0; i < 12; i++ {
			x, y := w.spawnPoint(s, "ground", "entry")
			ents[[2]int{int(x / 20), int(y / 20)}] = true
		}
		t.Logf("%s: заводы %d районов, поставки %d районов", data.SideNames[s], len(pts), len(ents))
		if len(pts) < 3 || len(ents) < 3 {
			t.Fatalf("%s: техника появляется в слишком малом числе мест", data.SideNames[s])
		}
	}
}

func TestTatarstanAndDeepStrike(t *testing.T) {
	w := newTestWorld(t)
	for _, n := range []string{"ОЭЗ «Алабуга» (Елабуга, Татарстан)", "ТАНЕКО (Нижнекамск)", "Казанский авиазавод им. Горбунова", "Заинская ГРЭС (Татарстан)", "ТЭЦ-22 (Москва)", "Авиабаза Энгельс"} {
		b := findBuilding(w, n)
		if b == nil || b.Side != data.RU {
			t.Fatalf("%s должен быть на карте у России", n)
		}
	}
	// Дальний удар Украины по Алабуге возможен только Фламинго.
	run(w, w.PrepEnd+1)
	air := findBuilding(w, "Аэродром Миргород")
	alabuga := findBuilding(w, "ОЭЗ «Алабуга» (Елабуга, Татарстан)")
	ua := w.Sides[data.UA]
	ua.Stocks["flamingo"], ua.Stocks["lyutyi"] = 5, 5
	air.Budget = 10
	if e := w.Strike(data.UA, StrikePlan{Source: air.ID, Munition: "lyutyi", Count: 1, Target: Pt{alabuga.X, alabuga.Y}}); e == "" {
		t.Fatal("Лютый (1000 км) не должен долетать до Татарстана")
	}
	if e := w.ValidateStrike(data.UA, StrikePlan{Source: air.ID, Munition: "flamingo", Count: 1, Target: Pt{alabuga.X, alabuga.Y}}); e != "" {
		t.Fatalf("Фламинго должен долетать до Алабуги: %s", e)
	}
	// А до Москвы Лютый долетает с аэродрома под Харьковом/Миргородом.
	msk := findBuilding(w, "ТЭЦ-22 (Москва)")
	if e := w.ValidateStrike(data.UA, StrikePlan{Source: air.ID, Munition: "lyutyi", Count: 1, Target: Pt{msk.X, msk.Y}}); e != "" {
		t.Fatalf("Лютый должен долетать до Москвы: %s", e)
	}
}

func TestPeaceNoBlackoutWholeMap(t *testing.T) {
	w := newTestWorld(t)
	run(w, 300)
	for s := 0; s < 2; s++ {
		for r, f := range w.Sides[s].RegionPower {
			if f < 0.8 && w.Sides[s].Blackout > 0.01 {
				t.Errorf("%s: дефицит энергии в области %s (%.2f)", data.SideNames[s], w.m.Regions[r].Name, f)
			}
		}
	}
}

func TestDamageEfficiency(t *testing.T) {
	w := newTestWorld(t)
	r := w.cat.Rules
	cases := []struct{ h, min, max float64 }{{1, 1, 1}, {0.9, 0.80, 0.90}, {0.5, 0.25, 0.40}, {0.2, 0.001, 0.06}, {0.1, 0, 0}}
	for _, c := range cases {
		e := Efficiency(r, c.h)
		t.Logf("HP %.0f%% → выпуск %.0f%%", c.h*100, e*100)
		if e < c.min-1e-9 || e > c.max+1e-9 {
			t.Fatalf("HP %.2f: выпуск %.3f вне [%.2f, %.2f]", c.h, e, c.min, c.max)
		}
	}
}

func TestRepairTimeAndCrews(t *testing.T) {
	w := newTestWorld(t)
	w.Sides[data.RU].PostureDir = [3]int{} // оборона: фронт не должен менять владельцев объектов
	w.Sides[data.UA].PostureDir = [3]int{}
	run(w, 5)
	ref := findBuilding(w, "Кременчугский НПЗ")
	ref.HP = 0
	// Ещё 9 повреждённых зданий (по порядку номеров) делят бригады.
	var ids []uint32
	for id, b := range w.Buildings {
		if b.Side == data.UA && b != ref && b.Type != "npp" {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	var hit []*Building
	for _, id := range ids[:9] {
		b := w.Buildings[id]
		b.HP = b.MaxHP * 0.3
		hit = append(hit, b)
	}
	run(w, 60)
	working := 0
	for _, b := range w.Buildings {
		if b.Side == data.UA && b.Repairing {
			working++
		}
	}
	if working > w.cat.Rules.RepairCrews || working < 3 {
		t.Fatalf("работает бригад %d, лимит %d", working, w.cat.Rules.RepairCrews)
	}
	run(w, 60*40)
	t.Logf("НПЗ через 41 ч: %.0f%%", ref.HP/ref.MaxHP*100)
	if ref.HP/ref.MaxHP > 0.7 {
		t.Fatal("НПЗ с нуля не должен восстанавливаться за 41 час")
	}
	run(w, 60*300)
	if ref.HP < ref.MaxHP-0.01 {
		t.Fatalf("НПЗ так и не восстановлен за 340 ч: %.0f%%", ref.HP/ref.MaxHP*100)
	}
	for _, b := range hit {
		if b.Side != data.UA {
			continue // за 340 часов войны объект могли захватить
		}
		if b.HP < b.MaxHP-0.01 {
			t.Fatalf("%s не восстановлен: %.0f%%", b.Name, b.HP/b.MaxHP*100)
		}
	}
}

func TestRepairPerTypeLimit(t *testing.T) {
	w := newTestWorld(t)
	run(w, 5)
	n := 0
	for _, b := range w.Buildings {
		if b.Side == data.UA && b.Type == "tpp" {
			b.HP = b.MaxHP * 0.2
			n++
		}
	}
	if n < 4 {
		t.Fatalf("мало ТЭС для проверки: %d", n)
	}
	run(w, 30)
	working := 0
	for _, b := range w.Buildings {
		if b.Side == data.UA && b.Type == "tpp" && b.Repairing {
			working++
		}
	}
	if working != w.cat.Rules.RepairPerType {
		t.Fatalf("ТЭС чинится одновременно %d, лимит на тип %d", working, w.cat.Rules.RepairPerType)
	}
}

func TestStrikeDroneRecon(t *testing.T) {
	seen := func(research bool) int {
		w := newTestWorld(t)
		run(w, w.PrepEnd+1)
		ru := w.Sides[data.RU]
		if research {
			ru.Effects["drone_recon"] = 1
		}
		for id := range ru.Known { // убираем довоенные данные
			delete(ru.Known, id)
		}
		ru.Stocks["geran2"] = 20
		air := findBuilding(w, "Аэродром Миллерово")
		air.Budget = 50
		tgt := findBuilding(w, "Завод им. Малышева (Харьков)")
		if e := w.Strike(data.RU, StrikePlan{Source: air.ID, Munition: "geran2", Count: 10, Target: Pt{tgt.X, tgt.Y}}); e != "" {
			t.Fatal(e)
		}
		run(w, 240)
		n := 0
		for _, c := range ru.Known {
			if c.Source == "Герань-2" {
				n++
			}
		}
		return n
	}
	without, with := seen(false), seen(true)
	t.Logf("замечено дронами: без исследования %d, с исследованием %d", without, with)
	if without != 0 || with == 0 {
		t.Fatal("ударные дроны должны разведывать только после исследования")
	}
}

func TestMoraleEffects(t *testing.T) {
	w := newTestWorld(t)
	r := w.cat.Rules
	if MoraleFront(r, 100) <= MoraleFront(r, 50) || MoraleFront(r, 0) >= 0.7 || MoraleProd(r, 0) > 0.71 || MoraleLosses(r, 0) < 1.4 {
		t.Fatalf("кривые морали неверны: фронт %.2f/%.2f/%.2f", MoraleFront(r, 100), MoraleFront(r, 50), MoraleFront(r, 0))
	}
	// Нулевая мораль сама по себе не заканчивает игру.
	run(w, w.PrepEnd+1)
	w.Sides[data.UA].Morale = 0
	run(w, 120)
	if w.Winner >= 0 {
		t.Fatalf("партия закончилась из-за морали: %s", w.WinReason)
	}
	// Но производство падает.
	w2 := newTestWorld(t)
	run(w2, 120)
	low := newTestWorld(t)
	low.Sides[data.UA].Morale = 0
	low.Sides[data.RU].Morale = 100
	run(low, 120)
	if low.Sides[data.UA].Income >= w2.Sides[data.UA].Income*0.85 {
		t.Fatalf("налоги при морали 0 не упали: %.0f против %.0f", low.Sides[data.UA].Income, w2.Sides[data.UA].Income)
	}
}

func TestPropagandaAndKeyHit(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ua := w.Sides[data.UA]
	ua.Morale = 40
	before, money := ua.Morale, ua.Res[data.ResMoney]
	if e := w.Apply(Command{Kind: CmdPropaganda, Side: data.UA}); e != "" {
		t.Fatal(e)
	}
	if ua.Morale <= before || ua.Res[data.ResMoney] >= money {
		t.Fatal("кампания не подняла мораль или не стоила денег")
	}
	if e := w.Apply(Command{Kind: CmdPropaganda, Side: data.UA}); e == "" {
		t.Fatal("повторная кампания должна быть на перезарядке")
	}
	// Вывод из строя ключевого объекта: мораль атакующего растёт, владельца падает.
	ru := w.Sides[data.RU]
	ru.Morale, ua.Morale = 50, 50
	tpp := findBuilding(w, "Трипольская ТЭС")
	tpp.HP = tpp.MaxHP * 0.12
	w.Strike(data.RU, StrikePlan{Source: findBuilding(w, "Севастополь — база ЧФ").ID, Munition: "kalibr", Count: 1, Target: Pt{tpp.X, tpp.Y}})
	for i := 0; i < 200 && len(w.Projs) > 0; i++ {
		w.Step(1)
		ua.Stocks["m_5v55"], ua.Stocks["m_9m38"] = 0, 0 // ПВО без ракет, чтобы дошло
		for _, u := range w.Units {
			u.Ready = 0
		}
	}
	if tpp.HP > tpp.MaxHP*0.1 {
		t.Skip("ракета не попала (вероятностный тест)")
	}
	if ru.Morale <= 50 || ua.Morale >= 50 {
		t.Fatalf("мораль после поражения ключевого объекта: РФ %.1f, Украина %.1f", ru.Morale, ua.Morale)
	}
}

// МОГ и расчёты дронов-перехватчиков выходят только у центров подготовки.
func TestTrainingCenterSpawn(t *testing.T) {
	w := newTestWorld(t)
	for s := 0; s < 2; s++ {
		var centers int
		for _, b := range w.Buildings {
			if b.Side == s && b.Type == "training_center" {
				centers++
			}
		}
		if centers < 2 {
			t.Fatalf("%s: центров подготовки %d, ожидалось не меньше 2", data.SideNames[s], centers)
		}
		for _, id := range []string{"mfg_ru", "mfg_ua", "idrone_ru", "idrone_ua"} {
			ut := w.cat.UnitByID[id]
			if (ut.Side == "ru") != (s == data.RU) {
				continue
			}
			for i := 0; i < 6; i++ {
				x, y := w.spawnUnit(s, ut, "")
				ok := false
				for _, b := range w.Buildings {
					if b.Side == s && b.Type == "training_center" && b.X == x && b.Y == y {
						ok = true
					}
				}
				if !ok {
					t.Fatalf("%s: %s появился не у центра подготовки", data.SideNames[s], id)
				}
			}
		}
	}
}

// Данные разведки точные: у всех источников координаты равны реальным.
func TestIntelExact(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	var enemy *Unit
	for _, u := range w.Units {
		if u.Side == data.UA && w.cat.UnitByID[u.Type].Emitter() {
			enemy = u
			break
		}
	}
	if enemy == nil {
		t.Fatal("нет излучающего юнита Украины")
	}
	for _, sensor := range []string{"rtr", "flash", "agent", SensorOptical} {
		delete(w.Sides[data.RU].Known, enemy.ID)
		w.observe(data.RU, enemy.ID, sensor, sensor)
		c := w.Sides[data.RU].Known[enemy.ID]
		if c == nil || c.X != enemy.X || c.Y != enemy.Y {
			t.Fatalf("%s: координаты метки не совпали с реальными", sensor)
		}
	}
}

// Старые метки мобильных объектов не живут на захваченной территории.
func TestStaleContactsOnCaptured(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ru := w.Sides[data.RU]
	var enemy *Unit
	for _, u := range w.Units {
		if u.Side == data.UA {
			enemy = u
			break
		}
	}
	w.observe(data.RU, enemy.ID, SensorOptical, "наблюдение")
	if ru.Known[enemy.ID] == nil {
		t.Fatal("метка не создана")
	}
	i := w.tileOf(enemy.X, enemy.Y)
	w.captureTile(i, data.RU)
	if ru.Known[enemy.ID] != nil {
		t.Fatal("метка уничтоженного при захвате юнита осталась")
	}
	// Метка без реального объекта на своей территории исчезает на следующем шаге.
	var x, y float64
	for k, o := range w.Owner {
		if int(o)-1 == data.RU && w.m.Terrain[k] == world.TerrainLand {
			x, y = w.m.TileCenter(k%w.m.W, k/w.m.W)
			break
		}
	}
	ru.Known[4000000] = &Contact{ID: 4000000, Kind: 1, Type: "buk_ua", X: x, Y: y, Seen: w.Time, HP: -1, Source: "РТР"}
	run(w, 1)
	if ru.Known[4000000] != nil {
		t.Fatal("метка на своей территории не удалилась")
	}
	// А метка на реальном здании противника остаётся.
	var bid uint32
	for id, b := range w.Buildings {
		if b.Side == data.UA {
			bid = id
			break
		}
	}
	run(w, 5)
	if ru.Known[bid] == nil {
		t.Fatal("метка на реальном здании противника пропала")
	}
}

func TestMoraleDiminishingReturns(t *testing.T) {
	w := newTestWorld(t)
	sd := w.Sides[data.UA]
	sd.Morale = 50
	g1 := w.moraleGain(data.UA, "key", 2)
	g2 := w.moraleGain(data.UA, "key", 2)
	g3 := w.moraleGain(data.UA, "key", 2)
	if !(g1 > g2 && g2 > g3) {
		t.Fatalf("повторные успехи должны давать всё меньше: %.2f %.2f %.2f", g1, g2, g3)
	}
	sd.Morale = 90
	hi := w.moraleGain(data.UA, "other", 2)
	sd.Morale = 20
	lo := w.moraleGain(data.UA, "another", 2)
	if hi >= lo || hi > 2*w.cat.Rules.MoraleGainFloor+0.5 {
		t.Fatalf("у высокой морали прирост должен быть меньше: %.2f против %.2f", hi, lo)
	}
	// Усталость проходит со временем.
	sd.Morale = 50
	run(w, w.cat.Rules.MoraleFatigueH*60+5)
	if g := w.moraleGain(data.UA, "key", 2); g < 1.99 {
		t.Fatalf("усталость не прошла: %.2f", g)
	}
}

func TestKeyHitMoraleOnce(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	tgt := findBuilding(w, "Ж/д узел Киев")
	if tgt == nil || tgt.Side != data.UA {
		t.Fatal("нет ж/д узла Киев у Украины")
	}
	m := *w.cat.MunitionByID["kalibr"]
	m.Accuracy, m.Damage = 1, 100000
	ru := w.Sides[data.RU]
	ru.Morale = 40
	w.impact(&Projectile{Side: data.RU, X: tgt.X, Y: tgt.Y}, &m)
	first := ru.Morale
	if first <= 40 {
		t.Fatal("вывод ключевого объекта из строя не поднял мораль")
	}
	tgt.HP = tgt.MaxHP // «починили» в обход бригад: флаг остаётся
	w.impact(&Projectile{Side: data.RU, X: tgt.X, Y: tgt.Y}, &m)
	if ru.Morale != first {
		t.Fatalf("повторное поражение того же объекта изменило мораль: %.2f → %.2f", first, ru.Morale)
	}
	// После настоящего ремонта флаг снимается.
	tgt.HP = tgt.MaxHP * 0.2
	tgt.Repair = true
	tgt.Built = 1
	w.Sides[data.UA].Res = data.ToRes(map[string]float64{"money": 1e6, "fuel": 1e6, "steel": 1e6, "electronics": 1e6, "ammo": 1e6})
	run(w, 48*60)
	if tgt.KeyHit {
		t.Fatal("флаг KeyHit не снят после ремонта")
	}
}

// Сталь РФ к Украине — ближе к реальному соотношению; все тыловые объекты встали на карту.
func TestSteelBalanceAndRear(t *testing.T) {
	w := newTestWorld(t)
	run(w, 30)
	var sum [2]float64
	for _, b := range w.Buildings {
		if b.Type == "steel_mill" {
			sum[b.Side] += 12 * b.scale() * w.output(b)
		}
	}
	if sum[data.UA] <= 0 || sum[data.RU]/sum[data.UA] < 2.5 {
		t.Fatalf("сталь РФ/Украина = %.0f/%.0f, нужно не меньше 2.5×", sum[data.RU], sum[data.UA])
	}
	have := map[string]bool{}
	for _, b := range w.Buildings {
		have[b.Name] = true
	}
	for _, o := range w.cat.Objects {
		if !have[o.Name] {
			t.Errorf("объект %q не получил владельца на карте", o.Name)
		}
	}
	hubs := map[int]int{}
	for _, b := range w.Buildings {
		if b.Type == "rail_hub" || b.Type == "rail_station" {
			hubs[b.Side]++
		}
	}
	if hubs[data.UA] < 15 || hubs[data.RU] < 10 {
		t.Fatalf("мало узлов и станций: %v", hubs)
	}
}

func TestMissions(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ua := w.Sides[data.UA]
	hammer := *w.cat.MunitionByID["kalibr"]
	hammer.Accuracy, hammer.Damage, hammer.BlastKm = 1, 100000, 0
	// У Украины есть задания, и в представлении они видны.
	if n := len(w.BuildView(data.UA, 0).Missions); n < 4 {
		t.Fatalf("заданий у Украины %d, ожидалось 4", n)
	}
	if len(w.BuildView(data.RU, 0).Missions) != 0 {
		t.Fatal("у России заданий быть не должно")
	}
	strike := func(b *Building) {
		w.impact(&Projectile{Side: data.UA, X: b.X, Y: b.Y}, &hammer)
	}
	// Крымский мост.
	before := ua.Stocks["atacms"]
	bridge := findBuilding(w, "Крымский мост")
	strike(bridge)
	if !ua.AidDone["mis_crimea_bridge"] || ua.Stocks["atacms"] < before+20 {
		t.Fatal("за Крымский мост награда не выдана")
	}
	// Москва: нужно три разных объекта в радиусе 60 км.
	cx, cy := w.m.Project(37.62, 55.75)
	var moscow []*Building
	for _, b := range w.Buildings {
		if b.Side == data.RU && dist(b.X, b.Y, cx, cy) <= 60 {
			moscow = append(moscow, b)
		}
	}
	sort.Slice(moscow, func(i, j int) bool { return moscow[i].ID < moscow[j].ID })
	if len(moscow) < 3 {
		t.Fatalf("в Москве всего %d объектов", len(moscow))
	}
	strike(moscow[0])
	strike(moscow[0]) // повторное поражение не считается
	strike(moscow[1])
	if ua.AidDone["mis_moscow"] {
		t.Fatal("награда выдана раньше времени")
	}
	strike(moscow[2])
	if !ua.AidDone["mis_moscow"] {
		t.Fatal("за три объекта в Москве награда не выдана")
	}
	// НПЗ: четыре штуки.
	n := 0
	for _, b := range w.Buildings {
		if b.Side == data.RU && b.Type == "refinery" && n < 4 {
			strike(b)
			n++
		}
	}
	if !ua.AidDone["mis_refineries"] {
		t.Fatal("за четыре НПЗ награда не выдана")
	}
	// Удержание Киева.
	run(w, 72*60+5)
	if !ua.AidDone["mis_hold_kyiv"] {
		t.Fatal("за удержание Киева награда не выдана")
	}
}

func TestMissionHoldFailed(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	for i := range w.Owner {
		if i == w.kyiv {
			w.Owner[i] = uint8(data.RU + 1)
		}
	}
	run(w, 72*60+5)
	if !w.Sides[data.UA].MissionFail["mis_hold_kyiv"] || w.Sides[data.UA].AidDone["mis_hold_kyiv"] {
		t.Fatal("задание на удержание потерянного Киева должно быть провалено")
	}
}

// Позиция задаётся отдельно по направлениям; старые сохранения мигрируют.
func TestPosturePerDirection(t *testing.T) {
	w := newTestWorld(t)
	ru := w.Sides[data.RU]
	w.Apply(Command{Kind: CmdPosture, Side: data.RU, Int: PostureOffense, Count: 2}) // Донбасс
	if ru.PostureDir != [3]int{PostureActive, PostureOffense, PostureActive} {
		t.Fatalf("позиции по направлениям: %v", ru.PostureDir)
	}
	w.Apply(Command{Kind: CmdPosture, Side: data.RU, Int: PostureDefense}) // все
	if ru.PostureDir != [3]int{} {
		t.Fatalf("команда для всех направлений: %v", ru.PostureDir)
	}
	if v := w.BuildView(data.RU, 0); v.Posture != ru.PostureDir {
		t.Fatal("позиции не попали в представление")
	}
	// Старое сохранение без PostureDir: берём единую позицию.
	old := &Side{Posture: PostureOffense}
	old.ensure(10)
	if old.PostureDir != [3]int{PostureOffense, PostureOffense, PostureOffense} {
		t.Fatalf("миграция позиции: %v", old.PostureDir)
	}
	// Наступление на одном направлении не включает давление на других.
	w2 := newTestWorld(t)
	run(w2, w2.PrepEnd+1)
	for s := 0; s < 2; s++ {
		w2.Sides[s].PostureDir = [3]int{}
	}
	w2.Sides[data.RU].PostureDir[1] = PostureOffense
	run(w2, 6*60)
	north := 0.0
	for i := range w2.Pressure {
		if w2.TileDir(i) != 1 && w2.Pressure[i] > 0 {
			north += float64(w2.Pressure[i])
		}
	}
	if north != 0 {
		t.Fatalf("давление появилось на направлениях в обороне: %.2f", north)
	}
}

// Истребители с аэродрома перехватывают низкие цели, пока аэродром не под вражеской ПВО.
func TestFighterIntercept(t *testing.T) {
	setup := func() (*World, *Building, *Projectile) {
		w := newTestWorld(t)
		run(w, w.PrepEnd+1)
		af := findBuilding(w, "Аэродром Миргород")
		if af == nil || af.Side != data.UA || af.Aircraft["fighter"] < 3 {
			t.Fatal("нет украинского аэродрома с истребителями")
		}
		w.cat.BuildingByID["airfield"].Intercept.PkLow = 0.98
		// убираем всю ПВО обеих сторон: работают только истребители
		for id, u := range w.Units {
			if w.cat.UnitByID[u.Type].Kind == "ad" {
				delete(w.Units, id)
			}
		}
		p := &Projectile{ID: w.newID(), Munition: "geran2", Side: data.RU, X: af.X + 10, Y: af.Y,
			Path: []Pt{{af.X + 60, af.Y}}, Home: Pt{af.X + 200, af.Y}}
		w.Projs[p.ID] = p
		return w, af, p
	}
	fighterEngs := func(w *World, af *Building) int {
		n := 0
		for _, e := range w.Engs {
			if e.Bld == af.ID {
				n++
			}
		}
		return n
	}

	w, af, p := setup()
	aam := w.Sides[data.UA].Stocks["m_aam_ua"]
	w.airDefense(0.01)
	if fighterEngs(w, af) != 1 || af.Busy != 1 {
		t.Fatalf("перехват не начался: перехватов %d, занято %d", fighterEngs(w, af), af.Busy)
	}
	if w.Sides[data.UA].Stocks["m_aam_ua"] != aam-1 {
		t.Fatal("ракета воздух—воздух не потрачена")
	}
	for i := 0; i < 5 && len(w.Engs) > 0; i++ {
		w.airDefense(1)
	}
	if _, alive := w.Projs[p.ID]; alive {
		t.Fatal("цель не сбита истребителями")
	}
	if af.Busy != 0 {
		t.Fatalf("канал перехвата не освободился: %d", af.Busy)
	}

	// Аэродром в зоне вражеской ПВО не взлетает.
	w, af, _ = setup()
	w.addUnit("s400", data.RU, af.X+5, af.Y)
	w.airDefense(0.01)
	if fighterEngs(w, af) != 0 {
		t.Fatal("аэродром под вражеской ПВО не должен поднимать истребители")
	}

	// Без ракет воздух—воздух не стреляют.
	w, af, _ = setup()
	w.Sides[data.UA].Stocks["m_aam_ua"] = 0
	w.airDefense(0.01)
	if fighterEngs(w, af) != 0 {
		t.Fatal("без ракет перехват невозможен")
	}

	// Баллистику и высокие цели истребители не берут.
	w, af, p = setup()
	p.Munition = "iskander_m"
	w.airDefense(0.01)
	if fighterEngs(w, af) != 0 {
		t.Fatal("истребители не должны перехватывать баллистику")
	}
}

func TestPlacement(t *testing.T) {
	w := newTestWorld(t)
	adBefore := 0
	for _, u := range w.Units {
		if w.cat.UnitByID[u.Type].Kind == "ad" {
			adBefore++
		}
	}
	w.StartPlacement()
	if !w.Placement || !w.Paused() {
		t.Fatal("расстановка не началась или время не остановлено")
	}
	for _, u := range w.Units {
		if w.cat.UnitByID[u.Type].Kind == "ad" {
			t.Fatal("комплексы ПВО должны уйти в резерв")
		}
	}
	ua := w.Sides[data.UA]
	if ua.Reserve["buk_ua"] < 8 || ua.Reserve["dronesite"] != 4 || len(ua.Hints) == 0 {
		t.Fatalf("резерв Украины: %v, подсказок %d", ua.Reserve, len(ua.Hints))
	}
	// Центры подготовки тоже расставляет игрок: из мира они уходят в резерв.
	ru := w.Sides[data.RU]
	if ua.Reserve["training_center"] != 3 || ru.Reserve["training_center"] != 3 {
		t.Fatalf("центры подготовки в резерве: UA %d, RU %d", ua.Reserve["training_center"], ru.Reserve["training_center"])
	}
	for _, bd := range w.Buildings {
		if bd.Type == "training_center" {
			t.Fatal("центр подготовки остался стоять до расстановки")
		}
	}
	// Нельзя поставить на чужую территорию и у фронта.
	var enemyTile, frontish int = -1, -1
	for i, o := range w.Owner {
		if int(o)-1 == data.RU && w.m.Terrain[i] == 1 && enemyTile < 0 {
			enemyTile = i
		}
	}
	ex, ey := w.m.TileCenter(enemyTile%w.m.W, enemyTile/w.m.W)
	if e := w.Apply(Command{Kind: CmdPlace, Side: data.UA, Item: "buk_ua", X: ex, Y: ey}); e == "" {
		t.Fatal("поставили на чужую территорию")
	}
	fi := w.frontT[data.UA][0]
	frontish = fi
	fx, fy := w.m.TileCenter(frontish%w.m.W, frontish/w.m.W)
	if e := w.Apply(Command{Kind: CmdPlace, Side: data.UA, Item: "buk_ua", X: fx, Y: fy}); e == "" {
		t.Fatal("поставили у самого фронта")
	}
	// Своя территория далеко от фронта: Киев.
	kx, ky := w.m.Project(30.5, 50.4)
	stock := ua.Stocks["m_9m38"]
	if e := w.Apply(Command{Kind: CmdPlace, Side: data.UA, Item: "buk_ua", X: kx, Y: ky}); e != "" {
		t.Fatalf("не удалось поставить: %s", e)
	}
	if ua.Reserve["buk_ua"] < 7 || ua.Stocks["m_9m38"] >= stock {
		t.Fatal("резерв не уменьшился или ракеты не взяты на пусковые")
	}
	var placed uint32
	for id, u := range w.Units {
		if u.Placed && u.Type == "buk_ua" {
			placed = id
		}
	}
	if placed == 0 {
		t.Fatal("юнит не создан")
	}
	if e := w.Apply(Command{Kind: CmdUnplace, Side: data.UA, ID: placed}); e != "" {
		t.Fatal(e)
	}
	if ua.Stocks["m_9m38"] != stock || ua.Reserve["buk_ua"] < 8 {
		t.Fatal("после снятия резерв и ракеты должны вернуться")
	}
	// «Готово» только при пустом резерве.
	if e := w.Apply(Command{Kind: CmdReady, Side: data.UA, Int: 1}); e == "" {
		t.Fatal("готовность принята при непустом резерве")
	}
	// Остальное расставляем одинаково: по подсказкам, иначе у Киева.
	for s, sd := range w.Sides {
		for typ, n := range sd.Reserve {
			for ; n > 0; n-- {
				x, y := w.m.Project(30.5, 50.4)
				if s == data.RU {
					x, y = w.m.Project(39.1, 51.6)
				}
				for k := 0; k < 200; k++ {
					if w.Apply(Command{Kind: CmdPlace, Side: s, Item: typ, X: x + float64(k%20)*3, Y: y + float64(k/20)*3}) == "" {
						break
					}
				}
			}
		}
	}
	_ = ru
	if w.PlacementLeft(data.UA) != 0 || w.PlacementLeft(data.RU) != 0 {
		t.Fatalf("осталось: UA %d RU %d", w.PlacementLeft(data.UA), w.PlacementLeft(data.RU))
	}
	w.Apply(Command{Kind: CmdReady, Side: data.UA, Int: 1})
	if !w.Placement {
		t.Fatal("расстановка закончилась, хотя РФ не готова")
	}
	w.Apply(Command{Kind: CmdReady, Side: data.RU, Int: 1})
	if w.Placement || w.Paused() {
		t.Fatal("после готовности обеих сторон игра должна идти")
	}
	// Повторно расстановка не начинается.
	w.StartPlacement()
	if w.Placement {
		t.Fatal("расстановка не должна начинаться повторно")
	}
	_ = adBefore
}

func TestSocialConfirmation(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	r := w.cat.Rules
	nearCity := func(b *Building) bool {
		for _, c := range w.m.Cities {
			if c.Pop >= r.SocialMinPop && dist(c.X, c.Y, b.X, b.Y) <= r.SocialCityKm {
				return true
			}
		}
		return false
	}
	var ids []uint32
	for id, b := range w.Buildings {
		if b.Side == data.UA && !w.cat.BuildingByID[b.Type].Untargetable {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var urban, rural []*Building
	for _, id := range ids {
		b := w.Buildings[id]
		if nearCity(b) {
			urban = append(urban, b)
		} else {
			rural = append(rural, b)
		}
	}
	if len(urban) < 2 || len(rural) < 1 {
		t.Fatalf("городских объектов %d, сельских %d", len(urban), len(rural))
	}
	hit := func(b *Building) {
		m := *w.cat.MunitionByID["kalibr"]
		m.Accuracy, m.Damage, m.BlastKm = 1, b.MaxHP*0.3, 0
		w.impact(&Projectile{Side: data.RU, X: b.X, Y: b.Y}, &m)
	}
	ru := w.Sides[data.RU]
	hit(rural[0])
	if len(ru.Posts) != 0 {
		t.Fatal("удар вне города не должен давать подтверждения в соцсетях")
	}
	tgt := urban[0]
	delete(ru.Known, tgt.ID)
	hit(tgt)
	if len(ru.Posts) != 1 {
		t.Fatalf("подтверждение не запланировано: %d", len(ru.Posts))
	}
	hit(tgt) // повторный удар до выхода подтверждения не плодит записи
	if len(ru.Posts) != 1 {
		t.Fatalf("дубль подтверждения: %d", len(ru.Posts))
	}
	if c := ru.Known[tgt.ID]; c != nil && c.Source == "соцсети" {
		t.Fatal("подтверждение пришло мгновенно")
	}
	run(w, r.SocialDelayMax+5)
	c := ru.Known[tgt.ID]
	if c == nil || c.Source != "соцсети" || c.Type != tgt.Type || c.HP < 0 {
		t.Fatalf("метка после подтверждения: %+v", c)
	}
	found := false
	for _, e := range ru.Events {
		if strings.HasPrefix(e.Text, "Соцсети:") {
			found = true
		}
	}
	if !found || len(ru.Posts) != 0 {
		t.Fatal("событие журнала не появилось")
	}
	// Исследование против утечек гасит подтверждения.
	w.Sides[data.UA].Effects["leak_block"] = 1
	hit(urban[1])
	if len(ru.Posts) != 0 {
		t.Fatal("при полной блокировке утечек подтверждение не должно появиться")
	}
}

func TestTechLines(t *testing.T) {
	w := newTestWorld(t)
	ru, ua := w.Sides[data.RU], w.Sides[data.UA]
	// Линейка Герани: четыре ступени, каждая дороже и требует предыдущую.
	steps := w.cat.LineSteps[data.RU]["geran"]
	if len(steps) != 4 {
		t.Fatalf("в линейке Герани %d ступеней, нужно 4", len(steps))
	}
	if w.TechAvailable(data.RU, "ru_geran3") {
		t.Fatal("Герань-3 не должна быть доступна без Герани-2")
	}
	w.completeTech(data.RU, w.cat.TechByID[data.RU]["ru_geran2"])
	ru.Stocks["geran2"] = 100
	if !w.TechAvailable(data.RU, "ru_geran3") {
		t.Fatal("Герань-3 должна открыться после Герани-2")
	}
	w.completeTech(data.RU, w.cat.TechByID[data.RU]["ru_geran3"])
	w.completeTech(data.RU, w.cat.TechByID[data.RU]["ru_geran4"])
	// Старые версии остаются, запас старой версии не меняется.
	if !ru.Unlocked["geran2"] || !ru.Unlocked["geran3"] || !ru.Unlocked["geran4"] || ru.Unlocked["geran5"] {
		t.Fatalf("версии открыты неверно: %v", ru.Unlocked)
	}
	if ru.Stocks["geran2"] != 100 || ru.Stocks["geran4"] != 0 {
		t.Fatal("запас не должен переоснащаться")
	}
	if w.Apply(Command{Kind: CmdOrderAdd, Side: data.RU, Item: "geran2", Count: 5}) != "" ||
		w.Apply(Command{Kind: CmdOrderAdd, Side: data.RU, Item: "geran4", Count: 5}) != "" {
		t.Fatal("заказывать можно и старую, и новую версию")
	}
	g2, g4 := w.cat.MunitionByID["geran2"], w.cat.MunitionByID["geran4"]
	if g4.Damage <= g2.Damage || g4.SpeedKmh <= g2.SpeedKmh {
		t.Fatal("новая версия должна быть сильнее")
	}
	// ПВО-юнит новой версии появляется отдельным типом, старые юниты остаются.
	w.completeTech(data.RU, w.cat.TechByID[data.RU]["ru_s350"])
	if !ru.Unlocked["s350"] || !ru.Unlocked["m_9m100"] {
		t.Fatal("С-350 и его ракеты должны открыться")
	}
	if ua.Unlocked["s350"] {
		t.Fatal("чужая версия не должна открываться")
	}
}

func TestTechLinesDataValid(t *testing.T) {
	cat, err := data.Load("")
	if err != nil {
		t.Fatal(err)
	}
	for s := 0; s < 2; s++ {
		used := map[string]bool{}
		for _, l := range cat.Lines[data.SideKeys[s]] {
			for _, st := range cat.LineSteps[s][l.ID] {
				used[st.ID] = true
				if st.Cost <= 0 || len(st.Unlocks) == 0 {
					t.Errorf("%s: ступень без цены или без предметов", st.ID)
				}
			}
		}
		if len(cat.Tech[data.SideKeys[s]]) < 15 {
			t.Errorf("%s: слишком мало исследований", data.SideNames[s])
		}
		for _, tc := range cat.Tech[data.SideKeys[s]] {
			if tc.Line == "" && len(tc.Effects) == 0 && len(tc.Unlocks) == 0 {
				// «Возможность» без линейки должна на что-то влиять: эффект или импорт.
				imp := false
				for _, im := range cat.Sides[s].Imports {
					imp = imp || im.Requires == tc.ID
				}
				if !imp {
					t.Errorf("%s: исследование ничего не даёт", tc.ID)
				}
			}
		}
	}
}

func TestFPVVersions(t *testing.T) {
	w := newTestWorld(t)
	sd := w.Sides[data.UA]
	sd.Alloc = [3]float64{1, 0, 0}
	f := &sd.Front[0]
	f.FPV, f.FPVPow = 0, 0
	p0 := w.dirPower(data.UA, 0)
	w.deliver(data.UA, "fpv", 1, "")
	p1 := w.dirPower(data.UA, 0)
	pw := f.FPVPow
	w.deliver(data.UA, "fpv_ai", 1, "")
	if f.FPV != 400 || f.FPVPow <= pw*2.5 {
		t.Fatalf("версии FPV учитываются неверно: %.0f шт, сила %.0f", f.FPV, f.FPVPow)
	}
	if p2 := w.dirPower(data.UA, 0); p2-p1 <= 1.5*(p1-p0) {
		t.Fatalf("FPV с ИИ должны усиливать сильнее обычных: +%.3f против +%.3f", p2-p1, p1-p0)
	}
	// Старое сохранение: без FPVPow все дроны считаются базовой версии.
	f.FPVPow = 0
	sd.ensure(len(w.Owner))
	if f.FPVPow != f.FPV {
		t.Fatal("миграция FPVPow не сработала")
	}
}

func TestSatelliteByTech(t *testing.T) {
	w := newTestWorld(t)
	count := func() int {
		n := 0
		for _, s := range w.BuildView(data.RU, 0).Sats {
			if s.Name == "Кондор-ФКА №2" {
				n++
			}
		}
		return n
	}
	if count() != 0 {
		t.Fatal("Кондор-ФКА №2 не должен летать до исследования")
	}
	w.completeTech(data.RU, w.cat.TechByID[data.RU]["ru_sat2"])
	if count() != 1 {
		t.Fatal("Кондор-ФКА №2 должен появиться после исследования")
	}
}

func TestCheatSandbox(t *testing.T) {
	w := newTestWorld(t)
	w.EnableCheat()
	ru := w.Sides[data.RU]
	if !ru.Unlocked["geran5"] || !ru.Unlocked["oreshnik"] || !ru.Researched["ru_s350"] {
		t.Fatal("в режиме «всё открыто» должно быть изучено всё")
	}
	if w.Sides[data.UA].Unlocked["geran5"] {
		t.Fatal("чужие предметы не должны открываться")
	}
	if e := w.Apply(Command{Kind: CmdOrderAdd, Side: data.RU, Item: "geran5", Count: 20}); e != "" {
		t.Fatal(e)
	}
	ru.Res = data.Res{} // без ресурсов: бесплатное производство всё равно идёт
	run(w, 2)
	if ru.Stocks["geran5"] < 20 {
		t.Fatalf("производство должно быть мгновенным: %.0f", ru.Stocks["geran5"])
	}
}

func TestMigrateOldTech(t *testing.T) {
	w := newTestWorld(t)
	ru := w.Sides[data.RU]
	ru.Researched = map[string]bool{"ru_fpv": true, "ru_fiber": true, "ru_lancet": true, "ru_leaks": true}
	ru.Progress = map[string]float64{"ru_kab": 50, "ru_geran2": 30}
	ru.Research = "ru_kab"
	ru.Effects = map[string]float64{"fpv_power": 0.5}
	ru.migrateTech(w.cat, data.RU)
	if !ru.Researched["ru_fpv_fiber"] || ru.Researched["ru_fiber"] || ru.Researched["ru_lancet"] {
		t.Fatalf("исследования перенесены неверно: %v", ru.Researched)
	}
	if ru.Research != "" || ru.Progress["ru_kab"] != 0 || ru.Progress["ru_geran2"] != 30 {
		t.Fatal("прогресс и текущее исследование мигрированы неверно")
	}
	if ru.Effects["fpv_power"] != 0 || ru.Effects["leak_block"] == 0 || !ru.Unlocked["fpv_fiber"] {
		t.Fatalf("эффекты или предметы пересчитаны неверно: %v", ru.Effects)
	}
}

func TestEnemyPowerEstimate(t *testing.T) {
	w := newTestWorld(t)
	w.updatePower()
	// Без разведданных о станциях оценка строится только по городам: генерации нет.
	for s := 0; s < 2; s++ {
		w.Sides[s].Known = map[uint32]*Contact{}
	}
	regs, gen, _ := w.estimateEnemyPower(data.RU)
	if gen != 0 {
		t.Fatalf("без меток станций генерация должна быть 0, а не %.0f", gen)
	}
	// Известные метки станций добавляют генерацию в их области.
	n := 0
	for id, b := range w.Buildings {
		if b.Side != data.UA || w.cat.BuildingByID[b.Type].Power <= 0 {
			continue
		}
		w.Sides[data.RU].Known[id] = &Contact{ID: id, Kind: 0, Type: b.Type, X: b.X, Y: b.Y, Seen: 10, HP: 1}
		n++
	}
	if n == 0 {
		t.Skip("нет станций у Украины")
	}
	regs2, gen2, _ := w.estimateEnemyPower(data.RU)
	if gen2 <= 0 || len(regs2) < len(regs) {
		t.Fatalf("известные станции должны дать генерацию: %.0f", gen2)
	}
	v := w.BuildView(data.RU, 0)
	if v.EnemyGen != gen2 {
		t.Fatal("оценка должна попасть в представление")
	}
	// Противник не видит оценку по неизвестному: у стороны без меток своих станций нет.
	if _, g3, _ := w.estimateEnemyPower(data.UA); g3 != 0 {
		t.Fatalf("у Украины нет меток российских станций: %.0f", g3)
	}
}

type nopCloser struct{ *bytes.Buffer }

func (nopCloser) Close() error { return nil }

func TestRecorder(t *testing.T) {
	w := newTestWorld(t)
	buf := &bytes.Buffer{}
	w.SetRecorder(nopCloser{buf}, "test", map[string]any{"x": 1})
	w.Apply(Command{Kind: CmdOrderAdd, Side: data.RU, Item: "kalibr", Count: 3})
	w.Apply(Command{Kind: CmdOrderAdd, Side: data.UA, Item: "geran5"}) // отказ: не изучено
	run(w, 130)
	w.CloseRecorder()
	kinds := map[string]int{}
	sides := map[int]bool{}
	var failed bool
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec struct {
			K string
			T float64
			D map[string]any
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("строка журнала не JSON: %v: %s", err, line)
		}
		kinds[rec.K]++
		if rec.K == "cmd" {
			sides[int(rec.D["side"].(float64))] = true
			failed = failed || rec.D["err"] != nil
		}
	}
	if kinds["meta"] != 1 || kinds["cmd"] != 2 || kinds["snap"] < 2 {
		t.Fatalf("неполный журнал: %v", kinds)
	}
	if !sides[0] || !sides[1] || !failed {
		t.Fatal("в журнале должны быть приказы обеих сторон и отказы")
	}
}

func TestMercenaries(t *testing.T) {
	w := newTestWorld(t)
	ua := w.Sides[data.UA]
	ua.Res[data.ResMoney] = 1000
	people, morale, labor := ua.People, ua.Morale, ua.LaborLoss
	front := ua.Front[0].Men + ua.Front[1].Men + ua.Front[2].Men
	if e := w.Apply(Command{Kind: CmdMobilize, Side: data.UA, Item: "ua_latam"}); e != "" {
		t.Fatal(e)
	}
	got := ua.Front[0].Men + ua.Front[1].Men + ua.Front[2].Men - front
	if got < 11.99 || got > 12.01 {
		t.Fatalf("наёмники должны дать 12 тыс. на фронт, а не %.2f", got)
	}
	if ua.People != people || ua.Morale != morale || ua.LaborLoss != labor {
		t.Fatal("наёмники не должны затрагивать резерв, мораль и выпуск")
	}
	if ua.Res[data.ResMoney] != 750 {
		t.Fatalf("наёмники стоят 250: осталось %.0f", ua.Res[data.ResMoney])
	}
	if e := w.Apply(Command{Kind: CmdMobilize, Side: data.UA, Item: "ua_latam"}); e == "" {
		t.Fatal("повторный найм сразу должен упираться в перерыв")
	}
	if w.Apply(Command{Kind: CmdMobilize, Side: data.RU, Item: "ua_latam"}) == "" {
		t.Fatal("у России такого варианта нет")
	}
	// Резерв исчерпан — наёмники всё равно доступны.
	ua.People = 0
	ua.MobReady["ua_latam"] = 0
	if e := w.Apply(Command{Kind: CmdMobilize, Side: data.UA, Item: "ua_latam"}); e != "" {
		t.Fatal(e)
	}
}

// Победа Украины: тайлы суши, окружённые морем (косы в сетке карты), фронт захватить не может —
// они не должны мешать освобождению всей страны (партия, где Россия держала 5 таких тайлов в Крыму, не заканчивалась).
func TestUkraineVictoryIgnoresUnreachableTiles(t *testing.T) {
	w := newTestWorld(t)
	w.Time = w.PrepEnd + 10
	var isolated, reachable int = -1, -1
	for i, c := range w.m.Country {
		if c != 1 || w.m.Terrain[i] != 1 {
			continue
		}
		w.Owner[i] = 2
		if w.attackableByLand(i) {
			reachable = i
		} else {
			isolated = i
		}
	}
	if isolated < 0 || reachable < 0 {
		t.Skip("на карте нет изолированных тайлов для проверки")
	}
	w.Owner[isolated] = 1
	w.checkVictory()
	if w.Winner != data.UA {
		t.Fatalf("Россия держит только недостижимый тайл — должна быть победа Украины, а победитель %d (%s)", w.Winner, w.WinReason)
	}
	w.Winner, w.WinReason = -1, ""
	w.Owner[reachable] = 1
	w.checkVictory()
	if w.Winner != -1 {
		t.Fatalf("Россия держит достижимый тайл — победы быть не должно, а победитель %d", w.Winner)
	}
}

func TestSanctions(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ru := w.Sides[data.RU]
	var imp data.ImportOffer
	for _, im := range w.cat.Sides[data.RU].Imports {
		if im.ID == "ru_imp_shahed" {
			imp = im
		}
	}
	price0 := w.ImportPrice(data.RU, imp)
	if len(ru.SanctionOn) != 0 || len(w.BuildView(data.UA, 0).Sanctions) != 0 {
		t.Fatal("до таймера санкций быть не должно; против Украины санкций нет")
	}
	// По таймеру: SWIFT (3 ч) и резервы (6 ч).
	run(w, 6*60+5)
	if !ru.SanctionOn["ru_s_swift"] || !ru.SanctionOn["ru_s_reserves"] || ru.SanctionOn["ru_s_chips"] {
		t.Fatalf("санкции по таймеру: %v", ru.SanctionOn)
	}
	if got := w.Sanction(data.RU, "tax"); math.Abs(got-0.13) > 1e-9 {
		t.Fatalf("штраф налогов %.3f, ожидалось 0.13", got)
	}
	if p := w.ImportPrice(data.RU, imp); math.Abs(p-price0*1.10) > 1e-6 {
		t.Fatalf("цена импорта %.2f, ожидалось %.2f", p, price0*1.10)
	}
	v := w.BuildView(data.RU, 0)
	if len(v.Sanctions) < 10 || v.SanctionTotal["tax"] == 0 {
		t.Fatal("санкции не видны в представлении России")
	}
	// В ответ на удары: четыре ТЭС Украины.
	hammer := *w.cat.MunitionByID["kalibr"]
	hammer.Accuracy, hammer.Damage, hammer.BlastKm = 1, 100000, 0
	n := 0
	for _, b := range w.Buildings {
		if b.Side == data.UA && b.Type == "tpp" && n < 4 {
			if ru.SanctionOn["ru_s_energy"] {
				t.Fatal("санкции введены раньше четвёртой ТЭС")
			}
			w.impact(&Projectile{Side: data.RU, X: b.X, Y: b.Y}, &hammer)
			n++
		}
	}
	if !ru.SanctionOn["ru_s_energy"] {
		t.Fatal("удары по четырём ТЭС не вызвали санкций")
	}
	// Взятие Харькова.
	for i, ci := range w.cityAt {
		if w.m.Cities[ci].Name == "Харьков" {
			w.Owner[i] = uint8(data.RU + 1)
		}
	}
	w.sanctionsTick(data.RU)
	if !ru.SanctionOn["ru_s_kharkiv"] {
		t.Fatal("взятие Харькова не вызвало санкций")
	}
	// Штраф не превышает предела.
	for _, p := range w.cat.Sides[data.RU].Sanctions {
		ru.SanctionOn[p.ID] = true
	}
	for k := range data.SanctionKeys {
		if w.Sanction(data.RU, k) > sanctionCap+1e-9 {
			t.Fatalf("штраф %s выше предела", k)
		}
	}
}

// Северные объекты (Петербург и область) стоят на суше России.
func TestNorthObjects(t *testing.T) {
	w := newTestWorld(t)
	n := 0
	for _, b := range w.Buildings {
		_, lat := w.m.Unproject(b.X, b.Y)
		if lat < 57 {
			continue
		}
		n++
		if b.Side != data.RU {
			t.Fatalf("%s на севере должен принадлежать России", b.Name)
		}
	}
	want := 0
	for _, o := range w.cat.Objects {
		if o.Lat >= 57 {
			want++
		}
	}
	if want < 20 || n != want {
		t.Fatalf("на севере объектов %d из %d (потерялись при привязке к суше)", n, want)
	}
	if findBuilding(w, "КИНЕФ (Кириши)") == nil || findBuilding(w, "Ленинградская АЭС") == nil {
		t.Fatal("нет КИНЕФ или Ленинградской АЭС")
	}
}

// Небо соседних стран закрыто, пока не открыто пакетом; для России — всегда.
func TestAirspaceOpening(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ua := w.Sides[data.UA]
	// Точка над сушей страны c (первый подходящий тайл).
	over := func(c uint8) Pt {
		for i, cc := range w.m.Country {
			if cc == c && w.m.Terrain[i] == world.TerrainLand {
				x, y := w.m.TileCenter(i%w.m.W, i/w.m.W)
				return Pt{X: x, Y: y}
			}
		}
		t.Fatalf("на карте нет страны %d", c)
		return Pt{}
	}
	pl, lt, lv, ee, fi, ro := over(world.CountryPoland), over(world.CountryLithuania), over(world.CountryLatvia),
		over(world.CountryEstonia), over(world.CountryFinland), over(world.CountryForeign)
	open := func(s int, p Pt) bool { return w.airspaceOK(s, p, p) }
	if open(data.UA, pl) || open(data.UA, lt) || open(data.UA, lv) || open(data.UA, ee) || open(data.UA, fi) {
		t.Fatal("до открытия небо Европы должно быть закрыто")
	}
	if c := w.airspaceBlock(data.UA, pl, pl); c != world.CountryPoland {
		t.Fatalf("закрыла страна %d, ожидалась Польша", c)
	}
	run(w, 37*60)
	if !ua.AirOpen["air_pl"] || ua.AirOpen["air_baltic"] || !open(data.UA, pl) || open(data.UA, lt) {
		t.Fatalf("после 37 ч открыта только Польша: %v", ua.AirOpen)
	}
	run(w, 36*60)
	if !open(data.UA, lt) || !open(data.UA, lv) || open(data.UA, ee) {
		t.Fatal("после 73 ч открыты Литва и Латвия, Эстония ещё нет")
	}
	run(w, 60*60)
	if !open(data.UA, ee) || !open(data.UA, fi) {
		t.Fatalf("после 133 ч открыты Эстония и Финляндия: %v", ua.AirOpen)
	}
	// Прочие страны закрыты всегда; Россия не получает ничего.
	if open(data.UA, ro) || open(data.RU, pl) || open(data.RU, ro) {
		t.Fatal("небо прочих стран закрыто, для России — всё закрыто")
	}
	v := w.BuildView(data.UA, 0)
	if !v.AirOpen[world.CountryPoland] || len(v.Airspace) != 4 {
		t.Fatalf("представление неба: %v, пакетов %d", v.AirOpen, len(v.Airspace))
	}
	if AirspaceBlock(w.m, false, v.AirOpen, pl, lt) != 0 && AirspaceBlock(w.m, false, v.AirOpen, pl, pl) != 0 {
		t.Fatal("клиентская проверка маршрута расходится с хостом")
	}
	if len(w.BuildView(data.RU, 0).Airspace) != 0 {
		t.Fatal("у России пакетов неба быть не должно")
	}
}

// В одиночной игре за Украину скорость задаёт человек (раньше учитывалась сторона 0 и кнопки не работали).
func TestSoloSpeedHumanSide(t *testing.T) {
	for human := 0; human < 2; human++ {
		w := newTestWorld(t)
		w.Sandbox, w.Solo, w.Human = true, true, human
		w.Apply(Command{Kind: CmdSpeed, Side: human, Int: 4})
		if got := w.EffectiveSpeed(); got != 4 {
			t.Fatalf("человек за сторону %d: скорость %d, ожидалась 4", human, got)
		}
	}
}

// Ж/д: между станциями и узлами юнит едет эшелоном и добирается быстрее, чем по дорогам.
func TestRailTransport(t *testing.T) {
	w := newTestWorld(t)
	side := data.UA
	// Две дальние друг от друга работающие станции/узлы Украины.
	var hubs []*Building
	for _, b := range w.Buildings {
		if b.Side == side && (b.Type == "rail_hub" || b.Type == "rail_station") && b.Operational() {
			hubs = append(hubs, b)
		}
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].ID < hubs[j].ID })
	var a, b *Building
	best := 0.0
	for i := range hubs {
		for j := i + 1; j < len(hubs); j++ {
			if d := dist(hubs[i].X, hubs[i].Y, hubs[j].X, hubs[j].Y); d > best {
				best, a, b = d, hubs[i], hubs[j]
			}
		}
	}
	if a == nil || best < 300 {
		t.Fatalf("не нашлись две дальние станции Украины (%.0f км)", best)
	}
	ut := w.cat.UnitByID["buk_ua"]
	from, to := Pt{a.X, a.Y}, Pt{b.X, b.Y}
	// Конец пути — на своей суше рядом со станцией.
	pathR, railR, waitR := w.findPath(side, from, to, ut.SpeedRoad, ut.SpeedOff, true)
	pathF, _, _ := w.findPath(side, from, to, ut.SpeedRoad, ut.SpeedOff, false)
	if pathR == nil || pathF == nil {
		t.Fatal("нет пути между станциями")
	}
	u := &Unit{Type: "buk_ua", Side: side, X: from.X, Y: from.Y, Path: pathR, PathRail: railR, PathWait: waitR}
	uf := &Unit{Type: "buk_ua", Side: side, X: from.X, Y: from.Y, Path: pathF}
	etaR := PathETA(w.m, w.cat.Rules, u, ut)
	etaF := PathETA(w.m, w.cat.Rules, uf, ut)
	nrail := 0
	for _, r := range railR {
		if r {
			nrail++
		}
	}
	if nrail < 10 {
		t.Fatalf("поезд почти не использован: %d участков", nrail)
	}
	if etaR > etaF*0.7 {
		t.Fatalf("с поездом %.0f мин, без — %.0f мин: выигрыш мал", etaR, etaF)
	}
	t.Logf("%.0f км: по дорогам %.0f мин, с эшелоном %.0f мин (%d участков по рельсам)", best, etaF, etaR, nrail)
	// Разрушенные станции и узлы рвут путь: рельсы не используются.
	for _, h := range hubs {
		h.HP = 0
	}
	pathD, railD, _ := w.findPath(side, from, to, ut.SpeedRoad, ut.SpeedOff, true)
	if pathD == nil {
		t.Skip("без станций пути нет вообще")
	}
	for _, r := range railD {
		if r {
			t.Fatal("поезд поехал при разрушенных станциях")
		}
	}
	// Чужая сторона не использует рельсы Украины.
	_, railE, _ := w.findPath(data.RU, from, to, ut.SpeedRoad, ut.SpeedOff, true)
	for _, r := range railE {
		if r {
			t.Fatal("противник едет по нашим рельсам")
		}
	}
}

// Эшелон останавливается, когда станцию на пути разбили, и дальше идёт по дорогам.
func TestRailCutDuringTrip(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	side := data.UA
	var hubs []*Building
	for _, b := range w.Buildings {
		if b.Side == side && (b.Type == "rail_hub" || b.Type == "rail_station") {
			hubs = append(hubs, b)
		}
	}
	sort.Slice(hubs, func(i, j int) bool { return hubs[i].ID < hubs[j].ID })
	var a, b *Building
	best := 0.0
	for i := range hubs {
		for j := i + 1; j < len(hubs); j++ {
			if d := dist(hubs[i].X, hubs[i].Y, hubs[j].X, hubs[j].Y); d > best {
				best, a, b = d, hubs[i], hubs[j]
			}
		}
	}
	u := w.addUnit("buk_ua", side, a.X, a.Y)
	u.State = UnitDeployed
	if e := w.MoveUnit(side, u.ID, Pt{b.X, b.Y}); e != "" {
		t.Fatal(e)
	}
	railLegs := 0
	for _, r := range u.PathRail {
		if r {
			railLegs++
		}
	}
	if railLegs == 0 {
		t.Skip("маршрут без рельсов")
	}
	run(w, 120) // свернулся, погрузился, поехал
	for _, h := range hubs {
		if h != a && h != b {
			h.HP = 0
		}
	}
	run(w, 24*60)
	for _, r := range u.PathRail {
		if r {
			t.Fatal("после разрушения станций осталась рельсовая часть пути")
		}
	}
	if len(u.Path) == 0 && dist(u.X, u.Y, b.X, b.Y) > 20 {
		t.Fatalf("юнит остановился в %.0f км от цели", dist(u.X, u.Y, b.X, b.Y))
	}
}

// Закупка пачкой, автозакупка, продажа излишков, предупреждение о ЗУР и автозаказ, замена ПВО.
func TestStocksAndImports(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	ru := w.Sides[data.RU]
	ru.Res[data.ResMoney] = 1000
	ru.Researched["ru_parallel_import"] = true
	before := len(ru.Deliveries)
	if e := w.Apply(Command{Kind: CmdImport, Side: data.RU, Item: "ru_imp_elec", Count: 5}); e != "" {
		t.Fatal(e)
	}
	price := w.ImportPrice(data.RU, data.ImportOffer{Money: 80})
	if len(ru.Deliveries) != before+5 || math.Abs(ru.Res[data.ResMoney]-(1000-5*price)) > 0.01 {
		t.Fatalf("пачка: %d поставок, денег %.0f", len(ru.Deliveries)-before, ru.Res[data.ResMoney])
	}
	// Денег хватает на меньше, чем просили: покупается сколько можно.
	ru.Res[data.ResMoney] = price*2 + 1
	n := len(ru.Deliveries)
	w.Apply(Command{Kind: CmdImport, Side: data.RU, Item: "ru_imp_elec", Count: 10})
	if len(ru.Deliveries) != n+2 {
		t.Fatalf("пачка на остаток денег: %d вместо 2", len(ru.Deliveries)-n)
	}
	// Автозакупка: электроника ниже порога, деньги выше резерва.
	ru.Deliveries = nil
	ru.Res[data.ResElectronics] = 10
	ru.Res[data.ResMoney] = 5000
	if e := w.Apply(Command{Kind: CmdAutoImport, Side: data.RU, Item: "ru_imp_elec", Int: 1}); e != "" {
		t.Fatal(e)
	}
	w.autoImportTick(data.RU)
	if len(ru.Deliveries) != 1 {
		t.Fatalf("автозакупка не сработала: %d поставок", len(ru.Deliveries))
	}
	w.autoImportTick(data.RU) // партия уже в пути
	if len(ru.Deliveries) != 1 {
		t.Fatal("автозакупка заказывает второй раз, пока первая в пути")
	}
	// Продажа излишков.
	ru.Res[data.ResFuel] = 5000
	m0 := ru.Res[data.ResMoney]
	if e := w.Apply(Command{Kind: CmdSell, Side: data.RU, Item: "res:fuel", Count: 0}); e != "" {
		t.Fatal(e)
	}
	if math.Abs(ru.Res[data.ResFuel]-w.cat.Rules.SellKeepMin) > 1 || ru.Res[data.ResMoney] <= m0 {
		t.Fatalf("после продажи топлива %.0f, деньги %.0f→%.0f", ru.Res[data.ResFuel], m0, ru.Res[data.ResMoney])
	}
	if e := w.Apply(Command{Kind: CmdSell, Side: data.RU, Item: "res:ammo"}); e == "" {
		t.Fatal("боеприпасы продавать нельзя")
	}
	// ЗУР: предупреждение один раз, потом кончились, автозаказ.
	ru.Unlocked["m_48n6"] = true
	logCount := func(sub string) int {
		c := 0
		for _, e := range w.BuildView(data.RU, 0).Events {
			if strings.Contains(e.Text, sub) {
				c++
			}
		}
		return c
	}
	ru.Stocks["m_48n6"] = 10
	w.interceptorStocks(data.RU)
	w.interceptorStocks(data.RU)
	if logCount("Заканчиваются ракеты") != 1 {
		t.Fatalf("предупреждение о малом запасе пришло %d раз", logCount("Заканчиваются ракеты"))
	}
	ru.Stocks["m_48n6"] = 0
	w.Apply(Command{Kind: CmdKeepStock, Side: data.RU, Item: "m_48n6", Int: 1})
	w.interceptorStocks(data.RU)
	w.interceptorStocks(data.RU)
	if logCount("Кончились ракеты") != 1 {
		t.Fatal("предупреждение «кончились» не пришло ровно один раз")
	}
	has := 0
	for _, o := range ru.Orders {
		if o.Item == "m_48n6" {
			has++
		}
	}
	if has != 1 {
		t.Fatalf("автозаказ ЗУР: %d позиций в госзаказе", has)
	}
	// Украина теряет ПВО — приходит замена.
	ua := w.Sides[data.UA]
	lost := 0
	for id, u := range w.Units {
		if u.Side == data.UA {
			if ut := w.cat.UnitByID[u.Type]; ut.Kind == "ad" && ut.Interceptor != "" && ut.Cost["money"] >= 100 {
				delete(w.Units, id)
				lost++
			}
		}
	}
	if lost < 5 || w.adLoss(data.UA) < 0.9 {
		t.Fatalf("потеряно %d комплексов, доля %.2f", lost, w.adLoss(data.UA))
	}
	run(w, 49*60)
	if !ua.AidDone["aid_ad_recovery1"] {
		t.Fatal("замена ПВО не пришла после потери парка")
	}
}

// Предупреждение о взлёте стратегической авиации приходит не чаще раза в bomber_warn_min минут.
func TestBomberWarningThrottle(t *testing.T) {
	w := newTestWorld(t)
	run(w, w.PrepEnd+1)
	var bs *Building
	for _, b := range w.Buildings {
		if b.Side == data.RU && b.Type == "airbase_strat" && b.Operational() && (bs == nil || b.ID < bs.ID) {
			bs = b
		}
	}
	if bs == nil {
		t.Skip("нет рабочей стратегической авиабазы")
	}
	tgt := findBuilding(w, "Трипольская ТЭС")
	bs.Budget = 20
	w.Sides[data.RU].Stocks["kh101"] = 20
	count := func() int {
		n := 0
		for _, e := range w.Sides[data.UA].Events {
			if strings.Contains(e.Text, "взлёт стратегической авиации") {
				n++
			}
		}
		return n
	}
	for i := 0; i < 3; i++ {
		if e := w.Strike(data.RU, StrikePlan{Source: bs.ID, Munition: "kh101", Count: 1, Target: Pt{tgt.X, tgt.Y}}); e != "" {
			t.Fatal(e)
		}
	}
	if n := count(); n != 1 {
		t.Fatalf("за три быстрых пуска ожидалось 1 предупреждение, пришло %d", n)
	}
	w.Time += w.cat.Rules.BomberWarnMin
	if e := w.Strike(data.RU, StrikePlan{Source: bs.ID, Munition: "kh101", Count: 1, Target: Pt{tgt.X, tgt.Y}}); e != "" {
		t.Fatal(e)
	}
	if n := count(); n != 2 {
		t.Fatalf("через %v мин ожидалось второе предупреждение, всего %d", w.cat.Rules.BomberWarnMin, n)
	}
}
