# Backlog

Improvements identified in the 2026-06-10 project review that were deliberately
deferred. Each entry records enough context to pick the work up later without
re-deriving the analysis.

## To discuss

### Direct YAML edits (`ReplaceRaw`) are not undoable

`document.ReplaceRaw` intentionally does not push an undo snapshot — only
committed block operations (`Insert`, `Replace`, `Remove`) enter the history.
This is documented behavior, but from the user's point of view it is
surprising: after editing in the YAML pane and committing, `Ctrl+U` does not
restore what they typed over.

Points for the discussion:

- The block editor has its own undo stack (`blockEditState.undoStack`), so
  in-editor mistakes are recoverable while the editor is open; the gap is only
  after the content reaches the document via `ReplaceRaw`.
- Snapshotting on every `ReplaceRaw` call would flood the 50-entry history
  with keystroke-level states unless calls are debounced or coalesced.
- A middle ground: snapshot once per "editing session" (first `ReplaceRaw`
  after any other operation), so undo returns to the state before the direct
  edit began.
- Document-level redo (`Ctrl+Y`, added 2026-06-10) follows whatever rule undo
  follows; no extra design needed there.

## CI / tooling

### Test on Windows (and macOS) in CI

CI runs only on `ubuntu-latest`, but the codebase has explicitly
platform-sensitive logic: CRLF normalization and restoration, UTF-8 BOM
stripping, and atomic rename semantics in `document/document.go`. Development
happens on Windows. Add an OS matrix (`ubuntu-latest`, `windows-latest`,
optionally `macos-latest`) to `.github/workflows/ci.yml`. Highest-value item
in this list.

### Release workflow parity with CI

`.github/workflows/release.yml` runs `go test ./...` without `-race` and skips
lint/security, so a tag is gated more weakly than a PR. Either reuse the
Makefile targets (`make test`) or require the CI workflow to have passed on
the tagged commit.

### Add govulncheck

CI runs gosec (static analysis of our code) but nothing scans dependencies for
known CVEs. Add a `vulncheck` Makefile target running
`golang.org/x/vuln/cmd/govulncheck@latest ./...` and a CI step. The
charmbracelet dependency tree is large enough to make this worthwhile.

### Automated dependency updates

No Dependabot or Renovate configuration exists. A minimal
`.github/dependabot.yml` covering `gomod` and `github-actions` ecosystems
keeps both module versions and pinned actions fresh with near-zero
maintenance.

### Tighten lint thresholds

`.golangci.yml` currently sets `lll: line-length: 300` (effectively disabled)
and `gocognit: min-complexity: 35` (very permissive). Tighten gradually — e.g.
`lll: 160` and `gocognit: 25` — fixing or `//nolint`-annotating the violations
each step surfaces.

### Coverage gate

CI generates coverage reports but nothing fails when coverage drops. Options:
a simple threshold check in the workflow (parse `go tool cover -func` total),
or upload to Codecov and enforce via its status check (also yields a README
badge).
