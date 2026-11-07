package main

import (
	"testing"

	"shbuilder/internal/core"
	"shbuilder/internal/geom"
)

// Тест: любое упорядочивание (T/H/V — края/центры, D — distribute) должно
// через PushOverlaps снимать сведшие блоки наложения — инвариант «блоки не
// пересекаются» не должен ломаться на этапе 1.4.
func TestArrangeNoOverlaps(t *testing.T) {
	for _, key := range []string{"T", "H", "V", "D"} {
		ed := &editor{doc: demoScene(), grid: 10, gridIdx: 1}
		ed.history.Reset(ed.doc)
		ed.sel = map[string]bool{}
		for _, b := range ed.doc.Blocks {
			ed.sel[b.ID] = true
		}
		arrangeKey(key, false, ed)
		if core.HasOverlaps(ed.doc) {
			t.Fatalf("arrange %q оставил перекрытия (%s)", key, ed.msg)
		}
	}
}

// Тест: сброс (drag-release → MoveBlock + pushOverlaps) не должен оставлять
// блока на месте другого.
func TestDropNoOverlap(t *testing.T) {
	ed := &editor{doc: demoScene(), grid: 10, gridIdx: 1}
	ed.history.Reset(ed.doc)
	ed.sel = map[string]bool{"A": true}
	ed.doc.MoveBlock("A", 120, 60) // старая позиция A (40,40) и B (120,60)
	ed.pushOverlaps("A")
	if core.HasOverlaps(ed.doc) {
		t.Fatalf("drop не разрешил перекрытие: %v", ed.doc.Blocks)
	}
}

// Тест: вставка блока из палитры/хоткея в занятый центр не должна оставлять
// наложения — блок ищет свободную позицию (или отодвигает другой).
func TestInsertNoOverlap(t *testing.T) {
	ed := &editor{
		doc:   &core.Document{},
		cam:   geom.Camera{Zoom: pxPerMM},
		grid:  10,
		viewW: 1000, viewH: 700,
	}
	ed.history.Reset(ed.doc)
	ed.doc.AddBlock(&core.Block{ID: "X", Kind: "src", X: 140, Y: 90, W: 30, H: 20, Label: "X"})
	ed.history.Reset(ed.doc)
	b := addPinBlockAt(ed, 2, ed.canvasInsertScreen()) // в центр канвы ≈ (140, 90) — занято
	if b == nil || core.HasOverlaps(ed.doc) {
		t.Fatalf("вставка не разрешила перекрытие: block=%v overlaps=%v", b, core.HasOverlaps(ed.doc))
	}
}
