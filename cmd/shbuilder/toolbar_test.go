package main

import (
	"testing"

	"shbuilder/internal/core"
	"shbuilder/internal/geom"
)

// toolTestEditor — минимальный editor для тестов тулбара: без окна и темы.
func toolTestEditor() *editor {
	ed := &editor{
		doc:       &core.Document{},
		cam:       geom.Camera{Zoom: pxPerMM},
		grid:      10,
		gridIdx:   1,
		viewW:     1000,
		viewH:     700,
		toolPress: -1,
		toolHover: -1,
	}
	ed.history.Reset(ed.doc)
	return ed
}

// TestToolbarRects — кнопки идут слева направо без наложений, в порядке
// таблицы, каждая внутри полосы по вертикали, все влезают в 1000 px.
func TestToolbarRects(t *testing.T) {
	rects := toolbarRects(1000)
	if len(rects) != len(toolButtons) {
		t.Fatalf("кнопок %d, ожидали %d", len(rects), len(toolButtons))
	}
	prevMax := float32(-1)
	for i, r := range rects {
		if r.Size().X <= 0 || r.Size().Y <= 0 {
			t.Fatalf("кнопка %d: пустой прямоугольник %v", i, r)
		}
		if r.Min.X < prevMax {
			t.Fatalf("кнопка %d налезает на предыдущую: %v (prevMax %v)", i, r, prevMax)
		}
		if r.Min.Y < 0 || r.Max.Y > toolbarH {
			t.Fatalf("кнопка %d вылезла за полосу: %v", i, r)
		}
		if r.Max.X > 1000 {
			t.Fatalf("кнопка %d не влезла в окно 1000: %v", i, r)
		}
		if r.Size().X != toolButtons[i].w {
			t.Fatalf("кнопка %d: ширина %v, ожидали %v", i, r.Size().X, toolButtons[i].w)
		}
		prevMax = r.Max.X
	}
	if rects[0].Min.X != toolPadX {
		t.Fatalf("первая кнопка не на отступе: %v", rects[0].Min.X)
	}
}

