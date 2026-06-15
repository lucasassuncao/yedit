package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/internal/yamlnode"
	"github.com/lucasassuncao/yedit/schema"
)

func parseValueNode(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(src), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return root.Content[0]
}

// ---------------------------------------------------------------------------
// nodeAt / setNodeAt - indexed focus paths into a live node tree
// ---------------------------------------------------------------------------

func TestNodeAt_indexedPath(t *testing.T) {
	// A self-referential filters tree: filters[0].any[0].regex
	src := `filters:
  - regex: outer
    any:
      - regex: inner
        glob: "*.go"
`
	doc := parseValueNode(t, src)                  // mapping {filters: seq}
	filters := yamlnode.ChildByKey(doc, "filters") // sequence

	// filters[0].any[0].regex == "inner"
	path := []pathSeg{segIdx(0), segKey("any"), segIdx(0), segKey("regex")}
	got := nodeAt(filters, path)
	if got == nil || got.Value != "inner" {
		t.Fatalf("nodeAt filters[0].any[0].regex = %v, want scalar \"inner\"", got)
	}
}

func TestSetNodeAt_preservesSiblingStructure(t *testing.T) {
	// Replacing a nested field must NOT collapse the sequence structure around it -
	// the exact class of bug that string splicing caused.
	src := `filters:
  - regex: ""
    any:
      - regex: ""
`
	doc := parseValueNode(t, src)
	filters := yamlnode.ChildByKey(doc, "filters")

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
// appendFieldFromSnippet - all fields from a multi-field snippet must be inserted
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
		t.Error("field 'recursive' missing after appendFieldFromSnippet - only first field was inserted")
	}
	if !keys["existing"] {
		t.Error("pre-existing field 'existing' was lost")
	}
}

// ---------------------------------------------------------------------------
// forceBlockStyle - flow sequences on leaf fields must be preserved
// ---------------------------------------------------------------------------

func TestForceBlockStyle_preservesFlowSequence(t *testing.T) {
	is := assert.New(t)
	input := `config:
  extensions: ["pdf", "txt"]
  name: test
`

	// withYAMLRoot is the main consumer of forceBlockStyle.
	result := withYAMLRoot(input, func(root *yaml.Node) bool {
		return true // no-op transform
	})

	// The result must NOT have converted [pdf, txt] to block style.
	is.NotContains(result, "\n  - pdf", "forceBlockStyle converted flow sequence to block style")
	is.NotContains(result, "\n  - txt", "forceBlockStyle converted flow sequence to block style")
}

// ---------------------------------------------------------------------------
// flowToBlockSeq - flow-style collection is transparently parsed
// ---------------------------------------------------------------------------

func TestFlowToBlockSeq_singleEntry(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	seqBase := "categories: [{name: images}]"
	entries := parseSeqEntries("categories", seqBase)
	must.Len(entries, 1, "expected 1 entry from flow-style input")
	is.Equal("images", entries[0].Label)
}

func TestFlowToBlockSeq_multipleEntries(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	seqBase := "categories: [{name: images}, {name: videos}]"
	entries := parseSeqEntries("categories", seqBase)
	must.Len(entries, 2, "expected 2 entries")
	is.Equal("images", entries[0].Label)
	is.Equal("videos", entries[1].Label)
}

// ---------------------------------------------------------------------------
// applyToggleAt - complex snippets (arrays, maps) must be appended correctly
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
		m := map[string]string{"tags": "tags:\n  - critical\n  - high\n"}
		ctx := toggleCtx{
			key:      "workers",
			snippets: func(s string) string { return m[s] },
		}
		return applyToggleAt(itemMapping, []string{}, "tags", true, ctx, false)
	})

	// The result should be valid YAML.
	var check any
	if err := yaml.Unmarshal([]byte(result), &check); err != nil {
		t.Errorf("resulting YAML is invalid: %v\nYAML:\n%s", err, result)
	}

	// Verify that "tags" is present with the array value.
	assert.Contains(t, result, "tags", "field 'tags' not found in result")
}

// ---------------------------------------------------------------------------
// applyToggleToSeqItem - toggling a child under an empty parent struct key
// ---------------------------------------------------------------------------

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
	assert.Contains(t, got, "path:", "toggling source.path did not add the field")
}
