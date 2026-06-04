# Schema Kinds Reference

How Go types map to yedit editor behavior, with complete code examples.

---

## Quick Reference

| Go type | Kind | Editor behavior |
|---|---|---|
| `string`, `int`, `bool`, `float64`, `time.Duration` | `KindPrimitive` | YAML pane only (no tree) |
| `type X string` + `validate:"oneof=a b"` | `KindEnum` | YAML pane + values list in hint |
| `SomeStruct` | `KindObject` | Tree with ADDED/AVAILABLE fields |
| `[]string`, `[]int` (scalar slice) | `KindList` (no child defs) | YAML pane only |
| `[]SomeStruct` | `KindList` with child defs | `[N]` navigator + field tree per entry |
| `[]map[string]T` | `KindList` (no child defs) | YAML pane only |
| `map[string]string`, `map[string]any` | `KindDictionary` (no child defs) | YAML pane only |
| `map[string]SomeStruct` | `KindDictionary` with child defs | `[N]` navigator keyed by map key |
| Implements `schema.Provider` | `KindVariant` | Delegates to `YeditSchema()` return |

---

## KindPrimitive — scalar value

**Go:**
```go
type Config struct {
    AppName      string        `yaml:"app-name"`
    Port         int           `yaml:"port"          jsonschema:"default=8080"`
    Debug        bool          `yaml:"debug"`
    Ratio        float64       `yaml:"ratio"         jsonschema:"default=1.0"`
    BuildTimeout time.Duration `yaml:"build-timeout"`
    Version      string        `yaml:"version"       validate:"required" jsonschema:"default=0.1.0"`
}
```

**YAML:**
```yaml
app-name: "my-app"
port: 8080
debug: false
ratio: 1.5
build-timeout: 30s
version: "0.1.0"
```

**Notes:**
- `jsonschema:"default=X"` — value shown in the hint panel; does **not** auto-fill the field
- `validate:"required"` — field marked with `*` in the hint; ctrl+l reports it missing
- `time.Duration` is stored as a plain string (`"30s"`, `"2m30s"`) — YAML does not have a duration type
- Pointer variants (`*string`, `*int`, `*bool`) behave identically; nil = field absent from the file

---

## KindEnum — one of a fixed set of values

**Go:**
```go
// Option A: plain string with validate tag (simplest)
type ServerConfig struct {
    LogLevel string `yaml:"log-level" validate:"oneof=debug info warn error" jsonschema:"default=info"`
    Protocol string `yaml:"protocol"  validate:"oneof=http https"`
}

// Option B: named string type (documents intent, same behavior in yedit)
type ConflictStrategy string

const (
    ConflictStrategyRename    ConflictStrategy = "rename"
    ConflictStrategyOverwrite ConflictStrategy = "overwrite"
    ConflictStrategySkip      ConflictStrategy = "skip"
)

type Category struct {
    ConflictStrategy ConflictStrategy `yaml:"conflict-strategy" validate:"oneof=rename overwrite skip"`
}
```

**YAML:**
```yaml
log-level: info
protocol: https
conflict-strategy: rename
```

**Notes:**
- yedit promotes the field to `KindEnum` when `validate:"oneof=..."` is present
- The allowed values appear in the hint panel under "values"
- Saving an invalid value is blocked by ctrl+l validation

---

## KindObject — struct with known fields

**Go:**
```go
type PoolConfig struct {
    MinSize int `yaml:"min-size" jsonschema:"default=2"`
    MaxSize int `yaml:"max-size" jsonschema:"default=10"`
    Timeout int `yaml:"timeout"  jsonschema:"default=30"`
}

type DatabaseConfig struct {
    Driver   string     `yaml:"driver"    validate:"required,oneof=postgres mysql sqlite"`
    DSN      string     `yaml:"dsn"       validate:"required"`
    MaxConns int        `yaml:"max-conns" jsonschema:"default=10"`
    Pool     PoolConfig `yaml:"pool"`     // nested struct — depth+1
}

type Config struct {
    Database DatabaseConfig `yaml:"database"`
}
```

