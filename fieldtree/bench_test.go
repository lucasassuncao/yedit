package fieldtree

import (
	"fmt"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

// catDefs is the schema of a category entry, the shape the benchmarks project.
func catDefs() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "name", Kind: schema.KindPrimitive},
		{YAMLName: "enabled", Kind: schema.KindPrimitive},
		{YAMLName: "source", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "path", Kind: schema.KindPrimitive},
			{YAMLName: "extensions", Kind: schema.KindList},
			{YAMLName: "filter", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "regex", Kind: schema.KindPrimitive},
				{YAMLName: "glob", Kind: schema.KindPrimitive},
			}},
		}},
		{YAMLName: "hooks", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "before", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "shell", Kind: schema.KindPrimitive},
			}},
			{YAMLName: "after", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "shell", Kind: schema.KindPrimitive},
			}},
		}},
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
		_ = DeriveChecked(node, nodes, false)
	}
}

// Full tree rebuild for a sequence collection: flattenDefsAsTree plus
// DeriveChecked per entry.
func BenchmarkBuildSeqNodes_10(b *testing.B)  { benchmarkBuildSeqNodes(b, 10) }
func BenchmarkBuildSeqNodes_100(b *testing.B) { benchmarkBuildSeqNodes(b, 100) }
func BenchmarkBuildSeqNodes_500(b *testing.B) { benchmarkBuildSeqNodes(b, 500) }

func benchmarkBuildSeqNodes(b *testing.B, n int) {
	b.Helper()
	node := makeBenchSeqNode(n)
	defs := catDefs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CollectionNodes(defs, node, false)
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
