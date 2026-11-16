package core

import (
	"testing"
)

// chainDoc — цепочка a → b → c (правый пин → левый пин), все блоки в столбик.
func chainDoc() *Document {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "a", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	b := d.AddBlock(&Block{ID: "b", X: 0, Y: 100, W: 30, H: 20, Pins: 2})
	c := d.AddBlock(&Block{ID: "c", X: 0, Y: 200, W: 30, H: 20, Pins: 2})
	d.AddLink(Endpoint{a.ID, 1}, Endpoint{b.ID, 0})
	d.AddLink(Endpoint{b.ID, 1}, Endpoint{c.ID, 0})
	return d
}

// Цепочка раскладывается тремя слоями слева направо, по одному блоку в слое.
func TestLayoutChainLeftToRight(t *testing.T) {
	d := chainDoc()
	n, layers := Layout(d, nil, 10)
	if n != 3 || layers != 3 {
		t.Fatalf("Layout вернул (%d, %d), ожидали (3, 3)", n, layers)
	}
	want := map[string]float32{"a": 0, "b": 50, "c": 100} // 30 + зазор 20
	for id, x := range want {
		b := d.BlockByID(id)
		if b.X != x || b.Y != 0 {
			t.Fatalf("%s = (%v, %v), ожидали (%v, 0)", id, b.X, b.Y, x)
		}
	}
}

// Блоки остаются на исходной рамке: раскладка не сдвигает схему целиком.
func TestLayoutKeepsOrigin(t *testing.T) {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "a", X: 130, Y: 70, W: 30, H: 20, Pins: 2})
	b := d.AddBlock(&Block{ID: "b", X: 140, Y: 200, W: 30, H: 20, Pins: 2})
	d.AddLink(Endpoint{a.ID, 1}, Endpoint{b.ID, 0})
	Layout(d, nil, 10)
	if a.X != 130 || a.Y != 70 {
		t.Fatalf("a = (%v, %v), ожидали (130, 70)", a.X, a.Y)
	}
	if b.X != 180 || b.Y != 70 { // 130 + 30 + 20
		t.Fatalf("b = (%v, %v), ожидали (180, 70)", b.X, b.Y)
	}
}

// Сетка: все координаты кратны шагу (для 5, 10 и 20 мм).
func TestLayoutSnapsToGrid(t *testing.T) {
	for _, grid := range []float32{5, 10, 20} {
		d := chainDoc()
		Layout(d, nil, grid)
		for _, b := range d.Blocks {
			if b.X/grid != float32(int(b.X/grid)) || b.Y/grid != float32(int(b.Y/grid)) {
				t.Fatalf("grid=%v: %s = (%v, %v) не кратно сетке", grid, b.ID, b.X, b.Y)
			}
		}
	}
}

// Плотная сцена с junction-кластером и циклом: наложений быть не должно.
func TestLayoutNoOverlaps(t *testing.T) {
	d := &Document{}
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	// вход перемешан: раскладка обязана быть детерминированной
	for i, id := range []string{"h", "c", "f", "a", "g", "d", "b", "e"} {
		d.AddBlock(&Block{ID: id, X: float32(i * 17), Y: float32(i * 23), W: 30, H: 20, Pins: 2})
	}
	add := func(from, to string) { d.AddLink(Endpoint{from, 1}, Endpoint{to, 0}) }
	add("a", "b")
	add("a", "c") // junction: два линка из одного пина
	add("b", "d")
	add("c", "d")
	add("d", "e")
	add("e", "f")
	add("f", "g")
	add("g", "h")
	add("h", "a") // цикл
	n, layers := Layout(d, ids, 10)
	if n != 8 || layers < 4 {
		t.Fatalf("Layout вернул (%d, %d)", n, layers)
	}
	if HasOverlaps(d) {
		t.Fatal("после раскладки блоки накладываются")
	}
	// x растёт по слоям: a левее b, b левее d
	if !(d.BlockByID("a").X < d.BlockByID("b").X && d.BlockByID("b").X < d.BlockByID("d").X) {
		t.Fatal("слои должны идти слева направо")
	}
}

// Цикл не вешает обход и не оставляет блок без места.
func TestLayoutCycle(t *testing.T) {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "a", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	b := d.AddBlock(&Block{ID: "b", X: 0, Y: 40, W: 30, H: 20, Pins: 2})
	c := d.AddBlock(&Block{ID: "c", X: 0, Y: 80, W: 30, H: 20, Pins: 2})
	d.AddLink(Endpoint{a.ID, 1}, Endpoint{b.ID, 0})
	d.AddLink(Endpoint{b.ID, 1}, Endpoint{c.ID, 0})
	d.AddLink(Endpoint{c.ID, 1}, Endpoint{a.ID, 0})
	_, layers := Layout(d, nil, 10)
	if layers != 3 {
		t.Fatalf("слоёв %d, ожидали 3", layers)
	}
	if HasOverlaps(d) {
		t.Fatal("цикл: блоки накладываются")
	}
}

// Несвязные схемы раскладываются отдельными столбиками друг под другом.
func TestLayoutDisconnected(t *testing.T) {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "a", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	b := d.AddBlock(&Block{ID: "b", X: 300, Y: 40, W: 30, H: 20, Pins: 2})
	c := d.AddBlock(&Block{ID: "c", X: 10, Y: 300, W: 30, H: 20, Pins: 2})
	e := d.AddBlock(&Block{ID: "e", X: 400, Y: 320, W: 30, H: 20, Pins: 2})
	d.AddLink(Endpoint{a.ID, 1}, Endpoint{b.ID, 0})
	d.AddLink(Endpoint{c.ID, 1}, Endpoint{e.ID, 0})
	Layout(d, nil, 10)
	if HasOverlaps(d) {
		t.Fatal("несвязные компоненты накладываются")
	}
	// обе компоненты начинаются от одной левой границы
	if a.X != 0 || c.X != 0 {
		t.Fatalf("левый край компонент: a=%v c=%v", a.X, c.X)
	}
	if !(c.Y > a.Y) {
		t.Fatalf("вторая компонента должна быть ниже: a.Y=%v c.Y=%v", a.Y, c.Y)
	}
}

