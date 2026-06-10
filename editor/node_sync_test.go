package editor

import (
	"github.com/lucasassuncao/yedit/internal/yamlnode"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

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
// valueNode — the structural ground truth a leaf's checked flag must mirror.
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
	for _, n := range be.tree.nodes {
		if n.kind != treeNodeField || !n.isLeaf {
			continue
		}
		want := keyExistsInNode(be.node, n.yamlPath)
		if n.checked != want {
			t.Errorf("tree/node disagree for %v: tree.checked=%v node-has-key=%v",
				n.yamlPath, n.checked, want)
		}
	}
}

// cursorToAddNew moves the tree cursor onto the "+ add new" row.
func cursorToAddNew(be blockEditState) blockEditState {
	for vi, ni := range be.tree.visibleNodes() {
		if be.tree.nodes[ni].kind == treeNodeAddNew {
			be.tree.cursor = vi
			break
		}
	}
	return be
}

func newStructEdit(t *testing.T, content string) blockEditState {
	t.Helper()
	return newBlockEdit(Config{}, blockSpec{
		key: "cfg", defs: cfgStructDefs(), kind: schema.KindObject, content: content,
	}, 120, 40)
}

// TestSOT_ToggleWhileBufferInvalid is the Classe A regression: with the YAML
// buffer mid-edit and unparseable, toggling a field in the tree must still leave
// tree and node in agreement and the buffer valid — the desync that motivated
// the single-source-of-truth refactor.
func TestSOT_ToggleWhileBufferInvalid(t *testing.T) {
	be := newStructEdit(t, "cfg:\n")

	// Simulate the user having typed invalid YAML (a tab indent) into the buffer.
	// The parse gate leaves be.node at its last good state (an empty mapping).
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("cfg:\n  bad: [unclosed\n")
	if validateSnippetText(be.yamlEditor.Value()) == nil {
		t.Fatal("test setup: expected the crafted buffer to be invalid YAML")
	}

	// Toggle a field via the tree.
	be.active = blockEditPanelTree
	be = cursorToLabel(be, "output")
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})

	if n, ok := nodeByLabel(be, "output"); !ok || !n.checked {
		t.Error("output should be checked after toggling it on")
	}
	if !keyExistsInNode(be.node, []string{"output"}) {
		t.Error("output key missing from canonical node after toggle")
	}
	assertTreeMatchesNode(t, be)
}

// TestSOT_ToggleSequenceConsistency drives a sequence of toggles (including a
// deep nested leaf) and asserts the invariant holds after every step.
func TestSOT_ToggleSequenceConsistency(t *testing.T) {
	be := newStructEdit(t, "cfg:\n")
	be = expandAll(be)

	for _, label := range []string{"output", "regex", "path", "log-file"} {
		be = cursorToLabel(be, label)
		be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
		be = expandAll(be)
		assertTreeMatchesNode(t, be)
	}

	// Toggle two back off; invariant must still hold.
	for _, label := range []string{"output", "regex"} {
		be = cursorToLabel(be, label)
		// ctrl+d on a filled leaf confirms; drive the pending removal directly.
		idx := -1
		for i, n := range be.tree.nodes {
			if n.kind == treeNodeField && n.label == label {
				idx = i
				break
			}
		}
		be, _ = be.Update(pendingRemoveMsg{nodeIdx: idx})
		be = expandAll(be)
		assertTreeMatchesNode(t, be)
	}
}

// assertCollTreeMatchesNode is the collection invariant: every entry's label and
// every child's checked flag mirror the canonical node.
func assertCollTreeMatchesNode(t *testing.T, be blockEditState) {
	t.Helper()
	isMap := be.coll.isMap
	nodes := be.tree.nodes
	for i := 0; i < len(nodes); i++ {
		if nodes[i].kind != treeNodeSeqItem {
			continue
		}
		seqIdx := nodes[i].seqIdx
		if want := entryLabel(be.node, isMap, seqIdx); nodes[i].label != want {
			t.Errorf("entry %d label %q != node label %q", seqIdx, nodes[i].label, want)
		}
		entry := entryValueNode(be.node, isMap, seqIdx)
		for j := i + 1; j < len(nodes) && nodes[j].depth > 0; j++ {
			c := nodes[j]
			if c.kind != treeNodeField || !c.isLeaf || len(c.yamlPath) < 2 {
				continue
			}
			want := keyExistsInNode(entry, c.yamlPath[1:])
			if c.checked != want {
				t.Errorf("entry %d child %v checked=%v want %v", seqIdx, c.yamlPath, c.checked, want)
			}
		}
	}
}

