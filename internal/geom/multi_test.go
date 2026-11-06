package geom

import "testing"

// TestRouteAvoidTwoObstacles — расширенный перебор из ПР-1: свой блок S,
// ДВА чужих блока (O1, O2) и цель T. Проверяет свод правил §4.6 на
// комбинациях, которых нет в одиночной конфигурации:
//   - П4: в маршруте нет крюков (Hairpins == 0);
//   - П5: нет петель (число точек ограничено);
//   - П6: каждый сегмент держит зазор >= out до ОБОИХ чужих блоков;
//   - концы маршрута совпадают с пинами.
func TestRouteAvoidTwoObstacles(t *testing.T) {
	const out float32 = 5
	S := Rect{Min: Vec{X: 0, Y: 0}, Max: Vec{X: 30, Y: 20}}
	sides := []Side{SideLeft, SideTop, SideRight, SideBottom}

	// решётки положений: цель T и два чужих блока O1/O2
	var tPos [][2]float32
	for _, x := range []float32{-120, -60, 0, 60, 120, 180} {
		for _, y := range []float32{-120, -60, 0, 60, 120, 180} {
			tPos = append(tPos, [2]float32{x, y})
		}
	}
	var oPos [][2]float32
	for _, x := range []float32{-140, -70, 0, 70, 140} {
		for _, y := range []float32{-140, -70, 0, 70, 140} {
			oPos = append(oPos, [2]float32{x, y})
		}
	}

	total := 0
	for _, tp := range tPos {
		T := NewRect(tp[0], tp[1], 30, 20)
		if near2(S, T, 15) {
			continue
		}
		for i, o1 := range oPos {
			for j, o2 := range oPos {
				if i == j {
					continue
				}
				O1 := NewRect(o1[0], o1[1], 30, 20)
				O2 := NewRect(o2[0], o2[1], 30, 20)
				if near2(O1, S, 15) || near2(O1, T, 15) ||
					near2(O2, S, 15) || near2(O2, T, 15) || near2(O1, O2, 15) {
					continue
				}
				obs := []Rect{O1, O2}
				for _, s1 := range sides {
					for _, s2 := range sides {
						p1, p2 := pinAt(S, s1), pinAt(T, s2)
						route := RouteAvoid(p1, s1, p2, s2, out, obs, []Rect{S, T})
						total++

						if n := Hairpins(route); n != 0 {
							t.Fatalf("крюков %d: s1=%v s2=%v O1=%v O2=%v T=%v route=%v",
								n, s1, s2, O1, O2, T, route)
						}
						if len(route) > 24 {
							t.Fatalf("петли (%d точек): s1=%v s2=%v O1=%v O2=%v T=%v route=%v",
								len(route), s1, s2, O1, O2, T, route)
						}
						if !near(route[0], p1) || !near(route[len(route)-1], p2) {
							t.Fatalf("концы потеряны: s1=%v s2=%v route=%v", s1, s2, route)
						}
						for k := 1; k < len(route); k++ {
							for _, o := range obs {
								if d := segRectDist(route[k-1], route[k], o); d < out-1e-3 {
									t.Fatalf("зазор %v < %v: сегмент %v→%v, блок %v, route=%v",
										d, out, route[k-1], route[k], o, route)
								}
							}
						}
					}
				}
			}
		}
	}
	if total < 200000 {
		t.Fatalf("перебор слишком мал: %d маршрутов", total)
	}
	t.Logf("проверено маршрутов с двумя препятствиями: %d", total)
}
