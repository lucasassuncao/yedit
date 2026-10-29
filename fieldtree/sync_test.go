package fieldtree

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.yaml.in/yaml/v3"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
)

// A free-form map (map[string]string, map[string]map[string]any) declares no
// child fields: its keys are user data, not schema. New already gets this right
// - the tree is empty and the YAML editor takes focus. The regression is that
// the first resync repopulates it, flagging every user key as UNKNOWN.
func TestFreeFormMapStaysEmptyAfterResync(t *testing.T) {
	cases := map[string]string{
		"containerEnv": "containerEnv:\n  LOG_LEVEL: debug\n  NODE_ENV: development\n",
		"features":     "features:\n  ghcr.io/devcontainers/features/github-cli:1: {}\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			tm := New(schema.KindDictionary, nil, content, 20)
			require.True(t, tm.IsEmpty(), "New should yield an empty tree for a free-form map")

			valueNode := yamledit.BlockValueNode(content)
			require.NotNil(t, valueNode)

			got := SyncCheckedFromNode(tm, valueNode)

			var unknown []string
			for _, n := range got.Nodes() {
				if n.Kind == KindUnknown {
					unknown = append(unknown, n.Label)
				}
			}
			assert.Empty(t, unknown, "user-chosen map keys must not be flagged as unknown schema fields")
			assert.True(t, got.IsEmpty(), "resync must leave a free-form map tree empty")
		})
	}
}

// The same resync on a real object block must keep flagging genuinely unknown
// keys - the fix must not blunt that.
func TestObjectBlockStillFlagsUnknownKeys(t *testing.T) {
	defs := []schema.FieldDef{
		{YAMLName: "dockerfile", Kind: schema.KindPrimitive, Scalar: "string"},
		{YAMLName: "context", Kind: schema.KindPrimitive, Scalar: "string"},
	}
	content := "build:\n  dockerfile: Dockerfile\n  bogusKey: 1\n"

	tm := New(schema.KindObject, defs, content, 20)
	got := SyncCheckedFromNode(tm, yamledit.BlockValueNode(content))

	var unknown []string
	for _, n := range got.Nodes() {
		if n.Kind == KindUnknown {
			unknown = append(unknown, n.Label)
		}
	}
	assert.Equal(t, []string{"bogusKey"}, unknown)
}

// nestedUnknownDefs is a struct block with two inline-struct levels (a > b) so
// unknown keys can appear at depth 1 and depth 2.
func nestedUnknownDefs() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "a", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "b", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "x", Kind: schema.KindPrimitive, Scalar: "string"},
			}},
		}},
	}
}

// unknownRows collects the (yamlPath, depth) of every KindUnknown row.
func unknownRows(tm Model) (paths [][]string, depths []int) {
	for _, n := range tm.Nodes() {
		if n.Kind == KindUnknown {
			paths = append(paths, n.YAMLPath)
			depths = append(depths, n.Depth)
		}
	}
	return paths, depths
}

// TestInjectNestedUnknowns_DepthTwo guards that unknown keys inside an inline
// struct nested two levels deep are surfaced in the tree (they used to appear
// only for depth-1 parents, leaving deeper strays invisible until save).
func TestInjectNestedUnknowns_DepthTwo(t *testing.T) {
	is := assert.New(t)
	content := "svc:\n  a:\n    stray: 1\n    b:\n      x: hi\n      bogus: 2\n"
	tm := New(schema.KindObject, nestedUnknownDefs(), content, 40)

	paths, depths := unknownRows(tm)
	is.Contains(paths, []string{"a", "stray"}, "depth-1 unknown must be flagged")
	is.Contains(paths, []string{"a", "b", "bogus"}, "depth-2 unknown must be flagged")
	for i, p := range paths {
		is.Equal(len(p)-1, depths[i], "unknown row %v must render at its parent's depth+1", p)
	}
}

// TestInjectNestedUnknowns_NoneWhenClean guards the no-op path: a document
// matching the schema injects no unknown rows.
func TestInjectNestedUnknowns_NoneWhenClean(t *testing.T) {
	is := assert.New(t)
	tm := New(schema.KindObject, nestedUnknownDefs(), "svc:\n  a:\n    b:\n      x: hi\n", 40)

	paths, _ := unknownRows(tm)
	is.Empty(paths, "a schema-clean document must inject no unknown rows")
}

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
