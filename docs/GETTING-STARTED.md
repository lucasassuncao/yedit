# Getting Started

This guide walks you through the full integration path: define your config struct, attach metadata, run the editor, add validators, add presets, and wire up documentation commands.

---

## 1. Define the config struct

yamltui drives the editor from a Go struct using `yaml` tags. No other annotations are needed.

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

See [Schema Kinds Reference](SCHEMA-KINDS.md) for how each Go type maps to editor behavior (`KindObject`, `KindList`, `KindDictionary`, etc.).

---

## 2. Build metadata

`metadata.New` validates field metadata against your struct at startup - typos in field names surface immediately instead of silently never showing in the hint panel.

Each struct declares its own fields via a `Metadata()` method. Nested structs that also implement `Metadata()` have their children composed automatically.

```go
import (
    "github.com/lucasassuncao/yedit/editor"
    "github.com/lucasassuncao/yedit/metadata"
)

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
        "tls": {FieldMeta: editor.FieldMeta{
            Description: "Enable HTTPS. Requires a certificate and key.",
            Default:     "false",
        }},
    }
}

func (LoggingConfig) Metadata() map[string]*metadata.Node {
    return map[string]*metadata.Node{
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
    }
}

// Root struct lists its top-level blocks; children are composed automatically.
func (Config) Metadata() map[string]*metadata.Node {
    return map[string]*metadata.Node{
        "server":  {FieldMeta: editor.FieldMeta{Description: "HTTP server configuration.", Required: true}},
        "logging": {FieldMeta: editor.FieldMeta{Description: "Application logging configuration."}},
    }
}

src, err := metadata.New(Config{})
if err != nil {
    log.Fatal(err) // unknown field name, schema mismatch, etc.
}
```

`metadata.Node` embeds `editor.FieldMeta` for description, type label, default, required flag, allowed values (`OneOf`), and example snippet. The `Type` field is auto-filled from the Go type if left empty.

For structs from third-party packages that cannot implement `Metadata()`, use `metadata.NewFromTree` and pass the full tree manually. See [Metadata and Hints](METADATA-AND-HINTS.md) for details.

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

### Schema vs Metadata

`Schema` and `Metadata` are distinct concerns and it is worth understanding the split before wiring them together:

- **Schema** describes **shape**: which fields exist, what Go type they are, how they nest. It is derived automatically from your struct via reflection — you get it for free.
- **Metadata** describes **meaning**: what a field does, which values it accepts, whether it is required, what a good example looks like. You declare this manually because only you know the semantics.

Concretely: the schema tells the editor that `logging.level` is a string field inside a `logging` object. The metadata tells it that the only valid values are `"debug"`, `"info"`, `"warn"`, and `"error"`, and that the field is required. One you get automatically; the other you write once and both the hint panel and the `FromMetadata` validators use it.

```go
// Schema — derived automatically from the struct:
type LoggingConfig struct {
    Level string `yaml:"level"` // editor knows: string field, lives under "logging"
    File  string `yaml:"file"`  // editor knows: string field, lives under "logging"
}

// Metadata — you declare the semantics:
func (LoggingConfig) Metadata() map[string]*metadata.Node {
    return map[string]*metadata.Node{
        "level": {FieldMeta: editor.FieldMeta{
            Description: "Minimum log severity to emit.",
            Required:    true,
            OneOf:       []string{"debug", "info", "warn", "error"},
        }},
        "file": {FieldMeta: editor.FieldMeta{
            Description: "Path to the log file. Empty disables file output.",
        }},
    }
}
```

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

Wire it in via `Config.Presets`. For presets shipped as embedded files, see [Presets](PRESETS.md).

---

## 6. Add documentation commands (optional)

`docgenerator` generates Markdown reference tables from your struct and metadata - the same information shown in the hint panel, as static files or a browsable TUI.

```go
import "github.com/lucasassuncao/yedit/docgenerator"

gen := docgenerator.NewSchemaGenerator(docgenerator.WithMetadata(src))

// Browse docs in the terminal:
docs := gen.GenerateDocsInMemory([]docgenerator.Entry{{Config: Config{}, SplitStructs: true}})
docgenerator.RenderMarkdownDocsInTerminal(docs, "myapp")

// Write markdown files to disk:
files, err := gen.GenerateDocsForEach([]docgenerator.Entry{{Config: Config{}, DocsDir: "docs/reference", SplitStructs: true}})
if err != nil {
    log.Fatal(err)
}
docgenerator.GenerateIndex("docs/reference", files)
```

Wire these as subcommands in your CLI so users can run `myapp show-docs` and `myapp generate-docs`. See [Doc Generation](DOC-GENERATION.md) for the full API (including the single-call `Generate`/`GenerateInMemory` variants for structs that implement `MetadataProvider`) and `examples/test/main.go` for a complete cobra integration.

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

src, err := metadata.NewFromTree(&Config{}, map[string]*metadata.Node{
    "filters": {Children: filterChildren},
})
```

Both `metadata.New` and `metadata.NewFromTree` are cycle-aware and handle shared pointers correctly.

---

## What's next

- [Config Reference](CONFIG-REFERENCE.md) - every `editor.Config` field in one table
- [Schema Kinds Reference](SCHEMA-KINDS.md) - full type mapping table with YAML examples
- [Validators Reference](VALIDATORS.md) - all 25+ built-in validators
- [Undo & Redo](UNDO.md) - the two-level undo model and what is and isn't tracked
- [Themes](THEMES.md) - built-in themes and how to customize colors
- [Doc Generation](DOC-GENERATION.md) - generating Markdown reference docs and a TUI doc browser from your schema
- [Architecture](dev/ARCHITECTURE.md) - package layout and import relationships (for contributing to yedit itself)
