<!-- markdownlint-disable MD033 -->
<p align="center">
  <img src="docs/yedit2.png" alt="Movelooper logo" width="385" height="256">
</p>
<!-- markdownlint-enable MD033 -->


A TUI YAML editor library for Go applications, built on [bubbletea](https://github.com/charmbracelet/bubbletea). Drop it into any CLI tool to give users a structured, schema-aware editor for their configuration files.

## What it does

- Opens a YAML file in a split-pane TUI: block list on the left, YAML editor on the right.
- Drives the editor from a Go struct - no JSON Schema, no struct tag annotations, just `yaml` tags.
- Shows a field tree for struct blocks (toggle fields on/off) and a `[N]` navigator for list/map blocks.
- Validates on save with a declarative rule set.
- Displays per-field hints, types, defaults, and examples in a side panel.
- Supports presets, two-level undo/redo, nested drill-in editing, and theming.

## Install

```sh
go get github.com/lucasassuncao/yedit
```

## Minimal example

```go
package main

import (
	"log"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/metadata"
)

type Config struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
}

func main() {
	src, err := metadata.NewFromTree(&Config{}, map[string]*metadata.Node{
		"server": {
			Children: map[string]*metadata.Node{
				"host": {FieldMeta: editor.FieldMeta{Description: "Address to bind.", Default: "localhost"}},
				"port": {FieldMeta: editor.FieldMeta{Description: "Port to listen on.", Default: "8080"}},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := editor.Run(editor.Config{
		Path:        "config.yaml",
		Schema:      &Config{},
		Metadata:    src,
		EnableHints: true,
	}); err != nil {
		log.Fatal(err)
	}
}
```

## Documentation

| Document | Contents |
|---|---|
| [Getting Started](docs/getting-started.md) | Full happy path: struct → metadata → editor.Run, validators, presets, and doc generation |
| [Architecture](docs/architecture.md) | Package layout and design rationale |
| [Schema Kinds Reference](docs/schema-kinds-reference.md) | How Go types map to editor behavior (KindObject, KindList, KindDictionary, KindVariant, …) |
| [Validators Reference](docs/validators.md) | All built-in validation rules with examples |
| [Presets & Metadata](docs/presets-hints.md) | PresetSource and MetadataSource configuration |
| [Interaction Model](docs/INTERACTION.md) | Key bindings and tree action matrix |

## Demo

![demo](docs/demo.gif)

## Example

`examples/test` is a self-contained example that exercises every schema pattern, validator, preset, and Config option. Run it from the repo root:

```sh
cd examples/test
go run . [--theme dracula]   # open the editor
go run . show-docs           # browse schema docs in the TUI
```
