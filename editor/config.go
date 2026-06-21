// Package editor provides the bubbletea TUI for editing a YAML file driven by
// a struct-based schema and a preset source.
package editor

import (
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/theme"
)

// ─── Hints ───────────────────────────────────────────────────────────────────

// FieldMeta carries a single field's metadata: displayed in the Hint/Example
// panel and enforced by the FromMetadata validator family. Fields at their zero
// value declare nothing - no panel line, no enforcement.
// MetadataSource is the sole authority: YEDIT never auto-populates any FieldMeta
// field from struct tags. If no MetadataSource is configured, the hint panel
// shows only a generated example.
type FieldMeta struct {
	Description string
	Type        string   // human-readable Go type: "string", "bool", "int", "[]string", "duration", "object", etc.
	Required    bool     // enforced by RequiredFromMetadata
	Default     string   // display only - no enforcement rule exists for defaults
	OneOf       []string // enforced by OneOfFromMetadata
	Example     string   // YAML snippet shown verbatim in the Example section

	// Value constraints, enforced by the FromMetadata validator family.
	Min, Max string // RangeFromMetadata - number, duration, or size strings (ValueInRange semantics)
	Pattern  string // PatternFromMetadata - RE2 regular expression (ValueMatches semantics)
	// Collection constraints. MinCount/MaxCount both zero means no rule;
	// MinCount > 0 with MaxCount == 0 means "at least MinCount, no upper bound".
	MinCount, MaxCount int  // CountFromMetadata (CountRange semantics)
	Unique             bool // UniqueFromMetadata - scalar list items must not repeat
	// Deprecation: non-empty marks the field deprecated; the value is the
	// migration hint shown to the user (DeprecatedFromMetadata).
	Deprecated string

	// Formats lists the acceptable string formats for this field.
	// FormatFromMetadata validates the field's value against each format
	// using OR semantics: valid if any format's validator returns true.
	// Empty means no format rule. Use FormatCustom for app-specific formats.
	Formats []Format
	// MinLength and MaxLength constrain string length in Unicode code points.
	// 0 means no rule. Enforced by LengthFromMetadata.
	MinLength int
	MaxLength int
	// NotOneOf is a case-sensitive denylist. Enforced by NotOneOfFromMetadata.
	// Skipped when empty or when the field value is empty.
	NotOneOf []string
	// Multiline is display-only: sets Type to "multiline string" when Type is
	// empty, and auto-generates a block-scalar example when Example is empty.
	// Does not change editor behavior.
	Multiline bool
	// Snippet is the YAML inserted when the field is toggled on in the tree
	// panel. Replaces the old Config.FieldSnippets source. Falls back to
	// "<fieldName>: \n" when empty.
	Snippet string
	// PreChecked marks the field as checked when a new (empty) block is opened.
	// Replaces the old Config.PreCheckedFields source.
	PreChecked bool
}

// MetadataSource provides per-field metadata for the Hint/Example panel and
// the FromMetadata validator family. It is called with the top-level block key
// and the field's dot-joined path from the block root (e.g. "source",
// "source.path"). For top-level block entries in the root list, fieldPath is
// empty (""). Returning a zero FieldMeta means "no override".
type MetadataSource interface {
	FieldMeta(blockKey, fieldPath string) FieldMeta
}

// MetadataFunc adapts a plain function to the MetadataSource interface:
//
//	editor.Run(editor.Config{
//	    Metadata: editor.MetadataFunc(func(block, fieldPath string) editor.FieldMeta {
//	        // return metadata for (block, fieldPath) ...
//	        return editor.FieldMeta{}
//	    }),
//	})
type MetadataFunc func(blockKey, fieldPath string) FieldMeta

// FieldMeta calls f.
func (f MetadataFunc) FieldMeta(blockKey, fieldPath string) FieldMeta { return f(blockKey, fieldPath) }

// ─── Validators ──────────────────────────────────────────────────────────────

// group is the display-grouping key for Violation. Violations sharing the same
// Group are merged under a single bullet in the error modal.
type group string

const (
	GroupMutuallyExclusive group = "Mutually Exclusive"
	GroupUnknownKeys       group = "Unknown Key(s)"
	GroupRules             group = "Rules"
)

