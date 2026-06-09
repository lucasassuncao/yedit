// Package editor provides the bubbletea TUI for editing a YAML file driven by
// a struct-based schema and a preset source.
package editor

import (
	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/theme"
)

// Validator is a pluggable rule executed at validate/save time. It returns
// human-readable messages for every violation it finds. Returning an empty
// slice (or nil) means "all good".
type Validator interface {
	Validate(raw []byte, blocks []document.Block) []string
}

// ValidatorFunc adapts a plain function to the Validator interface, letting
// callers register inline validators without defining a named type:
//
//	editor.Run(editor.Config{
//	    Validators: []editor.Validator{
//	        editor.ValidatorFunc(func(raw []byte, blocks []document.Block) []string {
//	            // custom rule ...
//	            return nil
//	        }),
//	    },
//	})
type ValidatorFunc func(raw []byte, blocks []document.Block) []string

// Validate calls f.
func (f ValidatorFunc) Validate(raw []byte, blocks []document.Block) []string {
	return f(raw, blocks)
}

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
// PreCheckedFields lists which sub-fields of a parent key start checked when
// the overlay opens. Keyed by top-level yaml name (e.g. "build" → ["dockerfile","context"]).
//
// FieldSnippets provides the indented YAML chunk inserted when the user
// toggles a sub-field on (keyed by parent key → child yaml name → snippet).
// When a snippet is missing, the editor falls back to "<child>: \n".
//
// FieldExamples provides a YAML snippet shown in the hint panel for each
// field (keyed by block yaml name → field yaml name → snippet). When absent
// the editor falls back to the "base" preset for that block, if one exists.
type Config struct {
	Path                 string
	Schema               any
	Title                string
	Presets              presets.Source
	Validators           []Validator
	PreCheckedFields     map[string][]string
	FieldSnippets        map[string]map[string]string
	FieldExamples        map[string]map[string]string
	Hidden               []string    // top-level keys to omit from the UI entirely
	PassthroughKeys      []string    // top-level keys preserved as-is; hidden from all sections and excluded from unknown-key validation
	Theme                theme.Theme // zero-value resolves to ThemeDark
	NoDeleteConfirm      bool        // skip the "Remove block?" confirmation dialog; deletion is still undoable via ctrl+u
	NoValidateOnSave     bool        // allow saving even when validators report errors; a warning alert is shown but does not block
	NoSaveConfirm        bool        // skip the "Save changes?" confirmation dialog; warning confirms (NoValidateOnSave) are still shown
	ReadOnly             bool        // disable all edits and saves; the title displays "(READ-ONLY MODE)"
	SavePath             string      // write to this path instead of Path; Path is still used for loading
	SchemaRecursionDepth int         // extra levels a self-referential type expands (e.g. CategoryFilter.Any []CategoryFilter); 0 uses the default (1)
}
