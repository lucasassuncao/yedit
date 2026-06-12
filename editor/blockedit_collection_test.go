package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
)

func mapSpec() blockSpec {
	return blockSpec{
		key: "portsAttributes",
		defs: []schema.FieldDef{
			{YAMLName: "label", Kind: schema.KindPrimitive},
			{YAMLName: "onAutoForward", Kind: schema.KindPrimitive},
		},
		kind: schema.KindDictionary,
		content: `portsAttributes:
  "3000":
    label: web
    onAutoForward: notify
  "8080":
    label: api
`,
	}
}

func seqItemLabels(be blockEditState) []string {
	var out []string
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			out = append(out, n.label)
		}
	}
	return out
}

func TestMapBlockOpensAsNavigator(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	if !be.isMapNav() {
		t.Fatal("expected isMapNav() true")
	}
	if be.active != blockEditPanelTree {
		t.Errorf("map-with-children should open the tree navigator, not raw YAML (active=%d)", be.active)
	}
	if labels := seqItemLabels(be); len(labels) != 2 || labels[0] != "3000" || labels[1] != "8080" {
		t.Errorf("entry labels = %v, want [3000 8080]", labels)
	}
	want := `portsAttributes:
  "3000":
    label: web
    onAutoForward: notify
`
	if be.yamlEditor.Value() != want {
		t.Errorf("editor shows %q, want %q", be.yamlEditor.Value(), want)
	}
}

func TestMapBlockAddEntry(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	// Move to the [+ add new] row (third visible) and press Enter.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})
	labels := seqItemLabels(be)
	if len(labels) != 3 {
		t.Fatalf("after add, entries = %v, want 3", labels)
	}
	if labels[2] != "key3" {
		t.Errorf("new entry label = %q, want placeholder key3", labels[2])
	}
}

func fieldNodeChecked(be blockEditState, entryLabel, field string) (checked, found bool) {
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeField && len(n.yamlPath) == 2 && n.yamlPath[0] == entryLabel && n.yamlPath[1] == field {
			return n.checked, true
		}
	}
	return false, false
}

// TestMapBlockAddEntrySeedsCheckedField guards issue 1: a newly added entry's
// seeded field must show checked in the tree.
func TestMapBlockAddEntrySeedsCheckedField(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})
	checked, found := fieldNodeChecked(be, "key3", "label")
	if !found {
		t.Fatal("new entry's label node not found")
	}
	if !checked {
		t.Error("seeded label field should be checked in the tree after add")
	}
}

// TestMapBlockRenameUpdatesTreeLabel guards issue 2: renaming an entry's key in
// the YAML pane updates its label in the left panel.
func TestMapBlockRenameUpdatesTreeLabel(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be.yamlEditor.SetValue(`portsAttributes:
  lucas:
    label: web
    onAutoForward: notify
`)
	// Simulate the parse-gated keystroke: splice the edited entry into the node.
	if kn, vn, ok := parseEntryFromView(be.yamlEditor.Value(), be.coll.isMap); ok {
		setEntry(&be.node, be.coll.isMap, be.coll.current, kn, vn)
	}
	be.tree = be.resyncTreeFromYAML()
	if labels := seqItemLabels(be); len(labels) == 0 || labels[0] != "lucas" {
		t.Errorf("after rename, labels = %v, want first = lucas", labels)
	}
}

// TestMapBlockNoCrossEntryContamination guards that re-syncing the current entry
// does not corrupt a sibling entry's checkmarks.
func TestMapBlockNoCrossEntryContamination(t *testing.T) {
	// 3000 has label+onAutoForward; 8080 has only label.
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be.tree = be.resyncTreeFromYAML()

	if c, _ := fieldNodeChecked(be, "8080", "onAutoForward"); c {
		t.Error("8080.onAutoForward must stay unchecked (only 3000 has it)")
	}
	if c, _ := fieldNodeChecked(be, "8080", "label"); !c {
		t.Error("8080.label should be checked")
	}
	if c, _ := fieldNodeChecked(be, "3000", "onAutoForward"); !c {
		t.Error("3000.onAutoForward should be checked")
	}
}

func TestMapBlockCommitReassembles(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be2, cmd := be.commit()
	if cmd == nil {
		t.Fatalf("commit produced no command; editorErr=%v", be2.editorErr)
	}
	committed, ok := cmd().(blockEditCommittedMsg)
	if !ok {
		t.Fatalf("expected blockEditCommittedMsg")
	}
	if !strings.Contains(committed.Snippet, "\"3000\":") || !strings.Contains(committed.Snippet, "\"8080\":") {
		t.Errorf("committed snippet dropped entries:\n%s", committed.Snippet)
	}
}

