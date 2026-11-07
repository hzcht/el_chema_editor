package main

import (
	"math"
	"slices"
	"testing"

	"shbuilder/internal/core"
	"shbuilder/internal/geom"
)

// TestLinkObstaclesForeignPins — О2: в препятствия маршрута попадают все
// пины, кроме двух концов линка (точка с зазором pinGap), чтобы провод не
// проходил ровно через чужой пин.
func TestLinkObstaclesForeignPins(t *testing.T) {
	d := &core.Document{}
	d.AddBlock(&core.Block{ID: "A", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	d.AddBlock(&core.Block{ID: "B", X: 100, Y: 0, W: 30, H: 20, Pins: 2})
	d.AddBlock(&core.Block{ID: "C", X: 50, Y: 100, W: 30, H: 20, Pins: 2})

	from := core.Endpoint{BlockID: "A", Pin: 1} // правый центр A
	to := core.Endpoint{BlockID: "B", Pin: 0}   // левый центр B
	obs := linkObstacles(d, from, to)

	points := map[geom.Vec]bool{}
	for _, o := range obs {
		if o.Gap == pinGap && o.Rect.Min == o.Rect.Max {
			points[o.Rect.Min] = true
		}
	}
	want := []geom.Vec{{X: 0, Y: 10}, {X: 130, Y: 10}, {X: 50, Y: 110}, {X: 80, Y: 110}}
	for _, p := range want {
		if !points[p] {
			t.Fatalf("пин %v не стал препятствием: %v", p, obs)
		}
	}
	if points[geom.Vec{X: 30, Y: 10}] || points[geom.Vec{X: 100, Y: 10}] {
		t.Fatalf("пины-концы не должны быть препятствиями: %v", obs)
	}
}

// routeScenePaths — конвейер drawLinks без отрисовки: маршруты в порядке
// ключа линка + разводка коллинеарных наложений. Нужен, чтобы проверять
// детерминизм и инварианты без layout.Context.
func routeScenePaths(d *core.Document) map[string][]geom.Vec {
	links := append([]core.Link(nil), d.Links...)
	slices.SortFunc(links, compareLinks)

	keys := make([]string, 0, len(links))
	paths := make([][]geom.Vec, 0, len(links))
	rects := make([][]geom.Rect, 0, len(links))
	for _, l := range links {
		p1, s1, ok1 := d.PinPos(l.From)
		p2, s2, ok2 := d.PinPos(l.To)
		if !ok1 || !ok2 {
			continue
		}
		paths = append(paths, geom.RouteEx(p1, s1, p2, s2, outset, geom.RouteOpts{
			Obstacles: linkObstacles(d, l.From, l.To),
		}))
		rects = append(rects, laneRects(d, l.From, l.To))
		keys = append(keys, linkKey(l))
	}
	paths = geom.OffsetLanes(paths, rects, laneGap)

	out := make(map[string][]geom.Vec, len(keys))
	for i, k := range keys {
		out[k] = paths[i]
	}
	return out
}

// TestRoutePipelineDeterministic — одинаковый документ даёт одинаковые
// маршруты независимо от порядка создания линков (П10 + разводка полос).
func TestRoutePipelineDeterministic(t *testing.T) {
	build := func(order []int) *core.Document {
		d := &core.Document{}
		d.AddBlock(&core.Block{ID: "A", X: 20, Y: 20, W: 30, H: 20, Pins: 4})
		d.AddBlock(&core.Block{ID: "B", X: 220, Y: 20, W: 30, H: 20, Pins: 4})
		d.AddBlock(&core.Block{ID: "C", X: 20, Y: 160, W: 30, H: 20, Pins: 4})
		d.AddBlock(&core.Block{ID: "D", X: 220, Y: 160, W: 30, H: 20, Pins: 4})
		pairs := [][2]core.Endpoint{
			{{BlockID: "A", Pin: 1}, {BlockID: "B", Pin: 0}},
			{{BlockID: "B", Pin: 1}, {BlockID: "C", Pin: 0}},
			{{BlockID: "A", Pin: 3}, {BlockID: "B", Pin: 0}}, // общий пин B.0
			{{BlockID: "C", Pin: 1}, {BlockID: "D", Pin: 0}},
		}
		for _, i := range order {
			if !d.AddLink(pairs[i][0], pairs[i][1]) {
				t.Fatalf("линк не добавился: %v", pairs[i])
			}
		}
		return d
	}

	forward := routeScenePaths(build([]int{0, 1, 2, 3}))
	reverse := routeScenePaths(build([]int{3, 2, 1, 0}))
	if len(forward) != len(reverse) {
		t.Fatalf("разное число маршрутов: %d / %d", len(forward), len(reverse))
	}
	for k, a := range forward {
		b, ok := reverse[k]
		if !ok || !samePathVec(a, b) {
			t.Fatalf("маршрут %s зависит от порядка создания:\n %v\n %v", k, a, b)
		}
		if n := geom.Hairpins(a); n != 0 {
			t.Fatalf("линк %s: крюков %d: %v", k, n, a)
		}
	}
}

func samePathVec(a, b []geom.Vec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !nearVec(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TestEightLinksLanes — расширенная мультилинковая сцена (8 линков, 4 пина
// на блок, центральный блок-препятствие). Проверяет П4/П5/П7 и П12: после
// разводки не остаётся длинных коллинеарных наложений, все пути приходят в
// свои пины, пересечения между линками есть (мостики будут).
func TestEightLinksLanes(t *testing.T) {
	d := &core.Document{}
	d.AddBlock(&core.Block{ID: "A", X: 20, Y: 20, W: 30, H: 20, Pins: 4})
	d.AddBlock(&core.Block{ID: "B", X: 260, Y: 20, W: 30, H: 20, Pins: 4})
	d.AddBlock(&core.Block{ID: "C", X: 20, Y: 180, W: 30, H: 20, Pins: 4})
	d.AddBlock(&core.Block{ID: "D", X: 260, Y: 180, W: 30, H: 20, Pins: 4})
	d.AddBlock(&core.Block{ID: "E", X: 140, Y: 90, W: 30, H: 20, Pins: 4})

	pairs := [][2]core.Endpoint{
		{{BlockID: "A", Pin: 1}, {BlockID: "B", Pin: 0}},
		{{BlockID: "A", Pin: 3}, {BlockID: "C", Pin: 1}},
		{{BlockID: "B", Pin: 3}, {BlockID: "D", Pin: 2}},
		{{BlockID: "C", Pin: 3}, {BlockID: "D", Pin: 0}},
		{{BlockID: "A", Pin: 0}, {BlockID: "D", Pin: 3}},
		{{BlockID: "B", Pin: 2}, {BlockID: "C", Pin: 0}},
		{{BlockID: "C", Pin: 2}, {BlockID: "D", Pin: 1}},
		{{BlockID: "A", Pin: 2}, {BlockID: "B", Pin: 1}},
	}
	for _, p := range pairs {
		if !d.AddLink(p[0], p[1]) {
			t.Fatalf("линк не добавился: %v → %v", p[0], p[1])
		}
	}

	paths := routeScenePaths(d)
	if len(paths) != len(pairs) {
		t.Fatalf("маршрутов %d, ждали %d", len(paths), len(pairs))
	}

	type named struct {
		key  string
		path []geom.Vec
	}
	list := make([]named, 0, len(paths))
	for k, p := range paths {
		list = append(list, named{k, p})
	}
	slices.SortFunc(list, func(a, b named) int {
		switch {
		case a.key < b.key:
			return -1
		case a.key > b.key:
			return 1
		}
		return 0
	})

	for _, it := range list {
		if n := geom.Hairpins(it.path); n != 0 {
			t.Fatalf("линк %s: крюков %d: %v", it.key, n, it.path)
		}
		if len(it.path) > 24 {
			t.Fatalf("линк %s: петли (%d точек): %v", it.key, len(it.path), it.path)
		}
	}
	// наложений длиннее laneGap не осталось
	for i := 0; i < len(list); i++ {
		for k := 1; k < len(list[i].path); k++ {
			a, b := list[i].path[k-1], list[i].path[k]
			for j := 0; j < i; j++ {
				for m := 1; m < len(list[j].path); m++ {
					if l := collinearOverlapVec(a, b, list[j].path[m-1], list[j].path[m]); l > laneGap+1e-3 {
						t.Fatalf("наложение %.1f мм: %s %v→%v и %s: %v",
							l, list[i].key, a, b, list[j].key, list[j].path)
					}
				}
			}
		}
	}
	// пересечения между линками действительно есть
	cross := 0
	for i := 0; i < len(list); i++ {
		for k := 1; k < len(list[i].path); k++ {
			for j := 0; j < i; j++ {
				for m := 1; m < len(list[j].path); m++ {
					if _, ok := geom.SegCross(list[i].path[k-1], list[i].path[k],
						list[j].path[m-1], list[j].path[m]); ok {
						cross++
					}
				}
			}
		}
	}
	if cross == 0 {
		t.Fatal("в сцене нет пересечений — мостики не проверены")
	}
	t.Logf("линков: %d, пересечений: %d", len(list), cross)
}

// collinearOverlapVec — длина общего участка коллинеарных осевых сегментов.
func collinearOverlapVec(a, b, c, d geom.Vec) float32 {
	lo := func(x, y float32) float32 {
		if x < y {
			return x
		}
		return y
	}
	hi := func(x, y float32) float32 {
		if x > y {
			return x
		}
		return y
	}
	if abs2f(a.Y-b.Y) <= 1e-3 && abs2f(c.Y-d.Y) <= 1e-3 && abs2f(a.Y-c.Y) <= 1e-3 {
		return hi(0, lo(hi(a.X, b.X), hi(c.X, d.X))-hi(lo(a.X, b.X), lo(c.X, d.X)))
	}
	if abs2f(a.X-b.X) <= 1e-3 && abs2f(c.X-d.X) <= 1e-3 && abs2f(a.X-c.X) <= 1e-3 {
		return hi(0, lo(hi(a.Y, b.Y), hi(c.Y, d.Y))-hi(lo(a.Y, b.Y), lo(c.Y, d.Y)))
	}
	return 0
}

// TestDemoProductionRoutes — демо-сцена проходит боевой конвейер (блоки +
// чужие пины): без крюков и петель, концы на пинах, ни один сегмент не
// подходит к чужому пину ближе pinGap.
func TestDemoProductionRoutes(t *testing.T) {
	d := demoScene()
	for _, l := range d.Links {
		p1, s1, ok1 := d.PinPos(l.From)
		p2, s2, ok2 := d.PinPos(l.To)
		if !ok1 || !ok2 {
			t.Fatalf("пины линка не найдены: %v", l)
		}
		route := geom.RouteEx(p1, s1, p2, s2, outset, geom.RouteOpts{
			Obstacles: linkObstacles(d, l.From, l.To),
		})
		if n := geom.Hairpins(route); n != 0 {
			t.Fatalf("линк %v→%v: крюков %d: %v", l.From, l.To, n, route)
		}
		if len(route) > 16 {
			t.Fatalf("линк %v→%v: петли (%d точек): %v", l.From, l.To, len(route), route)
		}
		if !nearVec(route[0], p1) || !nearVec(route[len(route)-1], p2) {
			t.Fatalf("линк %v→%v: концы потеряны: %v", l.From, l.To, route)
		}
		for _, b := range d.Blocks {
			for i := 0; i < len(core.PinLayout(b.Pins)); i++ {
				ep := core.Endpoint{BlockID: b.ID, Pin: i}
				if ep == l.From || ep == l.To {
					continue
				}
				pin, _, ok := b.PinPos(i)
				if !ok {
					continue
				}
				for k := 1; k < len(route); k++ {
					if dist := segPointGap(route[k-1], route[k], pin); dist < pinGap-1e-3 {
						t.Fatalf("линк %v→%v проходит в %.2f мм от пина %v: %v",
							l.From, l.To, dist, ep, route)
					}
				}
			}
		}
	}
}

// segPointGap — расстояние от осевого отрезка до точки (без математики
// hypot: отрезки только горизонтальные/вертикальные).
func segPointGap(a, b, p geom.Vec) float32 {
	ax, bx := a.X, b.X
	ay, by := a.Y, b.Y
	if bx < ax {
		ax, bx = bx, ax
	}
	if by < ay {
		ay, by = by, ay
	}
	dx := float32(0)
	if p.X < ax {
		dx = ax - p.X
	} else if p.X > bx {
		dx = p.X - bx
	}
	dy := float32(0)
	if p.Y < ay {
		dy = ay - p.Y
	} else if p.Y > by {
		dy = p.Y - by
	}
	if dx == 0 && dy == 0 {
		return 0
	}
	if dx == 0 {
		return dy
	}
	if dy == 0 {
		return dx
	}
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}
