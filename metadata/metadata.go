// Package metadata provides a tree-based implementation of editor.MetadataSource.
// Declare each field's editor.FieldMeta in a Node tree keyed by yaml names,
// then call Build to validate the tree against the schema struct and obtain
// the MetadataSource consumed by editor.Config.Metadata and the FromMetadata
// validator family.
package metadata

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/schema"
)

// Node is one field's metadata plus its children, keyed by yaml name.
// Use shared pointers in Children to model recursive schema types (e.g. a
// filter whose "any"/"all" children are filters again) without duplicating
// definitions - NewFromTree handles the cycle.
type Node struct {
	editor.FieldMeta
	Children map[string]*Node
}

// MetadataProvider is implemented by any struct that declares its own field metadata.
// Each struct returns only its direct fields - Children for fields whose types also
// implement MetadataProvider are composed automatically by New.
type MetadataProvider interface {
	Metadata() map[string]*Node
}

var metadataProviderType = reflect.TypeOf((*MetadataProvider)(nil)).Elem()

func collectMissing(fields []schema.FieldDef, nodes map[string]*Node, prefix string, missing *[]string) {
	for _, f := range fields {
		if f.YAMLName == "" || f.YAMLName == "-" {
			continue
		}
		path := f.YAMLName
		if prefix != "" {
			path = prefix + "." + f.YAMLName
		}
		node, ok := nodes[f.YAMLName]
		if !ok {
			*missing = append(*missing, path)
			continue
		}
		if len(f.Children) > 0 && f.Kind != schema.KindVariant {
			collectMissing(f.Children, node.Children, path, missing)
		}
	}
}

// NewFromTree validates tree against the schema struct (the same pointer handed to
// editor.Config.Schema), fills each node's FieldMeta.Type from the Go type
// (explicitly set Type values are kept), and returns the MetadataSource.
//
// When to use NewFromTree vs New:
//
//   - Use New when the root struct is yours and implements MetadataProvider.
//     It builds the tree automatically by calling Metadata() on each nested
//     struct that also implements the interface.
//
//   - Use NewFromTree when the root struct is from a third-party package and
//     cannot implement MetadataProvider. You assemble the Node tree manually
//     and pass it alongside the struct pointer.
//
// New calls NewFromTree internally as its final step - after composing the tree
// via reflection it delegates validation and Type inference here.
//
// Validation: a tree key with no matching yaml-tagged field in the struct is an
// error naming the full offending path (e.g. "categories.sourc"), so typos and
// renames surface at startup instead of becoming silently dead metadata. Children
// declared under types reflection cannot see into (interfaces, schema.Provider
// unions) are not validated.
func NewFromTree(schemaPtr any, tree map[string]*Node) (editor.MetadataSource, error) {
	rootType := reflect.TypeOf(schemaPtr)
	visited := map[*Node]bool{}
	for blockName, node := range tree {
		ft := fieldTypeByYAML(rootType, blockName)
		if ft == nil {
			return nil, fmt.Errorf("metadata: unknown key %q - no field with that yaml tag in the schema", blockName)
		}
		if err := fill(node, ft, blockName, visited); err != nil {
			return nil, err
		}
	}
	return &metadataSource{tree: tree}, nil
}

