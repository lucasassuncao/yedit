package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
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
// empty - fixing the bug where openable fields always looked active.
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
	is.True(n.checked, "filled 'any' should be checked/active")

	empty := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
`), 100, 40)
	tm = empty.collectionDeriveTree()
	n, ok = findFieldNode(tm, "any")
	must.True(ok, "empty 'any' field not found in tree")
	is.False(n.checked, "empty 'any' should be unchecked/muted")
}

// TestCtrlDOnFilledOpenableAsksRemove verifies ctrl+d now acts on an openable
// field with content (opens the remove confirm), instead of being a no-op.
func TestCtrlDOnFilledOpenableAsksRemove(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
    any:
      - regex: "y"
`), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	is.Equal(modeConfirming, be.mode, "ctrl+d on a filled openable should open the remove confirm")
}

// TestCtrlDOnEmptyOpenableNoop verifies ctrl+d on an empty openable does nothing
// (there is no content to remove) and never opens a confirm.
func TestCtrlDOnEmptyOpenableNoop(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
`), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	is.Equal(modeEditing, be.mode, "ctrl+d on an empty openable should be a no-op")
}

// TestPresetBrowser_updateAndSelection exercises the preset-picker sub-model:
// construction, cursor navigation, focus toggling, and the apply/append/dismiss
// actions it reports back to the block editor.
func TestPresetBrowser_updateAndSelection(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	src := stubPresets{data: map[string]string{
		"workers/alpha": "workers:\n  - name: a\n",
		"workers/beta":  "workers:\n  - name: b\n",
	}}

	_, ok := newPresetBrowser(nil, "workers", "")
	is.False(ok, "nil source should not open a browser")
	_, ok = newPresetBrowser(src, "nothing", "")
	is.False(ok, "field without presets should not open a browser")

	pb, ok := newPresetBrowser(src, "workers", "beta")
	must.True(ok, "expected a browser for workers")
	is.Equal("beta", pb.names[pb.cursor], "cursor should pre-select the current preset")

	keyOf := func(s string) tea.KeyMsg {
		switch s {
		case "enter":
			return tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			return tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			return tea.KeyMsg{Type: tea.KeyTab}
		default:
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		}
	}

	action, _ := pb.Update(tea.KeyMsg{Type: tea.KeyUp}, false)
	is.Equal(presetNone, action, "up should not trigger action")
	is.Equal(0, pb.cursor, "up should move cursor to 0")

	action, name := pb.Update(keyOf("enter"), false)
	is.Equal(presetApplied, action, "enter should apply preset")
	is.Equal(pb.names[0], name, "enter should apply first preset name")

	action, name = pb.Update(keyOf("a"), true)
	is.Equal(presetAppended, action, "a with allowAppend should append")
	is.Equal(pb.names[0], name, "a should append first preset name")

	action, _ = pb.Update(keyOf("a"), false)
	is.Equal(presetNone, action, "a without allowAppend should be a no-op")

	// Tab moves focus to the preview; esc first returns focus, then dismisses.
	pb.Update(keyOf("tab"), false)
	must.True(pb.previewFocus, "tab should focus the preview")

	action, _ = pb.Update(keyOf("enter"), false)
	is.Equal(presetNone, action, "enter with preview focused should be a no-op")

	action, _ = pb.Update(keyOf("esc"), false)
	is.Equal(presetNone, action, "first esc should only return focus to the list")
	is.False(pb.previewFocus, "first esc should clear previewFocus")

	action, _ = pb.Update(keyOf("esc"), false)
	is.Equal(presetDismissed, action, "second esc should dismiss")
}

// TestAppendPreset_addsEntriesToExisting verifies that appendPreset appends
// all entries from the preset after the existing entries and positions the
// cursor on the last entry.
func TestAppendPreset_addsEntriesToExisting(t *testing.T) {
	is := assert.New(t)
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n",
	}}
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)

	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "extra")
	be = be.appendPreset("extra", y)

	base := nodeToContent("categories", &be.node)
	is.Contains(base, "name: existing", "entries missing original entry")
	is.Contains(base, "name: appended", "entries missing appended entry")

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	is.Equal(2, seqCount, "tree should have 2 seq items")
	is.Equal(1, be.tree.cursor, "cursor should be on last entry")
	is.Contains(be.yamlEditor.Value(), "appended", "yamlEditor not showing appended entry")
	is.True(be.dirty, "dirty should be true after appendPreset")
}

// TestAppendPreset_indentMismatch verifies that append works correctly when the
// existing seqBase and the preset YAML use different indentation levels.
func TestAppendPreset_indentMismatch(t *testing.T) {
	is := assert.New(t)
	// existing uses 4-space, preset uses 2-space
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n    enabled: null\n",
	}}
	spec := blockSpec{
		key:  "categories",
		kind: schema.KindList,
		defs: []schema.FieldDef{{YAMLName: "name", Kind: schema.KindPrimitive}},
		content: `categories:
    - name: existing
      enabled: true
`,
	}
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "extra")
	be = be.appendPreset("extra", y)

	base2 := nodeToContent("categories", &be.node)
	is.Contains(base2, "name: existing", "entries missing original entry")
	is.Contains(base2, "name: appended", "entries missing appended entry")
	is.Contains(be.yamlEditor.Value(), "appended", "yamlEditor not showing appended entry")
}

// TestAppendPreset_multiEntryPreset verifies that a preset with multiple
// entries adds all of them.
func TestAppendPreset_multiEntryPreset(t *testing.T) {
	is := assert.New(t)
	stub := stubPresets{data: map[string]string{
		"categories/multi": "categories:\n  - name: alpha\n  - name: beta\n",
	}}
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "multi")
	be = be.appendPreset("multi", y)

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	is.Equal(3, seqCount, "tree should have 3 seq items (1 existing + 2 from preset)")
	is.Equal(2, be.tree.cursor, "cursor should be on last entry")
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

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after field remove")
	is.NotContains(be.yamlEditor.Value(), "output:", "output still present after remove")

	be = be.restoreUndo()
	is.Equal(want, be.yamlEditor.Value(), "after undo YAML")
}

// TestUndo_structFieldRemoveWithContent verifies that removing a field that has
// content (triggers confirm dialog) still saves an undo snap, and that
// restoreUndo brings the field back with its original value.
func TestUndo_structFieldRemoveWithContent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	want := be.yamlEditor.Value()

	// ctrl+d on output (value "both") → shows confirm dialog.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	must.Equal(modeConfirming, be.mode, "expected modeConfirming after ctrl+d on field with content")
	// YAML unchanged until the user confirms.
	is.Contains(be.yamlEditor.Value(), "output:", "YAML must not change before confirmation")

	// Locate the output node and simulate the user confirming via dispatch.
	outputIdx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "output" {
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

// TestUndo_structFieldAdd verifies that adding an unchecked field saves an undo
// snap and that restoreUndo removes the field again.
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
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after field add")
	is.Contains(be.yamlEditor.Value(), "output:", "output missing after add")

	be = be.restoreUndo()
	is.Equal(want, be.yamlEditor.Value(), "after undo YAML")
}

// TestUndo_seqItemDelete verifies that deleting a seq item saves an undo snap
// and that restoreUndo restores the original entries.
func TestUndo_seqItemDelete(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := nodeToContent("categories", &be.node)

	// ctrl+d on the first item (alpha) now confirms before deleting.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	must.Equal(modeConfirming, be.mode, "ctrl+d on a seq item should confirm first")
	be = be.dispatch(DeleteEntry{SeqIdx: 0})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after seq item delete")

	be = be.restoreUndo()
	gotBase := nodeToContent("categories", &be.node)
	is.Equal(wantBase, gotBase, "after undo entries")
	is.Equal(2, seqItemCount(be), "after undo seq item count")
}

// TestUndo_seqItemAdd verifies that adding a seq item saves an undo snap and
// that restoreUndo removes the added item.
func TestUndo_seqItemAdd(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := nodeToContent("categories", &be.node)

	// Navigate to [+ add new] and press Enter.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})

	must.NotEmpty(be.undoStack, "undoStack must be non-empty after seq item add")
	is.Equal(2, seqItemCount(be), "after add seq item count")

	be = be.restoreUndo()
	gotBase := nodeToContent("categories", &be.node)
	is.Equal(wantBase, gotBase, "after undo entries")
	is.Equal(1, seqItemCount(be), "after undo seq item count")
}

// TestCollectionNav_RawSetValueDoesNotMutateNode verifies that replacing the
// editor text out-of-band (without a keystroke through updateKey, which is what
// parse-gates edits into the node) leaves the canonical node untouched until a
// real flush. Only navigation and commit reconcile the buffer into the node.
func TestCollectionNav_RawSetValueDoesNotMutateNode(t *testing.T) {
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	original := be.entryYAML(0)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	must.Equal(original, be.entryYAML(0), "canonical node was mutated by a raw SetValue")
}

// TestCollectionNav_CommitFlushesAndSerializesAll verifies that commit() flushes
// the current buffer into entries and includes all entries in the snippet.
func TestCollectionNav_CommitFlushesAndSerializesAll(t *testing.T) {
	must := require.New(t)
	spec := seqSpec("categories:\n  - name: alpha\n  - name: beta\n")
	be := newBlockEdit(Config{}, spec, 100, 40)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	_, cmd := be.commit()
	msg := cmd().(blockEditCommittedMsg)

	must.Contains(msg.Snippet, "name: alpha_edited", "snippet missing edited entry")
	must.Contains(msg.Snippet, "name: beta", "snippet missing second entry")
}

// TestCollectionNav_DoubleCommitIdempotent is a regression test for the
// duplication bug: committing twice must produce identical snippets.
func TestCollectionNav_DoubleCommitIdempotent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
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

	must.Equal(snippet1, snippet2, "double commit diverged")
	is.Equal(1, strings.Count(snippet2, "name: beta"), "duplication detected")
}

// TestPrimitiveBlock_showsFieldItemAndHint verifies that a tree-less block shows
// the field itself in the left panel and its metadata in the hint panel, instead
// of the "(no fields)" / "select a field" placeholders.
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
		Metadata: MetadataFunc(func(block, fieldPath string) FieldMeta {
			if block == "debug" {
				return FieldMeta{Type: "bool", Default: "false"}
			}
			return FieldMeta{}
		}),
	}
	be := newBlockEdit(cfg, spec, 100, 40)

	must.True(be.tree.isEmpty(), "expected an empty tree for a primitive block")

	left := be.fieldItemView()
	is.Contains(left, "debug", "left panel should name the field")

	hint := be.hintContent()
	for _, want := range []string{"Type:", "bool", "Default:", "false"} {
		is.Contains(hint, want, "hint panel missing %q", want)
	}

	is.NotContains(be.View(nil), "(no fields)", "full view should no longer show the (no fields) placeholder")
}

// TestRenderFieldHint_typeAndRequiredBehavior verifies that Type is shown only
// when set and Required is shown only when true.
func TestRenderFieldHint_typeAndRequiredBehavior(t *testing.T) {
	th := resolveTheme(theme.Theme{})

	t.Run("type shown when set", func(t *testing.T) {
		is := assert.New(t)
		out := renderFieldHint(th, FieldMeta{Type: "string"}, "")
		is.Contains(out, "Type:")
		is.Contains(out, "string")
	})

	t.Run("type omitted when empty", func(t *testing.T) {
		is := assert.New(t)
		out := renderFieldHint(th, FieldMeta{Description: "desc"}, "")
		is.NotContains(out, "Type:", "expected no Type line when Type is empty")
	})

	t.Run("required shown only when true", func(t *testing.T) {
		is := assert.New(t)
		out := renderFieldHint(th, FieldMeta{Required: true}, "")
		is.Contains(out, "Required:")
		is.Contains(out, "yes")
	})

	t.Run("required omitted when false", func(t *testing.T) {
		is := assert.New(t)
		out := renderFieldHint(th, FieldMeta{Description: "desc"}, "")
		is.NotContains(out, "Required:", "expected no Required line when false")
	})
}

// TestRenderFieldHint_constraints verifies that the constraint fields render
// when set and stay absent on a zero FieldMeta.
func TestRenderFieldHint_constraints(t *testing.T) {
	is := assert.New(t)
	th := resolveTheme(theme.Theme{})
	out := renderFieldHint(th, FieldMeta{
		Min: "1s", Max: "168h",
		Pattern:    `^\d+$`,
		MinCount:   1,
		Unique:     true,
		Deprecated: "use limits instead",
	}, "")
	for _, want := range []string{
		"Range:", "1s – 168h",
		"Pattern:", `^\d+$`,
		"Entries:", "1 – ∞",
		"Unique:", "yes",
		"Deprecated:", "use limits instead",
	} {
		is.Contains(out, want, "rendered hint should contain %q", want)
	}
	empty := renderFieldHint(th, FieldMeta{}, "")
	is.NotContains(empty, "Range:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Pattern:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Entries:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Unique:", "zero FieldMeta must render no constraint lines")
	is.NotContains(empty, "Deprecated:", "zero FieldMeta must render no constraint lines")
}

// TestRestoreUndo_emptyStackIsNoOp guards restoreUndo against an empty stack:
// the function must not panic and must return the state unchanged.
func TestRestoreUndo_emptyStackIsNoOp(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, blockSpec{key: "debug", kind: schema.KindPrimitive}, 100, 40)
	must.Empty(be.undoStack, "a freshly opened editor must have an empty undo stack")
	got := be.restoreUndo() // must not panic with an empty stack
	is.Empty(got.undoStack, "restoreUndo on an empty stack should leave it empty")
}

// ---------------------------------------------------------------------------
// resyncTreeFromYAML - tolerant, non-authoritative visual projection
// ---------------------------------------------------------------------------

// TestResyncToleratesInvalidYAML_struct verifies that a transiently unparseable
// buffer (mid-typing) neither panics nor wipes the tree's checked state: the
// per-keystroke resync leaves the last good visual state in place.
func TestResyncToleratesInvalidYAML_struct(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)

	before := map[string]bool{}
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeField {
			before[n.label] = n.checked
		}
	}

	// Unterminated flow sequence - definitely invalid YAML.
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("configuration:\n  output: [unterminated\n")

	tm := be.resyncTreeFromYAML() // must not panic

	after := map[string]bool{}
	for _, n := range tm.nodes {
		if n.kind == treeNodeField {
			after[n.label] = n.checked
		}
	}
	must.Len(after, len(before), "tree fields changed on invalid YAML")
	for k, v := range before {
		is.Equal(v, after[k], "checked state for %q changed on invalid YAML (state should be preserved)", k)
	}
}

// TestResyncToleratesInvalidYAML_collection verifies the same tolerance for a
// collection navigator: an unparseable current entry preserves the entry's label
// and never mutates the canonical entries slice.
func TestResyncToleratesInvalidYAML_collection(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, seqSpec("categories:\n  - name: alpha\n"), 100, 40)
	nodeBefore := nodeToContent(be.key, &be.node)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: [unterminated\n")

	tm := be.resyncTreeFromYAML() // must not panic

	// The canonical node must be untouched: the tree is derived from it, not from
	// the (now invalid) buffer.
	must.Equal(nodeBefore, nodeToContent(be.key, &be.node), "resync mutated canonical node")

	// The existing item label must survive an unparseable buffer.
	foundAlpha := false
	for _, n := range tm.nodes {
		if n.kind == treeNodeSeqItem && n.label == "alpha" {
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

// TestCtrlDRemovesNestedParentBlock reproduces the movelooper bug: ctrl+d on a
// nested struct parent (hooks.before) carries no checkbox of its own, so the old
// handleRemove returned treeNoAction and nothing happened. ctrl+d must now offer
// removal and delete the whole subtree, leaving sibling blocks (after) intact.
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
	for i := range be.tree.nodes {
		be.tree.nodes[i].expanded = true
	}
	be = cursorToLabel(be, "before")

	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyCtrlD})
	must.Equal(modeConfirming, be.mode, "ctrl+d on nested parent did not offer removal")

	// Locate the captured "before" node index and confirm the removal.
	beforeIdx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "before" {
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

// TestRedo_structFieldRemove verifies the undo→redo round-trip: removing a
// field, undoing, then redoing lands back on the post-remove state, and the
// redo itself can be undone again.
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

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
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

// TestRedo_clearedByNewMutation verifies that a new mutation after an undo
// discards the redo stack - the editor forks away from the undone state.
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

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	be = be.restoreUndo()
	must.NotEmpty(be.redoStack, "redoStack must be non-empty after undo")

	// New mutation (remove again) must clear the redo stack.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	is.Empty(be.redoStack, "redoStack must be cleared by a new mutation")
}

// TestRestoreRedo_emptyStackIsNoOp guards restoreRedo against an empty stack.
func TestRestoreRedo_emptyStackIsNoOp(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	got := be.restoreRedo() // must not panic with an empty stack
	is.Empty(got.redoStack, "restoreRedo on an empty stack should leave it empty")
}
