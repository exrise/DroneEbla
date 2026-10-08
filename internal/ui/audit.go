package ui

import (
	"fmt"
	"image"
	"os"
)

// Автопроверка вёрстки: DRONEEBLA_AUDIT=1 печатает в консоль строки, которые вылезают за рамку панели
// или обрезаны на кнопках. Рамкой считается последняя нарисованная панель/плашка/карточка.
var (
	auditOn   = os.Getenv("DRONEEBLA_AUDIT") != ""
	auditCont image.Rectangle // последняя рамка (логические координаты)
	auditSeen = map[string]bool{}
)

func auditRect(x, y, w, h float64) {
	if auditOn {
		auditCont = image.Rect(int(x), int(y), int(x+w), int(y+h))
	}
}

func auditReport(kind, text string, a, b float64) {
	key := kind + "|" + text
	if auditSeen[key] {
		return
	}
	auditSeen[key] = true
	fmt.Printf("ВЁРСТКА (%s): «%s» — %.0f при доступных %.0f\n", kind, text, a, b)
}

// auditText проверяет, помещается ли строка в текущую рамку по правому краю.
func auditText(s string, x, y, size float64, align int) {
	if !auditOn || auditCont.Empty() {
		return
	}
	w := textWidth(s, size)
	left, right := x, x+w
	switch align {
	case 1:
		left, right = x-w/2, x+w/2
	case 2:
		left, right = x-w, x
	}
	c := auditCont
	if left < float64(c.Min.X) || left >= float64(c.Max.X) || y < float64(c.Min.Y) || y > float64(c.Max.Y) {
		return
	}
	if right > float64(c.Max.X)+1 {
		auditReport("за рамкой", s, right-float64(c.Min.X), float64(c.Dx()))
	}
}
