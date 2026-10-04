// Package ui — интерфейс игры на Ebitengine: меню, штабная карта, панели.
package ui

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// Палитра.
var (
	colPanel     = color.RGBA{24, 28, 34, 236}
	colPanel2    = color.RGBA{36, 42, 50, 245}
	colBorder    = color.RGBA{70, 80, 92, 255}
	colText      = color.RGBA{230, 230, 228, 255}
	colDim       = color.RGBA{150, 156, 164, 255}
	colAccent    = color.RGBA{217, 164, 65, 255}
	colGood      = color.RGBA{110, 190, 110, 255}
	colBad       = color.RGBA{225, 90, 80, 255}
	colWarn      = color.RGBA{235, 190, 70, 255}
	colButton    = color.RGBA{52, 60, 72, 255}
	colButtonHi  = color.RGBA{72, 84, 100, 255}
	colButtonOn  = color.RGBA{150, 112, 40, 255}
	colButtonOff = color.RGBA{40, 44, 50, 255}
	colRU        = color.RGBA{192, 57, 43, 255}
	colUA        = color.RGBA{46, 90, 172, 255}
	colMapText   = color.RGBA{40, 36, 30, 255}
)

func sideColor(s int) color.RGBA {
	if s == 0 {
		return colRU
	}
	return colUA
}

var (
	fontSrc     *text.GoTextFaceSource
	fontBoldSrc *text.GoTextFaceSource
	faces       = map[float64]*text.GoTextFace{}
	boldFaces   = map[float64]*text.GoTextFace{}
)

func init() {
	var err error
	fontSrc, err = text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		panic(err)
	}
	fontBoldSrc, err = text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))
	if err != nil {
		panic(err)
	}
}

func face(size float64) *text.GoTextFace {
	if f, ok := faces[size]; ok {
		return f
	}
	f := &text.GoTextFace{Source: fontSrc, Size: size}
	faces[size] = f
	return f
}

func boldFace(size float64) *text.GoTextFace {
	if f, ok := boldFaces[size]; ok {
		return f
	}
	f := &text.GoTextFace{Source: fontBoldSrc, Size: size}
	boldFaces[size] = f
	return f
}

// Ввод одного кадра.
type input struct {
	mx, my     int
	click      bool // левая кнопка нажата (ожидает обработки)
	rclick     bool
	dblclick   bool
	down       bool
	rdown      bool
	mdown      bool
	wheel      float64
	consumed   bool // клик уже обработан виджетом
	overUI     bool // курсор над панелью
	lastClickT int
	tick       int
}

// UI — непосредственный режим: виджеты рисуются и обрабатывают ввод в Draw.
type UI struct {
	in     input
	screen *ebiten.Image
	W, H   int
	tip    string
	clip   image.Rectangle // если задан, ввод принимается только внутри
}

func (u *UI) mouseIn(x, y, w, h int) bool {
	if !u.clip.Empty() && !image.Pt(u.in.mx, u.in.my).In(u.clip) {
		return false
	}
	return u.in.mx >= x && u.in.mx < x+w && u.in.my >= y && u.in.my < y+h
}

// clicked — был ли клик в прямоугольнике (и забрать его).
func (u *UI) clicked(x, y, w, h int) bool {
	if u.in.click && !u.in.consumed && u.mouseIn(x, y, w, h) {
		u.in.consumed = true
		return true
	}
	return false
}

// blockUI отмечает прямоугольник как панель (клики не уходят на карту).
func (u *UI) blockUI(x, y, w, h int) {
	if u.mouseIn(x, y, w, h) {
		u.in.overUI = true
	}
}

func fillRect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	vector.FillRect(dst, float32(x), float32(y), float32(w), float32(h), c, false)
}

func strokeRect(dst *ebiten.Image, x, y, w, h float64, c color.Color, t float32) {
	vector.StrokeRect(dst, float32(x), float32(y), float32(w), float32(h), t, c, false)
}

func line(dst *ebiten.Image, x0, y0, x1, y1 float64, c color.Color, t float32) {
	vector.StrokeLine(dst, float32(x0), float32(y0), float32(x1), float32(y1), t, c, false)
}

