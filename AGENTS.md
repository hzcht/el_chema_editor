# AGENTS.md

## What this repo is

Desktop circuit editor + AC/DC simulator in Go (rewrite of the author's old
Pascal/Delphi school project). **Stage 1 (pure graphical editor)** is implemented
in `cmd/shbuilder` on top of `internal/{core,geom}`; no physics/simulation yet.

## Key files

- `ideas.md` — the spec: task (§1), architecture (§3), **routing rules digest
  (§4.6)**, roadmap/status (§8, §13.3), open questions (§10). Read before designing.
- `problems.md` — play-test journal. One problem = **Проблема / Что найдено /
  Что сделано / Что предлагается**, status `открыта` → `исправлена, ждёт
  проверки` → `закрыта`. Append, don't rewrite history.
- `DECISIONS.md` — ADRs: Gio (0001), two stages (0003), snapshots (0004),
  JSON v1 (0005), mm/grid (0006), routing (0007).

## Ground rules

- **Stage 1 only:** blocks, connectors, live redraw, crossings/junctions,
  align/distribute, undo/redo, save/load. **No physics, no netlist**; do not
  scaffold `simulate`.
- **Gio v0.10.3 is the chosen UI** (ADR-0001). `CGO_ENABLED=0`, no C compiler.
  Fyne is rejected (needs cgo+MinGW); the Wails spike is a fallback only.
- `model`/`topology`/`simulate` must stay pure Go with zero GUI deps (CLI reuse,
  unit tests). Simulation MVP is **linear-only** (R/L/C + sources, MNA);
  nonlinear (diodes/Newton) is out of scope.
- Main module stays GUI-free in `internal/*`; `spike/*` are separate modules.

## Commands

```powershell
# Main module: verify right after any change
go build ./... ; go test ./... ; go vet ./...

# Run the editor (optional arg = JSON file, default schematic.json)
go build -o shbuilder.exe ./cmd/shbuilder ; .\shbuilder.exe
.\shbuilder.exe path\to\scheme.json

# Spike Gio — builds WITHOUT cgo
cd spike/gio   ; go build -o spike-gio.exe .   ; .\spike-gio.exe

# Spike Wails — build tags MANDATORY; without them it exits 0 with no window
cd spike/wails ; go build -tags desktop,production -o spike-wails.exe . ; .\spike-wails.exe
```

## Gotchas (do not rediscover)

- **`go build ./...` writes `shbuilder.exe` to repo root** (main package). For a
  fresh smoke-run build explicitly: `go build -o shbuilder.exe ./cmd/shbuilder`.
- **Editor UI (stage 1):** `1..6` add a block with N pins; the left **palette**
  (132 px, 6 rows with mini-previews) does the same on click — it inserts at the
  center of the visible canvas and selects the block; hotkeys over the palette
  clamp there too. `Del` deletes selection (or block under cursor), `G` cycles
  grid 5/10/20 mm, drag-from-pin creates a link (rubber exits along the pin
  normal, backward pulls bypass around the block end), `Ctrl+Z/Y` undo/redo,
  `Ctrl+C/V` clipboard, `Ctrl+S/O` save/load. Undo granularity: Capture-on-press
  / Finish-on-release (idle clicks don't pollute history — `core.History`).
  Palette input is consumed before canvas logic; `fitToWindow`/HUD account for
  `paletteW`.
- **Wire routing:** `geom.RouteEx` builds detour *candidates*, picks
  "no hairpin → shorter"; contract is `geom.Hairpins(route)==0` (§4.6). Obstacles
  carry `Gap` (`geom.Obstacle`): blocks — `outset` 5 mm, own blocks — 0 (only
  "don't cut"), foreign pins — `pinGap` 2 mm (point rect). `RouteOpts.Prev` +
  `Sticky` (3 mm) hold the detour side. `geom.RouteAvoid` is the blocks-only
  compatibility wrapper. `drawLinks`: route → sort by `compareLinks`
  (deterministic, NOT creation order) → `geom.OffsetLanes` (collinear overlaps
  `laneGap` 2 mm; endpoints/pin junctions untouched) → hop arcs (`arcR` 1.6 mm).
  **Do not** bring back "always return to the original line at b" — that is
  exactly the old «крюк» (down then up at the same x). Regressions:
  `TestRouteAvoidNoHooks` (~142k configs), `TestDemoLinksNoHooks`,
  `TestRouteAvoidForeignPin`, `TestDetourSideSticky`, `TestOffsetLanes*`,
  `TestEightLinksLanes`.
- **Gio keyboard focus is triple-deferred — drop any one and keys die silently:**
  every frame needs (1) `event.Op(gtx.Ops, keyTag)` (handler `visible`),
  (2) `key.FocusFilter{Target: keyTag}` inside `q.Event(...)` (`focusable`),
  (3) `key.FocusCmd{Tag: keyTag}`. Without (1)+(2) `keyQueue.Frame` clears focus
  at the END of every frame. `key.Filter{}` must list every modifier in
  `Optional:` (Shift/Ctrl/Alt/Command/Super), else modified keys fail
  `keyFilterMatch`. Headless regression: `TestKeyDeliveryHeadless`.
- **Block overlap invariant (stage 1):** blocks must never overlap. After any
  mutation that can move blocks (drop, insert/paste, align/distribute, load) the
  editor runs `core.PushOverlaps` and checks `core.HasOverlaps`. `arrangeKey`
  always pushes after align/distribute; `PushOverlaps` falls back to placing the
  movable block in the nearest free grid cell, so it resolves or reports
  `false` only when both blocks are protected.

- **World unit = 1 mm** (ADR-0006): zoom 100% = 4 px/mm, default block 30×20 mm,
  all coords snap to the current grid step. JSON v1 stores plain unitless
  floats — old files just show bigger-than-grid coords (resnap by hand).
- **Wails** needs `-tags desktop,production` (or `dev`); plain `go build` exits 0
  silently. **Fyne** needs cgo+MinGW (impossible here) — do not add a cgo GUI
  without resolving the toolchain. Stale v1 import: use `fyne.io/fyne/v2` if ever.
