// Package schema discovers the editable shape of a Go struct via reflection
// over yaml/validate/jsonschema tags. It produces a FieldDef tree that
// yedit/editor uses to drive its UI.
package schema

// Kind classifies a discovered field's shape.
type Kind int

const (
	KindPrimitive  Kind = iota // scalar: string, int, bool, …
	KindObject                 // struct with typed fields
	KindList                   // slice or array
	KindDictionary             // map[K]V
	KindVariant                // union type via the Provider interface
	KindEnum                   // scalar with a fixed oneof set (validate:"oneof=…")
	KindAny                    // interface{}/any — use Provider or raw YAML editing
)

// FieldDef describes a single editable field discovered from a Go struct.
//
// Children is populated when the field nests a struct (Kind == KindObject) or
// when its type implements Provider.
//
// Required, Default, Description, and OneOf are populated by Discover but are
// not currently consumed by the built-in editor UI. They are part of the public
// API for external tooling (e.g. doc generators, custom renderers) that wants
// richer field metadata without re-running reflection. The built-in editor may
// use them in a future release to render hints and pre-fill defaults.
type FieldDef struct {
	YAMLName     string
	Kind         Kind
	Scalar       string   // concrete scalar type for primitives/enums ("string", "int", "bool", "float", "duration", "uint"); empty for non-scalars
	Required     bool     // from validate:"required" or jsonschema:"required"
	Default      string   // from jsonschema:"default=X"
	Description  string   // from jsonschema_description
	OneOf        []string // from validate:"oneof=a b c"
	Children     []FieldDef
	OmitEmpty    bool   // yaml:",omitempty" — zero value is not written to disk
	Flow         bool   // yaml:",flow" — serialised inline rather than block style
	MapKeyScalar string // KindDictionary only: scalar type of the map key ("int", "string", …); "" means string
}

// Provider is an opt-in interface for types that reflection cannot introspect
// correctly — typically union types (e.g. a value that can be a string OR a
// struct OR a map). Implementations return the FieldDef tree they want the
// editor to see in place of the wrapper type's own fields.
type Provider interface {
	YeditSchema() []FieldDef
}
