// Package yedit provides reusable building blocks for TUI editors over
// structured YAML files.
//
// It is only a library, with no standalone binary: a program uses it to build
// a schema-aware editor for its own config into itself.
//
// The library is composed of independent sub-packages:
//
//   - schema:       reflection over the client's Go structs (yaml tags only)
//   - spec:         the shared vocabulary (FieldMeta, Violation, Validator, Format)
//   - metadata:     tree-based MetadataSource with strict schema validation
//   - validate:     the validation rules, usable without importing the TUI
//   - report:       renders violations for humans (tree, table, plain) and CI (JSON)
//   - document:     YAML state with block-level mutations, history, and parsing
//   - editor:       two-panel bubbletea TUI that ties the pieces together
//   - presets:      Source interface + struct-backed helpers (ForField, Combine) for per-field YAML snippets
//   - viewer:       read-only TUI to browse a preset Source
//   - keys:         the bindings the list, tree and browser match against
//   - blocklist:    the root list, projected onto bezel/list
//   - fieldtree:    the block editor's field tree, projected onto bezel/tree
//   - yamlnode:     query and navigation helpers over yaml.v3 node trees
//   - render:       small shared rendering helpers (glamour YAML fence)
//   - trace:        JSONL session recorder behind editor.Config.Trace.Dump
//
// The chrome - theme, panels, header, legend, status, modals - and the generic
// widgets (list, tree, browser, animation) come from
// github.com/lucasassuncao/bezel; import bezel/theme to pick a theme.
//
// yedit is intentionally headless of any specific YAML schema. Clients pass
// a pointer to their own annotated struct and (optionally) a preset Source;
// the editor introspects the struct via the schema package and renders an
// add/edit/remove UI keyed by the canonical top-level order.
//
// See editor.Run and editor.Config for the main entry point.
package yedit
