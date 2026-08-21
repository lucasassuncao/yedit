package editor

import (
	"testing"

	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/schema"
)

func TestBreadcrumbSegments_field(t *testing.T) {
	be := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	be = cursorToLabel(be, "httproutes")
	got := be.tree.BreadcrumbSegments()
	if len(got) != 1 || got[0] != "httproutes" {
		t.Errorf("got %v, want [httproutes]", got)
	}
}

func TestBlockBreadcrumbPrefix_singleEditor(t *testing.T) {
	be := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	m := model{blockEdits: []blockEditState{be}}
	if got := m.blockBreadcrumbPrefix(); got != nil {
		t.Errorf("single editor: want nil prefix, got %v", got)
	}
}

// Drilling into a depth-0 openable field must not duplicate its name: the
// parent's last BreadcrumbSegments entry equals the child's key.
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

	if len(got) != 1 || got[0] != "yedit" {
		t.Errorf("got prefix %v, want [yedit]", got)
	}
}

// Collection case: with the parent cursor on ["[0]", "extensions"], the prefix
// keeps "[0]" and drops "extensions", the child's key.
func TestBlockBreadcrumbPrefix_collectionChild(t *testing.T) {
	// Simulate a collection editor whose cursor sits on a depth-1 openable field.
	parent := blockEditState{
		key: "workers",
		tree: fieldtree.Model{
			Nodes: []fieldtree.Node{
				{Kind: fieldtree.KindSeqItem, Label: "[0]", YAMLPath: []string{"[0]"}, Depth: 0, Expanded: true},
				{Kind: fieldtree.KindField, Label: "extensions", YAMLPath: []string{"[0]", "extensions"}, Depth: 1, IsLeaf: true, Openable: true},
			},
			Cursor: 1, // visible index of "extensions"
		},
	}
	child := blockEditState{key: "extensions"}

	m := model{blockEdits: []blockEditState{parent, child}}
	got := m.blockBreadcrumbPrefix()

	if len(got) != 2 || got[0] != "workers" || got[1] != "[0]" {
		t.Errorf("got prefix %v, want [workers [0]]", got)
	}
}

// Assembles the full segment list the way breadcrumbHeader does; no key may
// appear consecutively, the symptom of the duplication bug.
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
