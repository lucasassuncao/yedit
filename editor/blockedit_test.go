package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/atotto/clipboard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/bubbles/textarea"
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

	var (
		action presetAction
		name   string
	)
	pb, action, _ = pb.Update(tea.KeyMsg{Type: tea.KeyUp}, false)
	is.Equal(presetNone, action, "up should not trigger action")
	is.Equal(0, pb.cursor, "up should move cursor to 0")

	pb, action, name = pb.Update(keyOf("enter"), false)
	is.Equal(presetApplied, action, "enter should apply preset")
	is.Equal(pb.names[0], name, "enter should apply first preset name")

	pb, action, name = pb.Update(keyOf("a"), true)
	is.Equal(presetAppended, action, "a with allowAppend should append")
	is.Equal(pb.names[0], name, "a should append first preset name")

	pb, action, _ = pb.Update(keyOf("a"), false)
	is.Equal(presetNone, action, "a without allowAppend should be a no-op")

	// Tab moves focus to the preview; esc first returns focus, then dismisses.
	pb, _, _ = pb.Update(keyOf("tab"), false)
	must.True(pb.previewFocus, "tab should focus the preview")

	pb, action, _ = pb.Update(keyOf("enter"), false)
	is.Equal(presetNone, action, "enter with preview focused should be a no-op")

	pb, action, _ = pb.Update(keyOf("esc"), false)
	is.Equal(presetNone, action, "first esc should only return focus to the list")
	is.False(pb.previewFocus, "first esc should clear previewFocus")

	pb, action, _ = pb.Update(keyOf("esc"), false)
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
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)

	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "extra")
	be = be.dispatch(AppendPreset{Name: "extra", Content: y})

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
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "extra")
	be = be.dispatch(AppendPreset{Name: "extra", Content: y})

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
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "multi")
	be = be.dispatch(AppendPreset{Name: "multi", Content: y})

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	is.Equal(3, seqCount, "tree should have 3 seq items (1 existing + 2 from preset)")
	is.Equal(2, be.tree.cursor, "cursor should be on last entry")
}

// TestAppendPreset_duplicateMapKeyRejected verifies that appending a preset
// whose entry key already exists in a map-nav collection is rejected instead
// of splicing a duplicate mapping key into the node.
func TestAppendPreset_duplicateMapKeyRejected(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	stub := stubPresets{data: map[string]string{
		"moves/dup": "moves:\n  alpha:\n    path: /from-preset\n",
	}}
	spec := blockSpec{
		key:  "moves",
		kind: schema.KindDictionary,
		defs: []schema.FieldDef{
			{YAMLName: "path", Kind: schema.KindPrimitive},
		},
		content: "moves:\n  alpha:\n    path: /original\n",
	}
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)
	must.True(be.isMapNav(), "spec must build a map-nav block")

	y, _ := stub.PresetYAML("moves", "dup")
	be = be.dispatch(AppendPreset{Name: "dup", Content: y})

	is.Equal(errPreset, be.editorErr.kind, "colliding append must set a preset error")
	count := 0
	for i := 0; i+1 < len(be.node.Content); i += 2 {
		if be.node.Content[i].Value == "alpha" {
			count++
		}
	}
	is.Equal(1, count, "'alpha' must remain a single entry")
	base := nodeToContent("moves", &be.node)
	is.Contains(base, "path: /original", "original entry value must be preserved")
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

	_, val, ok := be.commit()
	must.True(ok, "commit failed")

	snippet := nodeToContent(be.key, val)
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
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	be, val1, ok := be.commit()
	must.True(ok, "first commit failed")
	snippet1 := nodeToContent(be.key, val1)

	// Re-sync after commit (mirrors the live flush path).
	be = be.resyncAfterCommit(snippet1)

	_, val2, ok := be.commit()
	must.True(ok, "second commit failed")
	snippet2 := nodeToContent(be.key, val2)

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

// TestUndo_treelessRetypeAfterUndo verifies that in a tree-less (YAML-only)
// block a keystroke after an undo re-checkpoints the restored state, so a
// second ctrl+u can undo the new edit instead of reporting "Nothing to undo."
// It also verifies the new edit invalidates the pending redo entry.
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
		be, _ = be.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	typeRune('x')
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	must.Equal(baseline, be.yamlEditor.Value(), "first undo must restore the opening content")

	typeRune('y')
	is.Empty(be.redoStack, "a new edit after undo must discard the redo entry")

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	is.Equal("Undone.", be.statusMsg, "second undo must work after retyping")
	is.Equal(baseline, be.yamlEditor.Value(), "second undo must restore the pre-retype content")
}

// TestRestoreRedo_emptyStackIsNoOp guards restoreRedo against an empty stack.
func TestRestoreRedo_emptyStackIsNoOp(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	got := be.restoreRedo() // must not panic with an empty stack
	is.Empty(got.redoStack, "restoreRedo on an empty stack should leave it empty")
}

