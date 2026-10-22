package fieldtree

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func labels(nodes []Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Label
	}
	return out
}

func TestApplySectionsGroupsByContent(t *testing.T) {
	nodes := []Node{
		{Kind: KindField, Label: "a", IsLeaf: true, Checked: true},
		{Kind: KindField, Label: "b", IsLeaf: true},
		{Kind: KindField, Label: "s"},
		{Kind: KindField, Label: "x", Depth: 1, IsLeaf: true, Checked: true},
		{Kind: KindField, Label: "m", Openable: true},
		{Kind: KindAddNew, Label: "+ add new"},
	}
	unknown := []Node{{Kind: KindUnknown, Label: "zzz"}}

	got := applySections(nodes, unknown)
	// s has no key of its own but a checked child, so it carries content.
	assert.Equal(t, []string{"ADDED", "a", "s", "x", "", "AVAILABLE", "b", "m", "+ add new", "", "UNKNOWN", "zzz"}, labels(got))
	assert.Equal(t, got, applySections(got, unknown), "re-applying strips the old headers and unknown rows first")
}

func TestApplySectionsOmitsEmptyGroups(t *testing.T) {
	nodes := []Node{
		{Kind: KindField, Label: "a", IsLeaf: true},
		{Kind: KindField, Label: "b", IsLeaf: true},
	}
	assert.Equal(t, []string{"AVAILABLE", "a", "b"}, labels(applySections(nodes, nil)))

	nodes[0].Checked, nodes[1].Checked = true, true
	assert.Equal(t, []string{"ADDED", "a", "b"}, labels(applySections(nodes, nil)))
}