// Компоненты, нарисованные вдали от начала координат, не наезжают друг на
// друга: следующая ставится под предыдущей через зазор layoutCompGap.
func TestLayoutComponentsKeepGap(t *testing.T) {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "a", X: 0, Y: 400, W: 30, H: 20, Pins: 2})
	b := d.AddBlock(&Block{ID: "b", X: 200, Y: 400, W: 30, H: 20, Pins: 2})
	z := d.AddBlock(&Block{ID: "z", X: 100, Y: 900, W: 30, H: 20, Pins: 2})
	d.AddLink(Endpoint{a.ID, 1}, Endpoint{b.ID, 0})
	Layout(d, nil, 10)
	if HasOverlaps(d) {
		t.Fatal("компоненты накладываются")
	}
	if a.Y != 400 {
		t.Fatalf("a.Y=%v, ожидали 400 (верх рамки)", a.Y)
	}
	if z.X != 0 {
		t.Fatalf("z.X=%v, ожидали 0 (левый край рамки)", z.X)
	}
	if z.Y != a.Y+40 {
		t.Fatalf("z.Y=%v, ожидали %v (низ компоненты + зазор)", z.Y, a.Y+40)
	}
}

// Изолированный блок — компонента из одной вершины; одиночка — no-op.
func TestLayoutIsolated(t *testing.T) {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "a", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	b := d.AddBlock(&Block{ID: "b", X: 0, Y: 100, W: 30, H: 20, Pins: 2})
	z := d.AddBlock(&Block{ID: "z", X: 200, Y: 300, W: 30, H: 20, Pins: 2})
	d.AddLink(Endpoint{a.ID, 1}, Endpoint{b.ID, 0})
	if n, _ := Layout(d, nil, 10); n != 3 {
		t.Fatalf("разложено %d блоков, ожидали 3", n)
	}
	if HasOverlaps(d) {
		t.Fatal("изолированный блок накладывается на схему")
	}
	if z.X != 0 {
		t.Fatalf("z.X=%v, ожидали 0 (левый край рамки)", z.X)
	}

	solo := &Document{}
	s := solo.AddBlock(&Block{ID: "s", X: 50, Y: 60, W: 30, H: 20, Pins: 2})
	if n, layers := Layout(solo, nil, 10); n != 0 || layers != 0 {
		t.Fatalf("одиночка: Layout вернул (%d, %d), ожидали (0, 0)", n, layers)
	}
	if s.X != 50 || s.Y != 60 {
		t.Fatal("одиночный блок не должен двигаться")
	}
}

// Раскладка только выделения: остальные блоки не трогаем.
func TestLayoutSelectionOnly(t *testing.T) {
	d := chainDoc()
	z := d.AddBlock(&Block{ID: "z", X: 500, Y: 500, W: 30, H: 20, Pins: 2})
	c := d.BlockByID("c")
	cx, cy := c.X, c.Y
	n, _ := Layout(d, []string{"a", "b"}, 10)
	if n != 2 {
		t.Fatalf("разложено %d блоков, ожидали 2", n)
	}
	if c.X != cx || c.Y != cy {
		t.Fatal("блок вне выделения сдвинулся")
	}
	if z.X != 500 || z.Y != 500 {
		t.Fatal("посторонний блок сдвинулся")
	}
	if d.BlockByID("b").X <= d.BlockByID("a").X {
		t.Fatal("выделенная пара должна встать слева направо")
	}
}

// Результат не зависит от порядка блоков в документе и повторяем.
func TestLayoutDeterministic(t *testing.T) {
	build := func(order []string) *Document {
		pos := map[string][2]float32{
			"a": {0, 0}, "b": {0, 100}, "c": {40, 20}, "d": {90, 120},
		}
		d := &Document{}
		for _, id := range order {
			d.AddBlock(&Block{ID: id, X: pos[id][0], Y: pos[id][1], W: 30, H: 20, Pins: 2})
		}
		d.AddLink(Endpoint{"a", 1}, Endpoint{"b", 0})
		d.AddLink(Endpoint{"a", 1}, Endpoint{"c", 0})
		d.AddLink(Endpoint{"c", 1}, Endpoint{"d", 0})
		d.AddLink(Endpoint{"d", 1}, Endpoint{"b", 0})
		return d
	}
	one := build([]string{"a", "b", "c", "d"})
	two := build([]string{"d", "c", "b", "a"})
	Layout(one, nil, 10)
	Layout(two, nil, 10)
	for _, id := range []string{"a", "b", "c", "d"} {
		x, y := one.BlockByID(id), two.BlockByID(id)
		if x.X != y.X || x.Y != y.Y {
			t.Fatalf("%s: (%v, %v) против (%v, %v)", id, x.X, x.Y, y.X, y.Y)
		}
	}
	// повторный прогон на уже разложенной схеме ничего не сдвигает
	before := map[string][2]float32{}
	for _, b := range one.Blocks {
		before[b.ID] = [2]float32{b.X, b.Y}
	}
	Layout(one, nil, 10)
	for _, b := range one.Blocks {
		if before[b.ID] != [2]float32{b.X, b.Y} {
			t.Fatalf("раскладка не идемпотентна: %s был %v, стал (%v, %v)", b.ID, before[b.ID], b.X, b.Y)
		}
	}
}
