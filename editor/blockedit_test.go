package editor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"go.yaml.in/yaml/v3"

	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/render"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
	"github.com/lucasassuncao/yedit/yamlnode"
)

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

// findFieldNode returns the first fieldtree.KindField with the given label.
func findFieldNode(tm fieldtree.Model, label string) (fieldtree.Node, bool) {
	for _, n := range tm.Nodes() {
		if n.Kind == fieldtree.KindField && n.Label == label {
			return n, true
		}
	}
	return fieldtree.Node{}, false
}

// cursorToFieldExpanded expands all seq items so field rows are visible, then
// positions the cursor on the named field.
func cursorToFieldExpanded(be blockEditState, label string) blockEditState {
	for i := range be.tree.Nodes() {
		if be.tree.Node(i).Kind == fieldtree.KindSeqItem {
			be.tree = be.tree.WithNodeMutated(i, func(n *fieldtree.Node) { n.Expanded = true })
		}
	}
	for vi, ni := range be.tree.VisibleNodes() {
		if be.tree.Node(ni).Kind == fieldtree.KindField && be.tree.Node(ni).Label == label {
			be.tree = be.tree.MoveTo(vi)
			break
		}
	}
	return be
}

// An openable field renders active only when it holds content, and muted when
// empty. Openable fields used to always look active.
func TestOpenableChildReflectsContent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	filled := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
    any:
      - regex: "y"
`), 100, 40)
	tm := filled.collectionDeriveTree()
	n, ok := findFieldNode(tm, "any")
	must.True(ok, "filled 'any' field not found in tree")
	is.True(n.Checked, "filled 'any' should be checked/active")

	empty := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
`), 100, 40)
	tm = empty.collectionDeriveTree()
	n, ok = findFieldNode(tm, "any")
	must.True(ok, "empty 'any' field not found in tree")
	is.False(n.Checked, "empty 'any' should be unchecked/muted")
}

// ctrl+d on an openable field with content opens the remove confirm rather than
// doing nothing.
func TestCtrlDOnFilledOpenableAsksRemove(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
    any:
      - regex: "y"
`), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	is.Equal(modeConfirming, be.mode, "ctrl+d on a filled openable should open the remove confirm")
}

// ctrl+d on an empty openable has nothing to remove and never opens a confirm.
func TestCtrlDOnEmptyOpenableNoop(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
`), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	is.Equal(modeEditing, be.mode, "ctrl+d on an empty openable should be a no-op")
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
		content: `configuration:
  output: both
  log-level: info
`,
	}
}

func seqItemCount(be blockEditState) int {
	n := 0
	for _, nd := range be.tree.Nodes() {
		if nd.Kind == fieldtree.KindSeqItem {
			n++
		}
	}
	return n
}

// TestCollectionNav_RawSetValueDoesNotMutateNode verifies that replacing the
// editor text out-of-band, bypassing the parse gate in updateKey, leaves the
// canonical node untouched: only navigation and commit reconcile the buffer.
func TestCollectionNav_RawSetValueDoesNotMutateNode(t *testing.T) {
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	original := be.entryYAML(0)

	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	must.Equal(original, be.entryYAML(0), "canonical node was mutated by a raw SetValue")
}

// commit() flushes the current buffer and includes every entry in the snippet.
func TestCollectionNav_CommitFlushesAndSerializesAll(t *testing.T) {
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	_, val, ok := be.commit()
	must.True(ok, "commit failed")

	snippet := yamledit.NodeToContent(be.key, val)
	must.Contains(snippet, "name: alpha_edited", "snippet missing edited entry")
	must.Contains(snippet, "name: beta", "snippet missing second entry")
}

