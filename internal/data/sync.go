package data

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prevDefaults — хэши файлов по умолчанию из прошлых версий игры. Если файл
// в папке data совпадает с одним из них, игрок его не трогал и его можно
// спокойно заменить новой версией.
var prevDefaults = map[string][]string{
	"rules.json":     {"b26eede4093eba0e6340b60d6436da6b767ffbd6193d1e65f1a1ed262c9b8a3a"},
	"buildings.json": {"bcbf30d3852c01a656ddbe5b3c81e337ef7743cb812d6575ed2366ebc2e6505e"},
	"units.json":     {"32afc87ac08a46fc9d61a85d736b4421e4a5d8eba7d5c5cdc8c42ff3afc01f69"},
	"munitions.json": {"288990b975653ce064d5495dd28036f8c17668a1f3368276e3a907bfd1919a13"},
	"front.json":     {"41c4cb89f9dfbe188654432073792d253f95ace92a63f75d86118345837faae5"},
	"tech.json":      {"c0a5162b56c3b1beab2492cf75b2d36be03662dd8cba3e94cdba500107d6e5c5"},
	"sides.json":     {"587bf40883594c93a88195cf0142e065451029a484226650ab778fabdcc0e3c3"},
	"objects.json":   {"4e46d3537900aca501daa10ad1826914d511ddaa875ba0de43962dc9ec11cdbf"},
}

const manifestName = ".defaults"

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// SyncDir выкладывает игровые данные в папку dir для правки:
// отсутствующие файлы создаёт, не изменённые игроком — обновляет до текущей
// версии, изменённые игроком — оставляет как есть (их список возвращается).
func SyncDir(dir string) (updated, kept []string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	manifest := map[string]string{}
	if f, err := os.Open(filepath.Join(dir, manifestName)); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if parts := strings.Fields(sc.Text()); len(parts) == 2 {
				manifest[parts[0]] = parts[1]
			}
		}
		f.Close()
	}
	var out strings.Builder
	for _, name := range Files {
		def, err := DefaultFile(name)
		if err != nil {
			return updated, kept, err
		}
		defSha := sha(def)
		p := filepath.Join(dir, name)
		cur, err := os.ReadFile(p)
		switch {
		case err != nil:
			if err := os.WriteFile(p, def, 0o644); err != nil {
				return updated, kept, err
			}
		case sha(cur) == defSha:
			// уже актуален
		case sha(cur) == manifest[name] || contains(prevDefaults[name], sha(cur)):
			if err := os.WriteFile(p, def, 0o644); err != nil {
				return updated, kept, err
			}
			updated = append(updated, name)
		default:
			kept = append(kept, name)
		}
		fmt.Fprintf(&out, "%s %s\n", name, defSha)
	}
	os.WriteFile(filepath.Join(dir, manifestName), []byte(out.String()), 0o644)
	return updated, kept, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
