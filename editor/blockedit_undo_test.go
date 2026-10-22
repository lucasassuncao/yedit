package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"github.com/lucasassuncao/bezel/shell"
	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
)

// Removing an empty-valued checked field saves an undo snap that restores it.
func TestUndo_structFieldRemove(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := blockSpec{
		key:  "configuration",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "output", Kind: schema.KindPrimitive},
			{YAMLName: "log-level", Kind: schema.KindPrimitive},
		},
		// output has an empty value so ctrl+d removes it directly (no confirm).
		content: `configuration:
  output: ""
  log-level: info
`,
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	want := be.yamlEditor.Value()

	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after field remove")
	is.NotContains(be.yamlEditor.Value(), "output:", "output still present after remove")

	be = be.restoreUndo()
	is.Equal(want, be.yamlEditor.Value(), "after undo YAML")
}

// Removing a field with content goes through the confirm dialog and still saves
// an undo snap that restores its original value.
func TestUndo_structFieldRemoveWithContent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	want := be.yamlEditor.Value()

	// ctrl+d on output (value "both") → shows confirm dialog.
	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	must.Equal(modeConfirming, be.mode, "expected modeConfirming after ctrl+d on field with content")
	// YAML unchanged until the user confirms.
	is.Contains(be.yamlEditor.Value(), "output:", "YAML must not change before confirmation")

	// Locate the output node and simulate the user confirming via dispatch.
	outputIdx := -1
	for i, n := range be.tree.Nodes {
		if n.Kind == fieldtree.KindField && n.Label == "output" {
			outputIdx = i
			break
		}
	}
	must.GreaterOrEqual(outputIdx, 0, "output node not found")
	be = be.dispatch(ToggleField{NodeIdx: outputIdx, Checked: false})

	is.NotContains(be.yamlEditor.Value(), "output:", "output still present after confirmed remove")
	must.NotEmpty(be.undoStack, "undoStack must be non-empty after confirmed remove")

	be = be.restoreUndo()
	is.Equal(want, be.yamlEditor.Value(), "after undo YAML")
}

// Adding an unchecked field saves an undo snap that removes it again.
func TestUndo_structFieldAdd(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
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
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after field add")
	is.Contains(be.yamlEditor.Value(), "output:", "output missing after add")

	be = be.restoreUndo()
	is.Equal(want, be.yamlEditor.Value(), "after undo YAML")
}

// Deleting a seq item saves an undo snap that restores the original entries.
func TestUndo_seqItemDelete(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := yamledit.NodeToContent("categories", &be.node)

	// ctrl+d on the first item (alpha) now confirms before deleting.
	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	must.Equal(modeConfirming, be.mode, "ctrl+d on a seq item should confirm first")
	be = be.dispatch(DeleteEntry{SeqIdx: 0})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after seq item delete")

	be = be.restoreUndo()
	gotBase := yamledit.NodeToContent("categories", &be.node)
	is.Equal(wantBase, gotBase, "after undo entries")
	is.Equal(2, seqItemCount(be), "after undo seq item count")
}

// Adding a seq item saves an undo snap that removes it again.
func TestUndo_seqItemAdd(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := yamledit.NodeToContent("categories", &be.node)

	// Navigate to [+ add new] and press Enter.
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	be, _ = be.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after seq item add")
	is.Equal(2, seqItemCount(be), "after add seq item count")

	be = be.restoreUndo()
	gotBase := yamledit.NodeToContent("categories", &be.node)
	is.Equal(wantBase, gotBase, "after undo entries")
	is.Equal(1, seqItemCount(be), "after undo seq item count")
}

// restoreUndo on an empty stack must not panic and must leave the state alone.
func TestRestoreUndo_emptyStackIsNoOp(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, blockSpec{key: "debug", kind: schema.KindPrimitive}, 100, 40)
	must.Empty(be.undoStack, "a freshly opened editor must have an empty undo stack")
	got := be.restoreUndo() // must not panic with an empty stack
	is.Empty(got.undoStack, "restoreUndo on an empty stack should leave it empty")
}

// Undo/redo round-trip: remove, undo, redo lands back on the post-remove state,
// and the redo itself can be undone.
func TestRedo_structFieldRemove(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := blockSpec{
		key:  "configuration",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "output", Kind: schema.KindPrimitive},
			{YAMLName: "log-level", Kind: schema.KindPrimitive},
		},
		// output has an empty value so ctrl+d removes it directly (no confirm).
		content: `configuration:
  output: ""
  log-level: info
`,
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	original := be.yamlEditor.Value()

	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	removed := be.yamlEditor.Value()
	must.NotContains(removed, "output:", "output still present after remove")

	be = be.restoreUndo()
	must.Equal(original, be.yamlEditor.Value(), "after undo YAML")
	must.NotEmpty(be.redoStack, "redoStack must be non-empty after undo")

	be = be.restoreRedo()
	is.Equal(removed, be.yamlEditor.Value(), "after redo YAML")

	// The redo itself is undoable.
	be = be.restoreUndo()
	is.Equal(original, be.yamlEditor.Value(), "after undoing the redo YAML")
}

