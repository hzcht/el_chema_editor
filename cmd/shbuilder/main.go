// cmd/shbuilder — точка входа приложения (этап 1: редактор).
// Текущее состояние: камера/сетка, блоки с 1–4 пинами (разные capacities),
// линки пин→пин с live-роутингом, junction-кластеры, расталкивание
// пересекающихся блоков при сбросе (PushOverlaps).
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"shbuilder/internal/core"
	"shbuilder/internal/geom"
)

const (
	gridStep = 32.0
	outset   = 16.0 // отступ выхода провода от пина (мировые юниты)
	pinHitR  = 12.0 // радиус попадания в пин
	pinSize  = 4.0  // половина размера пина на канве
)

// --- Режимы ввода ----------------------------------------------------------

type mode int8

const (
	modeFree mode = iota
	modeDragBlock
	modePan
	modeLink
)

type editor struct {
	doc *core.Document
	cam geom.Camera
	th  *material.Theme

	mode mode

	// drag блока
	drag    *core.Block
	dragOff geom.Vec

	// pan
	panStart geom.Vec
	panFrom  geom.Camera

	// тянем линк от пина
	linkFrom  core.Endpoint
	linkTo    core.Endpoint
	linkHover bool

	cursor geom.Vec // текущая точка курсора (экран)
}

func newApp() *editor {
	d := &core.Document{}
	d.AddBlock(&core.Block{ID: "A", Kind: "src", X: 100, Y: 100, W: 120, H: 64, Label: "A", Pins: 2})
	d.AddBlock(&core.Block{ID: "B", Kind: "hub", X: 460, Y: 220, W: 120, H: 64, Label: "B", Pins: 4})
	d.AddBlock(&core.Block{ID: "C", Kind: "load", X: 140, Y: 480, W: 120, H: 64, Label: "C", Pins: 1})
	d.AddBlock(&core.Block{ID: "D", Kind: "term", X: 520, Y: 560, W: 120, H: 64, Label: "D", Pins: 3})
	d.AddLink(core.Endpoint{"A", 0}, core.Endpoint{"B", 0})
	d.AddLink(core.Endpoint{"C", 0}, core.Endpoint{"B", 0}) // junction на B.0
	d.AddLink(core.Endpoint{"A", 1}, core.Endpoint{"D", 0})

	return &editor{
		doc: d,
		cam: geom.Camera{Zoom: 1},
		th:  material.NewTheme(),
	}
}

// --- Ввод ------------------------------------------------------------------

var canvasTag = new(struct{})

const allKinds = pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel |
	pointer.Scroll | pointer.Enter | pointer.Leave

var scrollRange = pointer.ScrollRange{Min: math.MinInt, Max: math.MaxInt}

func handleInput(q input.Source, ed *editor) {
	for {
		pe, ok := q.Event(pointer.Filter{Target: canvasTag, Kinds: allKinds, ScrollY: scrollRange})
		if !ok {
			break
		}
		e, ok := pe.(pointer.Event)
		if !ok {
			continue
		}
		scr := geom.Vec{e.Position.X, e.Position.Y}
		ed.cursor = scr
		world := ed.cam.ScreenToWorld(scr)

		switch e.Kind {
		case pointer.Press:
			if !e.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			// приоритет: пин > блок > пусто (пан)
			if ep, ok := ed.doc.HitPin(world, pinHitR); ok {
				ed.mode = modeLink
				ed.linkFrom = ep
				ed.linkHover = false
			} else if b := ed.doc.HitBlock(world); b != nil {
				ed.mode = modeDragBlock
				ed.drag = b
				ed.dragOff = world.Sub(geom.Vec{b.X, b.Y})
			} else {
				ed.mode = modePan
				ed.panStart = scr
				ed.panFrom = ed.cam
			}
			q.Execute(pointer.GrabCmd{Tag: canvasTag, ID: e.PointerID})

		case pointer.Drag:
			switch ed.mode {
			case modeDragBlock:
				ed.drag.X = world.X - ed.dragOff.X
				ed.drag.Y = world.Y - ed.dragOff.Y
			case modePan:
				ed.cam.Pan = ed.panFrom.Pan.Add(scr.Sub(ed.panStart))
			case modeLink:
				ed.linkHover = false
				if ep, hit := ed.doc.HitPin(world, pinHitR); hit && ep != ed.linkFrom {
					ed.linkTo, ed.linkHover = ep, true
				}
			}

		case pointer.Release, pointer.Cancel:
			switch ed.mode {
			case modeDragBlock:
				if b := ed.drag; b != nil {
					// сброс: snap + разрешение перекрытий («рядом», §1)
					ed.doc.MoveBlock(b.ID, geom.Snap(b.X, gridStep), geom.Snap(b.Y, gridStep))
					PushOverlaps(ed.doc, b.ID)
				}
			case modeLink:
				if ed.linkHover {
					ed.doc.AddLink(ed.linkFrom, ed.linkTo)
				}
			}
			ed.mode = modeFree
			ed.drag = nil
			ed.linkHover = false

		case pointer.Scroll:
			ed.cam.ZoomAt(scr, float32(math.Pow(1.1, float64(-e.Scroll.Y))))
		}
	}
}

