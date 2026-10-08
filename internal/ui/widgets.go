// Package ui — интерфейс игры на Ebitengine: меню, штабная карта, панели.
package ui

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Палитра.
var (
	colPanel     = color.RGBA{24, 28, 34, 236}
	colPanel2    = color.RGBA{36, 42, 50, 245}
	colBorder    = color.RGBA{70, 80, 92, 255}
	colText      = color.RGBA{230, 230, 228, 255}
	colDim       = color.RGBA{176, 183, 194, 255}
	colAccent    = color.RGBA{217, 164, 65, 255}
	colGood      = color.RGBA{110, 190, 110, 255}
	colBad       = color.RGBA{240, 110, 98, 255}
	colWarn      = color.RGBA{235, 190, 70, 255}
	colButton    = color.RGBA{52, 60, 72, 255}
	colButtonHi  = color.RGBA{72, 84, 100, 255}
	colButtonOn  = color.RGBA{150, 112, 40, 255}
	colButtonOff = color.RGBA{40, 44, 50, 255}
	colRU        = color.RGBA{192, 57, 43, 255}
	colUA        = color.RGBA{46, 90, 172, 255}
	colMapText   = color.RGBA{40, 36, 30, 255}
)

// sideText — светлый оттенок цвета стороны для текста на тёмном стекле.
func sideText(s int) color.RGBA {
	if s == 0 {
		return color.RGBA{242, 112, 100, 255}
	}
	return color.RGBA{112, 162, 244, 255}
}

func sideColor(s int) color.RGBA {
	if s == 0 {
		return colRU
	}
	return colUA
}

//go:embed fonts/Inter.ttf
var interTTF []byte

var (
	fontSrc   *text.GoTextFaceSource
	faces     = map[faceKey]*text.GoTextFace{}
	boldFaces = map[faceKey]*text.GoTextFace{}
)

func init() {
	var err error
	fontSrc, err = text.NewGoTextFaceSource(bytes.NewReader(interTTF))
	if err != nil {
		panic(err)
	}
}

// faceKey — кегль в логических единицах и масштаб отрисовки.
type faceKey struct{ size, scale float64 }

// newFaceScaled — шрифт Inter кегля size (логические единицы), нарисованный в scale раз крупнее;
// вес — вариативная ось wght, для крупных размеров включается оптический размер (ось opsz).
func newFaceScaled(size, weight, scale float64) *text.GoTextFace {
	f := &text.GoTextFace{Source: fontSrc, Size: size * scale}
	f.SetVariation(text.MustParseTag("wght"), float32(weight))
	f.SetVariation(text.MustParseTag("opsz"), float32(math.Max(14, math.Min(32, size))))
	return f
}

func face(size float64) *text.GoTextFace {
	k := faceKey{size, rs}
	if f, ok := faces[k]; ok {
		return f
	}
	f := newFaceScaled(size, 400, rs)
	faces[k] = f
	return f
}

func boldFace(size float64) *text.GoTextFace {
	k := faceKey{size, rs}
	if f, ok := boldFaces[k]; ok {
		return f
	}
	f := newFaceScaled(size, 650, rs)
	boldFaces[k] = f
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
	chars      []rune // введённые за тики с прошлого кадра символы
	backspace  int    // нажатия Backspace (с автоповтором) с прошлого кадра
}

