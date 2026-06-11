package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
)

func catDefs() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "name", Kind: schema.KindPrimitive},
		{YAMLName: "enabled", Kind: schema.KindPrimitive},
		{YAMLName: "source", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "path", Kind: schema.KindPrimitive},
			{YAMLName: "extensions", Kind: schema.KindList},
			{YAMLName: "filter", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "regex", Kind: schema.KindPrimitive},
				{YAMLName: "glob", Kind: schema.KindPrimitive},
			}},
		}},
		{YAMLName: "hooks", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "before", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "shell", Kind: schema.KindPrimitive},
			}},
			{YAMLName: "after", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "shell", Kind: schema.KindPrimitive},
			}},
		}},
	}
}

func seqNode(path ...string) treeNode {
	return treeNode{kind: treeNodeField, yamlPath: path, label: path[len(path)-1], depth: len(path) - 1, isLeaf: true}
}

func seqCtx() toggleCtx { return toggleCtx{key: "categories", childDefs: catDefs()} }

// TestAudit_DeepNestToggleUnderEmptyAncestors toggles a depth-3 leaf
// (source.filter.regex) into an item that has only an empty "source:". Both
// source and filter must be created/coerced.
func TestAudit_DeepNestToggleUnderEmptyAncestors(t *testing.T) {
	content := `categories:
  - name: "a"
    source:
`
	node := seqNode("a", "source", "filter", "regex")
	got := applyToggleToSeqItem(seqCtx(), node, true, content)
	if !strings.Contains(got, "filter:") || !strings.Contains(got, "regex:") {
		t.Errorf("deep nested toggle failed:\n%s", got)
	}
}

// TestAudit_ToggleOffPrunesEmptyAncestors toggles the only leaf off; the now-empty
// source mapping should be pruned so we don't leave a dangling "source:".
func TestAudit_ToggleOffPrunesEmptyAncestors(t *testing.T) {
	content := `categories:
  - name: "a"
    source:
      path: /x
`
	node := seqNode("a", "source", "path")
	got := applyToggleToSeqItem(seqCtx(), node, false, content)
	if strings.Contains(got, "path:") {
		t.Errorf("path not removed:\n%s", got)
	}
	if strings.Contains(got, "source:") {
		t.Errorf("empty source should be pruned:\n%s", got)
	}
}

// TestAudit_ToggleRoundTrip ON then OFF should return to the original.
func TestAudit_ToggleRoundTrip(t *testing.T) {
	content := `categories:
  - name: "a"
`
	node := seqNode("a", "source", "filter", "regex")
	on := applyToggleToSeqItem(seqCtx(), node, true, content)
	off := applyToggleToSeqItem(seqCtx(), node, false, on)
	if strings.TrimSpace(off) != strings.TrimSpace(content) {
		t.Errorf("round-trip not stable:\nwant:\n%q\ngot:\n%q", content, off)
	}
}

// TestAudit_MapEntryDeepNestSymmetry mirrors the deep-nest test for the map
// navigator: a map entry with an empty nested struct must accept a deep child.
func TestAudit_MapEntryDeepNestSymmetry(t *testing.T) {
	ctx := toggleCtx{key: "items", childDefs: catDefs()}
	content := `items:
  k1:
    source:
`
	node := seqNode("k1", "source", "filter", "regex")
	got := applyToggleToMapEntry(ctx, node, true, content)
	if !strings.Contains(got, "filter:") || !strings.Contains(got, "regex:") {
		t.Errorf("map entry deep nested toggle failed:\n%s", got)
	}
}

// TestAudit_ToggleSecondSiblingKeepsFirst adds path then extensions; both must
// survive (no clobber of the freshly-created parent).
func TestAudit_ToggleSecondSiblingKeepsFirst(t *testing.T) {
	content := `categories:
  - name: "a"
    source:
`
	c1 := applyToggleToSeqItem(seqCtx(), seqNode("a", "source", "path"), true, content)
	c2 := applyToggleToSeqItem(seqCtx(), seqNode("a", "source", "extensions"), true, c1)
	if !strings.Contains(c2, "path:") || !strings.Contains(c2, "extensions:") {
		t.Errorf("second sibling clobbered first:\n%s", c2)
	}
}

// TestAudit_ToggleParentStructOnAddsKey toggling an inline struct parent (hooks)
// ON via the apply layer should add the key (asStruct=false path) without panic.
func TestAudit_ToggleParentStructOnThenChild(t *testing.T) {
	content := `categories:
  - name: "a"
`
	// toggle hooks.before.shell directly into an item that has no hooks at all.
	node := seqNode("a", "hooks", "before", "shell")
	got := applyToggleToSeqItem(seqCtx(), node, true, content)
	if !strings.Contains(got, "hooks:") || !strings.Contains(got, "before:") || !strings.Contains(got, "shell:") {
		t.Errorf("triple-nested struct creation failed:\n%s", got)
	}
}

