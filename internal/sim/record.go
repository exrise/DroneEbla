package sim

import (
	"bufio"
	"encoding/json"
	"io"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/exrise/droneebla/internal/data"
)

// Recorder пишет журнал партии построчным JSON: метаданные, приказы обоих игроков, события
// и почасовые срезы состояния обеих сторон. Нужен для настройки баланса по реальным партиям.
type Recorder struct {
	mu       sync.Mutex
	bw       *bufio.Writer
	c        io.Closer
	lastHour int
	ended    bool
}

// SetRecorder включает журнал партии в wc (nil отключает). mode — «network», «solo», «sandbox»…
func (w *World) SetRecorder(wc io.WriteCloser, mode string, extra map[string]any) {
	if w.rec != nil {
		w.rec.Close()
		w.rec = nil
	}
	if wc == nil {
		return
	}
	r := &Recorder{bw: bufio.NewWriterSize(wc, 1<<16), c: wc, lastHour: int(w.Time / 60)}
	w.rec = r
	meta := map[string]any{
		"mode": mode, "seed": w.Seed, "game_time": w.Time, "sandbox": w.Sandbox, "solo": w.Solo, "cheat": w.Cheat,
		"started": time.Now().Format(time.RFC3339),
	}
	for k, v := range extra {
		meta[k] = v
	}
	r.write("meta", w.Time, meta)
	r.flush()
}

// CloseRecorder дописывает итог и закрывает журнал.
func (w *World) CloseRecorder() {
	if w.rec == nil {
		return
	}
	w.recSnapshot()
	w.rec.Close()
	w.rec = nil
}

func (r *Recorder) write(kind string, t float64, v any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, err := json.Marshal(map[string]any{"k": kind, "t": math.Round(t*10) / 10, "d": v})
	if err != nil {
		return
	}
	r.bw.Write(b)
	r.bw.WriteByte('\n')
}

func (r *Recorder) flush() {
	r.mu.Lock()
	r.bw.Flush()
	r.mu.Unlock()
}

// Close сбрасывает буфер и закрывает файл.
func (r *Recorder) Close() {
	r.mu.Lock()
	r.bw.Flush()
	r.c.Close()
	r.mu.Unlock()
}

func (w *World) recCommand(c Command, err string) {
	if w.rec == nil {
		return
	}
	d := map[string]any{"side": c.Side, "cmd": c.Kind}
	if c.Item != "" {
		d["item"] = c.Item
	}
	if c.ID != 0 {
		d["id"] = c.ID
	}
	if c.Count != 0 {
		d["count"] = c.Count
	}
	if c.Int != 0 {
		d["int"] = c.Int
	}
	if c.X != 0 || c.Y != 0 {
		d["x"], d["y"] = math.Round(c.X), math.Round(c.Y)
	}
	if len(c.Pts) > 0 {
		d["pts"] = len(c.Pts)
	}
	if err != "" {
		d["err"] = err
	}
	w.rec.write("cmd", w.Time, d)
}

func (w *World) recEvent(side, level int, text string) {
	if w.rec == nil {
		return
	}
	w.rec.write("event", w.Time, map[string]any{"side": side, "lvl": level, "text": text})
}

// recTick вызывается после каждого шага: почасовой срез и итог партии.
func (w *World) recTick() {
	if w.rec == nil {
		return
	}
	if h := int(w.Time / 60); h > w.rec.lastHour {
		w.rec.lastHour = h
		w.recSnapshot()
		w.rec.flush()
	}
	if w.Winner >= 0 && !w.rec.ended {
		w.rec.ended = true
		w.recSnapshot()
		w.rec.write("end", w.Time, map[string]any{"winner": w.Winner, "reason": w.WinReason})
		w.rec.flush()
	}
}

// recSnapshot — состояние обеих сторон.
func (w *World) recSnapshot() {
	if w.rec == nil {
		return
	}
	var sides [2]map[string]any
	owned := [3]int{}
	for _, o := range w.Owner {
		if int(o) < len(owned) {
			owned[o]++
		}
	}
	for s := 0; s < 2; s++ {
		sd := w.Sides[s]
		res := map[string]float64{}
		for i, k := range data.ResKeys {
			res[k] = math.Round(sd.Res[i])
		}
		front := make([]map[string]any, 3)
		for d := 0; d < 3; d++ {
			f := sd.Front[d]
			front[d] = map[string]any{
				"men": r1(f.Men), "armor": r1(f.Armor), "art": r1(f.Artillery), "fpv": r1(f.FPV), "fpv_pow": r1(f.FPVPow),
				"power": r1(w.dirPower(s, d)), "supply": r1(f.Supply * 100), "tiles": f.Tiles, "losses": r1(f.Losses),
				"posture": sd.PostureDir[d], "alloc": r1(sd.Alloc[d] * 100),
			}
		}
		stocks := map[string]float64{}
		for k, v := range sd.Stocks {
			if v >= 1 {
				stocks[k] = math.Round(v)
			}
		}
		units := map[string]int{}
		for _, u := range w.Units {
			if u.Side == s {
				units[u.Type]++
			}
		}
		bld := map[string]map[string]float64{}
		for _, b := range w.Buildings {
			if b.Side != s {
				continue
			}
			e := bld[b.Type]
			if e == nil {
				e = map[string]float64{}
				bld[b.Type] = e
			}
			e["n"]++
			e["hp"] += b.frac()
		}
		for _, e := range bld {
			e["hp"] = r1(e["hp"] / e["n"] * 100)
		}
		var orders []string
		for _, o := range sd.Orders {
			orders = append(orders, o.Item)
		}
		sort.Strings(orders)
		var researched []string
		for id := range sd.Researched {
			researched = append(researched, id)
		}
		sort.Strings(researched)
		sides[s] = map[string]any{
			"res": res, "income": r1(sd.Income), "morale": r1(sd.Morale), "people": r1(sd.People), "labor_loss": r1(sd.LaborLoss * 100),
			"power_gen": r1(sd.Power[0]), "power_use": r1(sd.Power[1]), "blackout": r1(sd.Blackout * 100),
			"front": front, "stocks": stocks, "units": units, "buildings": bld, "orders": orders,
			"researched": researched, "researching": sd.Research, "res_rate": r1(sd.ResRate), "res_fund": sd.ResFund,
			"known_contacts": len(sd.Known), "tiles": owned[s+1], "speed": sd.Speed,
		}
	}
	w.rec.write("snap", w.Time, map[string]any{"war": w.War(), "sides": sides, "proj": len(w.Projs)})
}

func r1(v float64) float64 { return math.Round(v*10) / 10 }
