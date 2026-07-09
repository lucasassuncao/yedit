# Examples

`examples/test/` is a small, self-contained editor (`go run ./examples/test`) used both for manual testing and as the binary every demo below drives. Its schema is intentionally tiny - see the doc comment at the top of `examples/test/main.go` for the full shape.

The numbered subfolders (`01-basic-edit/`, `02-nested-drill-in/`, ...) are self-contained feature demos, each recorded as a GIF with [VHS](https://github.com/charmbracelet/vhs). Every one of them follows the same shape:

```
NN-example-name/
├── config.yaml   # fixture for this demo, tracked and never mutated by recording
└── demo.tape     # VHS script: launches the editor, drives it, shows the result
```

## Conventions

- **Each `demo.tape` is self-contained.** It copies its folder's `config.yaml` to a throwaway `session.yaml` at the start (inside a `Hide` ... `Show` block, so the recording only shows the interesting part), runs the editor against that copy, and deletes it at the end. The tracked `config.yaml` is never written to - re-running a tape from a clean checkout always reproduces the same demo. `session.yaml` is gitignored.
- **Every `demo.tape` uses `Wait+Screen /regex/` on every screen transition**, never a blind `Sleep` to wait for a redraw. The TUI is interactive and stateful - a keystroke sent before the previous screen finished rendering desyncs the whole recording. Sleeps are only for pacing *within* an already-confirmed screen.
- **Every tape references the demo binary built once at the repo root** (`../../demo-app`, relative to the example folder) rather than building it itself - see [Recording](#recording).

## Recording

The `vhs` container has no Go toolchain, so cross-compile the example once from the repo root before recording anything:

```bash
CGO_ENABLED=0 GOOS=linux go build -o demo-app ./examples/test
```

```powershell
$env:GOOS = "linux"; $env:CGO_ENABLED = "0"
go build -o demo-app ./examples/test
Remove-Item env:GOOS, env:CGO_ENABLED
```

Native `vhs` is known to hang on some Windows setups (stalls at browser/ttyd startup) - use the Docker image instead.

### Devcontainer / Linux (native `vhs` installed)

```bash
cd examples/01-basic-edit
vhs demo.tape
```

### Windows (or anywhere without a native `vhs` setup), via Docker

From an example folder:

```powershell
docker run --rm `
  -v "${PWD}/../..:/repo" -w /repo/examples/01-basic-edit `
  ghcr.io/charmbracelet/vhs demo.tape
```

(bash/Git Bash equivalent: replace the backtick line continuations with `\`.)

Each run produces `demo.gif` in its own folder. `demo-app` at the repo root is gitignored - rebuild it once per recording session, not per tape.

## What each example demonstrates

| Folder | Demonstrates |
| --- | --- |
| `01-basic-edit` | Editing a `KindPrimitive` field - no tree, straight to the YAML pane |
| `02-nested-drill-in` | Drilling into a struct nested inside another struct (`server.pool`), then popping back out with edits preserved |
| `03-list-navigator` | The `[N]` entry navigator for a `KindList` with child defs (`workers`), including adding a new entry |
| `04-presets` | The preset picker (`p`) replacing a block's content with a ready-made snippet |
| `05-validation-errors` | `Ctrl+L` reporting `RequiredFromMetadata` violations on a fixture with missing required fields |
| `06-hints-panel` | `h` showing per-field description/type/default/example, both in the root list and inside a block editor |

The root-level [`demo.tape`](../demo.tape) (recorded to `docs/demo.gif`, embedded in the main `README.md`) is a longer, combined walkthrough - hints, drilling into a block, applying a preset, saving, and validating in one session.
