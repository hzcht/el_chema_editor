package geom

import "testing"

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
	route := RouteAvoid(Vec{X: 0, Y: 50}, SideRight, Vec{X: 100, Y: 50}, SideLeft, 2, obs)
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
	route := RouteAvoid(Vec{X: 50, Y: 0}, SideBottom, Vec{X: 50, Y: 100}, SideTop, 2, obs)
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
	route := RouteAvoid(Vec{X: 0, Y: 0}, SideRight, Vec{X: 100, Y: 100}, SideLeft, 2, nil)
	if len(route) < 2 {
		t.Fatalf("пустой маршрут: %v", route)
	}
	if !near(route[0], Vec{X: 0, Y: 0}) || !near(route[len(route)-1], Vec{X: 100, Y: 100}) {
		t.Fatalf("концы не совпадают: %v", route)
	}
}
