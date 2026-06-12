# Architecture

How the yedit packages fit together and why they are split the way they are.

---

## Folder structure

```
yedit/
├── editor/             - public API: Config, Run, FieldMeta, MetadataSource, Validator, …
├── metadata/           - BuildWithTree: validates a Node tree; BuildFromProvider: auto-composes from MetadataProvider structs
├── schema/             - schema.Discover: reflects a Go struct into a []FieldDef tree
├── document/           - raw YAML bytes, block list, undo/redo history
├── presets/            - presets.FromFS: embed.FS-backed PresetSource
├── docgenerator/       - generates Markdown reference tables; TUI doc browser
├── theme/              - color palette, layout helpers
├── viewer/             - reusable list+viewport model (used by docgenerator TUI)
├── internal/
│   ├── alert/          - modal alert overlay (bubbletea component)
│   └── yamlnode/       - *yaml.Node helpers shared by editor sub-packages
├── examples/
│   └── test/           - runnable example exercising every schema pattern and Config option
└── docs/               - reference documentation
```

## Package map

### Typical import graph

```
your app
  └── editor          ← Config, Run, MetadataSource, Validator, …
        ├── schema    ← Discover (schema struct → []FieldDef tree)
        ├── document  ← raw YAML mutations + undo stack
        └── (internal packages)

  └── metadata        ← BuildWithTree / BuildFromProvider (→ MetadataSource)
        └── editor    ← FieldMeta, MetadataSource

  └── docgenerator    ← SchemaGenerator, RenderMarkdownDocsInTerminal
        ├── editor    ← MetadataSource
        └── schema    ← Discover

  └── presets         ← FromFS (embed.FS-backed PresetSource)
```

`docgenerator` imports `editor` (for `MetadataSource`) but `editor` does not import `docgenerator` - the dependency is one-way, so wiring doc commands does not add weight to the editor itself.

---

## editor

The main entry point. `editor.Run` starts a bubbletea program that manages:

- A **list view** - left panel shows top-level YAML blocks (ADDED / AVAILABLE sections).
- A **block editor** - opens when the user selects a block; owns a field tree (left) and YAML pane (right).
- An **editor stack** - drill-in (Enter on a nested field) pushes a new `blockEditState` onto the stack; drill-out (Esc) pops it. The single `editRoot *yaml.Node` holds all edits until Ctrl+S commits them to `document.Document`.
- A **hint panel** - shown when `EnableHints` is set; renders `FieldMeta` from `Config.Metadata` for the focused field.

`editor.Config` is the integration surface. See `editor/config.go` for the full field list.

### MetadataSource

```go
type MetadataSource interface {
    FieldMeta(blockKey, fieldPath string) FieldMeta
}
```

`MetadataSource` is the single source of truth for both the hint panel and the `FromMetadata` validator family. Declare constraints once (`Required: true`, `OneOf: [...]`, etc.) - the panel displays them and the validators enforce them.

The recommended implementation is `metadata.BuildFromProvider` (when your struct implements `MetadataProvider`) or `metadata.BuildWithTree` (for manual trees), both of which validate field names against the struct at startup.

### Validators

Validators implement `editor.Validator` and are called before every save via `RunAll`. Two families:

- **FromMetadata** (`RequiredFromMetadata`, `OneOfFromMetadata`, etc.) - walk the schema tree and query `MetadataSource` for each field. Wire in at session start via `editor.Config.Validators`.
- **Explicit** (`Required`, `ValueOneOf`, `ValueInRange`, `MutuallyExclusive`, etc.) - self-contained; work from raw bytes and the block list. Used for one-off or cross-field rules.

---

## metadata

Two construction paths, both validating field names against the struct at startup:

- **`BuildFromProvider(v any)`** - the recommended path. The struct implements `MetadataProvider` (returns `map[string]*Node` for its direct fields). Nested structs that also implement `MetadataProvider` have their children composed automatically via reflection. Cycles (e.g. `Filter.Any []Filter`) are resolved through shared-pointer caching. Returns an error if any `yaml`-tagged field is undocumented.

- **`BuildWithTree(schemaPtr any, tree map[string]*Node)`** - the manual path. Pass a fully-constructed tree; useful for structs you don't own or when child metadata is built programmatically.

