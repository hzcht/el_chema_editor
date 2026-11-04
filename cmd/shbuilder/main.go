// cmd/shbuilder — точка входа приложения (этап 1: редактор).
// Текущее состояние: камера/сетка, блоки с 1–4 пинами (разные capacities),
// линки пин→пин с live-роутингом, junction-кластеры, расталкивание
// пересекающихся блоков при сбросе (PushOverlaps).
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"log"
	"math"
	"os"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
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
	modeBoxSelect // рамка выделения (левый клик по пустому)
)

type editor struct {
	doc *core.Document
	cam geom.Camera
	th  *material.Theme

	history core.History
	file    string // путь текущего файла (аргумент CLI или schematic.json)
	dirty   bool
	msg     string     // статусное сообщение (save/load/undo/info)
	clip    *core.Clip // буфер обмена (Ctrl+C/V)

	mode mode

	// drag блока
	drag       *core.Block
	dragOff    geom.Vec
	dragLast   geom.Vec        // world-точка пред. кадра драга (для delta группы)
	sel        map[string]bool // выделение по ID блоков
	boxA, boxB geom.Vec        // рамка выделения (мировые точки)

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
	path := "schematic.json"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	d := &core.Document{}
	msg := ""
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, d); err != nil {
			msg = "load: " + err.Error()
			d = &core.Document{} // битый файл не используем
		} else {
			msg = fmt.Sprintf("открыт %s: %d блоков", path, len(d.Blocks))
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		msg = "новая схема: " + path + " (демо-сцена)"
		d = demoScene()
	} else {
		msg = "read: " + err.Error()
	}

	ed := &editor{
		doc:  d,
		cam:  geom.Camera{Zoom: 1},
		th:   material.NewTheme(),
		file: path,
		msg:  msg,
	}
	ed.history.Reset(d)
	return ed
}

