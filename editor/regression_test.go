package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

// ---------------------------------------------------------------------------
// nodeAt / setNodeAt — indexed focus paths into a live node tree
// ---------------------------------------------------------------------------

func parseValueNode(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return root.Content[0]
}

func TestNodeAt_indexedPath(t *testing.T) {
	// A self-referential filters tree: filters[0].any[0].regex
	src := `filters:
  - regex: outer
    any:
      - regex: inner
        glob: "*.go"
`
	doc := parseValueNode(t, src)         // mapping {filters: seq}
	filters := childByKey(doc, "filters") // sequence

	// filters[0].any[0].regex == "inner"
	path := []pathSeg{segIdx(0), segKey("any"), segIdx(0), segKey("regex")}
	got := nodeAt(filters, path)
	if got == nil || got.Value != "inner" {
		t.Fatalf("nodeAt filters[0].any[0].regex = %v, want scalar \"inner\"", got)
	}
}

func TestSetNodeAt_preservesSiblingStructure(t *testing.T) {
	// Replacing a nested field must NOT collapse the sequence structure around it —
	// the exact class of bug that string splicing caused.
	src := `filters:
  - regex: ""
    any:
      - regex: ""
`
	doc := parseValueNode(t, src)
	filters := childByKey(doc, "filters")

	// Replace filters[0].any[0] with a richer mapping.
	newItem := parseValueNode(t, "regex: deep\nglob: x\n")
	if !setNodeAt(filters, []pathSeg{segIdx(0), segKey("any"), segIdx(0)}, newItem) {
		t.Fatal("setNodeAt returned false")
	}

	// Re-encode the whole doc and confirm it is still a sequence-of-mappings, not
	// a mapping-of-mappings (the corruption symptom).
	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var check struct {
		Filters []struct {
			Regex string `yaml:"regex"`
			Any   []struct {
				Regex string `yaml:"regex"`
				Glob  string `yaml:"glob"`
			} `yaml:"any"`
		} `yaml:"filters"`
	}
	if err := yaml.Unmarshal(out, &check); err != nil {
		t.Fatalf("result is not the expected sequence structure: %v\n%s", err, out)
	}
	if len(check.Filters) != 1 || len(check.Filters[0].Any) != 1 {
		t.Fatalf("structure changed: %s", out)
	}
	if check.Filters[0].Any[0].Regex != "deep" || check.Filters[0].Any[0].Glob != "x" {
		t.Errorf("nested replace lost data: %s", out)
	}
}

// ---------------------------------------------------------------------------
// resyncTreeFromYAML — tolerant, non-authoritative visual projection
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

	// Unterminated flow sequence — definitely invalid YAML.
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
// appendFieldFromSnippet — all fields from a multi-field snippet must be inserted
// ---------------------------------------------------------------------------

func TestAppendFieldFromSnippet_multipleFields(t *testing.T) {
	// Simulate a FieldSnippet that contains two sub-fields.
	snippet := "  path: /foo\n  recursive: true\n"

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(`parent:
  existing: ok
`), &root); err != nil {
		t.Fatal(err)
	}
	valueNode := root.Content[0].Content[1] // the mapping under "parent"

	if !appendFieldFromSnippet(valueNode, "parent", snippet) {
		t.Fatal("appendFieldFromSnippet returned false")
	}

	// Both path and recursive must be present in the mapping.
	keys := make(map[string]bool)
	for i := 0; i+1 < len(valueNode.Content); i += 2 {
		keys[valueNode.Content[i].Value] = true
	}
	if !keys["path"] {
		t.Error("field 'path' missing after appendFieldFromSnippet")
	}
	if !keys["recursive"] {
		t.Error("field 'recursive' missing after appendFieldFromSnippet — only first field was inserted")
	}
	if !keys["existing"] {
		t.Error("pre-existing field 'existing' was lost")
	}
}

// ---------------------------------------------------------------------------
// flushCurrentEntry — missing key header sets errMsg, does not update entries
// ---------------------------------------------------------------------------

