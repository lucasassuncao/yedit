package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

// ceStructSpec is a struct block that contains a nested map-of-struct field
// (httproutes), mirroring containerengine in the workload schema.
func ceStructSpec() blockSpec {
	return blockSpec{
		key: "containerengine",
		defs: []schema.FieldDef{
			{YAMLName: "deployment", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "replicas", Kind: schema.KindPrimitive},
			}},
			{YAMLName: "httproutes", Kind: schema.KindDictionary, Children: []schema.FieldDef{
				{YAMLName: "host", Kind: schema.KindPrimitive},
				{YAMLName: "port", Kind: schema.KindPrimitive},
			}},
		},
		kind: schema.KindObject,
		content: `containerengine:
  httproutes:
    web:
      host: example.com
`,
	}
}

func nodeByLabel(be blockEditState, label string) (treeNode, bool) {
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == label {
			return n, true
		}
	}
	return treeNode{}, false
}

func cursorToLabel(be blockEditState, label string) blockEditState {
	for vi, ni := range be.tree.visibleNodes() {
		if be.tree.nodes[ni].label == label {
			be.tree.cursor = vi
			break
		}
	}
	return be
}

// TestNestedMapFieldIsOpenable guards that a map-of-struct field in a struct
// block is flagged openable (drill-in) rather than a plain toggle leaf.
func TestNestedMapFieldIsOpenable(t *testing.T) {
	be := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	n, ok := nodeByLabel(be, "httproutes")
	if !ok {
		t.Fatal("httproutes node not found")
	}
	if !n.openable {
		t.Error("httproutes (map-of-struct) should be openable")
	}
	// deployment is a plain nested struct: expandable inline, not openable.
	d, ok := nodeByLabel(be, "deployment")
	if !ok {
		t.Fatal("deployment node not found")
	}
	if d.openable {
		t.Error("deployment (struct) should not be openable")
	}
}

// TestEnterOnNestedMapEmitsOpenChild guards that pressing Enter on the nested
// map field emits an openChildMsg scoped to that field, carrying the focus-path
// suffix to it (the model resolves content from the canonical tree).
func TestEnterOnNestedMapEmitsOpenChild(t *testing.T) {
	be := newBlockEdit(Config{}, ceStructSpec(), 100, 40)
	be = cursorToLabel(be, "httproutes")
	_, cmd := be.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on httproutes produced no command")
	}
	msg, ok := cmd().(openChildMsg)
	if !ok {
		t.Fatalf("expected openChildMsg, got %T", cmd())
	}
	if msg.key != "httproutes" || msg.kind != schema.KindDictionary {
		t.Errorf("openChildMsg = {key:%q kind:%d}, want {httproutes map}", msg.key, msg.kind)
	}
	// A struct block addresses its child by a single mapping-key segment.
	if len(msg.relSegs) != 1 || msg.relSegs[0].isIndex || msg.relSegs[0].key != "httproutes" {
		t.Errorf("relSegs = %+v, want [segKey(httproutes)]", msg.relSegs)
	}
}

// TestDrillInCommitsThroughCanonicalTree exercises the canonical-tree model:
// drilling into a nested field reads its content from editRoot (no substring
// copy), and Ctrl+S flushes the focused editor back into editRoot and serializes
// the whole block to the document — structurally intact, with no per-level splice.
func TestDrillInCommitsThroughCanonicalTree(t *testing.T) {
	type ceProbe struct {
		HTTPRoutes map[string]struct {
			Host string `yaml:"host,omitempty"`
		} `yaml:"httproutes,omitempty"`
	}
	type rootProbe struct {
		ContainerEngine *ceProbe `yaml:"containerengine,omitempty"`
	}

	path := filepath.Join(t.TempDir(), "w.yaml")
	if err := os.WriteFile(path, []byte(`containerengine:
  httproutes:
    web:
      host: example.com
`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "containerengine", Existing: true}})
	m = updated.(model)
	if len(m.blockEdits) != 1 {
		t.Fatalf("after open: stack depth %d, want 1", len(m.blockEdits))
	}

	// Drill into httproutes by focus suffix; the model resolves content from editRoot.
	updated, _ = m.Update(openChildMsg{
		key:     "httproutes",
		defs:    []schema.FieldDef{{YAMLName: "host", Kind: schema.KindPrimitive}},
		kind:    schema.KindDictionary,
		relSegs: []pathSeg{segKey("httproutes")},
	})
	m = updated.(model)
	if len(m.blockEdits) != 2 {
		t.Fatalf("after drill-in: stack depth %d, want 2", len(m.blockEdits))
	}
	if !m.topBE().isMapNav() {
		t.Error("child editor should be a map navigator")
	}
	if got := m.topBE().yamlEditor.Value(); !strings.Contains(got, "web") || !strings.Contains(got, "host: example.com") {
		t.Errorf("child editor did not receive existing content from canonical tree:\n%s", got)
	}

	// Ctrl+S commits the whole stack through the canonical tree and returns to list.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	if len(m.blockEdits) != 0 {
		t.Fatalf("after ctrl+s: stack depth %d, want 0 (returned to list)", len(m.blockEdits))
	}

	// The document must still hold a structurally-intact nested mapping.
	var check rootProbe
	if err := yaml.Unmarshal(m.doc.Raw(), &check); err != nil {
		t.Fatalf("committed doc is not structurally valid: %v\n%s", err, m.doc.Raw())
	}
	if check.ContainerEngine == nil || check.ContainerEngine.HTTPRoutes["web"].Host != "example.com" {
		t.Errorf("nested content lost or corrupted:\n%s", m.doc.Raw())
	}
}

