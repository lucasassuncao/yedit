package schema

import (
	"encoding"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// defaultSkip lists YAML keys that the discoverer suppresses by default.
// Clients can extend this through Config.Hidden when invoking the editor.
var defaultSkip = map[string]bool{
	"$schema": true,
}

var (
	yamlMarshalerType = reflect.TypeOf((*yaml.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// Discover walks the type of v by reflection and returns the editable schema
// of its exported fields. Fields without a yaml tag, with yaml:"-", or with a
// name in defaultSkip are omitted. Anonymous embeds follow yaml.v3: only
// yaml:",inline" promotes an embed's fields; a bare exported embed is a
// regular field named after its lowercased type name, and a bare unexported
// embed is skipped (yaml.v3 cannot marshal it). Nested struct fields recurse
// one level deeper.
//
// Only the yaml tag is read. Field metadata (required, allowed values, ranges,
// descriptions) is not derived from struct tags - declare it through the
// editor's MetadataSource instead (see the yedit/metadata package).
//
// To customise discovery for union types (a value that can be a scalar OR a
// struct OR a map), make the wrapper type implement Provider - its
// YeditSchema() return value is used in place of reflective traversal.
//
// The optional recursionLimit controls how many extra times each individual
// type may re-enter the traversal beyond its first visit. Omitted, it defaults
// to 1, which allows one recursive level so that fields like "any
// []CategoryFilter" are navigable. Passing 0 explicitly selects strict mode:
// recursive occurrences are not expanded at all. The bound is counted per
// type, so mutually recursive chains (A contains B contains A) may expand
// deeper overall than a single self-referential type would.
func Discover(v any, recursionLimit ...int) []FieldDef {
	limit := 1 // default: one extra recursive level beyond the first visit
	if len(recursionLimit) > 0 {
		limit = recursionLimit[0] // caller-specified; 0 = strict (no extra levels)
	}
	t := reflect.TypeOf(v)
	if t == nil {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return discoverFields(t, 0, make(map[reflect.Type]int), limit)
}

func discoverFields(t reflect.Type, depth int, seen map[reflect.Type]int, limit int) []FieldDef {
	if seen[t] > limit || depth > 20 || t.Kind() != reflect.Struct {
		return nil
	}
	seen[t]++
	defer func() { seen[t]-- }()

	var out []FieldDef
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		yamlTag := f.Tag.Get("yaml")
		yamlName := strings.Split(yamlTag, ",")[0]
		inline := yamlName == "" && strings.Contains(yamlTag, "inline")
		if !f.IsExported() {
			// yaml.v3 can only serialise an unexported embed through an
			// explicit yaml:",inline" tag (the promoted exported fields are
			// reachable); a bare unexported embed panics at Marshal time.
			if f.Anonymous && inline {
				out = append(out, embedFields(f, depth, seen, limit)...)
			}
			continue
		}
		if yamlName == "-" {
			continue
		}
		if inline {
			out = append(out, embedFields(f, depth, seen, limit)...)
			continue
		}
		if yamlName == "" {
			if yamlTag == "" && !f.Anonymous {
				continue // untagged plain fields are not part of the editable schema
			}
			// yaml.v3 keys a bare anonymous embed and a name-less tag such as
			// yaml:",omitempty" by the lowercased field name (for an embed the
			// field name is the type name).
			yamlName = strings.ToLower(f.Name)
		}
		if defaultSkip[yamlName] {
			continue
		}
		info := buildFieldDef(f, yamlName, yamlTag)
		fillFieldChildren(&info, f, depth, seen, limit)
		out = append(out, info)
	}
	return out
}

// embedFields promotes the exported fields of an anonymous or inline struct embed.
func embedFields(f reflect.StructField, depth int, seen map[reflect.Type]int, limit int) []FieldDef {
	ft := f.Type
	for ft.Kind() == reflect.Pointer {
		ft = ft.Elem()
	}
	if ft.Kind() != reflect.Struct {
		return nil
	}
	return discoverFields(ft, depth+1, seen, limit)
}

// buildFieldDef constructs a FieldDef from a struct field's yaml tag.
func buildFieldDef(f reflect.StructField, yamlName, yamlTag string) FieldDef {
	info := FieldDef{
		YAMLName: yamlName,
		Kind:     kindOf(f.Type),
		Scalar:   ScalarLabel(f.Type),
	}
	for _, opt := range strings.Split(yamlTag, ",")[1:] {
		switch strings.TrimSpace(opt) {
		case "omitempty":
			info.OmitEmpty = true
		case "flow":
			info.Flow = true
		}
	}
	if info.Kind == KindDictionary {
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Map {
			info.MapKeyScalar = ScalarLabel(ft.Key())
		}
	}
	return info
}

// fillFieldChildren populates info.Kind and info.Children via provider check,
// marshaler check, or recursive struct descent.
func fillFieldChildren(info *FieldDef, f reflect.StructField, depth int, seen map[reflect.Type]int, limit int) {
	if children := providerChildren(f.Type); children != nil {
		info.Kind = KindVariant
		info.Children = children
		return
	}
	if isMarshalerType(f.Type) {
		return
	}
	nested := unwrap(f.Type)
	if nested.Kind() == reflect.Struct {
		info.Children = discoverFields(nested, depth+1, seen, limit)
	}
}

// isMarshalerType reports whether t (or *t) implements yaml.Marshaler or
// encoding.TextMarshaler. These types serialise as scalars; their struct
// fields must not be exposed in the editor.
func isMarshalerType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Implements(yamlMarshalerType) ||
		reflect.PointerTo(t).Implements(yamlMarshalerType) ||
		t.Implements(textMarshalerType) ||
		reflect.PointerTo(t).Implements(textMarshalerType)
}

// providerChildren returns the FieldDef list declared by a type implementing
// Provider, or nil if the type does not implement the interface.
func providerChildren(t reflect.Type) []FieldDef {
	// Unwrap wrappers until stable so map[string]*T, map[string][]T, []*T,
	// and other combinations all reach the element type T.
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	providerType := reflect.TypeOf((*Provider)(nil)).Elem()
	// An interface type has no concrete value to call YeditSchema on - such
	// fields classify as KindAny instead.
	if t.Kind() == reflect.Interface {
		return nil
	}
	// Instantiate via reflect.New so the receiver is never a typed nil
	// pointer, which would panic when YeditSchema has a value receiver.
	switch {
	case t.Implements(providerType):
		zero := reflect.New(t).Elem().Interface().(Provider)
		return zero.YeditSchema()
	case reflect.PointerTo(t).Implements(providerType):
		zero := reflect.New(t).Interface().(Provider)
		return zero.YeditSchema()
	}
	return nil
}

// kindOf classifies a Go type for the FieldDef.Kind field.
func kindOf(t reflect.Type) Kind {
	if t.Kind() == reflect.Pointer {
		return kindOf(t.Elem())
	}
	if t.Kind() == reflect.Interface {
		return KindAny
	}
	if isMarshalerType(t) {
		return KindPrimitive
	}
	switch t.Kind() {
	case reflect.Struct:
		return KindObject
	case reflect.Slice, reflect.Array:
		return KindList
	case reflect.Map:
		return KindDictionary
	default:
		return KindPrimitive
	}
}

// ScalarLabel returns a human label for a scalar Go type ("string", "int",
// "bool", "float", "duration", "uint") or "" when t is not a scalar. Named
// types with their own meaning (time.Duration) take precedence over their
// underlying kind. It is the single vocabulary for scalar type labels: it
// enriches FieldDef.Scalar and the metadata package builds its hint-panel
// labels on top of it, so the two can never name the same type differently.
func ScalarLabel(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.PkgPath() == "time" && t.Name() == "Duration" {
		return "duration"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "float"
	default:
		return ""
	}
}

// unwrap removes pointer, slice, and map wrappers to reach the element type
// that might be a struct worth recursing into.
func unwrap(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
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
