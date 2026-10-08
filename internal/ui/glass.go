package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Стиль «жидкое стекло»: карта размывается в уменьшенной копии, панель рисуется шейдером,
// который берёт размытый фон, преломляет его у краёв, подкрашивает и добавляет блик по кромке.
// Включается клавишей F9 или кнопкой в главном меню; без шейдеров интерфейс плоский, как раньше.

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

func sdBox(p vec2, b vec2, r float) float {
	q := abs(p) - b + vec2(r)
	return length(max(q, vec2(0))) + min(max(q.x, q.y), 0) - r
}

func bg(pos vec2) vec3 {
	o := imageSrc0Origin()
	s := imageSrc0Size()
	p := clamp(o+pos*0.5, o+vec2(0.5), o+s-vec2(0.5))
	return imageSrc0At(p).rgb
}

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	half := Rect.zw * 0.5
	p := dst.xy - (Rect.xy + half)
	d := sdBox(p, half, Radius)
	// Тень вокруг стекла.
	shadow := (1 - smoothstep(0, 16, d)) * 0.30
	if d > 0 {
		return vec4(0, 0, 0, shadow) * step(0.001, shadow)
	}
	depth := -d
	e := 1.0
	n := vec2(
		sdBox(p+vec2(e, 0), half, Radius)-sdBox(p-vec2(e, 0), half, Radius),
		sdBox(p+vec2(0, e), half, Radius)-sdBox(p-vec2(0, e), half, Radius))
	n = n / (length(n) + 0.0001)
	// Линза: у края видно содержимое глубже внутри, с лёгкой хроматической дисперсией.
	edge := 1 - smoothstep(0, 26, depth)
	k := edge * edge * 22
	col := vec3(bg(dst.xy-n*k*1.00).r, bg(dst.xy-n*k*1.12).g, bg(dst.xy-n*k*1.25).b)
	col = mix(col, Tint.rgb, Tint.a)
	// Мягкий вертикальный градиент и внутреннее свечение у края.
	col += vec3(0.025) * (1 - clamp((dst.y-Rect.y)/Rect.w, 0, 1))
	col += vec3(0.06) * (1 - smoothstep(0, 10, depth))
	// Блик по кромке: сильнее с левого верхнего края.
	l := 0.5 + 0.5*dot(n, normalize(vec2(-0.55, -0.85)))
	rim := (1 - smoothstep(0, 2.2, depth)) * (0.22 + 0.6*l)
	col += vec3(rim)
	aa := clamp(0.5+depth, 0, 1)
	return vec4(col*aa, aa) + vec4(0, 0, 0, shadow)*(1-aa)
}
`

// glassFX — ресурсы эффекта.
type glassFX struct {
	blur, glass *ebiten.Shader
	small, tmp  *ebiten.Image
	w, h        int
}

// glassOn — включён ли стиль «стекло» и готов ли эффект.
func (g *Game) glassOn() bool { return g.glass && g.fx != nil }

// glassKey переключает стиль по F9.
func (g *Game) glassKey() {
	if inpututil.IsKeyJustPressed(ebiten.KeyF9) {
		g.glass = !g.glass
	}
}

// glassPrep готовит размытую копию уже нарисованной карты; вызывается до панелей.
func (g *Game) glassPrep(screen *ebiten.Image) {
	if !g.glass {
		return
	}
	if g.fx == nil {
		if g.fxFailed {
			return
		}
		fx := &glassFX{}
		var err1, err2 error
		fx.blur, err1 = ebiten.NewShader([]byte(blurKage))
		fx.glass, err2 = ebiten.NewShader([]byte(glassKage))
		if err1 != nil || err2 != nil {
			g.fxFailed, g.glass = true, false
			g.toast("Стиль «стекло» не поддерживается этой видеокартой — включён обычный")
			return
		}
		g.fx = fx
	}
	fx := g.fx
	b := screen.Bounds()
	w, h := b.Dx()/2, b.Dy()/2
	if fx.small == nil || fx.w != w || fx.h != h {
		fx.small, fx.tmp = ebiten.NewImage(w, h), ebiten.NewImage(w, h)
		fx.w, fx.h = w, h
	}
	fx.small.Clear()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(0.5, 0.5)
	fx.small.DrawImage(screen, op)
	for i := 0; i < 2; i++ {
		fx.pass(fx.tmp, fx.small, 2.2, 0)
		fx.pass(fx.small, fx.tmp, 0, 2.2)
	}
}

func (fx *glassFX) pass(dst, src *ebiten.Image, dx, dy float32) {
	dst.Clear()
	op := &ebiten.DrawRectShaderOptions{}
	op.Images[0] = src
	op.Uniforms = map[string]any{"Dir": []float32{dx, dy}}
	dst.DrawRectShader(fx.w, fx.h, fx.blur, op)
}

// glassRect рисует стеклянную плашку (экранные координаты, радиус скругления r).
func (g *Game) glassRect(x, y, w, h, r float32) {
	fx := g.fx
	if fx == nil || fx.small == nil {
		return
	}
	const m = 18 // запас под тень
	x0, y0, x1, y1 := x-m, y-m, x+w+m, y+h+m
	vs := []ebiten.Vertex{
		{DstX: x0, DstY: y0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y0, SrcX: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x0, DstY: y1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y1, SrcX: 1, SrcY: 1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = fx.small
	op.Uniforms = map[string]any{
		"Rect":   []float32{x, y, w, h},
		"Radius": r,
		"Tint":   []float32{0.06, 0.08, 0.11, 0.62},
	}
	g.ui.screen.DrawTrianglesShader(vs, []uint16{0, 1, 2, 1, 2, 3}, fx.glass, op)
}
