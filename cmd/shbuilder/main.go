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
	"slices"

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
	pxPerMM  = 4.0     // масштаб «100%»: экранных пикселей на 1 мм мира
	outset   = 5.0     // отступ выхода провода от пина, мм
	pinHitR  = 3.0     // радиус попадания в пин, мм
	pinSize  = 1.5     // половина размера пина, мм
	pinGap   = 2.0     // зазор до чужого пина: провод не идёт через точку подключения, мм
	laneGap  = 2.0     // разводка коллинеарных наложений линков, мм
	stickyMM = 3.0     // гистерезис стороны обхода при сдвиге блока, мм
	arcR     = 1.6     // радиус арки на пересечении проводников, мм
	worldMax = 50000.0 // граница мира, мм (рисуем сетку до неё)
)

// gridChoices — циклу по G: шаг сетки, мм.
var gridChoices = [...]float32{5.0, 10.0, 20.0}

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

	grid    float32 // шаг координатной сетки, мм
	gridIdx int     // индекс в gridChoices (для цикла по G)
	viewW   float32 // размер окна в px (обновляется в drawFrame)
	viewH   float32
	lastKey string // последняя нажатая клавиша (отладка клавиатуры)
	// lastRoutes — прошлые маршруты линков по ключу: память стороны обхода
	// (гистерезис), чтобы линия не перескакивала от микросдвига блока.
	lastRoutes map[string][]geom.Vec

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
	selLinkKey string          // ключ выделенного коннектора «from->to» (пусто — не выделен)
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
		doc:     d,
		cam:     geom.Camera{Zoom: pxPerMM},
		grid:    10, // мм (см. gridChoices[1])
		gridIdx: 1,  // 10 мм — умолчание
		th:      material.NewTheme(),
		file:    path,
		msg:     msg,
	}
	// Инвариант: блоки не накладываются. Старый/ручной файл с пересечениями
	// разводим сразу, до снимка истории (undo не должен возвращать наложения).
	if core.HasOverlaps(d) {
		core.PushOverlaps(d, nil, 10, 64)
		ed.msg += " (перекрытия разведены)"
	}
	ed.history.Reset(d)
	return ed
}

// demoScene — стартовая сцена в мм: блоки 1–4 пинов, junction, перекрёстные линки.
func demoScene() *core.Document {
	d := &core.Document{}
	d.AddBlock(&core.Block{ID: "A", Kind: "src", X: 40, Y: 40, W: 30, H: 20, Label: "A", Pins: 2})
	d.AddBlock(&core.Block{ID: "B", Kind: "hub", X: 120, Y: 60, W: 30, H: 20, Label: "B", Pins: 4})
	d.AddBlock(&core.Block{ID: "C", Kind: "load", X: 40, Y: 130, W: 30, H: 20, Label: "C", Pins: 1})
	d.AddBlock(&core.Block{ID: "D", Kind: "term", X: 140, Y: 150, W: 30, H: 20, Label: "D", Pins: 3})
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

		// Палитра слева: клик по строке ставит блок в центр видимой канвы.
		// Если идёт драг/линк/панорама, курсор может заходить на палитру —
		// такие события не перехватываем, иначе драг «залипнет».
		if inPalette(scr) {
			if e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonPrimary) {
				if n, ok := paletteHit(scr); ok {
					b := addPinBlockAt(ed, n, ed.canvasInsertScreen())
					ed.sel = map[string]bool{b.ID: true}
					ed.msg = fmt.Sprintf("из палитры: %s (%d %s)", b.ID, n, pinWord(n))
				} else {
					ed.msg = "палитра: выберите строку (1..6 пинов)"
				}
				continue
			}
			if ed.mode == modeFree {
				continue // палитра не отдаёт канве клики/колесо
			}
		}

		// клик по мини-карте — перенести зону видимости на кликнутую точку
		if e.Kind == pointer.Press && e.Buttons.Contain(pointer.ButtonPrimary) {
			if x0, y0, x1, y1 := miniMapScreenRect(ed); scr.X >= x0-3 && scr.X <= x1+3 && scr.Y >= y0-3 && scr.Y <= y1+3 {
				box := miniMapBox(ed)
				ww, wh := maxf(box.Size().X, 1), maxf(box.Size().Y, 1)
				sc := minf(miniMapW/ww, miniMapH/wh)
				worldX := box.Min.X + (scr.X-x0)/sc
				worldY := box.Min.Y + (scr.Y-y0)/sc
				ed.cam.Pan = geom.Vec{X: ed.viewW/2 - worldX*ed.cam.Zoom, Y: ed.viewH/2 - worldY*ed.cam.Zoom}
				ed.msg = "переход по мини-карте"
				continue
			}
		}

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
				} else if li, ok := ed.hitLink(world); ok {
					// клик по коннектору — выделить его, снять выделение блоков
					ed.sel = nil
					ed.selLinkKey = linkKey(ed.doc.Links[li])
					ed.msg = fmt.Sprintf("коннектор %s выделен", ed.selLinkKey)
				} else {
					ed.mode = modeBoxSelect
					ed.boxA, ed.boxB = world, world
				}
				q.Execute(pointer.GrabCmd{Tag: canvasTag, ID: e.PointerID})

			case e.Buttons.Contain(pointer.ButtonSecondary) ||
				e.Buttons.Contain(pointer.ButtonTertiary): // ПКМ/СКМ — панорама
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
						ed.snapSelection()
						ed.pushOverlapsSel()
					} else {
						ed.doc.MoveBlock(b.ID, geom.Snap(b.X, ed.grid), geom.Snap(b.Y, ed.grid))
						ed.pushOverlaps(b.ID)
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
			if ed.mode == modeFree { // в драге зум только путает координаты
				// Скролл на Windows может присылать большие значения Scroll.Y (±120 за тик).
				// Берём только знак — один тик = 5% зума, независимо от размера дельты.
				var factor float32
				switch {
				case e.Scroll.Y < 0: // колесо на себя — зум в
					factor = 1.05
				case e.Scroll.Y > 0: // колесо от себя — зум вы
					factor = 1.0 / 1.05
				default:
					factor = 1.0
				}
				if factor != 1.0 {
					ed.cam.ZoomAt(scr, factor)
				}
			}
		}
	}
}

