package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/schema"
)

// This file exhaustively validates the tree interaction layer: every cursor-target
// state crossed with every key tree.Update handles. The grid is 11 targets × 6
// actions = 66 cells; each cell asserts the resulting treeAction (the contract the
// blockEditState reacts to), plus the key state change for expand/collapse/toggle.
//
// Targets (how the 4 schema Kinds + structure + state manifest in the tree):
//   leaf (Primitive / childless List|Dictionary): unchecked, checked
//   inline parent (Object+children):              collapsed/expanded × empty/content
//   openable (List|Dictionary +children):         empty, content   (list==map here)
//   seqItem (collection entry):                   collapsed, expanded
//   addNew row
//
// Actions tree.Update handles: up, down, left, right, enter, ctrl+d.

func mkTree(nodes []treeNode, cursorLabel string) treeModel {
	tm := treeModel{nodes: nodes, height: 40}
	for vi, ni := range tm.visibleNodes() {
		if tm.nodes[ni].label == cursorLabel {
			tm.cursor = vi
			break
		}
	}
	return tm
}

// targetSpec builds a minimal tree exhibiting one target state, with the cursor on it.
type targetSpec struct {
	name   string
	nodes  []treeNode
	cursor string
}

func matrixTargets() []targetSpec {
	leaf := func(checked bool) []treeNode {
		return []treeNode{{kind: treeNodeField, label: "p", depth: 0, isLeaf: true, checked: checked}}
	}
	inline := func(expanded, childChecked bool) []treeNode {
		return []treeNode{
			{kind: treeNodeField, label: "par", depth: 0, isLeaf: false, expanded: expanded},
			{kind: treeNodeField, label: "c", depth: 1, isLeaf: true, checked: childChecked},
		}
	}
	openable := func(checked bool) []treeNode {
		return []treeNode{{kind: treeNodeField, label: "op", depth: 0, isLeaf: true, openable: true, checked: checked}}
	}
	seqItem := func(expanded bool) []treeNode {
		return []treeNode{
			{kind: treeNodeSeqItem, label: "e0", depth: 0, isLeaf: false, expanded: expanded, checked: true, seqIdx: 0},
			{kind: treeNodeField, label: "c", depth: 1, isLeaf: true, checked: true},
			{kind: treeNodeAddNew, label: "add", depth: 0, isLeaf: true},
		}
	}
	return []targetSpec{
		{"addNew", []treeNode{{kind: treeNodeAddNew, label: "add", depth: 0, isLeaf: true}}, "add"},
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

// expectedAction is the ground-truth treeAction for every (target, action) cell,
// derived from the tree.Update handlers.
func expectedAction() map[string]map[string]treeAction {
	N, EXP, COL, TOG, ADD, DEL, OPEN := treeNoAction, treeExpanded, treeCollapsed, treeToggled, treeAddNew, treeDeleted, treeOpenChild
	return map[string]map[string]treeAction{
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
	tm := mkTree([]treeNode{{kind: treeNodeField, label: "par", isLeaf: false, expanded: false}, {kind: treeNodeField, label: "c", depth: 1, isLeaf: true}}, "par")
	tm, _ = tm.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	is.True(tm.nodes[0].expanded, "right did not expand the collapsed inline parent")
	// left on an expanded inline parent collapses it.
	tm, _ = tm.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	is.False(tm.nodes[0].expanded, "left did not collapse the expanded inline parent")
	// enter on an unchecked leaf checks it; ctrl+d on a checked leaf unchecks it.
	tm = mkTree([]treeNode{{kind: treeNodeField, label: "p", isLeaf: true}}, "p")
	tm, _ = tm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.True(tm.nodes[0].checked, "enter did not check the leaf")
	tm, _ = tm.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	is.False(tm.nodes[0].checked, "ctrl+d did not uncheck the leaf")
}

// TestMatrix_LeftMovesToParent verifies the non-action side effect of left on a
// nested node: when it can't collapse, it moves the cursor to the parent row.
func TestMatrix_LeftMovesToParent(t *testing.T) {
	is := assert.New(t)
	tm := mkTree([]treeNode{
		{kind: treeNodeField, label: "par", depth: 0, isLeaf: false, expanded: true},
		{kind: treeNodeField, label: "c", depth: 1, isLeaf: true},
	}, "c")
	vis := tm.visibleNodes()
	tm, act := tm.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	is.Equal(treeNoAction, act, "left on nested leaf action should be noAction")
	is.Equal("par", tm.nodes[vis[tm.cursor]].label, "left on nested leaf should move cursor to parent 'par'")
}

// TestMatrix_ToggleConsequenceAcrossContexts crosses the most common mutating
// action (toggle a leaf on, then off) with the three block contexts that have a
// tree: struct (KindObject), seq-of-struct (KindList), map-of-struct
// (KindDictionary). The downstream apply path differs per context, so each is
// driven through the real blockEditState and asserted.
func TestMatrix_ToggleConsequenceAcrossContexts(t *testing.T) {
	structDefs := []schema.FieldDef{
		{YAMLName: "name", Kind: schema.KindPrimitive},
		{YAMLName: "path", Kind: schema.KindPrimitive},
	}
	cases := []struct {
		name string
		spec blockSpec
		leaf string
	}{
		{"struct", blockSpec{key: "cfg", defs: structDefs, kind: schema.KindObject, content: "cfg:\n  name: x\n"}, "path"},
		{"seq", blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: `categories:
  - name: "a"
`}, "path"},
		{"map", blockSpec{key: "items", defs: catDefs(), kind: schema.KindDictionary, content: `items:
  k1:
    name: "a"
`}, "path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			is := assert.New(t)
			must := require.New(t)
			be := newBlockEdit(Config{}, tc.spec, 120, 40)
			be = expandAll(be)
			be = cursorToLabel(be, tc.leaf)

			// Enter toggles the leaf ON - it must appear in the editor YAML.
			be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
			must.Contains(be.yamlEditor.Value(), tc.leaf+":", "[%s] toggle ON did not add %q", tc.name, tc.leaf)

			// ctrl+d on the now-empty leaf removes it directly (empty value → no confirm).
			be = expandAll(be)
			be = cursorToLabel(be, tc.leaf)
			be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
			must.NotEqual(modeConfirming, be.mode, "[%s] empty leaf removal should not confirm", tc.name)
			is.NotContains(be.yamlEditor.Value(), tc.leaf+":", "[%s] toggle OFF did not remove %q", tc.name, tc.leaf)
		})
	}
}

// TestRestoreCursorToPathDoesNotMutateSharedNodes guards the tree's
// copy-on-write discipline: when restoreCursorToPath needs to expand a
// collapsed ancestor to reveal the target, it must clone the nodes slice
// instead of writing through the (possibly shared) backing array. Undo
// snapshots shallow-copy tree nodes and would otherwise be corrupted.
func TestRestoreCursorToPathDoesNotMutateSharedNodes(t *testing.T) {
	shared := []treeNode{
		{kind: treeNodeField, yamlPath: []string{"parent"}, label: "parent", depth: 0, isLeaf: false},
		{kind: treeNodeField, yamlPath: []string{"parent", "child"}, label: "child", depth: 1, isLeaf: true},
	}
	tm := treeModel{nodes: shared, height: 10}

	got := tm.restoreCursorToPath([]string{"parent", "child"})

	if !got.nodes[0].expanded {
		t.Error("restoreCursorToPath should expand the collapsed ancestor in its own copy")
	}
	vis := got.visibleNodes()
	if got.cursor >= len(vis) || got.nodes[vis[got.cursor]].label != "child" {
		t.Errorf("cursor should land on the revealed child, got cursor=%d", got.cursor)
	}
	if shared[0].expanded {
		t.Error("restoreCursorToPath mutated the shared input slice in place (copy-on-write violated)")
	}
}
