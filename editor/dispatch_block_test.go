package editor

import (
	"testing"

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
	be, idx := dispatchTestBE(t)

	be2 := be.dispatch(ToggleField{NodeIdx: idx, Checked: false})

	if len(be2.undoStack) == 0 {
		t.Fatal("dispatch(ToggleField) must push to undoStack")
	}
	if len(be2.actionLog) != 1 {
		t.Fatalf("actionLog len: want 1, got %d", len(be2.actionLog))
	}
	if _, ok := be2.actionLog[0].(ToggleField); !ok {
		t.Fatal("actionLog[0] must be ToggleField")
	}
}

func TestDispatchUndoRedo(t *testing.T) {
	be, idx := dispatchTestBE(t)
	fieldLabel := be.tree.nodes[idx].label
	checkedBefore, _ := checkedFor(be, fieldLabel)

	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: !checkedBefore})
	if after, ok := checkedFor(be, fieldLabel); !ok || after == checkedBefore {
		t.Fatalf("toggle did not change checked state for %q", fieldLabel)
	}

	be = be.dispatch(Undo{})
	if after, ok := checkedFor(be, fieldLabel); !ok || after != checkedBefore {
		t.Fatalf("field %q must return to original state after Undo", fieldLabel)
	}
	if be.statusMsg != "Undone." {
		t.Fatalf("statusMsg after Undo: want 'Undone.', got %q", be.statusMsg)
	}

	be = be.dispatch(Redo{})
	if after, ok := checkedFor(be, fieldLabel); !ok || after == checkedBefore {
		t.Fatalf("field %q must toggle again after Redo", fieldLabel)
	}
	if be.statusMsg != "Redone." {
		t.Fatalf("statusMsg after Redo: want 'Redone.', got %q", be.statusMsg)
	}
}

func TestDispatchActionLog_accumulates(t *testing.T) {
	be, idx := dispatchTestBE(t)
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
	be = be.dispatch(Undo{})
	be = be.dispatch(Redo{})

	if len(be.actionLog) != 3 {
		t.Fatalf("actionLog: want 3, got %d", len(be.actionLog))
	}
	if _, ok := be.actionLog[0].(ToggleField); !ok {
		t.Error("actionLog[0] must be ToggleField")
	}
	if _, ok := be.actionLog[1].(Undo); !ok {
		t.Error("actionLog[1] must be Undo")
	}
	if _, ok := be.actionLog[2].(Redo); !ok {
		t.Error("actionLog[2] must be Redo")
	}
}

func TestReplayBlock(t *testing.T) {
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
	if string(finalYAML) != string(replayedYAML) {
		t.Fatalf("replay produced different node:\nwant: %s\ngot:  %s", finalYAML, replayedYAML)
	}
}

func TestDispatchNoEmptySequenceItem(t *testing.T) {
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{NoDeleteConfirm: true}, spec, 120, 40)

	// Add a new entry then navigate back (simulates committing with no fields added).
	be = be.dispatch(AddEntry{})
	if seqItemCount(be) < 2 {
		t.Fatal("AddEntry did not add a new entry")
	}

	// Navigate to first entry; this flushes the new (empty) entry.
	be = be.dispatch(NavigateEntry{Idx: 0})

	// No empty mapping items should remain in be.node.
	for i, item := range be.node.Content {
		if item.Kind == yaml.MappingNode && len(item.Content) == 0 {
			t.Fatalf("empty mapping item at index %d found after AddEntry + NavigateEntry", i)
		}
	}
}
