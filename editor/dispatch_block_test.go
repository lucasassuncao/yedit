package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// dispatchTestBE returns a blockEditState for a struct block with two leaf fields.
func dispatchTestBE(t *testing.T) (blockEditState, int) {
	t.Helper()
	be := newBlockEdit(Config{NoDeleteConfirm: true}, structSpec(), 120, 40)
	idx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.isLeaf {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatal("dispatchTestBE: no leaf field found in tree")
	}
	return be, idx
}

// checkedFor returns the checked state of the tree node with the given label.
func checkedFor(be blockEditState, label string) (bool, bool) {
	for _, n := range be.tree.nodes {
		if n.label == label {
			return n.checked, true
		}
	}
	return false, false
}

func TestDispatchToggleField_pushesUndoAndLogs(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be, idx := dispatchTestBE(t)

	be2 := be.dispatch(ToggleField{NodeIdx: idx, Checked: false})

	must.NotEmpty(be2.undoStack, "dispatch(ToggleField) must push to undoStack")
	must.Len(be2.actionLog, 1, "actionLog len should be 1")
	is.IsType(ToggleField{}, be2.actionLog[0], "actionLog[0] must be ToggleField")
}

func TestDispatchUndoRedo(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be, idx := dispatchTestBE(t)
	fieldLabel := be.tree.nodes[idx].label
	checkedBefore, _ := checkedFor(be, fieldLabel)

	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: !checkedBefore})
	after, ok := checkedFor(be, fieldLabel)
	must.True(ok, "field %q not found after toggle", fieldLabel)
	must.NotEqual(checkedBefore, after, "toggle did not change checked state for %q", fieldLabel)

	be = be.dispatch(Undo{})
	after, ok = checkedFor(be, fieldLabel)
	must.True(ok, "field %q not found after Undo", fieldLabel)
	is.Equal(checkedBefore, after, "field %q must return to original state after Undo", fieldLabel)
	is.Equal("Undone.", be.statusMsg, "statusMsg after Undo")

	be = be.dispatch(Redo{})
	after, ok = checkedFor(be, fieldLabel)
	must.True(ok, "field %q not found after Redo", fieldLabel)
	is.NotEqual(checkedBefore, after, "field %q must toggle again after Redo", fieldLabel)
	is.Equal("Redone.", be.statusMsg, "statusMsg after Redo")
}

func TestDispatchActionLog_accumulates(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be, idx := dispatchTestBE(t)
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
	be = be.dispatch(Undo{})
	be = be.dispatch(Redo{})

	must.Len(be.actionLog, 3, "actionLog should have 3 entries")
	is.IsType(ToggleField{}, be.actionLog[0], "actionLog[0] must be ToggleField")
	is.IsType(Undo{}, be.actionLog[1], "actionLog[1] must be Undo")
	is.IsType(Redo{}, be.actionLog[2], "actionLog[2] must be Redo")
}

func TestReplayBlock(t *testing.T) {
	must := require.New(t)
	be, idx := dispatchTestBE(t)
	initial := be
	fieldLabel := be.tree.nodes[idx].label
	checkedBefore, _ := checkedFor(be, fieldLabel)

	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: !checkedBefore})
	// find the node again (position may shift after toggle)
	for i, n := range be.tree.nodes {
		if n.label == fieldLabel {
			idx = i
			break
		}
	}
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: checkedBefore})
	final := be

	replayed := replayBlock(initial, final.actionLog)

	finalYAML, _ := yaml.Marshal(final.node)
	replayedYAML, _ := yaml.Marshal(replayed.node)
	must.Equal(string(finalYAML), string(replayedYAML), "replay produced different node")
}

func TestReplayBlock_withSyncYAML(t *testing.T) {
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 120, 40)
	initial := be

	newContent := be.yamlEditor.Value() + "# comment\n"
	be = be.dispatch(SyncYAML{Content: newContent, Checkpoint: false})
	be = be.dispatch(SyncYAML{Content: newContent + "# second\n", Checkpoint: false})
	final := be

	replayed := replayBlock(initial, final.actionLog)

	finalYAML, _ := yaml.Marshal(final.node)
	replayedYAML, _ := yaml.Marshal(replayed.node)
	must.Equal(string(finalYAML), string(replayedYAML), "replayBlock with SyncYAML produced different node")
	must.Len(final.actionLog, 2, "both SyncYAML dispatches must appear in actionLog")
}

func TestSyncYAML_checkpointSavesUndo(t *testing.T) {
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 120, 40)

	be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value() + "# paste\n", Checkpoint: true})
	must.Len(be.undoStack, 1, "Checkpoint:true must push to undoStack")

	be2 := newBlockEdit(Config{}, structSpec(), 120, 40)
	be2 = be2.dispatch(SyncYAML{Content: be2.yamlEditor.Value() + "# keystroke\n", Checkpoint: false})
	must.Empty(be2.undoStack, "Checkpoint:false must not push to undoStack")
}

func TestDispatchNoEmptySequenceItem(t *testing.T) {
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{NoDeleteConfirm: true}, spec, 120, 40)

	// Add a new entry then navigate back (simulates committing with no fields added).
	be = be.dispatch(AddEntry{})
	must.GreaterOrEqual(seqItemCount(be), 2, "AddEntry did not add a new entry")

	// Navigate to first entry; this flushes the new (empty) entry.
	be = be.dispatch(NavigateEntry{Idx: 0})

	// No empty mapping items should remain in be.node.
	for i, item := range be.node.Content {
		if item.Kind == yaml.MappingNode && len(item.Content) == 0 {
			must.Fail("empty mapping item found after AddEntry + NavigateEntry", "index %d", i)
		}
	}
}
