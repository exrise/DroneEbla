package ui

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Стиль «жидкое стекло». Панели рисуются шейдером: под ними размытая копия уже нарисованной
// сцены (карта или фон меню), у краёв — преломление с лёгкой дисперсией, тёмная подкраска для
// читаемости текста, блик по кромке и тень. Кнопки, вкладки, поля и полосы — лёгкие «таблетки»
// без размытия (только заливка и кромка), чтобы не нагружать видеокарту.
// Включается клавишей F9 или кнопкой в главном меню; без шейдеров интерфейс плоский.

const blurKage = `//kage:unit pixels
package main

var Dir vec2

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	o := imageSrc0Origin()
	s := imageSrc0Size()
	lo := o + vec2(0.5)
	hi := o + s - vec2(0.5)
	c := imageSrc0At(clamp(src, lo, hi)) * 0.227027
	c += imageSrc0At(clamp(src+Dir*1.3846153846, lo, hi)) * 0.3162162162
	c += imageSrc0At(clamp(src-Dir*1.3846153846, lo, hi)) * 0.3162162162
	c += imageSrc0At(clamp(src+Dir*3.2307692308, lo, hi)) * 0.0702702703
	c += imageSrc0At(clamp(src-Dir*3.2307692308, lo, hi)) * 0.0702702703
	return c
}
`

const glassKage = `//kage:unit pixels
package main

var Rect vec4
var Radius float
var Tint vec4
var Scale float

//SD

func bg(pos vec2) vec3 {
	o := imageSrc0Origin()
	s := imageSrc0Size()
	p := clamp(o+pos*0.5, o+vec2(0.5), o+s-vec2(0.5))
	return imageSrc0At(p).rgb
}

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	// Вся геометрия — в логических единицах интерфейса, а сглаживание края — в физических пикселях.
	lp := dst.xy / Scale
	half := Rect.zw * 0.5
	p := lp - (Rect.xy + half)
	d0 := sdSq(p, half, Radius)
	e := 1.0
	n := vec2(
		sdSq(p+vec2(e, 0), half, Radius)-sdSq(p-vec2(e, 0), half, Radius),
		sdSq(p+vec2(0, e), half, Radius)-sdSq(p-vec2(0, e), half, Radius))
	d := d0 / max(length(n)*0.5, 0.35)
	n = n / (length(n) + 0.0001)
	shadow := (1 - smoothstep(0, 16, d)) * 0.30
	if d > 0 {
		return vec4(0, 0, 0, shadow) * step(0.001, shadow)
	}
	depth := -d
	edge := 1 - smoothstep(0, 26, depth)
	k := edge * edge * 22
	col := vec3(bg(lp-n*k*1.00).r, bg(lp-n*k*1.12).g, bg(lp-n*k*1.25).b)
	col = mix(col, Tint.rgb, Tint.a)
	col += vec3(0.025) * (1 - clamp((lp.y-Rect.y)/Rect.w, 0, 1))
	col += vec3(0.06) * (1 - smoothstep(0, 10, depth))
	l := 0.5 + 0.5*dot(n, normalize(vec2(-0.55, -0.85)))
	rim := (1 - smoothstep(0, 2.2, depth)) * (0.22 + 0.6*l)
	col += vec3(rim)
	aa := clamp(0.5+depth*Scale, 0, 1)
	return vec4(col*aa, aa) + vec4(0, 0, 0, shadow)*(1-aa)
}
`

// Лёгкая «таблетка» без выборки фона.
const pillKage = `//kage:unit pixels
package main

var Rect vec4
var Radius float
var Fill vec4
var RimCol vec3
var Rim float
var Light float
var Scale float

//SD

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	lp := dst.xy / Scale
	half := Rect.zw * 0.5
	p := lp - (Rect.xy + half)
	d0 := sdSq(p, half, Radius)
	e := 1.0
	n := vec2(
		sdSq(p+vec2(e, 0), half, Radius)-sdSq(p-vec2(e, 0), half, Radius),
		sdSq(p+vec2(0, e), half, Radius)-sdSq(p-vec2(0, e), half, Radius))
	d := d0 / max(length(n)*0.5, 0.35)
	n = n / (length(n) + 0.0001)
	aa := clamp(0.5-d*Scale, 0, 1)
	if aa <= 0 {
		return vec4(0)
	}
	depth := -d
	g := 1 - clamp((lp.y-Rect.y)/Rect.w, 0, 1)
	col := Fill.rgb + vec3(0.10)*g*Light
	a := Fill.a + 0.06*g*Light
	l := 0.5 + 0.5*dot(n, normalize(vec2(-0.5, -0.85)))
	r := (1 - smoothstep(0, 1.4, depth)) * Rim * (0.35 + 0.65*l)
	col = mix(col, RimCol, clamp(r, 0, 1))
	a = clamp(a+r*0.7, 0, 1)
	return vec4(col*a*aa, a*aa)
}
`

