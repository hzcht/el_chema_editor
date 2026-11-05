package geom

import (
	"math"
	"testing"
)

func ptEq(a, b Vec) bool { return near(a, b) }

// H × V внутри обоих отрезков.
func TestSegCrossHV(t *testing.T) {
	p, ok := SegCross(
		Vec{X: 100, Y: 100}, Vec{X: 200, Y: 100}, // горизонтальный
		Vec{X: 150, Y: 50}, Vec{X: 150, Y: 200}, // вертикальный
	)
	if !ok || !ptEq(p, Vec{X: 150, Y: 100}) {
		t.Fatalf("HV: %v %v", p, ok)
	}
	// порядок аргументов и направление не важны
	p, ok = SegCross(
		Vec{X: 200, Y: 100}, Vec{X: 100, Y: 100},
		Vec{X: 150, Y: 200}, Vec{X: 150, Y: 50},
	)
	if !ok || !ptEq(p, Vec{X: 150, Y: 100}) {
		t.Fatalf("HV перенаправление: %v %v", p, ok)
	}
}

// V × H внутри обоих.
func TestSegCrossVH(t *testing.T) {
	p, ok := SegCross(
		Vec{X: 100, Y: 50}, Vec{X: 100, Y: 200},
		Vec{X: 50, Y: 120}, Vec{X: 220, Y: 120},
	)
	if !ok || !ptEq(p, Vec{X: 100, Y: 120}) {
		t.Fatalf("VH: %v %v", p, ok)
	}
}

// Промах по диапазону — нет пересечения.
func TestSegCrossMiss(t *testing.T) {
	_, ok := SegCross(
		Vec{X: 100, Y: 100}, Vec{X: 200, Y: 100},
		Vec{X: 250, Y: 50}, Vec{X: 250, Y: 200},
	)
	if ok {
		t.Fatal("вертикаль вне диапазона не должна пересекать")
	}
}

// Пересечение ровно на конце = узел, не арка.
func TestSegCrossEndpoint(t *testing.T) {
	for _, c := range [][2]Vec{
		{Vec{X: 100, Y: 100}, Vec{X: 200, Y: 100}}, // заканчивается в x=100
	} {
		_, ok := SegCross(c[0], c[1], Vec{X: 100, Y: 50}, Vec{X: 100, Y: 200})
		if ok {
			t.Fatal("конец на конце — не пересечение")
		}
	}
}

// Параллельные и вырожденные.
func TestSegCrossDegenerate(t *testing.T) {
	if _, ok := SegCross(Vec{X: 0, Y: 0}, Vec{X: 10, Y: 0}, Vec{X: 5, Y: 10}, Vec{X: 5, Y: 20}); ok {
		t.Fatal("паралл (H,H) не должен пересекаться")
	}
	if _, ok := SegCross(Vec{X: 0, Y: 0}, Vec{X: 10, Y: 0}, Vec{X: 5, Y: 5}, Vec{X: 5, Y: 5}); ok {
		t.Fatal("точка — вырожденный отрезок")
	}
}

// Clean схлопывает монотонные коллинеарные, но не трогает развороты.
func TestCleanHairpin(t *testing.T) {
	pts := Clean([]Vec{
		Vec{X: 0, Y: 0},
		Vec{X: 100, Y: 0},
		Vec{X: 100, Y: 50},
		Vec{X: 100, Y: 20}, // разворот (hairpin) — должен остаться
	})
	if len(pts) != 4 {
		t.Fatalf("hairpin съеден: %v", pts)
	}
}

// SegCrossRect: горизонтальный отрезок заходит в прямоугольник слева.
func TestSegCrossRectH(t *testing.T) {
	r := Rect{Min: Vec{X: 40, Y: 40}, Max: Vec{X: 60, Y: 60}}
	pt, ok := SegCrossRect(Vec{X: 0, Y: 50}, Vec{X: 100, Y: 50}, r)
	if !ok || !near(pt, Vec{X: 40, Y: 50}) {
		t.Fatalf("H entry: %v %v", pt, ok)
	}
}

// SegCrossRect: вертикальный отрезок заходит снизу.
func TestSegCrossRectV(t *testing.T) {
	r := Rect{Min: Vec{X: 40, Y: 40}, Max: Vec{X: 60, Y: 60}}
	pt, ok := SegCrossRect(Vec{X: 50, Y: 0}, Vec{X: 50, Y: 100}, r)
	if !ok || !near(pt, Vec{X: 50, Y: 40}) {
		t.Fatalf("V entry: %v %v", pt, ok)
	}
}

// SegCrossRect: пролет мимо — нет пересечения.
func TestSegCrossRectMiss(t *testing.T) {
	r := Rect{Min: Vec{X: 40, Y: 40}, Max: Vec{X: 60, Y: 60}}
	if _, ok := SegCrossRect(Vec{X: 0, Y: 10}, Vec{X: 100, Y: 10}, r); ok {
		t.Fatal("линия выше блока не должна пересекать")
	}
	if _, ok := SegCrossRect(Vec{X: 10, Y: 0}, Vec{X: 10, Y: 100}, r); ok {
		t.Fatal("линия левее блока не должна пересекать")
	}
}

