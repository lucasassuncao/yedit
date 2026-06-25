// Package schema discovers the editable shape of a Go struct via reflection
// over yaml tags. It produces a FieldDef tree that yedit/editor uses to drive
// its UI.
package schema

// Presentation controls how a field's children are shown in the tree panel.
// It is applied after schema discovery via the editor's applyPresentation step.
// KindPrimitive fields are always PresentationFlat regardless of what is set.
type Presentation int

const (
	PresentationDefault Presentation = iota // derive from Kind: Object→Inline, List/Dict→Overlay, Primitive→Flat
	PresentationFlat                        // leaf with no children shown
	PresentationInline                      // children expanded inline in the tree
	PresentationOverlay                     // children opened in a dedicated overlay editor
)

// Kind classifies a discovered field's shape.
type Kind int

const (
	KindPrimitive  Kind = iota // scalar: string, int, bool, …
	KindObject                 // struct with typed fields
	KindList                   // slice or array
	KindDictionary             // map[K]V
	KindVariant                // union type via the Provider interface
	KindAny                    // interface{}/any - use Provider or raw YAML editing
)

// FieldDef describes a single editable field discovered from a Go struct.
//
// Children is populated when the field nests a struct (Kind == KindObject) or
// when its type implements Provider.
//
// FieldDef carries structure only. Field metadata (required, allowed values,
// ranges, descriptions) is declared through the editor's MetadataSource - see the
// yedit/metadata package.
type FieldDef struct {
	YAMLName     string
	Kind         Kind
	Presentation Presentation // how children are shown; set by editor.applyPresentation
	Scalar       string       // concrete scalar type for primitives ("string", "int", "bool", "float", "duration", "uint"); empty for non-scalars
	Children     []FieldDef
	OmitEmpty    bool   // yaml:",omitempty" - zero value is not written to disk
	Flow         bool   // yaml:",flow" - serialised inline rather than block style
	MapKeyScalar string // KindDictionary only: scalar type of the map key ("int", "string", …); "" means string
}

// Provider is an opt-in interface for types that reflection cannot introspect
// correctly - typically union types (e.g. a value that can be a string OR a
// struct OR a map). Implementations return the FieldDef tree they want the
// editor to see in place of the wrapper type's own fields.
type Provider interface {
	YeditSchema() []FieldDef
}