// TestDrillOutKeepsEdits verifies that Esc inside a nested editor navigates back
// up one level while PRESERVING the edits made there (flushed into the canonical
// tree), so the user can drill in, edit, return to fix a parent field, and lose
// nothing. This is the drill-out the stack lacked.
func TestDrillOutKeepsEdits(t *testing.T) {
	type ceProbe struct {
		HTTPRoutes map[string]struct {
			Host string `yaml:"host,omitempty"`
		} `yaml:"httproutes,omitempty"`
	}
	type rootProbe struct {
		ContainerEngine *ceProbe `yaml:"containerengine,omitempty"`
	}

	path := filepath.Join(t.TempDir(), "w.yaml")
	if err := os.WriteFile(path, []byte(`containerengine:
  httproutes:
    web:
      host: old.com
`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "containerengine", Existing: true}})
	m = updated.(model)

	// Drill into httproutes.
	updated, _ = m.Update(openChildMsg{
		key:     "httproutes",
		defs:    []schema.FieldDef{{YAMLName: "host", Kind: schema.KindPrimitive}},
		kind:    schema.KindDictionary,
		relSegs: []pathSeg{segKey("httproutes")},
	})
	m = updated.(model)
	if len(m.blockEdits) != 2 {
		t.Fatalf("after drill-in: stack depth %d, want 2", len(m.blockEdits))
	}

	// Edit the child: change the route host.
	child := *m.topBE()
	child.yamlEditor.SetValue(`httproutes:
  web:
    host: new.com
`)
	child.dirty = true
	m.setTopBE(&child)

	// Esc inside the nested editor → drill out, keeping the edit.
	updated, _ = m.Update(drillOutMsg{})
	m = updated.(model)
	if len(m.blockEdits) != 1 {
		t.Fatalf("after drill-out: stack depth %d, want 1 (back at parent)", len(m.blockEdits))
	}
	// The parent editor must reflect the child's edit (refreshed from canonical tree).
	if got := m.topBE().yamlEditor.Value(); !strings.Contains(got, "new.com") {
		t.Errorf("parent did not reflect the child edit after drill-out:\n%s", got)
	}
	// And the block must be dirty so leaving to the list still warns.
	if !m.topBE().dirty {
		t.Error("block should be dirty after keeping child edits")
	}

	// Ctrl+S then persists the kept edit through the canonical tree.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	var check rootProbe
	if err := yaml.Unmarshal(m.doc.Raw(), &check); err != nil {
		t.Fatalf("doc invalid after commit: %v\n%s", err, m.doc.Raw())
	}
	if check.ContainerEngine == nil || check.ContainerEngine.HTTPRoutes["web"].Host != "new.com" {
		t.Errorf("kept edit not persisted:\n%s", m.doc.Raw())
	}
}

// ---------------------------------------------------------------------------
// Nested toggle combinations — deep nesting, pruning, and interaction probes
// ---------------------------------------------------------------------------

// catDefs mirrors the movelooper category schema shape: nested structs (source,
// source.filter), scalar lists, and a hooks struct with before/after children.
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
// as having content — for both coloring and ctrl+d removal.
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
// list-of-struct field (filter.any) must not spawn phantom inline child nodes —
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
	be, _ = be.Update(pendingEntryDeleteMsg{seqIdx: 0})
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
// removal and an empty leaf removes directly — identically at top level and when
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
		be, _ = be.Update(pendingRemoveMsg{nodeIdx: idx})
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
