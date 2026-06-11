# Validators

This document describes every built-in validator in `yedit/editor`, how paths are
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

| Validator | Rule |
|---|---|
| [`Required`](#required) | listed paths must be present and non-empty |
| [`RequiredFromSchema`](#requiredfromschema) | enforce the schema's `validate:"required"` markers |
| [`RequiredWith`](#requiredwith) | key requires another top-level key to be set |
| [`RequiredIf`](#requiredif) | key is required when another field has a given value |
| [`AtLeastOneOf`](#atleastoneof) | at least one of the listed keys must be present |
| [`ExactlyOneOf`](#exactlyoneof) | exactly one of the listed keys must be present |
| [`MutuallyExclusive`](#mutuallyexclusive) | at most one of the listed keys may be present |
| [`MutuallyExclusiveNested`](#mutuallyexclusivenested) | mutual exclusion at every occurrence of a key, recursively |
| [`AllOrNone`](#allornone) | listed keys must appear together or not at all |
| [`ValueOneOf`](#valueoneof) | value must be in a fixed allowed set |
| [`ValueInRange`](#valueinrange) | numeric/duration/size value must be within `[min, max]` |
| [`ValueMatches`](#valuematches) | value must match a regular expression |
| [`ValueHasPrefix`](#valuehasprefix--valuehassuffix) | value must start with a fixed prefix |
| [`ValueHasSuffix`](#valuehasprefix--valuehassuffix) | value must end with a fixed suffix |
| [`CrossFieldOrdered`](#crossfieldordered) | one field's value must be strictly less than another's |
| [`CountRange`](#countrange) | list/mapping must have between min and max entries |
| [`UniqueValues`](#uniquevalues) | scalar list items must not repeat |
| [`NoDuplicates`](#noduplicates) | struct list items must not repeat a given field's value |
| [`Deprecated`](#deprecated) | flag a field that should no longer be used |

---

## Path semantics

Most validators take dot-separated YAML paths (`server.tls.cert`). Three rules
apply consistently:

- **Sequence expansion** — when a path segment lands on a sequence, every item
  is checked. `categories.name` checks `name` inside each entry of the
  `categories` list, reporting violations as `categories[2].name`.
- **Dictionary expansion** — when a segment lands on a dict-style mapping
  (arbitrary keys, struct values), every value is checked.
- **Absent is not an error** — value validators (`ValueOneOf`, `ValueInRange`,
  `ValueMatches`, `CountRange`, …) report nothing when the path is absent or the
  scalar is empty. Combine them with `Required` when the field is mandatory.

Validators built from invalid arguments (a bad regex, dotted paths with
diverging parents, an inconsistent range) do not silently never fire — they
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

### RequiredFromSchema

```go
editor.RequiredFromSchema()
```

Enforces the schema's required markers (`validate:"required"` /
`jsonschema:"required"`) at validate/save time. Without it the marker is
display-only: the `*` in the tree and the "Required: yes" hint line do not
block saving.

A required field is only enforced where its parent exists. Sequence and
dictionary entries are checked individually. The editor wires the discovered
schema into this validator when the session starts; outside `editor.Run` it
reports nothing.

### RequiredWith

```go
editor.RequiredWith("service", "dockerComposeFile")
editor.RequiredWith("server.tls-key", "server.tls-cert")
```

Reports a violation when `key` is present but `parent` is not. Supports the
same two forms as `MutuallyExclusive`: plain keys are checked against the
document's top-level blocks, and dotted paths — both sharing the same parent
prefix — are checked inside every mapping reached by that parent, with
sequences and dict-style mappings expanded automatically. For symmetric pairs
(both or neither), prefer `AllOrNone`.

### RequiredIf

```go
// every servers[n] with protocol https needs its own tls-cert
editor.RequiredIf("servers.tls-cert", "servers.protocol", "https")
```

Reports a violation when `key` is absent but the field at `condPath` equals
`condValue`. When the two paths share the same parent prefix, the rule is
evaluated inside every mapping reached by that parent — each list entry is
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

### MutuallyExclusive

```go
// top-level keys
editor.MutuallyExclusive("image", "build", "dockerComposeFile")

// dotted paths — all must share the same parent prefix
editor.MutuallyExclusive(
    "categories.installers.source.filter.any",
    "categories.installers.source.filter.all",
)
```

Reports a violation when more than one of the listed keys is present at the
same time. Plain keys are checked against the document's top-level blocks.
Dotted paths must all share the same parent prefix; the validator navigates to
that parent — expanding sequences and dictionaries — and checks the leaf keys
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
`"0.5"`), `time.Duration` strings (`"24h"`), or size strings — and all three
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
with `prefix` / end with `suffix` — a simpler alternative to `ValueMatches`
when the rule is a fixed affix and no regex is needed.

---

## Cross-field

### CrossFieldOrdered

```go
editor.CrossFieldOrdered("retry.min-delay", "retry.max-delay")
```

Reports a violation when both paths are present but the value at `smallerPath`
is not strictly less than the value at `largerPath`. Values are compared as
plain numbers, durations, or sizes (same rules as `ValueInRange`); both sides
must be of the same kind.

When the two paths share the same parent prefix, the pair is compared inside
every mapping reached by that parent — each list entry's own min/max pair is
checked. Unrelated parents are resolved from the document root.

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
share the same value. Non-scalar items are skipped — use `NoDuplicates` to
deduplicate struct entries by one of their fields.

### NoDuplicates

```go
editor.NoDuplicates("servers", "name")
editor.NoDuplicates("categories.installers", "meta.name")
```

Reports a violation when two or more items in the sequence at `seqPath` share
the same value for `field`. Sequences and dict-style mappings along `seqPath`
are expanded automatically, and uniqueness is checked per reached list —
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
            // in.Raw    — document bytes, CRLF-normalised
            // in.Root   — parsed YAML root (nil when the document is invalid YAML)
            // in.Blocks — top-level blocks
            return nil
        }),
    },
})
```

Outside the editor, `editor.RunAll(validators, raw, blocks)` executes a set of
validators against a document and collects the violations — useful for CLI
`lint`-style commands that reuse the same rules.