// Анимированный фон меню: тёмный градиент, слева флаг Украины, справа флаг
// России. Полотнища слегка колышутся и растворяются к центру и краям экрана.
const backdropKage = `//kage:unit pixels
package main

var Time float
var Size vec2

// wave — смещение полотна по вертикали и освещённость складок.
func wave(x float, y float, ph float) vec2 {
	a := 9.0*x - Time*1.1 + ph
	b := 5.0*x + 3.0*y - Time*0.7 + ph*1.7
	d := 0.022*sin(a) + 0.012*sin(b)
	sh := 0.20*cos(a) + 0.10*cos(b)
	return vec2(d, sh)
}

// band — 1 внутри полосы [a, b) с мягкими краями.
func band(y float, a float, b float, e float) float {
	return smoothstep(a-e, a+e, y) * (1 - smoothstep(b-e, b+e, y))
}

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	uv := dst.xy / Size
	base := mix(vec3(0.045, 0.06, 0.095), vec3(0.085, 0.11, 0.17), uv.y)
	e := 1.5 / Size.y
	// Вертикальное затухание у верхнего и нижнего края.
	vfade := smoothstep(0.0, 0.22, uv.y) * (1 - smoothstep(0.78, 1.0, uv.y))

	// Украина: синий над жёлтым.
	wl := wave(uv.x, uv.y, 0)
	yl := uv.y + wl.x
	ua := vec3(0.0, 0.34, 0.72)*band(yl, -1, 0.5, e) + vec3(1.0, 0.80, 0.0)*band(yl, 0.5, 2, e)
	al := (1 - smoothstep(0.10, 0.46, uv.x)) * vfade * 0.50
	col := mix(base, ua*(0.85+wl.y), al)

	// Россия: белый, синий, красный.
	wr := wave(1-uv.x, uv.y, 2.1)
	yr := uv.y + wr.x
	ru := vec3(0.90, 0.92, 0.95)*band(yr, -1, 1.0/3.0, e) + vec3(0.0, 0.22, 0.65)*band(yr, 1.0/3.0, 2.0/3.0, e) + vec3(0.84, 0.17, 0.12)*band(yr, 2.0/3.0, 2, e)
	ar := smoothstep(0.54, 0.90, uv.x) * vfade * 0.50
	col = mix(col, ru*(0.85+wr.y), ar)
	return vec4(col, 1)
}
`

// Непрерывные («яблочные») скругления: в зоне угла расстояние считается по p-норме
// (суперэллипс), поэтому кривизна нарастает плавно и угол не похож на дугу окружности.
const (
	squircleN     = "4.0" // показатель суперэллипса (у Apple угол плавно разгоняется на ~1.5 радиуса)
	squircleScale = "1.6" // эффективный радиус: на диагонали суперэллипс «срезан», компенсируем
)

const sdSquircleKage = `func sdSq(p vec2, b vec2, r float) float {
	R := min(r*` + squircleScale + `, min(b.x, b.y))
	q := abs(p) - b + vec2(R)
	qp := max(q, vec2(0.00001))
	corner := pow(pow(qp.x, ` + squircleN + `)+pow(qp.y, ` + squircleN + `), 1.0/` + squircleN + `)
	return corner + min(max(q.x, q.y), 0.0) - R
}
`

// withSquircle подставляет общую функцию расстояния в текст шейдера.
func withSquircle(src string) []byte {
	return []byte(strings.Replace(src, "//SD\n", sdSquircleKage, 1))
}

// glassFX — ресурсы эффекта.
type glassFX struct {
	blur, glass, pill, backdrop *ebiten.Shader
	small, tmp                  *ebiten.Image
	mid                         []*ebiten.Image
	w, h                        int
}

// glassKey переключает стиль по F9.
func (g *Game) glassKey() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF9) {
		g.setGlass(!g.glass)
	}
}

// fxInit компилирует шейдеры при первом включении стекла; при ошибке стекло отключается.
func (g *Game) fxInit() {
	u := &g.ui
	u.on = false
	if !g.glass || g.fxFailed {
		return
	}
	if u.fx == nil {
		fx := &glassFX{}
		var errs [4]error
		fx.blur, errs[0] = ebiten.NewShader([]byte(blurKage))
		fx.glass, errs[1] = ebiten.NewShader(withSquircle(glassKage))
		fx.pill, errs[2] = ebiten.NewShader(withSquircle(pillKage))
		fx.backdrop, errs[3] = ebiten.NewShader([]byte(backdropKage))
		for _, e := range errs {
			if e != nil {
				g.fxFailed, g.glass = true, false
				g.toast("Стиль «стекло» не поддерживается этой видеокартой — включён обычный")
				return
			}
		}
		u.fx = fx
	}
	u.on = true
}

// drawBackdrop рисует анимированный фон экранов без карты.
func (g *Game) drawBackdrop(screen *ebiten.Image) {
	u := &g.ui
	if !u.on {
		return
	}
	b := screen.Bounds()
	op := &ebiten.DrawRectShaderOptions{}
	op.Uniforms = map[string]any{
		"Time": float32(u.in.tick) / 60,
		"Size": []float32{float32(b.Dx()), float32(b.Dy())},
	}
	screen.DrawRectShader(b.Dx(), b.Dy(), u.fx.backdrop, op)
}

