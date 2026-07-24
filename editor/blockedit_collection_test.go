package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"

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
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be2, val, ok := be.commit()
	must.True(ok, "commit failed; editorErr=%v", be2.editorErr)
	snippet := nodeToContent(be2.key, val)
	is.Contains(snippet, "\"3000\":", "committed snippet dropped entries")
	is.Contains(snippet, "\"8080\":", "committed snippet dropped entries")
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
	is := assert.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	be.editorErr = editorError{kind: errParse, message: "stale error from before"}
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")

	result := be.flushCurrentEntry()

	is.Equal(errNone, result.editorErr.kind, "editorErr should be cleared on successful flush, got %q", result.editorErr.message)
	is.Contains(result.entryYAML(0), "alpha_edited", "entry not updated")
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

// ---------------------------------------------------------------------------
// Duplicate map keys: neither the per-keystroke sync path nor any error-
// clearing escape path (add-new, delete, preset) may persist a mapping with
// two identical keys.
// ---------------------------------------------------------------------------

// TestMapSyncRejectsDuplicateKey guards the per-keystroke sync path: renaming
// the current entry's key to one that already exists must keep the last good
// node and surface an error instead of splicing a second identical key.
func TestMapSyncRejectsDuplicateKey(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	// Current entry is "3000"; rename it to the existing "8080" via the
	// dispatch path used on every keystroke.
	be = be.dispatch(SyncYAML{Content: "portsAttributes:\n  \"8080\":\n    label: web\n"})
	if dup, ok := findDuplicateMappingKey(&be.node); ok {
		t.Fatalf("duplicate key %q reached the canonical node", dup)
	}
	if be.editorErr.kind != errParse {
		t.Errorf("expected errParse feedback, got kind=%d msg=%q", be.editorErr.kind, be.editorErr.message)
	}
	// Escape path: [+ add new] clears the sticky error and commits cleanly;
	// the earlier duplicate attempt must still not be present.
	be = be.dispatch(AddEntry{})
	committed, val, ok := be.commit()
	if !ok {
		t.Fatalf("commit after add-new should succeed, got %q", committed.editorErr.message)
	}
	if dup, has := findDuplicateMappingKey(val); has {
		t.Fatalf("commit produced duplicate key %q", dup)
	}
}

// TestCommitRejectsDuplicateMappingKey guards the commit backstop: a duplicate
// that reached the canonical node through any path must not be committed.
func TestCommitRejectsDuplicateMappingKey(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	kn := &yaml.Node{Kind: yaml.ScalarNode, Value: "8080"}
	vn := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "label"}, {Kind: yaml.ScalarNode, Value: "x"},
	}}
	be.node.Content = append(be.node.Content, kn, vn)
	committed, _, ok := be.commit()
	if ok {
		t.Fatal("commit must fail on duplicate mapping keys")
	}
	if committed.editorErr.kind != errCommit {
		t.Errorf("expected errCommit, got kind=%d msg=%q", committed.editorErr.kind, committed.editorErr.message)
	}
}

// ---------------------------------------------------------------------------
// flushCurrentEntry data-loss holes.
// ---------------------------------------------------------------------------

// TestFlushEmptyCollectionRejectsUnparsedText: with an empty collection, text
// that never parsed into a first entry must block the commit instead of being
// silently dropped.
func TestFlushEmptyCollectionRejectsUnparsedText(t *testing.T) {
	spec := mapSpec()
	spec.content = ""
	be := newBlockEdit(Config{}, spec, 100, 40)
	if be.coll.current != -1 {
		t.Fatalf("empty collection should open with current=-1, got %d", be.coll.current)
	}
	be.yamlEditor.SetValue("portsAttributes:\n  \"3000\"\n  broken indent\n")
	committed, _, ok := be.commit()
	if ok {
		t.Fatal("commit must be blocked when unparsed buffer text would be dropped")
	}
	if committed.editorErr.kind != errParse {
		t.Errorf("expected errParse, got kind=%d msg=%q", committed.editorErr.kind, committed.editorErr.message)
	}
}

// TestFlushEmptyCollectionPlaceholderIsClean: the untouched placeholder of an
// empty collection is a clean no-op at commit time.
func TestFlushEmptyCollectionPlaceholderIsClean(t *testing.T) {
	spec := mapSpec()
	spec.content = ""
	be := newBlockEdit(Config{}, spec, 100, 40)
	if _, _, ok := be.commit(); !ok {
		t.Fatal("committing the pristine empty-collection placeholder must succeed")
	}
}

// TestFlushEmptiedBufferBlocks: emptying the buffer of an existing entry must
// surface an error instead of silently resurrecting the old content.
func TestFlushEmptiedBufferBlocks(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be.yamlEditor.SetValue("")
	be = be.flushCurrentEntry()
	if be.editorErr.kind != errParse {
		t.Fatalf("expected errParse for emptied buffer, got kind=%d", be.editorErr.kind)
	}
}

// TestDeleteOtherEntryBlockedByInvalidCurrent: deleting a different entry
// while the current one holds invalid unsaved edits must refuse instead of
// silently reverting those edits. Deleting the current entry itself proceeds.
func TestDeleteOtherEntryBlockedByInvalidCurrent(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be.yamlEditor.SetValue("portsAttributes:\n  \"3000\": [broken\n")
	before := seqItemLabels(be)

	blocked := be.performEntryDelete(1) // delete "8080" while "3000" is invalid
	if got := seqItemLabels(blocked); len(got) != len(before) {
		t.Fatalf("delete of another entry must be blocked, labels=%v", got)
	}
	if blocked.editorErr.kind != errParse {
		t.Errorf("expected errParse, got kind=%d", blocked.editorErr.kind)
	}

	deleted := be.performEntryDelete(0) // deleting the invalid current entry is the remedy
	if got := seqItemLabels(deleted); len(got) != len(before)-1 {
		t.Errorf("deleting the current invalid entry must proceed, labels=%v", got)
	}
}

// TestEmptyMapKeyRefreshesTreeRow: a map entry whose key becomes the empty
// string must refresh its tree row (visible placeholder), not keep the stale
// previous label.
func TestEmptyMapKeyRefreshesTreeRow(t *testing.T) {
	be := newBlockEdit(Config{}, mapSpec(), 100, 40)
	be.yamlEditor.SetValue("portsAttributes:\n  \"\":\n    label: web\n")
	if kn, vn, ok := parseEntryFromView(be.yamlEditor.Value(), be.coll.isMap); ok {
		setEntry(&be.node, be.coll.isMap, be.coll.current, kn, vn)
	} else {
		t.Fatal("entry with empty key should parse")
	}
	be.tree = be.resyncTreeFromYAML()
	labels := seqItemLabels(be)
	if len(labels) == 0 || labels[0] != `""` {
		t.Errorf("labels = %v, want first = %q placeholder", labels, `""`)
	}
}
