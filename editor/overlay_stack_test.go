package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
// the whole block to the document - structurally intact, with no per-level splice.
func TestDrillInCommitsThroughCanonicalTree(t *testing.T) {
	type ceProbe struct {
		HTTPRoutes map[string]struct {
			Host string `yaml:"host,omitempty"`
		} `yaml:"httproutes,omitempty"`
	}
	type rootProbe struct {
		ContainerEngine *ceProbe `yaml:"containerengine,omitempty"`
	}

	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "w.yaml")
	must.NoError(os.WriteFile(path, []byte(`containerengine:
  httproutes:
    web:
      host: example.com
`), 0o600))
	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "containerengine", Existing: true}})
	m = updated.(model)
	must.Len(m.blockEdits, 1, "after open: stack depth should be 1")

	// Drill into httproutes by focus suffix; the model resolves content from editRoot.
	updated, _ = m.Update(openChildMsg{
		key:     "httproutes",
		defs:    []schema.FieldDef{{YAMLName: "host", Kind: schema.KindPrimitive}},
		kind:    schema.KindDictionary,
		relSegs: []pathSeg{segKey("httproutes")},
	})
	m = updated.(model)
	must.Len(m.blockEdits, 2, "after drill-in: stack depth should be 2")
	is.True(m.topBE().isMapNav(), "child editor should be a map navigator")
	got := m.topBE().yamlEditor.Value()
	assert.Contains(t, got, "web", "child editor did not receive existing content from canonical tree")
	assert.Contains(t, got, "host: example.com", "child editor did not receive existing content from canonical tree")

	// Ctrl+S emits a commit request; the model commits the whole stack through the
	// canonical tree and returns to the list.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	must.NotNil(cmd, "ctrl+s should emit a commit request")
	updated, _ = m.Update(cmd())
	m = updated.(model)
	must.Empty(m.blockEdits, "after ctrl+s: stack should be empty (returned to list)")

	// The document must still hold a structurally-intact nested mapping.
	var check rootProbe
	must.NoError(yaml.Unmarshal(m.doc.Raw(), &check), "committed doc is not structurally valid")
	must.NotNil(check.ContainerEngine, "nested content lost")
	is.Equal("example.com", check.ContainerEngine.HTTPRoutes["web"].Host, "nested content corrupted")
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

	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "w.yaml")
	must.NoError(os.WriteFile(path, []byte(`containerengine:
  httproutes:
    web:
      host: old.com
`), 0o600))
	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	must.NoError(err, "newModel")
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
	must.Len(m.blockEdits, 2, "after drill-in: stack depth should be 2")

	// Edit the child: change the route host.
	child := *m.topBE()
	child.yamlEditor.SetValue(`httproutes:
  web:
    host: new.com
`)
	child.dirty = true
	m = m.withTopBE(child)

	// Esc inside the nested editor → drill out, keeping the edit.
	updated, _ = m.Update(drillOutMsg{})
	m = updated.(model)
	must.Len(m.blockEdits, 1, "after drill-out: stack depth should be 1 (back at parent)")
	// The parent editor must reflect the child's edit (refreshed from canonical tree).
	is.Contains(m.topBE().yamlEditor.Value(), "new.com", "parent did not reflect the child edit after drill-out")
	// And the block must be dirty so leaving to the list still warns.
	is.True(m.topBE().dirty, "block should be dirty after keeping child edits")

	// Ctrl+S then persists the kept edit through the canonical tree.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = updated.(model)
	must.NotNil(cmd, "ctrl+s should emit a commit request")
	updated, _ = m.Update(cmd())
	m = updated.(model)
	var check rootProbe
	must.NoError(yaml.Unmarshal(m.doc.Raw(), &check), "doc invalid after commit")
	must.NotNil(check.ContainerEngine, "kept edit not persisted: ContainerEngine nil")
	is.Equal("new.com", check.ContainerEngine.HTTPRoutes["web"].Host, "kept edit not persisted")
}