// PushOverlaps — тонкие обёртки над core.PushOverlaps с текущим шагом сетки.
// Сначала двигаются незащищённые блоки; если проходов не хватило и наложения
// остались, разрешаем двигать всех — инвариант «блоки не пересекаются» важнее.
func (ed *editor) pushOverlaps(id string) {
	core.PushOverlaps(ed.doc, map[string]bool{id: true}, ed.grid, 64)
	if core.HasOverlaps(ed.doc) {
		core.PushOverlaps(ed.doc, nil, ed.grid, 64)
	}
}

func (ed *editor) pushOverlapsSel() {
	core.PushOverlaps(ed.doc, selSet(ed.sel), ed.grid, 64)
	if core.HasOverlaps(ed.doc) {
		core.PushOverlaps(ed.doc, nil, ed.grid, 64)
	}
}

// snapSelection — snap к текущей сетке для всех выделенных.
func (ed *editor) snapSelection() {
	for id := range ed.sel {
		if b := ed.doc.BlockByID(id); b != nil {
			b.X = geom.Snap(b.X, ed.grid)
			b.Y = geom.Snap(b.Y, ed.grid)
		}
	}
}

// --- Выделение --------------------------------------------------------------

func selIDs(sel map[string]bool) []string {
	ids := make([]string, 0, len(sel))
	for id := range sel {
		ids = append(ids, id)
	}
	return ids
}

// selSet — карта выделения как она есть (для core.PushOverlaps).
func selSet(sel map[string]bool) map[string]bool { return sel }

