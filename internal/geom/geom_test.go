package geom

import "testing"

func eqf(a, b float32) bool { return absf(a-b) < 0.01 }

// Camera: screen = world*zoom + pan; roundtrip.
func TestCameraRoundtrip(t *testing.T) {
	c := Camera{Zoom: 2.5, Pan: Vec{-130, 700}}
	world := Vec{-42.5, 197.5}
	screen := c.WorldToScreen(world)
	back := c.ScreenToWorld(screen)
	if !eqf(back.X, world.X) || !eqf(back.Y, world.Y) {
		t.Fatalf("roundtrip: got %v want %v", back, world)
	}
}

// ZoomAt держит мировую точку под курсором.
func TestZoomAtAnchorsCursor(t *testing.T) {
	c := Camera{Zoom: 1, Pan: Vec{0, 0}}
	cursor := Vec{300, 500}
	world := c.ScreenToWorld(cursor) // world == cursor при zoom=1

	for _, f := range []float32{1.25, 0.8, 4, 0.1} {
		c.ZoomAt(cursor, f)
		w2 := c.ScreenToWorld(cursor)
		if !eqf(w2.X, world.X) || !eqf(w2.Y, world.Y) {
			t.Fatalf("factor %v: world under cursor drifted: got %v want %v", f, w2, world)
		}
		if c.Zoom < ZoomMin || c.Zoom > ZoomMax {
			t.Fatalf("zoom out of clamp: %v", c.Zoom)
		}
	}
}

func TestSnap(t *testing.T) {
	cases := []struct {
		v, step, want float32
	}{
		{13, 10, 10}, {17, 10, 20}, {-13, 10, -10}, {-17, 10, -20}, {15, 10, 20},
	}
	for _, c := range cases {
		if got := Snap(c.v, c.step); got != c.want {
			t.Fatalf("Snap(%v, %v) = %v, want %v", c.v, c.step, got, c.want)
		}
	}
}

// Свойства маршрута: концы верные, все сегменты ортогональные, выходы
// перпендикулярны сторонам, нулевых сегментов нет.
func checkRoute(t *testing.T, p1 Vec, s1 Side, p2 Vec, s2 Side) {
	t.Helper()
	pts := Route(p1, s1, p2, s2, 16)
	if len(pts) < 2 {
		t.Fatalf("route too short: %v", pts)
	}
	if !near(pts[0], p1) || !near(pts[len(pts)-1], p2) {
		t.Fatalf("route endpoints wrong: got first=%v want=%v last=%v want=%v",
			pts[0], p1, pts[len(pts)-1], p2)
	}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		dx, dy := absf(a.X-b.X), absf(a.Y-b.Y)
		if dx > eps && dy > eps {
			t.Fatalf("наклонный сегмент %d: %v → %v", i, a, b)
		}
		if dx <= eps && dy <= eps {
			t.Fatalf("нулевой сегмент %d: %v → %v", i, a, b)
		}
	}
	// первый сегмент вдоль Normal(s1)
	d1 := pts[1].Sub(pts[0])
	if !alongNormal(d1, s1) {
		t.Fatalf("выход не перпендикулярен стороне: %v от %v (s=%v)", d1, p1, s1)
	}
	// последний сегмент против Normal(s2) — вход в пин со стороны стороны
	dn := pts[len(pts)-1].Sub(pts[len(pts)-2])
	if !alongNormal(dn, inverse(s2)) {
		t.Fatalf("вход не перпендикулярен стороне: %v к %v (s=%v)", dn, p2, s2)
	}
}

func alongNormal(d Vec, s Side) bool {
	n := s.Normal()
	if n.X != 0 {
		return nearf(d.Y, 0) && d.X*n.X > 0
	}
	return nearf(d.X, 0) && d.Y*n.Y > 0
}

// inverse: сторона, чья нормальная противоположна s.
func inverse(s Side) Side {
	switch s {
	case SideLeft:
		return SideRight
	case SideRight:
		return SideLeft
	case SideTop:
		return SideBottom
	case SideBottom:
		return SideTop
	}
	return SideNone
}

func TestRouteInvariants(t *testing.T) {
	cases := []struct {
		p1 Vec
		s1 Side
		p2 Vec
		s2 Side
	}{
		{Vec{200, 200}, SideRight, Vec{400, 200}, SideLeft},    // прямая
		{Vec{200, 200}, SideRight, Vec{400, 320}, SideLeft},    // Z
		{Vec{200, 200}, SideLeft, Vec{120, 400}, SideTop},      // колено перпен.
		{Vec{300, 100}, SideTop, Vec{300, 400}, SideBottom},    // прямая (верт)
		{Vec{300, 100}, SideTop, Vec{180, 400}, SideBottom},    // Z (верт)
		{Vec{100, 100}, SideLeft, Vec{200, 380}, SideLeft},     // одна и та же сторона
		{Vec{100, 100}, SideBottom, Vec{320, 140}, SideBottom}, // одна и та же сторона (гор.)
		{Vec{200, 200}, SideRight, Vec{150, 260}, SideLeft},    // L→R «назад»
		{Vec{200, 200}, SideBottom, Vec{240, 100}, SideTop},    // B→T «назад» (верт)
		{Vec{200, 200}, SideRight, Vec{420, 300}, SideTop},     // горизонт→вертик
		{Vec{200, 200}, SideBottom, Vec{420, 300}, SideRight},  // вертик→горизонт
	}
	for _, c := range cases {
		checkRoute(t, c.p1, c.s1, c.p2, c.s2)
	}
}

func TestRouteStraightIsMinimal(t *testing.T) {
	pts := Route(Vec{200, 200}, SideRight, Vec{400, 200}, SideLeft, 16)
	if len(pts) != 2 {
		t.Fatalf("для прямой линии ожидали 2 точки, получили %v", pts)
	}
}
