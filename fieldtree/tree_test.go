package fieldtree

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
)

// This file exhaustively validates the tree interaction layer: every cursor-target
// state crossed with every key tree.Update handles. The grid is 11 targets × 6
// actions = 66 cells; each cell asserts the resulting Action (the contract the
// the block editor reacts to), plus the key state change for expand/collapse/toggle.
//
// Targets (how the 4 schema Kinds + structure + state manifest in the tree):
//   leaf (Primitive / childless List|Dictionary): unchecked, checked
//   inline parent (Object+children):              collapsed/expanded × empty/content
//   openable (List|Dictionary +children):         empty, content   (list==map here)
//   seqItem (collection entry):                   collapsed, expanded
//   addNew row
//
// Actions tree.Update handles: up, down, left, right, enter, ctrl+d.

func mkTree(nodes []Node, cursorLabel string) Model {
	tm := Model{Nodes: nodes, Height: 40}
	for vi, ni := range tm.VisibleNodes() {
		if tm.Nodes[ni].Label == cursorLabel {
			tm.Cursor = vi
			break
		}
	}
	return tm
}

// targetSpec builds a minimal tree exhibiting one target state, with the cursor on it.
type targetSpec struct {
	name   string
	nodes  []Node
	cursor string
}

func matrixTargets() []targetSpec {
	leaf := func(checked bool) []Node {
		return []Node{{Kind: KindField, Label: "p", Depth: 0, IsLeaf: true, Checked: checked}}
	}
	inline := func(expanded, childChecked bool) []Node {
		return []Node{
			{Kind: KindField, Label: "par", Depth: 0, IsLeaf: false, Expanded: expanded},
			{Kind: KindField, Label: "c", Depth: 1, IsLeaf: true, Checked: childChecked},
		}
	}
	openable := func(checked bool) []Node {
		return []Node{{Kind: KindField, Label: "op", Depth: 0, IsLeaf: true, Openable: true, Checked: checked}}
	}
	seqItem := func(expanded bool) []Node {
		return []Node{
			{Kind: KindSeqItem, Label: "e0", Depth: 0, IsLeaf: false, Expanded: expanded, Checked: true, SeqIdx: 0},
			{Kind: KindField, Label: "c", Depth: 1, IsLeaf: true, Checked: true},
			{Kind: KindAddNew, Label: "add", Depth: 0, IsLeaf: true},
		}
	}
	return []targetSpec{
		{"addNew", []Node{{Kind: KindAddNew, Label: "add", Depth: 0, IsLeaf: true}}, "add"},
		{"seqItem-collapsed", seqItem(false), "e0"},
		{"seqItem-expanded", seqItem(true), "e0"},
		{"leaf-unchecked", leaf(false), "p"},
		{"leaf-checked", leaf(true), "p"},
		{"inline-collapsed-empty", inline(false, false), "par"},
		{"inline-collapsed-content", inline(false, true), "par"},
		{"inline-expanded-empty", inline(true, false), "par"},
		{"inline-expanded-content", inline(true, true), "par"},
		{"openable-empty", openable(false), "op"},
		{"openable-content", openable(true), "op"},
	}
}

func matrixActions() map[string]tea.KeyPressMsg {
	return map[string]tea.KeyPressMsg{
		"up":     {Code: tea.KeyUp},
		"down":   {Code: tea.KeyDown},
		"left":   {Code: tea.KeyLeft},
		"right":  {Code: tea.KeyRight},
		"enter":  {Code: tea.KeyEnter},
		"ctrl+d": {Code: 'd', Mod: tea.ModCtrl},
	}
}

// expectedAction is the ground-truth Action for every (target, action) cell,
// derived from the tree.Update handlers.
func expectedAction() map[string]map[string]Action {
	N, EXP, COL, TOG, ADD, DEL, OPEN := ActionNone, ActionExpanded, ActionCollapsed, ActionToggled, ActionAddNew, ActionDeleted, ActionOpenChild
	return map[string]map[string]Action{
		"addNew":                   {"up": N, "down": N, "left": N, "right": N, "enter": ADD, "ctrl+d": N},
		"seqItem-collapsed":        {"up": N, "down": N, "left": N, "right": EXP, "enter": N, "ctrl+d": DEL},
		"seqItem-expanded":         {"up": N, "down": N, "left": COL, "right": N, "enter": N, "ctrl+d": DEL},
		"leaf-unchecked":           {"up": N, "down": N, "left": N, "right": N, "enter": TOG, "ctrl+d": N},
		"leaf-checked":             {"up": N, "down": N, "left": N, "right": N, "enter": N, "ctrl+d": TOG},
		"inline-collapsed-empty":   {"up": N, "down": N, "left": N, "right": EXP, "enter": EXP, "ctrl+d": N},
		"inline-collapsed-content": {"up": N, "down": N, "left": N, "right": EXP, "enter": EXP, "ctrl+d": TOG},
		"inline-expanded-empty":    {"up": N, "down": N, "left": COL, "right": N, "enter": N, "ctrl+d": N},
		"inline-expanded-content":  {"up": N, "down": N, "left": COL, "right": N, "enter": N, "ctrl+d": TOG},
		"openable-empty":           {"up": N, "down": N, "left": N, "right": OPEN, "enter": OPEN, "ctrl+d": N},
		"openable-content":         {"up": N, "down": N, "left": N, "right": OPEN, "enter": OPEN, "ctrl+d": TOG},
	}
}

