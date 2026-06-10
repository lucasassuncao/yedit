// Package editor provides the bubbletea TUI for editing a YAML file driven by
// a struct-based schema and a preset source.
package editor

import (
	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/theme"
)

// ─── Presets ─────────────────────────────────────────────────────────────────

// PresetSource supplies YAML preset snippets keyed by (field, preset name).
// The editor uses it to populate the preset picker and to seed the YAML editor
// when a block is opened. Returning an empty slice from ListFields disables the
// preset picker for that session.
type PresetSource interface {
	// ListFields returns the field names that have at least one preset.
	ListFields() []string

	// ListPresets returns the preset names available for the given field,
	// or an empty slice if the field has no presets.
	ListPresets(field string) []string

	// PresetYAML returns the YAML snippet for (field, name) or an error if
	// either is unknown.
	PresetYAML(field, name string) (string, error)
}

// PresetFunc adapts a plain function to the PresetSource interface when only
// a single preset per field is needed and a full struct would be boilerplate:
//
//	editor.Run(editor.Config{
//	    Presets: editor.PresetFunc(func(field, name string) (string, error) {
//	        // return the YAML snippet for (field, name) ...
//	        return "", nil
//	    }),
//	})
//
// ListFields and ListPresets return nil; the picker will not appear.
// Use a struct implementing PresetSource directly when the picker is needed.
type PresetFunc func(field, name string) (string, error)

func (f PresetFunc) ListFields() []string                          { return nil }
func (f PresetFunc) ListPresets(_ string) []string                 { return nil }
func (f PresetFunc) PresetYAML(field, name string) (string, error) { return f(field, name) }

// ─── Hints ───────────────────────────────────────────────────────────────────

// FieldMeta carries display metadata for a single field in the Hint/Example
// panel. Fields at their zero value are omitted from the rendered output.
// HintSource is the sole authority: YEDIT never auto-populates any FieldMeta
// field from struct tags. If no HintSource is configured, the hint panel
// shows only a generated example.
type FieldMeta struct {
	Description string
	Type        string // human-readable Go type: "string", "bool", "int", "[]string", "duration", "object", etc.
	Required    bool
	Default     string
	OneOf       []string
	Example     string // YAML snippet shown verbatim in the Example section
}

// HintSource provides per-field display metadata for the Hint/Example panel.
// It is called once per field render with the top-level block key and the
// field's dot-joined path from the block root (e.g. "source", "source.path").
// For top-level block entries in the root list, fieldPath is empty ("").
// Returning a zero FieldMeta means "no override".
type HintSource interface {
	FieldHint(blockKey, fieldPath string) FieldMeta
}

// HintFunc adapts a plain function to the HintSource interface:
//
//	editor.Run(editor.Config{
//	    Hints: editor.HintFunc(func(block, fieldPath string) editor.FieldMeta {
//	        // return metadata for (block, fieldPath) ...
//	        return editor.FieldMeta{}
//	    }),
//	})
type HintFunc func(blockKey, fieldPath string) FieldMeta

// FieldHint calls f.
func (f HintFunc) FieldHint(blockKey, fieldPath string) FieldMeta { return f(blockKey, fieldPath) }

// ─── Checked Fields ──────────────────────────────────────────────────────────

// CheckedFieldSource returns the sub-field names that start checked when a
// block overlay opens for the given block key. Returning nil or an empty slice
// means "none pre-checked".
type CheckedFieldSource interface {
	CheckedFields(blockKey string) []string
}

// CheckedFieldFunc adapts a plain function to the CheckedFieldSource interface:
//
//	editor.Run(editor.Config{
//	    PreCheckedFields: editor.CheckedFieldFunc(func(blockKey string) []string {
//	        return []string{"name", "enabled"}
//	    }),
//	})
type CheckedFieldFunc func(blockKey string) []string

// CheckedFields calls f.
func (f CheckedFieldFunc) CheckedFields(blockKey string) []string { return f(blockKey) }

// CheckedFieldMap is a map-backed CheckedFieldSource.
// Use it as a drop-in replacement for map[string][]string:
//
//	editor.Run(editor.Config{
//	    PreCheckedFields: editor.CheckedFieldMap{
//	        "categories": {"name", "source", "destination"},
//	    },
//	})
type CheckedFieldMap map[string][]string

// CheckedFields returns the pre-checked field names for blockKey.
func (m CheckedFieldMap) CheckedFields(blockKey string) []string { return m[blockKey] }

// ─── Field Snippets ───────────────────────────────────────────────────────────

// FieldSnippetSource returns the YAML snippet to insert when a sub-field is
// toggled on. Returning an empty string falls back to "<fieldName>: \n".
type FieldSnippetSource interface {
	FieldSnippet(blockKey, fieldName string) string
}

// FieldSnippetFunc adapts a plain function to the FieldSnippetSource interface:
//
//	editor.Run(editor.Config{
//	    FieldSnippets: editor.FieldSnippetFunc(func(blockKey, fieldName string) string {
//	        return ""
//	    }),
//	})
type FieldSnippetFunc func(blockKey, fieldName string) string