// TestCollectionNav_DoubleCommitIdempotent is a regression test for the
// duplication bug: committing twice must produce identical snippets.
func TestCollectionNav_DoubleCommitIdempotent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	be, val1, ok := be.commit()
	must.True(ok, "first commit failed")
	snippet1 := yamledit.NodeToContent(be.key, val1)

	// Re-sync after commit (mirrors the live flush path).
	be = be.resyncAfterCommit(snippet1)

	_, val2, ok := be.commit()
	must.True(ok, "second commit failed")
	snippet2 := yamledit.NodeToContent(be.key, val2)

	must.Equal(snippet1, snippet2, "double commit diverged")
	is.Equal(1, strings.Count(snippet2, "name: beta"), "duplication detected")
}

// A tree-less block shows the field itself on the left and its metadata in the
// hint panel, not the "(no fields)" / "select a field" placeholders.
func TestPrimitiveBlock_showsFieldItemAndHint(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := blockSpec{
		key:     "debug",
		kind:    schema.KindPrimitive,
		def:     schema.FieldDef{YAMLName: "debug", Kind: schema.KindPrimitive, Scalar: "bool"},
		content: "debug: false\n",
	}
	cfg := Config{
		// fieldPath must be "" for a tree-less block's own metadata (same
		// contract as the root list's hint panel) - a mock that ignored
		// fieldPath here would not have caught the regression where
		// blockedit_hint.go passed be.def.YAMLName instead of "".
		Metadata: MetadataFunc(func(block, fieldPath string) FieldMeta {
			if block == "debug" && fieldPath == "" {
				return FieldMeta{Type: "bool", Default: "false"}
			}
			return FieldMeta{}
		}),
	}
	be := newBlockEdit(cfg, spec, 100, 40)

	must.True(be.tree.IsEmpty(), "expected an empty tree for a primitive block")

	left := be.fieldItemView()
	is.Contains(left, "debug", "left panel should name the field")

	hint := be.hintContent()
	for _, want := range []string{"Type:", "bool", "Default:", "false"} {
		is.Contains(hint, want, "hint panel missing %q", want)
	}

	is.NotContains(be.View(nil), "(no fields)", "full view should no longer show the (no fields) placeholder")
}

// resyncTreeFromYAML is a tolerant, non-authoritative visual projection.

// A transiently unparseable buffer must neither panic nor wipe the tree's
// checked state: the resync leaves the last good visual state in place.
func TestResyncToleratesInvalidYAML_struct(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)

	before := map[string]bool{}
	for _, n := range be.tree.Nodes() {
		if n.Kind == fieldtree.KindField {
			before[n.Label] = n.Checked
		}
	}

	// Unterminated flow sequence - definitely invalid YAML.
	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("configuration:\n  output: [unterminated\n")

	tm := be.resyncTreeFromYAML() // must not panic

	after := map[string]bool{}
	for _, n := range tm.Nodes() {
		if n.Kind == fieldtree.KindField {
			after[n.Label] = n.Checked
		}
	}
	must.Len(after, len(before), "tree fields changed on invalid YAML")
	for k, v := range before {
		is.Equal(v, after[k], "checked state for %q changed on invalid YAML (state should be preserved)", k)
	}
}

// Same tolerance in a collection navigator: an unparseable current entry keeps
// its label and never mutates the canonical entries.
func TestResyncToleratesInvalidYAML_collection(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, seqSpec("categories:\n  - name: alpha\n"), 100, 40)
	nodeBefore := yamledit.NodeToContent(be.key, &be.node)

	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("categories:\n  - name: [unterminated\n")

	tm := be.resyncTreeFromYAML() // must not panic

	// The canonical node must be untouched: the tree is derived from it, not from
	// the (now invalid) buffer.
	must.Equal(nodeBefore, yamledit.NodeToContent(be.key, &be.node), "resync mutated canonical node")

	// The existing item label must survive an unparseable buffer.
	foundAlpha := false
	for _, n := range tm.Nodes() {
		if n.Kind == fieldtree.KindSeqItem && n.Label == "alpha" {
			foundAlpha = true
		}
	}
	is.True(foundAlpha, "seq item label \"alpha\" was lost on invalid YAML")
}

// ---------------------------------------------------------------------------
// editorH - must never return a negative value
// ---------------------------------------------------------------------------