// RouteAvoid: прямой путь сквозь блок обходится (сверху или снизу — оба валидны).
func TestRouteAvoidBelow(t *testing.T) {
	obs := []Rect{{Min: Vec{X: 40, Y: 40}, Max: Vec{X: 60, Y: 60}}}
	route := RouteAvoid(Vec{X: 0, Y: 50}, SideRight, Vec{X: 100, Y: 50}, SideLeft, 2, obs, nil)
	// ни один горизонтальный сегмент не должен проходить через внутренность блока
	for i := 1; i < len(route); i++ {
		if nearf(route[i-1].Y, route[i].Y) {
			y := route[i-1].Y
			x0, x1 := minf(route[i-1].X, route[i].X), maxf(route[i-1].X, route[i].X)
			if y > obs[0].Min.Y && y < obs[0].Max.Y {
				if x0 < obs[0].Max.X && x1 > obs[0].Min.X {
					t.Fatalf("сегмент проходит сквозь блок: y=%v x[%v,%v]", y, x0, x1)
				}
			}
		}
	}
	if !near(route[0], Vec{X: 0, Y: 50}) || !near(route[len(route)-1], Vec{X: 100, Y: 50}) {
		t.Fatalf("концы не совпадают: %v", route)
	}
}

// RouteAvoid: вертикальный путь сквозь блок обходится слева или справа.
func TestRouteAvoidSide(t *testing.T) {
	obs := []Rect{{Min: Vec{X: 40, Y: 40}, Max: Vec{X: 60, Y: 60}}}
	route := RouteAvoid(Vec{X: 50, Y: 0}, SideBottom, Vec{X: 50, Y: 100}, SideTop, 2, obs, nil)
	for i := 1; i < len(route); i++ {
		if nearf(route[i-1].X, route[i].X) {
			x := route[i-1].X
			y0, y1 := minf(route[i-1].Y, route[i].Y), maxf(route[i-1].Y, route[i].Y)
			if x > obs[0].Min.X && x < obs[0].Max.X {
				if y0 < obs[0].Max.Y && y1 > obs[0].Min.Y {
					t.Fatalf("сегмент проходит сквозь блок: x=%v y[%v,%v]", x, y0, y1)
				}
			}
		}
	}
	if !near(route[0], Vec{X: 50, Y: 0}) || !near(route[len(route)-1], Vec{X: 50, Y: 100}) {
		t.Fatalf("концы не совпадают: %v", route)
	}
}

// RouteAvoid: без препятствий — как обычный Route.
func TestRouteAvoidNone(t *testing.T) {
	route := RouteAvoid(Vec{X: 0, Y: 0}, SideRight, Vec{X: 100, Y: 100}, SideLeft, 2, nil, nil)
	if len(route) < 2 {
		t.Fatalf("пустой маршрут: %v", route)
	}
	if !near(route[0], Vec{X: 0, Y: 0}) || !near(route[len(route)-1], Vec{X: 100, Y: 100}) {
		t.Fatalf("концы не совпадают: %v", route)
	}
}

