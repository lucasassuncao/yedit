# Metadata and Hints

This document explains how to configure `Config.Metadata` in `editor.Config`.

---

Metadata populates the Hint/Example panel shown when the user presses `h` in the main list or when a field is selected in the block editor. Each entry carries a description, type label, required flag, default value, allowed values, and an example snippet.

When `Config.Metadata` is `nil`, the hint panel shows only a generated example. Set `Metadata` to a `MetadataSource` and set `EnableHints: true` to enable the full panel.

## Interface

```go
type MetadataSource interface {
    FieldMeta(blockKey, fieldPath string) FieldMeta
}
```

- `blockKey` - the top-level YAML key (e.g. `"server"`).
- `fieldPath` - dot-separated path within the block (e.g. `"pool.timeout"`), or `""` for the block-level entry.

`MetadataSource` is the sole authority for all hint display data and `FromMetadata` validator constraints. yamltui does not derive metadata from struct tags.

## FieldMeta

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

## metadata.New (recommended)

Use when the root struct is yours and can implement `MetadataProvider`. Each struct declares its own direct fields via `Metadata()`; nested structs that also implement `MetadataProvider` have their `Children` populated automatically. Full coverage is enforced: adding a yaml-tagged field to the struct without updating `Metadata()` is a startup error.

```go
import (
    "github.com/lucasassuncao/yedit/editor"
    "github.com/lucasassuncao/yedit/metadata"
)

// Each struct declares only its own direct fields.
func (ServerConfig) Metadata() map[string]*metadata.Node {
    return map[string]*metadata.Node{
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
    }
}

// Root struct lists its top-level blocks; Children for nested structs that
// implement MetadataProvider are populated automatically.
func (Config) Metadata() map[string]*metadata.Node {
    return map[string]*metadata.Node{
        "server": {FieldMeta: editor.FieldMeta{
            Description: "HTTP server configuration.",
            Required:    true,
        }},
        // no Children needed - ServerConfig.Metadata() is composed automatically
    }
}

src, err := metadata.New(Config{})
if err != nil {
    log.Fatal(err)
}

editor.Run(editor.Config{
    Metadata:    src,
    EnableHints: true,
    // ...
})
```

## metadata.NewFromTree (escape hatch)

Use when the root struct comes from a third-party package and cannot implement `MetadataProvider`. You assemble the full `Node` tree manually and pass it alongside the struct pointer. `New` calls `NewFromTree` internally as its final step, so both provide the same validation and `Type` inference.

```go
// ThirdPartyConfig is from an external package - you cannot add methods to it.
src, err := metadata.NewFromTree(&ThirdPartyConfig{}, map[string]*metadata.Node{
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
```

`metadata.Node` embeds `editor.FieldMeta` and adds `Children map[string]*Node`. The `Type` field is auto-filled from the Go type if left empty.

## MetadataFunc adapter

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

## Recursive types

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

src, err := metadata.NewFromTree(&Config{}, map[string]*metadata.Node{
    "filters": {Children: filterChildren},
})
```

A Go map literal cannot reference itself during construction, so this two-phase pattern is required. Both `NewFromTree` and `New` are cycle-aware and handle shared pointers correctly.

## Type labels

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

yamltui displays the `Type` label as-is; any string meaningful to your users is valid.

## Full example

See `examples/test/main.go` for a complete, runnable example that exercises presets, metadata, and validators together.
