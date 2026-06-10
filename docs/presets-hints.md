# Presets and Hints

This document explains how to configure `Presets` and `Hints` in `editor.Config`.

---

## Presets

Presets populate the preset picker with ready-made YAML snippets for a block. When a user selects a preset, the editor seeds the YAML text area with the chosen snippet.

### Interface

```go
type Source interface {
    ListFields()              []string
    ListPresets(field string) []string
    PresetYAML(field, name string) (string, error)
}
```

- `ListFields` — block keys that have at least one preset.
- `ListPresets` — preset names for a given block (used to build the picker list).
- `PresetYAML` — the YAML snippet for a `(field, name)` pair. The snippet must be a valid YAML mapping keyed by the field name, e.g. `server:\n  host: localhost\n`.

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

## Hints

Hints populate the hint panel shown when the user presses `h` or selects a field in the block editor. Each hint carries a description, type label, required flag, default value, allowed values, and an example snippet.

### Interface

```go
type HintSource interface {
    FieldHint(blockKey, fieldPath string) FieldMeta
}
```

- `blockKey` — the top-level YAML key (e.g. `"server"`).
- `fieldPath` — dot-separated path within the block (e.g. `"pool.timeout"`), or `""` for the block-level hint.

`HintSource` is the sole authority for all hint display data. yedit does not derive hints from struct tags.

### FieldMeta

```go
type FieldMeta struct {
    Description string
    Type        string   // "string", "bool", "int", "[]string", "object", "duration", …
    Required    bool
    Default     string
    OneOf       []string
    Example     string
}
```

Set only the fields that are meaningful for the field being described. Zero values are omitted from the hint panel.

### HintFunc adapter

For simple cases with no nesting, use `editor.HintFunc`:

```go
editor.Run(editor.Config{
    Hints: editor.HintFunc(func(block, fieldPath string) editor.FieldMeta {
        if block == "server" && fieldPath == "host" {
            return editor.FieldMeta{
                Description: "Address the server binds to.",
                Type:        "string",
                Default:     "localhost",
            }
        }
        return editor.FieldMeta{}
    }),
})
```

### HintNode tree (recommended)

For schemas with nested fields, build a hierarchical tree. Copy the `hintNode` type and `buildHintSource` helper into your application — they are intentionally small and dependency-free:

```go
type hintNode struct {
    editor.FieldMeta
    Children map[string]*hintNode
}

func buildHintSource(tree map[string]*hintNode) editor.HintSource {
    return editor.HintFunc(func(block, fieldPath string) editor.FieldMeta {
        node, ok := tree[block]
        if !ok {
            return editor.FieldMeta{}
        }
        if fieldPath == "" {
            return node.FieldMeta
        }
        cur := node
        for _, seg := range strings.Split(fieldPath, ".") {
            if cur.Children == nil {
                return editor.FieldMeta{}
            }
            next, ok := cur.Children[seg]
            if !ok {
                return editor.FieldMeta{}
            }
            cur = next
        }
        return cur.FieldMeta
    })
}
```

Build the tree and wire it in:

```go
var appHints = buildHintSource(map[string]*hintNode{
    "server": {
        FieldMeta: editor.FieldMeta{
            Description: "HTTP server configuration.",
            Type:        "object",
            Required:    true,
        },
        Children: map[string]*hintNode{
            "host": {FieldMeta: editor.FieldMeta{
                Description: "Address the server binds to.",
                Type:        "string",
                Default:     "localhost",
                Example:     "host: 0.0.0.0",
            }},
            "port": {FieldMeta: editor.FieldMeta{
                Description: "TCP port to listen on.",
                Type:        "int",
                Default:     "8080",
                Example:     "port: 8080",
            }},
        },
    },
})

editor.Run(editor.Config{
    Hints: appHints,
    // ...
})
```

### Recursive types

When a schema type is self-referential (e.g. a `Filter` that contains `Any []Filter`), use shared pointers and two-phase initialization to avoid infinite recursion:

```go
// Phase 1: create the shared node.
anyNode := &hintNode{FieldMeta: editor.FieldMeta{
    Description: "OR logic: file must match at least one sub-filter.",
}}

// Phase 2: build the children map, then back-assign to close the cycle.
filterChildren := map[string]*hintNode{
    "regex": {FieldMeta: editor.FieldMeta{Description: "RE2 regex matched against the filename."}},
    "any":   anyNode,
}
anyNode.Children = filterChildren // shared pointer — resolves at any depth
```

A Go map literal cannot reference itself during construction (the variable is not yet assigned when the literal is evaluated), so this two-phase pattern is required.

### Type labels

Set `Type` to a human-readable string that matches the Go type of the field:

| Go type            | Type label        |
|--------------------|-------------------|
| `string`           | `"string"`        |
| `bool`             | `"bool"`          |
| `int`              | `"int"`           |
| `float64`          | `"float"`         |
| `time.Duration`    | `"duration"`      |
| `[]string`         | `"[]string"`      |
| `[]SomeStruct`     | `"[]object"`      |
| `map[string]V`     | `"map[string]V"`  |
| struct             | `"object"`        |
| `interface{}`      | `"any"`           |

yedit displays the `Type` label as-is; choose any string that is meaningful to your users.

---

## Full example

See `examples/test/main.go` for a complete, runnable example of both patterns side by side.