// TestSOT_CollectionToggleWhileBufferInvalid is the Classe A regression for the
// collection navigator: toggling a field in an entry while its buffer is invalid
// still leaves tree and node in agreement and produces valid YAML.
func TestSOT_CollectionToggleWhileBufferInvalid(t *testing.T) {
	be := newBlockEdit(Config{}, blockSpec{
		key: "categories", defs: catDefs(), kind: schema.KindList,
		content: `categories:
  - name: a
`,
	}, 120, 40)
	be = expandAll(be)

	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: [bad\n")
	if validateSnippetText(be.yamlEditor.Value()) == nil {
		t.Fatal("test setup: expected invalid YAML")
	}

	be.active = blockEditPanelTree
	be = cursorToLabel(be, "path")
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})

	entry := entryValueNode(be.node, false, 0)
	if !keyExistsInNode(entry, []string{"source", "path"}) {
		t.Errorf("source.path not added to entry 0:\n%s", be.yamlEditor.Value())
	}
	if err := validateSnippetText(be.yamlEditor.Value()); err != nil {
		t.Errorf("buffer invalid after toggle: %v", err)
	}
	assertCollTreeMatchesNode(t, be)
}

// TestSOT_CollectionAddDeleteConsistency drives add and delete and asserts the
// node is the single source of truth for the entry list and labels (Classe B/D).
func TestSOT_CollectionAddDeleteConsistency(t *testing.T) {
	be := newBlockEdit(Config{NoDeleteConfirm: true}, blockSpec{
		key: "categories", defs: catDefs(), kind: schema.KindList,
		content: `categories:
  - name: a
  - name: b
`,
	}, 120, 40)

	if got := entryCount(be.node, false); got != 2 {
		t.Fatalf("initial entry count = %d, want 2", got)
	}
	assertCollTreeMatchesNode(t, be)

	// Add an entry via the [+ add new] row.
	be = cursorToAddNew(be)
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
	if got := entryCount(be.node, false); got != 3 {
		t.Fatalf("after add, entry count = %d, want 3", got)
	}
	assertCollTreeMatchesNode(t, be)

	// Delete the first entry.
	be = be.performEntryDelete(0)
	if got := entryCount(be.node, false); got != 2 {
		t.Fatalf("after delete, entry count = %d, want 2", got)
	}
	if l := entryLabel(be.node, false, 0); l != "b" {
		t.Errorf("after deleting entry 0, first label = %q, want b", l)
	}
	assertCollTreeMatchesNode(t, be)
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
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "output" {
			idx = i
			break
		}
	}
	be, _ = be.Update(pendingRemoveMsg{nodeIdx: idx})

	if strings.Contains(be.yamlEditor.Value(), "output:") {
		t.Error("output key still present after removal")
	}
	if !strings.Contains(be.yamlEditor.Value(), "# log destination") {
		t.Errorf("leading comment lost after editing sibling:\n%s", be.yamlEditor.Value())
	}
}

// TestSOT_ToggleRoundTripNode toggling a deep leaf on then off returns the node
// to an empty mapping (the nested ancestors are pruned), proving structural
// round-trip stability.
func TestSOT_ToggleRoundTripNode(t *testing.T) {
	be := newStructEdit(t, "cfg:\n")
	be = expandAll(be)

	be = cursorToLabel(be, "regex")
	be, _ = be.updateTreePanel(tea.KeyMsg{Type: tea.KeyEnter})
	if !keyExistsInNode(be.node, []string{"source", "filter", "regex"}) {
		t.Fatalf("regex not created:\n%s", be.yamlEditor.Value())
	}

	idx := -1
	for i, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.label == "regex" {
			idx = i
			break
		}
	}
	be, _ = be.Update(pendingRemoveMsg{nodeIdx: idx})

	if be.node.Kind == yaml.MappingNode && len(be.node.Content) != 0 {
		t.Errorf("node not empty after round-trip:\n%s", be.yamlEditor.Value())
	}
	assertTreeMatchesNode(t, be)
}
