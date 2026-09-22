# Doc Generation

Generating reference documentation from your schema lives in a separate module:
**[docgen](https://github.com/lucasassuncao/docgen)**.

It reads the same `Metadata()` method the editor does, so the descriptions,
defaults, constraints and examples shown in the hint panel are the ones that end
up in the docs. Neither module imports the other.

```go
import "github.com/lucasassuncao/docgen"

files, err := docgen.Generate(
    []docgen.Entry{{Config: Config{}, SplitStructs: true}},
    docgen.WithMarkdown("docs/reference"),
    docgen.WithJSONSchema("docs/schema"),
    docgen.WithExamples(myPresets, "docs/examples", titles),
    docgen.WithIndex("docs/reference"),
)
```

Wire it as a `generate-docs` subcommand in your CLI.

## What it writes

| option | output |
|---|---|
| `WithMarkdown(dir)` | one reference page per entry, plus one per split child |
| `WithJSONSchema(dir)` | `<type>.schema.json`, draft 2020-12, recursion as `$defs`/`$ref` |
| `WithExamples(src, dir, titles)` | one page per preset field, linked from the reference pages |
| `WithIndex(dir)` | `README.md` linking the generated pages |

Every output is opt-in; calling `Generate` with none is an error.

`WithExamples` takes anything with `ListFields`, `ListPresets` and `PresetYAML`,
which `yedit/presets.Source` satisfies - pass the same source you gave
`editor.Config.BlockPresets`.

See the [docgen README](https://github.com/lucasassuncao/docgen) for the full
key list and for supplying structure or metadata that does not come from a Go
struct.