// UI — непосредственный режим: виджеты рисуются и обрабатывают ввод в Draw.
type UI struct {
	in      input
	screen  *ebiten.Image
	W, H    int
	tip     string
	clip    image.Rectangle // если задан, ввод принимается только внутри
	on      bool            // стиль «стекло» включён и готов
	glassK  float32         // множитель плотности подложки стекла (ползунок в настройках)
	drag    string          // какой ползунок сейчас тянут
	rowSize float64         // общий кегль подписей ряда кнопок (0 — каждая кнопка подбирает свой)
	fx      *glassFX
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

// rs — масштаб отрисовки: физических пикселей на логическую единицу интерфейса. Вся вёрстка
// ведётся в логических единицах, а рисуется сразу в родном разрешении экрана — так на 4K
// текст и линии остаются чёткими (а не растягиваются из маленького кадра).
var rs = 1.0

// mapLineK — множитель толщины штрихов при рисовании карты: на малом зуме линии и контуры значков тоньше.
// Вне карты всегда 1.
var mapLineK = 1.0

// lineK — множитель по зуму камеры: от 1 при приближении до 0,5 на самом малом масштабе.
func lineK(z float64) float64 { return math.Max(0.5, math.Min(1, 0.35+0.65*z)) }

// vecAA — сглаживание векторных линий и кругов. Должно быть false: с antialias=true Ebiten 2.10
// в партии раздувает кучу Go до гигабайт (замер: 5 ГБ против 50 МБ) и кадр уходит в сотни миллисекунд.
const vecAA = false

// callStats — сколько примитивов нарисовано за кадр (для оверлея F3 и замеров).
var callStats struct{ rect, stroke, line, circle, disc, text, glyphs, tri int }

// px переводит логическую координату в физическую.
func px(v float64) float32 { return float32(v * rs) }

// snap — физическая координата, округлённая до пикселя (рамки без «лесенки» и швов).
func snap(v float64) float32 { return float32(math.Round(v * rs)) }

func fillRect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	callStats.rect++
	x0, y0, x1, y1 := snap(x), snap(y), snap(x+w), snap(y+h)
	vector.FillRect(dst, x0, y0, x1-x0, y1-y0, c, false)
}

func strokeRect(dst *ebiten.Image, x, y, w, h float64, c color.Color, t float32) {
	callStats.stroke++
	t *= float32(mapLineK)
	x0, y0, x1, y1 := snap(x), snap(y), snap(x+w), snap(y+h)
	vector.StrokeRect(dst, x0, y0, x1-x0, y1-y0, float32(math.Max(1, math.Round(float64(t)*rs))), c, false)
}

// offscreen — отрезок целиком по одну сторону от границы изображения (физические координаты).
func offscreen(dst *ebiten.Image, minX, minY, maxX, maxY, pad float64) bool {
	b := dst.Bounds()
	return maxX+pad < float64(b.Min.X) || minX-pad > float64(b.Max.X) || maxY+pad < float64(b.Min.Y) || minY-pad > float64(b.Max.Y)
}

func line(dst *ebiten.Image, x0, y0, x1, y1 float64, c color.Color, t float32) {
	callStats.line++
	t *= float32(mapLineK)
	if offscreen(dst, math.Min(x0, x1)*rs, math.Min(y0, y1)*rs, math.Max(x0, x1)*rs, math.Max(y0, y1)*rs, float64(t)*rs+2) {
		return
	}
	vector.StrokeLine(dst, px(x0), px(y0), px(x1), px(y1), float32(math.Max(1, float64(t)*rs)), c, vecAA)
}

func circle(dst *ebiten.Image, x, y, r float64, c color.Color, t float32) {
	callStats.circle++
	t *= float32(mapLineK)
	// Кольцо, которое не пересекает изображение: целиком снаружи или экран целиком внутри кольца.
	cx, cy, pr, pt := x*rs, y*rs, r*rs, float64(t)*rs+2
	if offscreen(dst, cx-pr, cy-pr, cx+pr, cy+pr, pt) {
		return
	}
	b := dst.Bounds()
	far := 0.0
	for _, p := range [4][2]float64{{float64(b.Min.X), float64(b.Min.Y)}, {float64(b.Max.X), float64(b.Min.Y)}, {float64(b.Min.X), float64(b.Max.Y)}, {float64(b.Max.X), float64(b.Max.Y)}} {
		far = math.Max(far, math.Hypot(p[0]-cx, p[1]-cy))
	}
	if far < pr-pt {
		return
	}
	vector.StrokeCircle(dst, px(x), px(y), px(r), float32(math.Max(1, float64(t)*rs)), c, vecAA)
}