// New composes the metadata tree from v, which must implement MetadataProvider.
// For each field whose type also implements MetadataProvider, Children are populated
// automatically via reflection. Nodes with Children already set are not overridden
// (explicit wins). Returns an error if any yaml-tagged field in the struct has no
// entry in the composed tree (built-in coverage validation).
//
// Use NewFromTree instead when the root struct is from a third-party package and
// cannot implement MetadataProvider.
//
// How it works:
//
//  1. Assert that v implements MetadataProvider. The root struct must declare its
//     own top-level fields via Metadata() - there is no way to auto-discover them
//     without a starting point.
//
//  2. Call v.Metadata() to obtain the root tree. At this stage the tree contains
//     only the nodes the root struct declared; nested structs have no Children yet.
//
//  3. Unwrap pointer indirection from the concrete type so reflection over struct
//     fields works uniformly regardless of whether v was passed as T or *T.
//
//  4. Run composeTree to auto-populate Children for every node whose field type
//     implements MetadataProvider. See composeTree for the full algorithm.
//
//  5. Discover all yaml-tagged fields in the struct via schema.Discover, then
//     walk the composed tree to find any field that has no corresponding node.
//     This enforces full coverage: adding a new yaml field to the struct without
//     updating Metadata() is a startup error, not a silent gap in the hint panel.
//
//  6. Delegate to NewFromTree to validate tree keys against the schema struct
//     and fill in each node's Type label from the Go type.
func New(v any) (editor.MetadataSource, error) {
	// Step 1: root must declare its own fields.
	p, ok := v.(MetadataProvider)
	if !ok {
		return nil, fmt.Errorf("metadata: %T does not implement MetadataProvider", v)
	}
	// Step 2: seed the tree from the root's own Metadata().
	tree := p.Metadata()
	// Step 3: unwrap pointer so reflect.Type always refers to a struct.
	baseType := reflect.TypeOf(v)
	for baseType.Kind() == reflect.Ptr {
		baseType = baseType.Elem()
	}
	// Step 4: auto-compose Children for nested MetadataProvider fields.
	cache := map[reflect.Type]map[string]*Node{}
	if err := composeTree(baseType, tree, cache); err != nil {
		return nil, err
	}
	// Step 5: enforce full coverage - every yaml-tagged field must have a node.
	fields := schema.Discover(reflect.New(baseType).Interface())
	var missing []string
	collectMissing(fields, tree, "", &missing)
	if len(missing) > 0 {
		return nil, fmt.Errorf("metadata: fields have no documentation: %s", strings.Join(missing, ", "))
	}
	// Step 6: validate keys and fill Type labels.
	return NewFromTree(reflect.New(baseType).Interface(), tree)
}

// composeTree auto-populates Children for nodes whose field types implement
// MetadataProvider. Nodes with Children already set are left untouched (explicit wins).
//
// How it works:
//
//  1. Iterate over the Go struct fields of t via reflection to resolve yaml names.
//     Each field's yaml name is derived from the yaml struct tag (or lowercased field
//     name as fallback), matching exactly how the YAML library maps keys to fields.
//
//  2. For each field, check whether a Node with the same yaml name exists in nodes
//     (the tree returned by the parent's Metadata()). Fields with no Node entry are
//     ignored - they are either non-editable or covered by a Provider union.
//
//  3. Skip nodes that already have Children set - explicit declaration wins over
//     auto-composition, allowing callers to override the subtree when needed.
//
//  4. Resolve the field's element type (unwrapping ptr/slice/map so that *T, []T,
//     and []*T all yield T). Skip scalars, maps-without-struct-values, and types
//     that implement schema.Provider (their children describe variant defs, not
//     struct fields, so reflection cannot verify them).
//
//  5. Check whether T (or *T) implements MetadataProvider. If not, skip - the field
//     is a plain nested struct with no metadata of its own and its parent is
//     responsible for declaring any Children manually.
//
//  6. Guard against recursive types (e.g. CategoryFilter.Any []CategoryFilter):
//     before calling Metadata() on T, check the cache. If T was already visited,
//     reuse the same map pointer instead of recursing - this breaks the cycle while
//     keeping the shared subtree consistent across all references.
//
//  7. Call T.Metadata() to get the child tree, register it in the cache immediately
//     (before recursing) so that self-referential types encountered deeper in the
//     tree hit the cache rather than looping, then recurse into T's own fields.
//
//  8. Assign the composed child tree to node.Children.
func composeTree(t reflect.Type, nodes map[string]*Node, cache map[reflect.Type]map[string]*Node) error {
	elem := elemType(t)
	if elem == nil || elem.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < elem.NumField(); i++ {
		// Step 1: resolve yaml name for this struct field.
		f := elem.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		yamlName := strings.SplitN(tag, ",", 2)[0]
		if yamlName == "" {
			yamlName = strings.ToLower(f.Name)
		}
		// Step 2: find the matching node in the parent's metadata tree.
		node, ok := nodes[yamlName]
		if !ok {
			continue
		}
		// Step 3: explicit Children win - do not overwrite what the caller set.
		if node.Children != nil {
			continue
		}
		// Step 4: unwrap to the element struct type; skip non-struct and Provider fields.
		ft := elemType(f.Type)
		if ft == nil || ft.Kind() != reflect.Struct || isProvider(ft) {
			continue
		}
		// Step 5: only compose fields whose type declares its own metadata.
		if !ft.Implements(metadataProviderType) && !reflect.PointerTo(ft).Implements(metadataProviderType) {
			continue
		}
		// Step 6: cycle guard - reuse the cached tree for recursive types.
		if childTree, seen := cache[ft]; seen {
			node.Children = childTree
			continue
		}
		// Steps 7-8: obtain the child tree, cache it before recursing, then attach.
		prov := reflect.New(ft).Interface().(MetadataProvider)
		childTree := prov.Metadata()
		cache[ft] = childTree
		if err := composeTree(ft, childTree, cache); err != nil {
			return err
		}
		node.Children = childTree
	}
	return nil
}

