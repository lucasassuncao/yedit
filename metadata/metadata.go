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
// definitions — Build handles the cycle.
type Node struct {
	editor.FieldMeta
	Children map[string]*Node
}

// Build validates tree against the schema struct (the same pointer handed to
// editor.Config.Schema), fills each node's FieldMeta.Type from the Go type
// (explicitly set Type values are kept), and returns the MetadataSource.
//
// A tree key with no matching yaml-tagged field is an error naming the full
// offending path, so typos and renames surface at startup instead of becoming
// silently dead metadata. Children declared under types reflection cannot see
// into (interfaces, schema.Provider unions) are not validated.
func Build(schemaPtr any, tree map[string]*Node) (editor.MetadataSource, error) {
	rootType := reflect.TypeOf(schemaPtr)
	visited := map[*Node]bool{}
	for blockName, node := range tree {
		ft := fieldTypeByYAML(rootType, blockName)
		if ft == nil {
			return nil, fmt.Errorf("metadata: unknown key %q — no field with that yaml tag in the schema", blockName)
		}
		if err := fill(node, ft, blockName, visited); err != nil {
			return nil, err
		}
	}
	return &metadataSource{tree: tree}, nil
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
				return fmt.Errorf("metadata: unknown key %q — no field with yaml tag %q", childPath, childName)
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

// isProvider reports whether t opts into schema.Provider — its metadata children
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
