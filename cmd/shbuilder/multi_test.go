package main

import (
	"slices"
	"testing"

	"shbuilder/internal/core"
	"shbuilder/internal/geom"
)

// nearVec — совпадение точек маршрута с точностью до 1e-3 мм.
func nearVec(a, b geom.Vec) bool {
	return abs2f(a.X-b.X) <= 1e-3 && abs2f(a.Y-b.Y) <= 1e-3
}

// TestLinkDrawOrderStable — П10: порядок отрисовки линков определяется
// ключом (блок/пин начала → конца), а не порядком создания. Иначе «кто
// прыгает мостиком» менялся бы от сессии к сессии.
func TestLinkDrawOrderStable(t *testing.T) {
	a := []core.Link{
		{From: core.Endpoint{BlockID: "b2", Pin: 0}, To: core.Endpoint{BlockID: "b1", Pin: 1}},
		{From: core.Endpoint{BlockID: "b1", Pin: 0}, To: core.Endpoint{BlockID: "b2", Pin: 1}},
		{From: core.Endpoint{BlockID: "b1", Pin: 0}, To: core.Endpoint{BlockID: "b1", Pin: 2}},
	}
	// другой порядок создания тех же линков
	b := []core.Link{a[2], a[0], a[1]}

	sa := append([]core.Link(nil), a...)
	sb := append([]core.Link(nil), b...)
	slices.SortFunc(sa, compareLinks)
	slices.SortFunc(sb, compareLinks)

	for i := range sa {
		if sa[i] != sb[i] {
			t.Fatalf("порядок зависит от порядка создания:\n A=%v\n B=%v", sa, sb)
		}
	}
	// и это именно порядок по ключу: b1.0→b1.2, b1.0→b2.1, b2.0→b1.1
	want := []core.Link{a[2], a[1], a[0]}
	for i := range want {
		if sa[i] != want[i] {
			t.Fatalf("неожиданный порядок: got %v, want %v", sa, want)
		}
	}
}

// TestManyLinksNoHooks — мультилинковая сцена: 4 блока по углам + центральный
// блок-препятствие и 6 линков, часть которых обязана пересечься. Проверяет
// свод правил §4.6 сразу на нескольких линках: нет крюков и петель, ни одна
// линия не режет внутренность блока, и пересечения действительно есть
// (значит, на отрисовке будут мостики).
func TestManyLinksNoHooks(t *testing.T) {
	d := &core.Document{}
	d.AddBlock(&core.Block{ID: "A", Kind: "src", X: 20, Y: 20, W: 30, H: 20, Label: "A", Pins: 4})
	d.AddBlock(&core.Block{ID: "B", Kind: "hub", X: 220, Y: 20, W: 30, H: 20, Label: "B", Pins: 4})
	d.AddBlock(&core.Block{ID: "C", Kind: "load", X: 20, Y: 160, W: 30, H: 20, Label: "C", Pins: 4})
	d.AddBlock(&core.Block{ID: "D", Kind: "term", X: 220, Y: 160, W: 30, H: 20, Label: "D", Pins: 4})
	d.AddBlock(&core.Block{ID: "E", Kind: "hub", X: 120, Y: 90, W: 30, H: 20, Label: "E", Pins: 4})

	pairs := [][2]core.Endpoint{
		{{BlockID: "A", Pin: 0}, {BlockID: "D", Pin: 0}},
		{{BlockID: "B", Pin: 0}, {BlockID: "C", Pin: 0}},
		{{BlockID: "A", Pin: 1}, {BlockID: "B", Pin: 1}},
		{{BlockID: "C", Pin: 1}, {BlockID: "D", Pin: 1}},
		{{BlockID: "A", Pin: 2}, {BlockID: "C", Pin: 2}},
		{{BlockID: "B", Pin: 2}, {BlockID: "D", Pin: 2}},
	}
	for _, p := range pairs {
		if !d.AddLink(p[0], p[1]) {
			t.Fatalf("линк не добавился: %v → %v", p[0], p[1])
		}
	}

	const pad float32 = 1e-3
	routes := make([][]geom.Vec, 0, len(d.Links))
	for _, l := range d.Links {
		obs, self := linkObs(d, l.From, l.To)
		p1, s1, ok1 := d.PinPos(l.From)
		p2, s2, ok2 := d.PinPos(l.To)
		if !ok1 || !ok2 {
			t.Fatalf("пины не найдены: %+v → %+v", l.From, l.To)
		}
		route := geom.RouteAvoid(p1, s1, p2, s2, outset, obs, self)
		routes = append(routes, route)

		if n := geom.Hairpins(route); n != 0 {
			t.Errorf("линк %v→%v: крюков %d, маршрут %v", l.From, l.To, n, route)
		}
		if len(route) > 20 {
			t.Errorf("линк %v→%v: петли (%d точек): %v", l.From, l.To, len(route), route)
		}
		if !nearVec(route[0], p1) || !nearVec(route[len(route)-1], p2) {
			t.Errorf("линк %v→%v: концы потеряны: %v", l.From, l.To, route)
		}
		all := append(append([]geom.Rect{}, obs...), self...)
		for i := 1; i < len(route); i++ {
			for _, r := range all {
				if segInteriorCross(route[i-1], route[i], r, pad) {
					t.Errorf("линк %v→%v: сегмент %v→%v режет блок %v",
						l.From, l.To, route[i-1], route[i], r)
				}
			}
		}
	}

	// хотя бы одно пересечение разных линков — иначе сцена не проверяет мостики
	crossings := 0
	for i := 0; i < len(routes); i++ {
		for j := i + 1; j < len(routes); j++ {
			for a := 1; a < len(routes[i]); a++ {
				for b := 1; b < len(routes[j]); b++ {
					if _, ok := geom.SegCross(routes[i][a-1], routes[i][a],
						routes[j][b-1], routes[j][b]); ok {
						crossings++
					}
				}
			}
		}
	}
	if crossings == 0 {
		t.Fatal("в сцене нет пересечений линков — мостики не проверены")
	}
	t.Logf("линков: %d, пересечений: %d", len(routes), crossings)
}
