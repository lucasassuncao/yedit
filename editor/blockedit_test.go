package editor

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucasassuncao/yedit/schema"
)

// stubPresets implements presets.Source for tests.
type stubPresets struct {
	data map[string]string // key: "field/name" → YAML snippet
}

func (s stubPresets) ListFields() []string { return nil }
func (s stubPresets) ListPresets(field string) []string {
	prefix := field + "/"
	var out []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, strings.TrimPrefix(k, prefix))
		}
	}
	return out
}
func (s stubPresets) PresetYAML(field, name string) (string, error) {
	if y, ok := s.data[field+"/"+name]; ok {
		return y, nil
	}
	return "", fmt.Errorf("not found")
}

// seqSpec builds a KindList blockSpec with one child field ("name").
func seqSpec(content string) blockSpec {
	return blockSpec{
		key:  "categories",
		kind: schema.KindList,
		defs: []schema.FieldDef{
			{YAMLName: "name", Kind: schema.KindPrimitive},
		},
		content: content,
	}
}

// filterSpec builds a self-referential KindList blockSpec: each filter item has a
// leaf "regex" and an openable "any" ([]filter), mirroring the workload Filter type.
func filterSpec(content string) blockSpec {
	return blockSpec{
		key:  "filters",
		kind: schema.KindList,
		defs: []schema.FieldDef{
			{YAMLName: "regex", Kind: schema.KindPrimitive},
			{YAMLName: "any", Kind: schema.KindList, Children: []schema.FieldDef{
				{YAMLName: "regex", Kind: schema.KindPrimitive},
			}},
		},
		content: content,
	}
}

// findFieldNode returns the first treeNodeField with the given label.
func findFieldNode(tm treeModel, label string) (treeNode, bool) {
	for _, n := range tm.nodes {
		if n.kind == treeNodeField && n.label == label {
			return n, true
		}
	}
	return treeNode{}, false
}

// cursorToFieldExpanded expands all seq items so field rows are visible, then
// positions the cursor on the named field.
func cursorToFieldExpanded(be blockEditState, label string) blockEditState {
	for i := range be.tree.nodes {
		if be.tree.nodes[i].kind == treeNodeSeqItem {
			be.tree.nodes[i].expanded = true
		}
	}
	for vi, ni := range be.tree.visibleNodes() {
		if be.tree.nodes[ni].kind == treeNodeField && be.tree.nodes[ni].label == label {
			be.tree.cursor = vi
			break
		}
	}
	return be
}

// TestOpenableChildReflectsContent verifies that an openable field (any/all)
// is "checked" (rendered active) only when it holds content, and muted when
// empty — fixing the bug where openable fields always looked active.
func TestOpenableChildReflectsContent(t *testing.T) {
	filled := newBlockEdit(Config{}, filterSpec("filters:\n  - regex: \"x\"\n    any:\n      - regex: \"y\"\n"), 100, 40)
	tm := filled.collectionDeriveTree()
	if n, ok := findFieldNode(tm, "any"); !ok || !n.checked {
		t.Errorf("filled 'any' should be checked/active; got ok=%v checked=%v", ok, n.checked)
	}

	empty := newBlockEdit(Config{}, filterSpec("filters:\n  - regex: \"x\"\n"), 100, 40)
	tm = empty.collectionDeriveTree()
	if n, ok := findFieldNode(tm, "any"); !ok || n.checked {
		t.Errorf("empty 'any' should be unchecked/muted; got ok=%v checked=%v", ok, n.checked)
	}
}

