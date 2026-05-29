package editor

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
)

// ceStructSpec is a struct block that contains a nested map-of-struct field
// (httproutes), mirroring containerengine in the workload schema.
func ceStructSpec() blockSpec {
	return blockSpec{
		key: "containerengine",
		defs: []schema.FieldDef{
			{YAMLName: "deployment", Kind: schema.KindStruct, Children: []schema.FieldDef{
				{YAMLName: "replicas", Kind: schema.KindScalar},
			}},
			{YAMLName: "httproutes", Kind: schema.KindMap, Children: []schema.FieldDef{
				{YAMLName: "host", Kind: schema.KindScalar},
				{YAMLName: "port", Kind: schema.KindScalar},
			}},
		},
		kind:    schema.KindStruct,
		content: "containerengine:\n  httproutes:\n    web:\n      host: example.com\n",
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
// map field emits an openChildMsg scoped to that field, carrying its current
// content and splice path.
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
	if msg.key != "httproutes" || msg.kind != schema.KindMap {
		t.Errorf("openChildMsg = {key:%q kind:%d}, want {httproutes map}", msg.key, msg.kind)
	}
	if len(msg.path) != 1 || msg.path[0] != "httproutes" {
		t.Errorf("splice path = %v, want [httproutes]", msg.path)
	}
	if !strings.Contains(msg.content, "web:") || !strings.Contains(msg.content, "host: example.com") {
		t.Errorf("child content missing existing entry:\n%s", msg.content)
	}
}

func TestExtractSubBlock(t *testing.T) {
	parent := "containerengine:\n  deployment:\n    replicas: 2\n  httproutes:\n    web:\n      host: example.com\n"
	got := extractSubBlock(parent, []string{"httproutes"})
	want := "httproutes:\n  web:\n    host: example.com\n"
	if got != want {
		t.Errorf("extractSubBlock:\n got %q\nwant %q", got, want)
	}
	// Absent path yields an empty collection header.
	if got := extractSubBlock("containerengine:\n  deployment:\n    replicas: 2\n", []string{"httproutes"}); got != "httproutes:\n" {
		t.Errorf("absent path = %q, want %q", got, "httproutes:\n")
	}
}

func TestReplaceSubBlock(t *testing.T) {
	parent := "containerengine:\n  deployment:\n    replicas: 2\n"
	child := "httproutes:\n  web:\n    host: example.com\n"
	got := replaceSubBlock(parent, []string{"httproutes"}, child)
	if !strings.Contains(got, "httproutes:") || !strings.Contains(got, "web:") || !strings.Contains(got, "host: example.com") {
		t.Errorf("replaceSubBlock dropped the child block:\n%s", got)
	}
	if !strings.Contains(got, "deployment:") {
		t.Errorf("replaceSubBlock dropped the sibling deployment block:\n%s", got)
	}
	// An empty child removes the key.
	pruned := replaceSubBlock(got, []string{"httproutes"}, "httproutes:\n")
	if strings.Contains(pruned, "httproutes:") {
		t.Errorf("empty child should prune the key:\n%s", pruned)
	}
}

func TestExtractReplaceRoundTrip(t *testing.T) {
	parent := "containerengine:\n  httproutes:\n    web:\n      host: example.com\n      port: 8080\n"
	child := extractSubBlock(parent, []string{"httproutes"})
	got := replaceSubBlock(parent, []string{"httproutes"}, child)
	if !strings.Contains(got, "port: 8080") || !strings.Contains(got, "host: example.com") {
		t.Errorf("round-trip lost fields:\n%s", got)
	}
}

// TestDrillInSplicesBackToParent exercises the model stack: opening a child map
// editor and committing it must splice the snippet into the parent block and pop
// back to the parent.
func TestDrillInSplicesBackToParent(t *testing.T) {
	type ceProbe struct {
		HTTPRoutes map[string]struct {
			Host string `yaml:"host,omitempty"`
		} `yaml:"httproutes,omitempty"`
	}
	type rootProbe struct {
		ContainerEngine *ceProbe `yaml:"containerengine,omitempty"`
	}

	m, err := newModel(Config{
		Path:   filepath.Join(t.TempDir(), "w.yaml"),
		Schema: &rootProbe{},
	})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(openItemMsg{Item: listItem{Key: "containerengine"}})
	m = updated.(model)
	if len(m.blockEdits) != 1 {
		t.Fatalf("after open: stack depth %d, want 1", len(m.blockEdits))
	}

	updated, _ = m.Update(openChildMsg{
		key:     "httproutes",
		defs:    []schema.FieldDef{{YAMLName: "host", Kind: schema.KindScalar}},
		kind:    schema.KindMap,
		content: "httproutes:\n",
		path:    []string{"httproutes"},
	})
	m = updated.(model)
	if len(m.blockEdits) != 2 {
		t.Fatalf("after drill-in: stack depth %d, want 2", len(m.blockEdits))
	}
	if !m.topBE().isMapNav() {
		t.Error("child editor should be a map navigator")
	}

	updated, _ = m.Update(blockEditCommittedMsg{Snippet: "httproutes:\n  web:\n    host: example.com\n"})
	m = updated.(model)
	if len(m.blockEdits) != 1 {
		t.Fatalf("after child commit: stack depth %d, want 1 (popped)", len(m.blockEdits))
	}
	parent := m.topBE().yamlEditor.Value()
	if !strings.Contains(parent, "httproutes:") || !strings.Contains(parent, "host: example.com") {
		t.Errorf("child commit not spliced into parent:\n%s", parent)
	}
	if !m.topBE().dirty {
		t.Error("parent should be marked dirty after splice")
	}
}
