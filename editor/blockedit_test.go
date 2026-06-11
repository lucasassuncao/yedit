package editor

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

// stubPresets implements PresetSource for tests.
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
	filled := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
    any:
      - regex: "y"
`), 100, 40)
	tm := filled.collectionDeriveTree()
	if n, ok := findFieldNode(tm, "any"); !ok || !n.checked {
		t.Errorf("filled 'any' should be checked/active; got ok=%v checked=%v", ok, n.checked)
	}

	empty := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
`), 100, 40)
	tm = empty.collectionDeriveTree()
	if n, ok := findFieldNode(tm, "any"); !ok || n.checked {
		t.Errorf("empty 'any' should be unchecked/muted; got ok=%v checked=%v", ok, n.checked)
	}
}

// TestCtrlDOnFilledOpenableAsksRemove verifies ctrl+d now acts on an openable
// field with content (opens the remove confirm), instead of being a no-op.
func TestCtrlDOnFilledOpenableAsksRemove(t *testing.T) {
	be := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
    any:
      - regex: "y"
`), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if be.mode != modeConfirming {
		t.Errorf("ctrl+d on a filled openable should open the remove confirm; mode=%d", be.mode)
	}
}

// TestCtrlDOnEmptyOpenableNoop verifies ctrl+d on an empty openable does nothing
// (there is no content to remove) and never opens a confirm.
func TestCtrlDOnEmptyOpenableNoop(t *testing.T) {
	be := newBlockEdit(Config{}, filterSpec(`filters:
  - regex: "x"
`), 100, 40)
	be.tree = be.collectionDeriveTree()
	be = cursorToFieldExpanded(be, "any")

	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if be.mode != modeEditing {
		t.Errorf("ctrl+d on an empty openable should be a no-op; mode=%d", be.mode)
	}
}

// TestPresetBrowser_updateAndSelection exercises the preset-picker sub-model:
// construction, cursor navigation, focus toggling, and the apply/append/dismiss
// actions it reports back to the block editor.
func TestPresetBrowser_updateAndSelection(t *testing.T) {
	src := stubPresets{data: map[string]string{
		"workers/alpha": "workers:\n  - name: a\n",
		"workers/beta":  "workers:\n  - name: b\n",
	}}

	if pb := newPresetBrowser(nil, "workers", ""); pb != nil {
		t.Error("nil source should not open a browser")
	}
	if pb := newPresetBrowser(src, "nothing", ""); pb != nil {
		t.Error("field without presets should not open a browser")
	}

	pb := newPresetBrowser(src, "workers", "beta")
	if pb == nil {
		t.Fatal("expected a browser for workers")
	}
	if pb.names[pb.cursor] != "beta" {
		t.Errorf("cursor should pre-select the current preset, got %q", pb.names[pb.cursor])
	}

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

	if action, _ := pb.Update(keyOf("k"), false); action != presetNone || pb.cursor != 0 {
		t.Errorf("up should move cursor to 0, got action=%v cursor=%d", action, pb.cursor)
	}
	if action, name := pb.Update(keyOf("enter"), false); action != presetApplied || name != pb.names[0] {
		t.Errorf("enter should apply %q, got action=%v name=%q", pb.names[0], action, name)
	}
	if action, name := pb.Update(keyOf("a"), true); action != presetAppended || name != pb.names[0] {
		t.Errorf("a with allowAppend should append, got action=%v name=%q", action, name)
	}
	if action, _ := pb.Update(keyOf("a"), false); action != presetNone {
		t.Errorf("a without allowAppend should be a no-op, got %v", action)
	}

	// Tab moves focus to the preview; esc first returns focus, then dismisses.
	pb.Update(keyOf("tab"), false)
	if !pb.previewFocus {
		t.Fatal("tab should focus the preview")
	}
	if action, _ := pb.Update(keyOf("enter"), false); action != presetNone {
		t.Errorf("enter with preview focused should be a no-op, got %v", action)
	}
	if action, _ := pb.Update(keyOf("esc"), false); action != presetNone || pb.previewFocus {
		t.Error("first esc should only return focus to the list")
	}
	if action, _ := pb.Update(keyOf("esc"), false); action != presetDismissed {
		t.Errorf("second esc should dismiss, got %v", action)
	}
}

// TestAppendPreset_addsEntriesToExisting verifies that appendPreset appends
// all entries from the preset after the existing entries and positions the
// cursor on the last entry.
func TestAppendPreset_addsEntriesToExisting(t *testing.T) {
	stub := stubPresets{data: map[string]string{
		"categories/extra": `categories:
  - name: appended
`,
	}}
	spec := seqSpec(`categories:
  - name: existing
`)
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)

	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "extra")
	be = be.appendPreset("extra", y)

	base := nodeToContent("categories", be.node)
	if !strings.Contains(base, "name: existing") {
		t.Errorf("entries missing original entry:\n%s", base)
	}
	if !strings.Contains(base, "name: appended") {
		t.Errorf("entries missing appended entry:\n%s", base)
	}

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	if seqCount != 2 {
		t.Errorf("tree has %d seq items, want 2", seqCount)
	}

	if be.tree.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (last entry)", be.tree.cursor)
	}

	if !strings.Contains(be.yamlEditor.Value(), "appended") {
		t.Errorf("yamlEditor not showing appended entry:\n%s", be.yamlEditor.Value())
	}

	if !be.dirty {
		t.Error("dirty should be true after appendPreset")
	}
}

// TestAppendPreset_indentMismatch verifies that append works correctly when the
// existing seqBase and the preset YAML use different indentation levels.
func TestAppendPreset_indentMismatch(t *testing.T) {
	// existing uses 4-space, preset uses 2-space
	stub := stubPresets{data: map[string]string{
		"categories/extra": `categories:
  - name: appended
    enabled: null
`,
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

	base2 := nodeToContent("categories", be.node)
	if !strings.Contains(base2, "name: existing") {
		t.Errorf("entries missing original entry:\n%s", base2)
	}
	if !strings.Contains(base2, "name: appended") {
		t.Errorf("entries missing appended entry:\n%s", base2)
	}
	if !strings.Contains(be.yamlEditor.Value(), "appended") {
		t.Errorf("yamlEditor not showing appended entry:\n%s", be.yamlEditor.Value())
	}
}

// TestAppendPreset_multiEntryPreset verifies that a preset with multiple
// entries adds all of them.
func TestAppendPreset_multiEntryPreset(t *testing.T) {
	stub := stubPresets{data: map[string]string{
		"categories/multi": `categories:
  - name: alpha
  - name: beta
`,
	}}
	spec := seqSpec(`categories:
  - name: existing
`)
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
	if seqCount != 3 {
		t.Errorf("tree has %d seq items, want 3 (1 existing + 2 from preset)", seqCount)
	}
	if be.tree.cursor != 2 {
		t.Errorf("cursor = %d, want 2", be.tree.cursor)
	}
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

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after field remove")
	}
	if strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Errorf("output still present after remove: %q", be.yamlEditor.Value())
	}

	be = be.restoreUndo()
	if be.yamlEditor.Value() != want {
		t.Errorf("after undo YAML = %q, want %q", be.yamlEditor.Value(), want)
	}
}

// TestUndo_structFieldRemoveWithContent verifies that removing a field that has
// content (triggers confirm dialog) still saves an undo snap, and that
// restoreUndo brings the field back with its original value.
func TestUndo_structFieldRemoveWithContent(t *testing.T) {
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	want := be.yamlEditor.Value()

	// ctrl+d on output (value "both") → shows confirm dialog.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if be.mode != modeConfirming {
		t.Fatal("expected modeConfirming after ctrl+d on field with content")
	}
	// YAML unchanged until the user confirms.
	if !strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Error("YAML must not change before confirmation")
	}

	// Locate the output node and simulate the user confirming via dispatch.
	// (saveUndo happens inside dispatch after confirmation, not before the dialog.)
	outputIdx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "output" {
			outputIdx = i
			break
		}
	}
	if outputIdx < 0 {
		t.Fatal("output node not found")
	}
	be = be.dispatch(ToggleField{NodeIdx: outputIdx, Checked: false})

	if strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Errorf("output still present after confirmed remove: %q", be.yamlEditor.Value())
	}
	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after confirmed remove")
	}

	be = be.restoreUndo()
	if be.yamlEditor.Value() != want {
		t.Errorf("after undo YAML = %q, want %q", be.yamlEditor.Value(), want)
	}
}

// TestUndo_structFieldAdd verifies that adding an unchecked field saves an undo
// snap and that restoreUndo removes the field again.
func TestUndo_structFieldAdd(t *testing.T) {
	spec := blockSpec{
		key:  "configuration",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "output", Kind: schema.KindPrimitive},
			{YAMLName: "log-level", Kind: schema.KindPrimitive},
		},
		content: `configuration:
  log-level: info
`,
	}
	be := newBlockEdit(Config{}, spec, 100, 40)
	want := be.yamlEditor.Value()

	// Navigate to output (AVAILABLE section) and add it with Enter.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after field add")
	}
	if !strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Errorf("output missing after add: %q", be.yamlEditor.Value())
	}

	be = be.restoreUndo()
	if be.yamlEditor.Value() != want {
		t.Errorf("after undo YAML = %q, want %q", be.yamlEditor.Value(), want)
	}
}

// TestUndo_seqItemDelete verifies that deleting a seq item saves an undo snap
// and that restoreUndo restores the original entries.
func TestUndo_seqItemDelete(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := nodeToContent("categories", be.node)

	// ctrl+d on the first item (alpha) now confirms before deleting.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if be.mode != modeConfirming {
		t.Fatalf("ctrl+d on a seq item should confirm first; mode=%d", be.mode)
	}
	be = be.dispatch(DeleteEntry{SeqIdx: 0})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after seq item delete")
	}

	be = be.restoreUndo()
	gotBase := nodeToContent("categories", be.node)
	if gotBase != wantBase {
		t.Errorf("after undo entries = %q, want %q", gotBase, wantBase)
	}
	if got := seqItemCount(be); got != 2 {
		t.Errorf("after undo got %d seq items, want 2", got)
	}
}

// TestUndo_seqItemAdd verifies that adding a seq item saves an undo snap and
// that restoreUndo removes the added item.
func TestUndo_seqItemAdd(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
`)
	be := newBlockEdit(Config{}, spec, 100, 40)
	wantBase := nodeToContent("categories", be.node)

	// Navigate to [+ add new] and press Enter.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyDown})
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(be.undoStack) == 0 {
		t.Fatal("undoStack must be non-empty after seq item add")
	}
	if got := seqItemCount(be); got != 2 {
		t.Errorf("after add got %d seq items, want 2", got)
	}

	be = be.restoreUndo()
	gotBase := nodeToContent("categories", be.node)
	if gotBase != wantBase {
		t.Errorf("after undo entries = %q, want %q", gotBase, wantBase)
	}
	if got := seqItemCount(be); got != 1 {
		t.Errorf("after undo got %d seq items, want 1", got)
	}
}

