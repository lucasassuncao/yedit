package editor

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/yamledit"
)

// be.coll.current legitimately holds -1 for an empty collection, and the map
// branch's 2*i < len check would accept a negative index on its own.
func TestEntryLabel_negativeIndex(t *testing.T) {
	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "web"},
		{Kind: yaml.MappingNode},
	}}
	if got := yamledit.EntryLabel(node, true, -1); got != "" {
		t.Errorf("yamledit.EntryLabel(map, -1) = %q, want \"\"", got)
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{
		{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "name"},
			{Kind: yaml.ScalarNode, Value: "item-0"},
		}},
	}}
	if got := yamledit.EntryLabel(seq, false, -1); got != "item 0" {
		t.Errorf("yamledit.EntryLabel(seq, -1) = %q, want fallback label", got)
	}
}