// boxSelectFinish завершает рамку: заменяет/дополняет выделение пересечёнными
// блоками; вырожденная рамка (клик по пустому) снимает выделение.
func (ed *editor) boxSelectFinish(world geom.Vec, mods key.Modifiers) {
	x0, x1 := min(ed.boxA.X, world.X), max(ed.boxA.X, world.X)
	y0, y1 := min(ed.boxA.Y, world.Y), max(ed.boxA.Y, world.Y)
	degenerate := (x1-x0) < 0.5 && (y1-y0) < 0.5 // мм: обычный клик без рамки
	if degenerate {
		ed.sel = nil
		ed.selLinkKey = ""
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
func (ed *editor) clearSelection() { ed.sel = nil; ed.selLinkKey = "" }

// hitLink ищет коннектор под точкой world (мировые координаты).
// Порог попадания — 2.5 мм (ikonка курсора вписывается в этот круг).
func (ed *editor) hitLink(world geom.Vec) (int, bool) {
	const hit = 2.5 // мм
	for i, l := range ed.doc.Links {
		p1, s1, ok1 := ed.doc.PinPos(l.From)
		p2, s2, ok2 := ed.doc.PinPos(l.To)
		if !ok1 || !ok2 {
			continue
		}
		path := geom.RouteEx(p1, s1, p2, s2, outset, geom.RouteOpts{
			Obstacles: linkObstacles(ed.doc, l.From, l.To),
		})
		for k := 1; k < len(path); k++ {
			a, b := path[k-1], path[k]
			d, _ := geom.SegPointDist(a, b, world)
			if d <= hit {
				return i, true
			}
		}
	}
	return -1, false
}

// --- Клавиатура -------------------------------------------------------------

var keyTag = new(struct{})

// keyFilter — catch-all по клавишам в фокусе канвы. Optional обязан
// перечислять ВСЕ модификаторы: keyFilterMatch отбрасывает событие, если
// e.Modifiers&^(Required|Optional) != 0 — без этого любое нажатие с зажатым
// Ctrl/Shift/Alt (Ctrl+Z, Shift+стрелки и т.п.) до handleKeys не доходит.
var keyFilter = key.Filter{
	Focus:    keyTag,
	Optional: key.ModShift | key.ModCtrl | key.ModCommand | key.ModAlt | key.ModSuper,
}

// handleKeys потребляет key-события текущего кадра.
func handleKeys(q input.Source, ed *editor) {
	for {
		// FocusFilter — не для событий, а ради жизненного цикла фокуса:
		// без него keyQueue.Frame() в конце каждого кадра снимает фокус
		// (нет handler'а с filter.focusable) и клавиши перестают доходить.
		ke, ok := q.Event(keyFilter, key.FocusFilter{Target: keyTag}) // пустое Name — все клавиши
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
	ed.lastKey = string(k.Name) // отладка: фиксируем каждую нажатую клавишу
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
	case !mod && ed.mode == modeFree && k.Name == key.NameHome:
		fitToWindow(ed) // вся схема в окно
	case !mod && ed.mode == modeFree && k.Name == "0":
		ed.cam.Zoom = pxPerMM // сброс зума к 100%
		ed.msg = "зум 100%"
	case !mod && ed.mode == modeFree && k.Name == "=":
		ed.cam.Zoom = geom.ClampZoom(ed.cam.Zoom * 1.1) // зум +
		ed.msg = fmt.Sprintf("зум %d%%", int(ed.cam.Zoom/pxPerMM*100))
	case !mod && ed.mode == modeFree && k.Name == "-":
		ed.cam.Zoom = geom.ClampZoom(ed.cam.Zoom / 1.1) // зум –
		ed.msg = fmt.Sprintf("зум %d%%", int(ed.cam.Zoom/pxPerMM*100))
	case !mod && ed.mode == modeFree &&
		(k.Name == key.NameLeftArrow || k.Name == key.NameRightArrow ||
			k.Name == key.NameUpArrow || k.Name == key.NameDownArrow):
		s := ed.grid * ed.cam.Zoom // шаг стрелками = шаг сетки
		if shift {
			s *= 5
		}
		switch k.Name {
		case key.NameLeftArrow:
			ed.cam.Pan.X += s
		case key.NameRightArrow:
			ed.cam.Pan.X -= s
		case key.NameUpArrow:
			ed.cam.Pan.Y += s
		case key.NameDownArrow:
			ed.cam.Pan.Y -= s
		}
	case !mod && ed.mode == modeFree && len(k.Name) == 1 && k.Name[0] >= '1' && k.Name[0] <= '6':
		addPinBlock(ed, int(k.Name[0]-'0'))
	case !mod && ed.mode == modeFree &&
		(k.Name == key.NameDeleteForward || k.Name == key.NameDeleteBackward):
		if ed.selLinkKey != "" {
			ed.deleteLink(ed.selLinkKey)
		} else {
			deleteAtCursor(ed)
		}
	case !mod && ed.mode == modeFree && (k.Name == "G" || k.Name == "g"):
		// цикл шага сетки: 5 → 10 → 20 → 5 (мм), вызывается ДО arrangeKey
		ed.gridIdx = (ed.gridIdx + 1) % len(gridChoices)
		ed.grid = gridChoices[ed.gridIdx]
		ed.msg = fmt.Sprintf("шаг сетки: %v мм", ed.grid)
	case !mod && ed.mode == modeFree && (k.Name == "L" || k.Name == "l"):
		layoutKey(ed)
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
	op(ed.doc, selIDs(ed.sel), ed.grid)
	// Выравнивание/распределение может свести блоки друг на друга — сразу
	// расталкиваем, иначе нарушается инвариант «блоки не пересекаются».
	if !core.PushOverlaps(ed.doc, nil, ed.grid, 64) {
		human += " (наложения не развелись)"
	}
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.msg = human
}

// layoutKey — автолайаут (roadmap 1.7): слои слева направо, источники слева.
// Выделено ≥ 2 блоков — раскладываем только их, иначе весь документ.
// Один undo-шаг; маршруты пересчитываются с нуля (сброс памяти гистерезиса).
func layoutKey(ed *editor) {
	var ids []string
	if len(ed.sel) >= 2 {
		ids = selIDs(ed.sel)
	}
	ed.history.Capture(ed.doc)
	n, layers := core.Layout(ed.doc, ids, ed.grid)
	msg := ""
	if n > 0 {
		// Разложенные блоки защищены от расталкивания: свежая раскладка
		// важнее, уводить с дороги нужно чужие блоки.
		laid := make(map[string]bool, n)
		if ids == nil {
			for _, b := range ed.doc.Blocks {
				laid[b.ID] = true
			}
		} else {
			for _, id := range ids {
				laid[id] = true
			}
		}
		if !core.PushOverlaps(ed.doc, laid, ed.grid, 64) {
			core.PushOverlaps(ed.doc, nil, ed.grid, 64)
			msg = " (наложения не развелись)"
		}
		ed.lastRoutes = nil
		msg = fmt.Sprintf("автолайаут: %d блоков, %d слоёв%s", n, layers, msg)
	} else {
		msg = "автолайаут: нечего раскладывать (нужно ≥ 2 блока)"
	}
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.msg = msg
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

// addPinBlock — новый блок с n пинами (клавиши 1..6): под курсором, а если
// курсор вне канвы (палитра/за краем окна) — в центре видимой части канвы.
func addPinBlock(ed *editor, n int) {
	addPinBlockAt(ed, n, ed.canvasInsertScreen())
}

// canvasInsertScreen — экранная точка вставки: курсор, если он над канвой,
// иначе центр видимой части канвы (полоса палитры слева не считается).
func (ed *editor) canvasInsertScreen() geom.Vec {
	p := ed.cursor
	if ed.viewW > paletteW && ed.viewH > 0 &&
		p.X >= paletteW && p.X <= ed.viewW && p.Y >= 0 && p.Y <= ed.viewH {
		return p
	}
	if ed.viewW <= paletteW || ed.viewH <= 0 {
		return p // окно ещё не измерено — вставляем под курсором
	}
	return geom.Vec{X: (paletteW + ed.viewW) / 2, Y: ed.viewH / 2}
}

// addPinBlockAt — вставка блока с n пинами в экранной точке scr (undo-шаг).
func addPinBlockAt(ed *editor, n int, scr geom.Vec) *core.Block {
	world := ed.cam.ScreenToWorld(scr)
	ed.history.Capture(ed.doc)
	b := ed.doc.AddBlock(&core.Block{
		Kind: "block",
		X:    geom.Snap(world.X-15, ed.grid),
		Y:    geom.Snap(world.Y-10, ed.grid),
		W:    30, H: 20, // мм: блок по умолчанию 30×20
		Pins: n,
	})
	b.Label = b.ID
	ed.pushOverlaps(b.ID) // не кладём поверх соседей — встаёт «рядом»
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.msg = fmt.Sprintf("добавлен %s (пинов %d)", b.ID, n)
	return b
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
	dx := geom.Snap(world.X-center.X, ed.grid)
	dy := geom.Snap(world.Y-center.Y, ed.grid)

	ed.history.Capture(ed.doc)
	newIDs := ed.doc.Paste(ed.clip, dx, dy)
	ed.sel = map[string]bool{}
	for _, id := range newIDs {
		ed.sel[id] = true
	}
	ed.pushOverlapsSel()
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

// deleteLink удаляет коннектор по ключу «from->to» и все T-соединения,
// ссылающиеся на него как на хоста. undo/redo через Capture/Finish.
func (ed *editor) deleteLink(key string) {
	ed.history.Capture(ed.doc)
	for i, l := range ed.doc.Links {
		if linkKey(l) == key {
			ed.doc.Links = append(ed.doc.Links[:i], ed.doc.Links[i+1:]...)
			break
		}
	}
	// T-соединения, чьим хозяином был удалённый линк, больше не имеют смысла.
	taps := ed.doc.Taps[:0]
	for _, t := range ed.doc.Taps {
		if t.Tap.Host != key {
			taps = append(taps, t)
		}
	}
	ed.doc.Taps = taps
	if ed.history.Finish(ed.doc) {
		ed.dirty = true
	}
	ed.selLinkKey = ""
	ed.msg = "коннектор удалён"
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
	note := ""
	if core.HasOverlaps(nd) {
		core.PushOverlaps(nd, nil, ed.grid, 64)
		note = " (перекрытия разведены)"
	}
	ed.doc = nd
	ed.history.Reset(nd)
	ed.dirty = false
	ed.sel = nil
	ed.msg = fmt.Sprintf("загружен %s (%d блоков)%s", ed.file, len(nd.Blocks), note)
}

// --- Отрисовка -------------------------------------------------------------

var (
	colBG      = color.NRGBA{R: 0xf7, G: 0xf7, B: 0xf8, A: 0xff}
	colGrid    = color.NRGBA{R: 0xd9, G: 0xdc, B: 0xdf, A: 0xff}
	colBlock   = color.NRGBA{R: 0x42, G: 0x74, B: 0xe0, A: 0xff}
	colBord    = color.NRGBA{R: 0x2b, G: 0x4d, B: 0x91, A: 0xff}
	colLink    = color.NRGBA{R: 0x3a, G: 0x5a, B: 0x9a, A: 0xff}
	colJct     = color.NRGBA{R: 0xe0, G: 0x6a, B: 0x2a, A: 0xff} // junction-пин
	colSel     = color.NRGBA{R: 0xf2, G: 0x96, B: 0x2e, A: 0xff} // рамка выделения
	colFree    = color.NRGBA{R: 0xaa, G: 0xaa, B: 0xb2, A: 0xff} // свободный пин
	colRubber  = color.NRGBA{R: 0x3a, G: 0x5a, B: 0x9a, A: 0x80}
	colText    = color.NRGBA{R: 0x55, G: 0x55, B: 0x60, A: 0xff}
	colTextDim = color.NRGBA{R: 0x8a, G: 0x8a, B: 0x96, A: 0xff}
	colPalBG   = color.NRGBA{R: 0xec, G: 0xee, B: 0xf2, A: 0xff} // фон палитры
	colPalBord = color.NRGBA{R: 0xc2, G: 0xc7, B: 0xd0, A: 0xff} // кромка палитры
	colPalHov  = color.NRGBA{R: 0x42, G: 0x74, B: 0xe0, A: 0x22} // подсветка строки
)

// Палитра блоков слева (roadmap 1.1): ширина полосы, геометрия строк.
const (
	paletteW     = float32(132) // ширина палитры, px
	paletteTitle = float32(46)  // высота заголовка, px
	paletteRowH  = float32(46)  // высота строки, px
	palPreviewW  = float32(38)  // мини-блок, px
	palPreviewH  = float32(26)
)

// paletteRowRect — строка палитры i (0..5 = блок с 1..6 пинами), экран.
func paletteRowRect(i int) geom.Rect {
	return geom.NewRect(0, paletteTitle+float32(i)*paletteRowH, paletteW, paletteRowH)
}

// paletteHit — какая строка палитры накрыта точкой (n = 1..6).
func paletteHit(scr geom.Vec) (int, bool) {
	for i := 0; i < 6; i++ {
		if paletteRowRect(i).Contains(scr) {
			return i + 1, true
		}
	}
	return 0, false
}

// inPalette — точка лежит в полосе палитры (клики не уходят на канву).
func inPalette(scr geom.Vec) bool { return scr.X >= 0 && scr.X <= paletteW }

// pinWord — «1 пин / 2 пина / 5 пинов» для подписи строки.
func pinWord(n int) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return "пин"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return "пина"
	default:
		return "пинов"
	}
}

func drawFrame(gtx layout.Context, ed *editor) {
	ed.viewW = float32(gtx.Constraints.Max.X)
	ed.viewH = float32(gtx.Constraints.Max.Y)
	paint.FillShape(gtx.Ops, colBG,
		clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}.Op())
	drawGrid(gtx, ed)
	drawLinks(gtx, ed)
	drawBlocks(gtx, ed)
	drawPins(gtx, ed)
	drawRubber(gtx, ed)
	drawSelBox(gtx, ed)
	drawMiniMap(gtx, ed)
	drawPalette(gtx, ed)
	drawHUD(gtx, ed)
}

// docBBox — габарит всех блоков с полями pad (мм); пустой документ — условный мир.
func docBBox(d *core.Document, pad float32) geom.Rect {
	if len(d.Blocks) == 0 {
		return geom.NewRect(-pad, -pad, 150+2*pad, 100+2*pad)
	}
	r := d.Blocks[0].Rect()
	for _, b := range d.Blocks[1:] {
		br := b.Rect()
		r.Min.X, r.Min.Y = minf(r.Min.X, br.Min.X), minf(r.Min.Y, br.Min.Y)
		r.Max.X, r.Max.Y = maxf(r.Max.X, br.Max.X), maxf(r.Max.Y, br.Max.Y)
	}
	return geom.Rect{
		Min: geom.Vec{X: r.Min.X - pad, Y: r.Min.Y - pad},
		Max: geom.Vec{X: r.Max.X + pad, Y: r.Max.Y + pad},
	}
}

// fitToWindow — «Home»: вся схема целиком в видимой канве (палитра слева
// отнимает полосу) с полями.
func fitToWindow(ed *editor) {
	if ed.viewW == 0 || ed.viewH == 0 {
		return
	}
	r := docBBox(ed.doc, 20)
	w, h := maxf(r.Size().X, 1), maxf(r.Size().Y, 1)
	const margin = 48.0 // px со всех сторон
	availW := maxf(ed.viewW-paletteW, 1)
	z := geom.ClampZoom(minf(maxf((availW-2*margin)/w, 0.0001), (ed.viewH-2*margin)/h))
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	sx := paletteW + availW/2 // центр видимой канвы на экране
	ed.cam = geom.Camera{
		Zoom: z,
		Pan:  geom.Vec{X: sx - cx*z, Y: ed.viewH/2 - cy*z},
	}
	ed.msg = "вся схема в окне"
}

const (
	miniMapW    = float32(150) // ширина мини-карты, px
	miniMapH    = float32(96)  // высота мини-карты, px
	miniMapG    = float32(16)  // отступ от краёв окна, px
	miniMapPadY = float32(20)  // отступ от строки подсказки, px
)

// miniMapBox — габарит для мини-карты: блоки + текущая видимая область.
func miniMapBox(ed *editor) geom.Rect {
	box := docBBox(ed.doc, 10)
	v0 := ed.cam.ScreenToWorld(geom.Vec{X: 0, Y: 0})
	v1 := ed.cam.ScreenToWorld(geom.Vec{X: ed.viewW, Y: ed.viewH})
	box.Min.X = minf(box.Min.X, v0.X)
	box.Min.Y = minf(box.Min.Y, v0.Y)
	box.Max.X = maxf(box.Max.X, v1.X)
	box.Max.Y = maxf(box.Max.Y, v1.Y)
	return box
}

// miniMapScreenRect — экранный прямоугольник мини-карты.
func miniMapScreenRect(ed *editor) (x0, y0, x1, y1 float32) {
	x0 = ed.viewW - miniMapG - miniMapW
	y0 = ed.viewH - miniMapG - miniMapH - miniMapPadY
	return x0, y0, x0 + miniMapW, y0 + miniMapH
}

// drawMiniMap — уголок-ориентир: уменьшенная схема и рамка видимой области
// (вместо скроллбаров: пан — ПКМ/СКМ, Home — вся схема, стрелки — листать,
// клик по мини-карте — перенести зону видимости).
func drawMiniMap(gtx layout.Context, ed *editor) {
	if ed.viewW == 0 || len(ed.doc.Blocks) == 0 {
		return
	}
	box := miniMapBox(ed)
	ww, wh := maxf(box.Size().X, 1), maxf(box.Size().Y, 1)
	sc := minf(miniMapW/ww, miniMapH/wh)
	ox, oy, _, _ := miniMapScreenRect(ed)
	toMini := func(w geom.Vec) geom.Vec {
		return geom.Vec{X: ox + (w.X-box.Min.X)*sc, Y: oy + (w.Y-box.Min.Y)*sc}
	}
	fillRect(gtx.Ops, image.Rectangle{
		Min: image.Pt(int(ox-3), int(oy-3)),
		Max: image.Pt(int(ox+miniMapW+3), int(oy+miniMapH+3)),
	}, color.NRGBA{R: 0x14, G: 0x18, B: 0x20, A: 0xc0})
	for _, b := range ed.doc.Blocks {
		a := toMini(geom.Vec{X: b.X, Y: b.Y})
		s := toMini(geom.Vec{X: b.X + b.W, Y: b.Y + b.H})
		if s.X-a.X < 2 {
			s.X = a.X + 2 // не теряем мелкие блоки
		}
		if s.Y-a.Y < 2 {
			s.Y = a.Y + 2
		}
		fillRect(gtx.Ops, image.Rectangle{
			Min: image.Pt(int(a.X), int(a.Y)),
			Max: image.Pt(int(s.X), int(s.Y)),
		}, colBlock)
	}
	va, vb := toMini(ed.cam.ScreenToWorld(geom.Vec{X: 0, Y: 0})), toMini(ed.cam.ScreenToWorld(geom.Vec{X: ed.viewW, Y: ed.viewH}))
	// зона видимости: полупрозрачный фон + чёткая оранжевая кайма
	vr := clip.Rect{Min: image.Pt(int(va.X), int(va.Y)), Max: image.Pt(int(vb.X), int(vb.Y))}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0xf2, G: 0x96, B: 0x2e, A: 0x30}, vr.Op())
	stroke(gtx.Ops, vr.Path(), 2.5, colSel)
}