// TestCollectionNav_RawSetValueDoesNotMutateNode verifies that replacing the
// editor text out-of-band (without a keystroke through updateKey, which is what
// parse-gates edits into the node) leaves the canonical node untouched until a
// real flush. Only navigation and commit reconcile the buffer into the node.
func TestCollectionNav_RawSetValueDoesNotMutateNode(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
	be := newBlockEdit(Config{}, spec, 100, 40)

	original := be.entryYAML(0)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	if be.entryYAML(0) != original {
		t.Fatalf("canonical node was mutated by a raw SetValue; got %q, want %q",
			be.entryYAML(0), original)
	}
}

// TestCollectionNav_CommitFlushesAndSerializesAll verifies that commit() flushes
// the current buffer into entries and includes all entries in the snippet.
func TestCollectionNav_CommitFlushesAndSerializesAll(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
	be := newBlockEdit(Config{}, spec, 100, 40)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")
	be.dirty = true

	_, cmd := be.commit()
	msg := cmd().(blockEditCommittedMsg)

	if !strings.Contains(msg.Snippet, "name: alpha_edited") {
		t.Fatalf("snippet missing edited entry: %s", msg.Snippet)
	}
	if !strings.Contains(msg.Snippet, "name: beta") {
		t.Fatalf("snippet missing second entry: %s", msg.Snippet)
	}
}