// TestDrillOutFromSeqNavInsideMapNav verifies that ESC works when the stack is:
// list → KindDictionary (map nav) → KindList (seq nav). The KindList editor is
// opened by drilling into a struct list field within an expanded map-nav entry.
func TestDrillOutFromSeqNavInsideMapNav(t *testing.T) {
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "w.yaml")
	must.NoError(os.WriteFile(path, []byte(`field2:
  key1:
    A: foo
    B:
      - C: bar
`), 0o600))

	type subField1Probe struct {
		C string `yaml:"C,omitempty"`
	}
	type field2Probe struct {
		A string           `yaml:"A,omitempty"`
		B []subField1Probe `yaml:"B,omitempty"`
	}
	type rootProbe struct {
		Field2 map[string]*field2Probe `yaml:"field2,omitempty"`
	}

	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	must.NoError(err)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Open "field2" from the list (top-level KindDictionary editor, focus = nil).
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "field2", Existing: true}})
	m = updated.(model)
	must.Len(m.blockEdits, 1, "stack depth after opening field2")

	subField1Defs := []schema.FieldDef{{YAMLName: "C", Kind: schema.KindPrimitive}}
	field2ChildDefs := []schema.FieldDef{
		{YAMLName: "A", Kind: schema.KindPrimitive},
		{YAMLName: "B", Kind: schema.KindList, Children: subField1Defs},
	}

	// Drill into B via the same path handleTreeOpenChild would emit for a map nav
	// entry: relSegs = [segKey("key1"), segKey("B")].
	updated, _ = m.Update(openChildMsg{
		key:     "B",
		defs:    subField1Defs,
		kind:    schema.KindList,
		relSegs: []pathSeg{segKey("key1"), segKey("B")},
	})
	m = updated.(model)
	must.Len(m.blockEdits, 2, "stack depth after drilling into B")
	must.True(m.topBE().isSeqNav(), "B editor should be a seq navigator")
	_ = field2ChildDefs // suppress unused

	// ESC inside B editor must navigate back to field2 (depth 1), not exit to list.
	updated, _ = m.Update(drillOutMsg{})
	m = updated.(model)
	must.Len(m.blockEdits, 1, "after ESC: stack depth should be 1 (back at field2)")
	must.Equal("field2", m.topBE().key, "after ESC: should be back at field2 editor")
}

// TestEscKeyFromSeqNavInsideMapNav is the same scenario as
// TestDrillOutFromSeqNavInsideMapNav but drives the full ESC→cmd→drillOut path
// instead of injecting drillOutMsg directly. This ensures the key handler in
// updateKey emits drillOutMsg for nested editors (len(be.focus) > 0).
func TestEscKeyFromSeqNavInsideMapNav(t *testing.T) {
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "w.yaml")
	must.NoError(os.WriteFile(path, []byte(`field2:
  key1:
    A: foo
    B:
      - C: bar
`), 0o600))

	type subField1Probe struct {
		C string `yaml:"C,omitempty"`
	}
	type field2Probe struct {
		A string           `yaml:"A,omitempty"`
		B []subField1Probe `yaml:"B,omitempty"`
	}
	type rootProbe struct {
		Field2 map[string]*field2Probe `yaml:"field2,omitempty"`
	}

	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	must.NoError(err)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "field2", Existing: true}})
	m = updated.(model)
	must.Len(m.blockEdits, 1, "stack depth after opening field2")

	subField1Defs := []schema.FieldDef{{YAMLName: "C", Kind: schema.KindPrimitive}}

	updated, _ = m.Update(openChildMsg{
		key:     "B",
		defs:    subField1Defs,
		kind:    schema.KindList,
		relSegs: []pathSeg{segKey("key1"), segKey("B")},
	})
	m = updated.(model)
	must.Len(m.blockEdits, 2, "stack depth after drilling into B")
	must.True(m.topBE().isSeqNav(), "B editor should be a seq navigator")

	// Send the actual ESC key — updateKey must emit drillOutMsg as a cmd
	// (not drillOutMsg directly) because len(be.focus) > 0.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	must.Len(m.blockEdits, 2, "after ESC key: stack not yet popped (cmd pending)")
	must.NotNil(cmd, "ESC in a nested editor must return a non-nil cmd (drillOutMsg)")

	// Execute the cmd: it must return drillOutMsg, which the model then processes.
	msg := cmd()
	_, ok := msg.(drillOutMsg)
	must.True(ok, "cmd returned by ESC in nested editor must be drillOutMsg")

	updated, _ = m.Update(msg)
	m = updated.(model)
	must.Len(m.blockEdits, 1, "after drillOutMsg: stack depth should be 1 (back at field2)")
	must.Equal("field2", m.topBE().key, "after drillOutMsg: should be back at field2 editor")
}

