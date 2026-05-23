package schema

import (
	"reflect"
	"strings"
)

// defaultSkip lists YAML keys that the discoverer suppresses by default.
// Clients can extend this through Config.Hidden when invoking the editor.
var defaultSkip = map[string]bool{
	"$schema": true,
}

// Discover walks the type of v by reflection and returns the editable schema
// of its exported fields. Fields without a yaml tag, with yaml:"-", or with a
// name in defaultSkip are omitted. Nested struct fields recurse one level
// deeper.
//
// Only the yaml tag is required. validate and jsonschema_description are
// optional and merely enrich the FieldDef when present:
//
//   - validate:"required"          → FieldDef.Required = true (renders a "*")
//   - validate:"oneof=a b c"       → FieldDef.OneOf populated
//   - jsonschema:"default=X"       → FieldDef.Default populated
//   - jsonschema:"required"        → FieldDef.Required = true (alternative)
//   - jsonschema_description:"..." → FieldDef.Description populated
//
// A struct annotated only with yaml tags discovers cleanly; fields just have
// zero-valued Required/Default/Description/OneOf.
//
// To customise discovery for union types (a value that can be a scalar OR a
// struct OR a map), make the wrapper type implement Provider — its
// YeditSchema() return value is used in place of reflective traversal.
func Discover(v any) []FieldDef {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return discoverFields(t, 0)
}

func discoverFields(t reflect.Type, depth int) []FieldDef {
	if depth > 3 || t.Kind() != reflect.Struct {
		return nil
	}
	var out []FieldDef
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		yamlTag := f.Tag.Get("yaml")
		yamlName := strings.Split(yamlTag, ",")[0]
		if yamlName == "" || yamlName == "-" {
			continue
		}
		if defaultSkip[yamlName] {
			continue
		}

		validateTag := f.Tag.Get("validate")
		jsTag := f.Tag.Get("jsonschema")

		info := FieldDef{
			YAMLName:    yamlName,
			Kind:        kindOf(f.Type),
			Required:    containsTagOption(validateTag, "required") || containsTagOption(jsTag, "required"),
			Default:     extractValue(jsTag, "default="),
			Description: f.Tag.Get("jsonschema_description"),
			OneOf:       extractList(validateTag, "oneof="),
		}

		// Custom union types take precedence over reflective descent.
		if children := providerChildren(f.Type); children != nil {
			info.Kind = KindUnion
			info.Children = children
		} else {
			nested := unwrap(f.Type)
			if nested.Kind() == reflect.Struct {
				info.Children = discoverFields(nested, depth+1)
			}
		}

		out = append(out, info)
	}
	return out
}

// providerChildren returns the FieldDef list declared by a type implementing
// Provider, or nil if the type does not implement the interface.
func providerChildren(t reflect.Type) []FieldDef {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() == reflect.Map {
		t = t.Elem()
	}
	providerType := reflect.TypeOf((*Provider)(nil)).Elem()
	switch {
	case t.Implements(providerType):
		zero := reflect.Zero(t).Interface().(Provider)
		return zero.YeditSchema()
	case reflect.PointerTo(t).Implements(providerType):
		zero := reflect.New(t).Interface().(Provider)
		return zero.YeditSchema()
	}
	return nil
}

// kindOf classifies a Go type for the FieldDef.Kind field.
func kindOf(t reflect.Type) Kind {
	switch t.Kind() {
	case reflect.Ptr:
		return kindOf(t.Elem())
	case reflect.Struct:
		return KindStruct
	case reflect.Slice, reflect.Array:
		return KindSlice
	case reflect.Map:
		return KindMap
	default:
		return KindScalar
	}
}

// unwrap removes pointer, slice, and map wrappers to reach the element type
// that might be a struct worth recursing into.
func unwrap(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() == reflect.Map {
		return unwrap(t.Elem())
	}
	return t
}

// TopLevelOrder returns the discovered top-level yaml names in declaration
// order. Use it as the knownOrder argument when constructing a document.Document.
func TopLevelOrder(fields []FieldDef) []string {
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.YAMLName)
	}
	return out
}
