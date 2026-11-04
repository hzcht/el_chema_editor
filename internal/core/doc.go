// Package core — модель документа редактора (этап 1: без физики и симуляции).
// Блоки с пинами разной ёмкости + линки пин→пин. Чистый Go, без GUI-зависимостей.
package core

import (
	"fmt"

	"shbuilder/internal/geom"
)

// Block — объект канвы этапа 1: графика + подпись + пины (без физического типа).
type Block struct {
	ID    string  `json:"id"`
	Kind  string  `json:"kind,omitempty"` // тип для палитры/иконки (этап 2 станет физическим классом)
	X     float32 `json:"x"`              // мировые координаты верхнего левого угла
	Y     float32 `json:"y"`
	W     float32 `json:"w"`
	H     float32 `json:"h"`
	Label string  `json:"label,omitempty"`
	Pins  int     `json:"pins"` // количество коннекторов (раскладка — PinLayout)
}

func (b *Block) Rect() geom.Rect { return geom.NewRect(b.X, b.Y, b.W, b.H) }

// PinPos — мировые координаты пина i и его сторона. Сторона всегда валидна.
func (b *Block) PinPos(i int) (geom.Vec, geom.Side, bool) {
	layout := PinLayout(b.Pins)
	if i < 0 || i >= len(layout) {
		return geom.Vec{}, geom.SideNone, false
	}
	pin := layout[i]
	switch pin.Side {
	case geom.SideLeft:
		return geom.Vec{b.X, b.Y + pin.T*b.H}, pin.Side, true
	case geom.SideRight:
		return geom.Vec{b.X + b.W, b.Y + pin.T*b.H}, pin.Side, true
	case geom.SideTop:
		return geom.Vec{b.X + pin.T*b.W, b.Y}, pin.Side, true
	case geom.SideBottom:
		return geom.Vec{b.X + pin.T*b.W, b.Y + b.H}, pin.Side, true
	}
	return geom.Vec{}, geom.SideNone, false
}

// Endpoint — конец линка: конкретный пин конкретного блока.
type Endpoint struct {
	BlockID string `json:"block"`
	Pin     int    `json:"pin"`
}

func (e Endpoint) String() string { return fmt.Sprintf("%s.%d", e.BlockID, e.Pin) }

// Link — связь пин→пин. Мультилинки на один пин допустимы (junction-кластер).
type Link struct {
	From Endpoint `json:"from"`
	To   Endpoint `json:"to"`
}

// Document — единственный источник истины о схеме (§3).
type Document struct {
	Blocks []*Block
	Links  []Link
	seq    int
}

// AddBlock добавляет блок; пустой ID автогенерируется (b1, b2, ...) —
// пропуская коллизии с уже существующими (документ может содержать «bN»
// с явными ID из файла).
func (d *Document) AddBlock(b *Block) *Block {
	if b.ID == "" {
		for {
			d.seq++
			b.ID = fmt.Sprintf("b%d", d.seq)
			if d.BlockByID(b.ID) == nil {
				break
			}
		}
	}
	if b.W == 0 {
		b.W, b.H = 120, 64
	}
	d.Blocks = append(d.Blocks, b)
	return b
}

func (d *Document) BlockByID(id string) *Block {
	for _, b := range d.Blocks {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// MoveBlock перемещает блок по ID; отсутствующий — no-op.
func (d *Document) MoveBlock(id string, x, y float32) {
	if b := d.BlockByID(id); b != nil {
		b.X, b.Y = x, y
	}
}

// AddLink добавляет связь; false — если endpoint битый или точная пара уже есть.
func (d *Document) AddLink(from, to Endpoint) bool {
	if !d.EndpointValid(from) || !d.EndpointValid(to) {
		return false
	}
	for _, l := range d.Links {
		if sameLink(l, Link{from, to}) {
			return false
		}
	}
	d.Links = append(d.Links, Link{from, to})
	return true
}

func sameLink(a, b Link) bool {
	return (a.From == b.From && a.To == b.To) || (a.From == b.To && a.To == b.From)
}

// EndpointValid — блок существует и пин в диапазоне.
func (d *Document) EndpointValid(ep Endpoint) bool {
	b := d.BlockByID(ep.BlockID)
	if b == nil {
		return false
	}
	return ep.Pin >= 0 && ep.Pin < len(PinLayout(b.Pins))
}

// PinPos — мировые координаты endpoint-а + его сторона.
func (d *Document) PinPos(ep Endpoint) (geom.Vec, geom.Side, bool) {
	b := d.BlockByID(ep.BlockID)
	if b == nil {
		return geom.Vec{}, geom.SideNone, false
	}
	return b.PinPos(ep.Pin)
}

// HitBlock — блок, содержащий точку (в мировых координатах).
func (d *Document) HitBlock(p geom.Vec) *Block {
	for i := len(d.Blocks) - 1; i >= 0; i-- { // верхние слои первыми
		if d.Blocks[i].Rect().Contains(p) {
			return d.Blocks[i]
		}
	}
	return nil
}

// HitPin — ближайший пин в радиусе r (мировые координаты).
func (d *Document) HitPin(p geom.Vec, r float32) (Endpoint, bool) {
	var best Endpoint
	bestDist := r * r
	found := false
	for _, b := range d.Blocks {
		for i := 0; i < len(PinLayout(b.Pins)); i++ {
			pos, _, ok := b.PinPos(i)
			if !ok {
				continue
			}
			dx, dy := pos.X-p.X, pos.Y-p.Y
			dist := dx*dx + dy*dy
			if dist <= bestDist {
				bestDist = dist
				best = Endpoint{b.ID, i}
				found = true
			}
		}
	}
	return best, found
}
