// Spike: Fyne (fyne.io/fyne/v2) — «сетка + блоки + коннектор + pan/zoom».
// Маска заданий идентична spike/gio — для сравнения фреймворков «на ощупь».
package main

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// --- Спецификация спайка ---------------------------------------------------

const (
	worldW, worldH = 2000.0, 1500.0
	gridStep       = 32.0
	blkW, blkH     = 120.0, 64.0
)

// Камера: screen = world*zoom + pan (мировой юнит == fyne unit при zoom=1)
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

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func snap(v, step float32) float32 {
	return float32(math.Round(float64(v/step))) * step
}

// --- Блок ------------------------------------------------------------------

type Block struct {
	widget.BaseWidget
	name string
	x, y float32 // мировой left-top
	host *Canvas
}

func newBlock(host *Canvas, name string, x, y float32) *Block {
	b := &Block{host: host, name: name, x: x, y: y}
	b.ExtendBaseWidget(b)
	return b
}

type blockRenderer struct {
	w     *Block
	body  *canvas.Rectangle
	label *canvas.Text
}

func (b *Block) CreateRenderer() fyne.WidgetRenderer {
	return &blockRenderer{
		w:     b,
		body:  canvas.NewRectangle(color.NRGBA{R: 0x42, G: 0x74, B: 0xe0, A: 0xff}),
		label: canvas.NewText(b.name, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}),
	}
}

func (r *blockRenderer) Layout(fyne.Size) {
	c := r.w.host.cam
	sx, sy := c.w2s(r.w.x, r.w.y)
	r.body.Move(fyne.NewPos(sx, sy))
	r.body.Resize(fyne.NewSize(blkW*c.zoom, blkH*c.zoom))
	r.label.Move(fyne.NewPos(sx+10, sy+max(c.zoom, 1)*8))
	r.label.TextSize = 12 * c.zoom
	canvas.Refresh(r.label)
}

func (r *blockRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.body, r.label}
}

func (r *blockRenderer) MinSize() fyne.Size { return fyne.NewSize(10, 10) }
func (r *blockRenderer) Destroy()           {}

// Dragged — живая перерисовка блока и коннектора при переносе.
func (b *Block) Dragged(ev *fyne.DragEvent) {
	z := b.host.cam.zoom
	b.x += ev.Dragged.DX / z
	b.y += ev.Dragged.DY / z
	b.host.Refresh()
}

func (b *Block) DragEnd() {
	b.x = snap(b.x, gridStep)
	b.y = snap(b.y, gridStep)
	b.host.Refresh()
}

// --- Коннектор -------------------------------------------------------------

type ConnectorView struct {
	lines []*canvas.Line
	from  *Block
	to    *Block
}

func newConnector(from, to *Block) *ConnectorView {
	c := &ConnectorView{from: from, to: to}
	for i := 0; i < 3; i++ { // маршрут: 3 сегмента с одним коленом
		c.lines = append(c.lines, canvas.NewLine(color.NRGBA{R: 0x3a, G: 0x5a, B: 0x9a, A: 0xff}))
	}
	return c
}

func (cv *ConnectorView) objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(cv.lines))
	for _, l := range cv.lines {
		objs = append(objs, l)
	}
	return objs
}

// segments — экранное согласование: правый центр from → левый центр to с x-коленом.
func (cv *ConnectorView) segments() (x1, y1, x2, y2 [3]float32) {
	c := cv.from.host.cam
	fxp, fyp := c.w2s(cv.from.x+blkW, cv.from.y+blkH/2)
	txp, typ := c.w2s(cv.to.x, cv.to.y+blkH/2)
	mx := (fxp + txp) / 2

	pts := [4][2]float32{{fxp, fyp}, {mx, fyp}, {mx, typ}, {txp, typ}}
	for i := 0; i < 3; i++ {
		x1[i], y1[i] = pts[i][0], pts[i][1]
		x2[i], y2[i] = pts[i+1][0], pts[i+1][1]
	}
	return
}

func (cv *ConnectorView) update() {
	x1, y1, x2, y2 := cv.segments()
	for i, l := range cv.lines {
		l.Position1 = fyne.NewPos(x1[i], y1[i])
		l.Position2 = fyne.NewPos(x2[i], y2[i])
		canvas.Refresh(l)
	}
}

// --- Хост-канвас -----------------------------------------------------------