`metadata.Node` embeds `editor.FieldMeta` and adds `Children map[string]*Node`. Shared pointers in `Children` model recursive types without infinite loops.

---

## schema

`schema.Discover(ptr)` reflects a Go struct into `[]schema.FieldDef`:

```go
type FieldDef struct {
    YAMLName string
    Kind      Kind       // KindPrimitive | KindObject | KindList | KindDictionary | KindVariant | KindAny
    Scalar    string     // concrete scalar type for KindPrimitive
    Children  []FieldDef // nested struct fields
    // …
}
```

`Kind` is the driving concept: `KindObject` gets a field tree, `KindList`/`KindDictionary` with children get a `[N]` navigator, everything else gets the raw YAML pane. See [Schema Kinds Reference](schema-kinds-reference.md) for the full mapping.

The schema package has no dependency on `editor` - it can be used standalone (e.g. by `docgenerator`).

---

## document

`document.Document` owns the raw YAML bytes. It divides the file into top-level **blocks** (`Block{Key, StartLine, EndLine}`) without full YAML parsing. All mutations splice raw lines:

| Method | Effect |
|---|---|
| `Insert(snippet)` | Append a new block, positioned by `knownOrder` |
| `Replace(key, snippet)` | Remove then insert the updated version |
| `Remove(key)` | Delete the block entirely |

Every mutation snapshots the current bytes to a history stack first - `doc.Undo()` / `doc.Redo()` restore those snapshots. This is the **document-level undo**, separate from the in-editor node-level undo inside `blockEditState`.

A round-trip guard validates each `Insert`/`Replace` by re-parsing the stored block and comparing its structure against the submitted snippet. Mismatches roll back the mutation before it reaches disk.

---

## docgenerator

Generates Markdown reference tables from a Go struct and a `MetadataSource`. Used for `show-docs` (TUI browser) and `generate-docs` (write files to disk) CLI subcommands.

```go
// In-memory TUI browser - struct implements MetadataProvider:
ds, _ := docgenerator.GenerateInMemory([]docgenerator.Entry{
    {Config: Config{}, SplitStructs: true},
})
docgenerator.RenderMarkdownDocsInTerminal(ds, "myapp")

// In-memory TUI browser - external MetadataSource:
gen := docgenerator.NewSchemaGenerator(docgenerator.WithMetadata(src))
ds = gen.GenerateDocsInMemory([]docgenerator.Entry{
    {Config: Config{}, SplitStructs: true},
})
docgenerator.RenderMarkdownDocsInTerminal(ds, "myapp")

// On disk - struct implements MetadataProvider:
docgenerator.Generate("docs/", []docgenerator.Entry{
    {Config: Config{}, DocsDir: "docs/reference", SplitStructs: true},
})

// On disk - external MetadataSource:
files, _ := gen.GenerateDocsForEach([]docgenerator.Entry{
    {Config: Config{}, DocsDir: "docs/reference", SplitStructs: true},
})
docgenerator.GenerateIndex("docs/", files)
```

`Entry.SplitStructs` controls the output shape. When `false` (default), one page is produced per entry with all nested sections inline. When `true`, the entry produces a root summary page whose table links to separate per-field pages; `DocSet.Children` records the parent→children relationship so the TUI can display hierarchy and wire `[1-9]` navigation.

`docgenerator` depends on `editor` (for `MetadataSource`) and `schema` (for `Discover`), but not the other way around - no import cycle.

---

## presets

`presets.FromFS(fs, dir)` returns a `PresetSource` backed by an `embed.FS`. Expected layout:

```
presets/
  server/
    minimal.yaml
    production.yaml
```

Each file is a YAML mapping keyed by the block name. For struct-backed presets (marshaled at runtime), implement `editor.PresetSource` directly - see [Presets & Metadata](presets-hints.md).

---

## Two-level undo

yedit maintains two independent undo stacks:

| Level | Scope | Keys |
|---|---|---|
| Block editor (`blockEditState`) | In-memory `*yaml.Node` changes while a block is open | Ctrl+U / Ctrl+Y |
| Document (`document.Document`) | Raw byte snapshots of committed saves and removals | Ctrl+U / Ctrl+Y in list view |

Ctrl+U while a block editor is open never falls through to the document level. Closing a block (Esc without saving) discards in-editor changes without touching the document history.
