package core

import "testing"

func clipDoc() *Document {
	d := &Document{}
	d.AddBlock(&Block{ID: "A", X: 100, Y: 100, W: 120, H: 64, Pins: 2})
	d.AddBlock(&Block{ID: "B", X: 300, Y: 100, W: 120, H: 64, Pins: 4})
	d.AddBlock(&Block{ID: "C", X: 100, Y: 300, W: 120, H: 64, Pins: 1})
	d.AddLink(Endpoint{"A", 1}, Endpoint{"B", 0}) // внутри набора A,B
	d.AddLink(Endpoint{"A", 0}, Endpoint{"C", 0}) // наружу: C не в наборе
	return d
}

// Копия: только внутренние линки; paste со сдвигом, новые ID, линки живут.
func TestCopyPaste(t *testing.T) {
	d := clipDoc()
	clip := CopySelection(d, []string{"A", "B"})
	if len(clip.Blocks) != 2 || len(clip.Links) != 1 {
		t.Fatalf("копия: blocks=%d links=%d (набор должен содержать 1 внутренний линк)",
			len(clip.Blocks), len(clip.Links))
	}
	if clip.Links[0].From.BlockID != "A" || clip.Links[0].To.BlockID != "B" {
		t.Fatalf("внутренний линк потерялся: %+v", clip.Links[0])
	}

	newIDs := d.Paste(clip, 320, 128)
	if len(newIDs) != 2 {
		t.Fatalf("paste: %d новых ID", len(newIDs))
	}
	// явные ID (A/B/C) не двигали seq → автогенерация с b1; коллизий нет
	if newIDs[0] != "b1" || newIDs[1] != "b2" {
		t.Fatalf("новые ID: %v", newIDs)
	}
	p := d.BlockByID(newIDs[0])
	if p.X != 420 || p.Y != 228 {
		t.Fatalf("смещение paste: (%v,%v)", p.X, p.Y)
	}
	// восстановленный внутренний линк
	link := d.Links[len(d.Links)-1]
	if link.From.BlockID != newIDs[0] || link.To.BlockID != newIDs[1] {
		t.Fatalf("линки после paste не перенеслись: %+v", link)
	}
	// оригиналы не съехали
	if d.BlockByID("A").X != 100 || d.BlockByID("B").X != 300 {
		t.Fatal("копирование съело оригиналы")
	}
}

// Копия одиночки без линков; paste получает свежий ID.
func TestCopyPasteSingle(t *testing.T) {
	d := clipDoc()
	clip := CopySelection(d, []string{"C"})
	if len(clip.Blocks) != 1 || len(clip.Links) != 0 {
		t.Fatalf("копия C: %+v", clip.Blocks)
	}
	newIDs := d.Paste(clip, 0, -128)
	if len(newIDs) != 1 || d.BlockByID(newIDs[0]) == nil {
		t.Fatalf("paste C: %v", newIDs)
	}
}

// Отсуственные ID в запросе копии игнорируются.
func TestCopyUnknownIDs(t *testing.T) {
	d := clipDoc()
	clip := CopySelection(d, []string{"ghost", "A"})
	if len(clip.Blocks) != 1 {
		t.Fatalf("ghost должен быть отброшен: %+v", clip.Blocks)
	}
}

// BoundsCenterOf: центр bbox набора.
func TestBoundsCenterOf(t *testing.T) {
	d := clipDoc()
	c, ok := BoundsCenterOf(d, []string{"A", "B"})
	if !ok {
		t.Fatal("центр не посчитан")
	}
	// bbox: x 100..420, y 100..164 → центр (260,132)
	if c.X != 260 || c.Y != 132 {
		t.Fatalf("центр %v, ожидали (260,132)", c)
	}
}
