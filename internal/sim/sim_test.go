package sim

import (
	"path/filepath"
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
	w.Sides[data.RU].Posture = PostureOffense
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
	run(w, 5)
	ref := findBuilding(w, "Кременчугский НПЗ")
	ref.HP = 0
	// Ещё 9 повреждённых зданий делят бригады.
	n := 0
	var hit []*Building
	for _, b := range w.Buildings {
		if b.Side == data.UA && b != ref && b.Type != "npp" && n < 9 {
			b.HP = b.MaxHP * 0.3
			hit = append(hit, b)
			n++
		}
	}
	run(w, 60)
	working := 0
	for _, b := range w.Buildings {
		if b.Side == data.UA && b.Repairing {
			working++
		}
	}
	if working != w.cat.Rules.RepairCrews {
		t.Fatalf("работает бригад %d, ожидалось %d", working, w.cat.Rules.RepairCrews)
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