func TestEditorH_nonNegative(t *testing.T) {
	is := assert.New(t)
	heights := []int{1, 2, 3, 5, 7, 10, 20}
	spec := seqSpec("categories:\n  - name: a\n")
	for _, h := range heights {
		be := newBlockEdit(Config{}, spec, 100, h)
		is.GreaterOrEqual(be.editorH(), 0, "editorH() must be >= 0 at terminal height %d", h)
	}
}

// ---------------------------------------------------------------------------
// ctrl+d on nested struct parent - must offer removal, not silently no-op
// ---------------------------------------------------------------------------

// A nested struct parent carries no checkbox of its own, which used to make
// ctrl+d a no-op. It must offer removal and delete the whole subtree, leaving
// siblings intact.
func TestCtrlDRemovesNestedParentBlock(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	defs := []schema.FieldDef{
		{YAMLName: "name", Kind: schema.KindPrimitive},
		{YAMLName: "hooks", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "before", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "shell", Kind: schema.KindPrimitive},
				{YAMLName: "run", Kind: schema.KindPrimitive},
			}},
			{YAMLName: "after", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "shell", Kind: schema.KindPrimitive},
			}},
		}},
	}
	content := `categories:
  - name: "lucas"
    hooks:
      before:
        shell: bash
        run: echo hi
      after:
        shell: bash
`
	be := newBlockEdit(Config{}, blockSpec{key: "categories", defs: defs, kind: schema.KindList, content: content}, 120, 40)

	// Expand every node so "before" is visible, then place the cursor on it.
	for i := range be.tree.Nodes() {
		be.tree = be.tree.WithNodeMutated(i, func(n *fieldtree.Node) { n.Expanded = true })
	}
	be = cursorToLabel(be, "before")

	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	must.Equal(modeConfirming, be.mode, "ctrl+d on nested parent did not offer removal")

	// Locate the captured "before" node index and confirm the removal.
	beforeIdx := -1
	for i, n := range be.tree.Nodes() {
		if n.Kind == fieldtree.KindField && n.Label == "before" {
			beforeIdx = i
			break
		}
	}
	must.GreaterOrEqual(beforeIdx, 0, "before node not found")
	be = be.dispatch(ToggleField{NodeIdx: beforeIdx, Checked: false})

	got := be.yamlEditor.Value()
	is.NotContains(got, "before:", "before block was not removed")
	is.Contains(got, "after:", "after block should remain")
}

// commitShapeSpec is a plain struct block used by the commit shape-validation
// tests: a "server" mapping with two scalar fields.
func commitShapeSpec() blockSpec {
	return blockSpec{
		key: "server",
		defs: []schema.FieldDef{
			{YAMLName: "host", Kind: schema.KindPrimitive, Scalar: "string"},
			{YAMLName: "port", Kind: schema.KindPrimitive, Scalar: "int"},
		},
		kind:    schema.KindObject,
		content: "server:\n  host: localhost\n",
	}
}

// A second top-level key typed into the editor used to be dropped silently. It
// must fail the commit with an error naming the stray key.
func TestCommit_RejectsExtraTopLevelKeys(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("server:\n  host: localhost\nlogging:\n  level: debug\n")

	committed, val, ok := be.commit()
	must.False(ok, "commit must reject a buffer with two top-level keys")
	is.Nil(val)
	is.Equal(errCommit, committed.editorErr.kind)
	is.Contains(committed.editorErr.message, "logging", "the error must name the stray key")
}

// A renamed block header used to be ignored, writing the content back under the
// original key. It must fail the commit with an explicit error.
func TestCommit_RejectsRenamedTopLevelKey(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("cluster:\n  host: localhost\n")

	committed, val, ok := be.commit()
	must.False(ok, "commit must reject a renamed top-level key")
	is.Nil(val)
	is.Equal(errCommit, committed.editorErr.kind)
	is.Contains(committed.editorErr.message, `"server"`, "the error must name the expected key")
}