**YAML:**
```yaml
database:
  driver: postgres          # field of DatabaseConfig
  dsn: "postgres://..."
  max-conns: 10
  pool:                     # nested struct PoolConfig
    min-size: 2
    max-size: 10
    timeout: 30
```

**Editor behavior:**
- Left panel shows all fields in ADDED (present) / AVAILABLE (absent) sections
- Toggling a field OFF removes it from the YAML; toggling ON inserts it (using `FieldSnippets` if configured)
- Nested structs (`pool`) appear as expandable nodes in the tree (→ to expand)

**FieldSnippets** — YAML inserted when a field is toggled ON:
```go
editor.Config{
    FieldSnippets: map[string]map[string]string{
        "database": {
            "driver":    "  driver: postgres\n",
            "dsn":       "  dsn: \"postgres://localhost/mydb\"\n",
            // multi-field snippet — all sub-fields are inserted at once:
            "pool":      "  pool:\n    min-size: 2\n    max-size: 10\n    timeout: 30\n",
        },
    },
}
```

**PreCheckedFields** — fields toggled ON automatically when opening a **new** block:
```go
editor.Config{
    PreCheckedFields: map[string][]string{
        "database": {"driver", "dsn"},
    },
}
```

---

## KindList (no child defs) — scalar list

**Go:**
```go
type Config struct {
    Tags  []string `yaml:"tags"`
    Ports []int    `yaml:"ports"`
}
```

**YAML:**
```yaml
tags:
  - go
  - tui

ports:
  - 80
  - 443
```

**Editor behavior:** YAML pane only. Tree left panel shows "(no fields)". User edits the raw YAML directly.

**Flow style** (also valid, preserved by the editor):
```yaml
extensions: ["go", "yaml", "json"]
```

---

## KindList with child defs — list of structs / `[N]` navigator

**Go:**
```go
type Worker struct {
    Name        string   `yaml:"name"        validate:"required"`
    Concurrency int      `yaml:"concurrency" jsonschema:"default=1"`
    Queue       string   `yaml:"queue"`
    Tags        []string `yaml:"tags"`        // scalar slice inside struct — YAML pane in hint
    Extensions  []string `yaml:"extensions"`  // same
}

type Config struct {
    Workers []Worker `yaml:"workers"`
}
```

**YAML:**
```yaml
workers:
  - name: "default"       # dash = new list item (start of a Worker struct)
    concurrency: 2        # field of Worker (no dash)
    queue: "main"
    tags:
      - critical
  - name: "background"    # next Worker
    concurrency: 1
    queue: "low"
```

**Editor behavior:**
- Left panel shows `[0] default`, `[1] background`, `[+ add new]`
- Selecting an entry loads it into the YAML pane; navigating away auto-saves it
- `→` expands an entry to show its fields as a toggleable tree
- `[+ add new]` appends a new empty entry

**Self-referential structs** — supported up to depth 10:
```go
type Filter struct {
    Regex string   `yaml:"regex"`
    Glob  string   `yaml:"glob"`
    Any   []Filter `yaml:"any"`   // recursive: []Filter contains []Filter
    All   []Filter `yaml:"all"`   // same
}
```

```yaml
filters:
  - glob: "*.go"
  - regex: ".*_test\\.go$"
    any:                         # nested Filter list
      - glob: "internal/*"
      - glob: "pkg/*"
```

---

## KindList (no child defs) — list of free-form maps

**Go:**
```go
type Config struct {
    Annotations []map[string]string `yaml:"annotations"`
}
```

**YAML:**
```yaml
annotations:
  - env: production        # dash = new map item
    region: us-east-1      # additional key in the same map (no dash)
  - env: staging
    region: eu-west-1
```

**Editor behavior:** YAML pane only — no `[N]` navigator because `map[string]string`
has no fixed keys to discover.