// TestCtrlDOnFilledOpenableAsksRemove verifies ctrl+d now acts on an openable
// field with content (opens the remove confirm), instead of being a no-op.
func TestCtrlDOnFilledOpenableAsksRemove(t *testing.T) {
	be := newBlockEdit(Config{}, filterSpec("filters:\n  - regex: \"x\"\n    any:\n      - regex: \"y\"\n"), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if be.mode != modeConfirming {
		t.Errorf("ctrl+d on a filled openable should open the remove confirm; mode=%d", be.mode)
	}
}

// TestCtrlDOnEmptyOpenableNoop verifies ctrl+d on an empty openable does nothing
// (there is no content to remove) and never opens a confirm.
func TestCtrlDOnEmptyOpenableNoop(t *testing.T) {
	be := newBlockEdit(Config{}, filterSpec("filters:\n  - regex: \"x\"\n"), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if be.mode != modeEditing {
		t.Errorf("ctrl+d on an empty openable should be a no-op; mode=%d", be.mode)
	}
}

// TestAppendPreset_addsEntriesToExisting verifies that appendPreset appends
// all entries from the preset after the existing entries and positions the
// cursor on the last entry.
func TestAppendPreset_addsEntriesToExisting(t *testing.T) {
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n",
	}}
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)

	be = be.openPresetPicker()
	be = be.appendPreset("extra")

	base := nodeToContent("categories", be.node)
	if !strings.Contains(base, "name: existing") {
		t.Errorf("entries missing original entry:\n%s", base)
	}
	if !strings.Contains(base, "name: appended") {
		t.Errorf("entries missing appended entry:\n%s", base)
	}

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	if seqCount != 2 {
		t.Errorf("tree has %d seq items, want 2", seqCount)
	}

	if be.tree.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (last entry)", be.tree.cursor)
	}

	if !strings.Contains(be.yamlEditor.Value(), "appended") {
		t.Errorf("yamlEditor not showing appended entry:\n%s", be.yamlEditor.Value())
	}

	if !be.dirty {
		t.Error("dirty should be true after appendPreset")
	}
}

// TestAppendPreset_indentMismatch verifies that append works correctly when the
// existing seqBase and the preset YAML use different indentation levels.
func TestAppendPreset_indentMismatch(t *testing.T) {
	// existing uses 4-space, preset uses 2-space
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n    enabled: null\n",
	}}
	spec := blockSpec{
		key:     "categories",
		kind:    schema.KindList,
		defs:    []schema.FieldDef{{YAMLName: "name", Kind: schema.KindPrimitive}},
		content: "categories:\n    - name: existing\n      enabled: true\n",
	}
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	be = be.appendPreset("extra")

	base2 := nodeToContent("categories", be.node)
	if !strings.Contains(base2, "name: existing") {
		t.Errorf("entries missing original entry:\n%s", base2)
	}
	if !strings.Contains(base2, "name: appended") {
		t.Errorf("entries missing appended entry:\n%s", base2)
	}
	if !strings.Contains(be.yamlEditor.Value(), "appended") {
		t.Errorf("yamlEditor not showing appended entry:\n%s", be.yamlEditor.Value())
	}
}

// TestAppendPreset_multiEntryPreset verifies that a preset with multiple
// entries adds all of them.
func TestAppendPreset_multiEntryPreset(t *testing.T) {
	stub := stubPresets{data: map[string]string{
		"categories/multi": "categories:\n  - name: alpha\n  - name: beta\n",
	}}
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	be = be.appendPreset("multi")

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	if seqCount != 3 {
		t.Errorf("tree has %d seq items, want 3 (1 existing + 2 from preset)", seqCount)
	}
	if be.tree.cursor != 2 {
		t.Errorf("cursor = %d, want 2", be.tree.cursor)
	}
}

// structSpec returns a KindObject blockSpec with two primitive fields.
func structSpec() blockSpec {
	return blockSpec{
		key:  "configuration",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "output", Kind: schema.KindPrimitive},
			{YAMLName: "log-level", Kind: schema.KindPrimitive},
		},
		content: "configuration:\n  output: both\n  log-level: info\n",
	}
}

func seqItemCount(be blockEditState) int {
	n := 0
	for _, nd := range be.tree.nodes {
		if nd.kind == treeNodeSeqItem {
			n++
		}
	}
	return n
}

