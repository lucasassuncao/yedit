package yamledit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.yaml.in/yaml/v3"
)

func TestFindDuplicateMappingKey(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantDup string
	}{
		{"none", "a: 1\nb:\n  c: 2\n", ""},
		{"top level", "a: 1\na: 2\n", "a"},
		{"nested mapping", "a:\n  b: 1\n  b: 2\n", "a.b"},
		{"inside a sequence item", "list:\n  - x: 1\n    x: 2\n", "list.x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dup, ok := FindDuplicateMappingKey(parseValueNode(t, tc.src))
			assert.Equal(t, tc.wantDup != "", ok)
			assert.Equal(t, tc.wantDup, dup)
		})
	}

	_, ok := FindDuplicateMappingKey(nil)
	assert.False(t, ok, "a nil node has no duplicates")
}

func TestNodeHasContent(t *testing.T) {
	doc := parseValueNode(t, "s: x\nempty: \"\"\nnull: ~\nlist: []\nfull: [1]\nmap: {}\nalias: &a 1\nref: *a\n")
	child := func(key string) *yaml.Node {
		for i := 0; i+1 < len(doc.Content); i += 2 {
			if doc.Content[i].Value == key {
				return doc.Content[i+1]
			}
		}
		t.Fatalf("no key %q", key)
		return nil
	}

	assert.True(t, NodeHasContent(child("s")))
	assert.False(t, NodeHasContent(child("empty")))
	assert.False(t, NodeHasContent(child("null")))
	assert.False(t, NodeHasContent(child("list")))
	assert.True(t, NodeHasContent(child("full")))
	assert.False(t, NodeHasContent(child("map")))
	assert.True(t, NodeHasContent(child("ref")), "an alias always counts as content")
	assert.False(t, NodeHasContent(nil))
}

func TestBlockValueNode(t *testing.T) {
	n := BlockValueNode("server:\n  host: a\n")
	require.NotNil(t, n)
	assert.Equal(t, yaml.MappingNode, n.Kind)
	assert.Len(t, n.Content, 2)

	for _, content := range []string{"", "server: [unclosed\n"} {
		n := BlockValueNode(content)
		require.NotNil(t, n, "content %q", content)
		assert.Equal(t, yaml.MappingNode, n.Kind)
		assert.Empty(t, n.Content, "empty or broken content yields an empty mapping")
	}
}

func TestBlockValueNodeOrNil(t *testing.T) {
	n := BlockValueNodeOrNil("  \n")
	require.NotNil(t, n, "blank content is an empty block")
	assert.Equal(t, yaml.MappingNode, n.Kind)
	assert.Empty(t, n.Content)

	assert.Nil(t, BlockValueNodeOrNil("server: [unclosed\n"), "corrupt content is not masked")
	assert.NotNil(t, BlockValueNodeOrNil("server:\n  host: a\n"))
}
