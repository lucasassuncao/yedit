// Package spec holds the vocabulary shared by everything that describes a
// configuration field: the editor, the validation rules, the metadata tree, and
// the documentation generator.
//
// These types used to live in the editor package, which meant that any consumer
// wanting to name a FieldMeta or implement a Validator had to import the whole
// TUI - roughly 35 packages of bubbletea, glamour, and Markdown machinery - for
// a handful of struct definitions. Keeping them here lets metadata,
// docgenerator, validate, and third-party rules depend on the vocabulary alone.
//
// spec deliberately imports only document, schema, and yamlnode, all of which
// are leaves. It must never import editor or validate.
package spec

import (
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// ─── Field metadata ──────────────────────────────────────────────────────────

// FieldMeta carries a single field's metadata: displayed in the Hint/Example
// panel and enforced by the FromMetadata validator family. Fields at their zero
// value declare nothing - no panel line, no enforcement.
// MetadataSource is the sole authority: yedit never auto-populates any FieldMeta
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
	// Presentation overrides how the field's children are shown in the tree panel.
	// PresentationOverlay: field opens in a dedicated overlay editor (drill-in).
	// PresentationInline: children are expanded inline in the tree.
	// PresentationFlat: field is shown as a leaf with no children.
	// Zero value (PresentationDefault) derives behavior from Kind.
	Presentation schema.Presentation
	// Multiline is display-only: sets Type to "multiline string" when Type is
	// empty, and auto-generates a block-scalar example when Example is empty.
	// Does not change editor behavior.
	Multiline bool
	// Snippet is the YAML inserted when the field is toggled on in the tree
	// panel. Falls back to "<fieldName>: \n" when empty.
	Snippet string
	// PreChecked marks the field as checked when a new (empty) block is opened.
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
//	    Metadata: spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
//	        // return metadata for (block, fieldPath) ...
//	        return spec.FieldMeta{}
//	    }),
//	})
type MetadataFunc func(blockKey, fieldPath string) FieldMeta

// FieldMeta calls f.
func (f MetadataFunc) FieldMeta(blockKey, fieldPath string) FieldMeta { return f(blockKey, fieldPath) }

// ─── Validation ──────────────────────────────────────────────────────────────

// Group is the display-grouping key for Violation. Violations sharing the same
// Group are merged under a single bullet in the error modal. It was unexported
// while it lived in the editor package; exporting it is what lets validation
// rules outside that package build a Violation.
type Group string

const (
	GroupMutuallyExclusive Group = "Mutually Exclusive"
	GroupUnknownKeys       Group = "Unknown Key(s)"
	GroupRules             Group = "Rules"
)

// Violation is a single rule violation reported by a Validator.
type Violation struct {
	Path    string // dot-separated YAML path to the offending node; empty for document-wide rules
	Message string // human-readable description, without the path prefix
	Group   Group  // when non-empty, violations with the same Group are merged under one bullet in the error display
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

// NewValidationInput parses raw once and bundles it with blocks for a
// validation run. Root is nil when raw is not valid YAML; an empty document
// yields an empty mapping so unconditional checks still run.
func NewValidationInput(raw []byte, blocks []document.Block) ValidationInput {
	root, _ := yamlnode.RootMapping(raw)
	return ValidationInput{Raw: raw, Root: root, Blocks: blocks}
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
//	    Validators: []spec.Validator{
//	        spec.ValidatorFunc(func(in spec.ValidationInput) []spec.Violation {
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
