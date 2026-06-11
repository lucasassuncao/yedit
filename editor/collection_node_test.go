package editor

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"
)

// BenchmarkDeriveChecked measures re-deriving checked states for all field
// nodes in a flat list - called once per user action on a collection.
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

// BenchmarkBuildSeqNodes measures the full tree rebuild for a sequence
// collection - includes flattenDefsAsTree + deriveChecked per entry.
func BenchmarkBuildSeqNodes_10(b *testing.B)  { benchmarkBuildSeqNodes(b, 10) }
func BenchmarkBuildSeqNodes_100(b *testing.B) { benchmarkBuildSeqNodes(b, 100) }
func BenchmarkBuildSeqNodes_500(b *testing.B) { benchmarkBuildSeqNodes(b, 500) }

func benchmarkBuildSeqNodes(b *testing.B, n int) {
	b.Helper()
	node := makeBenchSeqNode(n)
	defs := catDefs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildSeqNodesFromNode(defs, node)
	}
}

// makeBenchSeqNode returns a SequenceNode with n entries, each a MappingNode
// carrying a "name" key - representative of a real category list.
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
