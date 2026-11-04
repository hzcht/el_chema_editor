package core

// Упорядочивание выделения (roadmap §13.3 item 1.4): align по общей рамке,
// distribute с равным шагом центров. Работают только по выбранным ID;
// grid=0 — без привязки к сетке. Детерминизм: перед выбором референса блоки
// сортируются (порядок обхода map в UI не влияет).
import (
	"sort"

	"shbuilder/internal/geom"
)

// arrange вытаскивает блоки по ID.
func arrange(d *Document, ids []string) []*Block {
	out := make([]*Block, 0, len(ids))
	for _, id := range ids {
		if b := d.BlockByID(id); b != nil {
			out = append(out, b)
		}
	}
	return out
}

func snapAxis(v, grid float32) float32 {
	if grid <= 0 {
		return v
	}
	return geom.Snap(v, grid)
}

func sortedCopy(bs []*Block, less func(a, b *Block) bool) []*Block {
	out := append([]*Block(nil), bs...)
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

func byX(a, b *Block) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.ID < b.ID
}

func byY(a, b *Block) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.ID < b.ID
}

func byBottom(a, b *Block) bool {
	ab, bb := a.Y+a.H, b.Y+b.H
	if ab != bb {
		return ab < bb
	}
	return a.ID < b.ID
}

func byCenterX(a, b *Block) bool {
	ac, bc := a.X+a.W/2, b.X+b.W/2
	if ac != bc {
		return ac < bc
	}
	return a.ID < b.ID
}

func byCenterY(a, b *Block) bool {
	ac, bc := a.Y+a.H/2, b.Y+b.H/2
	if ac != bc {
		return ac < bc
	}
	return a.ID < b.ID
}

// AlignLefts выравнивает левые края выделенных по крайнему левому.
func AlignLefts(d *Document, ids []string, grid float32) {
	bs := sortedCopy(arrange(d, ids), byX)
	if len(bs) < 2 {
		return
	}
	target := snapAxis(bs[0].X, grid)
	for _, b := range bs {
		b.X = target
	}
}

// AlignRights выравнивает правые края по крайнему правому.
func AlignRights(d *Document, ids []string, grid float32) {
	bs := sortedCopy(arrange(d, ids), byX)
	if len(bs) < 2 {
		return
	}
	last := bs[len(bs)-1]
	target := snapAxis(last.X+last.W, grid)
	for _, b := range bs {
		b.X = target - b.W
	}
}

// AlignTops — верхние края к крайнему верхнему.
func AlignTops(d *Document, ids []string, grid float32) {
	bs := sortedCopy(arrange(d, ids), byY)
	if len(bs) < 2 {
		return
	}
	target := snapAxis(bs[0].Y, grid)
	for _, b := range bs {
		b.Y = target
	}
}

// AlignBottoms — нижние края к крайнему нижнему.
func AlignBottoms(d *Document, ids []string, grid float32) {
	bs := sortedCopy(arrange(d, ids), byBottom)
	if len(bs) < 2 {
		return
	}
	last := bs[len(bs)-1]
	target := snapAxis(last.Y+last.H, grid)
	for _, b := range bs {
		b.Y = target - b.H
	}
}

// AlignCentersX — центры по X к общему среднему центров.
func AlignCentersX(d *Document, ids []string, grid float32) {
	bs := arrange(d, ids)
	if len(bs) < 2 {
		return
	}
	sum := float32(0)
	for _, b := range bs {
		sum += b.X + b.W/2
	}
	target := snapAxis(sum/float32(len(bs)), grid)
	for _, b := range bs {
		b.X = target - b.W/2
	}
}

// AlignCentersY — центры по Y к общему среднему центров.
func AlignCentersY(d *Document, ids []string, grid float32) {
	bs := arrange(d, ids)
	if len(bs) < 2 {
		return
	}
	sum := float32(0)
	for _, b := range bs {
		sum += b.Y + b.H/2
	}
	target := snapAxis(sum/float32(len(bs)), grid)
	for _, b := range bs {
		b.Y = target - b.H/2
	}
}

// DistributeX — равный шаг центров по X между первым и последним
// (крайние не двигаются). Меньше трёх — no-op.
func DistributeX(d *Document, ids []string, grid float32) {
	bs := sortedCopy(arrange(d, ids), byCenterX)
	if len(bs) < 3 {
		return
	}
	first, last := bs[0], bs[len(bs)-1]
	c0 := first.X + first.W/2
	c1 := last.X + last.W/2
	step := (c1 - c0) / float32(len(bs)-1)
	for i, b := range bs {
		if i == 0 || i == len(bs)-1 {
			continue
		}
		target := c0 + step*float32(i)
		target = snapAxis(target, grid)
		b.X = target - b.W/2
	}
}

// DistributeY — равный шаг центров по Y между первым и последним.
func DistributeY(d *Document, ids []string, grid float32) {
	bs := sortedCopy(arrange(d, ids), byCenterY)
	if len(bs) < 3 {
		return
	}
	first, last := bs[0], bs[len(bs)-1]
	c0 := first.Y + first.H/2
	c1 := last.Y + last.H/2
	step := (c1 - c0) / float32(len(bs)-1)
	for i, b := range bs {
		if i == 0 || i == len(bs)-1 {
			continue
		}
		target := c0 + step*float32(i)
		target = snapAxis(target, grid)
		b.Y = target - b.H/2
	}
}