// A buffer whose block header was deleted or replaced by a bare scalar must get
// an error explaining the missing header, not a misleading "internal error".
func TestCommit_RejectsMissingHeader(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	for _, buffer := range []string{"just a scalar", ""} {
		be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
		be.active = blockEditPanelYAML
		be.yamlEditor = be.yamlEditor.SetValue(buffer)

		committed, val, ok := be.commit()
		must.False(ok, "commit must reject buffer %q", buffer)
		is.Nil(val)
		is.Equal(errCommit, committed.editorErr.kind)
		is.Contains(committed.editorErr.message, `"server:"`, "the error must name the expected header for buffer %q", buffer)
	}
}

// TestCommit_AcceptsWellFormedBlock is the happy-path counterpart: a buffer
// with exactly the block's own key commits and yields its value node.
func TestCommit_AcceptsWellFormedBlock(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("server:\n  host: example.org\n  port: 8080\n")

	committed, val, ok := be.commit()
	must.True(ok, "commit failed; editorErr=%v", committed.editorErr)
	must.NotNil(val)
	is.Equal("server:\n  host: example.org\n  port: 8080\n", yamledit.NodeToContent("server", val))
}

// TestSOT_CommitPreservesLeadingComments: toggling a field on and then
// committing via the YAML buffer must not drop comment lines that appear above
// sibling keys in the buffer (regression against the EndLine off-by-one that
// used to swallow leading comments when ParseBlocks ran on the snippet).
func TestSOT_CommitPreservesLeadingComments(t *testing.T) {
	// The snippet has a comment above "log-file" that must survive an edit to "output".
	content := `cfg:
  output: stdout

  # log destination
  log-file: /var/log/app.log
`
	be := newBlockEdit(Config{}, blockSpec{
		key: "cfg", defs: cfgStructDefs(), kind: schema.KindObject, content: content,
	}, 120, 40)

	// Toggle "output" off; the YAML buffer is updated from the canonical node.
	be = expandAll(be)
	be = cursorToLabel(be, "output")
	idx := -1
	for i, n := range be.tree.Nodes() {
		if n.Kind == fieldtree.KindField && n.Label == "output" {
			idx = i
			break
		}
	}
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})

	is := assert.New(t)
	is.NotContains(be.yamlEditor.Value(), "output:", "output key still present after removal")
	is.Contains(be.yamlEditor.Value(), "# log destination", "leading comment lost after editing sibling")
}

// The hint panel's scroll bound comes from the content height, so the last
// panel-full of a long hint is reachable.
func TestHintScroll_ReachesEndOfLongContent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	longExample := strings.Repeat("line\n", 40)
	cfg := Config{
		EnableHints: true,
		Metadata: MetadataFunc(func(block, fieldPath string) FieldMeta {
			return FieldMeta{Description: "toggle", Example: longExample}
		}),
	}
	be := newBlockEdit(cfg, blockSpec{key: "debug", kind: schema.KindPrimitive, content: "debug: false\n"}, 100, 40)
	be.prevActive = be.active
	be.active = blockEditPanelHint

	lines := strings.Count(strings.TrimSuffix(be.hintContent(), "\n"), "\n") + 1
	wantMax := lines - be.hintH()
	must.Greater(wantMax, be.hintH()-1, "test setup: content must exceed twice the panel height")

	down := tea.KeyPressMsg{Code: tea.KeyDown}
	for range lines * 2 {
		be = be.scrollHint(down)
	}
	is.Equal(wantMax, be.hintScroll, "scroll must reach the last panel-full of the hint content")
}

