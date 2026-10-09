package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Логотип «ATTRITION»: широкие жирные буквы с разрядкой, золотой градиент сверху вниз,
// тонкие линии по бокам и маленький купол ПВО над буквой «I» с тремя дронами.
// Слово один раз рисуется белым в отдельное изображение (по кеглю и масштабу rs),
// затем растягивается по горизонтали и красится двумя оттенками золота.

const (
	logoWord    = "ATTRITION"
	logoSize    = 46.0 // кегль до растяжения
	logoTrack   = 7.0  // разрядка между буквами
	logoStretch = 1.55 // растяжение по горизонтали
	logoH       = 62.0 // высота изображения слова (логических единиц)
)

var logoCache struct {
	img   *ebiten.Image
	scale float64
	w     float64 // ширина слова до растяжения
}

func logoImage() (*ebiten.Image, float64) {
	if logoCache.img != nil && logoCache.scale == rs {
		return logoCache.img, logoCache.w
	}
	w := -logoTrack
	for _, r := range logoWord {
		w += textWidth(string(r), logoSize) + logoTrack
	}
	img := ebiten.NewImage(int(w*rs)+2, int(logoH*rs)+2)
	x := 0.0
	for _, r := range logoWord {
		drawBold(img, string(r), x, 6, logoSize, color.White, 0)
		x += textWidth(string(r), logoSize) + logoTrack
	}
	logoCache.img, logoCache.scale, logoCache.w = img, rs, w
	return img, w
}

// drawLogo рисует логотип по центру cx, верх слова на y.
func (g *Game) drawLogo(cx, y float64) {
	u := &g.ui
	img, w := logoImage()
	ww := w * logoStretch
	x0 := cx - ww/2
	top, bot := color.RGBA{244, 205, 120, 255}, color.RGBA{196, 140, 46, 255}
	half := img.Bounds().Dy() / 2
	for i, part := range []image.Rectangle{
		image.Rect(0, 0, img.Bounds().Dx(), half),
		image.Rect(0, half, img.Bounds().Dx(), img.Bounds().Dy()),
	} {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(0, float64(part.Min.Y))
		op.GeoM.Scale(logoStretch, 1)
		op.GeoM.Translate(x0*rs, y*rs)
		c := top
		if i == 1 {
			c = bot
		}
		op.ColorScale.ScaleWithColor(c)
		op.Filter = ebiten.FilterLinear
		u.screen.DrawImage(img.SubImage(part).(*ebiten.Image), op)
	}
	// Линии по бокам слова.
	ly := y + 33
	gold := color.RGBA{217, 164, 65, 255}
	for _, s := range []float64{-1, 1} {
		xa := cx + s*(ww/2+18)
		xb := cx + s*(ww/2+18+120)
		line(u.screen, xa, ly, xb, ly, withAlpha(gold, 170), 2)
		line(u.screen, xa, ly+7, xa+s*70, ly+7, withAlpha(gold, 90), 1)
	}
	// Купол над «I» и три дрона: над второй «I» (индекс 6).
	px := 0.0
	for i, r := range logoWord {
		wr := textWidth(string(r), logoSize)
		if i == 6 {
			px = px + wr/2
			break
		}
		px += wr + logoTrack
	}
	ix := x0 + px*logoStretch
	arcY := y + 2
	for k := -1.0; k <= 1.0; k += 0.1 {
		x1, x2 := ix+k*22, ix+(k+0.1)*22
		y1, y2 := arcY-9*(1-k*k), arcY-9*(1-(k+0.1)*(k+0.1))
		line(u.screen, x1, y1, x2, y2, withAlpha(gold, 200), 1.5)
	}
	for _, d := range [][2]float64{{-11, -2}, {0, -5}, {11, -2}} {
		disc(u.screen, ix+d[0], arcY+d[1]-14, 1.8, gold)
	}
}