// Violation is a single rule violation reported by a Validator.
type Violation struct {
	Path    string // dot-separated YAML path to the offending node; empty for document-wide rules
	Message string // human-readable description, without the path prefix
	Group   group  // when non-empty, violations with the same Group are merged under one bullet in the error display
}

// String renders "<path>: <message>", or just the message when Path is empty.
func (v Violation) String() string {
	if v.Path == "" {
		return v.Message
	}
	return v.Path + ": " + v.Message
}

// ValidationInput carries the document state inspected by validators. RunAll
// builds it once per run and shares it across all validators, so the document
// is parsed a single time instead of once per validator. Build one with
// NewValidationInput when invoking a validator directly.
type ValidationInput struct {
	Raw    []byte           // document bytes, CRLF-normalised
	Root   *yaml.Node       // parsed document root; an empty document yields an empty mapping, invalid YAML yields nil
	Blocks []document.Block // top-level blocks
}

// Validator is a pluggable rule executed at validate/save time. It returns
// one Violation per problem it finds. Returning an empty slice (or nil)
// means "all good".
type Validator interface {
	Validate(in ValidationInput) []Violation
}

// ValidatorFunc adapts a plain function to the Validator interface, letting
// callers register inline validators without defining a named type:
//
//	editor.Run(editor.Config{
//	    Validators: []editor.Validator{
//	        editor.ValidatorFunc(func(in editor.ValidationInput) []editor.Violation {
//	            // custom rule ...
//	            return nil
//	        }),
//	    },
//	})
type ValidatorFunc func(in ValidationInput) []Violation

// Validate calls f.
func (f ValidatorFunc) Validate(in ValidationInput) []Violation {
	return f(in)
}

// ─── Config ──────────────────────────────────────────────────────────────────

// Config bundles everything the editor needs from the embedding application.
//
// Schema must be a pointer to the Go type describing the YAML document's top
// level (e.g. &MyConfig{}). The editor introspects it through yedit/schema.
//
// Presets is optional - when nil the editor opens fresh blocks with a minimal
// "<key>:\n" template and the preset picker is disabled.
//
// Validators run before every save and on the explicit "validate" shortcut.
// Use editor.MutuallyExclusive and editor.RequiredWith for the common cases.
//
// Hints is optional - when set, each field's Hint/Example panel is populated
// from the returned FieldMeta. All FieldMeta fields are used as-is; YEDIT
// does not fall back to struct tag values. When Hints is nil, the panel shows
// only a generated example.
//
// FieldMeta.PreChecked lists sub-fields that start checked when a new block
// overlay opens. FieldMeta.Snippet provides the YAML inserted when a sub-field
// is toggled on; falls back to "<fieldName>: \n" when empty.
type Config struct {
	Path                 string         // YAML file to load; also the default save target when SavePath is empty
	Schema               any            // non-nil struct pointer; typed as any because the editor uses reflection (e.g. &MyConfig{})
	Title                string         // label shown in the TUI header
	BlockPresets         presets.Source // optional; nil disables the preset picker inside block editors
	DocPresets           presets.Source // optional; when set, ctrl+p on the root list opens a whole-document template picker
	EnableHints          bool           // show the Hint/Example panel; requires Metadata to be set (a warning is shown if it is not)
	Metadata             MetadataSource // field metadata displayed in the hint panel and enforced by the FromMetadata validators
	Validators           []Validator    // rules evaluated before every save and on the validate shortcut
	Hidden               []string       // top-level keys to omit from the UI entirely
	PassthroughKeys      []string       // top-level keys preserved as-is; hidden from all sections and excluded from unknown-key validation
	Theme                theme.Theme    // zero-value resolves to ThemeDark
	NoDeleteConfirm      bool           // skip the "Remove block?" confirmation dialog; deletion is still undoable via ctrl+u
	NoValidateOnSave     bool           // allow saving even when validators report errors; a warning alert is shown but does not block
	NoSaveConfirm        bool           // skip the "Save changes?" confirmation dialog; warning confirms (NoValidateOnSave) are still shown
	SavePath             string         // write to this path instead of Path; Path is still used for loading
	SchemaRecursionDepth int            // extra levels a self-referential type expands (e.g. CategoryFilter.Any []CategoryFilter); 0 uses the default (1)
}
