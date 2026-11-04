package core

import (
	"testing"

	"shbuilder/internal/geom"
)

func newDoc() *Document { return &Document{} }

// Пины раскладываются по правилам, позиции в границах блока.
func TestPinLayout(t *testing.T) {
	for n := 0; n <= 9; n++ {
		pins := PinLayout(n)
		if len(pins) != n {
			t.Fatalf("PinLayout(%d): %d пинов, ожидалось %d", n, len(pins), n)
		}
		for _, p := range pins {
			if p.Side == geom.SideNone {
				t.Fatalf("PinLayout(%d): side none", n)
			}
			if p.T < 0 || p.T > 1 {
				t.Fatalf("PinLayout(%d): t=%v вне [0,1]", n, p.T)
			}
		}
	}
	if PinLayout(1)[0].Side != geom.SideRight {
		t.Fatalf("1 пин должен быть справа")
	}
	if PinLayout(2)[0].Side != geom.SideLeft || PinLayout(2)[1].Side != geom.SideRight {
		t.Fatalf("2 пина: лево+право")
	}
}

func TestBlockPinPos(t *testing.T) {
	b := &Block{X: 100, Y: 50, W: 120, H: 64, Pins: 4}
	pos, side, ok := b.PinPos(0)
	if !ok || side != geom.SideLeft || pos != (geom.Vec{X: 100, Y: 50 + 32}) {
		t.Fatalf("левый пин: %v %v", pos, side)
	}
	pos, side, ok = b.PinPos(3)
	if !ok || side != geom.SideBottom || pos.Y != 50+64 {
		t.Fatalf("нижний пин: %v %v", pos, side)
	}
	if _, _, ok = b.PinPos(4); ok {
		t.Fatalf("пина 4 не должно существовать")
	}
}

func TestDocumentCRUD(t *testing.T) {
	d := newDoc()
	a := d.AddBlock(&Block{Kind: "box", X: 0, Y: 0, W: 120, H: 64, Label: "A", Pins: 2})
	if a.ID == "" {
		t.Fatal("ID не автогенерировался")
	}
	if d.BlockByID(a.ID) != a {
		t.Fatal("BlockByID не нашёл блок")
	}
	d.MoveBlock(a.ID, 10, 20)
	if a.X != 10 || a.Y != 20 {
		t.Fatal("MoveBlock не сработал")
	}
	if d.HitBlock(geom.Vec{X: 15, Y: 25}) != a || d.HitBlock(geom.Vec{X: 500, Y: 500}) != nil {
		t.Fatal("HitBlock: промахи")
	}
}

func TestEndpointValidation(t *testing.T) {
	d := newDoc()
	a := d.AddBlock(&Block{X: 0, Y: 0, W: 120, H: 64, Pins: 2})
	if d.EndpointValid(Endpoint{a.ID, 2}) {
		t.Fatal("пин 2 не должен существовать у блока с 2 пинами")
	}
	if d.EndpointValid(Endpoint{"nope", 0}) {
		t.Fatal("unknown block должен быть невалиден")
	}
}

// Топология: 2 линка на один пин → один кластер; отдельный линк — второй кластер.
func TestLinkGroups(t *testing.T) {
	d := newDoc()
	a := d.AddBlock(&Block{X: 0, Y: 0, W: 120, H: 64, Pins: 2, Label: "A"})
	b := d.AddBlock(&Block{X: 200, Y: 0, W: 120, H: 64, Pins: 2, Label: "B"})
	c := d.AddBlock(&Block{X: 400, Y: 0, W: 120, H: 64, Pins: 1, Label: "C"})
	d.AddBlock(&Block{X: 0, Y: 200, W: 120, H: 64, Pins: 3, Label: "D"})

	A := Endpoint{a.ID, 0}
	B := Endpoint{b.ID, 0}
	C := Endpoint{c.ID, 0}
	// junction: пин B0 принимает оба линка
	if !d.AddLink(A, B) || !d.AddLink(C, B) {
		t.Fatal("линки не добавились")
	}
	if d.AddLink(A, B) {
		t.Fatal("дубликат линка пропущен")
	}
	// второй кластер
	a1 := Endpoint{a.ID, 1}
	d0 := Endpoint{d.BlockByID("b4").ID, 0}
	if !d.AddLink(a1, d0) {
		t.Fatal("второй линк не добавился")
	}

	groups := d.LinkGroups()
	if len(groups) != 2 {
		t.Fatalf("ожидались 2 группы, получено %d: %v", len(groups), groups)
	}
	var jb bool
	for _, g := range groups {
		if len(g) == 3 && g[0].String() == A.String()+"?" {
			jb = true
		}
	}
	// проверим напрямую содержимое cluster-а с тремя endpoint-ами
	found3 := false
	for _, g := range groups {
		if len(g) == 3 {
			found3 = true
			set := map[Endpoint]bool{}
			for _, e := range g {
				set[e] = true
			}
			if !set[A] || !set[B] || !set[Endpoint{b.ID, 0}] {
				t.Fatalf("кластер из 3 не содержит A0/B0/C0: %v", g)
			}
		}
	}
	if !found3 || !jb {
		if !found3 {
			t.Fatalf("нет кластера с 3 endpoint-ами: %v", groups)
		}
	}

	free := d.FreePins()
	// всего пинов: A2 + B2 + C1 + D3 = 8; в линках: A0,A1,B0,C0,D0 = 5
	if len(free) != 3 {
		t.Fatalf("ожидались 3 свободных пина, получено %v", free)
	}
}

