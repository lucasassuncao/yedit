package editor

import (
	"testing"

	"github.com/lucasassuncao/yedit/schema"
)

// ── BreadcrumbSegments ────────────────────────────────────────────────────────

func TestBreadcrumbSegments_empty(t *testing.T) {
	if got := (treeModel{}).BreadcrumbSegments(); got != nil {
		t.Errorf("empty tree: want nil, got %v", got)
	}
}

func TestBreadcrumbSegments_field(t *testing.T) {
	be := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	be = cursorToLabel(be, "httproutes")
	got := be.tree.BreadcrumbSegments()
	if len(got) != 1 || got[0] != "httproutes" {
		t.Errorf("got %v, want [httproutes]", got)
	}
}

func TestBreadcrumbSegments_nestedField(t *testing.T) {
	// Depth-1 field under an expanded depth-0 parent; yamlPath carries the full
	// path so BreadcrumbSegments returns both segments.
	tm := treeModel{
		nodes: []treeNode{
			{kind: treeNodeField, label: "deploy", yamlPath: []string{"deploy"}, depth: 0, expanded: true},
			{kind: treeNodeField, label: "strategy", yamlPath: []string{"deploy", "strategy"}, depth: 1, isLeaf: true},
		},
		cursor: 1,
	}
	got := tm.BreadcrumbSegments()
	want := []string{"deploy", "strategy"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBreadcrumbSegments_seqItem(t *testing.T) {
	tm := treeModel{
		nodes:  []treeNode{{kind: treeNodeSeqItem, label: "[2]", yamlPath: []string{"[2]"}, depth: 0}},
		cursor: 0,
	}
	got := tm.BreadcrumbSegments()
	if len(got) != 1 || got[0] != "[2]" {
		t.Errorf("got %v, want [[2]]", got)
	}
}

func TestBreadcrumbSegments_addNew(t *testing.T) {
	tm := treeModel{
		nodes:  []treeNode{{kind: treeNodeAddNew, label: "+ add new", depth: 0, isLeaf: true}},
		cursor: 0,
	}
	got := tm.BreadcrumbSegments()
	if len(got) != 1 || got[0] != "+ add new" {
		t.Errorf("got %v, want [+ add new]", got)
	}
}

// ── blockBreadcrumbPrefix ─────────────────────────────────────────────────────

func TestBlockBreadcrumbPrefix_singleEditor(t *testing.T) {
	be := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	m := model{blockEdits: []blockEditState{be}}
	if got := m.blockBreadcrumbPrefix(); got != nil {
		t.Errorf("single editor: want nil prefix, got %v", got)
	}
}

// TestBlockBreadcrumbPrefix_structChild verifies that drilling into a depth-0
// openable field does not duplicate its name in the prefix: the last segment of
// the parent's BreadcrumbSegments equals the child's key and must be dropped.
func TestBlockBreadcrumbPrefix_structChild(t *testing.T) {
	parent := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	parent = cursorToLabel(parent, "httproutes") // cursor on the field that was drilled into

	child := newBlockEdit(Config{}, blockSpec{
		key:     "httproutes",
		kind:    schema.KindDictionary,
		defs:    []schema.FieldDef{{YAMLName: "host", Kind: schema.KindPrimitive}},
		content: "httproutes:\n",
	}, 100, 40)

	m := model{blockEdits: []blockEditState{parent, child}}
	got := m.blockBreadcrumbPrefix()

	if len(got) != 1 || got[0] != "containerengine" {
		t.Errorf("got prefix %v, want [containerengine]", got)
	}
}

// TestBlockBreadcrumbPrefix_collectionChild verifies the collection case: the
// parent cursor is on a sub-field of a seq entry (yamlPath = ["[0]", "extensions"]).
// The prefix must keep the "[0]" segment but drop "extensions" (the child's key).
func TestBlockBreadcrumbPrefix_collectionChild(t *testing.T) {
	// Simulate a collection editor whose cursor sits on a depth-1 openable field.
	parent := blockEditState{
		key: "workers",
		tree: treeModel{
			nodes: []treeNode{
				{kind: treeNodeSeqItem, label: "[0]", yamlPath: []string{"[0]"}, depth: 0, expanded: true},
				{kind: treeNodeField, label: "extensions", yamlPath: []string{"[0]", "extensions"}, depth: 1, isLeaf: true, openable: true},
			},
			cursor: 1, // visible index of "extensions"
		},
	}
	child := blockEditState{key: "extensions"}

	m := model{blockEdits: []blockEditState{parent, child}}
	got := m.blockBreadcrumbPrefix()

	if len(got) != 2 || got[0] != "workers" || got[1] != "[0]" {
		t.Errorf("got prefix %v, want [workers [0]]", got)
	}
}

// ── full path assembly (no duplication) ──────────────────────────────────────

// TestBreadcrumbFullPath_noDuplication assembles the complete segment list the
// way breadcrumbHeader does and verifies that no key appears consecutively, which
// is the symptom of the duplication bug.
func TestBreadcrumbFullPath_noDuplication(t *testing.T) {
	parent := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	parent = cursorToLabel(parent, "httproutes")

	child := newBlockEdit(Config{}, blockSpec{
		key:     "httproutes",
		kind:    schema.KindDictionary,
		defs:    []schema.FieldDef{{YAMLName: "host", Kind: schema.KindPrimitive}},
		content: "httproutes:\n  web:\n    host: example.com\n",
	}, 100, 40)

	m := model{blockEdits: []blockEditState{parent, child}}
	prefix := m.blockBreadcrumbPrefix()
	segs := append(append(prefix, child.key), child.tree.BreadcrumbSegments()...)

	for i := 1; i < len(segs); i++ {
		if segs[i] == segs[i-1] {
			t.Errorf("duplicate consecutive segment %q at positions %d and %d in %v", segs[i], i-1, i, segs)
		}
	}
}
