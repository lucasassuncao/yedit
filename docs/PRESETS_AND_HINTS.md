# Presets and Metadata

This document explains how to configure `Presets` and `Metadata` in `editor.Config`.

---

## Presets

Presets populate the preset picker with ready-made YAML snippets for a block. When a user selects a preset, the editor seeds the YAML text area with the chosen snippet.

Presets are defined as Go structs and marshaled to YAML automatically - no hand-written YAML files or boilerplate interface implementations required.

### The three-function pattern

For each field that has presets, define three functions:

```go
type DatabaseConfig struct {
    Driver   string `yaml:"driver"`
    DSN      string `yaml:"dsn"`
    MaxConns int    `yaml:"max-conns"`
}

// private: holds all preset values
func databasePresetsMap() map[string]DatabaseConfig {
    return map[string]DatabaseConfig{
        "postgres-local": {Driver: "postgres", DSN: "postgres://localhost/mydb", MaxConns: 10},
        "mysql-local":    {Driver: "mysql",    DSN: "mysql://localhost/mydb",    MaxConns: 5},
        "sqlite":         {Driver: "sqlite",   DSN: "file:app.db"},
    }
}

// public: returns the content of a specific preset by name
func DatabasePreset(name string) DatabaseConfig {
    return databasePresetsMap()[name]
}

// public: lists all available preset names (sorted)
func ListOfDatabasePresets() []string {
    field := "database"
    return presets.ForField(field, databasePresetsMap()).ListPresets(field)
}
```

### Single field

When only one field has presets, pass `presets.ForField` directly:

```go
editor.Run(editor.Config{
    Schema:  &MyConfig{},
    Presets: presets.ForField("database", databasePresetsMap()),
    // ...
})
```

### Multiple fields

When more than one field has presets, wrap them with `presets.Combine`:

```go
editor.Run(editor.Config{
    Schema: &MyConfig{},
    Presets: presets.Combine(
        presets.ForField("database", databasePresetsMap()),
        presets.ForField("server",   serverPresetsMap()),
        presets.ForField("logging",  loggingPresetsMap()),
    ),
    // ...
})
```

`Combine` merges the sources into one: when the editor asks for `"database"` presets, it dispatches to the `ForField("database", ...)` source; `"server"` goes to the next, and so on.

### Dynamic lookup (no picker)

When presets come from an external source (database, API) and cannot be enumerated upfront, use `presets.Func`. The picker will not appear, but direct `(field, name)` lookups still work:

```go
editor.Run(editor.Config{
    Presets: presets.Func(func(field, name string) (string, error) {
        snippet, err := myRegistry.FetchPreset(field, name)
        return snippet, err
    }),
})
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
