# yedit

A reusable TUI library for editing structured YAML files in Go.

`yedit` turns any Go struct annotated with `yaml` tags into a two-panel
bubbletea editor. The left panel lists the top-level keys discovered from
the struct; the right panel shows a live YAML preview. Pressing `Enter`
on a key opens a full-screen block editor where sub-fields can be toggled
on/off, edited with presets, or written directly in YAML.

The library is schema-agnostic — clients supply a Go struct, optional
presets, and any cross-field validation rules.

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
    if err := editor.Run(editor.Config{
        Path:   "config.yaml",
        Schema: &Config{},
        Title:  "my editor",
    }); err != nil {
        log.Fatal(err)
    }
}
```

A non-existent `Path` is not an error — the editor starts with an empty
document and saves to that path on `Ctrl+S`.

## UI layout

The editor adapts its left panel to the field type:

**Main list** — shows all top-level keys split into ADDED (present in the
file) and AVAILABLE (schema-known but not yet set). `Enter` opens a block;
`h` toggles a Hint/Example panel describing the selected key.

**Struct tree** (KindObject) — left panel lists sub-fields in ADDED /
AVAILABLE sections; `Enter` adds a field, `Ctrl+D` removes it, and `→` / `←`
expand or collapse a node. The right panel shows a live YAML preview.

**Collection navigator** (KindList / KindDictionary with child defs) — left
panel shows `[0] item`, `[1] item` … (map entries are keyed by their map key)
and a `[+ add new]` row. `Enter` adds an item, `Ctrl+D` deletes the selected
one.

**Single field** (KindPrimitive, KindEnum, free-form map/list) — the YAML
editor takes focus immediately; the left panel shows the field itself and the
Hint/Example panel shows its type, constraints, and an example.

A **Hint/Example** panel (bottom-right) shows the focused field's concrete type
(`string`, `int`, `bool`, …), whether it is required, its default, allowed
values, and an example. It is always visible in the block editor and toggled
with `h` in the main list.

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
| `validate:"required"` | Marks field as required (stored in `FieldDef.Required`). |
| `validate:"oneof=a b c"` | Restricts accepted values (stored in `FieldDef.OneOf`). |
| `jsonschema:"required"` | Alternative way to mark required. |
| `jsonschema:"default=X"` | Default value (stored in `FieldDef.Default`). |
| `jsonschema_description:"..."` | Description text (stored in `FieldDef.Description`). |

> `Required`, `Default`, `OneOf`, and `Description` are surfaced in the editor's
> Hint/Example panel, and are also available to external tooling (doc generators,
> custom renderers) via `schema.FieldDef`.

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
    PreCheckedFields: map[string][]string{
        "build": {"dockerfile", "context"},
    },

    // Default YAML snippets inserted when a sub-field is toggled on
    FieldSnippets: map[string]map[string]string{
        "build": {
            "dockerfile": "  dockerfile: Dockerfile\n",
            "context":    "  context: .\n",
        },
    },

    // Top-level keys to hide (e.g. legacy aliases)
    Hidden: []string{"dockerFile"},
})
```

## Union types

Reflection cannot infer the shape of types that wrap a union (scalar /
sequence / mapping). Such types opt into a small interface:

```go
type Provider interface {
    YeditSchema() []schema.FieldDef
}
```

If a field's type implements `Provider`, the editor uses the returned
`[]FieldDef` instead of descending by reflection. The field kind is set to
`KindVariant` and the YAML editor takes focus directly.

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

Each field can have named presets. Implement `presets.Source` or use
`presets.FromFS` with a directory tree:

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
| `components` | Reusable bubbletea widgets (`alert`) |

## Examples

See [`examples/test`](examples/test/main.go) for a self-contained program
that exercises all four schema patterns, `KindVariant`, nested slices, deep
nesting, `oneof`, `MutuallyExclusive`, and unknown-key validation.

## Status

Pre-1.0. The API may change between minor versions until v1.0.0.

## License

MIT — see [LICENSE](LICENSE).