**To get the `[N]` navigator**, define a concrete struct:
```go
// Before: free-form, YAML pane only
Annotations []map[string]string `yaml:"annotations"`

// After: [N] navigator with field tree
type Annotation struct {
    Env    string `yaml:"env"    validate:"oneof=production staging development"`
    Region string `yaml:"region"`
}
Annotations []Annotation `yaml:"annotations"`
```

---

## KindDictionary (no child defs) — free-form map

**Go:**
```go
type Config struct {
    Labels   map[string]string `yaml:"labels"`
    Settings map[string]any    `yaml:"settings"`
}
```

**YAML:**
```yaml
labels:                    # no dashes — key: value pairs
  env: production
  team: backend

settings:
  cache: true
  max-upload: 10mb
  nested:
    allowed: true
```

**Editor behavior:** YAML pane only. Tree left panel shows "(no fields)".

**`map[string]any`** accepts any YAML value per key — scalars, lists, or nested maps.

---

## KindDictionary with child defs — map of structs / `[N]` navigator

**Go:**
```go
type PortAttr struct {
    Label         string `yaml:"label"`
    OnAutoForward string `yaml:"on-auto-forward" validate:"oneof=notify openBrowser ignore silent"`
    Protocol      string `yaml:"protocol"        validate:"oneof=http https tcp udp"`
}

type Config struct {
    PortAttrs map[string]PortAttr `yaml:"port-attrs"`
}
```

**YAML:**
```yaml
port-attrs:
  "3000":                       # user-defined key (the map key)
    label: "frontend"           # fields of PortAttr (no dash)
    on-auto-forward: openBrowser
    protocol: http
  "8080":                       # another user-defined key
    label: "api"
    on-auto-forward: notify
    protocol: http
```

**Editor behavior:**
- Left panel shows `[0] 3000`, `[1] 8080`, `[+ add new]`
- Each entry is keyed by the map key (e.g. `"3000"`)
- Fields of `PortAttr` appear in the field tree per entry
- Renaming the key in the YAML pane updates the entry label in the tree

**Important distinction from `[]SomeStruct`:**

```go
// KindList with child defs — ordered, integer-indexed
Workers []Worker `yaml:"workers"`
// YAML: - name: foo    (each item has no name, position is its identity)

// KindDictionary with child defs — unordered, string-keyed
PortAttrs map[string]PortAttr `yaml:"port-attrs"`
// YAML: "3000": ...    (each item's key IS its identity)
```

---

## KindVariant — union type via `schema.Provider`

For fields that can be either a scalar **or** a struct in YAML:

**Go:**
```go
// Implement schema.Provider to bypass reflection and declare the schema manually.
type TimeoutValue struct{}

func (TimeoutValue) YeditSchema() []schema.FieldDef {
    return []schema.FieldDef{
        {YAMLName: "connect", Kind: schema.KindPrimitive, Default: "5s",
            Description: "TCP connection timeout"},
        {YAMLName: "read", Kind: schema.KindPrimitive, Default: "30s",
            Description: "Read timeout per request"},
        {YAMLName: "write", Kind: schema.KindPrimitive, Default: "30s",
            Description: "Write timeout per request"},
    }
}

type Config struct {
    Timeout TimeoutValue `yaml:"timeout"`
}
```

**YAML (scalar form):**
```yaml
timeout: 30s
```

**YAML (struct form):**
```yaml
timeout:
  connect: 5s
  read: 30s
  write: 30s
```

**Notes:**
- yedit renders the schema from `YeditSchema()` — reflection is skipped entirely
- Useful for types that don't have a clean Go representation (union types, custom DSLs)
- `schema.Provider` can also be implemented on a pointer receiver

---

## Struct tags reference

