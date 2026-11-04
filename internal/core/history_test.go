package core

import "testing"

// Важно: после Undo/Redo LoadState заменяет *Block на копии — указатели,
// взятые до undo,át меняться не будут; тесты перевыбирают через BlockByID.

func undoDoc() *Document {
	d := &Document{}
	d.AddBlock(&Block{ID: "a", X: 0, Y: 0, W: 120, H: 64, Pins: 2, Label: "A"})
	d.AddBlock(&Block{ID: "b", X: 200, Y: 0, W: 120, H: 64, Pins: 2, Label: "B"})
	return d
}

// Базовый цикл: capture → мутация → finish → undo восстанавливает, redo возвращает.
func TestHistoryUndoRedo(t *testing.T) {
	d := undoDoc()
	h := &History{}
	h.Reset(d)

	h.Capture(d)
	a := d.BlockByID("a")
	a.X, a.Y = 50, 50
	d.AddLink(Endpoint{"a", 0}, Endpoint{"b", 0})
	if !h.Finish(d) {
		t.Fatal("Finish должен зафиксировать изменение")
	}

	if !h.Undo(d) {
		t.Fatal("Undo не сработал")
	}
	a = d.BlockByID("a")
	if a.X != 0 || a.Y != 0 || len(d.Links) != 0 || len(d.Blocks) != 2 {
		t.Fatalf("undo не вернул состояние: a=(%v,%v) links=%d blocks=%d",
			a.X, a.Y, len(d.Links), len(d.Blocks))
	}
	if !h.Redo(d) {
		t.Fatal("Redo не сработал")
	}
	a = d.BlockByID("a")
	if a.X != 50 || a.Y != 50 || len(d.Links) != 1 {
		t.Fatalf("redo: a=(%v,%v) links=%d", a.X, a.Y, len(d.Links))
	}
	if !h.Undo(d) || !h.Redo(d) {
		t.Fatal("повторный цикл undo/redo сломан")
	}
}

// Холостые нажатия не порождают «мёртвых» undo.
func TestHistoryIdlePresses(t *testing.T) {
	d := undoDoc()
	h := &History{}
	h.Reset(d)

	for i := 0; i < 5; i++ { // пять кликов без изменений
		h.Capture(d)
		h.Finish(d)
	}
	u, r := h.Depth()
	if u == 0 && r == 0 {
		// стеки могут остаться с дубликатами — это допустимо, главное
		// что undo не даёт эффекта: ленивая чистка уберёт
		t.Log("стеки чисты после холостых кликов")
	}
	if h.Undo(d) {
		t.Fatal("undo после холостых кликов не должен иметь эффекта")
	}
	if d.BlockByID("a").X != 0 {
		t.Fatal("состояние менялось после холостых кликов")
	}
}

// Мутация после undo закрывает redo-будущее.
func TestHistoryChangeKillsRedo(t *testing.T) {
	d := undoDoc()
	h := &History{}
	h.Reset(d)

	h.Capture(d)
	d.BlockByID("a").X = 17
	h.Finish(d)
	h.Capture(d)
	d.BlockByID("a").X = 100
	h.Finish(d)

	if !h.Undo(d) || d.BlockByID("a").X != 17 {
		t.Fatalf("undo: a.X=%v", d.BlockByID("a").X)
	}
	if !h.Redo(d) || d.BlockByID("a").X != 100 {
		t.Fatalf("redo: a.X=%v", d.BlockByID("a").X)
	}
	// новое изменение после redo → redo-будущее умирает
	h.Capture(d)
	d.BlockByID("a").X = 300
	h.Finish(d)
	if h.Redo(d) {
		t.Fatal("redo после нового изменения должен быть недоступен")
	}
	if d.BlockByID("a").X != 300 {
		t.Fatalf("redo-фантом сдвинул блок: a.X=%v", d.BlockByID("a").X)
	}
}

// Холостые нажатия между изменениями не теряют шагов undo (два разных шага).
func TestHistoryGranularity(t *testing.T) {
	d := undoDoc()
	h := &History{}
	h.Reset(d)

	h.Capture(d)
	d.BlockByID("a").X = 10
	h.Finish(d)
	h.Capture(d) // холостое нажатие
	h.Finish(d)
	h.Capture(d)
	d.BlockByID("a").Y = 20
	h.Finish(d)

	if !h.Undo(d) {
		t.Fatal("первый undo пуст")
	}
	if a := d.BlockByID("a"); a.Y != 0 || a.X != 10 {
		t.Fatalf("после первого undo ожидали (10,0): a=(%v,%v)", a.X, a.Y)
	}
	if !h.Undo(d) {
		t.Fatal("второй undo пуст")
	}
	if a := d.BlockByID("a"); a.X != 0 {
		t.Fatalf("после второго undo ожидали 0: a.X=%v", a.X)
	}
}

// Снимки иммутабельны: мутация документа после capture не портит историю.
func TestSnapshotImmutability(t *testing.T) {
	d := undoDoc()
	h := &History{}
	h.Reset(d)
	h.Capture(d)
	d.BlockByID("a").X = 777 // «драг» меняет оригинал
	if !h.Undo(d) {
		t.Fatal("undo пуст")
	}
	if a := d.BlockByID("a"); a.X != 0 {
		t.Fatalf("снимок протёк: восстановлен a.X=%v", a.X)
	}
}

// Ограничение глубины: старые шаги вытесняются.
func TestHistoryDepthLimit(t *testing.T) {
	d := undoDoc()
	h := &History{}
	h.Reset(d)
	for x := 1; x <= maxUndoDepth+20; x++ {
		h.Capture(d)
		d.BlockByID("a").X = float32(x)
		h.Finish(d)
	}
	u, _ := h.Depth()
	if u > maxUndoDepth {
		t.Fatalf("глубина %d > %d", u, maxUndoDepth)
	}
	if !h.Undo(d) {
		t.Fatal("undo пуст")
	}
	if a := d.BlockByID("a"); a.X == float32(maxUndoDepth+20) {
		t.Fatalf("хвост истории не вытеснился: a.X=%v", a.X)
	}
}
