# Validators

This document describes every built-in validator in `yamltui/editor`, how paths are
resolved, and how to write custom rules. Validators are pluggable rules executed
at validate/save time, registered through `editor.Config`:

```go
editor.Run(editor.Config{
    Validators: []editor.Validator{
        editor.Required("version"),
        editor.MutuallyExclusive("image", "build"),
        editor.ValueInRange("server.port", "1", "65535"),
    },
})
```

Each validator returns zero or more `Violation`s. A `Violation` carries the
dot-separated YAML `Path` to the offending node (empty for document-wide rules)
and a human-readable `Message`.

---

## Quick reference

**FromMetadata** — per-field constraints driven by `FieldMeta`:

| Validator | `FieldMeta` fields enforced |
|---|---|
| [`RequiredFromMetadata`](#the-frommetadata-family) | `Required` |
| [`OneOfFromMetadata`](#the-frommetadata-family) | `OneOf` |
| [`NotOneOfFromMetadata`](#the-frommetadata-family) | `NotOneOf` |
| [`RangeFromMetadata`](#the-frommetadata-family) | `Min`, `Max` |
| [`PatternFromMetadata`](#the-frommetadata-family) | `Pattern` |
| [`LengthFromMetadata`](#the-frommetadata-family) | `MinLength`, `MaxLength` |
| [`CountFromMetadata`](#the-frommetadata-family) | `MinCount`, `MaxCount` |
| [`UniqueFromMetadata`](#the-frommetadata-family) | `Unique` |
| [`FormatFromMetadata`](#the-frommetadata-family) | `Formats` |
| [`DeprecatedFromMetadata`](#the-frommetadata-family) | `Deprecated` |

**Cross-field** — explicit path rules:

| Validator | Signature | Rule |
|---|---|---|
| [`Required`](#required) | `(paths ...string)` | paths must be present and non-empty |
| [`RequiredWith`](#requiredwith) | `(key, parent string)` | `key` required when `parent` is present |
| [`RequiredIf`](#requiredif) | `(key, condPath, condValue string)` | `key` required when `condPath == condValue` |
| [`MutuallyExclusive`](#mutuallyexclusive) | `(keys ...string)` | at most one of `keys` may be present |
| [`MutuallyExclusiveNested`](#mutuallyexclusivenested) | `(scopedPath string, keys ...string)` | mutual exclusion at every occurrence of a key, recursively |
| [`MutuallyExclusiveGroupsNested`](#mutuallyexclusivegroupsnested) | `(scopedPath string, groups ...[]string)` | N groups of fields: no two groups may have keys present at the same mapping, recursively |
| [`AtLeastOneOf`](#atleastoneof) | `(keys ...string)` | at least one of `keys` must be present |
| [`ExactlyOneOf`](#exactlyoneof) | `(keys ...string)` | exactly one of `keys` must be present |
| [`AllOrNone`](#allornone) | `(keys ...string)` | all of `keys` present or none |
| [`CrossFieldOrdered`](#crossfieldordered) | `(smallerPath, largerPath string)` | `smaller < larger` (numeric/duration/size) |
| [`CrossFieldOrderedNested`](#crossfieldorderednested) | `(scopedPath, smallerLeaf, largerLeaf string)` | `smaller < larger` at every occurrence of a key, recursively |
| [`NoDuplicates`](#noduplicates) | `(seqPath, field string)` | field values across a list are unique |
| [`ValueOneOf`](#valueoneof) | `(path string, allowed ...string)` | value at path must be in `allowed` |
| [`ValueInRange`](#valueinrange) | `(path, minVal, maxVal string)` | value at path within `[min, max]` |
| [`ValueMatches`](#valuematches) | `(path, pattern string)` | value at path matches a regular expression |
| [`ValueHasPrefix`](#valuehasprefix--valuehassuffix) | `(path, prefix string)` | value at path starts with `prefix` |
| [`ValueHasSuffix`](#valuehasprefix--valuehassuffix) | `(path, suffix string)` | value at path ends with `suffix` |
| [`CountRange`](#countrange) | `(path string, minCount, maxCount int)` | list/mapping has between min and max entries |
| [`UniqueValues`](#uniquevalues) | `(seqPath string)` | scalar list items must not repeat |
| [`Deprecated`](#deprecated) | `(path, message string)` | field is flagged as deprecated |

**Escape hatch:**

| Validator | Rule |
|---|---|
| [`ValidatorFunc`](#custom-validators) | any custom logic via an anonymous function |

---

## Choosing a family

**FromMetadata** requires a `MetadataSource` in `editor.Config.Metadata`. Build one with `metadata.New(v)` when your structs implement `metadata.MetadataProvider` (the `Metadata()` method), or `metadata.NewFromTree(schemaPtr, tree)` when the root struct is from a third-party package that cannot implement the interface.

**Explicit** validators need nothing — no `MetadataSource`, no `Metadata()` method, no wiring step. They are the only option when structs cannot implement `MetadataProvider`.

Several explicit validators are per-field equivalents of their `FromMetadata` counterparts: `Required` ↔ `RequiredFromMetadata`, `ValueOneOf` ↔ `OneOfFromMetadata`, `ValueInRange` ↔ `RangeFromMetadata`, `ValueMatches` ↔ `PatternFromMetadata`, `CountRange` ↔ `CountFromMetadata`, `UniqueValues` ↔ `UniqueFromMetadata`, `Deprecated` ↔ `DeprecatedFromMetadata`. Use `FromMetadata` when you have a `MetadataSource`; use the explicit form otherwise.

`ValueHasPrefix` and `ValueHasSuffix` have no `FromMetadata` equivalent because `PatternFromMetadata` already covers them (`Pattern: "^prefix"` / `"suffix$"`).

`AtLeastOneOf`, `ExactlyOneOf`, `RequiredIf`, `AllOrNone`, and the `MutuallyExclusive*`/`CrossFieldOrdered*` family have no `FromMetadata` equivalent — they are inherently cross-field rules and cannot be expressed in per-field metadata.

---

## Path semantics

Most validators take dot-separated YAML paths (`server.tls.cert`). Three rules
apply consistently:

- **Sequence expansion** - when a path segment lands on a sequence, every item
  is checked. `categories.name` checks `name` inside each entry of the
  `categories` list, reporting violations as `categories[2].name`.
- **Dictionary expansion** - when a segment lands on a dict-style mapping
  (arbitrary keys, struct values), every value is checked.
- **Absent is not an error** - value validators (`ValueOneOf`, `ValueInRange`,
  `ValueMatches`, `CountRange`, …) report nothing when the path is absent or the
  scalar is empty. Combine them with `Required` when the field is mandatory.

Validators built from invalid arguments (a bad regex, dotted paths with
diverging parents, an inconsistent range) do not silently never fire - they
report the misconfiguration as a violation on every run, so the mistake
surfaces on the first validate.

---

## Presence

### Required

```go
editor.Required("version")          // top-level, unconditional
editor.Required("categories.name")  // every category entry needs "name"
```

Reports a violation when any of the given paths is absent or holds an
empty/null scalar. A non-scalar value (mapping or sequence) counts as present.

A path with no dots is required unconditionally at the document root. A dotted
path is conditional: the leaf is only required where its parent exists, so a
required field inside an optional block is not reported while the block is
absent.

### The FromMetadata family

Field constraints declared in the `MetadataSource` (`FieldMeta`) are enforced at
validate/save time by a family of validators - declare once, the hint panel
displays it and the save enforces it:

```go
editor.Run(editor.Config{
    Metadata: src, // e.g. built with metadata.New or metadata.NewFromTree
    Validators: []editor.Validator{
        editor.RequiredFromMetadata(),
        editor.OneOfFromMetadata(),
        editor.NotOneOfFromMetadata(),
        editor.RangeFromMetadata(),
        editor.PatternFromMetadata(),
        editor.LengthFromMetadata(),
        editor.CountFromMetadata(),
        editor.UniqueFromMetadata(),
        editor.FormatFromMetadata(),
        editor.DeprecatedFromMetadata(),
    },
})
```

| Constructor | FieldMeta fields | Semantics of |
|---|---|---|
| `RequiredFromMetadata()` | `Required` | `Required` |
| `OneOfFromMetadata()` | `OneOf` | `ValueOneOf` |
| `NotOneOfFromMetadata()` | `NotOneOf` | inverse of `ValueOneOf` |
| `RangeFromMetadata()` | `Min`, `Max` | `ValueInRange` |
| `PatternFromMetadata()` | `Pattern` | `ValueMatches` |
| `LengthFromMetadata()` | `MinLength`, `MaxLength` | string length bounds |
| `CountFromMetadata()` | `MinCount`, `MaxCount` | `CountRange` |
| `UniqueFromMetadata()` | `Unique` | `UniqueValues` |
| `FormatFromMetadata()` | `Formats` | runs each `Format.Validate` fn |
| `DeprecatedFromMetadata()` | `Deprecated` | `Deprecated` |

All share the same engine: the walk is guided by the discovered schema; for
every schema path the validator queries the `MetadataSource` -
`FieldMeta(block, "")` for a top-level block, `FieldMeta(block, "source.path")`
for nested fields, the same convention as the hint panel - and applies its
rule where the corresponding `FieldMeta` fields are set. Zero-valued fields
declare nothing. Sequence and dictionary entries are checked individually,
and a rule only fires where the field's parent exists (top-level required
blocks are always enforced).

Notes:

- Value rules (`OneOf`, `Range`, `Pattern`) follow the shared contract: an
  absent or empty value reports nothing - combine with `Required: true` when
  the field is mandatory.
- `Range` bounds may be one-sided (`Min` only = "at least", `Max` only = "at
  most"). Malformed or mixed-kind bounds, and invalid `Pattern` regexes, are
  reported as misconfiguration violations.
- `MinCount`/`MaxCount` both zero means no rule; `MinCount > 0` with
  `MaxCount == 0` means "at least MinCount, no upper bound".

The editor wires the schema and the configured `MetadataSource` in at session
start; outside `editor.Run`, or without a `MetadataSource`, the family is inert.

### RequiredWith

```go
editor.RequiredWith("service", "dockerComposeFile")
editor.RequiredWith("server.tls-key", "server.tls-cert")
```

Reports a violation when `key` is present but `parent` is not. Supports the
same two forms as `MutuallyExclusive`: plain keys are checked against the
document's top-level blocks, and dotted paths - both sharing the same parent
prefix - are checked inside every mapping reached by that parent, with
sequences and dict-style mappings expanded automatically. For symmetric pairs
(both or neither), prefer `AllOrNone`.

### RequiredIf

```go
// every servers[n] with protocol https needs its own tls-cert
editor.RequiredIf("servers.tls-cert", "servers.protocol", "https")
```

Reports a violation when `key` is absent but the field at `condPath` equals
`condValue`. When the two paths share the same parent prefix, the rule is
evaluated inside every mapping reached by that parent - each list entry is
checked against its own condition value. Paths with unrelated parents are both
resolved from the document root.

---

## Key combinations

### AtLeastOneOf

```go
editor.AtLeastOneOf("image", "build")
editor.AtLeastOneOf("auth.token", "auth.password")
```

Reports a violation when none of the listed keys is present. Supports the same
two forms as `MutuallyExclusive` (top-level keys, or dotted paths sharing a
parent). The rule only fires where the parent mapping exists.

### ExactlyOneOf

```go
editor.ExactlyOneOf("image", "build", "dockerComposeFile")
editor.ExactlyOneOf("source.git", "source.local")
```

Reports a violation when none or more than one of the listed keys is present.
Supports the same two forms as `MutuallyExclusive` (top-level keys, or dotted
paths sharing a parent). The rule only fires where the parent mapping exists.

| Variant | Scope | Unit checked | Use when |
|---|---|---|---|
| `MutuallyExclusive` | flat: document root or one fixed parent | individual keys | keys conflict at a single, known level |
| `MutuallyExclusiveNested` | recursive walk from a scoped root | individual keys | the same key repeats at unpredictable depths (recursive schemas) |
| `MutuallyExclusiveGroupsNested` | recursive walk from a scoped root | two groups of keys | two *families* of fields conflict — any key from groupA with any key from groupB |

### MutuallyExclusive

```go
// top-level keys
editor.MutuallyExclusive("image", "build", "dockerComposeFile")

// dotted paths - all must share the same parent prefix
editor.MutuallyExclusive(
    "categories.installers.source.filter.any",
    "categories.installers.source.filter.all",
)
```

Reports a violation when more than one of the listed keys is present at the
same time. Plain keys are checked against the document's top-level blocks.
Dotted paths must all share the same parent prefix; the validator navigates to
that parent - expanding sequences and dictionaries - and checks the leaf keys
inside every mapping it reaches.

### MutuallyExclusiveNested

```go
// fire at every mapping under a key named "filter", anywhere in the document
editor.MutuallyExclusiveNested("filter", "any", "all")

// scoped: only within this subtree (preferred when key names repeat)
editor.MutuallyExclusiveNested("categories.installers.source.filter", "any", "all")
```

Walks the YAML tree and fires at every mapping whose direct parent key is the
last segment of `scopedPath`, checking that at most one of `keys` is present.
Use this instead of `MutuallyExclusive` for constraints that must hold at every
occurrence of a key regardless of depth (e.g. recursive schemas).

### MutuallyExclusiveGroupsNested

```go
// two groups: composite (union/intersect) and leaf (path/name) cannot coexist
editor.MutuallyExclusiveGroupsNested(
    "pipeline.rule",
    []string{"union", "intersect"},
    []string{"path", "name"},
)

// three groups: at most one of image / build / compose may be present
editor.MutuallyExclusiveGroupsNested(
    "services.container",
    []string{"image"},
    []string{"build"},
    []string{"compose"},
)
```

Uses the same recursive walk as `MutuallyExclusiveNested`, but instead of
checking individual keys it checks N groups: a violation is reported for every
pair of groups that both have at least one key present in the same mapping.
With two groups that means one possible violation; with three groups up to
three (one per pair).

Use this for schemas where families of fields are mutually exclusive at a given
level. To cover violations at every nesting depth, register one validator per
scope that can contain violations:

```go
editor.MutuallyExclusiveGroupsNested("pipeline.rule",           groupA, groupB),
editor.MutuallyExclusiveGroupsNested("pipeline.rule.union",     groupA, groupB),
editor.MutuallyExclusiveGroupsNested("pipeline.rule.intersect", groupA, groupB),
```

### AllOrNone

```go
editor.AllOrNone("tls-cert", "tls-key")
editor.AllOrNone("server.tls-cert", "server.tls-key")
```

Reports a violation when only some of the listed keys are present: they must
appear together or not at all (e.g. a TLS cert/key pair). Supports the same two
forms as `MutuallyExclusive` (top-level keys, or dotted paths sharing a
parent).

---

## Values

### ValueOneOf

```go
editor.ValueOneOf("log.level", "debug", "info", "warn", "error")
```

Reports a violation when the field at `path` exists but its value is not in the
allowed set. A mapping or sequence at the path is flagged as "expected a scalar
value".

### ValueInRange

```go
editor.ValueInRange("server.port", "1", "65535")
editor.ValueInRange("filter.max-age", "1h", "8760h")
editor.ValueInRange("cache.max-size", "1MB", "2GiB")
```

Reports a violation when the scalar at `path` is present but outside the
inclusive `[min, max]` range. Bounds and value may be plain numbers (`"1"`,
`"0.5"`), `time.Duration` strings (`"24h"`), or size strings - and all three
must be of the same kind. Size suffixes follow their standard meaning:
`KB`/`MB`/`GB`/`TB` are decimal (powers of 1000) and `KiB`/`MiB`/`GiB`/`TiB`
are binary (powers of 1024).

### ValueMatches

```go
editor.ValueMatches("version", `^\d+\.\d+\.\d+$`)
```

Reports a violation when the scalar at `path` is present but does not match the
regular expression. An invalid pattern is itself reported as a violation.

### ValueHasPrefix / ValueHasSuffix

```go
editor.ValueHasPrefix("image", "registry.example.com/")
editor.ValueHasSuffix("output", ".yaml")
```

Report a violation when the scalar at `path` is present but does not start
with `prefix` / end with `suffix` - a simpler alternative to `ValueMatches`
when the rule is a fixed affix and no regex is needed.

---

## Cross-field

| Variant | Scope | Use when |
|---|---|---|
| `CrossFieldOrdered` | explicit sibling paths (flat or one fixed parent) | min/max constraint at a single, known level |
| `CrossFieldOrderedNested` | recursive walk from a scoped root | the same min/max block repeats at unpredictable depths (recursive schemas) |

### CrossFieldOrdered

```go
editor.CrossFieldOrdered("retry.min-delay", "retry.max-delay")
```

Reports a violation when both paths are present but the value at `smallerPath`
is not strictly less than the value at `largerPath`. Values are compared as
plain numbers, durations, or sizes (same rules as `ValueInRange`); both sides
must be of the same kind.

When the two paths share the same parent prefix, the pair is compared inside
every mapping reached by that parent - each list entry's own min/max pair is
checked. Unrelated parents are resolved from the document root.

### CrossFieldOrderedNested

```go
// age.min < age.max at every "age" mapping anywhere in the tree
editor.CrossFieldOrderedNested("pipeline.rule.age", "min", "max")
```

Uses the same recursive walk as `MutuallyExclusiveNested` to find every mapping
whose direct parent key matches the last segment of `scopedPath`, then checks
that `smallerLeaf < largerLeaf` inside it. Values follow the same comparison
rules as `CrossFieldOrdered` (numeric, duration, or size).

Use this instead of `CrossFieldOrdered` when the min/max constraint must hold
at every occurrence of a key regardless of nesting depth - for example, in
recursive filter schemas where the same `age` block can appear at the root, inside
`any[i]`, inside `any[i].all[j]`, and so on.

---

## Collections

### CountRange

```go
editor.CountRange("workers", 1, 10)
editor.CountRange("categories", 1, -1) // at least one, no upper bound
```

Reports a violation when the collection at `path` has fewer than `minCount` or
more than `maxCount` entries. `maxCount < 0` means no upper bound. Sequences
count items; mappings count keys.

### UniqueValues

```go
editor.UniqueValues("tags")
```

Reports a violation when two or more scalar items in the sequence at `seqPath`
share the same value. Non-scalar items are skipped - use `NoDuplicates` to
deduplicate struct entries by one of their fields.

### NoDuplicates

```go
editor.NoDuplicates("servers", "name")
editor.NoDuplicates("categories.installers", "meta.name")
```

Reports a violation when two or more items in the sequence at `seqPath` share
the same value for `field`. Sequences and dict-style mappings along `seqPath`
are expanded automatically, and uniqueness is checked per reached list -
entries in different lists may repeat. `field` may be a dotted path inside
each item.

---

## Lifecycle

### Deprecated

```go
editor.Deprecated("dockerFile", "use build.dockerfile instead")
```

Reports a violation whenever `path` is present, carrying a migration hint for
the user. Combine with `Config.NoValidateOnSave` to make it a non-blocking
warning instead of a save blocker.

---

## Custom validators

Any type implementing the `Validator` interface can be registered. For one-off
rules, `ValidatorFunc` adapts a plain function:

```go
editor.Run(editor.Config{
    Validators: []editor.Validator{
        editor.ValidatorFunc(func(in editor.ValidationInput) []editor.Violation {
            // in.Raw    - document bytes, CRLF-normalised
            // in.Root   - parsed YAML root (nil when the document is invalid YAML)
            // in.Blocks - top-level blocks
            return nil
        }),
    },
})
```

---

## Using validators outside the TUI (`Wire` + `RunAll`)

For CLI lint-style commands that reuse the same rules without opening the
editor, use `Wire` followed by `RunAll`:

```go
wired := editor.Wire(MyValidators, editor.Config{
    Schema:   &MySchema{},
    Metadata: hints,
})
violations := editor.RunAll(wired, raw, blocks)
```

### Why `Wire` exists — and why `RunAll` requires `WiredValidators`

`RunAll` accepts `WiredValidators`, not `[]Validator` directly. This is
intentional: FromMetadata validators (`RequiredFromMetadata`, `OneOfFromMetadata`,
etc.) hold unexported `defs` and `hints` fields that must be populated before
they can run. Without `Wire`, they silently return zero violations.

By requiring `WiredValidators` as the argument type, the compiler prevents
the "forgot to wire" mistake at compile time rather than letting it manifest
as a silent test gap at runtime.

Inside `editor.Run`, wiring happens automatically — `newModel` calls `Wire`
once and stores the result in the model. You only need `Wire` explicitly when
operating outside a TUI session.

### Safety properties of `Wire`

- **The original slice is never modified.** `Wire` allocates a new slice and
  copies each `*metadataRuleValidator` struct before injecting `defs`/`hints`.
  The same global validator slice can be passed to `Wire` from multiple call
  sites or goroutines without data races.
- **Calling `Wire` with `Config{Schema: nil}` is safe** — it wraps the slice
  as-is and explicit validators run normally. Only FromMetadata validators
  remain inert (same behaviour as before wiring).
- **`Wire` is cheap to call repeatedly** — schema discovery (`schema.Discover`)
  runs once per `Wire` call, not once per `RunAll` call. For high-frequency
  paths (e.g. inside a loop), call `Wire` once outside the loop and reuse the
  `WiredValidators` handle.