func disc(dst *ebiten.Image, x, y, r float64, c color.Color) {
	callStats.disc++
	vector.FillCircle(dst, px(x), px(y), px(r), c, vecAA)
}

// drawText рисует строку; align: 0 влево, 1 по центру, 2 вправо.
func drawText(dst *ebiten.Image, s string, x, y float64, size float64, c color.Color, align int) {
	callStats.text++
	callStats.glyphs += len(s)
	auditText(s, x, y, size, align)
	f := face(size)
	op := &text.DrawOptions{}
	op.GeoM.Translate(math.Round(x*rs), math.Round(y*rs))
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
	callStats.text++
	callStats.glyphs += len(s)
	auditText(s, x, y, size, align)
	f := boldFace(size)
	op := &text.DrawOptions{}
	op.GeoM.Translate(math.Round(x*rs), math.Round(y*rs))
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

type measureKey struct {
	s     string
	size  float64
	scale float64
}

// measureCache — ширины строк: text.Measure дорог, а за кадр одни и те же подписи измеряются десятки раз.
var measureCache = map[measureKey]float64{}

func textWidth(s string, size float64) float64 {
	k := measureKey{s, size, rs}
	if w, ok := measureCache[k]; ok {
		return w
	}
	if len(measureCache) > 20000 {
		measureCache = map[measureKey]float64{}
	}
	w, _ := text.Measure(s, face(size), 0)
	w /= rs
	measureCache[k] = w
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
	size := 14.0
	if h < 22 {
		size = 12
	}
	if u.rowSize > 0 {
		size = u.rowSize
	}
	for size > 10 && textWidth(label, size) > float64(w-8) {
		size -= 0.5
	}
	tc := colText
	if !enabled {
		tc = colDim
	}
	if u.on {
		fill, rim, light := pillNormal, float32(0.35), float32(1)
		switch {
		case !enabled:
			fill, rim, light = pillOff, 0.15, 0
		case on:
			fill, rim = pillOn, 0.55
		case hover:
			fill, rim = pillHover, 0.6
		}
		u.pill(float32(x), float32(y), float32(w), float32(h), 10, fill, rim, [3]float32{1, 1, 1}, light)
		if on && enabled {
			tc = color.RGBA{255, 250, 238, 255}
		}
	} else {
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
	}
	lbl := fitText(label, size, float64(w-8))
	if lbl != label {
		auditReport("кнопка обрезана", label, textWidth(label, size), float64(w-8))
	}
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
	frac = math.Max(0, math.Min(1, frac))
	if u.on {
		u.pill(float32(x), float32(y), float32(w), float32(h), 6, pillDark, 0.25, [3]float32{1, 1, 1}, 0)
		if fw := float64(w) * frac; fw >= 2 {
			r, g, b, _ := c.RGBA()
			u.pill(float32(x), float32(y), float32(fw), float32(h), 6, [4]float32{float32(r) / 0xffff, float32(g) / 0xffff, float32(b) / 0xffff, 0.95}, 0.3, [3]float32{1, 1, 1}, 1)
		}
		return
	}
	fillRect(u.screen, float64(x), float64(y), float64(w), float64(h), color.RGBA{20, 20, 24, 255})
	fillRect(u.screen, float64(x), float64(y), float64(w)*frac, float64(h), c)
	strokeRect(u.screen, float64(x), float64(y), float64(w), float64(h), colBorder, 1)
}

// Panel — фон панели (стеклянная плашка или плоский фон).
func (u *UI) Panel(x, y, w, h int) { u.PanelT(x, y, w, h, 0.66) }

// PanelT — панель с заданной плотностью подкраски стекла (0.5…0.9).
func (u *UI) PanelT(x, y, w, h int, tint float32) {
	auditRect(float64(x), float64(y), float64(w), float64(h))
	if u.on {
		u.glassPanel(float32(x), float32(y), float32(w), float32(h), 24, tint)
	} else {
		fillRect(u.screen, float64(x), float64(y), float64(w), float64(h), colPanel)
		strokeRect(u.screen, float64(x), float64(y), float64(w), float64(h), colBorder, 1)
	}
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
	if u.on {
		u.pill(float32(x), float32(y), float32(w+16), float32(h+4), 10, [4]float32{0.04, 0.055, 0.08, 0.93}, 0.5, [3]float32{1, 1, 1}, 0.4)
	} else {
		fillRect(u.screen, x, y, w+12, h, color.RGBA{14, 16, 20, 245})
		strokeRect(u.screen, x, y, w+12, h, colAccent, 1)
	}
	for i, l := range lines {
		drawText(u.screen, l, x+8, y+5+float64(i)*17, 13, colText, 0)
	}
	u.tip = ""
}

// sub возвращает подизображение экрана для обрезки содержимого.
func (u *UI) sub(x, y, w, h int) *ebiten.Image {
	return u.screen.SubImage(image.Rect(int(snap(float64(x))), int(snap(float64(y))), int(snap(float64(x+w))), int(snap(float64(y+h))))).(*ebiten.Image)
}

// TextField — однострочное поле ввода.
type TextField struct {
	Text    string
	Focused bool
	Max     int
	Allowed func(rune) bool // допустимые символы (nil — любые печатные)
}

// applyInput применяет введённые символы и Backspace к тексту.
func (t *TextField) applyInput(chars []rune, backspaces int) {
	for _, r := range chars {
		if r < 32 || r == 127 || (t.Allowed != nil && !t.Allowed(r)) {
			continue
		}
		if t.Max == 0 || utf8.RuneCountInString(t.Text) < t.Max {
			t.Text += string(r)
		}
	}
	for ; backspaces > 0 && len(t.Text) > 0; backspaces-- {
		_, n := utf8.DecodeLastRuneInString(t.Text)
		t.Text = t.Text[:len(t.Text)-n]
	}
}

// collectTextInput вызывается из Update раз за тик: Draw может идти чаще
// (монитор 120/144 Гц), и ввод, прочитанный в Draw, дублировался бы.
func (in *input) collectTextInput() {
	in.chars = ebiten.AppendInputChars(in.chars)
	if repeatKey(ebiten.KeyBackspace) {
		in.backspace++
	}
}

// Draw рисует поле и обрабатывает ввод.
func (t *TextField) Draw(u *UI, x, y, w, h int) {
	if u.clicked(x, y, w, h) {
		t.Focused = true
	} else if u.in.click && !u.mouseIn(x, y, w, h) {
		t.Focused = false
	}
	if t.Focused {
		t.applyInput(u.in.chars, u.in.backspace)
		u.in.chars, u.in.backspace = nil, 0
	}
	if u.on {
		rc := [3]float32{1, 1, 1}
		rim := float32(0.3)
		if t.Focused {
			rc, rim = [3]float32{0.92, 0.66, 0.24}, 0.95
		}
		u.pill(float32(x), float32(y), float32(w), float32(h), 10, pillDark, rim, rc, 0)
	} else {
		fillRect(u.screen, float64(x), float64(y), float64(w), float64(h), color.RGBA{14, 16, 20, 255})
		bc := colBorder
		if t.Focused {
			bc = colAccent
		}
		strokeRect(u.screen, float64(x), float64(y), float64(w), float64(h), bc, 1)
	}
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
	callStats.tri++
	r, g, b, a := c.RGBA()
	cr, cg, cb, ca := float32(r)/0xffff, float32(g)/0xffff, float32(b)/0xffff, float32(a)/0xffff
	vs := []ebiten.Vertex{
		{DstX: px(x1), DstY: px(y1), SrcX: 1, SrcY: 1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		{DstX: px(x2), DstY: px(y2), SrcX: 1, SrcY: 1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		{DstX: px(x3), DstY: px(y3), SrcX: 1, SrcY: 1, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
	}
	op := &ebiten.DrawTrianglesOptions{ColorScaleMode: ebiten.ColorScaleModePremultipliedAlpha}
	dst.DrawTriangles32(vs, []uint32{0, 1, 2}, whitePix, op)
}

// plate — плашка для уведомлений и подсказок: тёмное стекло с цветной кромкой (в плоском стиле — рамка).
func (u *UI) plate(x, y, w, h float64, flat, rim color.RGBA) {
	auditRect(x, y, w, h)
	if u.on {
		u.pill(float32(x), float32(y), float32(w), float32(h), 14, [4]float32{0.04, 0.055, 0.08, 0.88},
			0.9, [3]float32{float32(rim.R) / 255, float32(rim.G) / 255, float32(rim.B) / 255}, 0.4)
		return
	}
	fillRect(u.screen, x, y, w, h, flat)
	strokeRect(u.screen, x, y, w, h, rim, 1)
}

// card — карточка внутри панели: лёгкая светлая плашка (в плоском стиле — тёмная заливка).
func (u *UI) card(x, y, w, h float64, rim color.RGBA, strong float32) {
	auditRect(x, y, w, h)
	if u.on {
		u.pill(float32(x), float32(y), float32(w), float32(h), 16, [4]float32{1, 1, 1, 0.06 + 0.04*strong},
			0.35+0.6*strong, [3]float32{float32(rim.R) / 255, float32(rim.G) / 255, float32(rim.B) / 255}, 1)
		return
	}
	fillRect(u.screen, x, y, w, h, color.RGBA{30, 35, 42, 255})
	strokeRect(u.screen, x, y, w, h, rim, 1)
}

// rowHi — подсветка строки списка под курсором.
func (u *UI) rowHi(x, y, w, h float64) {
	fillRect(u.screen, x, y, w, h, color.RGBA{34, 34, 34, 34})
}

// Slider — горизонтальный ползунок 0…1; возвращает новое значение и признак изменения.
func (u *UI) Slider(id string, x, y, w, h int, v float64) (float64, bool) {
	const pad = 10 // от края до центра ручки
	changed := false
	if u.in.click && !u.in.consumed && u.mouseIn(x-4, y, w+8, h) {
		u.in.consumed = true
		u.drag = id
	}
	if u.drag == id {
		if u.in.down {
			nv := math.Max(0, math.Min(1, (float64(u.in.mx-x-pad))/float64(w-2*pad)))
			if nv != v {
				v, changed = nv, true
			}
		} else {
			u.drag = ""
		}
	}
	ty := float64(y + h/2)
	kx := float64(x+pad) + v*float64(w-2*pad)
	// дорожка и заполненная часть
	if u.on {
		u.pill(float32(x), float32(ty-3), float32(w), 6, 3, [4]float32{1, 1, 1, 0.16}, 0.3, [3]float32{1, 1, 1}, 0)
		u.pill(float32(x), float32(ty-3), float32(kx-float64(x)), 6, 3, pillOn, 0.3, [3]float32{1, 1, 1}, 0)
	} else {
		fillRect(u.screen, float64(x), ty-3, float64(w), 6, colButton)
		fillRect(u.screen, float64(x), ty-3, kx-float64(x), 6, colButtonOn)
	}
	r := 8.0
	if u.drag == id || u.mouseIn(x, y, w, h) {
		r = 9
	}
	disc(u.screen, kx, ty, r, color.RGBA{245, 240, 225, 255})
	circle(u.screen, kx, ty, r, color.RGBA{0, 0, 0, 90}, 1)
	return v, changed
}

// fitRow подбирает один кегль для подписей ряда кнопок шириной w каждая — чтобы в ряду не было букв разного размера.
// Результат нужно присвоить u.rowSize на время рисования ряда и сбросить в 0.
func fitRow(labels []string, w int, h int) float64 {
	size := 14.0
	if h < 22 {
		size = 12
	}
	for size > 10 {
		ok := true
		for _, l := range labels {
			if textWidth(l, size) > float64(w-8) {
				ok = false
				break
			}
		}
		if ok {
			break
		}
		size -= 0.5
	}
	return size
}
