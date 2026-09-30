# Development Guide

Day-to-day commands for working on yedit itself: the `Makefile` targets, and the full workflow for recording demo GIFs with [VHS](https://github.com/charmbracelet/vhs).

---

## Common commands

All tools are invoked via `go run` in the `Makefile` - no global install required beyond `go` itself.

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

## Recording the demo GIF

The README embeds `docs/demo.gif`, recorded with VHS from [`demo.tape`](../../demo.tape) and driven by [`cmd/demo`](../../cmd/demo/main.go), a minimal editor whose doc comment describes its schema.

The tape is written for Windows and runs in PowerShell. With `vhs`, `ttyd` and `ffmpeg` on the `PATH`, from the repo root:

```sh
vhs demo.tape
```

It builds `demo.exe` off camera, records, and deletes the binary and the seeded `demo.yaml` when done; both are gitignored in case a recording is interrupted. It also clears `NO_COLOR` before building, since a value inherited from the parent shell would record in monochrome.

### Tape conventions

- Guard every screen change with `Wait+Screen /regex/`, never a blind `Sleep`: the TUI is interactive and stateful, and a keystroke sent before the previous screen finished rendering desyncs the whole recording. Sleeps are only for pacing within a screen already confirmed.
- `Set Framerate 20` keeps the GIF small for a tour this long; the default 30 works too, at a larger file.
- The demo seeds `demo.yaml` on first run; the tape deletes it before launching and after quitting, so every recording starts from the same file.