// PushOverlaps — тонкая обёртка над core.PushOverlaps (защищён = перетаскиваемый).
func PushOverlaps(d *core.Document, protectedID string) {
	core.PushOverlaps(d, map[string]bool{protectedID: true}, gridStep, 8)
}

// --- Отрисовка -------------------------------------------------------------

var (
	colBG     = color.NRGBA{R: 0xf7, G: 0xf7, B: 0xf8, A: 0xff}
	colGrid   = color.NRGBA{R: 0xd9, G: 0xdc, B: 0xdf, A: 0xff}
	colBlock  = color.NRGBA{R: 0x42, G: 0x74, B: 0xe0, A: 0xff}
	colBord   = color.NRGBA{R: 0x2b, G: 0x4d, B: 0x91, A: 0xff}
	colLink   = color.NRGBA{R: 0x3a, G: 0x5a, B: 0x9a, A: 0xff}
	colJct    = color.NRGBA{R: 0xe0, G: 0x6a, B: 0x2a, A: 0xff} // junction-пин
	colFree   = color.NRGBA{R: 0xaa, G: 0xaa, B: 0xb2, A: 0xff} // свободный пин
	colRubber = color.NRGBA{R: 0x3a, G: 0x5a, B: 0x9a, A: 0x80}
	colText   = color.NRGBA{R: 0x55, G: 0x55, B: 0x60, A: 0xff}
)

func drawFrame(gtx layout.Context, ed *editor) {
	paint.FillShape(gtx.Ops, colBG,
		clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())
	drawGrid(gtx, ed)
	drawLinks(gtx, ed)
	drawBlocks(gtx, ed)
	drawPins(gtx, ed)
	drawRubber(gtx, ed)
	drawHUD(gtx, ed)
}

func drawGrid(gtx layout.Context, ed *editor) {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	minW := ed.cam.ScreenToWorld(geom.Vec{X: 0, Y: 0})
	maxW := ed.cam.ScreenToWorld(geom.Vec{X: w, Y: h})
	minWx, minWy, maxWx, maxWy := minW.X, minW.Y, maxW.X, maxW.Y

	startX := geom.Snap(max(minWx, 0), gridStep)
	for x := startX; x <= min(maxWx, 100000); x += gridStep {
		p1 := ed.cam.WorldToScreen(geom.Vec{x, max(minWy, 0)})
		p2 := ed.cam.WorldToScreen(geom.Vec{x, min(maxWy, 100000)})
		drawSeg(gtx.Ops, p1, p2, 1, colGrid)
	}
	startY := geom.Snap(max(minWy, 0), gridStep)
	for y := startY; y <= min(maxWy, 100000); y += gridStep {
		p1 := ed.cam.WorldToScreen(geom.Vec{max(minWx, 0), y})
		p2 := ed.cam.WorldToScreen(geom.Vec{min(maxWx, 100000), y})
		drawSeg(gtx.Ops, p1, p2, 1, colGrid)
	}
}

func drawLinks(gtx layout.Context, ed *editor) {
	for _, l := range ed.doc.Links {
		p1, s1, ok1 := ed.doc.PinPos(l.From)
		p2, s2, ok2 := ed.doc.PinPos(l.To)
		if !ok1 || !ok2 {
			continue
		}
		route := geom.Route(p1, s1, p2, s2, outset)
		for i := 1; i < len(route); i++ {
			a := ed.cam.WorldToScreen(route[i-1])
			b := ed.cam.WorldToScreen(route[i])
			drawSeg(gtx.Ops, a, b, 2, colLink)
		}
	}
}

func drawRubber(gtx layout.Context, ed *editor) {
	if ed.mode != modeLink {
		return
	}
	p1, _, ok := ed.doc.PinPos(ed.linkFrom)
	if !ok {
		return
	}
	var pts []geom.Vec
	if ed.linkHover {
		p2, s2, _ := ed.doc.PinPos(ed.linkTo)
		pts = geom.Route(p1, sideOf(ed.doc, ed.linkFrom), p2, s2, outset)
	} else {
		pts = geometryL(p1, ed.cam.ScreenToWorld(ed.cursor))
	}
	col := colRubber
	if ed.linkHover {
		col = colLink
	}
	for i := 1; i < len(pts); i++ {
		a := ed.cam.WorldToScreen(pts[i-1])
		b := ed.cam.WorldToScreen(pts[i])
		drawSeg(gtx.Ops, a, b, 2, col)
	}
}