// TestUndo_structFieldRemove verifies that removing an empty-valued checked
// field (no confirm dialog) saves an undo snap and restoreUndo brings it back.
func TestUndo_structFieldRemove(t *testing.T) {
	spec := blockSpec{
		key:  "configuration",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "output", Kind: schema.KindPrimitive},
			{YAMLName: "log-level", Kind: schema.KindPrimitive},
		},
		// output has an empty value so ctrl+d removes it directly (no confirm).
		content: "configuration:\n  output: \"\"\n  log-level: info\n",
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	want := be.yamlEditor.Value()

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after field remove")
	}
	if strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Errorf("output still present after remove: %q", be.yamlEditor.Value())
	}

	be = be.restoreUndo()
	if be.yamlEditor.Value() != want {
		t.Errorf("after undo YAML = %q, want %q", be.yamlEditor.Value(), want)
	}
}

// TestUndo_structFieldRemoveWithContent verifies that removing a field that has
// content (triggers confirm dialog) still saves an undo snap, and that
// restoreUndo brings the field back with its original value.
func TestUndo_structFieldRemoveWithContent(t *testing.T) {
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	want := be.yamlEditor.Value()

	// ctrl+d on output (value "both") → shows confirm dialog.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty when confirm dialog is shown")
	}
	if be.mode != modeConfirming {
		t.Fatal("expected modeConfirming after ctrl+d on field with content")
	}
	// YAML unchanged until the user confirms.
	if !strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Error("YAML must not change before confirmation")
	}

	// Locate the output node and simulate the user confirming.
	outputIdx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "output" {
			outputIdx = i
			break
		}
	}
	if outputIdx < 0 {
		t.Fatal("output node not found")
	}
	be = be.applyPendingRemove(outputIdx)

	if strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Errorf("output still present after confirmed remove: %q", be.yamlEditor.Value())
	}

	be = be.restoreUndo()
	if be.yamlEditor.Value() != want {
		t.Errorf("after undo YAML = %q, want %q", be.yamlEditor.Value(), want)
	}
}

// TestUndo_structFieldAdd verifies that adding an unchecked field saves an undo
// snap and that restoreUndo removes the field again.
func TestUndo_structFieldAdd(t *testing.T) {
	spec := blockSpec{
		key:  "configuration",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "output", Kind: schema.KindPrimitive},
			{YAMLName: "log-level", Kind: schema.KindPrimitive},
		},
		content: "configuration:\n  log-level: info\n",
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	want := be.yamlEditor.Value()

	// Navigate to output (AVAILABLE section) and add it with Enter.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after field add")
	}
	if !strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Errorf("output missing after add: %q", be.yamlEditor.Value())
	}

	be = be.restoreUndo()
	if be.yamlEditor.Value() != want {
		t.Errorf("after undo YAML = %q, want %q", be.yamlEditor.Value(), want)
	}
}

// TestUndo_seqItemDelete verifies that deleting a seq item saves an undo snap
// and that restoreUndo restores the original entries.
func TestUndo_seqItemDelete(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := nodeToContent("categories", be.node)

	// ctrl+d on the first item (alpha) now confirms before deleting.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if be.mode != modeConfirming {
		t.Fatalf("ctrl+d on a seq item should confirm first; mode=%d", be.mode)
	}
	be, _ = be.Update(pendingEntryDeleteMsg{seqIdx: 0})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after seq item delete")
	}

	be = be.restoreUndo()
	gotBase := nodeToContent("categories", be.node)
	if gotBase != wantBase {
		t.Errorf("after undo entries = %q, want %q", gotBase, wantBase)
	}
	if got := seqItemCount(be); got != 2 {
		t.Errorf("after undo got %d seq items, want 2", got)
	}
}

// TestUndo_seqItemAdd verifies that adding a seq item saves an undo snap and
// that restoreUndo removes the added item.
func TestUndo_seqItemAdd(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := nodeToContent("categories", be.node)

	// Navigate to [+ add new] and press Enter.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after seq item add")
	}
	if got := seqItemCount(be); got != 2 {
		t.Errorf("after add got %d seq items, want 2", got)
	}

	be = be.restoreUndo()
	gotBase := nodeToContent("categories", be.node)
	if gotBase != wantBase {
		t.Errorf("after undo entries = %q, want %q", gotBase, wantBase)
	}
	if got := seqItemCount(be); got != 1 {
		t.Errorf("after undo got %d seq items, want 1", got)
	}
}

