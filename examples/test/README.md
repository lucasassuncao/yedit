# yedit example — `test`

A self-contained demo that exercises every schema pattern, `Config` option, and
known edge case. See the package doc comment at the top of [`main.go`](main.go)
for the full catalog of patterns. This file documents the `seedYAML` constant.

## What `seedYAML` is for

`seedYAML` is the **sample YAML document the example writes on first run**. It is
not part of the editor itself — it only gives the demo something to open.

In `main()`:

```go
const path = "test.yaml"
if _, err := os.Stat(path); os.IsNotExist(err) {
    if err := os.WriteFile(path, []byte(seedYAML), 0600); err != nil {
        panic(err)
    }
}
```

So:

- **First run** — `test.yaml` does not exist, so the seed is written to disk and
  the editor opens it.
- **Every later run** — `test.yaml` already exists, so the seed is ignored and
  the editor opens whatever is on disk. This is deliberate: it lets undo, save,
  and validate be exercised *across restarts*, since your edits survive.

The seed's value is that it's a single fixture rich enough to touch every editor
feature at once. Each block is chosen to drive a specific code path:

| Seed key       | Schema kind / behavior                                    |
| -------------- | --------------------------------------------------------- |
| `import`       | `PassthroughKeys` — preserved as-is, hidden from all sections |
| `app-name`     | `KindPrimitive` (string)                                  |
| `debug`        | `KindPrimitive` (bool)                                    |
| `version`      | `KindPrimitive` (string, required, `default=0.1.0`)       |
| `port`         | `KindPrimitive` (int, `default=8080`)                     |
| `ratio`        | `KindPrimitive` (float64)                                 |
| `build-timeout`| `KindPrimitive` (duration, e.g. `30s`)                    |
| `tags`         | `KindList` of strings — no child defs                     |
| `settings`     | `KindDictionary` (`map[string]any`) — free-form, no child defs |
| `server`       | `KindObject` struct with `[]string` and `map[string]string` leaves |
| `logging`      | `KindObject` struct with a `KindEnum` (`level`) and a bool |
| `workers`      | `KindList` with child defs; `extensions: ["go","yaml"]` exercises flow-style leaves |
| `port-attrs`   | `KindDictionary` with child defs (`map[key]struct` navigator) |
| `filters`      | `KindList` with child defs; `Filter` is self-referential (cycle-detected) |
| `edge-cases`   | `KindObject` exercising anonymous embed, `yaml:",inline"`, `omitempty`, `flow`, `map[int]Struct`, `yaml.Marshaler` → `KindPrimitive`, `interface{}` → `KindAny` |
| `unknown-key`  | Not in the schema → shows in the UNKNOWN section and is flagged by `ctrl+l` |

Schema fields that are **not** in the seed (`labels`, `ports`, `timeout`,
`database`, `deploy`, `routes`) start empty and appear as *available* blocks in
the editor's tree — that path is exercised too, just from the other direction.

## Resetting to the seed

Because the file is reused once it exists, the checked-in `test.yaml` drifts
from the seed as you edit it. To start fresh from the full seed:

```sh
rm examples/test/test.yaml
go run ./examples/test
```
