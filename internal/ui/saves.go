package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/sim"
)

// Автосохранение хоста и описание сохранений: рядом с каждым .sav лежит .meta.json
// (режим, сторона, игровое время, дата) — по нему список сохранений показывает понятные подписи.

const (
	autosaveSlots    = 3
	autosaveEveryMin = 360.0 // игровых минут между автосохранениями
)

type saveMeta struct {
	Mode  string  `json:"mode"`
	Side  int     `json:"side"`
	Time  float64 `json:"time"`
	Saved string  `json:"saved"`
	Auto  bool    `json:"auto,omitempty"`
}

func metaPath(sav string) string { return strings.TrimSuffix(sav, ".sav") + ".meta.json" }

func (g *Game) modeName(v *sim.View) string {
	switch {
	case v.Solo:
		return "Одиночная"
	case v.Sandbox:
		return "Песочница"
	}
	return "Сеть"
}

func writeMeta(sav string, m saveMeta) {
	if b, err := json.Marshal(m); err == nil {
		os.WriteFile(metaPath(sav), b, 0o644)
	}
}

func readMeta(sav string) (saveMeta, bool) {
	var m saveMeta
	b, err := os.ReadFile(metaPath(sav))
	if err != nil || json.Unmarshal(b, &m) != nil {
		return m, false
	}
	return m, true
}

// saveLabel — подпись сохранения в списке.
func saveLabel(sav string) string {
	name := filepath.Base(sav)
	m, ok := readMeta(sav)
	if !ok {
		return name
	}
	prefix := ""
	if m.Auto {
		prefix = "Автосохранение · "
	}
	side := ""
	if m.Side >= 0 && m.Side < len(data.SideNames) {
		side = data.SideNames[m.Side] + " · "
	}
	return fmt.Sprintf("%s%s · %s%s · %s", prefix, sim.FmtTime(m.Time), side, m.Mode, m.Saved)
}

// autosaveTick вызывается каждый кадр в игре: хост раз в 6 игровых часов пишет сохранение по кругу в 3 слота.
func (g *Game) autosaveTick() {
	v := g.view
	if v == nil || g.sess == nil || !g.sess.IsHost() || v.Placement || v.Winner >= 0 || g.autoBusy {
		return
	}
	if g.autoNext == 0 || v.Time < g.autoNext-autosaveEveryMin*2 {
		g.autoNext = v.Time + autosaveEveryMin // началась новая партия или загрузка
		return
	}
	if v.Time < g.autoNext {
		return
	}
	g.autoNext = v.Time + autosaveEveryMin
	slot := g.autoN%autosaveSlots + 1
	g.autoN++
	os.MkdirAll(g.saveDir, 0o755)
	path := filepath.Join(g.saveDir, fmt.Sprintf("autosave_%d.sav", slot))
	meta := saveMeta{Mode: g.modeName(v), Side: v.Side, Time: v.Time, Saved: time.Now().Format("02.01 15:04"), Auto: true}
	sess := g.sess
	g.autoBusy = true
	go func() {
		if err := sess.Save(path); err == nil {
			writeMeta(path, meta)
		}
		g.autoBusy = false
	}()
}

// deleteSave удаляет сохранение и его описание.
func deleteSave(sav string) {
	os.Remove(sav)
	os.Remove(metaPath(sav))
}