// TestMatrix_TreeActions validates all 66 (target × action) cells against the
// ground-truth table. A mismatch means the interaction layer changed behavior.
func TestMatrix_TreeActions(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	targets := matrixTargets()
	actions := matrixActions()
	expected := expectedAction()
	actionOrder := []string{"up", "down", "left", "right", "enter", "ctrl+d"}

	cells := 0
	for _, tgt := range targets {
		exp, ok := expected[tgt.name]
		must.True(ok, "no expected row for target %q", tgt.name)
		for _, act := range actionOrder {
			tm := mkTree(tgt.nodes, tgt.cursor)
			_, got := tm.Update(actions[act])
			is.Equal(exp[act], got, "[%s × %s] action", tgt.name, act)
			cells++
		}
	}
	is.Equal(66, cells, "validated %d cells, expected 66")
}

// TestMatrix_StateMutations checks that the actions which change tree state
// actually do so (expand sets expanded, collapse clears it, toggle flips checked).
func TestMatrix_StateMutations(t *testing.T) {
	is := assert.New(t)
	// right on a collapsed inline parent expands it.
	tm := mkTree([]Node{{Kind: KindField, Label: "par", IsLeaf: false, Expanded: false}, {Kind: KindField, Label: "c", Depth: 1, IsLeaf: true}}, "par")
	tm, _ = tm.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	is.True(tm.Nodes[0].Expanded, "right did not expand the collapsed inline parent")
	// left on an expanded inline parent collapses it.
	tm, _ = tm.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	is.False(tm.Nodes[0].Expanded, "left did not collapse the expanded inline parent")
	// enter on an unchecked leaf checks it; ctrl+d on a checked leaf unchecks it.
	tm = mkTree([]Node{{Kind: KindField, Label: "p", IsLeaf: true}}, "p")
	tm, _ = tm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.True(tm.Nodes[0].Checked, "enter did not check the leaf")
	tm, _ = tm.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	is.False(tm.Nodes[0].Checked, "ctrl+d did not uncheck the leaf")
}

// TestMatrix_LeftMovesToParent verifies the non-action side effect of left on a
// nested node: when it can't collapse, it moves the cursor to the parent row.
func TestMatrix_LeftMovesToParent(t *testing.T) {
	is := assert.New(t)
	tm := mkTree([]Node{
		{Kind: KindField, Label: "par", Depth: 0, IsLeaf: false, Expanded: true},
		{Kind: KindField, Label: "c", Depth: 1, IsLeaf: true},
	}, "c")
	vis := tm.VisibleNodes()
	tm, act := tm.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	is.Equal(ActionNone, act, "left on nested leaf action should be noAction")
	is.Equal("par", tm.Nodes[vis[tm.Cursor]].Label, "left on nested leaf should move cursor to parent 'par'")
}

// TestRestoreCursorToPathDoesNotMutateSharedNodes guards the tree's
// copy-on-write discipline: when restoreCursorToPath needs to expand a
// collapsed ancestor to reveal the target, it must clone the nodes slice
// instead of writing through the (possibly shared) backing array. Undo
// snapshots shallow-copy tree nodes and would otherwise be corrupted.
func TestRestoreCursorToPathDoesNotMutateSharedNodes(t *testing.T) {
	shared := []Node{
		{Kind: KindField, YAMLPath: []string{"parent"}, Label: "parent", Depth: 0, IsLeaf: false},
		{Kind: KindField, YAMLPath: []string{"parent", "child"}, Label: "child", Depth: 1, IsLeaf: true},
	}
	tm := Model{Nodes: shared, Height: 10}

	got := tm.restoreCursorToPath([]string{"parent", "child"})

	if !got.Nodes[0].Expanded {
		t.Error("restoreCursorToPath should expand the collapsed ancestor in its own copy")
	}
	vis := got.VisibleNodes()
	if got.Cursor >= len(vis) || got.Nodes[vis[got.Cursor]].Label != "child" {
		t.Errorf("cursor should land on the revealed child, got cursor=%d", got.Cursor)
	}
	if shared[0].Expanded {
		t.Error("restoreCursorToPath mutated the shared input slice in place (copy-on-write violated)")
	}
}
