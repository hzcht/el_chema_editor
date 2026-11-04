package core

// Буфер обмена для блоков и ВНУТРЕННИХ линков выделения (roadmap §13.3 1.5).
// Линки выделения с невыделенными не копируются (им не с чем стать в paste).

import "shbuilder/internal/geom"

// Clip — копипаст-заначка: значения блоков (без ссылки на документ) и линки,
// чьи оба конца внутри скопированного набора.
type Clip struct {
	Blocks []Block
	Links  []Link
}

// CopySelection — снять копию выделения по ID.
func CopySelection(d *Document, ids []string) *Clip {
	sel := map[string]bool{}
	var blocks []Block
	for _, id := range ids {
		if b := d.BlockByID(id); b != nil {
			blocks = append(blocks, *b) // значение: отвязка от документа
			sel[id] = true
		}
	}
	var links []Link
	for _, l := range d.Links {
		if sel[l.From.BlockID] && sel[l.To.BlockID] {
			links = append(links, l)
		}
	}
	return &Clip{Blocks: blocks, Links: links}
}

// Paste — вставить копию со смещением (dx, dy), ID перегенерируются;
// возвращает новые ID (в том же порядке, что Clip.Blocks).
// Линки восстанавливаются, если оба их конца были в копии.
func (d *Document) Paste(clip *Clip, dx, dy float32) []string {
	oldNew := make(map[string]string, len(clip.Blocks))
	newIDs := make([]string, 0, len(clip.Blocks))
	for _, b := range clip.Blocks {
		nb := b    // значение
		nb.ID = "" // AddBlock сгенерирует свежий
		nb.X, nb.Y = b.X+dx, b.Y+dy
		pasted := d.AddBlock(&nb)
		oldNew[b.ID] = pasted.ID
		newIDs = append(newIDs, pasted.ID)
	}
	for _, l := range clip.Links {
		from, ok1 := oldNew[l.From.BlockID]
		to, ok2 := oldNew[l.To.BlockID]
		if ok1 && ok2 {
			d.AddLink(Endpoint{from, l.From.Pin}, Endpoint{to, l.To.Pin})
		}
	}
	// нумерация линков не мешает undo: снимки берут состояние целиком
	return newIDs
}

// BoundsCenterOf — центр bbox набора блоков (для позиционирования paste).
func BoundsCenterOf(d *Document, ids []string) (geom.Vec, bool) {
	bs := arrange(d, ids)
	if len(bs) == 0 {
		return geom.Vec{}, false
	}
	x0, y0 := bs[0].X, bs[0].Y
	x1, y1 := bs[0].X+bs[0].W, bs[0].Y+bs[0].H
	for _, b := range bs[1:] {
		if b.X < x0 {
			x0 = b.X
		}
		if b.Y < y0 {
			y0 = b.Y
		}
		if b.X+b.W > x1 {
			x1 = b.X + b.W
		}
		if b.Y+b.H > y1 {
			y1 = b.Y + b.H
		}
	}
	return geom.Vec{X: (x0 + x1) / 2, Y: (y0 + y1) / 2}, true
}
