# yedit

A reusable TUI library for editing structured YAML files in Go.

`yedit` turns any Go struct annotated with `yaml` tags into a two-panel bubbletea editor. The left panel lists the top-level keys discovered from the struct; the right panel shows a live YAML preview. Pressing `Enter`
on a key opens a full-screen block editor where sub-fields can be toggled on/off, edited with presets, or written directly in YAML.

The library is schema-agnostic: clients supply a Go struct, optional presets, and any cross-field validation rules.

## Example

![yedit demo](docs/demo.gif)

## Requirements

Go 1.24+

## Install

```bash
go get github.com/lucasassuncao/yedit
```

## Quick start

```go
package main

import (
    "log"
    "github.com/lucasassuncao/yedit/editor"
)

type Config struct {
    Name  string `yaml:"name"`
    Image string `yaml:"image"`
    Build *struct {
        Dockerfile string `yaml:"dockerfile"`
        Context    string `yaml:"context"`
    } `yaml:"build"`
}

func main() {
    res, err := editor.Run(editor.Config{
        Path:   "config.yaml",
        Schema: &Config{},
        Title:  "my editor",
    })
    if err != nil {
        log.Fatal(err)
    }
    if res.Saved {
        // at least one save succeeded — reload config.yaml, apply it, etc.
    }
}
```

A non-existent `Path` is not an error — the editor starts with an empty document and saves to that path on `Ctrl+S`.

`editor.Run` returns a `Result` whose `Saved` field reports whether the user saved at least once during the session. Use `editor.RunContext(ctx, cfg)` when the editor should shut down on context cancellation.

## UI layout

The editor adapts its left panel to the field type:

**Main list** — shows all top-level keys split into ADDED (present in the file) and AVAILABLE (schema-known but not yet set). `Enter` opens a block; `h` toggles a Hint/Example panel describing the selected key.

**Struct tree** (KindObject) — left panel lists sub-fields in ADDED/AVAILABLE sections; `Enter` adds a field, `Ctrl+D` removes it, and `→` / `←` expand or collapse a node. The right panel shows a live YAML preview.

**Collection navigator** (KindList / KindDictionary with child defs) — left panel shows `[0] item`, `[1] item` … (map entries are keyed by their map key) and a `[+ add new]` row. `Enter` adds an item, `Ctrl+D` deletes the selected one.

**Single field** (KindPrimitive, KindEnum, free-form map/list) — the YAML editor takes focus immediately; the left panel shows the field itself and the Hint/Example panel shows its type, constraints, and an example.

A **Hint/Example** panel (bottom-right) shows the focused field's concrete type (`string`, `int`, `bool`, …), whether it is required, its default, allowed values, and an example. It is only shown when `Config.Hints` is set; toggled with `h` in the main list and always visible inside the block editor.

## Keyboard reference

### Main list

| Key | Action |
|-----|--------|
| `↑` / `k`, `↓` / `j` | Navigate |
| `g` / `G` | Jump to top / bottom |
| `/` | Filter list |
| `h` | Toggle the Hint/Example panel |
| `Enter` | Open or add block |
| `Ctrl+D` | Delete block (with confirmation) |
| `Ctrl+U` | Undo last change |
| `Ctrl+Y` | Redo last undone change |
| `Ctrl+R` | Reload file from disk (prompts if unsaved) |
| `Tab` | Focus the read-only preview pane |
| `Ctrl+S` | Save changes |
| `Ctrl+L` | Validate document |
| `Esc` / `q` | Quit (prompts if unsaved) |

### Block-edit tree

| Key | Action |
|-----|--------|
| `↑`, `↓` | Navigate |
| `→`, `←` | Expand / collapse node (on an openable `→` field, drills into a nested editor) |
| `Enter` | Add field / item, or drill into an openable field |
| `Ctrl+D` | Remove field / sequence item |
| `Ctrl+U` | Undo last change in this editor |
| `Ctrl+Y` | Redo last undone change in this editor |
| `p` | Open preset picker |
| `Tab` | Switch to YAML editor |
| `Ctrl+S` | Commit changes |
| `Esc` | Back (prompts if uncommitted) |