// glassPrep готовит размытую копию уже нарисованной сцены; вызывается до панелей.
func (g *Game) glassPrep(screen *ebiten.Image) {
	u := &g.ui
	if !u.on {
		return
	}
	fx := u.fx
	b := screen.Bounds()
	// Размытая копия — половина логического размера экрана: радиус размытия не зависит от разрешения.
	w, h := int(float64(b.Dx())/rs/2), int(float64(b.Dy())/rs/2)
	if fx.small == nil || fx.w != w || fx.h != h {
		fx.small, fx.tmp = ebiten.NewImage(w, h), ebiten.NewImage(w, h)
		fx.w, fx.h = w, h
	}
	fx.small.Clear()
	fx.downsample(screen, b.Dx(), b.Dy())
	for i := 0; i < 2; i++ {
		fx.pass(fx.tmp, fx.small, 2.2, 0)
		fx.pass(fx.small, fx.tmp, 0, 2.2)
	}
}

// downsample сжимает кадр до fx.small по половине за шаг: одним билинейным шагом из 4K
// получились бы пропуски пикселей и мерцание размытия при движении карты.
func (fx *glassFX) downsample(screen *ebiten.Image, sw, sh int) {
	src, w, h := screen, sw, sh
	for i := 0; w >= fx.w*4; i++ {
		w, h = w/2, h/2
		if i >= len(fx.mid) {
			fx.mid = append(fx.mid, nil)
		}
		if fx.mid[i] == nil || fx.mid[i].Bounds().Dx() != w || fx.mid[i].Bounds().Dy() != h {
			if fx.mid[i] != nil {
				fx.mid[i].Deallocate()
			}
			fx.mid[i] = ebiten.NewImage(w, h) // промежуточные картинки живут между кадрами
		}
		step := fx.mid[i]
		step.Clear()
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Scale(0.5, 0.5)
		step.DrawImage(src, op)
		src = step
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(float64(fx.w)/float64(w), float64(fx.h)/float64(h))
	fx.small.DrawImage(src, op)
}

func (fx *glassFX) pass(dst, src *ebiten.Image, dx, dy float32) {
	dst.Clear()
	op := &ebiten.DrawRectShaderOptions{}
	op.Images[0] = src
	op.Uniforms = map[string]any{"Dir": []float32{dx, dy}}
	dst.DrawRectShader(fx.w, fx.h, fx.blur, op)
}

var quadIdx = []uint16{0, 1, 2, 1, 2, 3}

func quad(x0, y0, x1, y1 float32) []ebiten.Vertex {
	return []ebiten.Vertex{
		{DstX: x0, DstY: y0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y0, SrcX: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x0, DstY: y1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y1, SrcX: 1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
}

// glassPanel рисует стеклянную панель; tintA — плотность тёмной подкраски (0.45…0.9).
func (u *UI) glassPanel(x, y, w, h, r, tintA float32) {
	fx := u.fx
	if fx == nil || fx.small == nil {
		return
	}
	const m = 18 // запас под тень
	if u.glassK > 0 {
		// Нижняя граница плотности: на самой прозрачной настройке текст всё равно читается (контраст не ниже ~4,5:1).
		tintA = min(0.97, max(0.5, tintA*u.glassK))
	}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = fx.small
	op.Uniforms = map[string]any{
		"Rect":   []float32{x, y, w, h},
		"Radius": r,
		"Tint":   []float32{0.06, 0.08, 0.11, tintA},
		"Scale":  float32(rs),
	}
	k := float32(rs)
	u.screen.DrawTrianglesShader(quad((x-m)*k, (y-m)*k, (x+w+m)*k, (y+h+m)*k), quadIdx, fx.glass, op)
}

// Заливки «таблеток».
var (
	pillNormal = [4]float32{1, 1, 1, 0.10}
	pillHover  = [4]float32{1, 1, 1, 0.19}
	pillOn     = [4]float32{0.45, 0.62, 0.92, 0.55}
	pillOff    = [4]float32{1, 1, 1, 0.04}
	pillDark   = [4]float32{0.02, 0.03, 0.05, 0.55}
)

// pill рисует лёгкую «таблетку»: fill — заливка (rgba), rim — сила кромки, light — верхний блик.
func (u *UI) pill(x, y, w, h, r float32, fill [4]float32, rim float32, rimCol [3]float32, light float32) {
	fx := u.fx
	if fx == nil {
		return
	}
	if r > h/2 {
		r = h / 2
	}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Uniforms = map[string]any{
		"Rect":   []float32{x, y, w, h},
		"Radius": r,
		"Fill":   fill[:],
		"RimCol": rimCol[:],
		"Rim":    rim,
		"Light":  light,
		"Scale":  float32(rs),
	}
	k := float32(rs)
	u.screen.DrawTrianglesShader(quad(x*k-1, y*k-1, (x+w)*k+1, (y+h)*k+1), quadIdx, fx.pill, op)
}