// TestToolbarHit — попадание в кнопку, в зазор, ниже полосы и за краем окна.
func TestToolbarHit(t *testing.T) {
	rects := toolbarRects(1000)
	for i, r := range rects {
		mid := geom.Vec{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
		if got := toolbarHit(mid, 1000); got != i {
			t.Fatalf("центр кнопки %d: toolbarHit(%v) = %d", i, mid, got)
		}
	}
	// Зазор между первой и второй кнопкой — мимо.
	gap := geom.Vec{X: (rects[0].Max.X + rects[1].Min.X) / 2, Y: toolbarH / 2}
	if got := toolbarHit(gap, 1000); got != -1 {
		t.Fatalf("зазор между кнопками: toolbarHit(%v) = %d, ожидали -1", gap, got)
	}
	// Ниже полосы — мимо (канва).
	if got := toolbarHit(geom.Vec{X: 20, Y: toolbarH}, 1000); got != -1 {
		t.Fatalf("точка ниже полосы: toolbarHit = %d, ожидали -1", got)
	}
	// Точка в полосе, но правее последней кнопки — мимо.
	last := rects[len(rects)-1]
	if got := toolbarHit(geom.Vec{X: last.Max.X + 20, Y: toolbarH / 2}, 1000); got != -1 {
		t.Fatalf("правее кнопок: toolbarHit = %d, ожидали -1", got)
	}
	// Узкое окно: кнопка, вылезшая за край, недоступна.
	if got := toolbarHit(geom.Vec{X: 199, Y: toolbarH / 2}, 200); got != -1 {
		t.Fatalf("кнопка за краем узкого окна: toolbarHit = %d, ожидали -1", got)
	}
	if inToolbar(geom.Vec{X: 10, Y: toolbarH}) {
		t.Fatalf("точка ровно на кромке снизу не должна считаться полосой")
	}
}

// TestToolEnabled — серые кнопки: undo/redo по глубине истории, раскладка
// требует ≥ 2 блоков, во время драга недоступно всё.
func TestToolEnabled(t *testing.T) {
	ed := toolTestEditor()
	if toolEnabled(ed, toolUndo) || toolEnabled(ed, toolRedo) {
		t.Fatalf("пустая история: undo/redo должны быть недоступны")
	}
	if toolEnabled(ed, toolLayout) {
		t.Fatalf("0 блоков: раскладка недоступна")
	}
	ed.doc.AddBlock(&core.Block{ID: "A", Kind: "block", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	if toolEnabled(ed, toolLayout) {
		t.Fatalf("1 блок: раскладка недоступна")
	}
	ed.doc.AddBlock(&core.Block{ID: "B", Kind: "block", X: 100, Y: 0, W: 30, H: 20, Pins: 2})
	if !toolEnabled(ed, toolLayout) {
		t.Fatalf("2 блока: раскладка доступна")
	}

	// Один undo-шаг: undo доступен, redo нет.
	ed.history.Reset(ed.doc)
	ed.history.Capture(ed.doc)
	ed.doc.AddBlock(&core.Block{ID: "C", Kind: "block", X: 200, Y: 0, W: 30, H: 20, Pins: 2})
	ed.history.Finish(ed.doc)
	if !toolEnabled(ed, toolUndo) || toolEnabled(ed, toolRedo) {
		t.Fatalf("после шага: undo=true redo=false")
	}
	toolRun(ed, toolUndo)
	if toolEnabled(ed, toolUndo) || !toolEnabled(ed, toolRedo) {
		t.Fatalf("после undo: undo=false redo=true")
	}

	// Драг: всё серое.
	ed.mode = modeDragBlock
	if toolEnabled(ed, toolSave) || toolEnabled(ed, toolZoomIn) || toolEnabled(ed, toolRedo) {
		t.Fatalf("во время драга кнопки должны быть недоступны")
	}
}

// TestToolRunUndoRedo — кнопки undo/redo ходят по истории как хоткеи.
func TestToolRunUndoRedo(t *testing.T) {
	ed := toolTestEditor()
	ed.history.Capture(ed.doc)
	ed.doc.AddBlock(&core.Block{ID: "A", Kind: "block", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	ed.history.Finish(ed.doc)

	toolRun(ed, toolUndo)
	if len(ed.doc.Blocks) != 0 {
		t.Fatalf("после undo блоков %d, ожидали 0", len(ed.doc.Blocks))
	}
	toolRun(ed, toolRedo)
	if len(ed.doc.Blocks) != 1 {
		t.Fatalf("после redo блоков %d, ожидали 1", len(ed.doc.Blocks))
	}
}

// TestToolRunZoomGrid — кнопки зума и цикл сетки совпадают с хоткеями.
func TestToolRunZoomGrid(t *testing.T) {
	ed := toolTestEditor()
	ed.cam.Zoom = pxPerMM
	toolRun(ed, toolZoomIn)
	if ed.cam.Zoom <= pxPerMM {
		t.Fatalf("зум + не увеличил масштаб: %v", ed.cam.Zoom)
	}
	toolRun(ed, toolZoom100)
	if ed.cam.Zoom != pxPerMM {
		t.Fatalf("зум 100%%: %v, ожидали %v", ed.cam.Zoom, pxPerMM)
	}
	toolRun(ed, toolZoomOut)
	if ed.cam.Zoom >= pxPerMM {
		t.Fatalf("зум – не уменьшил масштаб: %v", ed.cam.Zoom)
	}

	ed.gridIdx, ed.grid = 0, gridChoices[0]
	for want := 1; want <= len(gridChoices); want++ {
		toolRun(ed, toolGrid)
		wantIdx := want % len(gridChoices)
		if ed.gridIdx != wantIdx || ed.grid != gridChoices[wantIdx] {
			t.Fatalf("цикл сетки: idx=%d grid=%v, ожидали %d %v",
				ed.gridIdx, ed.grid, wantIdx, gridChoices[wantIdx])
		}
	}
}

// TestToolRunDelete — кнопка «удалить» убирает выделение одним undo-шагом.
func TestToolRunDelete(t *testing.T) {
	ed := toolTestEditor()
	ed.doc.AddBlock(&core.Block{ID: "A", Kind: "block", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	ed.doc.AddBlock(&core.Block{ID: "B", Kind: "block", X: 100, Y: 0, W: 30, H: 20, Pins: 2})
	ed.history.Reset(ed.doc)
	ed.sel = map[string]bool{"A": true, "B": true}

	toolRun(ed, toolDelete)
	if len(ed.doc.Blocks) != 0 {
		t.Fatalf("после удаления блоков %d, ожидали 0", len(ed.doc.Blocks))
	}
	if undoN, _ := ed.history.Depth(); undoN != 1 {
		t.Fatalf("удаление дало %d undo-шагов, ожидали 1", undoN)
	}
	toolRun(ed, toolUndo)
	if len(ed.doc.Blocks) != 2 {
		t.Fatalf("после undo блоков %d, ожидали 2", len(ed.doc.Blocks))
	}
}

// TestToolRunLayout — кнопка «раскладка» раскладывает блоки без наложений.
func TestToolRunLayout(t *testing.T) {
	ed := toolTestEditor()
	ed.doc.AddBlock(&core.Block{ID: "A", Kind: "block", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	ed.doc.AddBlock(&core.Block{ID: "B", Kind: "block", X: 0, Y: 0, W: 30, H: 20, Pins: 2})
	ed.history.Reset(ed.doc)

	toolRun(ed, toolLayout)
	if core.HasOverlaps(ed.doc) {
		t.Fatalf("раскладка оставила наложения: %v", ed.doc.Blocks)
	}
	if a, b := ed.doc.BlockByID("A"), ed.doc.BlockByID("B"); a == nil || b == nil || (a.X == b.X && a.Y == b.Y) {
		t.Fatalf("блоки не разъехались: A=%v B=%v", a, b)
	}
	if undoN, _ := ed.history.Depth(); undoN != 1 {
		t.Fatalf("раскладка дала %d undo-шагов, ожидали 1", undoN)
	}
}
