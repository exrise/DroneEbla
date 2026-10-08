package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayoutScale(t *testing.T) {
	for _, c := range []struct {
		w, h, dsf, manual, want float64
	}{
		{1600, 900, 1, 0, 1}, // авто
		{1600, 900, 1, 1, 1}, // 100%
		{1600, 900, 1, 0.75, 0.75},
		{1600, 900, 1, 2, 1.1}, // 200% не помещается: урезано до 1440 логических по ширине
		{1920, 1080, 1, 1.25, 1.25},
		{1920, 1080, 1, 2, 1.3},
		{3840, 2160, 1, 2, 2},
		{1280, 720, 1, 1.25, 0.85}, // на минимальном окне больше 85% нельзя
		{1280, 720, 1.5, 1, 1.3},   // Windows 150%: 100% = 1.5 физических пикселя, но интерфейс не помещается
		{1600, 900, 1.5, 0, 1.5},   // авто считается по физическим пикселям
	} {
		if got := layoutScale(c.w, c.h, c.dsf, c.manual); got != c.want {
			t.Errorf("layoutScale(%v×%v, dpi %v, ручной %v) = %v, ожидалось %v", c.w, c.h, c.dsf, c.manual, got, c.want)
		}
		// Логический экран никогда не меньше минимального.
		if c.manual > 0 {
			s := layoutScale(c.w, c.h, c.dsf, c.manual)
			if lw, lh := c.w*c.dsf/s, c.h*c.dsf/s; lw < minLogicalW-1 || lh < minLogicalH-1 {
				t.Errorf("логический экран %.0f×%.0f меньше минимального", lw, lh)
			}
		}
	}
}

func TestStepScale(t *testing.T) {
	for _, c := range []struct {
		cur  float64
		up   bool
		want float64
	}{{1, true, 1.25}, {1, false, 0.75}, {0.75, false, 0.75}, {2, true, 2}, {1.1, true, 1.25}, {1.1, false, 1}, {0.85, true, 1}} {
		if got := stepScale(c.cur, c.up); got != c.want {
			t.Errorf("stepScale(%v, %v) = %v, ожидалось %v", c.cur, c.up, got, c.want)
		}
	}
}

func TestWindowChoices(t *testing.T) {
	if got := windowChoices(0, 0); len(got) != len(winPresets) {
		t.Errorf("монитор неизвестен: %d вариантов вместо %d", len(got), len(winPresets))
	}
	got := windowChoices(1920, 1080)
	if last := got[len(got)-1]; last != [2]int{1600, 900} {
		t.Errorf("на 1920×1080 последним должен быть 1600×900, а он %v", last)
	}
	if got := windowChoices(1024, 600); len(got) != 1 || got[0] != [2]int{1280, 720} {
		t.Errorf("на маленьком мониторе остаётся минимальный размер, а вышло %v", got)
	}
}

func TestSettingsLoadSave(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if s := LoadSettings(p); s != DefaultSettings() {
		t.Fatalf("без файла ожидались значения по умолчанию, а вышло %+v", s)
	}
	os.WriteFile(p, []byte("{мусор"), 0o644)
	if s := LoadSettings(p); s != DefaultSettings() {
		t.Fatalf("битый файл должен давать значения по умолчанию, а вышло %+v", s)
	}
	// Частичный файл: недостающее берётся по умолчанию, недопустимое отбрасывается.
	os.WriteFile(p, []byte(`{"scale":1.25,"win_w":200,"win_h":100}`), 0o644)
	s := LoadSettings(p)
	if s.Scale != 1.25 || s.WinW != 1600 || s.WinH != 900 || !s.Glass {
		t.Fatalf("частичный файл разобран неверно: %+v", s)
	}
	os.WriteFile(p, []byte(`{"scale":1.3,"win_w":1920,"win_h":1080,"glass":false,"fullscreen":true}`), 0o644)
	s = LoadSettings(p)
	if s.Scale != 0 || s.WinW != 1920 || s.Glass || !s.Fullscreen {
		t.Fatalf("неверно разобрано: %+v", s)
	}
	// Запись и чтение обратно.
	g := &Game{set: Settings{WinW: 1366, WinH: 768, Scale: 1.5, Glass: true}, setPath: p}
	g.saveSettings()
	if got := LoadSettings(p); got != g.set {
		t.Fatalf("после записи прочиталось %+v, а записано %+v", got, g.set)
	}
}