// TestInnerH_AdjustsForLegendLines verifies that innerH() shrinks by one for
// each extra legend line beyond the first.
func TestInnerH_AdjustsForLegendLines(t *testing.T) {
	base := blockEditState{width: 80, height: 30}

	base.legendLines = 1
	h1 := base.innerH()

	base.legendLines = 2
	h2 := base.innerH()
	if h2 != h1-1 {
		t.Errorf("legendLines=2 want innerH=%d, got %d", h1-1, h2)
	}

	base.legendLines = 3
	h3 := base.innerH()
	if h3 != h1-2 {
		t.Errorf("legendLines=3 want innerH=%d, got %d", h1-2, h3)
	}
}

// TestInnerH_MinimumOne ensures innerH never returns less than 1.
func TestInnerH_MinimumOne(t *testing.T) {
	be := blockEditState{width: 80, height: 4, legendLines: 10}
	if h := be.innerH(); h < 1 {
		t.Errorf("innerH must be at least 1, got %d", h)
	}
}

// TestSaveUndoDeduplicatesSpeculativeCheckpoints guards that repeated Tab
// switches into the YAML panel (speculative checkpoints with no edit between
// them) push a single snapshot instead of piling up identical ones.
func TestSaveUndoDeduplicatesSpeculativeCheckpoints(t *testing.T) {
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)

	for range 3 {
		be = be.switchPanel() // tree → yaml: checkpoint
		be = be.switchPanel() // yaml → tree: no checkpoint
	}

	must.Len(be.undoStack, 1, "identical speculative checkpoints must be deduplicated")
}

// TestUndoAfterSpeculativeCheckpointRestoresInOnePress guards the restore-side
// dedupe: after a real change followed by a Tab checkpoint (equal to the live
// state), a single undo must land on the pre-change state instead of first
// "restoring" the identical checkpoint.
func TestUndoAfterSpeculativeCheckpointRestoresInOnePress(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	want := be.yamlEditor.Value()

	outputIdx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "output" {
			outputIdx = i
			break
		}
	}
	must.GreaterOrEqual(outputIdx, 0, "output node not found")
	be = be.dispatch(ToggleField{NodeIdx: outputIdx, Checked: false})
	must.NotContains(be.yamlEditor.Value(), "output:", "toggle should remove the field")

	be = be.switchPanel() // speculative checkpoint at the post-toggle state

	be = be.restoreUndo()
	is.Equal(want, be.yamlEditor.Value(), "one undo must restore the pre-toggle state")
	is.Equal("Undone.", be.statusMsg)
}

// TestRestoreUndoWithOnlyNoopSnapshotsReportsNothing guards the honest status:
// when the stack holds only snapshots identical to the live state, undo drops
// them and reports "Nothing to undo." instead of a phantom "Undone.".
func TestRestoreUndoWithOnlyNoopSnapshotsReportsNothing(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)

	be = be.switchPanel() // checkpoint identical to the live state
	must.Len(be.undoStack, 1)

	be = be.restoreUndo()
	is.Equal("Nothing to undo.", be.statusMsg)
	is.Empty(be.undoStack, "no-op snapshots must be dropped")
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

// TestCommit_RejectsExtraTopLevelKeys guards against silent data loss: a
// second top-level key typed into the editor used to be dropped without
// warning (only the first key's value was committed). It must instead fail
// the commit with an explicit error naming the stray key.
func TestCommit_RejectsExtraTopLevelKeys(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("server:\n  host: localhost\nlogging:\n  level: debug\n")

	committed, val, ok := be.commit()
	must.False(ok, "commit must reject a buffer with two top-level keys")
	is.Nil(val)
	is.Equal(errCommit, committed.editorErr.kind)
	is.Contains(committed.editorErr.message, "logging", "the error must name the stray key")
}

// TestCommit_RejectsRenamedTopLevelKey guards the other silent-loss shape: a
// renamed block header used to be ignored (content written back under the
// original key). It must fail the commit with an explicit error.
func TestCommit_RejectsRenamedTopLevelKey(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("cluster:\n  host: localhost\n")

	committed, val, ok := be.commit()
	must.False(ok, "commit must reject a renamed top-level key")
	is.Nil(val)
	is.Equal(errCommit, committed.editorErr.kind)
	is.Contains(committed.editorErr.message, `"server"`, "the error must name the expected key")
}

// TestCommit_RejectsMissingHeader covers a buffer whose block header was
// deleted or replaced by a bare scalar: the old path surfaced a misleading
// "internal error"; it must now explain the missing header.
func TestCommit_RejectsMissingHeader(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	for _, buffer := range []string{"just a scalar", ""} {
		be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
		be.active = blockEditPanelYAML
		be.yamlEditor.SetValue(buffer)

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
	be.yamlEditor.SetValue("server:\n  host: example.org\n  port: 8080\n")

	committed, val, ok := be.commit()
	must.True(ok, "commit failed; editorErr=%v", committed.editorErr)
	must.NotNil(val)
	is.Equal("server:\n  host: example.org\n  port: 8080\n", nodeToContent("server", val))
}

// TestTreelessBlock_UndoRestoresInitialContent guards the undo baseline for
// blocks that open with the YAML panel focused (primitives, free-form
// collections): typing must be reversible back to the content the editor
// opened with, even though no Tab checkpoint ever fired.
func TestTreelessBlock_UndoRestoresInitialContent(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, blockSpec{key: "debug", kind: schema.KindPrimitive, content: "debug: false\n"}, 100, 40)
	must.Equal(blockEditPanelYAML, be.active, "tree-less block must focus the YAML panel")
	must.Empty(be.undoStack, "a freshly opened editor must have an empty undo stack")
	original := be.yamlEditor.Value()

	be, _ = be.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	must.NotEqual(original, be.yamlEditor.Value(), "typing must change the buffer")

	be = be.restoreUndo()
	is.Equal(original, be.yamlEditor.Value(), "ctrl+u must restore the content the editor opened with")
	is.Equal("Undone.", be.statusMsg)
}

