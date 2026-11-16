package main

import (
	"image/color"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"shbuilder/internal/geom"
)

// Тест без окна: путь с «аркой» через равные фокусы (окружность) и обводка.
// Используется в drawLinks для мостиков на пересечении проводников; здесь
// ловим паники/деградации gio на уровне op/clip, не поднимая окна.
func TestArcStrokeHeadless(t *testing.T) {
	ops := new(op.Ops)
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(f32.Point{X: 10, Y: 20}) // старт слева от мостика
	p.LineTo(f32.Point{X: 30, Y: 20}) // прямая до входа в арку
	// равные фокусы (30..50 → центр 40) = окружность r=10; π — пол-оборота,
	// конец напротив начала: (50,20); припёра остаётся непрерывной линией
	c := f32.Point{X: 40, Y: 20}
	p.ArcTo(c, c, float32(math.Pi))
	p.LineTo(f32.Point{X: 70, Y: 20}) // продолжение за аркой
	s := clip.Stroke{Path: p.End(), Width: 2}.Op().Push(ops)
	paint.Fill(ops, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	s.Pop()
}

// Fit: центр bbox ложится в центр видимой канвы (правее палитры и ниже
// тулбара), весь bbox внутри с полями.
func TestFitToWindow(t *testing.T) {
	ed := &editor{
		doc:     demoScene(),
		cam:     geom.Camera{Zoom: pxPerMM},
		grid:    10,
		gridIdx: 1,
	}
	ed.viewW, ed.viewH = 1600, 900
	fitToWindow(ed)

	box := docBBox(ed.doc, 20)
	cx := (box.Min.X + box.Max.X) / 2
	cy := (box.Min.Y + box.Max.Y) / 2
	midX := paletteW + (ed.viewW-paletteW)/2
	midY := toolbarH + (ed.viewH-toolbarH)/2
	mid := ed.cam.ScreenToWorld(geom.Vec{X: midX, Y: midY})
	if abs2f(mid.X-cx) > 0.5 || abs2f(mid.Y-cy) > 0.5 {
		t.Fatalf("центр: мир (%v,%v), ожидали (%v,%v)", mid.X, mid.Y, cx, cy)
	}
	for _, w := range []geom.Vec{box.Min, box.Max} {
		s := ed.cam.WorldToScreen(w)
		if s.X < paletteW+24 || s.X > 1600-24 || s.Y < toolbarH+24 || s.Y > 900-24 {
			t.Fatalf("угол (%v,%v) вылез за поля: экран (%v,%v)", w.X, w.Y, s.X, s.Y)
		}
	}
}

func abs2f(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// segInteriorCross — попадает ли осевой отрезок a-b во внутренность r
// (с отступом pad от кромки). Точки ровно на кромке (штыри пинов, подход
// к пину) не считаются пересечением. Метод слэбов по обеим осям.
func segInteriorCross(a, b geom.Vec, r geom.Rect, pad float32) bool {
	minX, maxX := r.Min.X+pad, r.Max.X-pad
	minY, maxY := r.Min.Y+pad, r.Max.Y-pad
	t0, t1 := float32(0), float32(1)
	slab := func(p, q, lo, hi float32) bool {
		d := q - p
		if d == 0 {
			return p >= lo && p <= hi
		}
		tn, tf := (lo-p)/d, (hi-p)/d
		if tn > tf {
			tn, tf = tf, tn
		}
		if tn > t0 {
			t0 = tn
		}
		if tf < t1 {
			t1 = tf
		}
		return t0 <= t1
	}
	return slab(a.X, b.X, minX, maxX) && slab(a.Y, b.Y, minY, maxY)
}

// TestDemoLinksDontCutBlocks — маршрут каждого линка демо-сцены не режет
// внутренность ни одного блока, ВКЛЮЧАЯ блоки его собственных концов.
// Ловит регресс «A.0→B.0 шёл сквозь блок A по линии пинов, сливаясь
// с правым коннектором A.1» (блоки концов ошибочно исключались из
// препятствий).
func TestDemoLinksDontCutBlocks(t *testing.T) {
	d := demoScene()
	const pad float32 = 1e-3 // кромка/штырь пина — не пересечение
	for _, l := range d.Links {
		obs, self := linkObs(d, l.From, l.To) // production-путь: как в drawLinks
		p1, s1, ok1 := d.PinPos(l.From)
		p2, s2, ok2 := d.PinPos(l.To)
		if !ok1 || !ok2 {
			t.Fatalf("пины не найдены: %+v → %+v", l.From, l.To)
		}
		route := geom.RouteAvoid(p1, s1, p2, s2, outset, obs, self)
		all := append(append(make([]geom.Rect, 0, len(obs)+len(self)), obs...), self...)
		for i := 1; i < len(route); i++ {
			for _, r := range all {
				if segInteriorCross(route[i-1], route[i], r, pad) {
					t.Errorf("линк %v→%v: сегмент %v→%v режет блок (внутренность %v)",
						l.From, l.To, route[i-1], route[i], r)
				}
			}
		}
	}
}

// TestDemoLinksNoHooks — свод правил трассировки на демо-сцене: ни один
// линк не содержит «крюка» (разворота вниз с возвратом вверх тем же x) и не
// крутит петель. Регресс на отзыв «тут сделал 3».
func TestDemoLinksNoHooks(t *testing.T) {
	d := demoScene()
	for _, l := range d.Links {
		obs, self := linkObs(d, l.From, l.To)
		p1, s1, ok1 := d.PinPos(l.From)
		p2, s2, ok2 := d.PinPos(l.To)
		if !ok1 || !ok2 {
			t.Fatalf("пины не найдены: %+v → %+v", l.From, l.To)
		}
		route := geom.RouteAvoid(p1, s1, p2, s2, outset, obs, self)
		if n := geom.Hairpins(route); n != 0 {
			t.Errorf("линк %v→%v: крюков %d, маршрут %v", l.From, l.To, n, route)
		}
		if len(route) > 16 {
			t.Errorf("линк %v→%v: петли (%d точек): %v", l.From, l.To, len(route), route)
		}
	}
}

// TestKeyDeliveryHeadless — без окна повторяет цепочку run(): input-area
// (event.Op), FocusCmd, фильтры handleKeys, кадр, нажатия между кадрами,
// снова handleKeys → processKey. Ловит обе ловушки, из-за которых
// «клавиши не реагировали»:
//  1. keyQueue.Frame() снимает фокус в конце кадра, если для тега нет
//     event.Op + key.FocusFilter (focusable/visible) — события отбрасываются;
//  2. фильтр с Optional:0 не пропускает клавиши с модификатором (Ctrl+…).
func TestKeyDeliveryHeadless(t *testing.T) {
	var r input.Router
	ed := &editor{cam: geom.Camera{Zoom: pxPerMM * 3}, grid: 10, gridIdx: 1}

	// кадр 1 — как в run()
	ops := new(op.Ops)
	event.Op(ops, keyTag)
	r.Source().Execute(key.FocusCmd{Tag: keyTag})
	handleKeys(r.Source(), ed) // регистрация фильтров (событий ещё нет)
	r.Frame(ops)               // здесь старый код терял фокус

	// между кадрами приходят нажатия: простое «0» и Ctrl+G
	r.Queue(key.Event{Name: "0", State: key.Press})
	r.Queue(key.Event{Name: "G", State: key.Press, Modifiers: key.ModCtrl})

	// кадр 2: handleKeys раздаёт события в processKey
	handleKeys(r.Source(), ed)

	if ed.cam.Zoom != pxPerMM {
		t.Fatalf("простая клавиша «0» не дошла до processKey: зум %v, ожидали %v",
			ed.cam.Zoom, pxPerMM)
	}
	if ed.lastKey != "G" {
		t.Fatalf("Ctrl+G не дошёл (фильтр отбросил модификатор): lastKey=%q", ed.lastKey)
	}
}