// The "h" hint toggle mirrors the root list view, restoring focus off a hidden
// panel, and must not fire while the YAML panel has focus.
func TestHintToggle_HKeyShowsAndHides(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	cfg := Config{
		EnableHints: true,
		Metadata: MetadataFunc(func(block, fieldPath string) FieldMeta {
			return FieldMeta{Description: "toggle"}
		}),
	}
	be := newBlockEdit(cfg, blockSpec{key: "debug", kind: schema.KindPrimitive, content: "debug: false\n"}, 100, 40)
	must.True(be.showHint, "EnableHints: true should show the hint panel on start")
	be.active = blockEditPanelTree // debug is tree-less and opens on the YAML panel; move off it to test the toggle

	h := tea.KeyPressMsg{Text: "h", Code: 'h'}
	be, _ = pressBEAction(t, be, h)
	must.False(be.showHint, "pressing h should hide the hint panel")
	is.NotContains(be.View(nil), "Hint/Example", "hint panel should be hidden after pressing h")

	be, _ = pressBEAction(t, be, h)
	must.True(be.showHint, "pressing h again should re-enable the hint panel")
	is.Contains(be.View(nil), "Hint/Example", "hint panel should be visible after toggling back on")

	// Hiding while the hint panel has focus must restore focus to the panel
	// that had it before.
	be.prevActive = blockEditPanelTree
	be.active = blockEditPanelHint
	be, _ = pressBEAction(t, be, h)
	is.Equal(blockEditPanelTree, be.active, "hiding the focused hint panel should restore the previous focus")

	// "h" typed into the YAML editor must insert the character, not toggle.
	be.showHint = true
	be.active = blockEditPanelYAML
	before := be.showHint
	be, _ = be.updateKey(h)
	is.Equal(before, be.showHint, "h while the YAML panel is focused must not toggle the hint panel")
}

// esc on the hint returns to the pane it came from: it must not leave the
// block, nor ask to discard its changes.
func TestEscOnTheHintReturnsToThePreviousPane(t *testing.T) {
	be := newBlockEdit(Config{EnableHints: true}, ceStructSpec(), 100, 40)
	be.showHint = true
	be.dirty = true
	be.prevActive, be.active = blockEditPanelYAML, blockEditPanelHint

	be, cmd := pressBEAction(t, be, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, blockEditPanelYAML, be.active)
	require.Equal(t, modeEditing, be.mode, "no discard prompt")
	require.False(t, be.sh.HasOverlay())
	if cmd != nil {
		_, left := cmd().(blockEditDiscardedMsg)
		require.False(t, left, "esc must not leave the block")
	}
}

// ctrl+h is not offered on the hint, but it still gives the keys back: it is
// how the reader got there, and the same key undoes it.
func TestCtrlHOnTheHintStillReturns(t *testing.T) {
	be := newBlockEdit(Config{EnableHints: true}, ceStructSpec(), 100, 40)
	be.showHint = true
	be.prevActive, be.active = blockEditPanelTree, blockEditPanelHint

	be, _ = be.Update(tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl})
	require.Equal(t, blockEditPanelTree, be.active)
}

// cfgStructDefs is a struct (KindObject) block shaped like movelooper's
// Configuration plus a nested struct, for single-source-of-truth invariants.
func cfgStructDefs() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "output", Kind: schema.KindPrimitive},
		{YAMLName: "log-file", Kind: schema.KindPrimitive},
		{YAMLName: "source", Kind: schema.KindObject, Children: []schema.FieldDef{
			{YAMLName: "path", Kind: schema.KindPrimitive},
			{YAMLName: "filter", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "regex", Kind: schema.KindPrimitive},
			}},
		}},
	}
}

// keyExistsInNode reports whether the mapping path resolves to a present key in
// valueNode - the structural ground truth a leaf's checked flag must mirror.
func keyExistsInNode(valueNode *yaml.Node, path []string) bool {
	cur := valueNode
	for i := 0; i < len(path)-1 && cur != nil; i++ {
		cur = yamlnode.ChildByKey(cur, path[i])
	}
	if cur == nil || len(path) == 0 {
		return false
	}
	return yamlnode.ChildByKey(cur, path[len(path)-1]) != nil
}

