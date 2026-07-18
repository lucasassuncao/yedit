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
// unions) are not validated. Nil nodes anywhere in the tree are an error.
//
// The caller's tree is never modified: the returned source is backed by a deep
// copy, so memoized Metadata() results and caller-assembled maps stay pristine.
func NewFromTree(schemaPtr any, tree map[string]*Node) (editor.MetadataSource, error) {
	rootType := reflect.TypeOf(schemaPtr)
	visited := map[fillKey]*Node{}
	filled := make(map[string]*Node, len(tree))
	for blockName, node := range tree {
		if node == nil {
			return nil, fmt.Errorf("metadata: nil node at %q", blockName)
		}
		ft := fieldTypeByYAML(rootType, blockName)
		if ft == nil {
			return nil, fmt.Errorf("metadata: unknown key %q - no field with that yaml tag in the schema", blockName)
		}
		node, err := fill(node, ft, blockName, visited)
		if err != nil {
			return nil, err
		}
		filled[blockName] = node
	}
	return &metadataSource{tree: filled}, nil
}

// New composes the metadata tree from v, which must implement MetadataProvider.
// For each field whose type also implements MetadataProvider, Children are populated
// automatically via reflection. Nodes with Children already set are not overridden
// (explicit wins). Fields with no metadata node are silently accepted and receive
// default (empty) FieldMeta values.
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
//  5. Delegate to NewFromTree to validate tree keys against the schema struct
//     and fill in each node's Type label from the Go type.
func New(v any) (editor.MetadataSource, error) {
	// Step 1: root must declare its own fields.
	p, ok := v.(MetadataProvider)
	if !ok {
		return nil, fmt.Errorf("metadata: %T does not implement MetadataProvider", v)
	}
	// Step 2: seed the tree from the root's own Metadata(). Clone it so
	// composition never mutates the caller's (possibly memoized) map.
	tree := cloneTree(p.Metadata(), map[*Node]*Node{})
	// Step 3: unwrap pointer so reflect.Type always refers to a struct.
	baseType := reflect.TypeOf(v)
	for baseType.Kind() == reflect.Pointer {
		baseType = baseType.Elem()
	}
	// Step 4: auto-compose Children for nested MetadataProvider fields.
	cache := map[reflect.Type]map[string]*Node{}
	if err := composeTree(baseType, tree, cache, map[*Node]bool{}); err != nil {
		return nil, err
	}
	// Step 5: validate keys and fill Type labels.
	return NewFromTree(reflect.New(baseType).Interface(), tree)
}

// composeTree auto-populates Children for nodes whose field types implement
// MetadataProvider. Nodes with Children already set keep them (explicit wins),
// but composition still descends into their subtree so grandchildren that
// implement MetadataProvider are composed too.
//
// How it works:
//
//  1. Iterate over the Go struct fields of t via reflection to resolve yaml names.
//     Each field's yaml name is derived from the yaml struct tag (or lowercased field
//     name as fallback), matching exactly how the YAML library maps keys to fields.
//     Fields promoted by a yaml:",inline" embed are resolved against the same
//     nodes map, since yaml.v3 serialises them at this level.
//
//  2. For each field, check whether a Node with the same yaml name exists in nodes
//     (the tree returned by the parent's Metadata()). Fields with no Node entry are
//     ignored - they are either non-editable or covered by a Provider union. The
//     visited set skips nodes already handled, which also breaks cycles built from
//     shared pointers in explicit Children.
//
//  3. Resolve the field's element type (unwrapping ptr/slice/map so that *T, []T,
//     and []*T all yield T). Skip scalars, maps-without-struct-values, and types
//     that implement schema.Provider (their children describe variant defs, not
//     struct fields, so reflection cannot verify them).
//
//  4. If the node already has Children, keep them (explicit wins) and recurse into
//     the subtree so deeper MetadataProvider fields still compose.
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
//  7. Call T.Metadata() and clone the result so the provider's (possibly memoized)
//     map is never mutated, register the clone in the cache immediately (before
//     recursing) so that self-referential types encountered deeper in the tree hit
//     the cache rather than looping, then recurse into T's own fields.
//
//  8. Assign the composed child tree to node.Children.
func composeTree(t reflect.Type, nodes map[string]*Node, cache map[reflect.Type]map[string]*Node, visited map[*Node]bool) error {
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
		if yamlName == "" && strings.Contains(tag, "inline") {
			// Inline embeds promote their fields to this level; resolve them
			// against the same nodes map.
			if err := composeTree(f.Type, nodes, cache, visited); err != nil {
				return err
			}
			continue
		}
		if yamlName == "" {
			yamlName = strings.ToLower(f.Name)
		}
		// Step 2: find the matching node in the parent's metadata tree.
		node, ok := nodes[yamlName]
		if !ok || node == nil || visited[node] {
			continue
		}
		visited[node] = true
		// Step 3: unwrap to the element struct type; skip non-struct and Provider fields.
		ft := elemType(f.Type)
		if ft == nil || ft.Kind() != reflect.Struct || isProvider(ft) {
			continue
		}
		// Step 4: explicit Children win, but still descend into the subtree.
		if node.Children != nil {
			if err := composeTree(ft, node.Children, cache, visited); err != nil {
				return err
			}
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
		childTree := cloneTree(prov.Metadata(), map[*Node]*Node{})
		cache[ft] = childTree
		if err := composeTree(ft, childTree, cache, visited); err != nil {
			return err
		}
		node.Children = childTree
	}
	return nil
}

