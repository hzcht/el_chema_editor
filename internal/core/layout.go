package core

// Автолайаут (roadmap §13.3, пункт 1.7): слоистая раскладка слева направо.
// Направление ребра выводится из сторон пинов (right → left), иначе из текущей
// геометрии; циклы снимаются обходом Канa с принудительным выбором вершины;
// порядок внутри слоя — barycenter (несколько проходов вниз/вверх).
// Разбор — docs/ALGORITHMS.md §6.1.
import (
	"math"
	"sort"

	"shbuilder/internal/geom"
)

// Зазоры раскладки (мм): округляются вверх до шага сетки.
const (
	layoutHGap    = 20 // между слоями по X
	layoutVGap    = 10 // между блоками одного слоя по Y
	layoutCompGap = 20 // между несвязными компонентами по Y
	layoutSweeps  = 4  // проходов barycenter (вниз/вверх)
)

// ledge — ребро графа раскладки: индексы вершин в списке блоков.
type ledge struct{ u, v int }

// layoutRun — состояние одной раскладки: блоки, рёбра и зазоры.
type layoutRun struct {
	nodes   []*Block
	edges   []ledge
	grid    float32
	hgap    float32
	vgap    float32
	compGap float32
}

// Layout раскладывает блоки слоями слева направо. ids == nil — весь документ,
// иначе только перечисленные блоки (порядок берётся из документа). Возвращает
// число разложенных блоков и число слоёв; (0, 0) — раскладывать нечего.
// Пересечений блоков не создаёт; вызывающий всё равно сверяет инвариант.
func Layout(d *Document, ids []string, grid float32) (blocks, layers int) {
	nodes := layoutNodes(d, ids)
	if len(nodes) < 2 {
		return 0, 0
	}
	run := layoutRun{
		nodes:   nodes,
		edges:   layoutEdges(d, nodes),
		grid:    grid,
		hgap:    snapUp(layoutHGap, grid),
		vgap:    snapUp(layoutVGap, grid),
		compGap: snapUp(layoutCompGap, grid),
	}
	comps := run.components()
	// Порядок компонент — по текущему положению: сверху вниз, затем слева.
	sort.Slice(comps, func(i, j int) bool { return run.compLess(comps[i], comps[j]) })

	originX, originY := layoutOrigin(nodes)
	top := originY
	for _, comp := range comps {
		ls := run.layers(comp)
		top = snapUp(top+run.place(ls, originX, top)+run.compGap, run.grid)
		if n := len(ls); n > layers {
			layers = n
		}
	}
	return len(nodes), layers
}

// layoutNodes — блоки для раскладки в порядке документа.
func layoutNodes(d *Document, ids []string) []*Block {
	if ids == nil {
		out := make([]*Block, len(d.Blocks))
		copy(out, d.Blocks)
		return out
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []*Block
	for _, b := range d.Blocks {
		if want[b.ID] {
			out = append(out, b)
		}
	}
	return out
}

// layoutOrigin — верхний левый угол рамки блоков: раскладка не сдвигает схему
// целиком, а перестраивает её на месте.
func layoutOrigin(nodes []*Block) (x, y float32) {
	x, y = nodes[0].X, nodes[0].Y
	for _, b := range nodes[1:] {
		if b.X < x {
			x = b.X
		}
		if b.Y < y {
			y = b.Y
		}
	}
	return x, y
}

// layoutEdges строит рёбра: линки между разными блоками, без дублей. Петля
// (линк в тот же блок) и линк за пределами раскладки игнорируются.
func layoutEdges(d *Document, nodes []*Block) []ledge {
	idx := make(map[string]int, len(nodes))
	for i, b := range nodes {
		idx[b.ID] = i
	}
	seen := map[[2]int]bool{}
	var out []ledge
	for _, l := range d.Links {
		iu, oku := idx[l.From.BlockID]
		iv, okv := idx[l.To.BlockID]
		if !oku || !okv || iu == iv {
			continue
		}
		_, su, _ := nodes[iu].PinPos(l.From.Pin)
		_, sv, _ := nodes[iv].PinPos(l.To.Pin)
		u, v := iu, iv
		if layoutDir(su, sv, nodes[iu], nodes[iv]) < 0 {
			u, v = iv, iu
		}
		k := [2]int{u, v}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, ledge{u, v})
	}
	return out
}

