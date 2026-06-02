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
	Path             string
	Schema           any
	Title            string
	Presets          presets.Source
	Validators       []Validator
	PreCheckedFields map[string][]string
	FieldSnippets    map[string]map[string]string
	FieldExamples    map[string]map[string]string
	Hidden           []string    // additional top-level keys to omit from the UI
	Theme            theme.Theme // zero-value resolves to ThemeDark
}

// fieldSnippetsFor returns the snippet map for parent (may be nil).
func (c Config) fieldSnippetsFor(parent string) map[string]string {
	if c.FieldSnippets == nil {
		return nil
	}
	return c.FieldSnippets[parent]
}
