package core

import (
	"fmt"
	"testing"
)

// HasOverlaps — хоть ли пара блоков пересекается.
func TestHasOverlaps(t *testing.T) {
	d := newDoc()
	if HasOverlaps(d) {
		t.Fatal("пустой документ должен быть без пересечений")
	}
	a := d.AddBlock(&Block{X: 0, Y: 0, W: 120, H: 64, ID: "a"})
	b := d.AddBlock(&Block{X: 100, Y: 0, W: 120, H: 64, ID: "b"})
	if !HasOverlaps(d) {
		t.Fatal("перекрывающийся документ не должен проходит HasOverlaps")
	}
	if b.X != 100 { // 100 + 120 > 0 + 120 → наложение
		t.Fatalf("блоки не должны накладываться: b.X=%v a.X=%v", b.X, a.X)
	}

	d2 := newDoc()
	d2.AddBlock(&Block{X: 0, Y: 0, W: 120, H: 64, ID: "a"})
	d2.AddBlock(&Block{X: 200, Y: 0, W: 120, H: 64, ID: "b"})
	if HasOverlaps(d2) {
		t.Fatal("документ без пересечений должен проходить HasOverlaps == false")
	}
}

// Инвариант: даже плотная сцена (20 блоков на тесной площадке) должна
// разрешиться — редактор не допускает наложения блоков.
func TestPushOverlapsDenseAlwaysResolves(t *testing.T) {
	d := newDoc()
	for i := 0; i < 20; i++ {
		d.AddBlock(&Block{
			X: float32(i%4) * 20, Y: float32(i/4) * 15,
			W: 30, H: 20, ID: fmt.Sprintf("b%d", i),
		})
	}
	if !PushOverlaps(d, nil, 10, 64) {
		t.Fatalf("PushOverlaps не разрешил плотную сцену, overlaps=%v", HasOverlaps(d))
	}
	if HasOverlaps(d) {
		t.Fatal("после PushOverlaps всё ещё есть перекрытия")
	}
	for i := 0; i < len(d.Blocks); i++ {
		for j := i + 1; j < len(d.Blocks); j++ {
			if d.Blocks[i].Rect().Overlaps(d.Blocks[j].Rect()) {
				t.Fatalf("перекрытие %v и %v", d.Blocks[i].Rect(), d.Blocks[j].Rect())
			}
		}
	}
}

// Цепочка: при слишком малом количестве проходов (passes=1) остаток
// переходит в принудительное свободное место — гарантия инварианта.
func TestPushOverlapsFallbackResolves(t *testing.T) {
	d := newDoc()
	for i := 0; i < 6; i++ {
		d.AddBlock(&Block{X: float32(i * 10), Y: float32(i * 10), W: 30, H: 20, ID: fmt.Sprintf("b%d", i)})
	}
	if !PushOverlaps(d, nil, 10, 1) {
		t.Fatalf("fallback в freeSpot не разрешил цепочку: overlaps=%v", HasOverlaps(d))
	}
	if HasOverlaps(d) {
		t.Fatal("fallback не снял перекрытия цепочки")
	}
}