// sideOf — сторона исходного пина (для rubber-маршрута).
func sideOf(d *core.Document, ep core.Endpoint) geom.Side {
	_, s, _ := d.PinPos(ep)
	return s
}

func geometryL(p, c geom.Vec) []geom.Vec {
	pts := []geom.Vec{p, {c.X, p.Y}, c}
	out := pts[:0]
	var prev geom.Vec
	for i, v := range pts {
		if i == 0 || v != prev {
			out = append(out, v)
		}
		prev = v
	}
	return out
}

func drawBlocks(gtx layout.Context, ed *editor) {
	for _, b := range ed.doc.Blocks {
		p := ed.cam.WorldToScreen(geom.Vec{b.X, b.Y})
		s := ed.cam.WorldToScreen(geom.Vec{b.X + b.W, b.Y + b.H})
		r := clip.Rect{Min: image.Pt(int(p.X), int(p.Y)), Max: image.Pt(int(s.X), int(s.Y))}
		paint.FillShape(gtx.Ops, colBlock, r.Op())
		stroke(gtx.Ops, r.Path(), 1.5, colBord)

		t := op.Offset(image.Pt(int(p.X)+10, int(p.Y)+6)).Push(gtx.Ops)
		sz := 12 * ed.cam.Zoom
		if sz < 6 {
			sz = 6
		}
		l := material.Label(ed.th, unit.Sp(sz), b.Label)
		l.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		l.Layout(gtx)
		t.Pop()
	}
}

func drawPins(gtx layout.Context, ed *editor) {
	for _, b := range ed.doc.Blocks {
		for i := 0; i < len(core.PinLayout(b.Pins)); i++ {
			pos, _, ok := b.PinPos(i)
			if !ok {
				continue
			}
			ep := core.Endpoint{BlockID: b.ID, Pin: i}
			gr := ed.doc.PinGroup(ep)
			col := colFree
			size := float32(pinSize)
			switch len(gr) {
			case 1:
			case 2:
				col = colLink
				size = pinSize
			default:
				col = colJct // 3+ линка в одном пине — junction
				size = pinSize * 1.3
			}
			if ed.mode == modeLink && ed.linkHover && ed.linkTo == ep {
				size *= 1.5
			}
			p := ed.cam.WorldToScreen(pos)
			half := size * ed.cam.Zoom
			fillRect(gtx.Ops, image.Rectangle{
				Min: image.Pt(int(p.X-half), int(p.Y-half)),
				Max: image.Pt(int(p.X+half), int(p.Y+half)),
			}, col)
		}
	}
}

func drawHUD(gtx layout.Context, ed *editor) {
	groups := len(ed.doc.LinkGroups())
	free := len(ed.doc.FreePins())
	txt := fmt.Sprintf("blocks %d | links %d | pin groups %d | free pins %d | zoom %d%%",
		len(ed.doc.Blocks), len(ed.doc.Links), groups, free, int(ed.cam.Zoom*100))
	if ed.mode == modeLink {
		txt += "   |   тянем от пина — отпусти на целевом пине"
	}
	l := material.Label(ed.th, unit.Sp(11), txt)
	l.Color = colText
	l.Layout(gtx)
}

// --- Gio текстовые примитивы ------------------------------------------------

func drawSeg(ops *op.Ops, a, b geom.Vec, width float32, c color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(f32.Point{X: a.X, Y: a.Y})
	p.LineTo(f32.Point{X: b.X, Y: b.Y})
	s := clip.Stroke{Path: p.End(), Width: width}.Op().Push(ops)
	paint.Fill(ops, c)
	s.Pop()
}

func stroke(ops *op.Ops, spec clip.PathSpec, width float32, c color.NRGBA) {
	s := clip.Stroke{Path: spec, Width: width}.Op().Push(ops)
	paint.Fill(ops, c)
	s.Pop()
}

func fillRect(ops *op.Ops, r image.Rectangle, c color.NRGBA) {
	paint.FillShape(ops, c, clip.Rect{Min: r.Min, Max: r.Max}.Op())
}

// --- Main -------------------------------------------------------------------

func run(w *app.Window) error {
	ed := newApp()
	for {
		e := w.Event()
		switch e := e.(type) {
		case app.FrameEvent:
			ops := new(op.Ops)
			gtx := app.NewContext(ops, e)
			event.Op(gtx.Ops, canvasTag)
			handleInput(e.Source, ed)
			drawFrame(gtx, ed)
			e.Frame(ops)
		case app.DestroyEvent:
			return nil
		}
	}
}

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title("sh_builder — этап 1, черновик"))
		if err := run(w); err != nil {
			log.Fatal(err)
		}
	}()
	app.Main()
}
