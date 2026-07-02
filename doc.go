// Package yedit provides reusable building blocks for TUI editors over
// structured YAML files.
//
// The library is composed of independent sub-packages:
//
//   - schema:       reflection over the client's Go structs (yaml tags only)
//   - metadata:     tree-based MetadataSource with strict schema validation
//   - document:     YAML state with block-level mutations, history, and parsing
//   - editor:       two-panel bubbletea TUI that ties the pieces together
//   - presets:      Source interface + struct-backed helpers (ForField, Combine) for per-field YAML snippets
//   - viewer:       read-only TUI to browse a preset Source
//   - docgenerator: markdown docs generated from a schema + MetadataSource, with a TUI browser
//   - theme:        palette and layout primitives (header, panels, two-column layout)
//   - alert:        modal alert/confirm component shared by the TUIs
//   - yamlnode:     query and navigation helpers over yaml.v3 node trees
//   - render:       small shared rendering helpers (glamour YAML fence)
//
// yedit is intentionally headless of any specific YAML schema. Clients pass
// a pointer to their own annotated struct and (optionally) a preset Source;
// the editor introspects the struct via the schema package and renders an
// add/edit/remove UI keyed by the canonical top-level order.
//
// See editor.Run and editor.Config for the main entry point.
package yedit
