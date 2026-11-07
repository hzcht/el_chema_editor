package geom

import (
	"math"
	"testing"
)

// segPointDist — расстояние от осевого сегмента a-b до точки p.
func segPointDist(a, b, p Vec) float32 {
	if nearf(a.Y, b.Y) {
		x0, x1 := minf(a.X, b.X), maxf(a.X, b.X)
		dx := maxf(maxf(x0-p.X, p.X-x1), 0)
		dy := absf(a.Y - p.Y)
		return float32(math.Sqrt(float64(dx*dx + dy*dy)))
	}
	if nearf(a.X, b.X) {
		y0, y1 := minf(a.Y, b.Y), maxf(a.Y, b.Y)
		dy := maxf(maxf(y0-p.Y, p.Y-y1), 0)
		dx := absf(a.X - p.X)
		return float32(math.Sqrt(float64(dx*dx + dy*dy)))
	}
	return 0
}

// TestRouteAvoidForeignPin — О2: провод не проходит ровно через чужой пин.
// Пин-точка с зазором Gap: маршрут обязан обойти её, как маленькое
// препятствие, иначе он визуально выглядит подключённым к этому пину.
func TestRouteAvoidForeignPin(t *testing.T) {
	const gap float32 = 2
	pin := Vec{X: 50, Y: 0}
	p1, p2 := Vec{X: 0, Y: 0}, Vec{X: 100, Y: 0}
	route := RouteEx(p1, SideRight, p2, SideLeft, 5, RouteOpts{
		Obstacles: []Obstacle{{Rect: Rect{Min: pin, Max: pin}, Gap: gap}},
	})

	if len(route) < 3 {
		t.Fatalf("маршрут прошёл сквозь пин: %v", route)
	}
	for i := 1; i < len(route); i++ {
		if d := segPointDist(route[i-1], route[i], pin); d < gap-1e-3 {
			t.Fatalf("сегмент %v→%v ближе %v к пину (%.3f): %v",
				route[i-1], route[i], gap, d, route)
		}
	}
	if n := Hairpins(route); n != 0 {
		t.Fatalf("крюков %d: %v", n, route)
	}
	if !near(route[0], p1) || !near(route[len(route)-1], p2) {
		t.Fatalf("концы потеряны: %v", route)
	}
}

// TestDetourSideSticky — О3: гистерезис стороны обхода. При симметричном
// препятствии выбирается верх; после микросдвига блока вверх нижний обход
// становится короче, но с Prev/Sticky маршрут держится прежней стороны,
// а без памяти — перескакивает.
func TestDetourSideSticky(t *testing.T) {
	const out float32 = 2
	p1, p2 := Vec{X: 0, Y: 50}, Vec{X: 100, Y: 50}
	obs := []Obstacle{{Rect: Rect{Min: Vec{X: 40, Y: 40}, Max: Vec{X: 60, Y: 60}}, Gap: out}}

	routeSide := func(r []Vec) float32 {
		lo, hi := float32(50), float32(50)
		for _, p := range r {
			lo, hi = minf(lo, p.Y), maxf(hi, p.Y)
		}
		if lo < 50-eps {
			return lo // обход сверху
		}
		return hi // обход снизу
	}

	first := RouteEx(p1, SideRight, p2, SideLeft, out, RouteOpts{Obstacles: obs})
	if got := routeSide(first); !nearf(got, 38) {
		t.Fatalf("симметричный обход: ждали верх (38), получили %v: %v", got, first)
	}

	// блок сдвинулся на 0.5 мм вверх: нижний обход короче на 2 мм
	obs[0].Rect = Rect{Min: Vec{X: 40, Y: 39.5}, Max: Vec{X: 60, Y: 59.5}}
	plain := RouteEx(p1, SideRight, p2, SideLeft, out, RouteOpts{Obstacles: obs})
	if got := routeSide(plain); !nearf(got, 61.5) {
		t.Fatalf("без памяти сторона не сменилась: %v (%v)", got, plain)
	}
	sticky := RouteEx(p1, SideRight, p2, SideLeft, out, RouteOpts{
		Obstacles: obs,
		Prev:      first,
		Sticky:    3,
	})
	if got := routeSide(sticky); !nearf(got, 37.5) {
		t.Fatalf("гистерезис не удержал сторону: %v (%v)", got, sticky)
	}
	if n := Hairpins(sticky); n != 0 {
		t.Fatalf("крюков %d: %v", n, sticky)
	}
}