// A new mutation after an undo discards the redo stack: the editor forks away
// from the undone state.
func TestRedo_clearedByNewMutation(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := blockSpec{
		key:  "configuration",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "output", Kind: schema.KindPrimitive},
			{YAMLName: "log-level", Kind: schema.KindPrimitive},
		},
		content: `configuration:
  output: ""
  log-level: info
`,
	}
	be := newBlockEdit(Config{}, spec, 100, 40)

	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	be = be.restoreUndo()
	must.NotEmpty(be.redoStack, "redoStack must be non-empty after undo")

	// New mutation (remove again) must clear the redo stack.
	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	is.Empty(be.redoStack, "redoStack must be cleared by a new mutation")
}

// In a YAML-only block, a keystroke after an undo re-checkpoints the restored
// state, so a second ctrl+u undoes the new edit rather than reporting "Nothing
// to undo." The new edit also invalidates the pending redo entry.
func TestUndo_treelessRetypeAfterUndo(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := blockSpec{
		key:     "log-level",
		kind:    schema.KindPrimitive,
		content: "log-level: info\n",
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	must.Equal(blockEditPanelYAML, be.active, "tree-less block must open in the YAML panel")
	baseline := be.yamlEditor.Value()

	typeRune := func(r rune) {
		be, _ = be.Update(tea.KeyPressMsg{Text: string(r), Code: r})
	}

	typeRune('x')
	be, _ = pressBEAction(t, be, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	must.Equal(baseline, be.yamlEditor.Value(), "first undo must restore the opening content")

	typeRune('y')
	is.Empty(be.redoStack, "a new edit after undo must discard the redo entry")

	be, _ = pressBEAction(t, be, tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	is.Equal("Undone.", be.statusMsg, "second undo must work after retyping")
	is.Equal(baseline, be.yamlEditor.Value(), "second undo must restore the pre-retype content")
}

// restoreRedo on an empty stack is a no-op.
func TestRestoreRedo_emptyStackIsNoOp(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	got := be.restoreRedo() // must not panic with an empty stack
	is.Empty(got.redoStack, "restoreRedo on an empty stack should leave it empty")
}

// Repeated Tab switches with no edit between them push a single snapshot instead
// of piling up identical ones.
// The focus moves a real Tab makes between the fields and the YAML editor.
var (
	toEditor = shell.FocusMsg{From: "fields", To: "editor"}
	toFields = shell.FocusMsg{From: "editor", To: "fields"}
)

func TestSaveUndoDeduplicatesSpeculativeCheckpoints(t *testing.T) {
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)

	for range 3 {
		be = be.focusPanel(toEditor) // tree → yaml: checkpoint
		be = be.focusPanel(toFields) // yaml → tree: no checkpoint
	}

	must.Len(be.undoStack, 1, "identical speculative checkpoints must be deduplicated")
}

// Restore-side dedupe: after a real change followed by a Tab checkpoint equal to
// the live state, one undo lands on the pre-change state.
func TestUndoAfterSpeculativeCheckpointRestoresInOnePress(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	want := be.yamlEditor.Value()

	outputIdx := -1
	for i, n := range be.tree.Nodes {
		if n.Kind == fieldtree.KindField && n.Label == "output" {
			outputIdx = i
			break
		}
	}
	must.GreaterOrEqual(outputIdx, 0, "output node not found")
	be = be.dispatch(ToggleField{NodeIdx: outputIdx, Checked: false})
	must.NotContains(be.yamlEditor.Value(), "output:", "toggle should remove the field")

	be = be.focusPanel(toEditor) // speculative checkpoint at the post-toggle state

	be = be.restoreUndo()
	is.Equal(want, be.yamlEditor.Value(), "one undo must restore the pre-toggle state")
	is.Equal("Undone.", be.statusMsg)
}

// When the stack holds only snapshots identical to the live state, undo drops
// them and reports "Nothing to undo." rather than a phantom "Undone.".
func TestRestoreUndoWithOnlyNoopSnapshotsReportsNothing(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)

	be = be.focusPanel(toEditor) // checkpoint identical to the live state
	must.Len(be.undoStack, 1)

	be = be.restoreUndo()
	is.Equal("Nothing to undo.", be.statusMsg)
	is.Empty(be.undoStack, "no-op snapshots must be dropped")
}

// Blocks that open with the YAML panel focused never fire a Tab checkpoint, so
// typing must still be reversible back to the content they opened with.
func TestTreelessBlock_UndoRestoresInitialContent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, blockSpec{key: "debug", kind: schema.KindPrimitive, content: "debug: false\n"}, 100, 40)
	must.Equal(blockEditPanelYAML, be.active, "tree-less block must focus the YAML panel")
	must.Empty(be.undoStack, "a freshly opened editor must have an empty undo stack")
	original := be.yamlEditor.Value()

	be, _ = be.updateKey(tea.KeyPressMsg{Text: "x", Code: 'x'})
	must.NotEqual(original, be.yamlEditor.Value(), "typing must change the buffer")

	be = be.restoreUndo()
	is.Equal(original, be.yamlEditor.Value(), "ctrl+u must restore the content the editor opened with")
	is.Equal("Undone.", be.statusMsg)
}
