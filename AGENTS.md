# AGENTS.md

## What this repo is

Greenfield project: a desktop circuit editor + AC/DC simulator in Go (spiritual
rewrite of the author's old Pascal/Delphi school project). **No code exists yet.**

## Key file

- `ideas.md` — the brainstorm/spec. Read it before designing anything: it contains
  the task statement (§1), UI framework candidates and the comparison table (§2, §11),
  proposed architecture (§3), topology/simulation approach (§4–5), roadmap (§8),
  risks (§9), and **open questions (§10) that are still unresolved**.

## Ground rules

- **Two-stage plan (§1 of `ideas.md`, decided):** stage 1 is a pure graphical
  editor — blocks, connectors, live connector redraw, crossing/junction handling,
  ordering (align/distribute), undo/redo, save/load. **No physics, no simulation,
  no netlist at stage 1.** Physical binding and math models come at stage 2+.
- Do not assume a UI framework or project layout has been fully chosen. Current state
  (§10 of `ideas.md`, DECISIONS.md): **Fyne rejected** on this machine (no cgo toolchain);
  final call between **Gio and Wails** pending the user's play-test of `spike/gio` and
  `spike/wails`. Do not commit the main module to a GUI framework before that.
- Planned layering (§3 of `ideas.md`): `model`/`topology`/`simulate` must stay pure Go
  with zero GUI dependencies so they can be unit-tested and reused in a CLI.
  The `simulate` package is stage-2+; do not scaffold it during stage 1.
- Simulation MVP is **linear-only** (R/L/C + sources, MNA solver); nonlinear elements
  (diodes, Newton–Raphson) are explicitly out of scope until a later phase.

## Commands

Toolchain: Go 1.27, git; **CGO_ENABLED=0 and no C compiler** on this machine.

```powershell
# Main module (no GUI deps yet): verify right after any change
go build ./... ; go test ./...

# Spike Gio — builds WITHOUT cgo
cd spike/gio   ; go build -o spike-gio.exe .   ; .\spike-gio.exe

# Spike Wails — build tags are MANDATORY; without them it silently exits 0
cd spike/wails ; go build -tags desktop,production -o spike-wails.exe . ; .\spike-wails.exe
```

Gotchas (cost debugging time once — do not rediscover):
- **Wails needs `-tags desktop,production`** (or `dev`) — plain `go build` yields exit 0
  with no window and no error.
- **Fyne needs cgo+MinGW** — impossible on this machine; spike code kept but not buildable.
  Do not add Fyne (or any cgo GUI) as a dependency without resolving the toolchain.
- Swap-dep note: `fyne.io/fyne` (v1 path) is stale; if ever used, import `fyne.io/fyne/v2`.
- `spike/*` are separate Go modules (own go.mod) so the main module stays GUI-free;
  `go build ./...` at repo root must never pull in GUI deps.
