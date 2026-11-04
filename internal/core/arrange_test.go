package core

import (
	"testing"
)

func arrangeDoc() (*Document, []string) {
	d := &Document{}
	// целевой порядок входа перемешан — функции обязаны быть детерминированными
	d.AddBlock(&Block{ID: "c", X: 400, Y: 60, W: 120, H: 64})
	d.AddBlock(&Block{ID: "a", X: 100, Y: 300, W: 120, H: 64})
	d.AddBlock(&Block{ID: "b", X: 250, Y: 150, W: 120, H: 64})
	return d, []string{"c", "a", "b"}
}

// AlignLefts: левые края по крайнему левому (кап прицелу на сетку 0 — без snap).
func TestAlignLefts(t *testing.T) {
	d, ids := arrangeDoc()
	AlignLefts(d, ids, 10) // сетка 10: minX=100 уже кратен
	for _, id := range ids {
		if b := d.BlockByID(id); b.X != 100 {
			t.Fatalf("%s.X=%v, ожидали 100", id, b.X)
		}
	}
	// при grid=0 работает точно так же
	d2, ids2 := arrangeDoc()
	d2.AddBlock(&Block{ID: "z", X: 133, Y: 0, W: 120, H: 64})
	ids2 = append(ids2, "z")
	AlignLefts(d2, ids2, 0)
	if b := d2.BlockByID("z"); b.X != 100 {
		t.Fatalf("без сетки z.X=%v, ожидали 100", b.X)
	}
}

func TestAlignRights(t *testing.T) {
	d, ids := arrangeDoc()
	AlignRights(d, ids, 10) // maxX = 400+120=520
	for _, id := range ids {
		if b := d.BlockByID(id); b.X+b.W != 520 {
			t.Fatalf("%s правый край %v, ожидали 520", id, b.X+b.W)
		}
	}
}

func TestAlignTopsBottoms(t *testing.T) {
	d, ids := arrangeDoc()
	AlignTops(d, ids, 10) // minY=60
	for _, id := range ids {
		if b := d.BlockByID(id); b.Y != 60 {
			t.Fatalf("%s.Y=%v, ожидали 60", id, b.Y)
		}
	}
	d2, ids2 := arrangeDoc()
	AlignBottoms(d2, ids2, 0) // без сетки: maxY = 300+64=364 точно
	for _, id := range ids2 {
		if b := d2.BlockByID(id); b.Y+b.H != 364 {
			t.Fatalf("%s нижний край %v, ожидали 364", id, b.Y+b.H)
		}
	}
}

// DistributeX: центры через равные шаги; крайние не двигаются.
func TestDistributeX(t *testing.T) {
	d, ids := arrangeDoc()
	// блок с центрами c: 460, 160, 310 → сортировка: a(160), b(310), c(460)
	DistributeX(d, ids, 10)
	// центры должны остаться равными (шаг уже равный: 150)
	for id, want := range map[string]float32{"a": 160, "b": 310, "c": 460} {
		b := d.BlockByID(id)
		if got := b.X + b.W/2; got != want {
			t.Fatalf("%s центр %v, ожидали %v", id, got, want)
		}
	}
	// неравные промежутки → выравнивание шага
	d2 := &Document{}
	d2.AddBlock(&Block{ID: "p", X: 0, Y: 0, W: 80, H: 40})
	d2.AddBlock(&Block{ID: "q", X: 30, Y: 100, W: 80, H: 40}) // центр 70 — почти у p
	d2.AddBlock(&Block{ID: "r", X: 500, Y: 50, W: 80, H: 40}) // центр 540
	DistributeX(d2, []string{"p", "q", "r"}, 0)
	// шаг центров = (540-40)/2 = 250
	if c := d2.BlockByID("q").X + 40; c != 290 {
		t.Fatalf("q центр %v, ожидали 290", c)
	}
	if d2.BlockByID("p").X != 0 || d2.BlockByID("r").X != 500 {
		t.Fatal("крайние блоки должны оставаться на месте")
	}
}

// Мало блоков — no-op.
func TestArrangeCardinality(t *testing.T) {
	d := &Document{}
	a := d.AddBlock(&Block{ID: "a", X: 50, Y: 50, W: 120, H: 64})
	AlignLefts(d, []string{"a"}, 0)
	DistributeX(d, []string{"a"}, 0)
	DistributeY(d, []string{"a"}, 0)
	if a.X != 50 || a.Y != 50 {
		t.Fatal("одиночка должен быть no-op")
	}
}

// Центрирование: к среднему центров.
func TestAlignCenters(t *testing.T) {
	d := &Document{}
	d.AddBlock(&Block{ID: "a", X: 0, Y: 0, W: 100, H: 50})
	d.AddBlock(&Block{ID: "b", X: 200, Y: 100, W: 100, H: 50})
	ids := []string{"a", "b"}
	AlignCentersX(d, ids, 0)
	// средний центр X = (50+250)/2 = 150
	if a := d.BlockByID("a"); a.X+a.W/2 != 150 {
		t.Fatalf("a центр X=%v", a.X+a.W/2)
	}
	if b := d.BlockByID("b"); b.X+b.W/2 != 150 {
		t.Fatalf("b центр X=%v", b.X+b.W/2)
	}
}
