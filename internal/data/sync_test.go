package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDir(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := SyncDir(dir); err != nil {
		t.Fatal(err)
	}
	// Игрок правит один файл, другой остаётся нетронутым.
	os.WriteFile(filepath.Join(dir, "rules.json"), []byte(`{"game_min_per_sec": 2}`), 0o644)
	up, kept, err := SyncDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(up) != 0 || len(kept) != 1 || kept[0] != "rules.json" {
		t.Fatalf("обновлено %v, сохранено %v", up, kept)
	}
	// Файл старой версии (не тронутый игроком) заменяется.
	os.WriteFile(filepath.Join(dir, "units.json"), []byte("старое"), 0o644)
	sha := sha([]byte("старое"))
	os.WriteFile(filepath.Join(dir, ".defaults"), []byte("units.json "+sha+"\n"), 0o644)
	up, _, _ = SyncDir(dir)
	if len(up) != 1 || up[0] != "units.json" {
		t.Fatalf("старый файл не обновлён: %v", up)
	}
}
