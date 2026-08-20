# Doc Generation

`docgenerator` turns a Go struct and a `MetadataSource` into reference artifacts - the same information shown in yedit's Hint/Example panel. Apps embedding yedit typically wire this as a `generate-docs` CLI subcommand that writes the files into the repository.

`docgenerator` depends on `schema`, `spec`, `metadata` and `presets`. It does not depend on `editor`, so wiring doc commands adds no TUI dependencies - its only external dependency is `gopkg.in/yaml.v3`.

---

## Entry

`Generate` takes `[]Entry`, one per struct you want documented:

```go
type Entry struct {
    Config         any    // struct (or struct pointer) to document
    MarkdownDir    string // optional: overrides where this entry's markdown pages go
    SplitStructs   bool   // false: one page per entry, all sections inline. true: root summary page + one page per nested field
    RecursionLimit *int   // extra levels a self-referential type expands; nil uses schema.Discover's default (1)
}
```

`SplitStructs: true` emits one Markdown file per nested struct instead of inlining them, cross-linked by relative Markdown links. Two entries whose split children share a name would write to the same file; that is reported as a `duplicate docs page` error rather than silently overwriting. It affects Markdown only.

Output directories otherwise belong to the output options. `MarkdownDir` is the one exception, for layouts that group each documented struct in its own subdirectory:

```go
docgenerator.Generate(
    []docgenerator.Entry{
        {Config: Configuration{}, MarkdownDir: "docs/attributes/configuration"},
        {Config: Category{}, MarkdownDir: "docs/attributes/categories", SplitStructs: true},
    },
    docgenerator.WithMarkdown("docs/attributes"),
    docgenerator.WithIndex("docs/"),
)
```

It redirects Markdown but does not enable it: an entry with `MarkdownDir` set and no `WithMarkdown` option writes nothing. JSON Schema files are unaffected - they stay together under `WithJSONSchema`, which is the directory you point a language server at. Example cross-links are computed from each entry's own directory, so entries at different depths each get a correct relative path.

## Generating

Every output is opt-in. `Generate` writes exactly what the `With*` options select; calling it with no output option returns `ErrNoOutput` rather than silently doing nothing.

```go
files, err := docgenerator.Generate(
    []docgenerator.Entry{{Config: Config{}, SplitStructs: true}},
    docgenerator.WithMetadata(src),           // optional
    docgenerator.WithMarkdown("docs/reference"),
    docgenerator.WithJSONSchema("docs/schema"),
    docgenerator.WithExamples(presetsSrc, "docs/examples", map[string]string{
        "category": "Category",
    }),
    docgenerator.WithIndex("docs/"),
)
```

`Generate` returns the files it wrote, in order: Markdown, JSON Schema, examples, index.

```go
type GeneratedFile struct {
    Name string // display title, e.g. "Config"
    Path string // path as written, e.g. "docs/reference/config.md"
}
```

`WithMetadata` is optional. Without it, each entry whose `Config` implements `metadata.Provider` gets its own source via `metadata.New`, and entries that do not implement it are documented from structure alone.

## Preset examples

`WithExamples(src, dir, titles)` owns the whole example concern. `titles` maps a `presets.Source` field name to its display title, and two things are derived from that single map:

- the example pages to write, one per field, each containing every preset's YAML;
- which Markdown reference pages get an "Examples" link, matched on the lowercased title.

The relative link path is computed from the two directories rather than typed. Fields absent from `titles`, or with no presets, are skipped.

```go
docgenerator.WithExamples(myPresetsSource, "docs/examples", map[string]string{
    "category": "Category",
})
```

The file is named after the lowercased title (`category.md`) so it matches the reference page generated for the same type.

## Index

`WithIndex(dir)` writes a `README.md` at `dir` with two sections, "Available Configurations" and "Examples", linking to the Markdown reference and example pages. Paths are relative to `dir`, so it works when outputs were written to different subdirectories. A section with no files is omitted.

Generated `.schema.json` files are not listed: the index is a readable documentation index, not a manifest of build artifacts.

## JSON Schema

`WithJSONSchema(dir)` writes one draft 2020-12 schema per entry, named `<lowercased type name>.schema.json`. Point `yaml-language-server` at it to get completion and validation in the editor for the same file yedit edits:

```yaml
# yaml-language-server: $schema=./docs/schema/config.schema.json
```

Recursive types are represented with `$defs`/`$ref` rather than expanded, so the output does not depend on `Entry.RecursionLimit`:

```json
{
  "$defs": {
    "Node": {
      "properties": {
        "children": {
          "items": { "$ref": "#/$defs/Node" },
          "type": "array"
        }
      },
      "type": "object"
    }
  },
  "properties": {
    "tree": { "$ref": "#/$defs/Node" }
  },
  "type": "object"
}
```

`FieldMeta` maps onto keywords directly: `OneOf` to `enum`, `NotOneOf` to `not.enum`, `Pattern` to `pattern`, `MinLength`/`MaxLength` to `minLength`/`maxLength`, `MinCount`/`MaxCount`/`Unique` to `minItems`/`maxItems`/`uniqueItems`, `Required` to the parent's `required` array, `Deprecated` to `deprecated` plus a `$comment` carrying the migration hint.

`additionalProperties` is not emitted. yedit flags unknown keys at save time, but fields hidden from the schema or passed through untouched are invisible to `schema.Discover` and would otherwise show as false errors in your editor.

Constraints with no JSON Schema equivalent are surfaced in `description` rather than dropped:

- non-numeric `Min`/`Max` (durations, sizes) become `Min: 30s`;
- formats with no standard keyword (`semver`, `port`, `cidr`, `git-ref`, and any `FormatCustom`) become `Format: semver`. Formats that do map (`url`, `uuid`, `date`, `email`, `ipv4`, `ipv6`, `duration`, `host`, `fqdn`) become `format`, or `anyOf` when a field declares several.

## Wiring as a CLI command

```go
var generateDocsCmd = &cobra.Command{
    Use: "generate-docs",
    RunE: func(cmd *cobra.Command, args []string) error {
        _, err := docgenerator.Generate(
            []docgenerator.Entry{{Config: Config{}, SplitStructs: true}},
            docgenerator.WithMarkdown("docs/reference"),
            docgenerator.WithJSONSchema("docs/schema"),
            docgenerator.WithIndex("docs/"),
        )
        return err
    },
}
```

See `examples/test/main.go` for a complete, runnable cobra integration.
