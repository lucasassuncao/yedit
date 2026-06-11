# Presets and Metadata

This document explains how to configure `Presets` and `Metadata` in `editor.Config`.

---

## Presets

Presets populate the preset picker with ready-made YAML snippets for a block. When a user selects a preset, the editor seeds the YAML text area with the chosen snippet.

### Interface

```go
type PresetSource interface {
    ListFields()              []string
    ListPresets(field string) []string
    PresetYAML(field, name string) (string, error)
}
```

- `ListFields` - block keys that have at least one preset.
- `ListPresets` - preset names for a given block (used to build the picker list).
- `PresetYAML` - the YAML snippet for a `(field, name)` pair. The snippet must be a valid YAML mapping keyed by the field name, e.g. `server:\n  host: localhost\n`.

### Struct-backed implementation

The recommended approach is a Go struct per block, marshaled with `yaml.Marshal`:

```go
type myPresetSource struct{}

var serverPresets = map[string]ServerConfig{
    "minimal":    {Host: "localhost", Port: 8080},
    "production": {Host: "0.0.0.0", Port: 443, TLS: true},
}

func (myPresetSource) ListFields() []string { return []string{"server"} }

func (myPresetSource) ListPresets(field string) []string {
    var names []string
    for name := range serverPresets {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}

func (myPresetSource) PresetYAML(field, name string) (string, error) {
    p, ok := serverPresets[name]
    if !ok {
        return "", fmt.Errorf("server preset %q not found", name)
    }
    out, err := yaml.Marshal(map[string]any{field: p})
    if err != nil {
        return "", err
    }
    return string(out), nil
}
```

Wire it in:

```go
editor.Run(editor.Config{
    Schema:  &MyConfig{},
    Presets: myPresetSource{},
    // ...
})
```

### Filesystem-backed implementation

For presets shipped as embedded files, use `presets.FromFS`:

```go
//go:embed presets
var presetsFS embed.FS

editor.Run(editor.Config{
    Presets: presets.FromFS(presetsFS, "presets"),
    // ...
})
```

Expected layout:

```
presets/
  server/
    minimal.yaml
    production.yaml
  logging/
    development.yaml
    production.yaml
```

Each file is a YAML mapping keyed by the block name:

```yaml
# presets/server/minimal.yaml
server:
  host: localhost
  port: 8080
```

---

## Metadata

Metadata populates the Hint/Example panel shown when the user presses `h` in the main list or when a field is selected in the block editor. Each entry carries a description, type label, required flag, default value, allowed values, and an example snippet.

When `Config.Metadata` is `nil`, the hint panel shows only a generated example. Set `Metadata` to a `MetadataSource` and set `EnableHints: true` to enable the full panel.

### Interface

```go
type MetadataSource interface {
    FieldMeta(blockKey, fieldPath string) FieldMeta
}
```

- `blockKey` - the top-level YAML key (e.g. `"server"`).
- `fieldPath` - dot-separated path within the block (e.g. `"pool.timeout"`), or `""` for the block-level entry.

`MetadataSource` is the sole authority for all hint display data and `FromMetadata` validator constraints. yedit does not derive metadata from struct tags.

### FieldMeta

```go
type FieldMeta struct {
    Description string
    Type        string   // "string", "bool", "int", "[]string", "object", "duration", …
    Required    bool
    Default     string
    OneOf       []string
    Example     string
    // constraint fields used by FromMetadata validators:
    Min, Max           string
    Pattern            string
    MinCount, MaxCount int
    Unique             bool
    Deprecated         string
}
```

Set only the fields that are meaningful for the field being described. Zero values declare nothing.

### metadata.Build (recommended)

`metadata.Build` validates the field name tree against your Go struct at startup - a typo in a field name is an error at launch, not a silently dead hint:

```go
import (
    "github.com/lucasassuncao/yedit/editor"
    "github.com/lucasassuncao/yedit/metadata"
)

src, err := metadata.Build(&Config{}, map[string]*metadata.Node{
    "server": {
        FieldMeta: editor.FieldMeta{
            Description: "HTTP server configuration.",
            Required:    true,
        },
        Children: map[string]*metadata.Node{
            "host": {FieldMeta: editor.FieldMeta{
                Description: "Address the server binds to.",
                Default:     "localhost",
                Example:     "host: 0.0.0.0",
            }},
            "port": {FieldMeta: editor.FieldMeta{
                Description: "TCP port to listen on.",
                Default:     "8080",
                Example:     "port: 8080",
            }},
        },
    },
})
if err != nil {
    log.Fatal(err)
}

editor.Run(editor.Config{
    Metadata:    src,
    EnableHints: true,
    // ...
})
```

`metadata.Node` embeds `editor.FieldMeta` and adds `Children map[string]*Node`. The `Type` field is auto-filled from the Go type if left empty.

### MetadataFunc adapter

For simple cases or programmatic sources, use `editor.MetadataFunc`:

```go
editor.Run(editor.Config{
    Metadata: editor.MetadataFunc(func(block, fieldPath string) editor.FieldMeta {
        if block == "server" && fieldPath == "host" {
            return editor.FieldMeta{
                Description: "Address the server binds to.",
                Type:        "string",
                Default:     "localhost",
            }
        }
        return editor.FieldMeta{}
    }),
    EnableHints: true,
})
```

### Recursive types

For self-referential structs (e.g. a `Filter` that contains `Any []Filter`), use shared pointers and two-phase initialization to avoid infinite recursion:

```go
// Phase 1: create the shared node.
anyNode := &metadata.Node{
    FieldMeta: editor.FieldMeta{Description: "OR logic: match at least one sub-filter."},
}

// Phase 2: build children map, then back-assign to close the cycle.
filterChildren := map[string]*metadata.Node{
    "regex": {FieldMeta: editor.FieldMeta{Description: "RE2 regex matched against the filename."}},
    "any":   anyNode,
}
anyNode.Children = filterChildren // shared pointer - resolves at any depth

src, err := metadata.Build(&Config{}, map[string]*metadata.Node{
    "filters": {Children: filterChildren},
})
```

A Go map literal cannot reference itself during construction, so this two-phase pattern is required. `metadata.Build` is cycle-aware and handles shared pointers correctly.

### Type labels

`metadata.Build` fills `Type` automatically from the Go type. When setting it manually, use:

| Go type         | Type label       |
|-----------------|------------------|
| `string`        | `"string"`       |
| `bool`          | `"bool"`         |
| `int`           | `"int"`          |
| `float64`       | `"float"`        |
| `time.Duration` | `"duration"`     |
| `[]string`      | `"[]string"`     |
| `[]SomeStruct`  | `"[]object"`     |
| `map[string]V`  | `"map[string]V"` |
| struct          | `"object"`       |
| `interface{}`   | `"any"`          |

yedit displays the `Type` label as-is; any string meaningful to your users is valid.

---

## Full example

See `examples/test/main.go` for a complete, runnable example that exercises presets, metadata, and validators together.
