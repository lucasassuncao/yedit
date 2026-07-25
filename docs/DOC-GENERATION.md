# Doc Generation

`docgenerator` turns a Go struct and a `MetadataSource` into Markdown reference documentation - the same information shown in yedit's Hint/Example panel. Apps embedding yedit typically wire this as a `generate-docs` CLI subcommand that writes the files into the repository.

`docgenerator` depends on `editor` (for `MetadataSource`) and `schema` (for `Discover`), but not the other way around - wiring doc commands does not add weight to the editor itself.

---

## Entry

Every generation function takes `[]Entry`, one per struct you want documented:

```go
type Entry struct {
    Config         any    // struct pointer to document
    DocsDir        string // output directory (ignored by the *InMemory variants)
    SplitStructs   bool   // false: one page per entry, all sections inline. true: root summary page + one page per nested field
    RecursionLimit *int   // extra levels a self-referential type expands; nil uses schema.Discover's default (1)
}
```

`SplitStructs: true` emits one file per nested struct instead of inlining them, cross-linked by relative Markdown links. Two entries whose split children share a name would write to the same file; that is reported as a `duplicate docs page` error rather than silently overwriting.

## Writing files to disk

```go
// Struct implements MetadataProvider - one call builds metadata, generates, and writes index.md:
err := docgenerator.Generate("docs/", []docgenerator.Entry{
    {Config: Config{}, DocsDir: "docs/reference", SplitStructs: true},
})

// External MetadataSource - generate then index separately:
gen := docgenerator.NewSchemaGenerator(docgenerator.WithMetadata(src))
files, err := gen.GenerateDocsForEach([]docgenerator.Entry{
    {Config: Config{}, DocsDir: "docs/reference", SplitStructs: true},
})
if err != nil {
    log.Fatal(err)
}
docgenerator.GenerateIndex("docs/", files)
```

`GenerateIndex` writes a `README.md` at `baseDir` linking to every generated file, with paths computed relative to `baseDir` so it works correctly even when entries were written to different subdirectories.

## Preset examples

`WithExamples(relDir, pages)` adds an "Examples" section to any generated page whose lowercased name is in `pages`, linking to a sibling example file. Pair it with `GenerateExampleDocs`, which writes one Markdown file per preset field - each containing every preset's YAML - keyed by the same title used for the doc page:

```go
gen := docgenerator.NewSchemaGenerator(
    docgenerator.WithMetadata(src),
    docgenerator.WithExamples("../examples", map[string]bool{"category": true}),
)
files, err := gen.GenerateDocsForEach([]docgenerator.Entry{
    {Config: Config{}, DocsDir: "docs/reference", SplitStructs: true},
})

// titles maps a presets.Source field name to the display title used for its page.
_, err = docgenerator.GenerateExampleDocs("docs/examples", myPresetsSource, map[string]string{
    "category": "Category",
})
```

The file is named after the lowercased title (`category.md`) so it matches the doc page generated for the same type, and fields absent from `titles` (or with no presets) are skipped.

## Wiring as a CLI command

```go
var generateDocsCmd = &cobra.Command{
    Use: "generate-docs",
    RunE: func(cmd *cobra.Command, args []string) error {
        return docgenerator.Generate("docs/", []docgenerator.Entry{
            {Config: Config{}, DocsDir: "docs/reference", SplitStructs: true},
        })
    },
}
```

See `examples/test/main.go` for a complete, runnable cobra integration.