// TestCollectionNav_DoubleCommitIdempotent is a regression test for the
// duplication bug: committing twice must produce identical snippets.
func TestCollectionNav_DoubleCommitIdempotent(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
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

	if snippet1 != snippet2 {
		t.Fatalf("double commit diverged:\nfirst:  %q\nsecond: %q", snippet1, snippet2)
	}
	if strings.Count(snippet2, "name: beta") != 1 {
		t.Fatalf("duplication detected: %q", snippet2)
	}
}

// TestPrimitiveBlock_showsFieldItemAndHint verifies that a tree-less block shows
// the field itself in the left panel and its metadata in the hint panel, instead
// of the "(no fields)" / "select a field" placeholders.
func TestPrimitiveBlock_showsFieldItemAndHint(t *testing.T) {
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

	if !be.tree.isEmpty() {
		t.Fatal("expected an empty tree for a primitive block")
	}

	left := be.fieldItemView()
	if !strings.Contains(left, "debug") {
		t.Errorf("left panel should name the field; got %q", left)
	}

	hint := be.hintContent()
	for _, want := range []string{"Type:", "bool", "Default:", "false"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint panel missing %q; got %q", want, hint)
		}
	}

	if view := be.View(nil); strings.Contains(view, "(no fields)") {
		t.Error("full view should no longer show the (no fields) placeholder")
	}
}

