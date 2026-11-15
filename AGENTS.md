# AGENTS.md

## What this repo is
Go circuit editor Stage 1. `cmd/shbuilder` + `internal/{core,geom}`. No physics/simulation. `CGO_ENABLED=0`.

## Key files
- `ideas.md` — spec: task (§1), architecture (§3), routing rules (§4.6), roadmap (§8, §13.3).
- `problems.md` — journal: Problem → Fixed → Closed.
- `DECISIONS.md` — ADRs: Gio (0001), two stages, snapshots, JSON, mm/grid, routing.
- `docs/ALGORITHMS.md` — how the algorithms work: routing (§1), lanes (§2), taps (§3), overlaps (§4), history (§5), arrange (§6), topology (§7), file format (§8), canvas (§9), invariants (§10). Code comments point here by § number.

## Ground rules (brief)
- **Stage 1 only:** blocks, connectors, redraw, align/distribute, undo/redo, save/load. No physics, no netlist.
- **UI:** Gio v0.10.3. Fyne rejected (cgo+MinGW); Wails fallback only.
- **Purity:** `model`/`topology`/`simulate` = pure Go, zero GUI deps. CLI reuse, unit tests. MVP: linear (R/L/C + sources, MNA). Nonlinear out of scope.
- **Modules:** Main GUI-free in `internal/*`; `spike/*` separate.

## Commands
```powershell
go build ./... ; go test ./... ; go vet ./...

go build -o shbuilder.exe ./cmd/shbuilder ; .\shbuilder.exe [json]

cd spike/gio ; go build -o spike-gio.exe . ; .\spike-gio.exe

cd spike/wails ; go build -tags desktop,production -o spike-wails.exe . ; .\spike-wails.exe
```

## Gotchas (critical only)
- `go build ./...` writes to repo root. Use `go build -o ./cmd/shbuilder` for fresh run.
- **UI:** Palette (132 px) inserts at canvas center. `Del` deletes, `G` cycles grid 5/10/20 mm. Undo: capture-on-press/finish-on-release.
- **Routing:** `geom.RouteEx` → "no hairpin → shorter". Contract: `geom.Hairpins(route)==0`. Never return to original line at b (the "крюк" regression).
- **Keyboard focus:** Triple-deferred: (1) `event.Op(gtx.Ops, keyTag)`, (2) `key.FocusFilter{Target: keyTag}`, (3) `key.FocusCmd{Tag: keyTag}`. Drop any one and keys die silently. `key.Filter{}` must list all modifiers in `Optional:`.
- **Overlap invariant:** Blocks must never overlap. After mov mutations: `core.PushOverlaps` + `core.HasOverlaps`. Fallback: nearest free grid cell.
- **Units:** World = 1 mm. Zoom 100% = 4 px/mm. Default block 30×20 mm. Coords snap to grid. JSON v1 = unitless floats (resnap by hand).
- **Wails:** Needs `-tags desktop,production`. Plain `go build` exits 0 silently. **Fyne:** Needs cgo+MinGW (impossible here). Use `fyne.io/fyne/v2` if ever.