// drawPalette — левая полоса с заготовками блоков: 6 строк (1..6 пинов),
// мини-превью с пинами; клик по строке ставит блок в центр канвы.
func drawPalette(gtx layout.Context, ed *editor) {
	h := float32(gtx.Constraints.Max.Y)
	fillRect(gtx.Ops, image.Rect(0, 0, int(paletteW), int(h)), colPalBG)
	drawSeg(gtx.Ops, geom.Vec{X: paletteW, Y: 0}, geom.Vec{X: paletteW, Y: h}, 1.5, colPalBord)

	label := func(txt string, x, y float32, sp unit.Sp, c color.NRGBA) {
		t := op.Offset(image.Pt(int(x), int(y))).Push(gtx.Ops)
		l := material.Label(ed.th, sp, txt)
		l.Color = c
		l.Layout(gtx)
		t.Pop()
	}
	label("Палитра", 10, 8, 12, colText)
	label("клик или 1..6", 10, 26, 9, colTextDim)

	for i := 0; i < 6; i++ {
		n := i + 1
		row := paletteRowRect(i)
		if ed.mode == modeFree && row.Contains(ed.cursor) {
			fillRect(gtx.Ops, image.Rectangle{
				Min: image.Pt(int(row.Min.X)+4, int(row.Min.Y)+3),
				Max: image.Pt(int(row.Max.X)-4, int(row.Max.Y)-3),
			}, colPalHov)
		}
		px := float32(10)
		py := row.Min.Y + (row.Size().Y-palPreviewH)/2
		drawPalettePreview(gtx, ed, n, px, py)
		label(fmt.Sprintf("%d %s", n, pinWord(n)), px+palPreviewW+8, py+5, 10, colText)
	}
}

