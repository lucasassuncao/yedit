// Package schema discovers the editable shape of a Go struct via reflection
// over yaml tags. It produces a FieldDef tree that yedit/editor uses to drive
// its UI.
package schema

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

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
	// TypeName is the Go type name of a struct-shaped field: the struct itself
	// for KindObject, the element type for a list of structs, the value type for
	// a map of structs. Empty for primitives, anonymous structs, KindVariant and
	// KindAny. It is set even when recursion limiting stops Children from being
	// populated, which is what lets a consumer recognise a recursive type rather
	// than a truncated one.
	TypeName     string
	Scalar       string // concrete scalar type for primitives ("string", "int", "bool", "float", "duration", "uint"); empty for non-scalars
	Children     []FieldDef
	OmitEmpty    bool   // yaml:",omitempty" - zero value is not written to disk
	Flow         bool   // yaml:",flow" - serialised inline rather than block style
	MapKeyScalar string // KindDictionary only: scalar type of the map key ("int", "string", …); "" means string
	// ElemScalar is the scalar type of a collection's element ("string", "int",
	// …) for KindList and KindDictionary. Empty when the element is not a scalar
	// (a struct, a nested collection, an interface). Scalar cannot carry this:
	// it labels the field's own type, which for a collection is not a scalar.
	ElemScalar string
}

// Provider is the opt-in for types reflection cannot read - a union that is a
// string OR a struct OR a map. Entries carrying a "kind" replace the wrapper's
// own fields. See docs/SCHEMA-KINDS.md.
type Provider interface {
	Metadata() map[string]any
}

// kindNames maps the metadata spelling of a kind to its value.
var kindNames = map[string]Kind{
	"primitive":  KindPrimitive,
	"object":     KindObject,
	"list":       KindList,
	"dictionary": KindDictionary,
	"variant":    KindVariant,
	"any":        KindAny,
}

// presentationNames maps the metadata spelling of a presentation to its value.
var presentationNames = map[string]Presentation{
	"":        PresentationDefault,
	"default": PresentationDefault,
	"flat":    PresentationFlat,
	"inline":  PresentationInline,
	"overlay": PresentationOverlay,
}

// UnmarshalYAML resolves a presentation declared by name.
func (p *Presentation) UnmarshalYAML(n *yaml.Node) error {
	var name string
	if err := n.Decode(&name); err != nil {
		return err
	}
	found, ok := presentationNames[name]
	if !ok {
		return fmt.Errorf("unknown presentation %q", name)
	}
	*p = found
	return nil
}

// MarshalYAML writes a presentation as its name.
func (p Presentation) MarshalYAML() (any, error) {
	for name, v := range presentationNames {
		if v == p && name != "" && name != "default" {
			return name, nil
		}
	}
	return "default", nil
}