// FieldSnippet calls f.
func (f FieldSnippetFunc) FieldSnippet(blockKey, fieldName string) string {
	return f(blockKey, fieldName)
}

// FieldSnippetMap is a map-backed FieldSnippetSource.
// Use it as a drop-in replacement for map[string]map[string]string:
//
//	editor.Run(editor.Config{
//	    FieldSnippets: editor.FieldSnippetMap{
//	        "categories": {"source": "source:\n  path: \"\"\n"},
//	    },
//	})
type FieldSnippetMap map[string]map[string]string

// FieldSnippet returns the YAML snippet for (blockKey, fieldName).
func (m FieldSnippetMap) FieldSnippet(blockKey, fieldName string) string {
	return m[blockKey][fieldName]
}

// ─── Validators ──────────────────────────────────────────────────────────────

// Violation is a single rule violation reported by a Validator.
type Violation struct {
	Path    string // dot-separated YAML path to the offending node; empty for document-wide rules
	Message string // human-readable description, without the path prefix
}

// String renders "<path>: <message>", or just the message when Path is empty.
func (v Violation) String() string {
	if v.Path == "" {
		return v.Message
	}
	return v.Path + ": " + v.Message
}

// Validator is a pluggable rule executed at validate/save time. It returns
// one Violation per problem it finds. Returning an empty slice (or nil)
// means "all good".
type Validator interface {
	Validate(raw []byte, blocks []document.Block) []Violation
}

// ValidatorFunc adapts a plain function to the Validator interface, letting
// callers register inline validators without defining a named type:
//
//	editor.Run(editor.Config{
//	    Validators: []editor.Validator{
//	        editor.ValidatorFunc(func(raw []byte, blocks []document.Block) []editor.Violation {
//	            // custom rule ...
//	            return nil
//	        }),
//	    },
//	})
type ValidatorFunc func(raw []byte, blocks []document.Block) []Violation

// Validate calls f.
func (f ValidatorFunc) Validate(raw []byte, blocks []document.Block) []Violation {
	return f(raw, blocks)
}

// ─── Config ──────────────────────────────────────────────────────────────────

// Config bundles everything the editor needs from the embedding application.
//
// Schema must be a pointer to the Go type describing the YAML document's top
// level (e.g. &MyConfig{}). The editor introspects it through yedit/schema.
//
// Presets is optional — when nil the editor opens fresh blocks with a minimal
// "<key>:\n" template and the preset picker is disabled.
//
// Validators run before every save and on the explicit "validate" shortcut.
// Use editor.MutuallyExclusive and editor.RequiredWith for the common cases.
//
// Hints is optional — when set, each field's Hint/Example panel is populated
// from the returned FieldMeta. All FieldMeta fields are used as-is; YEDIT
// does not fall back to struct tag values. When Hints is nil, the panel shows
// only a generated example.
//
// PreCheckedFields lists which sub-fields of a parent key start checked when
// the overlay opens (e.g. "build" → ["dockerfile","context"]). Use CheckedFieldMap
// as a zero-boilerplate adapter when a static map is enough.
//
// FieldSnippets provides the indented YAML chunk inserted when the user
// toggles a sub-field on. Use FieldSnippetMap as a zero-boilerplate adapter
// when a static map is enough. When a snippet is missing, the editor falls
// back to "<child>: \n".
type Config struct {
	Path                 string             // YAML file to load; also the default save target when SavePath is empty
	Schema               any                // non-nil struct pointer; typed as any because the editor uses reflection (e.g. &MyConfig{})
	Title                string             // label shown in the TUI header
	Presets              PresetSource       // optional; nil disables the preset picker
	Hints                HintSource         // optional; nil hides the Hint/Example panel entirely
	Validators           []Validator        // rules evaluated before every save and on the validate shortcut
	PreCheckedFields     CheckedFieldSource // sub-fields that start checked when a block overlay opens; keyed by top-level yaml name
	FieldSnippets        FieldSnippetSource // YAML inserted when a sub-field is toggled on; keyed by parent key → child yaml name
	Hidden               []string           // top-level keys to omit from the UI entirely
	PassthroughKeys      []string           // top-level keys preserved as-is; hidden from all sections and excluded from unknown-key validation
	Theme                theme.Theme        // zero-value resolves to ThemeDark
	NoDeleteConfirm      bool               // skip the "Remove block?" confirmation dialog; deletion is still undoable via ctrl+u
	NoValidateOnSave     bool               // allow saving even when validators report errors; a warning alert is shown but does not block
	NoSaveConfirm        bool               // skip the "Save changes?" confirmation dialog; warning confirms (NoValidateOnSave) are still shown
	ReadOnly             bool               // disable all edits and saves; the title displays "(READ-ONLY MODE)"
	SavePath             string             // write to this path instead of Path; Path is still used for loading
	SchemaRecursionDepth int                // extra levels a self-referential type expands (e.g. CategoryFilter.Any []CategoryFilter); 0 uses the default (1)
}
