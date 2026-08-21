package fieldtree

import "testing"

func TestBreadcrumbSegments_empty(t *testing.T) {
	if got := (Model{}).BreadcrumbSegments(); got != nil {
		t.Errorf("empty tree: want nil, got %v", got)
	}
}
func TestBreadcrumbSegments_nestedField(t *testing.T) {
	// Depth-1 field under an expanded depth-0 parent; yamlPath carries the full
	// path so BreadcrumbSegments returns both segments.
	tm := Model{
		Nodes: []Node{
			{Kind: KindField, Label: "deploy", YAMLPath: []string{"deploy"}, Depth: 0, Expanded: true},
			{Kind: KindField, Label: "strategy", YAMLPath: []string{"deploy", "strategy"}, Depth: 1, IsLeaf: true},
		},
		Cursor: 1,
	}
	got := tm.BreadcrumbSegments()
	want := []string{"deploy", "strategy"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBreadcrumbSegments_seqItem(t *testing.T) {
	tm := Model{
		Nodes:  []Node{{Kind: KindSeqItem, Label: "[2]", YAMLPath: []string{"[2]"}, Depth: 0}},
		Cursor: 0,
	}
	got := tm.BreadcrumbSegments()
	if len(got) != 1 || got[0] != "[2]" {
		t.Errorf("got %v, want [[2]]", got)
	}
}

func TestBreadcrumbSegments_addNew(t *testing.T) {
	tm := Model{
		Nodes:  []Node{{Kind: KindAddNew, Label: "+ add new", Depth: 0, IsLeaf: true}},
		Cursor: 0,
	}
	got := tm.BreadcrumbSegments()
	if len(got) != 1 || got[0] != "+ add new" {
		t.Errorf("got %v, want [+ add new]", got)
	}
}