// layoutDir — направление ребра «from → to»: > 0 — from левее, < 0 — правее.
// Правый пин в левый пин — явный сигнал направления; иначе решает геометрия.
func layoutDir(sf, st geom.Side, fb, tb *Block) int {
	switch {
	case sf == geom.SideRight && st == geom.SideLeft:
		return 1
	case sf == geom.SideLeft && st == geom.SideRight:
		return -1
	case sf == geom.SideRight && st != geom.SideRight:
		return 1
	case sf == geom.SideLeft && st != geom.SideLeft:
		return -1
	}
	if fb.X != tb.X {
		if fb.X < tb.X {
			return 1
		}
		return -1
	}
	if fb.Y != tb.Y {
		if fb.Y < tb.Y {
			return 1
		}
		return -1
	}
	if fb.ID < tb.ID {
		return 1
	}
	return -1
}

// snapUp — округление вверх до шага сетки; grid <= 0 — без привязки.
func snapUp(v, grid float32) float32 {
	if grid <= 0 {
		return v
	}
	q := float64(v) / float64(grid)
	return float32(math.Ceil(q-1e-9)) * grid
}

// components — группы блоков, связанных линками (несвязные схемы
// раскладываются отдельными столбиками). Индексы вершин — в порядке документа.
func (r layoutRun) components() [][]int {
	par := make([]int, len(r.nodes))
	for i := range par {
		par[i] = i
	}
	find := func(x int) int {
		for par[x] != x {
			par[x] = par[par[x]]
			x = par[x]
		}
		return x
	}
	for _, e := range r.edges {
		ru, rv := find(e.u), find(e.v)
		if ru != rv {
			par[ru] = rv
		}
	}
	groups := map[int][]int{}
	for i := range r.nodes {
		root := find(i)
		groups[root] = append(groups[root], i)
	}
	out := make([][]int, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	return out
}

// compLess — порядок компонент: по верхнему левому блоку (Y, X, ID).
func (r layoutRun) compLess(a, b []int) bool {
	ba, bb := r.anchor(a), r.anchor(b)
	if ba.Y != bb.Y {
		return ba.Y < bb.Y
	}
	if ba.X != bb.X {
		return ba.X < bb.X
	}
	return ba.ID < bb.ID
}

// anchor — самый верхний (затем самый левый, затем меньший ID) блок компоненты.
func (r layoutRun) anchor(comp []int) *Block {
	best := r.nodes[comp[0]]
	for _, i := range comp[1:] {
		b := r.nodes[i]
		switch {
		case b.Y < best.Y:
		case b.Y == best.Y && b.X < best.X:
		case b.Y == best.Y && b.X == best.X && b.ID < best.ID:
		default:
			continue
		}
		best = b
	}
	return best
}

// before — текущий порядок блоков (Y, X, ID): исходный порядок внутри слоя.
func (r layoutRun) before(a, b int) bool {
	ba, bb := r.nodes[a], r.nodes[b]
	if ba.Y != bb.Y {
		return ba.Y < bb.Y
	}
	if ba.X != bb.X {
		return ba.X < bb.X
	}
	return ba.ID < bb.ID
}

// layers — слои компоненты, каждый срез уже упорядочен внутри слоя.
// Слой вершины — длина самого длинного пути от корней (longest path).
func (r layoutRun) layers(comp []int) [][]int {
	inComp := make(map[int]bool, len(comp))
	for _, i := range comp {
		inComp[i] = true
	}
	adj := make(map[int][]int, len(comp))
	nb := make(map[int][]int, len(comp))
	indeg := make(map[int]int, len(comp))
	for _, e := range r.edges {
		if !inComp[e.u] || !inComp[e.v] {
			continue
		}
		adj[e.u] = append(adj[e.u], e.v)
		indeg[e.v]++
		nb[e.u] = append(nb[e.u], e.v)
		nb[e.v] = append(nb[e.v], e.u)
	}

	layerOf := make(map[int]int, len(comp))
	done := make(map[int]bool, len(comp))
	ready := make([]int, 0, len(comp))
	for _, i := range comp {
		if indeg[i] == 0 {
			ready = append(ready, i)
		}
	}
	sortReady := func() {
		sort.Slice(ready, func(a, b int) bool { return r.before(ready[a], ready[b]) })
	}
	sortReady()

	left := len(comp)
	for left > 0 {
		if len(ready) == 0 {
			// остались только циклы: принудительно берём верхний левый блок,
			// рёбра внутрь уже пройденных вершин становятся обратными и на
			// слои не влияют.
			force := -1
			for _, i := range comp {
				if !done[i] && (force < 0 || r.before(i, force)) {
					force = i
				}
			}
			if force < 0 {
				break
			}
			ready = append(ready, force)
		}
		u := ready[0]
		ready = ready[1:]
		if done[u] {
			continue
		}
		done[u] = true
		left--
		for _, v := range adj[u] {
			if !done[v] && layerOf[u]+1 > layerOf[v] {
				layerOf[v] = layerOf[u] + 1
			}
			if indeg[v] > 0 {
				indeg[v]--
			}
			if indeg[v] == 0 && !done[v] {
				ready = append(ready, v)
				sortReady()
			}
		}
	}

	maxLayer := 0
	for _, i := range comp {
		if layerOf[i] > maxLayer {
			maxLayer = layerOf[i]
		}
	}
	byLayer := make([][]int, maxLayer+1)
	for _, i := range comp {
		l := layerOf[i]
		byLayer[l] = append(byLayer[l], i)
	}
	pos := make(map[int]int, len(comp))
	reindex := func(l int) {
		for k, i := range byLayer[l] {
			pos[i] = k
		}
	}
	for l, layer := range byLayer {
		sort.Slice(layer, func(a, b int) bool { return r.before(layer[a], layer[b]) })
		reindex(l)
	}

	// Barycenter: слой упорядочивается по средней позиции соседей в опорном
	// слое. Проходы чередуются: вниз (опора — предыдущий слой), вверх (следующий).
	for s := 0; s < layoutSweeps; s++ {
		down := s%2 == 0
		for k := range byLayer {
			l, ref := k, k-1
			if !down {
				l, ref = len(byLayer)-1-k, len(byLayer)-k
			}
			if ref < 0 || ref >= len(byLayer) || len(byLayer[l]) == 0 {
				continue
			}
			// Веса считаются ДО сортировки: компаратор обязан быть
			// согласованным, а pos меняется только после неё (reindex).
			layer := byLayer[l]
			bc := make(map[int]float32, len(layer))
			for _, i := range layer {
				bc[i] = r.barycenter(i, ref, layerOf, nb, pos)
			}
			sort.SliceStable(layer, func(a, b int) bool {
				ia, ib := layer[a], layer[b]
				if bc[ia] != bc[ib] {
					return bc[ia] < bc[ib]
				}
				if pos[ia] != pos[ib] {
					return pos[ia] < pos[ib]
				}
				return r.nodes[ia].ID < r.nodes[ib].ID // тай-брейк по ID: порядок документа не влияет
			})
			reindex(l)
		}
	}
	return byLayer
}

// barycenter — средняя позиция соседей в опорном слое; соседей нет — своя позиция.
func (r layoutRun) barycenter(i, ref int, layerOf map[int]int, nb map[int][]int, pos map[int]int) float32 {
	sum, n := 0, 0
	for _, j := range nb[i] {
		if layerOf[j] == ref {
			sum += pos[j]
			n++
		}
	}
	if n == 0 {
		return float32(pos[i])
	}
	return float32(sum) / float32(n)
}

// place расставляет блоки компоненты: слои слева направо, внутри слоя — сверху
// вниз. Возвращает высоту компоненты (без замыкающего зазора). Пересечений нет
// по построению: слои не перекрываются по X (ширина слоя — максимум по блокам),
// блоки слоя — по Y.
func (r layoutRun) place(layers [][]int, x0, y0 float32) float32 {
	width := make([]float32, len(layers))
	for l, layer := range layers {
		for _, i := range layer {
			if w := r.nodes[i].W; w > width[l] {
				width[l] = w
			}
		}
	}
	xs := make([]float32, len(layers))
	x := x0
	for l := range layers {
		xs[l] = x
		x = snapUp(x+width[l]+r.hgap, r.grid)
	}
	height := float32(0)
	for l, layer := range layers {
		y := y0
		bottom := y0
		for _, i := range layer {
			b := r.nodes[i]
			b.X, b.Y = xs[l], y
			if e := y + b.H; e > bottom {
				bottom = e
			}
			y = snapUp(y+b.H+r.vgap, r.grid)
		}
		if h := bottom - y0; h > height {
			height = h
		}
	}
	return height
}
