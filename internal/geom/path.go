package geom

import "math"

// Работа с готовым маршрутом как с кривой: точка на доле длины и поиск
// сегмента под курсором (T-соединения, идея 1.3).
// Разбор — docs/ALGORITHMS.md §3.1.

// PathLen — суммарная длина ломаной.
func PathLen(pts []Vec) float32 {
	var sum float32
	for i := 1; i < len(pts); i++ {
		sum += absf(pts[i].X-pts[i-1].X) + absf(pts[i].Y-pts[i-1].Y)
	}
	return sum
}

// PathAt — точка на доле t (0..1) длины пути и единичное направление
// сегмента, на котором она лежит. t вне диапазона прижимается к концам.
// Ось-выровненность не проверяется: маршруты редактора всегда ортогональны.
func PathAt(pts []Vec, t float32) (p, dir Vec, ok bool) {
	if len(pts) == 0 {
		return Vec{}, Vec{}, false
	}
	if len(pts) == 1 {
		return pts[0], Vec{}, true
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	total := PathLen(pts)
	if total <= eps {
		return pts[0], Vec{}, true
	}
	rest := t * total
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		l := absf(b.X-a.X) + absf(b.Y-a.Y)
		if l <= eps {
			continue
		}
		if rest <= l || i == len(pts)-1 {
			f := rest / l
			if f > 1 {
				f = 1
			}
			dir = Vec{X: (b.X - a.X) / l, Y: (b.Y - a.Y) / l}
			return Vec{X: a.X + (b.X-a.X)*f, Y: a.Y + (b.Y-a.Y)*f}, dir, true
		}
		rest -= l
	}
	return pts[len(pts)-1], Vec{}, true
}

// PathFraction — доля длины пути, ближайшая к точке p: обратная к PathAt.
// Возвращает долю и расстояние до пути (точка может быть мимо него).
func PathFraction(pts []Vec, p Vec) (t, dist float32) {
	total := PathLen(pts)
	if total <= eps {
		if len(pts) > 0 {
			return 0, distVec(pts[0], p)
		}
		return 0, 0
	}
	best, bestD := float32(0), float32(1e30)
	var run float32
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		l := absf(b.X-a.X) + absf(b.Y-a.Y)
		if l <= eps {
			continue
		}
		d, f := SegPointDist(a, b, p)
		if d < bestD {
			bestD = d
			best = (run + f*l) / total
		}
		run += l
	}
	return best, bestD
}

// SegPointDist — расстояние от точки до осевого отрезка a-b и доля f (0..1)
// ближайшей точки вдоль него. Для неортогональных отрезков считается по
// проекции — редактор таких не строит, но формула от этого не ломается.
func SegPointDist(a, b, p Vec) (dist, f float32) {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 <= eps*eps {
		return distVec(a, p), 0
	}
	f = ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l2
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	q := Vec{X: a.X + dx*f, Y: a.Y + dy*f}
	return distVec(q, p), f
}

func distVec(a, b Vec) float32 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}
