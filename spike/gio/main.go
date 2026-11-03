// Spike: Gio (gioui.org) — «сетка + блоки + коннектор + pan/zoom».
// Маска заданий общая со spike/fyne: одинаковый спектр поведения для сравнения фреймворков.
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
)

// --- Спецификация спайка ---------------------------------------------------

const (
	worldW, worldH = 2000.0, 1500.0 // размеры мира в юнитах (world unit = 1 px при zoom=1)
	gridStep       = 32.0
	blkW, blkH     = 120.0, 64.0
)

type block struct {
	name string
	x, y float32 // левый верхний угол в мировых координатах
}

// Камера: screen = world*zoom + pan
type cam struct {
	zoom       float32
	panX, panY float32
}

func (c *cam) w2s(x, y float32) (float32, float32) { return x*c.zoom + c.panX, y*c.zoom + c.panY }
func (c *cam) s2w(x, y float32) (float32, float32) {
	return (x - c.panX) / c.zoom, (y - c.panY) / c.zoom
}

func (c *cam) zoomAt(sx, sy, factor float32) {
	wx, wy := c.s2w(sx, sy)
	z := clamp(c.zoom*factor, 0.1, 8)
	c.zoom = z
	c.panX = sx - wx*z
	c.panY = sy - wy*z
}

// --- Состояние редактора ---------------------------------------------------

type state struct {
	blocks []block
	cam    cam

	// коннектор: from → to (индексы blocks, точки: правый-центр → левый-центр)
	connFrom, connTo int

	// ввод
	dragging int // индекс блока или -1
	dragOff  f32.Point
	panning  bool
	panFrom  cam // камера на момент начала панорамирования
	panStart f32.Point

	th *material.Theme
}

func newState() *state {
	return &state{
		blocks: []block{
			{"A", 100, 100},
			{"B", 520, 280},
			{"C", 200, 520},
		},
		cam:      cam{zoom: 1},
		dragging: -1,
		th:       material.NewTheme(),
	}
}

func (s *state) hitBlock(wx, wy float32) int {
	for i, b := range s.blocks {
		if wx >= b.x && wx <= b.x+blkW && wy >= b.y && wy <= b.y+blkH {
			return i
		}
	}
	return -1
}

// Точки подключения коннектора.
func (s *state) connPoints() (fx, fy, tx, ty float32) {
	a := s.blocks[s.connFrom]
	b := s.blocks[s.connTo]
	fx, fy = a.x+blkW, a.y+blkH/2 // правый центр A
	tx, ty = b.x, b.y+blkH/2      // левый центр B
	return
}

// Ортогональный маршрут с 1–2 коленами (по x середины).
func connPath(fx, fy, tx, ty float32) [][2]float32 {
	mx := (fx + tx) / 2
	return [][2]float32{{fx, fy}, {mx, fy}, {mx, ty}, {tx, ty}}
}

// --- Ввод ------------------------------------------------------------------

var canvasTag = new(struct{})

const allKinds = pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel |
	pointer.Scroll | pointer.Enter | pointer.Leave

var scrollRange = pointer.ScrollRange{Min: math.MinInt, Max: math.MaxInt}

func handleInput(q input.Source, st *state) {
	for {
		pe, ok := q.Event(pointer.Filter{Target: canvasTag, Kinds: allKinds, ScrollY: scrollRange})
		if !ok {
			break
		}
		e, ok := pe.(pointer.Event)
		if !ok {
			continue
		}
		sx, sy := e.Position.X, e.Position.Y

		switch e.Kind {
		case pointer.Press:
			if e.Buttons.Contain(pointer.ButtonPrimary) {
				mx, my := st.cam.s2w(sx, sy)
				if i := st.hitBlock(mx, my); i >= 0 {
					st.dragging = i
					st.dragOff = f32.Point{X: mx - st.blocks[i].x, Y: my - st.blocks[i].y}
				} else {
					st.panning = true
					st.panStart = f32.Point{X: sx, Y: sy}
					st.panFrom = st.cam
				}
			}
			q.Execute(pointer.GrabCmd{Tag: canvasTag, ID: e.PointerID})

		case pointer.Drag:
			switch {
			case st.dragging >= 0:
				mx, my := st.cam.s2w(sx, sy)
				st.blocks[st.dragging].x = mx - st.dragOff.X
				st.blocks[st.dragging].y = my - st.dragOff.Y
			case st.panning:
				st.cam.panX = st.panFrom.panX + (sx - st.panStart.X)
				st.cam.panY = st.panFrom.panY + (sy - st.panStart.Y)
			}

		case pointer.Release, pointer.Cancel:
			if st.dragging >= 0 {
				st.blocks[st.dragging].x = snap(st.blocks[st.dragging].x, gridStep)
				st.blocks[st.dragging].y = snap(st.blocks[st.dragging].y, gridStep)
			}
			st.dragging, st.panning = -1, false

		case pointer.Scroll:
			f := float32(math.Pow(1.1, float64(-e.Scroll.Y)))
			st.cam.zoomAt(sx, sy, f)
		}
	}
}

func snap(v, step float32) float32 {
	return float32(math.Round(float64(v/step))) * step
}

