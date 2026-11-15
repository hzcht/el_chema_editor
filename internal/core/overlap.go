package core

// Разрешение перекрытий: при сбросе блока «расталкиваем» пересекающиеся
// блоки так, чтобы они встали рядом (без пересечений), с привязкой к сетке.
// Правило §1: «Перекрытия объектов ведут к перерасположению их рядом».
import "shbuilder/internal/geom"

// PushOverlaps итеративно устраняет пересечения блоков документа: двигаются
// только НЕ защищённые (protected) блоки. true — наложений не осталось.
// Инвариант: после любой мутации (drop/вставка/paste/align/load) наложений быть
// не должно; если проходов не хватило, подвижный блок ставится в свободную
// клетку сетки. Разбор — docs/ALGORITHMS.md §4.1.
func PushOverlaps(d *Document, protected map[string]bool, grid float32, passes int) bool {
	if len(d.Blocks) < 2 {
		return true
	}
	if grid <= 0 {
		grid = 1 // шаг поиска всегда > 0, иначе freeSpot зациклится
	}
	for ; passes > 0; passes-- {
		changed := false
		for i := 0; i < len(d.Blocks); i++ {
			for j := i + 1; j < len(d.Blocks); j++ {
				a, b := d.Blocks[i], d.Blocks[j]
				if !a.Rect().Overlaps(b.Rect()) {
					continue
				}
				m, other, ok := moverPair(a, b, protected)
				if !ok {
					continue // оба защищены — не трогаем
				}
				push(m, other, grid)
				changed = true
			}
		}
		if !changed {
			return true
		}
	}
	// Проходов не хватило (плотная сцена/цепочка): страховка — ставим
	// подвижный блок в ближайшую свободную точку сетки. Позиция свободна от
	// ВСЕХ остальных блоков, поэтому новых наложений не создаёт.
	for i := 0; i < len(d.Blocks); i++ {
		for j := i + 1; j < len(d.Blocks); j++ {
			a, b := d.Blocks[i], d.Blocks[j]
			if !a.Rect().Overlaps(b.Rect()) {
				continue
			}
			m, _, ok := moverPair(a, b, protected)
			if !ok {
				continue
			}
			if x, y, ok := freeSpot(d, m, grid); ok {
				m.X, m.Y = x, y
			}
		}
	}
	return !HasOverlaps(d)
}

// HasOverlaps — есть ли хоть одна пара пересекающихся блоков.
func HasOverlaps(d *Document) bool {
	for i := 0; i < len(d.Blocks); i++ {
		for j := i + 1; j < len(d.Blocks); j++ {
			if d.Blocks[i].Rect().Overlaps(d.Blocks[j].Rect()) {
				return true
			}
		}
	}
	return false
}

// freeSpot — ближайшая к текущей позиции m точка сетки, прямоугольник из которой
// не пересекает ни один другой блок. Кольца растут детерминированно.
// См. docs/ALGORITHMS.md §4.3.
func freeSpot(d *Document, m *Block, grid float32) (float32, float32, bool) {
	const maxRing = 512 // 512 клеток с запасом покрывают любую сцену этапа 1
	bx, by := geom.Snap(m.X, grid), geom.Snap(m.Y, grid)
	for k := 0; k <= maxRing; k++ {
		var bestX, bestY, bestD float32
		found := false
		for dx := -k; dx <= k; dx++ {
			for dy := -k; dy <= k; dy++ {
				if max(float32(absInt(dx)), float32(absInt(dy))) != float32(k) {
					continue
				}
				nx, ny := bx+float32(dx)*grid, by+float32(dy)*grid
				if !freeAt(d, m, nx, ny) {
					continue
				}
				d2 := float32(dx*dx + dy*dy)
				if !found || d2 < bestD {
					found, bestX, bestY, bestD = true, nx, ny, d2
				}
			}
		}
		if found {
			return bestX, bestY, true
		}
	}
	return 0, 0, false
}

// freeAt — свободен ли прямоугольник m в точке (x, y) от остальных блоков.
func freeAt(d *Document, m *Block, x, y float32) bool {
	r := geom.NewRect(x, y, m.W, m.H)
	for _, b := range d.Blocks {
		if b == m {
			continue
		}
		if r.Overlaps(b.Rect()) {
			return false
		}
	}
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// moverPair выбирает, кого двигать: защищённый блок не двигается вообще.
// Детерминированно: если оба не защищены — двигается b (правый индекс).
func moverPair(a, b *Block, protected map[string]bool) (mover, other *Block, ok bool) {
	if protected[b.ID] {
		if protected[a.ID] {
			return nil, nil, false
		}
		return a, b, true
	}
	return b, a, true
}

// push сдвигает mover по оси наименьшего перекрытия и ставит его РЯДОМ с other,
// снапя позицию наружу. См. docs/ALGORITHMS.md §4.2.
func push(mover, other *Block, grid float32) {
	mr, or := mover.Rect(), other.Rect()
	wx := min(mr.Max.X, or.Max.X) - max(mr.Min.X, or.Min.X) // перекрытие по x
	wy := min(mr.Max.Y, or.Max.Y) - max(mr.Min.Y, or.Min.Y) // перекрытие по y
	if wx <= 0 || wy <= 0 {
		return // не пересекаются
	}

	type axis struct {
		vertical bool
		sign     float32
	}
	var ax axis

	if wx < wy {
		if (mr.Min.X + mr.Max.X) <= (or.Min.X + or.Max.X) {
			ax = axis{sign: -1} // mover левее → сдвигаем влево
			mover.X = snapOutward(or.Min.X-mover.W, -1, grid)
		} else {
			ax = axis{sign: 1}
			mover.X = snapOutward(or.Max.X, 1, grid)
		}
	} else {
		if (mr.Min.Y + mr.Max.Y) <= (or.Min.Y + or.Max.Y) {
			ax = axis{vertical: true, sign: -1}
			mover.Y = snapOutward(or.Min.Y-mover.H, -1, grid)
		} else {
			ax = axis{vertical: true, sign: 1}
			mover.Y = snapOutward(or.Max.Y, 1, grid)
		}
	}

	// страховка: если snap вернул пересечение — шагаем по оси, пока не выйдем
	for k := 0; k < 4 && mover.Rect().Overlaps(or); k++ {
		if ax.vertical {
			mover.Y += ax.sign * grid
		} else {
			mover.X += ax.sign * grid
		}
	}
}

// snapOutward округляет к сетке в направлении dir (+1/-1), чтобы не вернуться
// внутрь other.
func snapOutward(pos, dir, grid float32) float32 {
	s := geom.Snap(pos, grid)
	if dir > 0 {
		if s < pos {
			s += grid
		}
	} else if s > pos {
		s -= grid
	}
	return s
}

func min(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