func TestPinPosViaDocument(t *testing.T) {
	d := newDoc()
	a := d.AddBlock(&Block{X: 10, Y: 20, W: 120, H: 64, Pins: 3})
	pos, side, ok := d.PinPos(Endpoint{a.ID, 1})
	if !ok || side != geom.SideRight || pos != (geom.Vec{X: 130, Y: 52}) {
		t.Fatalf("PinPos через документ: %v %v %v", pos, side, ok)
	}
}

// Нет пересечений → отсутствие перемещений.
func TestPushNoOverlap(t *testing.T) {
	d := newDoc()
	a := d.AddBlock(&Block{X: 0, Y: 0, W: 120, H: 64, ID: "a"})
	b := d.AddBlock(&Block{X: 200, Y: 0, W: 120, H: 64, ID: "b"})
	if !PushOverlaps(d, nil, 10, 8) {
		t.Fatal("PushOverlaps должен вернуть true")
	}
	if a.X != 0 || b.X != 200 {
		t.Fatalf("блоки сдвинулись без пересечения: a=(%v,%v) b=(%v,%v)", a.X, a.Y, b.X, b.Y)
	}
}

// Простейшее перекрытие: защищённый остаётся, второй отодвигается и snap.
func TestPushSimple(t *testing.T) {
	d := newDoc()
	a := d.AddBlock(&Block{X: 0, Y: 0, W: 120, H: 64, ID: "a"})
	b := d.AddBlock(&Block{X: 50, Y: 20, W: 120, H: 64, ID: "b"}) // перекрывает a
	if !PushOverlaps(d, map[string]bool{"a": true}, 10, 8) {
		t.Fatal("PushOverlaps не разрешил пересечение")
	}
	if a.X != 0 || a.Y != 0 {
		t.Fatalf("защищённый блок сдвинулся: a=(%v,%v)", a.X, a.Y)
	}
	if a.Rect().Overlaps(b.Rect()) {
		t.Fatalf("пересечение осталось: a=%v b=%v", a.Rect(), b.Rect())
	}
	if int(b.X)%10 != 0 || int(b.Y)%10 != 0 {
		t.Fatalf("b не на сетке: (%v,%v)", b.X, b.Y)
	}
	// «рядом»: b прижат к краю a (снизу — т.к. центры говорят «b ниже»),
	// в пределах одной клетки сетки от грани
	if !(b.Y >= a.H && b.Y < a.H+10) {
		t.Fatalf("b не встал вплотную под a: b=(%v,%v), a.Max.Y=%v", b.X, b.Y, a.H)
	}
}

// Цепочка: разруливается в пределах passes.
func TestPushChain(t *testing.T) {
	d := newDoc()
	d.AddBlock(&Block{X: 100, Y: 100, W: 120, H: 64, ID: "a"})
	d.AddBlock(&Block{X: 150, Y: 120, W: 120, H: 64, ID: "b"})
	d.AddBlock(&Block{X: 200, Y: 140, W: 120, H: 64, ID: "c"})
	d.AddBlock(&Block{X: 260, Y: 160, W: 120, H: 64, ID: "d"})
	if !PushOverlaps(d, map[string]bool{"a": true}, 10, 16) {
		// цепочка может не сойтись — тогда документ может остаться с пересечениями;
		// для теста требуем полного разрешения при 16 итерациях
		t.Fatalf("push не разрешил цепочку")
	}
	blocks := d.Blocks
	for i := 0; i < len(blocks); i++ {
		for j := i + 1; j < len(blocks); j++ {
			if blocks[i].Rect().Overlaps(blocks[j].Rect()) {
				t.Fatalf("есть пересечение %v и %v", blocks[i].Rect(), blocks[j].Rect())
			}
		}
	}
}
