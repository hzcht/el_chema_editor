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
