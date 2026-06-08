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
