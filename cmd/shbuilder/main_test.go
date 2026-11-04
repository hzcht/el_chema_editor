package main

import (
	"image/color"
	"math"
	"testing"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
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
