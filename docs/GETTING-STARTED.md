# Getting Started

This guide walks you through the full integration path: define your config struct, attach metadata, run the editor, add validators, add presets, and wire up documentation commands.

---

## 1. Define the config struct

yedit drives the editor from a Go struct using `yaml` tags. No other annotations are needed.

```go
type Config struct {
    Server  ServerConfig  `yaml:"server"`
    Logging LoggingConfig `yaml:"logging"`
}

type ServerConfig struct {
    Host string `yaml:"host"`
    Port int    `yaml:"port"`
    TLS  bool   `yaml:"tls"`
}

type LoggingConfig struct {
    Level      string `yaml:"level"`
    File       string `yaml:"file"`
    ShowCaller bool   `yaml:"show-caller"`
}
```

See [Schema Kinds Reference](schema-kinds-reference.md) for how each Go type maps to editor behavior (`KindObject`, `KindList`, `KindDictionary`, etc.).

---

## 2. Build metadata

`metadata.Build` validates a field-metadata tree against your struct at startup - typos in field names surface immediately instead of silently never showing in the hint panel.

```go
import (
    "github.com/lucasassuncao/yedit/editor"
    "github.com/lucasassuncao/yedit/metadata"
)

src, err := metadata.Build(&Config{}, map[string]*metadata.Node{
    "server": {
        FieldMeta: editor.FieldMeta{Description: "HTTP server configuration.", Required: true},
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
            "tls": {FieldMeta: editor.FieldMeta{
                Description: "Enable HTTPS. Requires a certificate and key.",
                Default:     "false",
            }},
        },
    },
    "logging": {
        FieldMeta: editor.FieldMeta{Description: "Application logging configuration."},
        Children: map[string]*metadata.Node{
            "level": {FieldMeta: editor.FieldMeta{
                Description: "Minimum log severity to emit.",
                Required:    true,
                OneOf:       []string{"debug", "info", "warn", "error"},
                Default:     "info",
            }},
            "file": {FieldMeta: editor.FieldMeta{
                Description: "Path to the log file.",
                Example:     "file: /var/log/app.log",
            }},
            "show-caller": {FieldMeta: editor.FieldMeta{
                Description: "Append source file and line to each log entry.",
                Default:     "false",
            }},
        },
    },
})
if err != nil {
    log.Fatal(err) // unknown field name, schema mismatch, etc.
}
```

`metadata.Node` embeds `editor.FieldMeta` for description, type label, default, required flag, allowed values (`OneOf`), and example snippet. The `Type` field is auto-filled from the Go type if left empty.

---

## 3. Run the editor

```go
res, err := editor.Run(editor.Config{
    Path:        "config.yaml",
    Schema:      &Config{},
    Metadata:    src,
    EnableHints: true,
})
if err != nil {
    log.Fatal(err)
}
if res.Saved {
    fmt.Println("saved")
}
```

`editor.Run` blocks until the user exits (Esc or Ctrl+C). It returns `RunResult.Saved = true` when the user wrote changes to disk.

### Key Config fields

| Field | Purpose |
|---|---|
| `Path` | YAML file to open (created if absent when using `editor.Run` directly) |
| `Schema` | Pointer to the config struct - drives field discovery |
| `Metadata` | `MetadataSource` for the hint panel and `FromMetadata` validators |
| `EnableHints` | Show the Hint/Example side panel |
| `Title` | Header text shown in the TUI |
| `Hidden` | Field YAML names to hide from the UI (preserved on save) |
| `PassthroughKeys` | Top-level keys preserved silently without validation |
| `PreCheckedFields` | Fields toggled ON automatically when opening a new block |
| `FieldSnippets` | YAML inserted when a field is toggled ON |

---

## 4. Add validators

Validators run before every save. Declare rules once - they appear in the hint panel and are enforced on save.

```go
editor.Run(editor.Config{
    // ...
    Validators: []editor.Validator{
        // Enforce FieldMeta.Required, FieldMeta.OneOf, etc. declared in metadata:
        editor.RequiredFromMetadata(),
        editor.OneOfFromMetadata(),

        // Explicit rules:
        editor.Required("server"),
        editor.MutuallyExclusive("image", "build"),
        editor.ValueInRange("server.port", "1", "65535"),
        editor.AllOrNone("server.tls-cert", "server.tls-key"),
    },
})
```

See [Validators Reference](validators.md) for the full list.

---

## 5. Add presets

Presets populate a picker shown when the user presses `p` inside a block editor.

```go
type myPresets struct{}

var serverPresets = map[string]ServerConfig{
    "minimal":    {Host: "localhost", Port: 8080},
    "production": {Host: "0.0.0.0", Port: 443, TLS: true},
}

func (myPresets) ListFields() []string { return []string{"server"} }

func (myPresets) ListPresets(field string) []string {
    if field != "server" {
        return nil
    }
    names := make([]string, 0, len(serverPresets))
    for name := range serverPresets {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}

func (myPresets) PresetYAML(field, name string) (string, error) {
    p, ok := serverPresets[name]
    if !ok {
        return "", fmt.Errorf("server preset %q not found", name)
    }
    out, err := yaml.Marshal(map[string]any{field: p})
    return string(out), err
}
```

Wire it in via `Config.Presets`. For presets shipped as embedded files, see [Presets & Metadata](presets-hints.md).

---

## 6. Add documentation commands (optional)

`docgenerator` generates Markdown reference tables from your struct and metadata - the same information shown in the hint panel, as static files or a browsable TUI.

```go
import "github.com/lucasassuncao/yedit/docgenerator"

gen := docgenerator.NewSchemaGenerator(docgenerator.WithMetadata(src))

// Browse docs in the terminal:
docs := gen.GenerateDocsInMemory(Config{})
docgenerator.RenderMarkdownDocsInTerminal(docs, "myapp")

// Write markdown files to disk:
names, err := gen.GenerateAllDocs(Config{}, "docs/reference")
if err != nil {
    log.Fatal(err)
}
docgenerator.GenerateIndex("docs/reference", names)
```

Wire these as subcommands in your CLI so users can run `myapp show-docs` and `myapp generate-docs`. See `examples/test/main.go` for a complete cobra integration.

---

## Recursive types

For self-referential structs (e.g. a filter that contains `Any []Filter`), use shared pointers in the metadata tree to avoid duplicating definitions:

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
anyNode.Children = filterChildren

src, err := metadata.Build(&Config{}, map[string]*metadata.Node{
    "filters": {Children: filterChildren},
})
```

`metadata.Build` is cycle-aware and handles shared pointers correctly.

---

## What's next

- [Schema Kinds Reference](schema-kinds-reference.md) - full type mapping table with YAML examples
- [Validators Reference](validators.md) - all 25+ built-in validators
- [Architecture](architecture.md) - package layout and import relationships