// assertTreeMatchesNode is the core invariant: for every leaf field node, its
// checked flag equals key-presence in be.node, and the buffer is valid YAML.
func assertTreeMatchesNode(t *testing.T, be blockEditState) {
	t.Helper()
	if err := validateSnippetText(be.yamlEditor.Value()); err != nil {
		t.Errorf("buffer is not valid YAML after a tree action: %v\n%s", err, be.yamlEditor.Value())
	}
	for _, n := range be.tree.Nodes() {
		if n.Kind != fieldtree.KindField || !n.IsLeaf {
			continue
		}
		want := keyExistsInNode(&be.node, n.YAMLPath)
		if n.Checked != want {
			t.Errorf("tree/node disagree for %v: tree.checked=%v node-has-key=%v",
				n.YAMLPath, n.Checked, want)
		}
	}
}

// assertCollTreeMatchesNode is the collection invariant: every entry's label and
// every child's checked flag mirror the canonical node.
func assertCollTreeMatchesNode(t *testing.T, be blockEditState) {
	t.Helper()
	isMap := be.coll.isMap
	nodes := be.tree.Nodes()
	for i := 0; i < len(nodes); i++ {
		if nodes[i].Kind != fieldtree.KindSeqItem {
			continue
		}
		seqIdx := nodes[i].SeqIdx
		if want := yamledit.EntryLabel(&be.node, isMap, seqIdx); nodes[i].Label != want {
			t.Errorf("entry %d label %q != node label %q", seqIdx, nodes[i].Label, want)
		}
		entry := yamledit.EntryValueNode(&be.node, isMap, seqIdx)
		for j := i + 1; j < len(nodes) && nodes[j].Depth > 0; j++ {
			c := nodes[j]
			if c.Kind != fieldtree.KindField || !c.IsLeaf || len(c.YAMLPath) < 2 {
				continue
			}
			want := keyExistsInNode(entry, c.YAMLPath[1:])
			if c.Checked != want {
				t.Errorf("entry %d child %v checked=%v want %v", seqIdx, c.YAMLPath, c.Checked, want)
			}
		}
	}
}

// FuzzStructInvariants feeds random action sequences into a struct block editor
// and asserts the SOT invariant (tree ≡ node) after every step.
// Run with: go test ./editor/... -fuzz=FuzzStructInvariants
func FuzzStructInvariants(f *testing.F) {
	f.Add([]byte{2, 0, 2, 1, 3, 2})
	f.Add([]byte{0, 0, 0, 2, 3, 1, 1, 2})
	f.Add([]byte{2, 2, 2, 3, 3, 3, 0, 2})
	f.Fuzz(func(t *testing.T, actions []byte) {
		be := newBlockEdit(Config{NoDeleteConfirm: true}, blockSpec{
			key: "cfg", defs: cfgStructDefs(), kind: schema.KindObject, content: "cfg:\n",
		}, 120, 40)
		be = expandAll(be)
		for _, a := range actions {
			be = applyFuzzAction(be, a)
			assertTreeMatchesNode(t, be)
		}
	})
}

// FuzzCollectionInvariants feeds random action sequences into a sequence
// collection editor and asserts the collection SOT invariant after every step.
// Run with: go test ./editor/... -fuzz=FuzzCollectionInvariants
func FuzzCollectionInvariants(f *testing.F) {
	f.Add([]byte{2, 0, 2, 3, 0, 2})
	f.Add([]byte{3, 3, 2, 0, 2, 1, 2})
	f.Fuzz(func(t *testing.T, actions []byte) {
		be := newBlockEdit(Config{NoDeleteConfirm: true}, blockSpec{
			key:  "categories",
			defs: catDefs(),
			kind: schema.KindList,
			content: `categories:
  - name: a
  - name: b
`,
		}, 120, 40)
		be = expandAll(be)
		for _, a := range actions {
			be = applyFuzzAction(be, a)
			assertCollTreeMatchesNode(t, be)
		}
	})
}

