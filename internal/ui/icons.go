package ui

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/exrise/droneebla/internal/data"
)

type imageDst = *ebiten.Image

// Значки на карте: форма зависит от типа юнита и здания, чтобы их можно было
// различать без подписей. Всё рисуется без сглаживания.

// unitGlyph рисует значок вида юнита внутри прямоугольника/ромба; s — полуразмер.
func unitGlyph(dst imageDst, ut *data.UnitType, cx, cy, s float64, c color.Color) {
	cx, cy = math.Round(cx), math.Round(cy)
	switch ut.Kind {
	case "ad":
		switch {
		case ut.Interceptor == "":
			disc(dst, cx, cy, math.Max(1.5, s*0.55), c) // пушки и МОГ — точка
		case ut.RangeKm >= 30:
			triangle(dst, cx, cy-s, cx+s, cy+s*0.8, cx-s, cy+s*0.8, c) // ЗРК дальнего действия — сплошной треугольник
		default:
			// ЗРК ближнего действия — контур треугольника
			line(dst, cx, cy-s, cx+s, cy+s*0.8, c, 1.5)
			line(dst, cx+s, cy+s*0.8, cx-s, cy+s*0.8, c, 1.5)
			line(dst, cx-s, cy+s*0.8, cx, cy-s, c, 1.5)
		}
	case "radar":
		circle(dst, cx, cy-s*0.2, math.Max(2, s*0.75), c, 1.5)
		line(dst, cx, cy-s*0.2, cx+s*0.7, cy-s, c, 1.5)
		line(dst, cx, cy+s*0.5, cx, cy+s, c, 1.5)
	case "reb":
		line(dst, cx-s, cy+s*0.5, cx-s*0.2, cy-s*0.8, c, 1.5)
		line(dst, cx-s*0.2, cy-s*0.8, cx+s*0.2, cy+s*0.8, c, 1.5)
		line(dst, cx+s*0.2, cy+s*0.8, cx+s, cy-s*0.5, c, 1.5)
	case "rtr":
		diamond(dst, cx, cy, s, c)
	case "launcher":
		triangle(dst, cx, cy-s, cx+s*0.8, cy, cx-s*0.8, cy, c) // стрелка вверх
		fillRect(dst, cx-s*0.3, cy, s*0.6, s*0.9, c)
	default:
		disc(dst, cx, cy, math.Max(1.5, s*0.5), c)
	}
}

// buildingShape возвращает форму значка здания.
func buildingShape(bt *data.BuildingType) string {
	switch {
	case bt.Bridge:
		return "bridge"
	case len(bt.Aircraft) > 0 || bt.ID == "airfield":
		return "air"
	case bt.ID == "naval_base" || bt.OilPort || bt.ID == "grain_port":
		return "port"
	case bt.Power > 0 || bt.Transfer > 0:
		return "diamond"
	case bt.Supply > 0:
		return "tri"
	case len(bt.Launch) > 0:
		return "launch"
	}
	return "square"
}

// drawBuildingIcon рисует значок здания: filled — заливка (свои), иначе контур (метка противника).
func drawBuildingIcon(dst imageDst, bt *data.BuildingType, cx, cy, s float64, c color.Color, filled bool) {
	cx, cy = math.Round(cx), math.Round(cy)
	h := s / 2
	white := color.RGBA{245, 240, 225, 255}
	switch buildingShape(bt) {
	case "diamond": // энергетика
		if filled {
			diamond(dst, cx, cy, h*1.2, c)
		} else {
			d := h * 1.2
			line(dst, cx, cy-d, cx+d, cy, c, 2)
			line(dst, cx+d, cy, cx, cy+d, c, 2)
			line(dst, cx, cy+d, cx-d, cy, c, 2)
			line(dst, cx-d, cy, cx, cy-d, c, 2)
		}
	case "tri": // логистика
		if filled {
			triangle(dst, cx, cy-h*1.2, cx+h*1.2, cy+h, cx-h*1.2, cy+h, c)
		} else {
			line(dst, cx, cy-h*1.2, cx+h*1.2, cy+h, c, 2)
			line(dst, cx+h*1.2, cy+h, cx-h*1.2, cy+h, c, 2)
			line(dst, cx-h*1.2, cy+h, cx, cy-h*1.2, c, 2)
		}
	case "bridge": // две планки
		if filled {
			fillRect(dst, cx-h*1.3, cy-h, h*2.6, h*0.8, c)
			fillRect(dst, cx-h*1.3, cy+h*0.2, h*2.6, h*0.8, c)
		} else {
			strokeRect(dst, cx-h*1.3, cy-h, h*2.6, h*0.8, c, 1.5)
			strokeRect(dst, cx-h*1.3, cy+h*0.2, h*2.6, h*0.8, c, 1.5)
		}
	case "air": // квадрат с крестом-самолётом
		if filled {
			fillRect(dst, cx-h, cy-h, s, s, c)
			line(dst, cx-h*0.8, cy, cx+h*0.8, cy, white, 1.5)
			line(dst, cx, cy-h*0.8, cx, cy+h*0.8, white, 1.5)
		} else {
			strokeRect(dst, cx-h, cy-h, s, s, c, 2)
			line(dst, cx-h*0.7, cy, cx+h*0.7, cy, c, 1.5)
			line(dst, cx, cy-h*0.7, cx, cy+h*0.7, c, 1.5)
		}
	case "port": // квадрат с кружком
		if filled {
			fillRect(dst, cx-h, cy-h, s, s, c)
			circle(dst, cx, cy, math.Max(1.5, h*0.5), white, 1.5)
		} else {
			strokeRect(dst, cx-h, cy-h, s, s, c, 2)
			circle(dst, cx, cy, math.Max(1.5, h*0.5), c, 1.5)
		}
	case "launch": // площадка пуска: квадрат со стрелкой
		if filled {
			fillRect(dst, cx-h, cy-h, s, s, c)
			triangle(dst, cx, cy-h*0.8, cx+h*0.6, cy+h*0.3, cx-h*0.6, cy+h*0.3, white)
		} else {
			strokeRect(dst, cx-h, cy-h, s, s, c, 2)
			triangle(dst, cx, cy-h*0.7, cx+h*0.5, cy+h*0.3, cx-h*0.5, cy+h*0.3, c)
		}
	default: // производство и прочее
		if filled {
			fillRect(dst, cx-h, cy-h, s, s, c)
		} else {
			strokeRect(dst, cx-h, cy-h, s, s, c, 2)
		}
	}
}