// TestHintScroll_ReachesEndOfLongContent guards the hint panel's scroll bound:
// it must be derived from the content height, so the last panel-full of a
// long hint is reachable (the old bound was the panel height itself).
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

	down := tea.KeyMsg{Type: tea.KeyDown}
	for range lines * 2 {
		be, _ = be.handleHintKey(down)
	}
	is.Equal(wantMax, be.hintScroll, "scroll must reach the last panel-full of the hint content")
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
	vis := be.tree.visibleNodes()
	switch a % 5 {
	case 0:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyDown})
	case 1:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyUp})
	case 2:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
		be = expandAll(be)
	case 3:
		if be.tree.cursor >= 0 && be.tree.cursor < len(vis) {
			ni := vis[be.tree.cursor]
			if ni < len(be.tree.nodes) {
				n := be.tree.nodes[ni]
				switch {
				case n.kind == treeNodeField && n.checked:
					be = be.dispatch(ToggleField{NodeIdx: ni, Checked: false})
					be = expandAll(be)
				case n.kind == treeNodeSeqItem:
					be = be.dispatch(DeleteEntry{SeqIdx: n.seqIdx})
				}
			}
		}
	case 4:
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyRight})
	}
	return be
}

// TestComputedDirty_ToggleOnOffReadsClean guards the derived dirty flag for
// struct blocks: toggling a field on and then off returns the node to its
// baseline, so the editor must read clean again.
func TestComputedDirty_ToggleOnOffReadsClean(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	// commitShapeSpec has "host" present and "port" absent (unchecked).
	be := newBlockEdit(Config{}, commitShapeSpec(), 100, 40)
	must.False(be.dirty, "freshly opened editor must be clean")

	idx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.isLeaf && !n.checked {
			idx = i
			break
		}
	}
	must.GreaterOrEqual(idx, 0, "need an unchecked leaf field")
	label := be.tree.nodes[idx].label

	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: true})
	must.True(be.dirty, "adding a field must read dirty")

	// The tree was resectioned by the toggle; find the field again by label.
	idx = -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == label && n.checked {
			idx = i
			break
		}
	}
	must.GreaterOrEqual(idx, 0, "toggled field not found after resync")
	be = be.dispatch(ToggleField{NodeIdx: idx, Checked: false})
	is.False(be.dirty, "removing the just-added field must read clean again")
}

// TestComputedDirty_CollectionRevertReadsClean guards the derived dirty flag
// for collections, which previously stayed dirty forever once touched: an
// entry edited and then reverted to its original content must read clean.
func TestComputedDirty_CollectionRevertReadsClean(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	be := newBlockEdit(Config{}, seqSpec("categories:\n  - name: alpha\n  - name: beta\n"), 100, 40)
	must.False(be.dirty, "freshly opened editor must be clean")
	original := be.yamlEditor.Value()

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value(), Checkpoint: false})
	must.True(be.dirty, "edited entry must read dirty")

	be.yamlEditor.SetValue(original)
	be = be.dispatch(SyncYAML{Content: original, Checkpoint: false})
	is.False(be.dirty, "reverting the entry to its original content must read clean again")
}

// TestPasteUndoRestoresBufferAndNode guards the non-key (paste) buffer path:
// the undo checkpoint must pair the pre-paste buffer with the pre-paste node.
// Before the fix it captured the post-paste buffer, so ctrl+u restored the old
// node but left the pasted text in the editor.
func TestPasteUndoRestoresBufferAndNode(t *testing.T) {
	if err := clipboard.WriteAll("  extra: value\n"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	be = be.switchPanel() // focus the YAML panel (checkpoints, like a real Tab)
	prevBuf := be.yamlEditor.Value()
	prevNode := nodeToContent(be.key, &be.node)

	be2, _ := be.updateEditing(textarea.Paste())
	if be2.yamlEditor.Value() == prevBuf {
		t.Skip("paste message did not mutate the buffer")
	}

	be3 := be2.dispatch(Undo{})
	if be3.yamlEditor.Value() != prevBuf {
		t.Errorf("undo after paste left the buffer at %q, want pre-paste %q", be3.yamlEditor.Value(), prevBuf)
	}
	if got := nodeToContent(be3.key, &be3.node); got != prevNode {
		t.Errorf("undo after paste left the node at %q, want pre-paste %q", got, prevNode)
	}
}
