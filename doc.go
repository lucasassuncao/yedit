// Package yedit provides reusable building blocks for TUI editors over
// structured YAML files.
//
// The library is composed of independent sub-packages:
//
//   - schema:     reflection over the client's Go structs (yaml tag required; validate and jsonschema_description optional)
//   - document:   YAML state with block-level mutations, history, and parsing
//   - editor:     two-panel bubbletea TUI that ties the pieces together
//   - presets:    Source interface + fs.FS-backed implementation for per-field YAML snippets
//   - viewer:     read-only TUI to browse a preset Source
//   - theme:      palette and layout primitives (header, panels, two-column layout)
//   - components: bubbletea widgets (alert, picker) that depend only on theme
//
// yedit is intentionally headless of any specific YAML schema. Clients pass
// a pointer to their own annotated struct and (optionally) a preset Source;
// the editor introspects the struct via the schema package and renders an
// add/edit/remove UI keyed by the canonical top-level order.
//
// See editor.Run and editor.Config for the main entry point.
package yedit