// segRectDist — расстояние между осевым отрезком a-b и осевым
// прямоугольником r: зазоры по каждой оси, при разлете по обеим осям —
// гипотенуза (ближайшая точка — угол r).
func segRectDist(a, b Vec, r Rect) float32 {
	x0, x1 := minf(a.X, b.X), maxf(a.X, b.X)
	y0, y1 := minf(a.Y, b.Y), maxf(a.Y, b.Y)
	dx := maxf(maxf(r.Min.X-x1, x0-r.Max.X), 0)
	dy := maxf(maxf(r.Min.Y-y1, y0-r.Max.Y), 0)
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// RouteAvoid: зазор до обтекаемого блока сохраняется — ни один сегмент
// не заходит в зону «блок + out» (провода нельзя приклеивать к кромке).
func TestRouteAvoidKeepsGap(t *testing.T) {
	const out float32 = 5
	obs := []Rect{{Min: Vec{X: 40, Y: 40}, Max: Vec{X: 60, Y: 60}}}
	cases := []struct {
		name   string
		p1, p2 Vec
		s1, s2 Side
	}{
		{"горизонталь сквозь блок", Vec{X: 0, Y: 50}, Vec{X: 120, Y: 50}, SideRight, SideLeft},
		{"вертикаль сквозь блок", Vec{X: 50, Y: 0}, Vec{X: 50, Y: 120}, SideBottom, SideTop},
		{"колено рядом с блоком", Vec{X: 0, Y: 0}, Vec{X: 120, Y: 120}, SideRight, SideLeft},
		{"пролет в 2 мм от блока", Vec{X: 0, Y: 38}, Vec{X: 120, Y: 38}, SideRight, SideLeft},
	}
	for _, c := range cases {
		route := RouteAvoid(c.p1, c.s1, c.p2, c.s2, out, obs, nil)
		for i := 1; i < len(route); i++ {
			d := segRectDist(route[i-1], route[i], obs[0])
			if d < out-1e-3 {
				t.Errorf("%s: сегмент %v→%v подошел к блоку на %v при зазоре %v",
					c.name, route[i-1], route[i], d, out)
			}
		}
		if !near(route[0], c.p1) || !near(route[len(route)-1], c.p2) {
			t.Errorf("%s: концы маршрута потеряны: %v", c.name, route)
		}
	}
}

// pinAt — центр стороны r (тестовая вспомогательная).
func pinAt(r Rect, s Side) Vec {
	switch s {
	case SideLeft:
		return Vec{X: r.Min.X, Y: (r.Min.Y + r.Max.Y) / 2}
	case SideRight:
		return Vec{X: r.Max.X, Y: (r.Min.Y + r.Max.Y) / 2}
	case SideTop:
		return Vec{X: (r.Min.X + r.Max.X) / 2, Y: r.Min.Y}
	case SideBottom:
		return Vec{X: (r.Min.X + r.Max.X) / 2, Y: r.Max.Y}
	}
	return Vec{}
}

// near2 — прямоугольники ближе m друг к другу (для фильтра перебора).
func near2(a, b Rect, m float32) bool {
	return a.Max.X+m > b.Min.X && a.Min.X-m < b.Max.X &&
		a.Max.Y+m > b.Min.Y && a.Min.Y-m < b.Max.Y
}

// TestRouteAvoidNoHooks — свод правил трассировки: в маршруте не бывает
// «крюка» (горизонталь → вниз → вверх тем же x → горизонталь) и петель.
// Регресс на отзыв «тут сделал 3»: до фикса такой же перебор давал сотни
// тысяч крюков и маршруты, крутившие петли до лимита итераций.
func TestRouteAvoidNoHooks(t *testing.T) {
	const out float32 = 5
	S := Rect{Min: Vec{X: 0, Y: 0}, Max: Vec{X: 30, Y: 20}}
	sides := []Side{SideLeft, SideRight}

	// точные конфигурации, на которых пользователь видел крюки
	reported := []struct {
		name   string
		s1, s2 Side
		O, T   Rect
	}{
		{"справа→слева", SideRight, SideLeft,
			Rect{Min: Vec{X: -80, Y: -40}, Max: Vec{X: -50, Y: -20}},
			Rect{Min: Vec{X: -80, Y: -80}, Max: Vec{X: -50, Y: -60}}},
		{"справа→справа", SideRight, SideRight,
			Rect{Min: Vec{X: -80, Y: -40}, Max: Vec{X: -50, Y: -20}},
			Rect{Min: Vec{X: -80, Y: -80}, Max: Vec{X: -50, Y: -60}}},
	}
	for _, c := range reported {
		p1, p2 := pinAt(S, c.s1), pinAt(c.T, c.s2)
		route := RouteAvoid(p1, c.s1, p2, c.s2, out, []Rect{c.O}, []Rect{S, c.T})
		if n := Hairpins(route); n != 0 {
			t.Errorf("%s: крюков %d, маршрут %v", c.name, n, route)
		}
		if !near(route[0], p1) || !near(route[len(route)-1], p2) {
			t.Errorf("%s: концы маршрута потеряны: %v", c.name, route)
		}
		if len(route) > 14 {
			t.Errorf("%s: подозрительно длинный маршрут (петли?): %v", c.name, route)
		}
	}

	// сплошной перебор: свой блок S, чужой O, цель T — ни одного крюка
	total := 0
	for tx := -80; tx <= 180; tx += 20 {
		for ty := -80; ty <= 200; ty += 20 {
			T := Rect{Min: Vec{X: float32(tx), Y: float32(ty)},
				Max: Vec{X: float32(tx + 30), Y: float32(ty + 20)}}
			if near2(S, T, 15) {
				continue
			}
			for ox := -80; ox <= 180; ox += 20 {
				for oy := -80; oy <= 200; oy += 20 {
					O := Rect{Min: Vec{X: float32(ox), Y: float32(oy)},
						Max: Vec{X: float32(ox + 30), Y: float32(oy + 20)}}
					if near2(O, S, 15) || near2(O, T, 15) {
						continue
					}
					for _, s1 := range sides {
						for _, s2 := range sides {
							p1, p2 := pinAt(S, s1), pinAt(T, s2)
							route := RouteAvoid(p1, s1, p2, s2, out,
								[]Rect{O}, []Rect{S, T})
							total++
							if n := Hairpins(route); n != 0 {
								t.Fatalf("крюков %d: s1=%v s2=%v T=%v O=%v route=%v",
									n, s1, s2, T, O, route)
							}
							if len(route) > 16 {
								t.Fatalf("петли (%d точек): s1=%v s2=%v T=%v O=%v route=%v",
									len(route), s1, s2, T, O, route)
							}
							if !near(route[0], p1) || !near(route[len(route)-1], p2) {
								t.Fatalf("концы потеряны: %v", route)
							}
						}
					}
				}
			}
		}
	}
	if total < 100000 {
		t.Fatalf("перебор слишком мал: %d маршрутов", total)
	}
	t.Logf("проверено маршрутов: %d", total)
}
