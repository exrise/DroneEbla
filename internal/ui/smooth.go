package ui

import (
	"math"
	"time"

	"github.com/exrise/droneebla/internal/sim"
)

// Плавное движение: представление приходит 5–10 раз в секунду, поэтому объекты на карте двигаются рывками.
// Между двумя последними представлениями позиции юнитов и боеприпасов интерполируются по времени кадра.

type smoothKey struct {
	proj bool
	id   uint32
}

type smoothPos struct{ px, py, cx, cy float64 }

type smoother struct {
	pos    map[smoothKey]*smoothPos
	last   *sim.View
	at     time.Time
	period float64 // сек между последними двумя представлениями
}

// maxSmoothKm — больший скачок за обновление показывается без интерполяции (телепорт, перерасстановка).
const maxSmoothKm = 400.0

// update вызывается раз в кадр: при новом представлении сдвигает текущие позиции в «прошлые».
func (s *smoother) update(v *sim.View, now time.Time) {
	if v == s.last {
		return
	}
	if s.pos == nil || v.Time < s.lastTime() {
		s.pos = map[smoothKey]*smoothPos{} // новая партия или загрузка: без интерполяции
	}
	if dt := now.Sub(s.at).Seconds(); !s.at.IsZero() {
		s.period = math.Max(0.05, math.Min(0.4, dt))
	} else {
		s.period = 0.2
	}
	next := make(map[smoothKey]*smoothPos, len(s.pos))
	put := func(k smoothKey, x, y float64) {
		p := s.pos[k]
		if p == nil {
			next[k] = &smoothPos{x, y, x, y}
			return
		}
		np := &smoothPos{px: p.cx, py: p.cy, cx: x, cy: y}
		if math.Hypot(np.cx-np.px, np.cy-np.py) > maxSmoothKm {
			np.px, np.py = x, y
		}
		next[k] = np
	}
	for _, u := range v.Units {
		put(smoothKey{false, u.ID}, u.X, u.Y)
	}
	for _, p := range v.Projs {
		put(smoothKey{true, p.ID}, p.X, p.Y)
	}
	s.pos, s.last, s.at = next, v, now
}

func (s *smoother) lastTime() float64 {
	if s.last == nil {
		return 0
	}
	return s.last.Time
}

// at возвращает показываемую позицию объекта; неизвестный объект — как есть.
func (s *smoother) get(proj bool, id uint32, x, y float64, now time.Time) (float64, float64) {
	p := s.pos[smoothKey{proj, id}]
	if p == nil {
		return x, y
	}
	f := math.Max(0, math.Min(1, now.Sub(s.at).Seconds()/s.period))
	return p.px + (p.cx-p.px)*f, p.py + (p.cy-p.py)*f
}