// TestCollectionNav_RawSetValueDoesNotMutateNode verifies that replacing the
// editor text out-of-band (without a keystroke through updateKey, which is what
// parse-gates edits into the node) leaves the canonical node untouched until a
// real flush. Only navigation and commit reconcile the buffer into the node.
func TestCollectionNav_RawSetValueDoesNotMutateNode(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	original := be.entryYAML(0)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	if be.entryYAML(0) != original {
		t.Fatalf("canonical node was mutated by a raw SetValue; got %q, want %q",
			be.entryYAML(0), original)
	}
}

// TestCollectionNav_CommitFlushesAndSerializesAll verifies that commit() flushes
// the current buffer into entries and includes all entries in the snippet.
func TestCollectionNav_CommitFlushesAndSerializesAll(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	_, cmd := be.commit()
	msg := cmd().(blockEditCommittedMsg)

	if !strings.Contains(msg.Snippet, "name: alpha_edited") {
		t.Fatalf("snippet missing edited entry: %s", msg.Snippet)
	}
	if !strings.Contains(msg.Snippet, "name: beta") {
		t.Fatalf("snippet missing second entry: %s", msg.Snippet)
	}
}

// TestCollectionNav_DoubleCommitIdempotent is a regression test for the
// duplication bug: committing twice must produce identical snippets.
func TestCollectionNav_DoubleCommitIdempotent(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	be, cmd := be.commit()
	snippet1 := cmd().(blockEditCommittedMsg).Snippet

	// Simulate handleOverlayConfirmed re-sync (new architecture)
	be = be.resyncAfterCommit(snippet1)

	_, cmd2 := be.commit()
	snippet2 := cmd2().(blockEditCommittedMsg).Snippet

	if snippet1 != snippet2 {
		t.Fatalf("double commit diverged:\nfirst:  %q\nsecond: %q", snippet1, snippet2)
	}
	if strings.Count(snippet2, "name: beta") != 1 {
		t.Fatalf("duplication detected: %q", snippet2)
	}
}

// TestPrimitiveBlock_showsFieldItemAndHint verifies that a tree-less block shows
// the field itself in the left panel and its metadata in the hint panel, instead
// of the "(no fields)" / "select a field" placeholders.
func TestPrimitiveBlock_showsFieldItemAndHint(t *testing.T) {
	spec := blockSpec{
		key:     "debug",
		kind:    schema.KindPrimitive,
		def:     schema.FieldDef{YAMLName: "debug", Kind: schema.KindPrimitive, Scalar: "bool", Default: "false"},
		content: "debug: false\n",
	}
	be := newBlockEdit(Config{}, spec, 100, 40)

	if !be.tree.isEmpty() {
		t.Fatal("expected an empty tree for a primitive block")
	}

	left := be.fieldItemView()
	if !strings.Contains(left, "debug") {
		t.Errorf("left panel should name the field; got %q", left)
	}

	hint := be.hintContent()
	for _, want := range []string{"type", "bool", "default", "false"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint panel missing %q; got %q", want, hint)
		}
	}

	if view := be.View(nil); strings.Contains(view, "(no fields)") {
		t.Error("full view should no longer show the (no fields) placeholder")
	}
}

// TestRestoreUndo_emptyStackIsNoOp guards restoreUndo against an empty stack:
// the function must not panic and must return the state unchanged.
func TestRestoreUndo_emptyStackIsNoOp(t *testing.T) {
	be := newBlockEdit(Config{}, blockSpec{key: "debug", kind: schema.KindPrimitive}, 100, 40)
	if len(be.undoStack) != 0 {
		t.Fatal("a freshly opened editor must have an empty undo stack")
	}
	got := be.restoreUndo() // must not panic with an empty stack
	if len(got.undoStack) != 0 {
		t.Error("restoreUndo on an empty stack should leave it empty")
	}
}
