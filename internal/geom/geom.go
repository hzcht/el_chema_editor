// Package geom — минимальная 2D-геометрия редактора.
// Чистый Go, никаких GUI-зависимостей (правило §3/§13.4: ядро не знает про gio).
package geom

import "math"

// Vec — точка/вектор (мир или экран). Y растёт вниз.
type Vec struct{ X, Y float32 }

func (v Vec) Add(o Vec) Vec       { return Vec{v.X + o.X, v.Y + o.Y} }
func (v Vec) Sub(o Vec) Vec       { return Vec{v.X - o.X, v.Y - o.Y} }
func (v Vec) Scale(f float32) Vec { return Vec{v.X * f, v.Y * f} }

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

const eps float32 = 1e-4

func nearf(a, b float32) bool { return absf(a-b) <= eps }
func near(a, b Vec) bool      { return nearf(a.X, b.X) && nearf(a.Y, b.Y) }

// Rect — ось-выровненный прямоугольник (Min ⊂ Max, полузакрытый).
type Rect struct{ Min, Max Vec }

func NewRect(x, y, w, h float32) Rect { return Rect{Vec{x, y}, Vec{x + w, y + h}} }
func (r Rect) Size() Vec              { return r.Max.Sub(r.Min) }
func (r Rect) Contains(p Vec) bool {
	return r.Min.X <= p.X && p.X <= r.Max.X && r.Min.Y <= p.Y && p.Y <= r.Max.Y
}
func (r Rect) Overlaps(o Rect) bool {
	return r.Min.X < o.Max.X && o.Min.X < r.Max.X &&
		r.Min.Y < o.Max.Y && o.Min.Y < r.Max.Y
}

// Snap округляет координату к кратной сетке.
func Snap(v, step float32) float32 {
	return float32(math.Round(float64(v/step))) * step
}

// Side — сторона прямоугольника. Normal — наружная нормаль (Y вниз).
type Side uint8

const (
	SideNone Side = iota
	SideLeft
	SideTop
	SideRight
	SideBottom
)

func (s Side) Normal() Vec {
	switch s {
	case SideLeft:
		return Vec{-1, 0}
	case SideRight:
		return Vec{1, 0}
	case SideTop:
		return Vec{0, -1}
	case SideBottom:
		return Vec{0, 1}
	}
	return Vec{}
}

// Opposite — противолежащие стороны (L↔R или T↔B).
func Opposite(a, b Side) bool {
	return (a == SideLeft && b == SideRight) || (a == SideRight && b == SideLeft) ||
		(a == SideTop && b == SideBottom) || (a == SideBottom && b == SideTop)
}

// --- Камера ----------------------------------------------------------------

const (
	// Границы зума в px/мм: при pxPerMM=4 это 10%…800% (§13.3).
	// Старые 0.1–8 «px-домена» давали 2.5%…200% — сцена схлопывалась в точку
	// и «уезжала» (отзыв от прогона).
	ZoomMin = float32(0.4)
	ZoomMax = float32(32.0)
)

// Camera: screen = world*zoom + pan.
type Camera struct {
	Zoom float32
	Pan  Vec
}

func (c *Camera) WorldToScreen(p Vec) Vec { return p.Scale(c.Zoom).Add(c.Pan) }
func (c *Camera) ScreenToWorld(p Vec) Vec { return p.Sub(c.Pan).Scale(1 / c.Zoom) }

// ZoomAt — зум к точке экрана (мировая точка под курсором остаётся под ним).
func (c *Camera) ZoomAt(screen Vec, factor float32) {
	world := c.ScreenToWorld(screen)
	c.Zoom = clampZoom(c.Zoom * factor)
	c.Pan = screen.Sub(world.Scale(c.Zoom))
}

// ClampZoom — общедоступный кламп (приложение им же ограничивает fit).
func ClampZoom(z float32) float32 { return clampZoom(z) }

func clampZoom(z float32) float32 {
	if z < ZoomMin {
		return ZoomMin
	}
	if z > ZoomMax {
		return ZoomMax
	}
	return z
}

// --- Пересечения осевых отрезков -------------------------------------------

// SegCross — точка пересечения двух ось-выровненных отрезков (H2V/V2H).
// Возвращает false для параллельных, вырожденных или касающихся
// только на концах (те — узлы, а не пересечения проводников).
func SegCross(a1, a2, b1, b2 Vec) (Vec, bool) {
	ah := axisSeg(a1, a2)
	bh := axisSeg(b1, b2)
	if ah == nil || bh == nil {
		return Vec{}, false // вырожденные (нулевые) отрезки
	}
	if *ah == *bh {
		return Vec{}, false // параллельные (или коллинеарные поверх)
	}
	// горизонтальный a × вертикальный b (или наоборот)
	hl, vl := [2]Vec{a1, a2}, [2]Vec{b1, b2}
	if !*ah {
		hl, vl = [2]Vec{b1, b2}, [2]Vec{a1, a2}
	}
	hx := lo(hl[0].X, hl[1].X)-eps <= vl[0].X && vl[0].X <= hi(hl[0].X, hl[1].X)+eps
	vy := lo(vl[0].Y, vl[1].Y)-eps <= hl[0].Y && hl[0].Y <= hi(vl[0].Y, vl[1].Y)+eps
	p := Vec{X: vl[0].X, Y: hl[0].Y}
	// касание на конце = совпадающие точки/узлы, арку не строим
	if !hx || !vy || near(p, hl[0]) || near(p, hl[1]) || near(p, vl[0]) || near(p, vl[1]) {
		return Vec{}, false
	}
	return p, true
}

