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

### Keeping the compiler in the loop

A map literal is not checked: `"descriptoin"` compiles and fails at startup
instead. Declare your own struct with yaml tags and the entries go back to being
compiler-checked, while `Metadata()` still returns a plain map:

```go
// your package - imports nothing
type meta struct {
    Description string   `yaml:"description,omitempty"`
    Required    bool     `yaml:"required,omitempty"`
    Default     string   `yaml:"default,omitempty"`
    OneOf       []string `yaml:"oneof,omitempty"`
}

func (ServerConfig) Metadata() map[string]any {
    return map[string]any{
        "host": meta{Description: "Address the server binds to.", Default: "localhost"},
        "port": meta{Description: "TCP port to listen on.", Default: "8080"},
    }
}
```

Declare only the keys you use. `omitempty` keeps zero fields out of the
decoded tree.

### Key names in `Metadata()`

A `Metadata()` entry names the same fields in lowercase:

| key | Go field | type |
|---|---|---|
| `description`, `type`, `default`, `example` | Description, Type, Default, Example | string |
| `required`, `unique`, `multiline`, `prechecked` | Required, Unique, Multiline, PreChecked | bool |
| `oneof`, `notoneof` | OneOf, NotOneOf | []string |
| `min`, `max`, `pattern`, `deprecated`, `snippet` | Min, Max, Pattern, Deprecated, Snippet | string |
| `mincount`, `maxcount`, `minlength`, `maxlength` | MinCount, MaxCount, MinLength, MaxLength | int |
| `formats` | Formats | []string, by format name (`"url"`, `"directory"`) |
| `presentation` | Presentation | `"flat"`, `"inline"`, `"overlay"` |
| `children` | Children | map, for nesting the parent must declare itself |

A format name must have been registered by `spec.FormatCustom`, which every
built-in format and any app-specific one does at package init. An unknown name
is a startup error.

## metadata.New (recommended)

Use when the root struct is yours and can implement `MetadataProvider`. Each struct declares its own direct fields via `Metadata()`; nested structs that also implement `MetadataProvider` have their children populated automatically. Fields not covered by `Metadata()` are silently accepted and receive default (empty) `FieldMeta` values.

`Metadata()` returns a plain `map[string]any`, not a package type, so the same method can also feed a documentation generator without either package importing the other. The keys are the lowercased `FieldMeta` field names; a key that matches none is a startup error naming it.

```go
import (
    "github.com/lucasassuncao/yedit/editor"
    "github.com/lucasassuncao/yedit/metadata"
)

// Each struct declares only its own direct fields.
func (ServerConfig) Metadata() map[string]any {
    return map[string]any{
        "host": map[string]any{
            "description": "Address the server binds to.",
            "default":     "localhost",
            "example":     "host: 0.0.0.0",
        },
        "port": map[string]any{
            "description": "TCP port to listen on.",
            "default":     "8080",
            "example":     "port: 8080",
        },
    }
}

// Root struct lists its top-level blocks; children for nested structs that
// implement MetadataProvider are populated automatically.
func (Config) Metadata() map[string]any {
    return map[string]any{
        "server": map[string]any{
            "description": "HTTP server configuration.",
            "required":    true,
        },
        // no children needed - ServerConfig.Metadata() is composed automatically
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

With `metadata.New`, a self-referential struct (a `Filter` that contains `Any []Filter`) declares its fields once and nothing more. `New` recognises the type when it comes round again and reuses the same subtree:

```go
func (Filter) Metadata() map[string]any {
    return map[string]any{
        "regex": map[string]any{"description": "RE2 regex matched against the filename."},
        "any":   map[string]any{"description": "OR logic: match at least one sub-filter."},
    }
}
```

A `Metadata()` map cannot contain a cycle, and does not need to.

With `metadata.NewFromTree`, where you assemble the tree yourself, use shared pointers and two-phase initialization instead - a Go map literal cannot reference itself during construction:

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

Both paths are cycle-aware.

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

## Animating the panel

By default, pressing `h` shows or hides the Hint/Example panel instantly. Set
`Config.AnimationDuration` to make the panel ease open and closed instead,
growing and shrinking within the right column while the preview (or the YAML
editor, inside a block) takes up the slack:

```go
editor.Config{
    EnableHints:       true,
    Metadata:          meta,
    AnimationDuration: 180 * time.Millisecond,
}
```

The motion follows a quadratic ease-in-out curve, so it accelerates away from
the start and settles into the end rather than sliding at a constant rate. The
curve is deliberately gentle: terminal output is quantised to whole cells, so a
sharper curve (a cubic, say) reads as stutter rather than as speed, freezing the
value for several frames at each end and then jumping many cells at once through
the middle.

Around `150ms`-`200ms` reads as responsive; much longer starts to feel sluggish,
since the panel is only ~10 terminal rows tall and the animation has that many
distinct steps to work with. Raising the frame rate does not help for the same
reason: extra frames would only redraw heights already on screen.

The setting governs the Hint/Example panel only. Modal dialogs (alerts,
validation errors, confirmation prompts) always appear and dismiss instantly.

The default of `0` is deliberate. yedit is a library, and animation means the
editor emits timer messages into the host application's bubbletea loop. With
`AnimationDuration` unset no tick is ever scheduled, so an embedding app that
did not ask for animation pays nothing for it. The tick loop is also
self-cancelling: it runs only while a transition is in flight, never while the
editor sits idle.

## Full example

See `examples/test/main.go` for a complete, runnable example that exercises presets, metadata, and validators together.
