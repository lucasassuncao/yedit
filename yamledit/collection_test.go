package yamledit

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

// be.coll.current legitimately holds -1 for an empty collection, and the map
// branch's 2*i < len check would accept a negative index on its own.
func TestEntryLabel_negativeIndex(t *testing.T) {
	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "web"},
		{Kind: yaml.MappingNode},
	}}
	if got := EntryLabel(node, true, -1); got != "" {
		t.Errorf("EntryLabel(map, -1) = %q, want \"\"", got)
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "name"},
			{Kind: yaml.ScalarNode, Value: "item-0"},
		}},
	}}
	if got := EntryLabel(seq, false, -1); got != "item 0" {
		t.Errorf("EntryLabel(seq, -1) = %q, want fallback label", got)
	}
}

// A list of scalars labels each entry by its value; an empty one falls back.
func TestEntryLabel_scalarItem(t *testing.T) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "./a.yaml"},
		{Kind: yaml.ScalarNode, Value: ""},
	}}
	if got := EntryLabel(seq, false, 0); got != "./a.yaml" {
		t.Errorf("EntryLabel(scalar) = %q, want the value", got)
	}
	if got := EntryLabel(seq, false, 1); got != "item 2" {
		t.Errorf("EntryLabel(empty scalar) = %q, want fallback label", got)
	}
}

func TestParseScalarItemFromView(t *testing.T) {
	item, ok := ParseScalarItemFromView("import:\n  - ./a.yaml\n")
	if !ok || item.Value != "./a.yaml" {
		t.Errorf("ParseScalarItemFromView(scalar) = %v, %v; want ./a.yaml, true", item, ok)
	}
	for _, view := range []string{"import:\n  - path: ./a.yaml\n", "import:\n", "import: ./a.yaml\n"} {
		if _, ok := ParseScalarItemFromView(view); ok {
			t.Errorf("ParseScalarItemFromView(%q) accepted a view that holds no scalar item", view)
		}
	}
}