// axisSeg: nil — вырожденный; true — горизонтальный, false — вертикальный.
func axisSeg(a, b Vec) *bool {
	ha := nearf(a.Y, b.Y)
	va := nearf(a.X, b.X)
	switch {
	case ha && !va:
		h := true
		return &h
	case va && !ha:
		h := false
		return &h
	}
	return nil
}

func lo(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func hi(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// --- Маршрутизация ---------------------------------------------------------

// Route строит ортогональный маршрут pin→pin с отступом наружу от каждой
// стороны (out): 0–2 колена, ни один сегмент не наклонный. Стороны SideNone
// (резиновая скрутка) сведутся к L-колену по x.
func Route(p1 Vec, s1 Side, p2 Vec, s2 Side, out float32) []Vec {
	if s1 == SideNone || s2 == SideNone {
		return Clean([]Vec{p1, {X: p2.X, Y: p1.Y}, p2})
	}
	e1 := p1.Add(s1.Normal().Scale(out))
	e2 := p2.Add(s2.Normal().Scale(out))
	return Clean(pathFor(p1, s1, e1, p2, s2, e2))
}

func pathFor(p1 Vec, s1 Side, e1 Vec, p2 Vec, s2 Side, e2 Vec) []Vec {
	h1 := s1 == SideLeft || s1 == SideRight
	h2 := s2 == SideLeft || s2 == SideRight

	switch {
	case h1 && h2:
		if Opposite(s1, s2) {
			forward := (s1 == SideRight && e1.X <= e2.X) ||
				(s1 == SideLeft && e1.X >= e2.X)
			if forward && nearf(e1.Y, e2.Y) {
				return []Vec{p1, p2} // одна прямая
			}
			if forward {
				kx := (e1.X + e2.X) / 2
				return []Vec{p1, e1, {kx, e1.Y}, {kx, e2.Y}, e2, p2}
			}
			// направление «назад» — вертикальный обход
			my := (e1.Y + e2.Y) / 2
			return []Vec{p1, e1, {e1.X, my}, {e2.X, my}, e2, p2}
		}
		// та же сторона — обход по среднему x
		mx := (e1.X + e2.X) / 2
		return []Vec{p1, e1, {mx, e1.Y}, {mx, e2.Y}, e2, p2}

	case !h1 && !h2:
		if Opposite(s1, s2) {
			forward := (s1 == SideBottom && e1.Y <= e2.Y) ||
				(s1 == SideTop && e1.Y >= e2.Y)
			if forward && nearf(e1.X, e2.X) {
				return []Vec{p1, p2}
			}
			if forward {
				ky := (e1.Y + e2.Y) / 2
				return []Vec{p1, e1, {e1.X, ky}, {e2.X, ky}, e2, p2}
			}
			// направление «назад» — горизонтальный обход
			mx := (e1.X + e2.X) / 2
			return []Vec{p1, e1, {mx, e1.Y}, {mx, e2.Y}, e2, p2}
		}
		// та же сторона — обход по среднему y
		my := (e1.Y + e2.Y) / 2
		return []Vec{p1, e1, {e1.X, my}, {e2.X, my}, e2, p2}

	case h1: // из горизонтали в вертикаль
		K := Vec{e1.X, e2.Y}
		return []Vec{p1, e1, K, e2, p2}

	default: // из вертикали в горизонталь
		K := Vec{e2.X, e1.Y}
		return []Vec{p1, e1, K, e2, p2}
	}
}

// Clean убирает дубли подряд и коллинеарные промежуточные точки,
// сохраняя точки разворота (hairpin).
func Clean(pts []Vec) []Vec {
	res := make([]Vec, 0, len(pts))
	for _, p := range pts {
		if len(res) == 0 || !near(res[len(res)-1], p) {
			res = append(res, p)
		}
	}
	i := 1
	for i < len(res)-1 {
		a, b, c := res[i-1], res[i], res[i+1]
		col := (nearf(a.X, b.X) && nearf(b.X, c.X) && (c.Y-b.Y)*(b.Y-a.Y) > 0) ||
			(nearf(a.Y, b.Y) && nearf(b.Y, c.Y) && (c.X-b.X)*(b.X-a.X) > 0)
		if col {
			// коллинеарные подряд и монотонно — схлопываем; точки разворота
			// (hairpin) не трогаем
			res = append(res[:i], res[i+1:]...)
			continue
		}
		i++
	}
	return res
}
