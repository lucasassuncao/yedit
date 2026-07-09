# Development Guide

Day-to-day commands for working on yedit itself: the `Makefile` targets, and the full workflow for recording demo GIFs with [VHS](https://github.com/charmbracelet/vhs).

---

## Common commands

All tools are invoked via `go run` in the `Makefile` - no global install required beyond `go` itself and, for a couple of targets, Docker.

```sh
make fmt            # go fmt ./...
make lint           # golangci-lint
make test           # go test -race ./... (testdox output via gotestsum)
make test-watch     # test, rerun on file changes
make test-coverage  # test + HTML/Cobertura coverage reports
make security       # gosec static analysis
make deps           # go mod download + go mod tidy
make docs           # regenerate the gomarkdoc-embedded section of package READMEs
make all            # fmt + docs + lint + security + test-coverage
make clean          # remove coverage artifacts and Go build/test cache
```

`make tag VERSION=v1.2.3` cuts and pushes an annotated release tag; it refuses to run on a dirty working tree.

## Recording demo GIFs

yedit's `README.md` and the [`examples/`](../../examples/README.md) folder embed GIFs recorded with VHS, driven by `examples/test` (a minimal, purpose-built editor - see its doc comment for the schema).

**Native `vhs` is known to hang on some Windows setups** (stalls at browser/ttyd startup even with a working ttyd/ffmpeg/Chrome). Use the official Docker image instead; the container has no Go toolchain, so the example binary is cross-compiled for Linux first.

### 1. Build the demo binary (once per recording session)

From the repo root:

```powershell
$env:GOOS = "linux"; $env:CGO_ENABLED = "0"
go build -o demo-app ./examples/test
Remove-Item env:GOOS, env:CGO_ENABLED
```

```bash
CGO_ENABLED=0 GOOS=linux go build -o demo-app ./examples/test
```

`demo-app` is gitignored - rebuild it whenever `examples/test`'s schema changes, not on every recording.

### 2. Record the main demo (`docs/demo.gif`)

From the repo root, where [`demo.tape`](../../demo.tape) lives:

```powershell
docker run --rm -v "${PWD}:/vhs" ghcr.io/charmbracelet/vhs demo.tape
Remove-Item demo-app
```

```bash
docker run --rm -v "$PWD:/vhs" ghcr.io/charmbracelet/vhs demo.tape
rm demo-app
```

### 3. Record a feature demo under `examples/`

Each numbered folder (`examples/01-basic-edit/`, ...) has its own `demo.tape`, referencing `../../demo-app` relative to itself - so the container still mounts the repo root, just with a different working directory:

```powershell
docker run --rm `
  -v "${PWD}:/repo" -w /repo/examples/01-basic-edit `
  ghcr.io/charmbracelet/vhs demo.tape
Remove-Item demo-app
```

```bash
docker run --rm \
  -v "$PWD:/repo" -w /repo/examples/01-basic-edit \
  ghcr.io/charmbracelet/vhs demo.tape
rm demo-app
```

Swap `01-basic-edit` for the target folder (`02-nested-drill-in`, `03-list-navigator`, `04-presets`, `05-validation-errors`, `06-hints-panel`). Each run writes `demo.gif` into its own folder.

### Devcontainer / native Linux

If `vhs` is installed natively (e.g. inside a Linux devcontainer), skip Docker entirely and run it directly from the relevant folder:

```bash
cd examples/01-basic-edit   # or the repo root, for demo.tape
vhs demo.tape
```

### Tape conventions

See [`examples/README.md`](../../examples/README.md#conventions) for the rules every `.tape` file follows: throwaway `session.yaml` working copies (fixtures are never mutated by recording), and `Wait+Screen /regex/` guards on every screen transition instead of blind `Sleep` - the TUI is interactive and stateful, so a keystroke sent before the previous screen finished rendering desyncs the whole recording.
