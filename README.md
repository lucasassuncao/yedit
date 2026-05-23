# yedit

A reusable TUI library for editing structured YAML files in Go.

`yedit` turns any Go struct annotated with `yaml` tags into a two-panel
bubbletea editor: the left panel lists the top-level keys discovered from
the struct; the right panel shows a live YAML preview. Pressing `Space`
on a key opens a sub-field overlay where the user toggles children on/off,
picks from optional presets, and edits the YAML snippet directly.

The library is headless of any specific schema — clients supply the
struct, optional presets, and any cross-field validation rules.

## Tags

Only the `yaml` tag is **required**. Everything else is optional and
enriches the editor when present:

| Tag                              | Effect                                            |
| -------------------------------- | ------------------------------------------------- |
| `yaml:"name"`                    | Required. The key as it appears in the YAML file. |
| `yaml:"-"`                       | Field is hidden from the editor.                  |
| `validate:"required"`            | Marks the field as required (renders a `*`).      |
| `validate:"oneof=a b c"`         | Restricts accepted values (available in `FieldDef.OneOf`). |
| `jsonschema:"required"`          | Alternative way to mark required.                 |
| `jsonschema:"default=X"`         | Default value (available in `FieldDef.Default`).  |
| `jsonschema_description:"..."`   | Description (available in `FieldDef.Description`). |

A struct with only `yaml` tags is enough to get a working editor:

```go
type Minimal struct {
    Name string `yaml:"name"`
    Port int    `yaml:"port"`
}
```

## Install

```bash
go get github.com/lucasassuncao/yedit
```

## Usage

Minimal — only `yaml` tags:

```go
package main

import (
    "log"

    "github.com/lucasassuncao/yedit/editor"
)

type MyConfig struct {
    Name  string `yaml:"name"`
    Image string `yaml:"image"`
    Build *struct {
        Dockerfile string `yaml:"dockerfile"`
        Context    string `yaml:"context"`
    } `yaml:"build"`
}

func main() {
    err := editor.Run(editor.Config{
        Path:   "config.yaml",
        Schema: &MyConfig{},
        Title:  "my editor",
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

Richer — add optional tags and validators to improve UX:

```go
type MyConfig struct {
    Name  string `yaml:"name"  validate:"required"             jsonschema_description:"Project name."`
    Image string `yaml:"image"                                 jsonschema_description:"Container image."`
    Build *struct {
        Dockerfile string `yaml:"dockerfile" validate:"required" jsonschema:"default=Dockerfile"`
        Context    string `yaml:"context"    validate:"required" jsonschema:"default=."`
    } `yaml:"build"`
}

editor.Run(editor.Config{
    Path:   "config.yaml",
    Schema: &MyConfig{},
    Title:  "my editor",
    Validators: []editor.Validator{
        editor.MutuallyExclusive("image", "build"),
    },
})
```

`editor.Run` blocks until the user quits. A non-existent `Path` is not
an error — the editor starts with an empty document and saves to the path
on `Ctrl+S`.

## Sub-packages

| Package      | Purpose                                                                 |
| ------------ | ----------------------------------------------------------------------- |
| `editor`     | Two-panel TUI; the main entry point (`editor.Run`)                      |
| `schema`     | Reflection over Go structs into a `FieldDef` tree; opt-in `Provider` for union types |
| `document`   | YAML document state: block-level parse/insert/remove/replace, undo, save |
| `presets`    | `Source` interface + `FromFS` implementation for per-field YAML snippets |
| `viewer`     | Read-only preset browser TUI                                            |
| `theme`      | Shared palette and layout primitives                                    |
| `components` | Reusable bubbletea widgets (`alert`, `picker`)                          |

## Union types

Reflection cannot infer the shape of types that wrap a union of scalar /
sequence / mapping. Such types opt into a small interface:

```go
type Provider interface {
    YeditSchema() []schema.FieldDef
}
```

If a field's type implements `Provider`, the editor uses the returned
`[]FieldDef` instead of descending by reflection. See
`devcontainerwizard/internal/model/mountorstring.go` for a working example.

## Presets

Each field can have any number of named presets. Implement `presets.Source`,
or pass an `fs.FS` to `presets.FromFS`:

```
my-presets/
├── build/
│   ├── base.yaml
│   └── multi-stage.yaml
└── customizations/
    ├── base.yaml
    └── vscode-go.yaml
```

The editor exposes a picker (`p` key in the overlay) over the names
returned by `Source.ListPresets(field)`.

## Status

Pre-1.0. The API may change between minor versions until v1.0.0.

## License

MIT — see [LICENSE](LICENSE).
