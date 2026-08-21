package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/fieldtree"
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

// Toggling a depth-3 leaf into an item holding only an empty "source:" must
// create or coerce both source and filter.
func TestAudit_DeepNestToggleUnderEmptyAncestors(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, blockSpec{
		key: "categories", defs: catDefs(), kind: schema.KindList,
		content: "categories:\n  - name: \"a\"\n    source:\n",
	}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, "regex")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.Contains(be.yamlEditor.Value(), "filter:", "deep nested toggle failed")
	is.Contains(be.yamlEditor.Value(), "regex:", "deep nested toggle failed")
}

// Toggling the only leaf off must prune the now-empty source mapping instead of
// leaving a dangling "source:".
func TestAudit_ToggleOffPrunesEmptyAncestors(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, blockSpec{
		key: "categories", defs: catDefs(), kind: schema.KindList,
		content: "categories:\n  - name: \"a\"\n    source:\n      path: /x\n",
	}, 120, 40)
	be = expandAll(be)
	idx := -1
	for i, n := range be.tree.Nodes {
		if n.Kind == fieldtree.KindField && n.Label == "path" {
			idx = i
			break
		}
	}
	must.NotEqual(-1, idx, "path node not found")
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
	is.NotContains(be.yamlEditor.Value(), "path:", "path not removed")
	is.NotContains(be.yamlEditor.Value(), "source:", "empty source should be pruned")
}

// Toggle on then off must leave no phantom keys.
func TestAudit_ToggleRoundTrip(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, blockSpec{
		key: "categories", defs: catDefs(), kind: schema.KindList,
		content: "categories:\n  - name: a\n",
	}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, "regex")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	be = expandAll(be)
	idx := -1
	for i, n := range be.tree.Nodes {
		if n.Kind == fieldtree.KindField && n.Label == "regex" {
			idx = i
			break
		}
	}
	must.NotEqual(-1, idx, "regex node not found after toggle ON")
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
	is.NotContains(be.yamlEditor.Value(), "regex:", "regex left behind after toggle OFF")
	is.NotContains(be.yamlEditor.Value(), "filter:", "filter not pruned")
	is.NotContains(be.yamlEditor.Value(), "source:", "source not pruned")
	is.Contains(be.yamlEditor.Value(), "name:", "name lost after round-trip")
}

// Map navigator counterpart: a map entry with an empty nested struct must accept
// a deep child.
func TestAudit_MapEntryDeepNestSymmetry(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, blockSpec{
		key: "items", defs: catDefs(), kind: schema.KindDictionary,
		content: "items:\n  k1:\n    source:\n",
	}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, "regex")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.Contains(be.yamlEditor.Value(), "filter:", "map entry deep nested toggle failed")
	is.Contains(be.yamlEditor.Value(), "regex:", "map entry deep nested toggle failed")
}

// Adding path then extensions must keep both: the fresh parent is not clobbered.
func TestAudit_ToggleSecondSiblingKeepsFirst(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, blockSpec{
		key: "categories", defs: catDefs(), kind: schema.KindList,
		content: "categories:\n  - name: \"a\"\n    source:\n",
	}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, "path")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	be = expandAll(be)
	be = cursorToLabel(be, "extensions")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.Contains(be.yamlEditor.Value(), "path:", "second sibling clobbered first")
	is.Contains(be.yamlEditor.Value(), "extensions:", "second sibling not added")
}

// Toggling hooks.before.shell into an item with no hooks must create every
// ancestor.
func TestAudit_ToggleParentStructOnThenChild(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, blockSpec{
		key: "categories", defs: catDefs(), kind: schema.KindList,
		content: "categories:\n  - name: \"a\"\n",
	}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, "shell")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.Contains(be.yamlEditor.Value(), "hooks:", "triple-nested struct creation failed")
	is.Contains(be.yamlEditor.Value(), "before:", "triple-nested struct creation failed")
	is.Contains(be.yamlEditor.Value(), "shell:", "triple-nested struct creation failed")
}

// --- interaction-layer probes (tree <-> blockEditState) ---

func expandSeqItems(be blockEditState) blockEditState {
	for i := range be.tree.Nodes {
		if be.tree.Nodes[i].Kind == fieldtree.KindSeqItem {
			be.tree.Nodes[i].Expanded = true
		}
	}
	return be
}

func expandAll(be blockEditState) blockEditState {
	for i := range be.tree.Nodes {
		be.tree.Nodes[i].Expanded = true
	}
	return be
}

// Enter/ctrl+d symmetry on an inline struct parent: whatever Enter creates,
// ctrl+d must remove.
func TestAudit_EnterThenCtrlDOnInlineParent(t *testing.T) {
	is := assert.New(t)
	content := `categories:
  - name: "a"
`
	be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: content}, 120, 40)
	be = expandSeqItems(be)
	be = cursorToLabel(be, "source")

	// Enter on an inline parent must expand it, not insert a stray empty "source:".
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.NotContains(be.yamlEditor.Value(), "source:", "Enter on inline parent created stray empty key")
	// And it must not leave a phantom checked state on the parent node.
	if n, ok := nodeByLabel(be, "source"); ok {
		is.False(n.Checked, "inline parent left with phantom checked=true after Enter")
	}
}

