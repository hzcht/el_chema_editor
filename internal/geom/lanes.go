package geom

// OffsetLanes разводит коллинеарные наложения маршрутов: поздний путь (порядок
// отрисовки) уводит внутренние сегменты на gap в сторону. Концы и подходы к
// пинам не двигаются; если сдвиг режет блок или ломает соседа — остаётся
// наложение. obstacles[i] — то, что путь i не должен резать после сдвига.
// Разбор — docs/ALGORITHMS.md §2.1.
func OffsetLanes(paths [][]Vec, obstacles [][]Rect, gap float32) [][]Vec {
	out := make([][]Vec, len(paths))
	for i := range paths {
		out[i] = append([]Vec(nil), paths[i]...)
	}
	if gap <= 0 {
		return out
	}
	for i := 1; i < len(out); i++ {
		for pass := 0; pass < 6; pass++ {
			changed := false
			// только внутренние сегменты: 1 .. len-3
			for j := 1; j+1 < len(out[i])-1; j++ {
				if !collinearWithAny(out[i][j], out[i][j+1], out[:i], gap) {
					continue
				}
				next, ok := shiftLane(out[i], j, gap, obstaclesAt(obstacles, i), out[:i])
				if !ok {
					continue
				}
				out[i] = next
				changed = true
				break // путь изменился — пересканировать
			}
			if !changed {
				break
			}
		}
	}
	return out
}

func obstaclesAt(obstacles [][]Rect, i int) []Rect {
	if i < len(obstacles) {
		return obstacles[i]
	}
	return nil
}

// shiftLane пробует увести внутренний сегмент path[j]→path[j+1] на ±gap.
// false — оба варианта ломают соседей (сегмент стал бы нулевым или развернулся)
// либо задевают блок. Выбор — минимальное наложение на более ранние пути.
// См. docs/ALGORITHMS.md §2.2.
func shiftLane(path []Vec, j int, gap float32, obs []Rect, earlier [][]Vec) ([]Vec, bool) {
	if j < 1 || j+1 > len(path)-2 {
		return nil, false
	}
	a, b := path[j], path[j+1]
	prev, next := path[j-1], path[j+2]
	horizontal := nearf(a.Y, b.Y)
	if !horizontal && !nearf(a.X, b.X) {
		return nil, false
	}

	var best []Vec
	var bestCost float32
	found := false
	for _, d := range [2]float32{gap, -gap} {
		cand := append([]Vec(nil), path...)
		if horizontal {
			if !nearf(prev.X, a.X) || !nearf(next.X, b.X) {
				return nil, false
			}
			// соседние вертикали не должны схлопнуться или развернуться
			if absf(a.Y-prev.Y) <= absf(d)+eps || absf(b.Y-next.Y) <= absf(d)+eps {
				continue
			}
			cand[j] = Vec{X: a.X, Y: a.Y + d}
			cand[j+1] = Vec{X: b.X, Y: b.Y + d}
		} else {
			if !nearf(prev.Y, a.Y) || !nearf(next.Y, b.Y) {
				return nil, false
			}
			if absf(a.X-prev.X) <= absf(d)+eps || absf(b.X-next.X) <= absf(d)+eps {
				continue
			}
			cand[j] = Vec{X: a.X + d, Y: a.Y}
			cand[j+1] = Vec{X: b.X + d, Y: b.Y}
		}
		if segHitsAny(cand[j], cand[j+1], obs) ||
			segHitsAny(prev, cand[j], obs) ||
			segHitsAny(cand[j+1], next, obs) {
			continue
		}
		cost := overlapLen(cand[j], cand[j+1], earlier)
		if !found || cost < bestCost-eps {
			best, bestCost, found = cand, cost, true
		}
	}
	return best, found
}

// collinearWithAny — есть ли у сегмента a-b коллинеарное перекрытие длиннее
// minLen с каким-нибудь сегментом более ранних путей.
func collinearWithAny(a, b Vec, earlier [][]Vec, minLen float32) bool {
	for _, p := range earlier {
		for k := 1; k < len(p); k++ {
			if collinearOverlapLen(a, b, p[k-1], p[k]) > minLen {
				return true
			}
		}
	}
	return false
}

// overlapLen — суммарная длина коллинеарных перекрытий сегмента a-b с
// сегментами более ранних путей.
func overlapLen(a, b Vec, earlier [][]Vec) float32 {
	var sum float32
	for _, p := range earlier {
		for k := 1; k < len(p); k++ {
			sum += collinearOverlapLen(a, b, p[k-1], p[k])
		}
	}
	return sum
}

// collinearOverlapLen — длина общего участка двух коллинеарных ось-выровненных
// сегментов (0 — если они не коллинеарны или не перекрываются).
func collinearOverlapLen(a, b, c, d Vec) float32 {
	switch {
	case nearf(a.Y, b.Y) && nearf(c.Y, d.Y) && nearf(a.Y, c.Y):
		return maxf(0, minf(maxf(a.X, b.X), maxf(c.X, d.X))-
			maxf(minf(a.X, b.X), minf(c.X, d.X)))
	case nearf(a.X, b.X) && nearf(c.X, d.X) && nearf(a.X, c.X):
		return maxf(0, minf(maxf(a.Y, b.Y), maxf(c.Y, d.Y))-
			maxf(minf(a.Y, b.Y), minf(c.Y, d.Y)))
	}
	return 0
}

func segHitsAny(a, b Vec, rects []Rect) bool {
	for _, r := range rects {
		if segHitsRect(a, b, r) {
			return true
		}
	}
	return false
}

// segHitsRect — заходит ли осевой сегмент a-b внутрь прямоугольника r.
// Кромка не считается пересечением (как и в SegCrossRect).
func segHitsRect(a, b Vec, r Rect) bool {
	if nearf(a.Y, b.Y) {
		y := a.Y
		if y <= r.Min.Y+eps || y >= r.Max.Y-eps {
			return false
		}
		return maxf(a.X, b.X) > r.Min.X+eps && minf(a.X, b.X) < r.Max.X-eps
	}
	if nearf(a.X, b.X) {
		x := a.X
		if x <= r.Min.X+eps || x >= r.Max.X-eps {
			return false
		}
		return maxf(a.Y, b.Y) > r.Min.Y+eps && minf(a.Y, b.Y) < r.Max.Y-eps
	}
	return false
}
