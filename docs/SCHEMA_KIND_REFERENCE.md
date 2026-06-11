# Schema Kinds Reference

How Go types map to yedit editor behavior, with complete code examples.

---

## Quick Reference

| Go type | Kind | Editor behavior |
|---|---|---|
| `string`, `int`, `bool`, `float64`, `time.Duration` | `KindPrimitive` | YAML pane only (no tree) |
| Implements `yaml.Marshaler` or `encoding.TextMarshaler` | `KindPrimitive` | YAML pane only - struct fields are NOT exposed |
| `interface{}` / `any` | `KindAny` | YAML pane only - use `Provider` for a typed schema |
| `SomeStruct` | `KindObject` | Tree with ADDED/AVAILABLE fields |
| `[]string`, `[]int` (scalar slice) | `KindList` (no child defs) | YAML pane only |
| `[]SomeStruct` | `KindList` with child defs | `[N]` navigator + field tree per entry |
| `[]map[string]T` | `KindList` (no child defs) | YAML pane only |
| `map[string]string`, `map[string]any` | `KindDictionary` (no child defs) | YAML pane only |
| `map[string]SomeStruct` | `KindDictionary` with child defs | `[N]` navigator keyed by map key |
| Implements `schema.Provider` | `KindVariant` | Delegates to `YeditSchema()` return |

---

## KindPrimitive - scalar value

**Go:**
```go
type Config struct {
    AppName      string        `yaml:"app-name"`
    Port         int           `yaml:"port"`
    Debug        bool          `yaml:"debug"`
    Ratio        float64       `yaml:"ratio"`
    BuildTimeout time.Duration `yaml:"build-timeout"`
    Version      string        `yaml:"version"`
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
- `time.Duration` is stored as a plain string (`"30s"`, `"2m30s"`) - YAML does not have a duration type
- Pointer variants (`*string`, `*int`, `*bool`) behave identically; nil = field absent from the file
- Field metadata (required, defaults, allowed values, ranges) is declared
  through the `MetadataSource` (`FieldMeta`), not struct tags - see the
  `yedit/metadata` package. Enum-like fields are plain strings whose
  `FieldMeta.OneOf` lists the allowed values, shown in the hint panel and
  enforced by `editor.OneOfFromMetadata()`.

---

## KindObject - struct with known fields

**Go:**
```go
type PoolConfig struct {
    MinSize int `yaml:"min-size"`
    MaxSize int `yaml:"max-size"`
    Timeout int `yaml:"timeout"`
}