// --- Отрисовка -------------------------------------------------------------

func drawFrame(gtx layout.Context, st *state) {
	// фон
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0xf7, G: 0xf7, B: 0xf8, A: 0xff},
		clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())

	drawGrid(gtx, st)
	drawConnector(gtx, st)
	for i := range st.blocks {
		drawBlock(gtx, st, i)
	}
	drawHUD(gtx, st)
}

func drawGrid(gtx layout.Context, st *state) {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	minWx, minWy := st.cam.s2w(0, 0)
	maxWx, maxWy := st.cam.s2w(w, h)

	c := color.NRGBA{R: 0xd9, G: 0xdc, B: 0xdf, A: 0xff}
	startX := snap(minWx, gridStep)
	for x := max(startX, 0); x <= min(maxWx, worldW); x += gridStep {
		x1, y1 := st.cam.w2s(x, max(minWy, 0))
		x2, y2 := st.cam.w2s(x, min(maxWy, worldH))
		drawLine(gtx.Ops, x1, y1, x2, y2, c)
	}
	startY := snap(minWy, gridStep)
	for y := max(startY, 0); y <= min(maxWy, worldH); y += gridStep {
		x1, y1 := st.cam.w2s(max(minWx, 0), y)
		x2, y2 := st.cam.w2s(min(maxWx, worldW), y)
		drawLine(gtx.Ops, x1, y1, x2, y2, c)
	}
}

func drawLine(ops *op.Ops, x1, y1, x2, y2 float32, c color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(f32.Point{X: x1, Y: y1})
	p.LineTo(f32.Point{X: x2, Y: y2})
	s := clip.Stroke{Path: p.End(), Width: 1}.Op().Push(ops)
	paint.Fill(ops, c)
	s.Pop()
}

func drawConnector(gtx layout.Context, st *state) {
	fx, fy, tx, ty := st.connPoints()
	pts := connPath(fx, fy, tx, ty)
	ops := gtx.Ops
	for i := 0; i < len(pts)-1; i++ {
		x1, y1 := st.cam.w2s(pts[i][0], pts[i][1])
		x2, y2 := st.cam.w2s(pts[i+1][0], pts[i+1][1])
		drawSegment(ops, x1, y1, x2, y2, color.NRGBA{R: 0x3a, G: 0x5a, B: 0x9a, A: 0xff})
	}
}

func drawSegment(ops *op.Ops, x1, y1, x2, y2 float32, c color.NRGBA) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(f32.Point{X: x1, Y: y1})
	p.LineTo(f32.Point{X: x2, Y: y2})
	s := clip.Stroke{Path: p.End(), Width: 2}.Op().Push(ops)
	paint.Fill(ops, c)
	s.Pop()
}

func drawBlock(gtx layout.Context, st *state, i int) {
	b := st.blocks[i]
	sx, sy := st.cam.w2s(b.x, b.y)
	sw := blkW * st.cam.zoom
	sh := blkH * st.cam.zoom

	// тело
	body := clip.Rect{Min: image.Pt(int(sx), int(sy)), Max: image.Pt(int(sx+sw), int(sy+sh))}.Op()
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x42, G: 0x74, B: 0xe0, A: 0xff}, body)

	// рамка
	stroke(gtx.Ops, clip.Rect{Min: image.Pt(int(sx), int(sy)), Max: image.Pt(int(sx+sw), int(sy+sh))}.Path(),
		1.5, color.NRGBA{R: 0x2b, G: 0x4d, B: 0x91, A: 0xff})

	// подпись
	t := op.Offset(image.Pt(int(sx)+10, int(sy)+6)).Push(gtx.Ops)
	l := material.Label(st.th, unit.Sp(12*st.cam.zoom), b.name)
	l.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	l.Layout(gtx)
	t.Pop()
}

func drawHUD(gtx layout.Context, st *state) {
	z := fmt.Sprintf("zoom %d%%   |   pan %d, %d",
		int(st.cam.zoom*100), int(st.cam.panX), int(st.cam.panY))
	l := material.Label(st.th, unit.Sp(11), z)
	l.Color = color.NRGBA{R: 0x55, G: 0x55, B: 0x60, A: 0xff}
	l.Layout(gtx)
}

func stroke(ops *op.Ops, spec clip.PathSpec, width float32, c color.NRGBA) {
	clip.Stroke{Path: spec, Width: width}.Op().Push(ops)
	paint.Fill(ops, c)
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- Main ------------------------------------------------------------------

func run(w *app.Window) error {
	st := newState()
	for {
		e := w.Event()
		switch e := e.(type) {
		case app.FrameEvent:
			ops := new(op.Ops)
			gtx := app.NewContext(ops, e)
			// зона приёма ввода — всё окно
			event.Op(gtx.Ops, canvasTag)
			handleInput(e.Source, st)
			drawFrame(gtx, st)
			e.Frame(ops)
		case app.DestroyEvent:
			return nil
		}
	}
}

func main() {
	go func() {
		w := new(app.Window)
		if err := run(w); err != nil {
			log.Fatal(err)
		}
	}()
	app.Main()
}
