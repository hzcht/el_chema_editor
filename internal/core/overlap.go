package core

// Разрешение перекрытий: при сбросе блока «расталкиваем» пересекающиеся
// блоки так, чтобы они встали рядом (без пересечений), с привязкой к сетке.
// Правило §1: «Перекрытия объектов ведут к перерасположению их рядом».
import "shbuilder/internal/geom"

// PushOverlaps итеративно устраняет пересечения блоков документа:
// двигаются только НЕ защищённые (protected) блоки.
// Возвращает true, если пересечений не осталось.
func PushOverlaps(d *Document, protected map[string]bool, grid float32, passes int) bool {
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
	// passes исчерпаны — финальная проверка
	for i := 0; i < len(d.Blocks); i++ {
		for j := i + 1; j < len(d.Blocks); j++ {
			if d.Blocks[i].Rect().Overlaps(d.Blocks[j].Rect()) {
				return false
			}
		}
	}
	return true
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

// push сдвигает mover минимальным смещением на сторону (по оси наименьшего
// перекрытия), вставляя блок РЯДОМ с тем краем other, с которого он пришёл;
// позиция snap'ится на сетку «наружу».
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