type Canvas struct {
	widget.BaseWidget
	blocks []*Block
	conn   *ConnectorView
	grid   []*canvas.Line
	cam    cam
}

func newCanvas() *Canvas {
	c := &Canvas{cam: cam{zoom: 1}}
	for _, p := range []struct {
		name string
		x, y float32
	}{{"A", 100, 100}, {"B", 420, 280}, {"C", 200, 520}} {
		c.blocks = append(c.blocks, newBlock(c, p.name, p.x, p.y))
	}
	c.conn = newConnector(c.blocks[0], c.blocks[1])

	c.grid = make([]*canvas.Line, 0)
	for x := gridStep; x <= worldW; x += gridStep {
		l := canvas.NewLine(color.NRGBA{R: 0xd9, G: 0xdc, B: 0xdf, A: 0xff})
		c.grid = append(c.grid, l)
	}
	for y := gridStep; y <= worldH; y += gridStep {
		l := canvas.NewLine(color.NRGBA{R: 0xd9, G: 0xdc, B: 0xdf, A: 0xff})
		c.grid = append(c.grid, l)
	}
	c.ExtendBaseWidget(c)
	return c
}

func (c *Canvas) CreateRenderer() fyne.WidgetRenderer {
	var objs []fyne.CanvasObject
	for _, l := range c.grid {
		objs = append(objs, l)
	}
	objs = append(objs, c.conn.objects()...)
	for _, b := range c.blocks {
		objs = append(objs, b)
	}
	return &canvasRenderer{h: c, objs: objs}
}

type canvasRenderer struct {
	h    *Canvas
	objs []fyne.CanvasObject
}

func (r *canvasRenderer) Layout(fyne.Size) {
	h := r.h

	// сетка: мировые прямые → экран
	for i, l := range h.grid {
		var x1, y1, x2, y2 float32
		if float32(i) < worldW/gridStep { // вертикальные первыми
			wx := gridStep + gridStep*float32(i)
			x1, y1 = h.cam.w2s(wx, 0)
			x2, y2 = h.cam.w2s(wx, worldH)
		} else {
			j := i - int(worldW/gridStep)
			wy := gridStep + gridStep*float32(j)
			x1, y1 = h.cam.w2s(0, wy)
			x2, y2 = h.cam.w2s(worldW, wy)
		}
		l.Position1 = fyne.NewPos(x1, y1)
		l.Position2 = fyne.NewPos(x2, y2)
		canvas.Refresh(l)
	}

	// блоки: мировые координаты → экран
	for _, b := range h.blocks {
		sx, sy := h.cam.w2s(b.x, b.y)
		b.Resize(fyne.NewSize(blkW*h.cam.zoom, blkH*h.cam.zoom))
		b.Move(fyne.NewPos(sx, sy))
	}

	h.conn.update()
}

func (r *canvasRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *canvasRenderer) MinSize() fyne.Size           { return fyne.NewSize(200, 200) }
func (r *canvasRenderer) Destroy()                     {}

// hitBlock — тест блока по точке экрана.
func (c *Canvas) hitBlock(sx, sy float32) *Block {
	wx, wy := c.cam.s2w(sx, sy)
	for _, b := range c.blocks {
		if wx >= b.x && wx <= b.x+blkW && wy >= b.y && wy <= b.y+blkH {
			return b
		}
	}
	return nil
}

// Pan: тянём пустое место (drag стартует не на блоке).
func (c *Canvas) Dragged(ev *fyne.DragEvent) {
	if c.hitBlock(ev.AbsolutePosition.X, ev.AbsolutePosition.Y) != nil {
		return
	}
	c.cam.panX += ev.Dragged.DX
	c.cam.panY += ev.Dragged.DY
	c.Refresh()
}

func (c *Canvas) DragEnd() {}

// Zoom к курсору колесом.
func (c *Canvas) Scrolled(ev *fyne.ScrollEvent) {
	f := float32(math.Pow(1.1, float64(-ev.Scrolled.DY)))
	sx, sy := ev.AbsolutePosition.X, ev.AbsolutePosition.Y
	c.cam.zoomAt(sx, sy, f)
	c.Refresh()
}

// --- Main ------------------------------------------------------------------

func main() {
	a := app.New()
	w := a.NewWindow("Spike Fyne")
	w.SetContent(newCanvas())
	w.Resize(fyne.NewSize(1100, 760))
	w.ShowAndRun()
}