// applyFuzzAction maps a byte to one of 5 safe editor actions.
func applyFuzzAction(be blockEditState, a byte) blockEditState {
	vis := be.tree.VisibleNodes()
	switch a % 5 {
	case 0:
		be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyDown})
	case 1:
		be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyUp})
	case 2:
		be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})
		be = expandAll(be)
	case 3:
		if be.tree.Cursor() >= 0 && be.tree.Cursor() < len(vis) {
			ni := vis[be.tree.Cursor()]
			if ni < be.tree.Len() {
				n := be.tree.Node(ni)
				switch {
				case n.Kind == fieldtree.KindField && n.Checked:
					be = be.dispatch(ToggleField{NodeIdx: ni, Checked: false})
					be = expandAll(be)
				case n.Kind == fieldtree.KindSeqItem:
					be = be.dispatch(DeleteEntry{SeqIdx: n.SeqIdx})
				}
			}
		}
	case 4:
		be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	return be
}

// Toggling a struct field on and then off returns the node to its baseline, so
// the editor must read clean again.
func TestComputedDirty_ToggleOnOffReadsClean(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	// commitShapeSpec has "host" present and "port" absent (unchecked).
	be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
	must.False(be.dirty, "freshly opened editor must be clean")

	idx := -1
	for i, n := range be.tree.Nodes() {
		if n.Kind == fieldtree.KindField && n.IsLeaf && !n.Checked {
			idx = i
			break
		}
	}
	must.GreaterOrEqual(idx, 0, "need an unchecked leaf field")
	label := be.tree.Node(idx).Label

	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: true})
	must.True(be.dirty, "adding a field must read dirty")

	// The tree was resectioned by the toggle; find the field again by label.
	idx = -1
	for i, n := range be.tree.Nodes() {
		if n.Kind == fieldtree.KindField && n.Label == label && n.Checked {
			idx = i
			break
		}
	}
	must.GreaterOrEqual(idx, 0, "toggled field not found after resync")
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
	is.False(be.dirty, "removing the just-added field must read clean again")
}

// Collections used to stay dirty forever once touched: an entry edited and then
// reverted must read clean.
func TestComputedDirty_CollectionRevertReadsClean(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, seqSpec("categories:\n  - name: alpha\n  - name: beta\n"), 100, 40)
	must.False(be.dirty, "freshly opened editor must be clean")
	original := be.yamlEditor.Value()

	be.active = blockEditPanelYAML
	be.yamlEditor = be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value(), Checkpoint: false})
	must.True(be.dirty, "edited entry must read dirty")

	be.yamlEditor = be.yamlEditor.SetValue(original)
	be = be.dispatch(SyncYAML{Content: original, Checkpoint: false})
	is.False(be.dirty, "reverting the entry to its original content must read clean again")
}

// The paste path's undo checkpoint must pair the pre-paste buffer with the
// pre-paste node; capturing the post-paste buffer left the pasted text behind.
func TestPasteUndoRestoresBufferAndNode(t *testing.T) {
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	be = be.focusPanel(toEditor) // focus the YAML panel (checkpoints, like a real Tab)
	prevBuf := be.yamlEditor.Value()
	prevNode := yamledit.NodeToContent(be.key, &be.node)

	be2, _ := be.updateEditing(tea.PasteMsg{Content: "  extra: value"})
	if be2.yamlEditor.Value() == prevBuf {
		t.Skip("paste message did not mutate the buffer")
	}

	be3 := be2.dispatch(Undo{})
	if be3.yamlEditor.Value() != prevBuf {
		t.Errorf("undo after paste left the buffer at %q, want pre-paste %q", be3.yamlEditor.Value(), prevBuf)
	}
	if got := yamledit.NodeToContent(be3.key, &be3.node); got != prevNode {
		t.Errorf("undo after paste left the node at %q, want pre-paste %q", got, prevNode)
	}
}

// The YAML editor paints the text as the preview does while it is focused.
func TestTheYAMLEditorIsHighlighted(t *testing.T) {
	ed, _ := blockEditState{}.newYAMLEditor("name: x\n").Focus()
	key := render.YAMLHighlight("name: x")[0][0]
	require.Equal(t, 0, key.Start)
	assert.Contains(t, ed.SetWidth(40).View(), key.Style.Render("name"))
}
