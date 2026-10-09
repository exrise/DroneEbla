package ai

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

// Стенд «ИИ против ИИ»: оба бота играют друг против друга, печатаются метрики по суткам.
// Долгий (минуты), поэтому включается переменной DRONEEBLA_DUEL=<сутки> (например, 8).
//
//	DRONEEBLA_DUEL=8 go test ./internal/ai -run TestDuel -v -count=1
type duelStats struct {
	strikes  map[string]int    // приказы на удар по боеприпасу
	byName   map[string][2]int // по названию: [долетело, выпущено]
	sent     int               // выпущено штук по итогам залпов
	reached  int               // долетело
	reconLos int
	posture  int // смен позиции
}

var reReach = regexp.MustCompile(`^Итог удара \((.+?)\): долетело (\d+) из (\d+)`)

func TestDuel(t *testing.T) {
	days, _ := strconv.Atoi(os.Getenv("DRONEEBLA_DUEL"))
	if days <= 0 {
		t.Skip("стенд включается переменной DRONEEBLA_DUEL=<сутки>")
	}
	w := newWorld(t)
	if seed, _ := strconv.ParseInt(os.Getenv("DRONEEBLA_DUEL_SEED"), 10, 64); seed != 0 {
		w.Seed = seed
		w.Attach(w.Catalog(), w.Map())
	}
	w.Solo = false
	w.StartPlacement()
	bots := [2]*AI{New(w.Catalog(), data.RU), New(w.Catalog(), data.UA)}
	// DRONEEBLA_DUEL_LEGACY=ru|ua|both — простой бот (smart=false) за выбранную сторону для сравнения.
	switch os.Getenv("DRONEEBLA_DUEL_LEGACY") {
	case "ru":
		bots[0].cfg.Smart = false
	case "ua":
		bots[1].cfg.Smart = false
	case "both":
		bots[0].cfg.Smart, bots[1].cfg.Smart = false, false
	}
	st := [2]*duelStats{{strikes: map[string]int{}, byName: map[string][2]int{}}, {strikes: map[string]int{}, byName: map[string][2]int{}}}
	for s := range bots {
		s := s
		bots[s].OnCommand = func(c sim.Command, err string) {
			if err != "" {
				return
			}
			switch c.Kind {
			case sim.CmdStrike:
				st[s].strikes[c.Item]++
			case sim.CmdPosture:
				st[s].posture++
			}
		}
		if os.Getenv("DRONEEBLA_DUEL_DEBUG") != "" {
			bots[s].Debug = func(f string, args ...any) { t.Logf(f, args...) }
		}
		bots[s].Place(w)
	}
	var seen [2]uint64
	line := func(day int) {
		var b strings.Builder
		fmt.Fprintf(&b, "сутки %2d:", day)
		for s := 0; s < 2; s++ {
			sd := w.Sides[s]
			ad, buildings, hp := 0, 0, 0.0
			for _, u := range w.Units {
				if u.Side == s {
					if ut := w.Catalog().UnitByID[u.Type]; ut != nil && ut.Kind == "ad" {
						ad++
					}
				}
			}
			for _, bd := range w.Buildings {
				if bd.Side == s {
					buildings++
					hp += bd.HP / bd.MaxHP
				}
			}
			men := sd.Dirs[0].Men + sd.Dirs[1].Men + sd.Dirs[2].Men + sd.Dirs[3].Men
			tiles := 0
			for _, o := range w.Owner {
				if int(o) == s+1 {
					tiles++
				}
			}
			fmt.Fprintf(&b, " | %s деньги %5.0f элек %4.0f ПВО %3d люди %4.0f рез %5.0f зданий %3d(%.0f%%) тайлов %5d мор %2.0f свет %2.0f%% удары %3d дол %3.0f%% пози %2d",
				data.SideKeys[s], sd.Res[data.ResMoney], sd.Res[data.ResElectronics], ad, men, sd.People, buildings, 100*hp/float64(max(buildings, 1)), tiles, sd.Morale, 100*sd.Blackout,
				sum(st[s].strikes), pct(st[s].reached, st[s].sent), st[s].posture)
		}
		t.Log(b.String())
	}
	for k := 1; k <= days*1440 && w.Winner < 0; k++ {
		w.Step(1)
		for s := range bots {
			bots[s].Tick(w)
			for _, e := range w.Sides[s].Events {
				if e.ID <= seen[s] {
					continue
				}
				seen[s] = e.ID
				if m := reReach.FindStringSubmatch(e.Text); m != nil {
					a, _ := strconv.Atoi(m[2])
					b, _ := strconv.Atoi(m[3])
					st[s].reached += a
					st[s].sent += b
					r := st[s].byName[m[1]]
					st[s].byName[m[1]] = [2]int{r[0] + a, r[1] + b}
				}
				if strings.HasPrefix(e.Text, "Потерян разведчик") {
					st[s].reconLos++
				}
			}
		}
		if k%1440 == 0 {
			line(k / 1440)
		}
	}
	t.Logf("победитель: %d (%s)", w.Winner, w.WinReason)
	for s := 0; s < 2; s++ {
		t.Logf("%s: удары %v, разведчиков потеряно %d", data.SideKeys[s], st[s].strikes, st[s].reconLos)
		t.Logf("%s: долетело/выпущено %v", data.SideKeys[s], st[s].byName)
		cnt := map[string]int{}
		for _, b := range w.Buildings {
			if b.Side == s && b.HP >= b.MaxHP*0.5 {
				cnt[b.Type]++
			}
		}
		t.Logf("%s: целых зданий %v", data.SideKeys[s], cnt)
		var stalled []string
		for _, o := range w.Sides[s].Orders {
			if o.Stalled {
				stalled = append(stalled, o.Item)
			}
		}
		t.Logf("%s: импорт %v, исследовано %d, исследуется %q", data.SideKeys[s], w.Sides[s].ImportCount, len(w.Sides[s].Researched), w.Sides[s].Research)
		t.Logf("%s: заказов %d, застряло %v; запасы %v", data.SideKeys[s], len(w.Sides[s].Orders), stalled, w.Sides[s].Stocks)
	}
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}
