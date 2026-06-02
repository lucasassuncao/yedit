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

	if !strings.Contains(be.seqBase, "name: existing") {
		t.Errorf("seqBase missing original entry:\n%s", be.seqBase)
	}
	if !strings.Contains(be.seqBase, "name: appended") {
		t.Errorf("seqBase missing appended entry:\n%s", be.seqBase)
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

	if !strings.Contains(be.seqBase, "name: existing") {
		t.Errorf("seqBase missing original entry:\n%s", be.seqBase)
	}
	if !strings.Contains(be.seqBase, "name: appended") {
		t.Errorf("seqBase missing appended entry:\n%s", be.seqBase)
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

	if be.undoSnap == nil {
		t.Fatal("undoSnap must be set after field remove")
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

	if be.undoSnap == nil {
		t.Fatal("undoSnap must be set when confirm dialog is shown")
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

	if be.undoSnap == nil {
		t.Fatal("undoSnap must be set after field add")
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
// and that restoreUndo restores the original seqBase.
func TestUndo_seqItemDelete(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantSeqBase := be.seqBase

	// ctrl+d on the first item (alpha).
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if be.undoSnap == nil {
		t.Fatal("undoSnap must be set after seq item delete")
	}

	be = be.restoreUndo()
	if be.seqBase != wantSeqBase {
		t.Errorf("after undo seqBase = %q, want %q", be.seqBase, wantSeqBase)
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
	wantSeqBase := be.seqBase

	// Navigate to [+ add new] and press Enter.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if be.undoSnap == nil {
		t.Fatal("undoSnap must be set after seq item add")
	}
	if got := seqItemCount(be); got != 2 {
		t.Errorf("after add got %d seq items, want 2", got)
	}

	be = be.restoreUndo()
	if be.seqBase != wantSeqBase {
		t.Errorf("after undo seqBase = %q, want %q", be.seqBase, wantSeqBase)
	}
	if got := seqItemCount(be); got != 1 {
		t.Errorf("after undo got %d seq items, want 1", got)
	}
}