// TestSeqBlockStillNavigates guards the existing sequence navigator against
// regression from the map generalization.
func TestSeqBlockStillNavigates(t *testing.T) {
	spec := blockSpec{
		key: "workers",
		defs: []schema.FieldDef{
			{YAMLName: "name", Kind: schema.KindPrimitive},
			{YAMLName: "queue", Kind: schema.KindPrimitive},
		},
		kind: schema.KindList,
		content: `workers:
  - name: a
    queue: q1
  - name: b
`,
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	if !be.isSeqNav() {
		t.Fatal("expected isSeqNav() true")
	}
	if be.active != blockEditPanelTree {
		t.Errorf("seq block should open the tree navigator (active=%d)", be.active)
	}
	if labels := seqItemLabels(be); len(labels) != 2 || labels[0] != "a" || labels[1] != "b" {
		t.Errorf("seq labels = %v, want [a b]", labels)
	}
}

// TestSeqBlockResyncNoContamination guards that the shared syncCurrentEntry path
// does not corrupt a sibling sequence item's checkmarks.
func TestSeqBlockResyncNoContamination(t *testing.T) {
	spec := blockSpec{
		key: "workers",
		defs: []schema.FieldDef{
			{YAMLName: "name", Kind: schema.KindPrimitive},
			{YAMLName: "queue", Kind: schema.KindPrimitive},
		},
		kind: schema.KindList,
		content: `workers:
  - name: a
    queue: q1
  - name: b
`,
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	be.tree = be.resyncTreeFromYAML()
	if c, _ := fieldNodeChecked(be, "b", "queue"); c {
		t.Error("worker b's queue must stay unchecked (only a has it)")
	}
	if c, _ := fieldNodeChecked(be, "a", "queue"); !c {
		t.Error("worker a's queue should be checked")
	}
}

// ---------------------------------------------------------------------------
// flushCurrentEntry - missing key header sets editorErr, does not update entries
// ---------------------------------------------------------------------------

func TestFlushCurrentEntry_missingHeader_setsErrMsg(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
	be := newBlockEdit(Config{}, spec, 100, 40)

	originalEntry := be.entryYAML(0)

	// Simulate user deleting the "categories:" prefix.
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("  - name: alpha_edited\n") // no "categories:" header

	result := be.flushCurrentEntry()

	if result.editorErr.kind == errNone {
		t.Error("expected editorErr to be set when key header is missing")
	}
	if result.entryYAML(0) != originalEntry {
		t.Error("entry 0 was modified despite missing key header - silent data loss")
	}
}

func TestFlushCurrentEntry_validContent_clearsErrMsg(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	be.editorErr = editorError{kind: errParse, message: "stale error from before"}
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")

	result := be.flushCurrentEntry()

	if result.editorErr.kind != errNone {
		t.Errorf("editorErr should be cleared on successful flush, got %q", result.editorErr.message)
	}
	if !strings.Contains(result.entryYAML(0), "alpha_edited") {
		t.Errorf("entry not updated: %q", result.entryYAML(0))
	}
}

// ---------------------------------------------------------------------------
// collectionDeriveTree - labels and checks of every entry are derived from the
// canonical node, so editing one entry never contaminates another.
// ---------------------------------------------------------------------------

func TestCollectionDerive_perEntryLabels(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
	be := newBlockEdit(Config{}, spec, 100, 40)

	// Edit entry 1 through the parse gate (the real keystroke path splices the
	// parsed entry into be.node), then re-derive the tree.
	be.coll.current = 1
	kn, vn, ok := parseEntryFromView("categories:\n  - name: beta_edited\n", false)
	if !ok {
		t.Fatal("parseEntryFromView failed on valid entry text")
	}
	setEntry(&be.node, false, 1, kn, vn)
	tm := be.collectionDeriveTree()

	labels := map[int]string{}
	for _, n := range tm.nodes {
		if n.kind == treeNodeSeqItem {
			labels[n.seqIdx] = n.label
		}
	}
	if labels[0] != "alpha" {
		t.Errorf("entry 0 label = %q, want alpha (must not be contaminated)", labels[0])
	}
	if labels[1] != "beta_edited" {
		t.Errorf("entry 1 label = %q, want beta_edited", labels[1])
	}
}