// demoScene — стартовая сцена: блоки 1–4 пинов, junction, перекрёстные линки.
func demoScene() *core.Document {
	d := &core.Document{}
	d.AddBlock(&core.Block{ID: "A", Kind: "src", X: 100, Y: 100, W: 120, H: 64, Label: "A", Pins: 2})
	d.AddBlock(&core.Block{ID: "B", Kind: "hub", X: 460, Y: 220, W: 120, H: 64, Label: "B", Pins: 4})
	d.AddBlock(&core.Block{ID: "C", Kind: "load", X: 140, Y: 480, W: 120, H: 64, Label: "C", Pins: 1})
	d.AddBlock(&core.Block{ID: "D", Kind: "term", X: 520, Y: 560, W: 120, H: 64, Label: "D", Pins: 3})
	d.AddLink(core.Endpoint{BlockID: "A", Pin: 0}, core.Endpoint{BlockID: "B", Pin: 0})
	d.AddLink(core.Endpoint{BlockID: "C", Pin: 0}, core.Endpoint{BlockID: "B", Pin: 0}) // junction на B.0
	d.AddLink(core.Endpoint{BlockID: "A", Pin: 1}, core.Endpoint{BlockID: "D", Pin: 0})
	return d
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
		scr := geom.Vec{X: e.Position.X, Y: e.Position.Y}
		ed.cursor = scr
		world := ed.cam.ScreenToWorld(scr)

		switch e.Kind {
		case pointer.Press:
			switch {
			case e.Buttons.Contain(pointer.ButtonPrimary):
				ed.history.Capture(ed.doc) // pre-state: undo восстановит позицию до драга/линка
				// приоритет: пин > блок > пустое (рамка выделения)
				if ep, ok := ed.doc.HitPin(world, pinHitR); ok {
					ed.mode = modeLink
					ed.linkFrom = ep
					ed.linkHover = false
				} else if b := ed.doc.HitBlock(world); b != nil {
					// Shift+клик — добавить/убрать из выделения; простой клик
					// по невыполненному — заменить выделение; по выбранному —
					// оставить группу (drag группы)
					if e.Modifiers.Contain(key.ModShift) {
						if ed.sel == nil {
							ed.sel = map[string]bool{}
						}
						if ed.sel[b.ID] {
							delete(ed.sel, b.ID)
						} else {
							ed.sel[b.ID] = true
						}
					} else if !ed.sel[b.ID] {
						ed.sel = map[string]bool{b.ID: true}
					}
					ed.mode = modeDragBlock
					ed.drag = b
					ed.dragOff = world.Sub(geom.Vec{X: b.X, Y: b.Y})
					ed.dragLast = world
				} else {
					ed.mode = modeBoxSelect
					ed.boxA, ed.boxB = world, world
				}
				q.Execute(pointer.GrabCmd{Tag: canvasTag, ID: e.PointerID})

			case e.Buttons.Contain(pointer.ButtonSecondary):
				ed.mode = modePan
				ed.panStart = scr
				ed.panFrom = ed.cam
				q.Execute(pointer.GrabCmd{Tag: canvasTag, ID: e.PointerID})
			}

		case pointer.Drag:
			switch ed.mode {
			case modeDragBlock:
				delta := world.Sub(ed.dragLast)
				ed.dragLast = world
				if ed.sel[ed.drag.ID] && len(ed.sel) > 0 {
					// двигаем всё выделение на delta
					for id := range ed.sel {
						if blk := ed.doc.BlockByID(id); blk != nil {
							blk.X += delta.X
							blk.Y += delta.Y
						}
					}
				} else {
					ed.drag.X = world.X - ed.dragOff.X
					ed.drag.Y = world.Y - ed.dragOff.Y
				}
			case modePan:
				ed.cam.Pan = ed.panFrom.Pan.Add(scr.Sub(ed.panStart))
			case modeLink:
				ed.linkHover = false
				if ep, hit := ed.doc.HitPin(world, pinHitR); hit && ep != ed.linkFrom {
					ed.linkTo, ed.linkHover = ep, true
				}
			case modeBoxSelect:
				ed.boxB = world
			}

		case pointer.Release, pointer.Cancel:
			switch ed.mode {
			case modeDragBlock:
				if b := ed.drag; b != nil {
					// сброс: snap + разрешение перекрытий («рядом», §1)
					if ed.sel[b.ID] {
						snapSelection(ed.doc, ed.sel)
						pushOverlapsSet(ed.doc, ed.sel)
					} else {
						ed.doc.MoveBlock(b.ID, geom.Snap(b.X, gridStep), geom.Snap(b.Y, gridStep))
						PushOverlaps(ed.doc, b.ID)
					}
				}
			case modeLink:
				if ed.linkHover {
					if ed.doc.AddLink(ed.linkFrom, ed.linkTo) {
						ed.msg = fmt.Sprintf("линк %v → %v", ed.linkFrom, ed.linkTo)
					} else {
						ed.msg = fmt.Sprintf("линк %v → %v уже существует", ed.linkFrom, ed.linkTo)
					}
				}
			case modeBoxSelect:
				ed.boxSelectFinish(world, e.Modifiers)
			}
			if ed.history.Finish(ed.doc) {
				ed.dirty = true
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

// --- Выделение --------------------------------------------------------------

func selIDs(sel map[string]bool) []string {
	ids := make([]string, 0, len(sel))
	for id := range sel {
		ids = append(ids, id)
	}
	return ids
}

// snapSelection — snap к сетке всех выделенных блоков.
func snapSelection(d *core.Document, sel map[string]bool) {
	for id := range sel {
		if b := d.BlockByID(id); b != nil {
			b.X = geom.Snap(b.X, gridStep)
			b.Y = geom.Snap(b.Y, gridStep)
		}
	}
}

// pushOverlapsSet — расталкивание с защитой всего выделения целиком.
func pushOverlapsSet(d *core.Document, sel map[string]bool) {
	prot := make(map[string]bool, len(sel))
	for id := range sel {
		prot[id] = true
	}
	core.PushOverlaps(d, prot, gridStep, 12)
}

// boxSelectFinish завершает рамку: заменяет/дополняет выделение пересечёнными
// блоками; вырожденная рамка (клик по пустому) снимает выделение.
func (ed *editor) boxSelectFinish(world geom.Vec, mods key.Modifiers) {
	x0, x1 := min(ed.boxA.X, world.X), max(ed.boxA.X, world.X)
	y0, y1 := min(ed.boxA.Y, world.Y), max(ed.boxA.Y, world.Y)
	degenerate := (x1-x0) < 2 && (y1-y0) < 2
	if degenerate {
		ed.sel = nil
		return
	}
	box := geom.NewRect(x0, y0, x1-x0, y1-y0)
	if mods.Contain(key.ModShift) && ed.sel == nil {
		ed.sel = map[string]bool{}
	}
	if !mods.Contain(key.ModShift) {
		ed.sel = map[string]bool{}
	}
	for _, b := range ed.doc.Blocks {
		if box.Overlaps(b.Rect()) {
			ed.sel[b.ID] = true
		}
	}
	if len(ed.sel) > 0 {
		ed.msg = fmt.Sprintf("выбрано %d блоков", len(ed.sel))
	}
}

// clearSelection — общая точка сброса (undo/redo/load сбивают ссылки).
func (ed *editor) clearSelection() { ed.sel = nil }

// --- Клавиатура -------------------------------------------------------------

var keyTag = new(struct{})

// handleKeys потребляет key-события текущего кадра.
func handleKeys(q input.Source, ed *editor) {
	for {
		ke, ok := q.Event(key.Filter{Focus: keyTag}) // пустое Name — все клавиши
		if !ok {
			break
		}
		k, ok := ke.(key.Event)
		if !ok || k.State != key.Press {
			continue // реагируем только на нажатие
		}
		processKey(k, ed)
	}
}

// processKey — хоткеи. Ctrl — key.ModShortcut (на Windows Ctrl, на mac Cmd).
func processKey(k key.Event, ed *editor) {
	mod := k.Modifiers.Contain(key.ModShortcut)
	shift := k.Modifiers.Contain(key.ModShift)
	switch {
	case mod && k.Name == "Z":
		if ed.mode != modeFree {
			return // во время драга не дёргаем (указатели ed.drag протухли бы)
		}
		if shift {
			redoMark(ed)
		} else {
			undoMark(ed)
		}
	case mod && k.Name == "Y":
		if ed.mode != modeFree {
			return
		}
		redoMark(ed)
	case mod && k.Name == "S":
		saveDoc(ed)
	case mod && k.Name == "O":
		loadDoc(ed)
	case mod && k.Name == "C":
		if ed.mode == modeFree {
			ed.clip = core.CopySelection(ed.doc, selIDs(ed.sel))
			if len(ed.clip.Blocks) == 0 {
				ed.msg = "буфер: нет выделения"
			} else {
				ed.msg = fmt.Sprintf("скопировано %d блоков", len(ed.clip.Blocks))
			}
		}
	case mod && k.Name == "V":
		if ed.mode == modeFree {
			pasteClip(ed)
		}
	case mod && k.Name == "A":
		if ed.mode == modeFree {
			ed.sel = map[string]bool{}
			for _, b := range ed.doc.Blocks {
				ed.sel[b.ID] = true
			}
			ed.msg = fmt.Sprintf("выбраны все: %d блоков", len(ed.sel))
		}
	case !mod && k.Name == key.NameEscape:
		ed.clearSelection()
		ed.msg = "выделение снято"
	case !mod && ed.mode == modeFree && len(k.Name) == 1 && k.Name[0] >= '1' && k.Name[0] <= '6':
		addPinBlock(ed, int(k.Name[0]-'0'))
	case !mod && ed.mode == modeFree &&
		(k.Name == key.NameDeleteForward || k.Name == key.NameDeleteBackward):
		deleteAtCursor(ed)
	case !mod && ed.mode == modeFree && len(k.Name) == 1:
		arrangeKey(string(k.Name), shift, ed)
	}
}

// arrangeKey — упорядочивание выделенных (roadmap 1.4):
// A/R — левые/правые края, T/B — верх/низ, H/V — центры, D — распределить
// (Shift+D — по вертикали). Через Capture/Finish — полноценный undo-шаг.
func arrangeKey(name string, shift bool, ed *editor) {
	if len(ed.sel) < 2 {
		ed.msg = "выделите ≥ 2 блока: рамкой на пустом, Shift+клик или Ctrl+A"
		return
	}
	var op func(*core.Document, []string, float32)
	human := ""
	switch name {
	case "A", "a":
		op, human = core.AlignLefts, "align left"
	case "R", "r":
		op, human = core.AlignRights, "align right"
	case "T", "t":
		op, human = core.AlignTops, "align top"
	case "B", "b":
		op, human = core.AlignBottoms, "align bottom"
	case "H", "h":
		op, human = core.AlignCentersX, "center X"
	case "V", "v":
		op, human = core.AlignCentersY, "center Y"
	case "D", "d":
		if shift {
			op, human = core.DistributeY, "distribute Y"
		} else {
			op, human = core.DistributeX, "distribute X"
		}
	default:
		return
	}
	ed.history.Capture(ed.doc)
	op(ed.doc, selIDs(ed.sel), gridStep)
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.msg = human
}

func undoMark(ed *editor) {
	if ed.history.Undo(ed.doc) {
		ed.dirty = true
		ed.sel = nil
		ed.msg = "undo"
	} else {
		ed.msg = "undo: истории нет"
	}
}

func redoMark(ed *editor) {
	if ed.history.Redo(ed.doc) {
		ed.dirty = true
		ed.sel = nil
		ed.msg = "redo"
	} else {
		ed.msg = "redo: нечего повторять"
	}
}

// addPinBlock — новый блок с n пинами под курсором (клавиши 1..6).
func addPinBlock(ed *editor, n int) {
	world := ed.cam.ScreenToWorld(ed.cursor)
	ed.history.Capture(ed.doc)
	b := ed.doc.AddBlock(&core.Block{
		Kind: "block",
		X:    geom.Snap(world.X-60, gridStep),
		Y:    geom.Snap(world.Y-32, gridStep),
		W:    120, H: 64,
		Pins: n,
	})
	b.Label = b.ID
	PushOverlaps(ed.doc, b.ID) // не кладём поверх соседей — встаёт «рядом»
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.msg = fmt.Sprintf("добавлен %s (пинов %d)", b.ID, n)
}

// pasteClip — вставка буфера под курсором: центр bbox бежит к курсору (снап),
// вставленное становится выделением; через Capture/Finish — undo-шаг.
func pasteClip(ed *editor) {
	if ed.clip == nil || len(ed.clip.Blocks) == 0 {
		ed.msg = "буфер пуст (Ctrl+C на выделении)"
		return
	}
	world := ed.cam.ScreenToWorld(ed.cursor)
	var x0, y0, x1, y1 float32
	for i, b := range ed.clip.Blocks {
		if i == 0 || b.X < x0 {
			x0 = b.X
		}
		if i == 0 || b.Y < y0 {
			y0 = b.Y
		}
		if i == 0 || b.X+b.W > x1 {
			x1 = b.X + b.W
		}
		if i == 0 || b.Y+b.H > y1 {
			y1 = b.Y + b.H
		}
	}
	center := geom.Vec{X: (x0 + x1) / 2, Y: (y0 + y1) / 2}
	dx := geom.Snap(world.X-center.X, gridStep)
	dy := geom.Snap(world.Y-center.Y, gridStep)

	ed.history.Capture(ed.doc)
	newIDs := ed.doc.Paste(ed.clip, dx, dy)
	ed.sel = map[string]bool{}
	for _, id := range newIDs {
		ed.sel[id] = true
	}
	pushOverlapsSet(ed.doc, ed.sel)
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.msg = fmt.Sprintf("вставлено %d блоков", len(newIDs))
}

// deleteAtCursor — Del/Backspace: выделение целиком, иначе блок под курсором.
func deleteAtCursor(ed *editor) {
	ids := selIDs(ed.sel)
	if len(ids) == 0 {
		world := ed.cam.ScreenToWorld(ed.cursor)
		b := ed.doc.HitBlock(world)
		if b == nil {
			ed.msg = "под курсором нет блока"
			return
		}
		ids = []string{b.ID}
	}
	ed.history.Capture(ed.doc)
	for _, id := range ids {
		ed.doc.RemoveBlock(id)
	}
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.sel = nil
	ed.msg = fmt.Sprintf("удалено блоков: %d", len(ids))
}

// --- Файлы ------------------------------------------------------------------

func saveDoc(ed *editor) {
	data, err := json.Marshal(ed.doc)
	if err != nil {
		ed.msg = fmt.Sprintf("save: %v", err)
		return
	}
	if err := os.WriteFile(ed.file, data, 0o644); err != nil {
		ed.msg = fmt.Sprintf("save: %v", err)
		return
	}
	ed.dirty = false
	ed.msg = "сохранено " + ed.file
}

func loadDoc(ed *editor) {
	data, err := os.ReadFile(ed.file)
	if err != nil {
		ed.msg = fmt.Sprintf("load: %v", err)
		return
	}
	nd := &core.Document{}
	if err := json.Unmarshal(data, nd); err != nil {
		ed.msg = fmt.Sprintf("load: %v", err)
		return
	}
	ed.doc = nd
	ed.history.Reset(nd)
	ed.dirty = false
	ed.sel = nil
	ed.msg = fmt.Sprintf("загружен %s (%d блоков)", ed.file, len(nd.Blocks))
}

// --- Отрисовка -------------------------------------------------------------

var (
	colBG     = color.NRGBA{R: 0xf7, G: 0xf7, B: 0xf8, A: 0xff}
	colGrid   = color.NRGBA{R: 0xd9, G: 0xdc, B: 0xdf, A: 0xff}
	colBlock  = color.NRGBA{R: 0x42, G: 0x74, B: 0xe0, A: 0xff}
	colBord   = color.NRGBA{R: 0x2b, G: 0x4d, B: 0x91, A: 0xff}
	colLink   = color.NRGBA{R: 0x3a, G: 0x5a, B: 0x9a, A: 0xff}
	colJct    = color.NRGBA{R: 0xe0, G: 0x6a, B: 0x2a, A: 0xff} // junction-пин
	colSel    = color.NRGBA{R: 0xf2, G: 0x96, B: 0x2e, A: 0xff} // рамка выделения
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
	drawSelBox(gtx, ed)
	drawHUD(gtx, ed)
}

// drawSelBox — полупрозрачная рамка выделения во время натягивания.
func drawSelBox(gtx layout.Context, ed *editor) {
	if ed.mode != modeBoxSelect {
		return
	}
	x0, x1 := min(ed.boxA.X, ed.boxB.X), max(ed.boxA.X, ed.boxB.X)
	y0, y1 := min(ed.boxA.Y, ed.boxB.Y), max(ed.boxA.Y, ed.boxB.Y)
	if (x1-x0) < 1 || (y1-y0) < 1 {
		return
	}
	a := ed.cam.WorldToScreen(geom.Vec{X: x0, Y: y0})
	b := ed.cam.WorldToScreen(geom.Vec{X: x1, Y: y1})
	r := clip.Rect{Min: image.Pt(int(a.X), int(a.Y)), Max: image.Pt(int(b.X), int(b.Y))}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x42, G: 0x74, B: 0xe0, A: 0x28}, r.Op())
	stroke(gtx.Ops, r.Path(), 1.5, colLink)
}

func drawGrid(gtx layout.Context, ed *editor) {
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	minW := ed.cam.ScreenToWorld(geom.Vec{X: 0, Y: 0})
	maxW := ed.cam.ScreenToWorld(geom.Vec{X: w, Y: h})
	minWx, minWy, maxWx, maxWy := minW.X, minW.Y, maxW.X, maxW.Y

	startX := geom.Snap(max(minWx, 0), gridStep)
	for x := startX; x <= min(maxWx, 100000); x += gridStep {
		p1 := ed.cam.WorldToScreen(geom.Vec{X: x, Y: max(minWy, 0)})
		p2 := ed.cam.WorldToScreen(geom.Vec{X: x, Y: min(maxWy, 100000)})
		drawSeg(gtx.Ops, p1, p2, 1, colGrid)
	}
	startY := geom.Snap(max(minWy, 0), gridStep)
	for y := startY; y <= min(maxWy, 100000); y += gridStep {
		p1 := ed.cam.WorldToScreen(geom.Vec{X: max(minWx, 0), Y: y})
		p2 := ed.cam.WorldToScreen(geom.Vec{X: min(maxWx, 100000), Y: y})
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
	pts := []geom.Vec{p, {X: c.X, Y: p.Y}, c}
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
		p := ed.cam.WorldToScreen(geom.Vec{X: b.X, Y: b.Y})
		s := ed.cam.WorldToScreen(geom.Vec{X: b.X + b.W, Y: b.Y + b.H})
		r := clip.Rect{Min: image.Pt(int(p.X), int(p.Y)), Max: image.Pt(int(s.X), int(s.Y))}
		paint.FillShape(gtx.Ops, colBlock, r.Op())
		border := colBord
		bw := float32(1.5)
		if ed.sel[b.ID] { // выделение — оранжевая рамка
			border = colSel
			bw = 2.5
		}
		stroke(gtx.Ops, r.Path(), bw, border)

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
	undoN, redoN := ed.history.Depth()
	dirtyMark := " "
	if ed.dirty {
		dirtyMark = " •"
	}
	groups := len(ed.doc.LinkGroups())
	free := len(ed.doc.FreePins())
	info := fmt.Sprintf("%s%s | undo:%d redo:%d | sel:%d | blocks %d | links %d | clusters %d | free pins %d | zoom %d%%",
		ed.file, dirtyMark, undoN, redoN, len(ed.sel),
		len(ed.doc.Blocks), len(ed.doc.Links), groups, free, int(ed.cam.Zoom*100))
	if ed.mode == modeLink {
		info += "   |   тянем от пина — отпусти на целевом пине"
	}
	drawHUDLine(gtx, ed.th, info, 0, colText)
	if ed.msg != "" {
		drawHUDLine(gtx, ed.th, ed.msg, 16, colJct)
	}
	h := float32(gtx.Constraints.Max.Y)
	hint := "1..6: блок с N пинами   Del: удалить   Ctrl+C/V: копить/вставить   " +
		"рамка на пустом: выделить   Shift+клик: +выделение   A/R/T/B: края   " +
		"H/V: центры   D: распределить   Ctrl+Z/Y: undo/redo   Ctrl+A: все   Ctrl+S/O: файл"
	drawHUDLine(gtx, ed.th, hint, h-18, colText)
}

// drawHUDLine — одна строка HUD со смещением от верхнего левого угла (и цветом).
func drawHUDLine(gtx layout.Context, th *material.Theme, txt string, dy float32, c color.NRGBA) {
	t := op.Offset(image.Pt(0, int(dy))).Push(gtx.Ops)
	l := material.Label(th, unit.Sp(11), txt)
	l.Color = c
	l.Layout(gtx)
	t.Pop()
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
			e.Source.Execute(key.FocusCmd{Tag: keyTag}) // канва держит фокус клавиатуры
			handleInput(e.Source, ed)
			handleKeys(e.Source, ed)
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