| Tag | Effect in yedit |
|---|---|
| `yaml:"name"` | YAML key used in the file and displayed in the editor |
| `yaml:"-"` | Field excluded from discovery (never shown) |
| `yaml:"name,omitempty"` | Treated same as `yaml:"name"` (omitempty is a marshaling hint, not a schema hint) |
| `validate:"required"` | Marks field as required (`*` in hint); ctrl+l reports it missing |
| `validate:"oneof=a b c"` | Promotes field to `KindEnum`; values listed in hint |
| `jsonschema:"default=X"` | Default value shown in hint panel |
| `jsonschema:"required"` | Alternative to `validate:"required"` |
| `jsonschema_description:"..."` | Description shown in hint panel |

**Full example:**
```go
type Route struct {
    Path    string `yaml:"path"
                    validate:"required"
                    jsonschema_description:"HTTP path pattern, e.g. /api/v1/users"`
    Method  string `yaml:"method"
                    validate:"required,oneof=GET POST PUT DELETE PATCH"
                    jsonschema:"default=GET"`
    Handler string `yaml:"handler"  validate:"required"`
    Auth    bool   `yaml:"auth"     jsonschema:"default=false"`
    Timeout int    `yaml:"timeout"  jsonschema:"default=30"
                    jsonschema_description:"Request timeout in seconds"`
}
```

---

## The child-defs rule

The `[N]` navigator activates **only** when the element/value type is a concrete struct:

```
[]SomeStruct          → KindList + child defs        → [N] navigator ✓
[]string              → KindList + no child defs      → YAML pane    ✗
[]map[string]T        → KindList + no child defs      → YAML pane    ✗

map[string]SomeStruct → KindDictionary + child defs   → [N] navigator ✓
map[string]string     → KindDictionary + no child defs → YAML pane   ✗
map[string]any        → KindDictionary + no child defs → YAML pane   ✗
```

yedit uses `reflect` to discover children. `map` types have no fixed keys at
the type level, so no child defs can be derived — regardless of the value type.

---

## Depth limit

Schema discovery recurses into nested structs up to **depth 10**. This prevents
infinite recursion on self-referential types.

```
Config                depth 0
  └─ DatabaseConfig   depth 1   ← KindObject, fields discovered
       └─ PoolConfig  depth 2   ← KindObject, fields discovered
            └─ ...    depth 3+  ← discovered up to depth 10
```

Fields at depth > 10 are silently omitted from the editor UI but are preserved
in the YAML file (the editor never deletes unknown content it can't render).

---

## Pointer fields

Pointer types are unwrapped transparently during discovery:

```go
type Category struct {
    Enabled *bool   `yaml:"enabled"`  // treated as bool
    Count   *int    `yaml:"count"`    // treated as int
    Note    *string `yaml:"note"`     // treated as string
}
```

**YAML (field present):**
```yaml
enabled: true
```

**YAML (field absent / nil pointer):**
```yaml
# "enabled" key is simply not written
```

In the editor, a nil pointer corresponds to the field being absent — shown as
unchecked (`○`) in the tree. Toggling it ON inserts the field with its default
or snippet value.

---

## Hidden and passthrough fields

Fields that should never appear in the editor:

```go
// Option A: yaml:"-" — excluded from discovery entirely
InternalID string `yaml:"-"`

// Option B: Config.Hidden — discovered but hidden in the UI
// Useful when the field exists in YAML but shouldn't be edited.
editor.Config{
    Hidden: []string{"internal-id", "schema-version"},
}
```

Keys that should be silently preserved without validation:

```go
// Config.PassthroughKeys — hidden from all sections, excluded from ctrl+l validation
editor.Config{
    PassthroughKeys: []string{"import", "$schema"},
}
```

**YAML (both hidden and passthrough are preserved on save):**
```yaml
import: shared.yaml          # passthrough — preserved, not shown in editor
$schema: "./schema.json"     # passthrough — preserved, not shown in editor
internal-id: "abc123"        # hidden — preserved, not shown in editor
name: "my-app"               # normal field — shown and editable
```