// drawPalettePreview — мини-блок с пинами по раскладке core.PinLayout.
func drawPalettePreview(gtx layout.Context, ed *editor, pins int, x, y float32) {
	r := clip.Rect{Min: image.Pt(int(x), int(y)), Max: image.Pt(int(x+palPreviewW), int(y+palPreviewH))}
	paint.FillShape(gtx.Ops, colBlock, r.Op())
	stroke(gtx.Ops, r.Path(), 1, colBord)
	b := &core.Block{X: x, Y: y, W: palPreviewW, H: palPreviewH, Pins: pins}
	for i := 0; i < len(core.PinLayout(pins)); i++ {
		pos, _, ok := b.PinPos(i)
		if !ok {
			continue
		}
		fillRect(gtx.Ops, image.Rectangle{
			Min: image.Pt(int(pos.X)-2, int(pos.Y)-2),
			Max: image.Pt(int(pos.X)+2, int(pos.Y)+2),
		}, colFree)
	}
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
	step := ed.grid
	for step*ed.cam.Zoom < 3 { // линия реже 3 px — разрежаем шаг ×2 (5→10→20…)
		step *= 2
	}
	w, h := float32(gtx.Constraints.Max.X), float32(gtx.Constraints.Max.Y)
	minW := ed.cam.ScreenToWorld(geom.Vec{X: 0, Y: 0})
	maxW := ed.cam.ScreenToWorld(geom.Vec{X: w, Y: h})
	minWx, minWy, maxWx, maxWy := minW.X, minW.Y, maxW.X, maxW.Y

	startX := geom.Snap(max(minWx, 0), step)
	for x := startX; x <= min(maxWx, worldMax); x += step {
		p1 := ed.cam.WorldToScreen(geom.Vec{X: x, Y: max(minWy, 0)})
		p2 := ed.cam.WorldToScreen(geom.Vec{X: x, Y: min(maxWy, worldMax)})
		drawSeg(gtx.Ops, p1, p2, 1, colGrid)
	}
	startY := geom.Snap(max(minWy, 0), step)
	for y := startY; y <= min(maxWy, worldMax); y += step {
		p1 := ed.cam.WorldToScreen(geom.Vec{X: max(minWx, 0), Y: y})
		p2 := ed.cam.WorldToScreen(geom.Vec{X: min(maxWx, worldMax), Y: y})
		drawSeg(gtx.Ops, p1, p2, 1, colGrid)
	}
}

