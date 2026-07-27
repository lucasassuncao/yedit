package editor

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"
)

// be.coll.current legitimately holds -1 for an empty collection, and the map
// branch's 2*i < len check would accept a negative index on its own.
func TestEntryLabel_negativeIndex(t *testing.T) {
	node := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "web"},
		{Kind: yaml.MappingNode},
	}}
	if got := entryLabel(node, true, -1); got != "" {
		t.Errorf("entryLabel(map, -1) = %q, want \"\"", got)
	}
	seq := makeBenchSeqNode(1)
	if got := entryLabel(seq, false, -1); got != "item 0" {
		t.Errorf("entryLabel(seq, -1) = %q, want fallback label", got)
	}
}

// Re-deriving checked states for every field node, once per user action.
func BenchmarkDeriveChecked_10(b *testing.B)  { benchmarkDeriveChecked(b, 10) }
func BenchmarkDeriveChecked_100(b *testing.B) { benchmarkDeriveChecked(b, 100) }
func BenchmarkDeriveChecked_500(b *testing.B) { benchmarkDeriveChecked(b, 500) }

func benchmarkDeriveChecked(b *testing.B, n int) {
	b.Helper()
	node := makeBenchSeqNode(n)
	nodes := flattenDefsAsTree(catDefs(), []string{}, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = deriveChecked(node, nodes, false)
	}
}

// Full tree rebuild for a sequence collection: flattenDefsAsTree plus
// deriveChecked per entry.
func BenchmarkBuildSeqNodes_10(b *testing.B)  { benchmarkBuildSeqNodes(b, 10) }
func BenchmarkBuildSeqNodes_100(b *testing.B) { benchmarkBuildSeqNodes(b, 100) }
func BenchmarkBuildSeqNodes_500(b *testing.B) { benchmarkBuildSeqNodes(b, 500) }

func benchmarkBuildSeqNodes(b *testing.B, n int) {
	b.Helper()
	node := makeBenchSeqNode(n)
	defs := catDefs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildCollectionNodesFromNode(defs, node, false)
	}
}

// makeBenchSeqNode returns n mapping entries with a "name" key, representative
// of a real category list.
func makeBenchSeqNode(n int) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for i := 0; i < n; i++ {
		seq.Content = append(seq.Content, &yaml.Node{
			Kind: yaml.MappingNode,
			Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: "name"},
				{Kind: yaml.ScalarNode, Value: fmt.Sprintf("item-%d", i)},
			},
		})
	}
	return seq
}
