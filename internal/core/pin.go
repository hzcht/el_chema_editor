package core

import "shbuilder/internal/geom"

// Pin — пин в локальной системе сторон блока: на какой стороне и в какой
// относительной позиции (0..1) вдоль стороны он находится.
type Pin struct {
	Side geom.Side
	T    float32 // 0..1 вдоль стороны (0 — «начало» стороны)
}

// PinLayout — каноническая раскладка пинов блока по количеству.
// 1 — правый центр; 2 — лево/право; 3 — лево/право/низ; 4 — лево/право/верх/низ;
// 5+ — равномерно по сторонам в порядке R, T, L, B.
func PinLayout(n int) []Pin {
	switch n {
	case 0:
		return nil
	case 1:
		return []Pin{{geom.SideRight, 0.5}}
	case 2:
		return []Pin{{geom.SideLeft, 0.5}, {geom.SideRight, 0.5}}
	case 3:
		return []Pin{{geom.SideLeft, 0.5}, {geom.SideRight, 0.5}, {geom.SideBottom, 0.5}}
	case 4:
		return []Pin{{geom.SideLeft, 0.5}, {geom.SideRight, 0.5},
			{geom.SideTop, 0.5}, {geom.SideBottom, 0.5}}
	}
	// n >= 5: равномерно по четырём сторонам
	sides := []geom.Side{geom.SideRight, geom.SideTop, geom.SideLeft, geom.SideBottom}
	counts := [4]int{}
	for i := 0; i < n; i++ {
		counts[i%4]++
	}
	tally := [4]int{}
	pins := make([]Pin, 0, n)
	for i := 0; i < n; i++ {
		k := i % 4
		t := (float32(tally[k]) + 0.5) / float32(counts[k])
		tally[k]++
		pins = append(pins, Pin{sides[k], t})
	}
	return pins
}