type DatabaseConfig struct {
    Driver   string     `yaml:"driver"`
    DSN      string     `yaml:"dsn"`
    MaxConns int        `yaml:"max-conns"`
    Pool     PoolConfig `yaml:"pool"`     // nested struct - depth+1
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

**FieldSnippets** - YAML inserted when a field is toggled ON:
```go
editor.Config{
    FieldSnippets: editor.FieldSnippetMap{
        "database": {
            "driver":    "  driver: postgres\n",
            "dsn":       "  dsn: \"postgres://localhost/mydb\"\n",
            // multi-field snippet - all sub-fields are inserted at once:
            "pool":      "  pool:\n    min-size: 2\n    max-size: 10\n    timeout: 30\n",
        },
    },
}
```

**PreCheckedFields** - fields toggled ON automatically when opening a **new** block:
```go
editor.Config{
    PreCheckedFields: editor.CheckedFieldMap{
        "database": {"driver", "dsn"},
    },
}
```

---

## KindList (no child defs) - scalar list

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

## KindList with child defs - list of structs / `[N]` navigator

**Go:**
```go
type Worker struct {
    Name        string   `yaml:"name"`
    Concurrency int      `yaml:"concurrency"`
    Queue       string   `yaml:"queue"`
    Tags        []string `yaml:"tags"`        // scalar slice inside struct - YAML pane in hint
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

**Self-referential structs** - cycle-detected, configurable depth (default 1 extra recursive level):
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

## KindList (no child defs) - list of free-form maps

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

**Editor behavior:** YAML pane only - no `[N]` navigator because `map[string]string`
has no fixed keys to discover.

**To get the `[N]` navigator**, define a concrete struct:
```go
// Before: free-form, YAML pane only
Annotations []map[string]string `yaml:"annotations"`

// After: [N] navigator with field tree
type Annotation struct {
    Env    string `yaml:"env"`
    Region string `yaml:"region"`
}
Annotations []Annotation `yaml:"annotations"`
```

---

## KindDictionary (no child defs) - free-form map

**Go:**
```go
type Config struct {
    Labels   map[string]string `yaml:"labels"`
    Settings map[string]any    `yaml:"settings"`
}
```

**YAML:**
```yaml
labels:                    # no dashes - key: value pairs
  env: production
  team: backend

settings:
  cache: true
  max-upload: 10mb
  nested:
    allowed: true
```

**Editor behavior:** YAML pane only. Tree left panel shows "(no fields)".

**`map[string]any`** accepts any YAML value per key - scalars, lists, or nested maps.

---

## KindDictionary with child defs - map of structs / `[N]` navigator

**Go:**
```go
type PortAttr struct {
    Label         string `yaml:"label"`
    OnAutoForward string `yaml:"on-auto-forward"`
    Protocol      string `yaml:"protocol"`
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
// KindList with child defs - ordered, integer-indexed
Workers []Worker `yaml:"workers"`
// YAML: - name: foo    (each item has no name, position is its identity)

// KindDictionary with child defs - unordered, string-keyed
PortAttrs map[string]PortAttr `yaml:"port-attrs"`
// YAML: "3000": ...    (each item's key IS its identity)
```

---

## KindVariant - union type via `schema.Provider`

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
- yedit renders the schema from `YeditSchema()` - reflection is skipped entirely
- Useful for types that don't have a clean Go representation (union types, custom DSLs)
- `schema.Provider` can also be implemented on a pointer receiver

---

## Struct tags reference

| Tag | Effect in yedit |
|---|---|
| `yaml:"name"` | YAML key used in the file and displayed in the editor |
| `yaml:"-"` | Field excluded from discovery (never shown) |
| `yaml:"name,omitempty"` | Sets `FieldDef.OmitEmpty = true`; zero value not written to disk |
| `yaml:"name,flow"` | Sets `FieldDef.Flow = true`; serialised inline (e.g. `[a, b, c]`) |

The `yaml` tag is the only tag yedit reads. Field metadata - description,
required, defaults, allowed values, ranges, patterns - is declared through the
`MetadataSource` (`editor.FieldMeta`), typically built with the `yedit/metadata`
package, and enforced by the FromMetadata validator family (see
`docs/validators.md`).

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
the type level, so no child defs can be derived - regardless of the value type.

---

## Depth limit and cycle detection

Schema discovery uses a **visit-count cycle guard**: each type may be visited at
most `1 + recursionLimit` times (default limit = 1). This allows one extra
recursive level for self-referential types (e.g. `any []Filter` is navigable)
while preventing infinite loops. The hard depth ceiling is 20.

```
Config                depth 0
  └─ DatabaseConfig   depth 1   ← KindObject, fields discovered
       └─ PoolConfig  depth 2   ← KindObject, fields discovered
            └─ ...    depth 3+  ← discovered up to depth 20
```

Fields beyond the depth ceiling or the recursion limit are silently omitted from
the editor UI but are preserved in the YAML file (the editor never deletes
unknown content it can't render).

To increase the recursion depth for a deeply self-referential type, pass
`Config.SchemaRecursionDepth`:

```go
editor.Run(editor.Config{
    Schema:               &MyConfig{},
    SchemaRecursionDepth: 3, // three extra recursive levels
})
```

---

## Anonymous embeds and yaml:",inline"

Exported fields promoted by anonymous embedding or `yaml:",inline"` are
discovered as if they were declared directly on the parent struct:

```go
type BaseMeta struct {
    CreatedBy  string `yaml:"created-by"`
    VersionTag string `yaml:"version-tag"`
}

type InlineAnnotations struct {
    Team    string `yaml:"team"`
    Contact string `yaml:"contact"`
}

type Config struct {
    BaseMeta                              // anonymous embed - fields promoted
    InlineAnnotations `yaml:",inline"`   // yaml inline - fields promoted
    Port int          `yaml:"port"`
}
// Discovers: created-by, version-tag, team, contact, port
```

Unexported anonymous embeds are also promoted (their exported fields surface at
the parent level).

---

## Types that serialise as scalars (yaml.Marshaler / encoding.TextMarshaler)

If a struct type implements `yaml.Marshaler` or `encoding.TextMarshaler`, yedit
classifies it as `KindPrimitive` and **does not** expose its internal struct
fields in the editor. The user edits the serialised form (e.g. `"#1e1e2e"` for
a color type, `"192.168.1.1"` for an IP type):

```go
type Color struct{ R, G, B uint8 }

func (c Color) MarshalYAML() (any, error) {
    return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B), nil
}

type Config struct {
    Background Color `yaml:"background"` // KindPrimitive - no R/G/B sub-fields
}
```

---

## interface{} / any → KindAny

Fields typed `interface{}` or `any` are classified as `KindAny`. The editor
shows the YAML pane with no tree. To provide a typed schema for a union field,
implement `schema.Provider` on a concrete wrapper type.

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

In the editor, a nil pointer corresponds to the field being absent - shown as
unchecked (`○`) in the tree. Toggling it ON inserts the field with its default
or snippet value.

---

## Hidden and passthrough fields

Fields that should never appear in the editor:

```go
// Option A: yaml:"-" - excluded from discovery entirely
InternalID string `yaml:"-"`

// Option B: Config.Hidden - discovered but hidden in the UI
// Useful when the field exists in YAML but shouldn't be edited.
editor.Config{
    Hidden: []string{"internal-id", "schema-version"},
}
```

Keys that should be silently preserved without validation:

```go
// Config.PassthroughKeys - hidden from all sections, excluded from ctrl+l validation
editor.Config{
    PassthroughKeys: []string{"import", "$schema"},
}
```

**YAML (both hidden and passthrough are preserved on save):**
```yaml
import: shared.yaml          # passthrough - preserved, not shown in editor
$schema: "./schema.json"     # passthrough - preserved, not shown in editor
internal-id: "abc123"        # hidden - preserved, not shown in editor
name: "my-app"               # normal field - shown and editable
```