func TestFlushCurrentEntry_missingHeader_setsErrMsg(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
	be := newBlockEdit(Config{}, spec, 100, 40)

	originalEntry := be.entryYAML(0)

	// Simulate user deleting the "categories:" prefix.
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("  - name: alpha_edited\n") // no "categories:" header

	result := be.flushCurrentEntry()

	if result.errMsg == "" {
		t.Error("expected errMsg to be set when key header is missing")
	}
	if result.entryYAML(0) != originalEntry {
		t.Error("entry 0 was modified despite missing key header — silent data loss")
	}
}

func TestFlushCurrentEntry_validContent_clearsErrMsg(t *testing.T) {
	spec := seqSpec("categories:\n  - name: alpha\n")
	be := newBlockEdit(Config{}, spec, 100, 40)
	be.errMsg = "stale error from before"
	be.active = blockEditPanelYAML
	be.yamlEditor.SetValue("categories:\n  - name: alpha_edited\n")

	result := be.flushCurrentEntry()

	if result.errMsg != "" {
		t.Errorf("errMsg should be cleared on successful flush, got %q", result.errMsg)
	}
	if !strings.Contains(result.entryYAML(0), "alpha_edited") {
		t.Errorf("entry not updated: %q", result.entryYAML(0))
	}
}

// ---------------------------------------------------------------------------
// forceBlockStyle — flow sequences on leaf fields must be preserved
// ---------------------------------------------------------------------------

func TestForceBlockStyle_preservesFlowSequence(t *testing.T) {
	input := `config:
  extensions: ["pdf", "txt"]
  name: test
`

	// withYAMLRoot is the main consumer of forceBlockStyle.
	result := withYAMLRoot(input, func(root *yaml.Node) bool {
		return true // no-op transform
	})

	// The result must NOT have converted [pdf, txt] to block style.
	if strings.Contains(result, "\n  - pdf") || strings.Contains(result, "\n  - txt") {
		t.Errorf("forceBlockStyle converted flow sequence to block style:\n%s", result)
	}
}

// ---------------------------------------------------------------------------
// flowToBlockSeq — flow-style collection is transparently parsed
// ---------------------------------------------------------------------------

func TestFlowToBlockSeq_singleEntry(t *testing.T) {
	seqBase := "categories: [{name: images}]"
	entries := parseSeqEntries("categories", seqBase)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry from flow-style input, got %d", len(entries))
	}
	if entries[0].Label != "images" {
		t.Errorf("entry label = %q, want %q", entries[0].Label, "images")
	}
}

func TestFlowToBlockSeq_multipleEntries(t *testing.T) {
	seqBase := "categories: [{name: images}, {name: videos}]"
	entries := parseSeqEntries("categories", seqBase)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Label != "images" || entries[1].Label != "videos" {
		t.Errorf("labels = %q %q, want images videos", entries[0].Label, entries[1].Label)
	}
}

// ---------------------------------------------------------------------------
// mapEntryKey — keys containing colons are preserved
// ---------------------------------------------------------------------------