### YAML editor (right panel)

Only `Tab` and `Esc` are captured — all other keys go to the textarea.

## Tags

Only the `yaml` tag is **required**. The rest are optional enrichments:

| Tag | Effect |
|-----|--------|
| `yaml:"name"` | Required. The YAML key name. |
| `yaml:"-"` | Hide field from the editor. |
| `yaml:"name,omitempty"` | Sets `FieldDef.OmitEmpty = true` (zero value not written to disk). |
| `yaml:"name,flow"` | Sets `FieldDef.Flow = true` (serialised inline, e.g. `[a, b]`). |
| `validate:"required"` | Marks field as required (stored in `FieldDef.Required`). |
| `validate:"oneof=a b c"` | Restricts accepted values (stored in `FieldDef.OneOf`). |
| `jsonschema:"required"` | Alternative way to mark required. |
| `jsonschema:"default=X"` | Default value (stored in `FieldDef.Default`). |
| `jsonschema_description:"..."` | Description text (stored in `FieldDef.Description`). |

> `Required`, `Default`, `OneOf`, `Description`, `OmitEmpty`, and `Flow` are
> surfaced in `schema.FieldDef` and available to external tooling (doc generators,
> custom renderers). `Required`, `Default`, `OneOf`, and `Description` are also
> shown in the editor's Hint/Example panel.

## Full Config

```go
editor.Run(editor.Config{
    // Required
    Path:   "config.yaml",
    Schema: &MyConfig{},

    // Optional
    Title:  "my editor",

    // Preset snippets loaded from an fs.FS (see presets.FromFS)
    Presets: presets.FromFS(embedFS, "."),

    // Cross-field validation rules
    Validators: []editor.Validator{
        editor.MutuallyExclusive("image", "build"),
        editor.RequiredWith("service", "dockerComposeFile"),
    },

    // Sub-fields to pre-check when a new block is opened for the first time
    PreCheckedFields: editor.CheckedFieldMap{
        "build": {"dockerfile", "context"},
    },

    // Default YAML snippets inserted when a sub-field is toggled on
    FieldSnippets: editor.FieldSnippetMap{
        "build": {
            "dockerfile": "  dockerfile: Dockerfile\n",
            "context":    "  context: .\n",
        },
    },

    // Top-level keys to hide (e.g. legacy aliases)
    Hidden: []string{"dockerFile"},

    // Extra recursive levels for self-referential types (default 1)
    SchemaRecursionDepth: 2,
})
```

## Validators

Validators run before every save and when the user presses `Ctrl+L`. Each returns a list of `editor.Violation` values — a `Path` locating the offending node (empty for document-wide rules) plus a human-readable `Message` — shown in a blocking alert. Pass them via `editor.Config.Validators`; use `editor.ValidatorFunc` for inline custom rules.

Dotted paths expand sequences and dict-style mappings automatically in every path-based validator: a rule on `categories.installers.source.type` is checked inside every installer of every category, whether `categories` is a list or a map. Cross-field validators (`RequiredIf`, `CrossFieldOrdered`) apply this per-entry when both paths share the same parent prefix.

### MutuallyExclusive

Reports a violation when more than one of the listed keys is present at the same time.

**Top-level keys** — compared against the document's root-level blocks:

```go
editor.Run(editor.Config{
    // ...
    Validators: []editor.Validator{
        editor.MutuallyExclusive("image", "build", "dockerComposeFile"),
    },
})
```