// --- interaction-layer probes (tree <-> blockEditState) ---

func expandSeqItems(be blockEditState) blockEditState {
	for i := range be.tree.nodes {
		if be.tree.nodes[i].kind == treeNodeSeqItem {
			be.tree.nodes[i].expanded = true
		}
	}
	return be
}

func expandAll(be blockEditState) blockEditState {
	for i := range be.tree.nodes {
		be.tree.nodes[i].expanded = true
	}
	return be
}

// TestAudit_EnterThenCtrlDOnInlineParent probes the Enter/ctrl+d symmetry on an
// inline struct parent. Whatever Enter creates, ctrl+d must be able to remove.
func TestAudit_EnterThenCtrlDOnInlineParent(t *testing.T) {
	content := `categories:
  - name: "a"
`
	be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: content}, 120, 40)
	be = expandSeqItems(be)
	be = cursorToLabel(be, "source")

	// Enter on an inline parent must expand it, not insert a stray empty "source:".
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
	if strings.Contains(be.yamlEditor.Value(), "source:") {
		t.Errorf("Enter on inline parent created stray empty key:\n%s", be.yamlEditor.Value())
	}
	// And it must not leave a phantom checked state on the parent node.
	if n, ok := nodeByLabel(be, "source"); ok && n.checked {
		t.Error("inline parent left with phantom checked=true after Enter")
	}
}

// TestAudit_UndoAfterTwoTogglesKeepsFirst: two toggles on the same entry, then one
// ctrl+u must undo only the second, keeping the first. If coll.entries is stale and
// restoreUndo reloads from it, both edits are lost.
func TestAudit_UndoAfterTwoTogglesKeepsFirst(t *testing.T) {
	content := `categories:
  - name: "a"
    source:
`
	be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: content}, 120, 40)
	be = expandAll(be)

	be = cursorToLabel(be, "path")
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
	be = expandAll(be)
	be = cursorToLabel(be, "extensions")
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
	t.Logf("after two toggles:\n%s", be.yamlEditor.Value())

	be = be.restoreUndo()
	got := be.yamlEditor.Value()
	t.Logf("after one undo:\n%s", got)
	if !strings.Contains(got, "path:") {
		t.Errorf("undo lost the first toggle (path):\n%s", got)
	}
	if strings.Contains(got, "extensions:") {
		t.Errorf("undo did not remove only the second toggle (extensions):\n%s", got)
	}
}

// TestAudit_HasCheckedDescendantCountsOpenable: an inline parent whose only
// content is a checked openable child (e.g. filter holding only "any") must count
// as having content - for both coloring and ctrl+d removal.
func TestAudit_HasCheckedDescendantCountsOpenable(t *testing.T) {
	nodes := []treeNode{
		{kind: treeNodeField, label: "filter", depth: 1, isLeaf: false},
		{kind: treeNodeField, label: "any", depth: 2, isLeaf: false, openable: true, checked: true},
	}
	if !hasCheckedDescendant(nodes, 0) {
		t.Error("filter with a checked openable child should count as having content")
	}
}

// TestAudit_OpenableListHasNoInlineChildren guards the cleanup: an openable
// list-of-struct field (filter.any) must not spawn phantom inline child nodes -
// it is drilled into, not expanded inline (matching openable maps).
func TestAudit_OpenableListHasNoInlineChildren(t *testing.T) {
	defs := []schema.FieldDef{
		{YAMLName: "filter", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "any", Kind: schema.KindList, Children: []schema.FieldDef{
				{YAMLName: "regex", Kind: schema.KindPrimitive},
			}},
		}},
	}
	nodes := flattenDefsAsTree(defs, nil, 0)
	for _, n := range nodes {
		if n.label == "regex" {
			t.Errorf("openable list spawned a phantom inline child %q", n.label)
		}
		if n.label == "any" {
			if !n.openable {
				t.Error("any should be openable")
			}
			if !n.isLeaf {
				t.Error("openable list should be leaf-like in the tree (no inline children)")
			}
		}
	}
}