// TestRenderFieldHint_typeAndRequiredBehavior verifies that Type is shown only
// when set and Required is shown only when true.
func TestRenderFieldHint_typeAndRequiredBehavior(t *testing.T) {
	th := resolveTheme(theme.Theme{})

	t.Run("type shown when set", func(t *testing.T) {
		out := renderFieldHint(th, FieldMeta{Type: "string"}, "")
		if !strings.Contains(out, "Type:") || !strings.Contains(out, "string") {
			t.Errorf("expected Type: string in output; got %q", out)
		}
	})

	t.Run("type omitted when empty", func(t *testing.T) {
		out := renderFieldHint(th, FieldMeta{Description: "desc"}, "")
		if strings.Contains(out, "Type:") {
			t.Errorf("expected no Type line when Type is empty; got %q", out)
		}
	})

	t.Run("required shown only when true", func(t *testing.T) {
		out := renderFieldHint(th, FieldMeta{Required: true}, "")
		if !strings.Contains(out, "Required:") || !strings.Contains(out, "yes") {
			t.Errorf("expected Required: yes; got %q", out)
		}
	})

	t.Run("required omitted when false", func(t *testing.T) {
		out := renderFieldHint(th, FieldMeta{Description: "desc"}, "")
		if strings.Contains(out, "Required:") {
			t.Errorf("expected no Required line when false; got %q", out)
		}
	})
}

// TestRenderFieldHint_constraints verifies that the constraint fields render
// when set and stay absent on a zero FieldMeta.
func TestRenderFieldHint_constraints(t *testing.T) {
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
		if !strings.Contains(out, want) {
			t.Errorf("rendered hint should contain %q; got:\n%s", want, out)
		}
	}
	if empty := renderFieldHint(th, FieldMeta{}, ""); strings.Contains(empty, "Range:") ||
		strings.Contains(empty, "Pattern:") || strings.Contains(empty, "Entries:") ||
		strings.Contains(empty, "Unique:") || strings.Contains(empty, "Deprecated:") {
		t.Errorf("zero FieldMeta must render no constraint lines; got:\n%s", empty)
	}
}

// TestRestoreUndo_emptyStackIsNoOp guards restoreUndo against an empty stack:
// the function must not panic and must return the state unchanged.
func TestRestoreUndo_emptyStackIsNoOp(t *testing.T) {
	be := newBlockEdit(Config{}, blockSpec{key: "debug", kind: schema.KindPrimitive}, 100, 40)
	if len(be.undoStack) != 0 {
		t.Fatal("a freshly opened editor must have an empty undo stack")
	}
	got := be.restoreUndo() // must not panic with an empty stack
	if len(got.undoStack) != 0 {
		t.Error("restoreUndo on an empty stack should leave it empty")
	}
}

// ---------------------------------------------------------------------------
// resyncTreeFromYAML - tolerant, non-authoritative visual projection
// ---------------------------------------------------------------------------

// TestResyncToleratesInvalidYAML_struct verifies that a transiently unparseable
// buffer (mid-typing) neither panics nor wipes the tree's checked state: the
// per-keystroke resync leaves the last good visual state in place.
func TestResyncToleratesInvalidYAML_struct(t *testing.T) {
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
	if len(after) != len(before) {
		t.Fatalf("tree fields changed on invalid YAML: before %d, after %d", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("checked state for %q changed on invalid YAML: %v → %v (state should be preserved)", k, v, after[k])
		}
	}
}