func TestMapEntryKey_withColon(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{`  "ghcr.io/features/git:1":`, "ghcr.io/features/git:1"},
		{`  "host:8080": {}`, "host:8080"},
		{`  "3000":`, "3000"},
		{`  plain:`, "plain"},
		{`  key: value`, "key"},
	}
	for _, c := range cases {
		got := mapEntryKey(c.line)
		if got != c.want {
			t.Errorf("mapEntryKey(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// applyToggleAt — complex snippets (arrays, maps) must be appended correctly
// ---------------------------------------------------------------------------

func TestApplyToggleAt_complexSnippetArray(t *testing.T) {
	// Simulates adding a field like "tags: []string" via toggle.
	// The snippet is a complex structure (array), not a simple scalar.
	// Verify that the resulting YAML is valid.
	snippet := `  - name: "item"
`
	result := withYAMLRoot("workers:\n"+snippet, func(root *yaml.Node) bool {
		mapping := root.Content[0]
		seqNode := mapping.Content[1]
		itemMapping := seqNode.Content[0]

		// Simulate adding a field with an array snippet.
		ctx := toggleCtx{
			key:      "workers",
			snippets: map[string]string{"tags": "tags:\n  - critical\n  - high\n"},
		}
		return applyToggleAt(itemMapping, []string{}, "tags", true, ctx, false)
	})

	// The result should be valid YAML.
	var check any
	if err := yaml.Unmarshal([]byte(result), &check); err != nil {
		t.Errorf("resulting YAML is invalid: %v\nYAML:\n%s", err, result)
	}

	// Verify that "tags" is present with the array value.
	if !strings.Contains(result, "tags") {
		t.Error("field 'tags' not found in result")
	}
}

// ---------------------------------------------------------------------------
// editorH — must never return a negative value
// ---------------------------------------------------------------------------

func TestEditorH_nonNegative(t *testing.T) {
	heights := []int{1, 2, 3, 5, 7, 10, 20}
	spec := seqSpec(`categories:
  - name: a
`)
	for _, h := range heights {
		be := newBlockEdit(Config{}, spec, 100, h)
		if got := be.editorH(); got < 0 {
			t.Errorf("editorH() = %d at terminal height %d — must be >= 0", got, h)
		}
	}
}

// ---------------------------------------------------------------------------
// collectionDeriveTree — labels and checks of every entry are derived from the
// canonical node, so editing one entry never contaminates another.
// ---------------------------------------------------------------------------

func TestCollectionDerive_perEntryLabels(t *testing.T) {
	spec := seqSpec(`categories:
  - name: alpha
  - name: beta
`)
	be := newBlockEdit(Config{}, spec, 100, 40)

	// Edit entry 1 through the parse gate (the real keystroke path splices the
	// parsed entry into be.node), then re-derive the tree.
	be.coll.current = 1
	kn, vn, ok := parseEntryFromView("categories:\n  - name: beta_edited\n", false)
	if !ok {
		t.Fatal("parseEntryFromView failed on valid entry text")
	}
	setEntry(be.node, false, 1, kn, vn)
	tm := be.collectionDeriveTree()

	labels := map[int]string{}
	for _, n := range tm.nodes {
		if n.kind == treeNodeSeqItem {
			labels[n.seqIdx] = n.label
		}
	}
	if labels[0] != "alpha" {
		t.Errorf("entry 0 label = %q, want alpha (must not be contaminated)", labels[0])
	}
	if labels[1] != "beta_edited" {
		t.Errorf("entry 1 label = %q, want beta_edited", labels[1])
	}
}

// TestToggleChildUnderEmptyParent reproduces the movelooper bug: a sequence item
// has an existing-but-empty nested struct key ("source:" with a null value).
// Toggling a child of that empty parent (source.path) must add it to the YAML.
// Before the fix, findOrCreateMappingChild returned the null scalar as-is and
// appendLeafToMapping silently no-op'd, so the toggle did nothing.
func TestToggleChildUnderEmptyParent(t *testing.T) {
	content := `categories:
  - name: "lucas"
    source:
`

	ctx := toggleCtx{
		key: "categories",
		childDefs: []schema.FieldDef{
			{YAMLName: "name", Kind: schema.KindPrimitive},
			{YAMLName: "source", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "path", Kind: schema.KindPrimitive},
			}},
		},
	}
	node := treeNode{
		kind:     treeNodeField,
		yamlPath: []string{"lucas", "source", "path"},
		label:    "path",
		depth:    2,
		def:      schema.FieldDef{YAMLName: "path", Kind: schema.KindPrimitive},
	}

	got := applyToggleToSeqItem(ctx, node, true, content)
	if !strings.Contains(got, "path:") {
		t.Errorf("toggling source.path did not add the field:\n%s", got)
	}
}

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
	be, _ = be.Update(pendingRemoveMsg{nodeIdx: beforeIdx})

	got := be.yamlEditor.Value()
	if strings.Contains(got, "before:") {
		t.Errorf("before block was not removed:\n%s", got)
	}
	if !strings.Contains(got, "after:") {
		t.Errorf("after block should remain:\n%s", got)
	}
}