func circle(dst *ebiten.Image, x, y, r float64, c color.Color, t float32) {
	vector.StrokeCircle(dst, float32(x), float32(y), float32(r), t, c, false)
}

func disc(dst *ebiten.Image, x, y, r float64, c color.Color) {
	vector.FillCircle(dst, float32(x), float32(y), float32(r), c, false)
}

// drawText рисует строку; align: 0 влево, 1 по центру, 2 вправо.
func drawText(dst *ebiten.Image, s string, x, y float64, size float64, c color.Color, align int) {
	f := face(size)
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	switch align {
	case 1:
		op.PrimaryAlign = text.AlignCenter
	case 2:
		op.PrimaryAlign = text.AlignEnd
	}
	text.Draw(dst, s, f, op)
}

func drawBold(dst *ebiten.Image, s string, x, y float64, size float64, c color.Color, align int) {
	f := boldFace(size)
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	switch align {
	case 1:
		op.PrimaryAlign = text.AlignCenter
	case 2:
		op.PrimaryAlign = text.AlignEnd
	}
	text.Draw(dst, s, f, op)
}

// drawTextHalo — текст с подложкой для карты.
func drawTextHalo(dst *ebiten.Image, s string, x, y, size float64, c, halo color.Color, align int) {
	for _, d := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		drawText(dst, s, x+d[0], y+d[1], size, halo, align)
	}
	drawText(dst, s, x, y, size, c, align)
}

func textWidth(s string, size float64) float64 {
	w, _ := text.Measure(s, face(size), 0)
	return w
}

// wrap разбивает текст на строки по ширине.
func wrap(s string, size float64, width float64) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		cur := ""
		for _, wd := range words {
			try := wd
			if cur != "" {
				try = cur + " " + wd
			}
			if textWidth(try, size) > width && cur != "" {
				out = append(out, cur)
				cur = wd
			} else {
				cur = try
			}
		}
		out = append(out, cur)
	}
	return out
}

// Button — кнопка. Возвращает true при нажатии.
func (u *UI) Button(x, y, w, h int, label string) bool {
	return u.ButtonState(x, y, w, h, label, false, true)
}

// ButtonState — кнопка с состоянием «включено» и «доступна».
func (u *UI) ButtonState(x, y, w, h int, label string, on, enabled bool) bool {
	hover := u.mouseIn(x, y, w, h)
	c := colButton
	switch {
	case !enabled:
		c = colButtonOff
	case on:
		c = colButtonOn
	case hover:
		c = colButtonHi
	}
	fillRect(u.screen, float64(x), float64(y), float64(w), float64(h), c)
	strokeRect(u.screen, float64(x), float64(y), float64(w), float64(h), colBorder, 1)
	tc := colText
	if !enabled {
		tc = colDim
	}
	size := 14.0
	if h < 22 {
		size = 12
	}
	lbl := fitText(label, size, float64(w-6))
	drawText(u.screen, lbl, float64(x+w/2), float64(y)+float64(h)/2-size*0.62, size, tc, 1)
	if !enabled {
		return false
	}
	return u.clicked(x, y, w, h)
}

// fitText обрезает строку под ширину.
func fitText(s string, size, w float64) string {
	if textWidth(s, size) <= w {
		return s
	}
	for utf8.RuneCountInString(s) > 1 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
		if textWidth(s+"…", size) <= w {
			return s + "…"
		}
	}
	return s
}

// Bar — полоса прогресса.
func (u *UI) Bar(x, y, w, h int, frac float64, c color.Color) {
	fillRect(u.screen, float64(x), float64(y), float64(w), float64(h), color.RGBA{20, 20, 24, 255})
	fillRect(u.screen, float64(x), float64(y), float64(w)*math.Max(0, math.Min(1, frac)), float64(h), c)
	strokeRect(u.screen, float64(x), float64(y), float64(w), float64(h), colBorder, 1)
}

// Panel — фон панели.
func (u *UI) Panel(x, y, w, h int) {
	fillRect(u.screen, float64(x), float64(y), float64(w), float64(h), colPanel)
	strokeRect(u.screen, float64(x), float64(y), float64(w), float64(h), colBorder, 1)
	u.blockUI(x, y, w, h)
}

