// DroneEbla — стратегия в реальном времени о современной войне на истощение.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/exrise/droneebla/internal/data"
	"github.com/exrise/droneebla/internal/ui"
	"github.com/exrise/droneebla/internal/world"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	dir := filepath.Dir(exe)
	if d := os.Getenv("DRONEEBLA_DIR"); d != "" {
		dir = d
	}
	dataDir := filepath.Join(dir, "data")
	saveDir := filepath.Join(dir, "saves")
	exportDefaults(dataDir)

	logf, _ := os.OpenFile(filepath.Join(dir, "droneebla.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if logf != nil {
		log.SetOutput(logf)
	}
	cat, err := data.Load(dataDir)
	if err != nil {
		fatal("Ошибка в игровых данных (папка data): " + err.Error())
	}
	m, err := world.Load()
	if err != nil {
		fatal("Ошибка карты: " + err.Error())
	}
	ebiten.SetWindowTitle("DroneEbla — война на истощение")
	ebiten.SetWindowSize(1600, 900)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(60)
	g := ui.New(cat, m, dataDir, saveDir)
	if err := ebiten.RunGame(g); err != nil {
		fatal(err.Error())
	}
}

// exportDefaults выкладывает встроенные JSON рядом с игрой, чтобы их
// можно было править. Существующие файлы не перезаписываются.
func exportDefaults(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	for _, f := range data.Files {
		p := filepath.Join(dir, f)
		if _, err := os.Stat(p); err == nil {
			continue
		}
		if b, err := data.DefaultFile(f); err == nil {
			os.WriteFile(p, b, 0o644)
		}
	}
	readme := filepath.Join(dir, "README.txt")
	if _, err := os.Stat(readme); err != nil {
		os.WriteFile(readme, []byte(dataReadme), 0o644)
	}
}

const dataReadme = `Игровые данные DroneEbla.

Здесь лежат все цифры игры: здания (buildings.json), юниты (units.json),
боеприпасы (munitions.json), снаряжение фронта (front.json), дерево
технологий (tech.json), стартовые условия сторон (sides.json), реальные
объекты на карте (objects.json) и общие правила (rules.json).

Файлы можно править в любом текстовом редакторе. Изменения применяются при
следующем запуске игры. Чтобы вернуть значения по умолчанию — удалите файл,
игра создаст его заново.

В сетевой игре файлы у обоих игроков должны совпадать, иначе подключение
будет отклонено.
`

func fatal(msg string) {
	log.Println(msg)
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
