package geom

import "testing"

// TestOffsetLanesSeparatesOverlaps — О1: коллинеарные наложения разводятся
// на gap; концы путей (пины) остаются на месте.
func TestOffsetLanesSeparatesOverlaps(t *testing.T) {
	const gap float32 = 2
	p1 := []Vec{{0, 0}, {10, 0}, {10, 50}, {90, 50}, {90, 0}, {100, 0}}
	p2 := []Vec{{0, 5}, {10, 5}, {10, 50}, {90, 50}, {90, 5}, {100, 5}}

	out := OffsetLanes([][]Vec{p1, p2}, [][]Rect{nil, nil}, gap)

	if !samePath(out[0], p1) {
		t.Fatalf("первый путь не должен двигаться: %v", out[0])
	}
	if !near(out[1][0], p2[0]) || !near(out[1][len(out[1])-1], p2[len(p2)-1]) {
		t.Fatalf("концы второго пути сдвинулись: %v", out[1])
	}
	if samePath(out[1], p2) {
		t.Fatalf("второй путь не разведён: %v", out[1])
	}
	for i := 1; i < len(out[1]); i++ {
		if collinearWithAny(out[1][i-1], out[1][i], [][]Vec{out[0]}, gap) {
			t.Fatalf("осталось наложение %v→%v: %v", out[1][i-1], out[1][i], out[1])
		}
	}
	if n := Hairpins(out[1]); n != 0 {
		t.Fatalf("сдвиг сделал крюк: %v", out[1])
	}
}

// TestOffsetLanesKeepsPinJunction — два линка в общий пин сходятся: подход
// к пину (сегмент, примыкающий к концу) не разводится.
func TestOffsetLanesKeepsPinJunction(t *testing.T) {
	const gap float32 = 2
	p1 := []Vec{{30, 10}, {100, 10}}
	p2 := []Vec{{40, 110}, {35, 110}, {65, 110}, {65, 10}, {95, 10}, {100, 10}}

	out := OffsetLanes([][]Vec{p1, p2}, [][]Rect{nil, nil}, gap)
	last := out[1][len(out[1])-1]
	before := out[1][len(out[1])-2]
	if !near(last, Vec{100, 10}) || !near(before, Vec{95, 10}) {
		t.Fatalf("подход к общему пину сдвинут: %v", out[1])
	}
}

// TestOffsetLanesAvoidsBlock — если сдвиг в одну сторону упирается в блок,
// выбирается другая; в блок путь не заходит.
func TestOffsetLanesAvoidsBlock(t *testing.T) {
	const gap float32 = 2
	p1 := []Vec{{0, 0}, {10, 0}, {10, 50}, {90, 50}, {90, 0}, {100, 0}}
	p2 := []Vec{{0, 5}, {10, 5}, {10, 50}, {90, 50}, {90, 5}, {100, 5}}
	block := Rect{Min: Vec{20, 51}, Max: Vec{80, 70}} // вниз (y+2) нельзя

	out := OffsetLanes([][]Vec{p1, p2}, [][]Rect{nil, {block}}, gap)

	for i := 1; i < len(out[1]); i++ {
		if segHitsRect(out[1][i-1], out[1][i], block) {
			t.Fatalf("сегмент %v→%v режет блок: %v", out[1][i-1], out[1][i], out[1])
		}
	}
	// горизонтальный участок ушёл вверх (48), а не вниз (52)
	found := false
	for i := 1; i < len(out[1]); i++ {
		if nearf(out[1][i-1].Y, 48) && nearf(out[1][i].Y, 48) {
			found = true
		}
	}
	if !found {
		t.Fatalf("сдвиг не ушёл от блока: %v", out[1])
	}
}