// compareLinks — стабильный ключ порядка отрисовки линков (П10): от порядка
// создания не зависит, поэтому мостики не «перепрыгивают» между сессиями.
// См. docs/ALGORITHMS.md §9.1.
func compareLinks(a, b core.Link) int {
	switch {
	case a.From.BlockID < b.From.BlockID:
		return -1
	case a.From.BlockID > b.From.BlockID:
		return 1
	case a.From.Pin < b.From.Pin:
		return -1
	case a.From.Pin > b.From.Pin:
		return 1
	case a.To.BlockID < b.To.BlockID:
		return -1
	case a.To.BlockID > b.To.BlockID:
		return 1
	case a.To.Pin < b.To.Pin:
		return -1
	case a.To.Pin > b.To.Pin:
		return 1
	}
	return 0
}

// drawLinks рисует линки ортогональными маршрутами; пересечение с более ранним
// проводником — «мостик» (арка). Разбор — docs/ALGORITHMS.md §9.1–9.2.
func drawLinks(gtx layout.Context, ed *editor) {
	type seg struct{ a, b geom.Vec }
	type routed struct {
		link core.Link
		path []geom.Vec
		obs  []geom.Rect
	}
	items := make([]routed, 0, len(ed.doc.Links))
	prevRoutes := make(map[string][]geom.Vec, len(ed.doc.Links))
	for _, l := range ed.doc.Links {
		p1, s1, ok1 := ed.doc.PinPos(l.From)
		p2, s2, ok2 := ed.doc.PinPos(l.To)
		if !ok1 || !ok2 {
			continue
		}
		key := linkKey(l)
		path := geom.RouteEx(p1, s1, p2, s2, outset, geom.RouteOpts{
			Obstacles: linkObstacles(ed.doc, l.From, l.To),
			Prev:      ed.lastRoutes[key], // гистерезис стороны обхода
			Sticky:    stickyMM,
		})
		prevRoutes[key] = path
		items = append(items, routed{
			link: l,
			path: path,
			obs:  laneRects(ed.doc, l.From, l.To),
		})
	}
	ed.lastRoutes = prevRoutes

	// П10: порядок отрисовки — по ключу линка, а не по порядку создания.
	slices.SortFunc(items, func(a, b routed) int { return compareLinks(a.link, b.link) })

	// О1: коллинеарные наложения разводим полосами (поздний линк уходит в
	// сторону); сами пути при этом не перестают приходить в свои пины.
	paths := make([][]geom.Vec, len(items))
	rects := make([][]geom.Rect, len(items))
	for i := range items {
		paths[i] = items[i].path
		rects[i] = items[i].obs
	}
	paths = geom.OffsetLanes(paths, rects, laneGap)

	all := make([][]seg, len(items))
	for i := range items {
		segs := make([]seg, 0, len(paths[i])-1)
		for k := 1; k < len(paths[i]); k++ {
			segs = append(segs, seg{a: paths[i][k-1], b: paths[i][k]})
		}
		all[i] = segs
	}

	for pos := range all {
		segs := all[pos]
		// пересечения моего линка с более ранними → параметры вдоль сегментов
		hopsPerSeg := make([][]float32, len(segs))
		for q := 0; q < pos; q++ {
			for _, other := range all[q] {
				for u, s := range segs {
					if pw, ok := geom.SegCross(s.a, s.b, other.a, other.b); ok {
						a := ed.cam.WorldToScreen(s.a)
						b := ed.cam.WorldToScreen(s.b)
						pw := ed.cam.WorldToScreen(pw)
						dx, dy := b.X-a.X, b.Y-a.Y
						dst2 := dx*dx + dy*dy
						if dst2 == 0 {
							continue
						}
						hopsPerSeg[u] = append(hopsPerSeg[u],
							((pw.X-a.X)*dx+(pw.Y-a.Y)*dy)/dst2)
					}
				}
			}
		}

		for u, sg := range segs {
			a := ed.cam.WorldToScreen(sg.a)
			b := ed.cam.WorldToScreen(sg.b)
			hops := hopsPerSeg[u]
			col, width := colLink, float32(2)
			if linkKey(items[pos].link) == ed.selLinkKey {
				col, width = colSel, float32(3) // выделенный коннектор — оранжевый, толще
			}
			if len(hops) == 0 {
				drawSeg(gtx.Ops, a, b, width, col)
				continue
			}
			dx, dy := b.X-a.X, b.Y-a.Y
			length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			rpx := arcR * ed.cam.Zoom // радиус арки, пикселей
			if length < 1 || rpx*2 >= length {
				drawSeg(gtx.Ops, a, b, width, col) // не протыкается — рисуем целиком
				continue
			}
			slices.Sort(hops)
			ux, uy := dx/length, dy/length
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Point{X: a.X, Y: a.Y})
			pass := float32(0) // пройдено вдоль сегмента, px
			for _, t := range hops {
				tp := t * length // позиция пересечения вдоль сегмента
				if tp-rpx <= pass+0.5 || tp+rpx >= length {
					continue // слишком тесно у конца/края — рисуем сплошняком
				}
				p.LineTo(f32.Point{X: a.X + ux*(tp-rpx), Y: a.Y + uy*(tp-rpx)})
				c := f32.Point{X: a.X + ux*tp, Y: a.Y + uy*tp}
				// равные фокусы = окружность с центром в точке пересечения;
				// пол-оборота от входа мостика до выхода по другую сторону
				p.ArcTo(c, c, math.Pi)
				pass = tp + rpx
			}
			if pass < length-0.5 {
				p.LineTo(f32.Point{X: b.X, Y: b.Y})
			}
			stroke(gtx.Ops, p.End(), width, col)
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
		pts = geom.RouteEx(p1, sideOf(ed.doc, ed.linkFrom), p2, s2, outset, geom.RouteOpts{
			Obstacles: linkObstacles(ed.doc, ed.linkFrom, ed.linkTo),
		})
	} else {
		pts = rubberRoute(ed.doc, ed.linkFrom, ed.cam.ScreenToWorld(ed.cursor), outset)
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

// linkKey — стабильный ключ линка для памяти маршрутов (гистерезис).
func linkKey(l core.Link) string { return l.From.String() + "->" + l.To.String() }

// linkObstacles — препятствия маршрута линка: чужие блоки с зоной запрета
// «блок + outset», свои — только без разрезания внутренности, чужие пины —
// точки с зазором pinGap. См. docs/ALGORITHMS.md §9.3.
func linkObstacles(d *core.Document, from, to core.Endpoint) []geom.Obstacle {
	out := make([]geom.Obstacle, 0, len(d.Blocks)+8)
	for _, b := range d.Blocks {
		r := b.Rect()
		if b.ID == from.BlockID || b.ID == to.BlockID {
			out = append(out, geom.Obstacle{Rect: r})
		} else {
			out = append(out, geom.Obstacle{Rect: r, Gap: outset})
		}
	}
	for _, b := range d.Blocks {
		for i := 0; i < len(core.PinLayout(b.Pins)); i++ {
			ep := core.Endpoint{BlockID: b.ID, Pin: i}
			if ep == from || ep == to {
				continue
			}
			p, _, ok := b.PinPos(i)
			if !ok {
				continue
			}
			out = append(out, geom.Obstacle{Rect: geom.Rect{Min: p, Max: p}, Gap: pinGap})
		}
	}
	return out
}

// laneRects — препятствия для сдвига полос (geom.OffsetLanes): блоки и зоны
// чужих пинов. Свои пины не мешают — путь из них выходит.
// См. docs/ALGORITHMS.md §9.3.
func laneRects(d *core.Document, from, to core.Endpoint) []geom.Rect {
	out := make([]geom.Rect, 0, len(d.Blocks)+8)
	for _, b := range d.Blocks {
		out = append(out, b.Rect())
	}
	for _, b := range d.Blocks {
		for i := 0; i < len(core.PinLayout(b.Pins)); i++ {
			ep := core.Endpoint{BlockID: b.ID, Pin: i}
			if ep == from || ep == to {
				continue
			}
			p, _, ok := b.PinPos(i)
			if !ok {
				continue
			}
			out = append(out, geom.Rect{
				Min: geom.Vec{X: p.X - pinGap, Y: p.Y - pinGap},
				Max: geom.Vec{X: p.X + pinGap, Y: p.Y + pinGap},
			})
		}
	}
	return out
}

// linkObs — препятствия в виде «чужие блоки / блоки концов» для RouteAvoid.
// Собственные блоки идут без зоны вокруг пина (иначе «флажок» у подхода).
// См. docs/ALGORITHMS.md §9.3.
func linkObs(d *core.Document, from, to core.Endpoint) (obstacles, self []geom.Rect) {
	for _, b := range d.Blocks {
		r := b.Rect()
		if b.ID == from.BlockID || b.ID == to.BlockID {
			self = append(self, r)
		} else {
			obstacles = append(obstacles, r)
		}
	}
	return obstacles, self
}

// rubberRoute — предпросмотр резинки: выход вдоль нормали пина, «вперёд» — одно
// колено, «назад» — канал за торцом блока, чтобы не перерезать блок.
// См. docs/ALGORITHMS.md §9.4.
func rubberRoute(d *core.Document, ep core.Endpoint, c geom.Vec, out float32) []geom.Vec {
	p, s, ok := d.PinPos(ep)
	if !ok {
		return []geom.Vec{c}
	}
	b := d.BlockByID(ep.BlockID)
	if b == nil {
		return []geom.Vec{p, c}
	}
	r := b.Rect()
	n := s.Normal()
	e1 := p.Add(n.Scale(out)) // точка выхода за габарит
	isH := s == geom.SideLeft || s == geom.SideRight

	if isH {
		// горизонтальный выход: сначала вдоль нормали, потом через X курсора
		if (c.X-e1.X)*n.X >= 0 { // курсор «вперёд» — обычное колено
			return geom.Clean([]geom.Vec{p, e1, {X: c.X, Y: e1.Y}, c})
		}
		// назад: вертикаль по внешней стороне грани, потом к курсору;
		// подрезаем блок, если курсор в его Y-полосе
		if c.Y <= r.Min.Y || c.Y >= r.Max.Y {
			return geom.Clean([]geom.Vec{p, e1, {X: e1.X, Y: c.Y}, c})
		}
		lane := r.Max.Y + out // канал за нижним торцом
		if c.Y < (r.Min.Y+r.Max.Y)/2 {
			lane = r.Min.Y - out // курсор ближе к верхнему краю
		}
		return geom.Clean([]geom.Vec{p, e1, {X: e1.X, Y: lane}, {X: c.X, Y: lane}, c})
	}

	// вертикальный выход: сначала вдоль нормали, потом через Y курсора
	lo, hi := minf(e1.Y, c.Y), maxf(e1.Y, c.Y)
	blocked := lo < r.Max.Y && hi > r.Min.Y // вертикаль на e1.X режет блок?
	if (c.Y-e1.Y)*n.Y >= 0 || !blocked {
		return geom.Clean([]geom.Vec{p, e1, {X: e1.X, Y: c.Y}, c})
	}
	lane := r.Max.X + out // канал за правым торцом
	if c.X < (r.Min.X+r.Max.X)/2 {
		lane = r.Min.X - out
	}
	return geom.Clean([]geom.Vec{p, e1, {X: lane, Y: e1.Y}, {X: lane, Y: c.Y}, c})
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
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

		t := op.Offset(image.Pt(int(p.X)+int(3*ed.cam.Zoom), int(p.Y)+int(2*ed.cam.Zoom))).Push(gtx.Ops)
		sz := 3 * ed.cam.Zoom // ~3 мм шрифт
		if sz < 8 {
			sz = 8
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
	info := fmt.Sprintf("зум %d%% | сетка %v мм | %s%s | undo:%d redo:%d | sel:%d | блоки %d | линки %d | cluster %d | своб.пины %d",
		int(ed.cam.Zoom/pxPerMM*100),
		ed.grid, ed.file, dirtyMark, undoN, redoN, len(ed.sel),
		len(ed.doc.Blocks), len(ed.doc.Links), groups, free)
	if ed.mode == modeLink {
		info += "   |   тянем от пина — отпусти на целевом пине"
	}
	drawHUDLine(gtx, ed.th, info, 0, colText)
	if ed.msg != "" {
		drawHUDLine(gtx, ed.th, ed.msg, 16, colJct)
	}
	h := float32(gtx.Constraints.Max.Y)
	hint := "колесо: зум к курсору (5%)   0/=/-: зум 100% / + / –   Home: вся схема   ПКМ/СКМ: панорама   ←↑↓→: листать   " +
		"палитра слева или 1..6: блок с N пинами   G: сетка 5/10/20   Del: удалить   Ctrl+C/V: буфер   " +
		"рамка/Shift+клик: выделить   A/R/T/B H/V D: выровнять   L: автолайаут   Ctrl+Z/Y: undo/redo   Ctrl+S/O: файл"
	drawHUDLine(gtx, ed.th, hint, h-18, colText)
}

// drawHUDLine — одна строка HUD со смещением от верхнего левого угла (и цветом).
func drawHUDLine(gtx layout.Context, th *material.Theme, txt string, dy float32, c color.NRGBA) {
	// сдвиг вправо: левая полоса занята палитрой
	t := op.Offset(image.Pt(int(paletteW), int(dy))).Push(gtx.Ops)
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
			// Отдельная input-area для клавиатуры: без event.Op для keyTag
			// у его handler'а не будет visible, и keyQueue.Frame() снимет
			// фокус в конце кадра — клавиши «не реагируют».
			event.Op(gtx.Ops, keyTag)
			e.Source.Execute(key.FocusCmd{Tag: keyTag}) // канва держит фокус клавиатуры
			handleInput(e.Source, ed)
			handleKeys(e.Source, ed)
			drawFrame(gtx, ed)
			e.Frame(ops)
		case app.DestroyEvent:
			// gio.app.Main() в v0.10 не умеет закрывать пустой пул окон:
			// после выхода последнего окна словим "all goroutines asleep".
			// Выходим сами, спокойно.
			os.Exit(0)
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