// TestAudit_EntryDeleteConfirms: ctrl+d on a collection entry must confirm before
// deleting (the most destructive tree action), and skip the confirm when
// NoDeleteConfirm is set.
func TestAudit_EntryDeleteConfirms(t *testing.T) {
	spec := blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList,
		content: `categories:
  - name: "a"
  - name: "b"
`}

	// Default: confirm, then delete on confirm.
	be := newBlockEdit(Config{}, spec, 120, 40)
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if be.mode != modeConfirming {
		t.Fatalf("entry delete should confirm; mode=%d", be.mode)
	}
	if n := seqItemCount(be); n != 2 {
		t.Errorf("entry must not be deleted before confirmation; have %d", n)
	}
	be = be.dispatch(DeleteEntry{SeqIdx: 0})
	if n := seqItemCount(be); n != 1 {
		t.Errorf("entry not deleted after confirm; have %d", n)
	}

	// NoDeleteConfirm: delete immediately, no confirm dialog.
	be2 := newBlockEdit(Config{NoDeleteConfirm: true}, spec, 120, 40)
	be2, _ = be2.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if be2.mode == modeConfirming {
		t.Error("NoDeleteConfirm should skip the entry-delete confirm")
	}
	if n := seqItemCount(be2); n != 1 {
		t.Errorf("entry not deleted with NoDeleteConfirm; have %d", n)
	}
}

// nodeByPathSuffix finds a field node whose yamlPath ends with the given segments.
func nodeByPathSuffix(be blockEditState, suffix ...string) (treeNode, bool) {
	for _, n := range be.tree.nodes {
		if n.kind != treeNodeField || len(n.yamlPath) < len(suffix) {
			continue
		}
		ok := true
		for i := range suffix {
			if n.yamlPath[len(n.yamlPath)-len(suffix)+i] != suffix[i] {
				ok = false
				break
			}
		}
		if ok {
			return n, true
		}
	}
	return treeNode{}, false
}

func confirmsOnCtrlD(content, label string) bool {
	be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: content}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, label)
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyCtrlD})
	return be.mode == modeConfirming
}

// TestAudit_RemovalConfirmIsDepthConsistent: a filled leaf confirms before
// removal and an empty leaf removes directly - identically at top level and when
// nested deep under hooks.before. ("Its content will be lost" → empty has none.)
func TestAudit_RemovalConfirmIsDepthConsistent(t *testing.T) {
	cases := []struct {
		name, content, label string
		want                 bool
	}{
		{"filled-top", `categories:
  - name: "a"
`, "name", true},
		{"empty-top", "categories:\n  - name:\n", "name", false},
		{"filled-nested", `categories:
  - name: "a"
    hooks:
      before:
        shell: bash
`, "shell", true},
		{"empty-nested", `categories:
  - name: "a"
    hooks:
      before:
        shell:
`, "shell", false},
	}
	for _, tc := range cases {
		if got := confirmsOnCtrlD(tc.content, tc.label); got != tc.want {
			t.Errorf("[%s] confirm=%v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestAudit_RemoveParentResetsDescendantChecks: removing an inline parent must
// clear the checked state of ALL its descendants (the sync used to leave stale
// checkmarks when an ancestor vanished), while siblings keep theirs.
func TestAudit_RemoveParentResetsDescendantChecks(t *testing.T) {
	full := `categories:
  - name: "a"
    source:
      path: /x
      filter:
        regex: foo
    hooks:
      before:
        shell: bash
      after:
        shell: zsh
`

	remove := func(parent string) blockEditState {
		be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: full}, 120, 40)
		be = expandAll(be)
		be = cursorToLabel(be, parent)
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyCtrlD})
		idx := -1
		for i, n := range be.tree.nodes {
			if n.kind == treeNodeField && n.label == parent {
				idx = i
				break
			}
		}
		be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
		return be
	}
	checked := func(be blockEditState, sfx ...string) bool {
		n, _ := nodeByPathSuffix(be, sfx...)
		return n.checked
	}

	// Remove hooks: every hooks descendant clears; source descendants survive.
	be := remove("hooks")
	if strings.Contains(be.yamlEditor.Value(), "hooks:") {
		t.Error("hooks not removed from YAML")
	}
	if checked(be, "before", "shell") || checked(be, "after", "shell") {
		t.Error("hooks descendants still checked after parent removal")
	}
	if !checked(be, "source", "path") || !checked(be, "source", "filter", "regex") {
		t.Error("source descendants should survive removing hooks")
	}

	// Remove source: deep descendants (path, filter.regex) clear; hooks survives.
	be = remove("source")
	if checked(be, "source", "path") || checked(be, "source", "filter", "regex") {
		t.Error("source descendants still checked after parent removal")
	}
	if !checked(be, "before", "shell") {
		t.Error("hooks.before.shell should survive removing source")
	}

	// Remove only before: before.shell clears, after.shell stays.
	be = remove("before")
	if checked(be, "before", "shell") {
		t.Error("before.shell should clear after removing before")
	}
	if !checked(be, "after", "shell") {
		t.Error("after.shell should stay after removing before")
	}
}