func TestFitToMonitor(t *testing.T) {
	s := Settings{WinW: 2560, WinH: 1440}
	s.FitToMonitor(1920, 1080)
	if s.WinW != 1920 || s.WinH != 1080 {
		t.Errorf("окно не уменьшено до монитора: %dx%d", s.WinW, s.WinH)
	}
	s = Settings{WinW: 1600, WinH: 900}
	s.FitToMonitor(1024, 600)
	if s.WinW != minWinW || s.WinH != minWinH {
		t.Errorf("слишком маленький монитор: окно должно быть не меньше %dx%d, а %dx%d", minWinW, minWinH, s.WinW, s.WinH)
	}
	s = Settings{WinW: 1600, WinH: 900}
	s.FitToMonitor(0, 0)
	if s.WinW != 1600 {
		t.Error("монитор неизвестен — размер должен остаться")
	}
}

func TestRenderQuality(t *testing.T) {
	for _, c := range []struct{ w, h, set, want float64 }{
		{1600, 900, 0, 1},
		{2560, 1440, 0, 1},
		{3840, 2160, 0, 2560.0 / 3840},
		{3840, 2160, 0.5, 0.5},
		{1920, 1080, 0.75, 0.75},
	} {
		if got := renderQuality(c.w, c.h, c.set); got != c.want {
			t.Errorf("renderQuality(%v×%v, %v) = %v, ожидалось %v", c.w, c.h, c.set, got, c.want)
		}
	}
	// Логический размер интерфейса не зависит от качества: кадр / (масштаб × качество).
	s := layoutScale(3840, 2160, 1, 0)
	for _, q := range []float64{1, 0.75, 0.5} {
		if lw := 3840 * q / (s * q); lw != 3840/s {
			t.Errorf("при качестве %v логическая ширина %v, ожидалось %v", q, lw, 3840/s)
		}
	}
	st := Settings{WinW: 1600, WinH: 900, Quality: 0.6}
	st.normalize()
	if st.Quality != 0 {
		t.Errorf("недопустимое качество должно сбрасываться в авто, а стало %v", st.Quality)
	}
}

func TestGlassOpacity(t *testing.T) {
	if got := glassTintK(0.5); got != 1 {
		t.Errorf("при 50%% множитель плотности должен быть 1 (прежний вид), а %v", got)
	}
	if glassTintK(0) >= glassTintK(0.5) || glassTintK(1) <= glassTintK(0.5) {
		t.Error("множитель должен расти с ползунком")
	}
	if glassTintK(-1) != glassTintK(0) || glassTintK(2) != glassTintK(1) {
		t.Error("значения вне 0…1 должны ограничиваться")
	}
	s := Settings{WinW: 1600, WinH: 900, GlassOpacity: 7}
	s.normalize()
	if s.GlassOpacity != 0.5 {
		t.Errorf("недопустимая прозрачность должна сбрасываться в 0,5, а стала %v", s.GlassOpacity)
	}
	if d := DefaultSettings(); d.GlassOpacity != 0.5 {
		t.Errorf("по умолчанию 0,5, а %v", d.GlassOpacity)
	}
	// Файл без ключа прозрачности даёт значение по умолчанию.
	p := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(p, []byte(`{"scale":1}`), 0o644)
	if got := LoadSettings(p).GlassOpacity; got != 0.5 {
		t.Errorf("без ключа ожидалось 0,5, а %v", got)
	}
}

func TestLineK(t *testing.T) {
	if lineK(3) != 1 || lineK(0.05) != 0.5 {
		t.Errorf("lineK: на большом зуме 1, на малом 0,5; вышло %v и %v", lineK(3), lineK(0.05))
	}
	if !(lineK(0.3) < lineK(0.6)) {
		t.Error("линии должны утолщаться с приближением")
	}
}

func TestFitRowSameSize(t *testing.T) {
	labels := []string{"Оборона", "Активная оборона", "Наступление"}
	rsOld := rs
	rs = 1
	defer func() { rs = rsOld }()
	a := fitRow(labels, 124, 26)
	for _, l := range labels {
		if textWidth(l, a) > 124-8 {
			t.Errorf("подпись %q не помещается при общем кегле %v", l, a)
		}
	}
	if a < 10 || a > 14 {
		t.Errorf("кегль вне 10…14: %v", a)
	}
}
