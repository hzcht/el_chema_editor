package main

import (
	"testing"

	"shbuilder/internal/core"
	"shbuilder/internal/geom"
)

// TestPaletteHit — строки палитры: i-я строка даёт блок с n=i+1 пинами;
// заголовок и область правее палитры блока не вставляют.
func TestPaletteHit(t *testing.T) {
	for i := 0; i < 6; i++ {
		r := paletteRowRect(i)
		mid := geom.Vec{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
		if n, ok := paletteHit(mid); !ok || n != i+1 {
			t.Fatalf("строка %d: paletteHit(%v) = %d,%v; ожидали %d", i, mid, n, ok, i+1)
		}
		if !inPalette(mid) {
			t.Fatalf("строка %d не попала в полосу палитры", i)
		}
	}
	if _, ok := paletteHit(geom.Vec{X: 20, Y: paletteTitle / 2}); ok {
		t.Fatalf("заголовок палитры не должен вставлять блок")
	}
	outside := geom.Vec{X: paletteW + 20, Y: 200}
	if inPalette(outside) {
		t.Fatalf("точка правее палитры ошибочно считается палитрой")
	}
	if _, ok := paletteHit(outside); ok {
		t.Fatalf("канва не должна попадать в строки палитры")
	}
}

func TestPinWord(t *testing.T) {
	want := map[int]string{1: "пин", 2: "пина", 3: "пина", 4: "пина", 5: "пинов", 6: "пинов"}
	for n, w := range want {
		if got := pinWord(n); got != w {
			t.Fatalf("pinWord(%d) = %q, ожидали %q", n, got, w)
		}
	}
}

// TestPaletteInsertCenter — вставка из палитры/по хоткею при курсоре над
// палитрой ставит блок в центр видимой канвы (не под палитру), с n пинами,
// и создаёт undo-шаг. Курсор над канвой — вставка под курсором.
func TestPaletteInsertCenter(t *testing.T) {
	ed := &editor{
		doc:   &core.Document{},
		cam:   geom.Camera{Zoom: pxPerMM},
		grid:  10,
		viewW: 1000, viewH: 700,
	}
	ed.history.Reset(ed.doc)
	ed.cursor = geom.Vec{X: 40, Y: paletteRowRect(2).Min.Y + 10} // строка «3 пина»

	n, ok := paletteHit(ed.cursor)
	if !ok || n != 3 {
		t.Fatalf("paletteHit(%v) = %d,%v; ожидали 3,true", ed.cursor, n, ok)
	}
	b := addPinBlockAt(ed, n, ed.canvasInsertScreen())
	if b.Pins != 3 {
		t.Fatalf("пинов %d, ожидали 3", b.Pins)
	}
	wantCX := (paletteW + ed.viewW) / 2
	wantCY := toolbarH + (ed.viewH-toolbarH)/2
	c := ed.cam.ScreenToWorld(geom.Vec{X: wantCX, Y: wantCY})
	wantX := geom.Snap(c.X-15, ed.grid)
	wantY := geom.Snap(c.Y-10, ed.grid)
	if b.X != wantX || b.Y != wantY {
		t.Fatalf("блок в (%v,%v), ожидали центр канвы (%v,%v)", b.X, b.Y, wantX, wantY)
	}
	if len(ed.doc.Blocks) != 1 {
		t.Fatalf("блоков %d, ожидали 1", len(ed.doc.Blocks))
	}
	if undoN, _ := ed.history.Depth(); undoN != 1 {
		t.Fatalf("undo-шагов %d, ожидали 1", undoN)
	}

	// Хоткей при курсоре над палитрой — тот же центр канвы.
	ed.cursor = geom.Vec{X: 20, Y: 60}
	if p := ed.canvasInsertScreen(); p.X != wantCX || p.Y != wantCY {
		t.Fatalf("курсор над палитрой: точка вставки (%v,%v), ожидали (%v,%v)", p.X, p.Y, wantCX, wantCY)
	}

	// Курсор над канвой — вставка под курсором (как раньше).
	ed.cursor = geom.Vec{X: 500, Y: 300}
	if p := ed.canvasInsertScreen(); p != ed.cursor {
		t.Fatalf("курсор над канвой: точка вставки %v, ожидали %v", p, ed.cursor)
	}
}
