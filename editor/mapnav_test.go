package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
)

func TestParseMapEntries(t *testing.T) {
	base := "portsAttributes:\n  \"3000\":\n    label: web\n    onAutoForward: notify\n  \"8080\":\n    label: api\n"
	entries := parseMapEntries("portsAttributes", base)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Label != "3000" || entries[1].Label != "8080" {
		t.Errorf("labels = [%q %q], want [3000 8080]", entries[0].Label, entries[1].Label)
	}
	if entries[0].Content != "  \"3000\":\n    label: web\n    onAutoForward: notify\n" {
		t.Errorf("entry[0].Content = %q", entries[0].Content)
	}
	// Round-trip back to the full block.
	if got := seqEntriesToBase("portsAttributes", entries); got != base {
		t.Errorf("round-trip mismatch:\n got %q\nwant %q", got, base)
	}
}

func TestParseMapEntries_notMap(t *testing.T) {
	if entries := parseMapEntries("x", "y:\n  a: 1\n"); entries != nil {
		t.Errorf("wrong prefix should yield nil, got %v", entries)
	}
}

func TestApplyToggleToMapEntry_remove(t *testing.T) {
	view := "portsAttributes:\n  \"3000\":\n    label: web\n    onAutoForward: notify\n"
	node := treeNode{yamlPath: []string{"3000", "onAutoForward"}}
	got := applyToggleToMapEntry(toggleCtx{key: "portsAttributes"}, node, false, view)
	want := "portsAttributes:\n  \"3000\":\n    label: web\n"
	if got != want {
		t.Errorf("after removing onAutoForward:\n got %q\nwant %q", got, want)
	}
}

func TestApplyToggleToMapEntry_add(t *testing.T) {
	view := "portsAttributes:\n  \"3000\":\n    label: web\n"
	node := treeNode{yamlPath: []string{"3000", "onAutoForward"}}
	got := applyToggleToMapEntry(toggleCtx{key: "portsAttributes"}, node, true, view)
	// An empty field renders as a null value ("onAutoForward:"), same as the
	// sequence navigator's appendLeafToMapping.
	want := "portsAttributes:\n  \"3000\":\n    label: web\n    onAutoForward:\n"
	if got != want {
		t.Errorf("after adding onAutoForward:\n got %q\nwant %q", got, want)
	}
}

func TestApplyToggleToMapEntry_addWithSnippet(t *testing.T) {
	view := "portsAttributes:\n  \"3000\":\n    label: web\n"
	node := treeNode{yamlPath: []string{"3000", "onAutoForward"}}
	ctx := toggleCtx{
		key:      "portsAttributes",
		snippets: map[string]string{"onAutoForward": "    onAutoForward: notify\n"},
	}
	got := applyToggleToMapEntry(ctx, node, true, view)
	// The snippet's value must be lifted, not nested as map[onAutoForward:notify].
	want := "portsAttributes:\n  \"3000\":\n    label: web\n    onAutoForward: notify\n"
	if got != want {
		t.Errorf("snippet value not lifted:\n got %q\nwant %q", got, want)
	}
}

func TestApplyToggleToMapEntry_normalizesFlowToBlock(t *testing.T) {
	// The entry arrived in flow style (e.g. an emptied {} that regained fields).
	view := "portsAttributes:\n  lucas: {label: '', onAutoForward: notify}\n"
	node := treeNode{yamlPath: []string{"lucas", "protocol"}}
	ctx := toggleCtx{key: "portsAttributes", snippets: map[string]string{"protocol": "    protocol: http\n"}}
	got := applyToggleToMapEntry(ctx, node, true, view)
	if strings.Contains(got, "{") {
		t.Errorf("entry should be block style, got flow: %q", got)
	}
	if !strings.Contains(got, "\n    onAutoForward: notify\n") || !strings.Contains(got, "\n    protocol: http\n") {
		t.Errorf("fields should be one per line: %q", got)
	}
}

func mapSpec() blockSpec {
	return blockSpec{
		key: "portsAttributes",
		defs: []schema.FieldDef{
			{YAMLName: "label", Kind: schema.KindPrimitive},
			{YAMLName: "onAutoForward", Kind: schema.KindPrimitive},
		},
		kind:    schema.KindDictionary,
		content: "portsAttributes:\n  \"3000\":\n    label: web\n    onAutoForward: notify\n  \"8080\":\n    label: api\n",
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
	want := "portsAttributes:\n  \"3000\":\n    label: web\n    onAutoForward: notify\n"
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
	be.yamlEditor.SetValue("portsAttributes:\n  lucas:\n    label: web\n    onAutoForward: notify\n")
	// Simulate the parse-gated keystroke: splice the edited entry into the node.
	if kn, vn, ok := parseEntryFromView(be.yamlEditor.Value(), be.coll.isMap); ok {
		setEntry(be.node, be.coll.isMap, be.coll.current, kn, vn)
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
		t.Fatalf("commit produced no command; errMsg=%q", be2.errMsg)
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
		kind:    schema.KindList,
		content: "workers:\n  - name: a\n    queue: q1\n  - name: b\n",
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
		kind:    schema.KindList,
		content: "workers:\n  - name: a\n    queue: q1\n  - name: b\n",
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
// YAML edge cases: tab indentation, anchors in collection content
// ---------------------------------------------------------------------------

func TestParseSeqEntries_tabIndented_doesNotPanic(t *testing.T) {
	// Tab-indented YAML is invalid; parseSeqEntries must not panic.
	seqBase := "categories:\n\t- name: foo\n"
	entries := parseSeqEntries("categories", seqBase)
	// May return nil (invalid YAML) or an empty slice — both are acceptable.
	_ = entries
}

func TestParseSeqEntries_anchorInEntry_doesNotPanic(t *testing.T) {
	// Anchors inside entries are unusual but should not cause a panic.
	seqBase := "categories:\n  - &ref\n    name: images\n  - *ref\n"
	entries := parseSeqEntries("categories", seqBase)
	// At least one entry should be parsed if the YAML is syntactically valid for gopkg.
	_ = entries
}

func TestParseMapEntries_colonInKey(t *testing.T) {
	// Map keys that contain colons (e.g. devcontainer feature keys) must round-trip.
	mapBase := "portsAttributes:\n  \"3000:80\":\n    label: web\n  \"8080\":\n    label: api\n"
	entries := parseMapEntries("portsAttributes", mapBase)
	if len(entries) != 2 {
		t.Fatalf("expected 2 map entries, got %d", len(entries))
	}
	if entries[0].Label != "3000:80" {
		t.Errorf("entry[0].Label = %q, want %q", entries[0].Label, "3000:80")
	}
	if entries[1].Label != "8080" {
		t.Errorf("entry[1].Label = %q, want %q", entries[1].Label, "8080")
	}
}