// TestDrillOutFromEmptyParent verifies that ESC from a child editor works even
// when the parent editor had empty content ("key:\n"). That flush writes a null
// scalar into editRoot; setNodeAt must coerce it to a mapping so the child's
// drill-out write succeeds instead of silently aborting.
func TestDrillOutFromEmptyParent(t *testing.T) {
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "w.yaml")
	// Parent block is present but has no fields set yet.
	must.NoError(os.WriteFile(path, []byte("gateway:\n"), 0o600))

	type serversDef struct {
		Host string `yaml:"host,omitempty"`
	}
	type gatewayDef struct {
		Servers []serversDef `yaml:"servers,omitempty"`
	}
	type rootProbe struct {
		Gateway *gatewayDef `yaml:"gateway,omitempty"`
	}

	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	must.NoError(err)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Open the "gateway" block (KindObject, empty content).
	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "gateway", Existing: true}})
	m = updated.(model)
	must.Len(m.blockEdits, 1, "stack depth after opening gateway")

	serversDefs := []schema.FieldDef{{YAMLName: "host", Kind: schema.KindPrimitive}}

	// Drill into "servers" (a KindList child). The parent flush writes a null
	// scalar into editRoot; setNodeAt must handle that for this to succeed.
	updated, _ = m.Update(openChildMsg{
		key:     "servers",
		defs:    serversDefs,
		kind:    schema.KindList,
		relSegs: []pathSeg{segKey("servers")},
	})
	m = updated.(model)
	must.Len(m.blockEdits, 2, "stack depth after drilling into servers")

	// ESC from the child editor must pop back to gateway, not abort.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	must.NotNil(cmd, "ESC in nested editor must return drillOutMsg cmd")
	msg := cmd()
	_, ok := msg.(drillOutMsg)
	must.True(ok, "cmd must be drillOutMsg")

	updated, _ = m.Update(msg)
	m = updated.(model)
	must.Len(m.blockEdits, 1, "after drill-out: back at gateway editor")
	must.Equal("gateway", m.topBE().key, "after drill-out: should be at gateway editor")
}

// TestFlushTopToRoot_rollbackOnSetNodeAtFailure verifies that editRoot is
// atomically restored when setNodeAt fails mid-traversal. Without rollback, a
// failed flush can leave editRoot in a partial state where intermediate nodes
// were already created before the failure.
func TestFlushTopToRoot_rollbackOnSetNodeAtFailure(t *testing.T) {
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "w.yaml")
	must.NoError(os.WriteFile(path, []byte("gateway:\n"), 0o600))

	type gatewayDef struct {
		Host string `yaml:"host,omitempty"`
	}
	type rootProbe struct {
		Gateway *gatewayDef `yaml:"gateway,omitempty"`
	}

	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	must.NoError(err)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "gateway", Existing: true}})
	m = updated.(model)
	must.Len(m.blockEdits, 1)

	// Snapshot editRoot before corrupting the focus.
	snapBefore, _ := yaml.Marshal(m.editRoot)

	// Set an impossible focus: sequence index 999 on a mapping editRoot.
	// setNodeAt will fail because editRoot.Kind != SequenceNode.
	be := *m.topBE()
	be.focus = []pathSeg{segIdx(999)}
	m = m.withTopBE(be)

	_, ok := m.flushTopToRoot()
	must.False(ok, "flushTopToRoot must return false on setNodeAt failure")

	snapAfter, _ := yaml.Marshal(m.editRoot)
	must.Equal(string(snapBefore), string(snapAfter), "editRoot must be identical after a failed flush (rollback)")
}

// TestDrillOutFromEmptyList verifies that drilling into an empty list child and
// immediately back out does not leave a phantom empty mapping in editRoot.
func TestDrillOutFromEmptyList(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "w.yaml")
	must.NoError(os.WriteFile(path, []byte("gateway:\n"), 0o600))

	type serversDef struct {
		Host string `yaml:"host,omitempty"`
	}
	type gatewayDef struct {
		Servers []serversDef `yaml:"servers,omitempty"`
	}
	type rootProbe struct {
		Gateway *gatewayDef `yaml:"gateway,omitempty"`
	}

	m, err := newModel(Config{Path: path, Schema: &rootProbe{}})
	must.NoError(err)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "gateway", Existing: true}})
	m = updated.(model)

	serversDefs := []schema.FieldDef{{YAMLName: "host", Kind: schema.KindPrimitive}}

	// Drill into "servers" (empty list) without adding anything.
	updated, _ = m.Update(openChildMsg{
		key:     "servers",
		defs:    serversDefs,
		kind:    schema.KindList,
		relSegs: []pathSeg{segKey("servers")},
	})
	m = updated.(model)
	must.Len(m.blockEdits, 2)

	// Drill back out immediately.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	must.NotNil(cmd)
	updated, _ = m.Update(cmd())
	m = updated.(model)
	must.Len(m.blockEdits, 1, "back at gateway editor")

	// editRoot must not contain a "servers" key with empty content.
	snap, _ := yaml.Marshal(m.editRoot)
	is.NotContains(string(snap), "servers", "empty servers list must be pruned on drill-out")
}

// ---------------------------------------------------------------------------
// Nested toggle combinations - deep nesting, pruning, and interaction probes
// ---------------------------------------------------------------------------

// catDefs mirrors the movelooper category schema shape: nested structs (source,
// source.filter), scalar lists, and a hooks struct with before/after children.