// cloneTree deep-copies a Node tree. The memo preserves shared pointers and
// cycles within one clone operation, so recursive trees stay finite.
func cloneTree(nodes map[string]*Node, memo map[*Node]*Node) map[string]*Node {
	if nodes == nil {
		return nil
	}
	out := make(map[string]*Node, len(nodes))
	for name, n := range nodes {
		out[name] = cloneNode(n, memo)
	}
	return out
}

func cloneNode(n *Node, memo map[*Node]*Node) *Node {
	if n == nil {
		return nil
	}
	if c, ok := memo[n]; ok {
		return c
	}
	c := &Node{FieldMeta: n.FieldMeta}
	memo[n] = c
	c.Children = cloneTree(n.Children, memo)
	return c
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

// fillKey identifies one (node, type) pair during filling. A node shared
// under two differently typed fields is cloned, typed, and validated once per
// type instead of once overall.
type fillKey struct {
	node *Node
	t    reflect.Type
}

// fill returns a deep copy of node with Type set from t (unless explicitly
// set) and child keys validated against t's element struct, recursing into
// Children. visited memoizes (node, type) pairs, which both breaks
// shared-pointer cycles and keeps recursive trees finite.
func fill(node *Node, t reflect.Type, path string, visited map[fillKey]*Node) (*Node, error) {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	key := fillKey{node: node, t: t}
	if clone, ok := visited[key]; ok {
		return clone, nil
	}
	clone := &Node{FieldMeta: node.FieldMeta}
	visited[key] = clone
	if t != nil && clone.Type == "" {
		clone.Type = typeLabel(t)
	}
	elem := elemType(t)
	verifiable := elem != nil && elem.Kind() == reflect.Struct && !isProvider(elem)
	if node.Children != nil {
		clone.Children = make(map[string]*Node, len(node.Children))
	}
	for childName, childNode := range node.Children {
		childPath := path + "." + childName
		if childNode == nil {
			return nil, fmt.Errorf("metadata: nil node at %q", childPath)
		}
		var childType reflect.Type
		if verifiable {
			childType = fieldTypeByYAML(elem, childName)
			if childType == nil {
				return nil, fmt.Errorf("metadata: unknown key %q - no field with yaml tag %q", childPath, childName)
			}
		}
		childClone, err := fill(childNode, childType, childPath, visited)
		if err != nil {
			return nil, err
		}
		clone.Children[childName] = childClone
	}
	return clone, nil
}

// elemType resolves the struct type that children of t belong to: slices and
// maps yield their element type, pointers are unwrapped.
func elemType(t reflect.Type) reflect.Type {
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice) {
		t = t.Elem()
	}
	if t != nil && t.Kind() == reflect.Map {
		t = t.Elem()
		for t.Kind() == reflect.Pointer {
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
// struct type t, descending into yaml:",inline" embeds whose fields yaml.v3
// promotes to this level. Returns nil when not found or t is not a struct.
func fieldTypeByYAML(t reflect.Type, yamlName string) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
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
		if name == "" && strings.Contains(tag, "inline") {
			if ft := fieldTypeByYAML(f.Type, yamlName); ft != nil {
				return ft
			}
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		if name == yamlName {
			return f.Type
		}
	}
	return nil
}

// typeLabel converts a Go type to the human-readable label shown in the hint
// panel ("string", "duration", "[]object", "map[string]int", …). Scalar labels
// come from schema.ScalarLabel - the shared vocabulary - so the hint panel and
// the discovered schema can never name the same type differently.
func typeLabel(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if s := schema.ScalarLabel(t); s != "" {
		return s
	}
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		for elem.Kind() == reflect.Pointer {
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