// TestResyncToleratesInvalidYAML_collection verifies the same tolerance for a
// collection navigator: an unparseable current entry preserves the entry's label
// and never mutates the canonical entries slice.
func TestResyncToleratesInvalidYAML_collection(t *testing.T) {
	be := newBlockEdit(Config{}, seqSpec(`categories:
  - name: alpha
`), 100, 40)
	nodeBefore := nodeToContent(be.key, be.node)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: [unterminated\n")

	tm := be.resyncTreeFromYAML() // must not panic

	// The canonical node must be untouched: the tree is derived from it, not from
	// the (now invalid) buffer.
	if got := nodeToContent(be.key, be.node); got != nodeBefore {
		t.Fatalf("resync mutated canonical node:\nbefore %q\nafter  %q", nodeBefore, got)
	}
	// The existing item label must survive an unparseable buffer.
	foundAlpha := false
	for _, n := range tm.nodes {
		if n.kind == treeNodeSeqItem && n.label == "alpha" {
			foundAlpha = true
		}
	}
	if !foundAlpha {
		t.Error("seq item label \"alpha\" was lost on invalid YAML")
	}
}

// ---------------------------------------------------------------------------
// editorH - must never return a negative value
// ---------------------------------------------------------------------------

func TestEditorH_nonNegative(t *testing.T) {
	heights := []int{1, 2, 3, 5, 7, 10, 20}
	spec := seqSpec(`categories:
  - name: a
`)
	for _, h := range heights {
		be := newBlockEdit(Config{}, spec, 100, h)
		if got := be.editorH(); got < 0 {
			t.Errorf("editorH() = %d at terminal height %d - must be >= 0", got, h)
		}
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
	if be.mode != modeConfirming {
		t.Fatalf("ctrl+d on nested parent did not offer removal (mode=%d, want modeConfirming)", be.mode)
	}

	// Locate the captured "before" node index and confirm the removal.
	beforeIdx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "before" {
			beforeIdx = i
			break
		}
	}
	if beforeIdx < 0 {
		t.Fatal("before node not found")
	}
	be = be.dispatch(ToggleField{NodeIdx: beforeIdx, Checked: false})

	got := be.yamlEditor.Value()
	if strings.Contains(got, "before:") {
		t.Errorf("before block was not removed:\n%s", got)
	}
	if !strings.Contains(got, "after:") {
		t.Errorf("after block should remain:\n%s", got)
	}
}

// TestRedo_structFieldRemove verifies the undo→redo round-trip: removing a
// field, undoing, then redoing lands back on the post-remove state, and the
// redo itself can be undone again.
func TestRedo_structFieldRemove(t *testing.T) {
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
	if strings.Contains(removed, "output:") {
		t.Fatalf("output still present after remove: %q", removed)
	}

	be = be.restoreUndo()
	if be.yamlEditor.Value() != original {
		t.Fatalf("after undo YAML = %q, want %q", be.yamlEditor.Value(), original)
	}
	if len(be.redoStack) == 0 {
		t.Fatal("redoStack must be non-empty after undo")
	}

	be = be.restoreRedo()
	if be.yamlEditor.Value() != removed {
		t.Errorf("after redo YAML = %q, want %q", be.yamlEditor.Value(), removed)
	}

	// The redo itself is undoable.
	be = be.restoreUndo()
	if be.yamlEditor.Value() != original {
		t.Errorf("after undoing the redo YAML = %q, want %q", be.yamlEditor.Value(), original)
	}
}

// TestRedo_clearedByNewMutation verifies that a new mutation after an undo
// discards the redo stack - the editor forks away from the undone state.
func TestRedo_clearedByNewMutation(t *testing.T) {
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
	if len(be.redoStack) == 0 {
		t.Fatal("redoStack must be non-empty after undo")
	}

	// New mutation (remove again) must clear the redo stack.
	be, _ = be.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if len(be.redoStack) != 0 {
		t.Errorf("redoStack must be cleared by a new mutation, got %d entries", len(be.redoStack))
	}
}

// TestRestoreRedo_emptyStackIsNoOp guards restoreRedo against an empty stack.
func TestRestoreRedo_emptyStackIsNoOp(t *testing.T) {
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	got := be.restoreRedo() // must not panic with an empty stack
	if len(got.redoStack) != 0 {
		t.Error("restoreRedo on an empty stack should leave it empty")
	}
}
