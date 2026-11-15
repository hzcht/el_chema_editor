package core

// История undo/redo по снимкам состояния (ADR-0004): составные операции этапа 1
// (drag + snap + push-apart) дешевле и надёжнее хранить целиком. Границы:
// Capture — до мутации (по нажатию), Finish — по отпуску/мутации клавишей.
// Разбор — docs/ALGORITHMS.md §5.

// maxUndoDepth — предел на каждый из стеков.
const maxUndoDepth = 200

// Snapshot — неизменяемая копия содержимого документа.
type Snapshot struct {
	Blocks []*Block
	Links  []Link
	Seq    int
}

// SnapshotState — глубокая копия текущего состояния (блоки копируются по значению).
func (d *Document) SnapshotState() Snapshot {
	blocks := make([]*Block, len(d.Blocks))
	for i, b := range d.Blocks {
		c := *b
		blocks[i] = &c
	}
	links := make([]Link, len(d.Links))
	copy(links, d.Links)
	return Snapshot{Blocks: blocks, Links: links, Seq: d.seq}
}

// LoadState загружает снимок внутрь документа копией (снимок остаётся неизменным).
func (d *Document) LoadState(s Snapshot) {
	blocks := make([]*Block, len(s.Blocks))
	for i, b := range s.Blocks {
		c := *b
		blocks[i] = &c
	}
	links := make([]Link, len(s.Links))
	copy(links, s.Links)
	d.Blocks, d.Links, d.seq = blocks, links, s.Seq
}

// History — undo/redo стеки + pending-состояние «нажатие → отпуск».
type History struct {
	undoStack, redoStack []Snapshot
	pending              *Snapshot
}

// Reset очищает историю и делает текущее состояние точкой отсчёта
// (начало работы, сразу после load).
func (h *History) Reset(d *Document) {
	h.undoStack, h.redoStack, h.pending = nil, nil, nil
}

// Capture запоминает состояние в undo-стек (без очистки redo: пользователь
// мог просто кликнуть без изменения). Вызывать ДО мутации. Глубина ограничена.
func (h *History) Capture(d *Document) {
	s := d.SnapshotState()
	h.undoStack = append(h.undoStack, s)
	if len(h.undoStack) > maxUndoDepth {
		h.undoStack = h.undoStack[len(h.undoStack)-maxUndoDepth:]
	}
	h.pending = &s
}

// Finish закрывает операцию (Release/Cancel или сразу после мутации клавишей).
// Возвращает true, если состояние изменилось — тогда redo-будущее умирает
// (стандартная семантика undo). Холостое нажатие не коммитит ничего.
func (h *History) Finish(d *Document) bool {
	if h.pending == nil {
		return false
	}
	pre := *h.pending
	h.pending = nil
	if snapshotEq(pre, d.SnapshotState()) {
		return false
	}
	h.redoStack = nil
	return true
}

// Undo загружает предыдущее состояние; дубли current наверху стека (холостые
// нажатия) счищаются лениво. false — реальной истории нет.
func (h *History) Undo(d *Document) bool {
	cur := d.SnapshotState()
	for len(h.undoStack) > 0 && snapshotEq(h.undoStack[len(h.undoStack)-1], cur) {
		h.undoStack = h.undoStack[:len(h.undoStack)-1]
	}
	if len(h.undoStack) == 0 {
		return false
	}
	s := h.undoStack[len(h.undoStack)-1]
	h.undoStack = h.undoStack[:len(h.undoStack)-1]
	d.LoadState(s)
	h.redoStack = append(h.redoStack, cur)
	if len(h.redoStack) > maxUndoDepth {
		h.redoStack = h.redoStack[len(h.redoStack)-maxUndoDepth:]
	}
	return true
}

// Redo — противоположно Undo.
func (h *History) Redo(d *Document) bool {
	cur := d.SnapshotState()
	for len(h.redoStack) > 0 && snapshotEq(h.redoStack[len(h.redoStack)-1], cur) {
		h.redoStack = h.redoStack[:len(h.redoStack)-1]
	}
	if len(h.redoStack) == 0 {
		return false
	}
	s := h.redoStack[len(h.redoStack)-1]
	h.redoStack = h.redoStack[:len(h.redoStack)-1]
	d.LoadState(s)
	h.undoStack = append(h.undoStack, cur)
	if len(h.undoStack) > maxUndoDepth {
		h.undoStack = h.undoStack[len(h.undoStack)-maxUndoDepth:]
	}
	return true
}

// Depth — размеры стеков (для HUD).
func (h *History) Depth() (undo, redo int) { return len(h.undoStack), len(h.redoStack) }

func snapshotEq(a, b Snapshot) bool {
	if len(a.Blocks) != len(b.Blocks) || len(a.Links) != len(b.Links) || a.Seq != b.Seq {
		return false
	}
	for i := range a.Blocks {
		if !blockEq(a.Blocks[i], b.Blocks[i]) {
			return false
		}
	}
	for i := range a.Links {
		if a.Links[i] != b.Links[i] {
			return false
		}
	}
	return true
}

func blockEq(a, b *Block) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Kind == b.Kind && a.X == b.X && a.Y == b.Y &&
		a.W == b.W && a.H == b.H && a.Label == b.Label && a.Pins == b.Pins
}