// Tooltip — подсказка у курсора (рисуется в конце кадра).
func (u *UI) Tooltip(x, y, w, h int, s string) {
	if u.mouseIn(x, y, w, h) {
		u.tip = s
	}
}

func (u *UI) drawTooltip() {
	if u.tip == "" {
		return
	}
	lines := wrap(u.tip, 13, 320)
	w := 0.0
	for _, l := range lines {
		w = math.Max(w, textWidth(l, 13))
	}
	h := float64(len(lines))*17 + 8
	x := float64(u.in.mx + 16)
	y := float64(u.in.my + 16)
	if x+w+12 > float64(u.W) {
		x = float64(u.W) - w - 12
	}
	if y+h > float64(u.H) {
		y = float64(u.in.my) - h - 4
	}
	fillRect(u.screen, x, y, w+12, h, color.RGBA{14, 16, 20, 245})
	strokeRect(u.screen, x, y, w+12, h, colAccent, 1)
	for i, l := range lines {
		drawText(u.screen, l, x+6, y+4+float64(i)*17, 13, colText, 0)
	}
	u.tip = ""
}

// sub возвращает подизображение экрана для обрезки содержимого.
func (u *UI) sub(x, y, w, h int) *ebiten.Image {
	return u.screen.SubImage(image.Rect(x, y, x+w, y+h)).(*ebiten.Image)
}

// TextField — однострочное поле ввода.
type TextField struct {
	Text    string
	Focused bool
	Max     int
}

// Draw рисует поле и обрабатывает ввод.
func (t *TextField) Draw(u *UI, x, y, w, h int) {
	if u.clicked(x, y, w, h) {
		t.Focused = true
	} else if u.in.click && !u.mouseIn(x, y, w, h) {
		t.Focused = false
	}
	if t.Focused {
		chars := ebiten.AppendInputChars(nil)
		for _, r := range chars {
			if t.Max == 0 || utf8.RuneCountInString(t.Text) < t.Max {
				t.Text += string(r)
			}
		}
		if repeatKey(ebiten.KeyBackspace) && len(t.Text) > 0 {
			_, n := utf8.DecodeLastRuneInString(t.Text)
			t.Text = t.Text[:len(t.Text)-n]
		}
	}
	fillRect(u.screen, float64(x), float64(y), float64(w), float64(h), color.RGBA{14, 16, 20, 255})
	bc := colBorder
	if t.Focused {
		bc = colAccent
	}
	strokeRect(u.screen, float64(x), float64(y), float64(w), float64(h), bc, 1)
	s := t.Text
	if t.Focused && (u.in.tick/30)%2 == 0 {
		s += "|"
	}
	drawText(u.screen, s, float64(x+6), float64(y)+float64(h)/2-9, 15, colText, 0)
}

var keyHeld = map[ebiten.Key]int{}

// repeatKey — нажатие с автоповтором.
func repeatKey(k ebiten.Key) bool {
	if ebiten.IsKeyPressed(k) {
		keyHeld[k]++
		n := keyHeld[k]
		return n == 1 || (n > 25 && n%3 == 0)
	}
	keyHeld[k] = 0
	return false
}

var whitePix = func() *ebiten.Image {
	img := ebiten.NewImage(3, 3)
	img.Fill(color.White)
	return img.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
}()

// triangle — заливка треугольника (без сглаживания, пакетно).
func triangle(dst *ebiten.Image, x1, y1, x2, y2, x3, y3 float64, c color.Color) {
	r, g, b, a := c.RGBA()
	cr, cg, cb, ca := float32(r)/0xffff, float32(g)/0xffff, float32(b)/0xffff, float32(a)/0xffff
	vs := []ebiten.Vertex{
		{DstX: float32(x1), DstY: float32(y1), SrcX: 1, SrcY: 1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		{DstX: float32(x2), DstY: float32(y2), SrcX: 1, SrcY: 1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		{DstX: float32(x3), DstY: float32(y3), SrcX: 1, SrcY: 1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
	}
	op := &ebiten.DrawTrianglesOptions{ColorScaleMode: ebiten.ColorScaleModePremultipliedAlpha}
	dst.DrawTriangles32(vs, []uint32{0, 1, 2}, whitePix, op)
}
