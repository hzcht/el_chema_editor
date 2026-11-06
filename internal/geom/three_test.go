package geom

import "testing"

// TestRouteAvoidThreeObstacles — продолжение расширенного перебора ПР-1:
// три чужих блока. Наборы неупорядоченные (i<j<k), поэтому перебор полный,
// но без дублей-перестановок. Проверяются те же правила §4.6, что и для
// двух препятствий: нет крюков, нет петель, зазор >= out до каждого чужого
// блока, концы на пинах.
func TestRouteAvoidThreeObstacles(t *testing.T) {
	const out float32 = 5
	S := Rect{Min: Vec{X: 0, Y: 0}, Max: Vec{X: 30, Y: 20}}
	sides := []Side{SideLeft, SideTop, SideRight, SideBottom}

	tPos := [][2]float32{
		{-120, -120}, {-120, 120}, {0, 180}, {180, -120},
		{180, 120}, {60, -60}, {-60, 60},
	}
	var oPos [][2]float32
	for _, x := range []float32{-120, -20, 80, 180} {
		for _, y := range []float32{-120, -20, 80, 180} {
			oPos = append(oPos, [2]float32{x, y})
		}
	}
	obsAt := func(p [2]float32) Rect { return NewRect(p[0], p[1], 30, 20) }

	total := 0
	for _, tp := range tPos {
		T := obsAt(tp)
		if near2(S, T, 15) {
			continue
		}
		for i := 0; i < len(oPos); i++ {
			for j := i + 1; j < len(oPos); j++ {
				for k := j + 1; k < len(oPos); k++ {
					O1, O2, O3 := obsAt(oPos[i]), obsAt(oPos[j]), obsAt(oPos[k])
					if near2(O1, S, 15) || near2(O1, T, 15) ||
						near2(O2, S, 15) || near2(O2, T, 15) ||
						near2(O3, S, 15) || near2(O3, T, 15) ||
						near2(O1, O2, 15) || near2(O1, O3, 15) || near2(O2, O3, 15) {
						continue
					}
					obs := []Rect{O1, O2, O3}
					for _, s1 := range sides {
						for _, s2 := range sides {
							p1, p2 := pinAt(S, s1), pinAt(T, s2)
							route := RouteAvoid(p1, s1, p2, s2, out, obs, []Rect{S, T})
							total++

							if n := Hairpins(route); n != 0 {
								t.Fatalf("крюков %d: s1=%v s2=%v obs=%v T=%v route=%v",
									n, s1, s2, obs, T, route)
							}
							if len(route) > 28 {
								t.Fatalf("петли (%d точек): s1=%v s2=%v obs=%v T=%v route=%v",
									len(route), s1, s2, obs, T, route)
							}
							if !near(route[0], p1) || !near(route[len(route)-1], p2) {
								t.Fatalf("концы потеряны: s1=%v s2=%v route=%v", s1, s2, route)
							}
							for s := 1; s < len(route); s++ {
								for _, o := range obs {
									if d := segRectDist(route[s-1], route[s], o); d < out-1e-3 {
										t.Fatalf("зазор %v < %v: сегмент %v→%v, блок %v, route=%v",
											d, out, route[s-1], route[s], o, route)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if total < 40000 {
		t.Fatalf("перебор слишком мал: %d маршрутов", total)
	}
	t.Logf("проверено маршрутов с тремя препятствиями: %d", total)
}
