package main

import (
	"image/color"
	"math"
	"testing"

	"gioui.org/f32"
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

// Fit: центр bbox ложится в центр окна, весь bbox внутри с полями.
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
	mid := ed.cam.ScreenToWorld(geom.Vec{X: 800, Y: 450})
	if abs2f(mid.X-cx) > 0.5 || abs2f(mid.Y-cy) > 0.5 {
		t.Fatalf("центр: мир (%v,%v), ожидали (%v,%v)", mid.X, mid.Y, cx, cy)
	}
	for _, w := range []geom.Vec{box.Min, box.Max} {
		s := ed.cam.WorldToScreen(w)
		if s.X < 24 || s.X > 1600-24 || s.Y < 24 || s.Y > 900-24 {
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