**Dotted paths** — all paths must share the same parent prefix (paths that don't are a configuration error, reported as a violation on every validate). The validator navigates to that parent in the YAML tree and checks the leaf keys there.

Sequences (`- item`) and dict-style mappings (`key: {…}`) are expanded automatically, so every entry in a list or every value in a map is checked:

```go
editor.Run(editor.Config{
    // ...
    Validators: []editor.Validator{
        // "any" and "all" must not appear together under every
        // categories › <name> › installers › [n] › source › filter.
        editor.MutuallyExclusive(
            "categories.installers.source.filter.any",
            "categories.installers.source.filter.all",
        ),
    },
})
```

### MutuallyExclusiveNested

Like `MutuallyExclusive` for dotted paths, but designed for **recursive schemas** where the same constraint must hold at every nesting level (e.g. a `filter` struct whose `any`/`all` lists can themselves contain `filter` structs).

**Single key** — searches the entire document for every mapping whose direct parent key equals the given name, regardless of depth:

```go
editor.Run(editor.Config{
    // ...
    Validators: []editor.Validator{
        editor.MutuallyExclusiveNested("filter", "any", "all"),
    },
})
```

**Scoped dotted path** — navigates to the prefix first, then recurses only within that subtree. Use this when multiple unrelated objects in the document share the same key name and the rule should only apply to one of them. The last segment of the path is the recursive key name:

```go
editor.Run(editor.Config{
    // ...
    Validators: []editor.Validator{
        // Only recurse through filters that live under
        // categories › <name> › installers › [n] › source.
        // A "filter" key elsewhere in the document is ignored.
        editor.MutuallyExclusiveNested(
            "categories.installers.source.filter",
            "any", "all",
        ),
    },
})
```

### RequiredWith

Reports a violation when `key` is present but `parent` is not:

```go
editor.Run(editor.Config{
    // ...
    Validators: []editor.Validator{
        editor.RequiredWith("service", "dockerComposeFile"),
    },
})
```

### AtLeastOneOf

Reports a violation when **none** of the listed keys is present:

```go
editor.AtLeastOneOf("image", "build", "dockerComposeFile")
```

### ExactlyOneOf

Reports a violation when **none or more than one** of the listed keys is present:

```go
editor.ExactlyOneOf("image", "build", "dockerComposeFile")
```

### RequiredIf

Reports a violation when `key` is absent but `condPath` equals `condValue`:

```go
// "tls-cert" is required when "protocol" is "https"
editor.RequiredIf("tls-cert", "server.protocol", "https")
```

### ValueOneOf

Reports a violation when the field at `path` exists but its value is not in the allowed set:

```go
editor.ValueOneOf("server.protocol", "http", "https", "grpc")
```

### CrossFieldOrdered

Reports a violation when both paths are present but the value at `smallerPath` is not strictly less than `largerPath`. Supports plain numbers (`"1"`, `"0.5"`), `time.Duration` strings (e.g. `"24h"`), and size strings — `KB`/`MB`/`GB`/`TB` are decimal (powers of 1000), `KiB`/`MiB`/`GiB`/`TiB` are binary (powers of 1024). Both sides must be of the same kind:

```go
editor.CrossFieldOrdered("source.filter.min-age", "source.filter.max-age")
editor.CrossFieldOrdered("source.filter.min-size", "source.filter.max-size")
```

### NoDuplicates

Reports a violation when two or more items in the sequence at `seqPath` share the same value for `field`:

```go
editor.NoDuplicates("categories", "name")
```

### Required

Reports a violation when a path is absent or holds an empty/null scalar. A path with no dots is required unconditionally; a dotted path is conditional — the leaf is only required where its parent exists, and sequences / dict-style mappings along the path are expanded automatically:

```go
editor.Required("version")          // top-level, unconditional
editor.Required("categories.name")  // every category entry needs "name"
```

### RequiredFromSchema

Enforces the schema's `validate:"required"` / `jsonschema:"required"` tags at validate/save time (without it the marker is display-only). Required fields inside an optional block are only enforced while the block is present; sequence and dictionary entries are checked individually. The editor wires the discovered schema in automatically:

```go
editor.RequiredFromSchema()
```

### AllOrNone

Reports a violation when only some of the listed keys are present — they must appear together or not at all (e.g. a TLS cert/key pair). Supports top-level keys and dotted paths with a shared parent, like `MutuallyExclusive`:

```go
editor.AllOrNone("tls-cert", "tls-key")
editor.AllOrNone("server.tls-cert", "server.tls-key")
```

### ValueInRange

Reports a violation when the scalar at `path` is present but outside the inclusive `[min, max]` range. Bounds and value may be plain numbers, `time.Duration` strings, or size strings (same kinds as `CrossFieldOrdered`):

```go
editor.ValueInRange("server.port", "1", "65535")
editor.ValueInRange("filter.max-age", "1h", "8760h")
```

### ValueMatches

Reports a violation when the scalar at `path` is present but does not match the regular expression. Combine with `Required` when the field is mandatory:

```go
editor.ValueMatches("version", `^\d+\.\d+\.\d+$`)
```

### CountRange

Reports a violation when the collection at `path` has fewer than `min` or more than `max` entries (`max < 0` means no upper bound). Sequences count items, mappings count keys:

```go
editor.CountRange("workers", 1, 10)
editor.CountRange("categories", 1, -1) // at least one
```

### UniqueValues

Reports a violation when two or more scalar items in the sequence at `seqPath` share the same value (the struct-entry variant is `NoDuplicates`):

```go
editor.UniqueValues("tags")
```

### Deprecated

Reports a violation whenever `path` is present, carrying a migration hint. Combine with `Config.NoValidateOnSave` to make it a non-blocking warning:

```go
editor.Deprecated("dockerFile", "use build.dockerfile instead")
```

### Combining validators

```go
editor.Run(editor.Config{
    // ...
    Validators: []editor.Validator{
        editor.MutuallyExclusive("image", "build", "dockerComposeFile"),
        editor.RequiredWith("service", "dockerComposeFile"),
        editor.MutuallyExclusiveNested(
            "categories.installers.source.filter",
            "any", "all",
        ),
    },
})
```

## Union types

Reflection cannot infer the shape of types that wrap a union (scalar / sequence / mapping). Such types opt into a small interface:

```go
type Provider interface {
    YeditSchema() []schema.FieldDef
}
```

If a field's type implements `Provider`, the editor uses the returned `[]FieldDef` instead of descending by reflection. The field kind is set to `KindVariant` and the YAML editor takes focus directly.

```go
type TimeoutValue struct{}

func (TimeoutValue) YeditSchema() []schema.FieldDef {
    return []schema.FieldDef{
        {YAMLName: "connect", Kind: schema.KindPrimitive},
        {YAMLName: "read",    Kind: schema.KindPrimitive},
        {YAMLName: "write",   Kind: schema.KindPrimitive},
    }
}
```

## Presets

Each field can have named presets. Implement `editor.PresetSource` or use `presets.FromFS` with a directory tree:

```
my-presets/
├── build/
│   ├── base.yaml
│   └── multi-stage.yaml
└── customizations/
    └── vscode-go.yaml
```

The preset picker is opened with `p` in the block-edit tree panel.

## Environment

| Variable | Effect |
|----------|--------|
| `NO_COLOR` | Disables all color output (monochrome mode). |

Minimum terminal size: **80 × 20**. Below that the editor shows a resize prompt.

## Sub-packages

| Package | Purpose |
|---------|---------|
| `editor` | Two-panel TUI; main entry point (`editor.Run`) |
| `schema` | Reflection over Go structs into a `FieldDef` tree; opt-in `Provider` for union types |
| `document` | YAML document state: block-level parse / insert / remove / replace, undo, save |
| `presets` | `Source` interface + `FromFS` for per-field YAML snippets |
| `viewer` | Read-only preset browser TUI |
| `theme` | Shared palette and layout primitives |
| `internal` | Non-public helpers (`alert` widget, `yamlnode` utilities) |

## Examples

See [`examples/test`](examples/test/main.go) for a self-contained program that exercises all five schema patterns, `KindVariant`, nested slices, deep nesting, `oneof`, `MutuallyExclusive`, unknown-key validation, and schema edge cases (anonymous embeds, `yaml.Marshaler`, `interface{}`, non-string map keys, `omitempty`/`flow` flags).

## Status

Pre-1.0. The API may change between minor versions until v1.0.0.

## License

MIT — see [LICENSE](LICENSE).
