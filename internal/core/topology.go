package core

// Топология линков: union-find по пинам. Группа = кластер пинов, соединённых
// линками (junction). Пин, не входящий ни в один линк — свободный.
import (
	"sort"

	"shbuilder/internal/geom"
)

type dsu map[string]string

func (u dsu) find(x string) string {
	root := x
	for {
		p, ok := u[root]
		if !ok || p == root {
			break
		}
		root = p
	}
	// сжатие пути
	for x != root {
		next, ok := u[x]
		u[x] = root
		if !ok {
			break
		}
		x = next
	}
	return root
}

func (u dsu) union(a, b string) { u[a] = b; _ = u.find(a) }

func key(ep Endpoint) string { return ep.String() }

// LinkGroups возвращает кластеры пинов, соединённых линками.
// Каждый кластер отсортирован по endpoint-ам; кластеры упорядочены детерминированно.
func (d *Document) LinkGroups() [][]Endpoint {
	u := dsu{}
	for _, l := range d.Links {
		u.union(key(l.From), key(l.To))
	}
	groups := map[string][]Endpoint{}
	seen := map[string]bool{}
	for _, l := range d.Links {
		for _, ep := range []Endpoint{l.From, l.To} {
			k := key(ep)
			if seen[k] {
				continue
			}
			seen[k] = true
			root := u.find(k)
			groups[root] = append(groups[root], ep)
		}
	}
	var out [][]Endpoint
	for _, eps := range groups {
		sortEndpoints(eps)
		out = append(out, eps)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i][0].String() < out[j][0].String()
	})
	return out
}

func sortEndpoints(eps []Endpoint) {
	sort.Slice(eps, func(i, j int) bool { return eps[i].String() < eps[j].String() })
}

// FreePins — пины без единого линка.
func (d *Document) FreePins() []Endpoint {
	linked := map[string]bool{}
	for _, l := range d.Links {
		linked[key(l.From)] = true
		linked[key(l.To)] = true
	}
	var out []Endpoint
	for _, b := range d.Blocks {
		for i := 0; i < len(PinLayout(b.Pins)); i++ {
			ep := Endpoint{b.ID, i}
			if !linked[key(ep)] {
				out = append(out, ep)
			}
		}
	}
	sortEndpoints(out)
	return out
}

// PinGroup — размер группы пина (1 = свободный/одиночный).
func (d *Document) PinGroup(ep Endpoint) []Endpoint {
	for _, g := range d.LinkGroups() {
		for _, e := range g {
			if e == ep {
				return g
			}
		}
	}
	return []Endpoint{ep}
}

// OverlapsOf — все блоки, пересекающиеся с данным (для UI-подсказок).
func (d *Document) OverlapsOf(b *Block) []*Block {
	var out []*Block
	for _, other := range d.Blocks {
		if other != b && other.Rect().Overlaps(b.Rect()) {
			out = append(out, other)
		}
	}
	return out
}

// Bounds полезны для авто-рамки (позже) — мин/макс по блокам.
func (d *Document) Bounds() (geom.Rect, bool) {
	if len(d.Blocks) == 0 {
		return geom.Rect{}, false
	}
	r := d.Blocks[0].Rect()
	for _, b := range d.Blocks[1:] {
		rb := b.Rect()
		if rb.Min.X < r.Min.X {
			r.Min.X = rb.Min.X
		}
		if rb.Min.Y < r.Min.Y {
			r.Min.Y = rb.Min.Y
		}
		if rb.Max.X > r.Max.X {
			r.Max.X = rb.Max.X
		}
		if rb.Max.Y > r.Max.Y {
			r.Max.Y = rb.Max.Y
		}
	}
	return r, true
}
