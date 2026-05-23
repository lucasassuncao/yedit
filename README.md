# yedit

A reusable TUI library for editing structured YAML files in Go.

`yedit` turns any annotated Go struct into a two-panel bubbletea editor:
the left panel lists the top-level keys discovered from the struct's
`yaml`/`validate`/`jsonschema_description` tags; the right panel shows a
live YAML preview. Pressing `Space` on a key opens a sub-field overlay
where the user toggles children on/off, picks from optional presets, and
edits the YAML snippet directly.

The library is headless of any specific schema — clients supply the
struct, optional presets, and any cross-field validation rules.

## Install

```bash
go get github.com/lucasassuncao/yedit
```

## Usage

```go
package main

import (
    "log"

    "github.com/lucasassuncao/yedit/editor"
)

type MyConfig struct {
    Name  string `yaml:"name"  validate:"required" jsonschema_description:"Project name."`
    Image string `yaml:"image"                       jsonschema_description:"Container image."`
    Build *struct {
        Dockerfile string `yaml:"dockerfile" validate:"required" jsonschema:"default=Dockerfile"`
        Context    string `yaml:"context"    validate:"required" jsonschema:"default=."`
    } `yaml:"build"`
}

func main() {
    err := editor.Run(editor.Config{
        Path:   "config.yaml",
        Schema: &MyConfig{},
        Title:  "my editor",
        Validators: []editor.Validator{
            editor.MutuallyExclusive("image", "build"),
        },
    })
    if err != nil {
        log.Fatal(err)
    }
}
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