// metadataSource implements editor.MetadataSource backed by a Node tree.
type metadataSource struct {
	tree map[string]*Node
}

func (h *metadataSource) FieldMeta(block, fieldPath string) editor.FieldMeta {
	node, ok := h.tree[block]
	if !ok {
		return editor.FieldMeta{}
	}
	if fieldPath == "" {
		return node.FieldMeta
	}
	cur := node
	for _, seg := range strings.Split(fieldPath, ".") {
		next, ok := cur.Children[seg]
		if !ok {
			return editor.FieldMeta{}
		}
		cur = next
	}
	return cur.FieldMeta
}

// fill sets node.Type from t (unless explicitly set), validates child keys
// against t's element struct, and recurses. visited breaks shared-pointer
// cycles; a revisited node is already filled and validated.
func fill(node *Node, t reflect.Type, path string, visited map[*Node]bool) error {
	if node == nil || visited[node] {
		return nil
	}
	visited[node] = true
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t != nil && node.Type == "" {
		node.Type = typeLabel(t)
	}
	elem := elemType(t)
	verifiable := elem != nil && elem.Kind() == reflect.Struct && !isProvider(elem)
	for childName, childNode := range node.Children {
		childPath := path + "." + childName
		var childType reflect.Type
		if verifiable {
			childType = fieldTypeByYAML(elem, childName)
			if childType == nil {
				return fmt.Errorf("metadata: unknown key %q - no field with yaml tag %q", childPath, childName)
			}
		}
		if err := fill(childNode, childType, childPath, visited); err != nil {
			return err
		}
	}
	return nil
}

// elemType resolves the struct type that children of t belong to: slices and
// maps yield their element type, pointers are unwrapped.
func elemType(t reflect.Type) reflect.Type {
	for t != nil && (t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice) {
		t = t.Elem()
	}
	if t != nil && t.Kind() == reflect.Map {
		t = t.Elem()
		for t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
	}
	return t
}

var providerType = reflect.TypeOf((*schema.Provider)(nil)).Elem()

// isProvider reports whether t opts into schema.Provider - its metadata children
// describe the provided defs, not struct fields, so they cannot be verified
// by reflection.
func isProvider(t reflect.Type) bool {
	return t.Implements(providerType) || reflect.PointerTo(t).Implements(providerType)
}

// fieldTypeByYAML finds the Go type of the field with yaml tag yamlName in
// struct type t. Returns nil when not found or t is not a struct.
func fieldTypeByYAML(t reflect.Type, yamlName string) reflect.Type {
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		if name == yamlName {
			return f.Type
		}
	}
	return nil
}

var durationType = reflect.TypeOf(time.Duration(0))

// typeLabel converts a Go type to the human-readable label shown in the hint
// panel ("string", "duration", "[]object", "map[string]int", …).
func typeLabel(t reflect.Type) string {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if t == durationType {
			return "duration"
		}
		return "int"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "uint"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Struct {
			return "[]object"
		}
		return "[]" + typeLabel(elem)
	case reflect.Map:
		return "map[" + typeLabel(t.Key()) + "]" + typeLabel(t.Elem())
	case reflect.Struct:
		return "object"
	case reflect.Interface:
		return "any"
	default:
		return t.Kind().String()
	}
}