// After two toggles on one entry, ctrl+u must undo only the second. Restoring
// from a stale entry list would lose both edits.
func TestAudit_UndoAfterTwoTogglesKeepsFirst(t *testing.T) {
	is := assert.New(t)
	content := `categories:
  - name: "a"
    source:
`
	be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: content}, 120, 40)
	be = expandAll(be)

	be = cursorToLabel(be, "path")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	be = expandAll(be)
	be = cursorToLabel(be, "extensions")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	t.Logf("after two toggles:\n%s", be.yamlEditor.Value())

	be = be.restoreUndo()
	got := be.yamlEditor.Value()
	t.Logf("after one undo:\n%s", got)
	is.Contains(got, "path:", "undo lost the first toggle (path)")
	is.NotContains(got, "extensions:", "undo did not remove only the second toggle (extensions)")
}

// ctrl+d on a collection entry, the most destructive tree action, must confirm
// unless NoDeleteConfirm is set.
func TestAudit_EntryDeleteConfirms(t *testing.T) {
	spec := blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList,
		content: `categories:
  - name: "a"
  - name: "b"
`}

	// Default: confirm, then delete on confirm.
	be := newBlockEdit(Config{}, spec, 120, 40)
	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
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
	be2, _ = be2.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if be2.mode == modeConfirming {
		t.Error("NoDeleteConfirm should skip the entry-delete confirm")
	}
	if n := seqItemCount(be2); n != 1 {
		t.Errorf("entry not deleted with NoDeleteConfirm; have %d", n)
	}
}

// nodeByPathSuffix finds a field node whose yamlPath ends with the given segments.
func nodeByPathSuffix(be blockEditState, suffix ...string) (fieldtree.Node, bool) {
	for _, n := range be.tree.Nodes {
		if n.Kind != fieldtree.KindField || len(n.YAMLPath) < len(suffix) {
			continue
		}
		ok := true
		for i := range suffix {
			if n.YAMLPath[len(n.YAMLPath)-len(suffix)+i] != suffix[i] {
				ok = false
				break
			}
		}
		if ok {
			return n, true
		}
	}
	return fieldtree.Node{}, false
}

func confirmsOnCtrlD(content, label string) bool {
	be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: catDefs(), kind: schema.KindList, content: content}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, label)
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	return be.mode == modeConfirming
}

// A filled leaf confirms before removal and an empty one goes straight through,
// identically at top level and nested deep under hooks.before.
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

// Removing an inline parent must clear every descendant's checked state, while
// siblings keep theirs.
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
		be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
		idx := -1
		for i, n := range be.tree.Nodes {
			if n.Kind == fieldtree.KindField && n.Label == parent {
				idx = i
				break
			}
		}
		be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
		return be
	}
	checked := func(be blockEditState, sfx ...string) bool {
		n, _ := nodeByPathSuffix(be, sfx...)
		return n.Checked
	}

	is := assert.New(t)

	// Remove hooks: every hooks descendant clears; source descendants survive.
	be := remove("hooks")
	is.NotContains(be.yamlEditor.Value(), "hooks:", "hooks not removed from YAML")
	is.False(checked(be, "before", "shell"), "hooks descendants still checked after parent removal")
	is.False(checked(be, "after", "shell"), "hooks descendants still checked after parent removal")
	is.True(checked(be, "source", "path"), "source descendants should survive removing hooks")
	is.True(checked(be, "source", "filter", "regex"), "source descendants should survive removing hooks")

	// Remove source: deep descendants (path, filter.regex) clear; hooks survives.
	be = remove("source")
	is.False(checked(be, "source", "path"), "source descendants still checked after parent removal")
	is.False(checked(be, "source", "filter", "regex"), "source descendants still checked after parent removal")
	is.True(checked(be, "before", "shell"), "hooks.before.shell should survive removing source")

	// Remove only before: before.shell clears, after.shell stays.
	be = remove("before")
	is.False(checked(be, "before", "shell"), "before.shell should clear after removing before")
	is.True(checked(be, "after", "shell"), "after.shell should stay after removing before")
}

// TestToggleChildUnderEmptyParent reproduces the movelooper bug: a sequence item
// has an existing-but-empty nested struct key ("source:" with a null value).
// Toggling a child of that empty parent (source.path) must add it to the YAML.
func TestToggleChildUnderEmptyParent(t *testing.T) {
	is := assert.New(t)
	defs := []schema.FieldDef{
		{YAMLName: "name", Kind: schema.KindPrimitive},
		{YAMLName: "source", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "path", Kind: schema.KindPrimitive},
		}},
	}
	be := newBlockEdit(Config{}, blockSpec{
		key: "categories", defs: defs, kind: schema.KindList,
		content: "categories:\n  - name: \"lucas\"\n    source:\n",
	}, 120, 40)
	be = expandAll(be)
	be = cursorToLabel(be, "path")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.Contains(be.yamlEditor.Value(), "path:", "toggling source.path did not add the field")
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
