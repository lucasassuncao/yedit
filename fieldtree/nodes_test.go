package fieldtree

import (
	"testing"

	"github.com/lucasassuncao/yedit/schema"
)

// An inline parent whose only content is a checked openable child must count as
// having content, for both colouring and ctrl+d removal.
func TestHasCheckedDescendantCountsOpenable(t *testing.T) {
	nodes := []Node{
		{Kind: KindField, Label: "filter", Depth: 1, IsLeaf: false},
		{Kind: KindField, Label: "any", Depth: 2, IsLeaf: false, Openable: true, Checked: true},
	}
	if !hasCheckedDescendant(nodes, 0) {
		t.Error("filter with a checked openable child should count as having content")
	}
}

// An openable list-of-struct field is drilled into, not expanded inline, so it
// must not spawn phantom child nodes.
func TestOpenableListHasNoInlineChildren(t *testing.T) {
	defs := []schema.FieldDef{
		{YAMLName: "filter", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "any", Kind: schema.KindList, Children: []schema.FieldDef{
				{YAMLName: "regex", Kind: schema.KindPrimitive},
			}},
		}},
	}
	nodes := flattenDefsAsTree(defs, nil, 0)
	for _, n := range nodes {
		if n.Label == "regex" {
			t.Errorf("openable list spawned a phantom inline child %q", n.Label)
		}
		if n.Label == "any" {
			if !n.Openable {
				t.Error("any should be openable")
			}
			if !n.IsLeaf {
				t.Error("openable list should be leaf-like in the tree (no inline children)")
			}
		}
	}
}
