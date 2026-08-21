package yamledit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/yamlnode"
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
// NodeAt / SetNodeAt - indexed focus paths into a live node tree
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
	path := []PathSeg{SegIdx(0), SegKey("any"), SegIdx(0), SegKey("regex")}
	got := NodeAt(filters, path)
	if got == nil || got.Value != "inner" {
		t.Fatalf("NodeAt filters[0].any[0].regex = %v, want scalar \"inner\"", got)
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
	if err := SetNodeAt(filters, []PathSeg{SegIdx(0), SegKey("any"), SegIdx(0)}, newItem); err != nil {
		t.Fatalf("SetNodeAt: %v", err)
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
	// Simulate a FieldSnippet for a struct field "parent" with two sub-fields.
	// appendFieldFromSnippet should add (parentKey → {path, recursive}) to the
	// container mapping — NOT flatten path/recursive as siblings of existing.
	snippet := "  path: /foo\n  recursive: true\n"

	var root yaml.Node
	if err := yaml.Unmarshal([]byte("existing: ok\n"), &root); err != nil {
		t.Fatal(err)
	}
	// container is the mapping that will receive the new "parent" field.
	container := root.Content[0]

	if !appendFieldFromSnippet(container, "parent", snippet) {
		t.Fatal("appendFieldFromSnippet returned false")
	}

	// container must now have both "existing" and "parent" at the top level.
	topKeys := make(map[string]int)
	for i := 0; i+1 < len(container.Content); i += 2 {
		topKeys[container.Content[i].Value] = i
	}
	if _, ok := topKeys["existing"]; !ok {
		t.Error("pre-existing field 'existing' was lost")
	}
	parentIdx, ok := topKeys["parent"]
	if !ok {
		t.Fatal("field 'parent' missing after appendFieldFromSnippet")
	}

	// The value of "parent" must be a mapping containing path and recursive.
	parentVal := container.Content[parentIdx+1]
	subKeys := make(map[string]bool)
	for i := 0; i+1 < len(parentVal.Content); i += 2 {
		subKeys[parentVal.Content[i].Value] = true
	}
	if !subKeys["path"] {
		t.Error("sub-field 'path' missing from parent's value")
	}
	if !subKeys["recursive"] {
		t.Error("sub-field 'recursive' missing from parent's value")
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
// ApplyToggleAt - complex snippets (arrays, maps) must be appended correctly
// ---------------------------------------------------------------------------

func TestApplyToggleAt_complexSnippetArray(t *testing.T) {
	// Simulates adding a field like "tags: []string" via toggle.
	// The snippet is a complex structure (array), not a simple scalar.
	snippet := `  - name: "item"
`
	result := withYAMLRoot("workers:\n"+snippet, func(root *yaml.Node) bool {
		mapping := root.Content[0]
		seqNode := mapping.Content[1]
		itemMapping := seqNode.Content[0]

		// Simulate adding a field with an array snippet ("<field>: ..." form).
		m := map[string]string{"tags": "tags:\n  - critical\n  - high\n"}
		ctx := ToggleCtx{
			Snippets: func(s string) string { return m[s] },
		}
		return ApplyToggleAt(itemMapping, []string{}, "tags", true, ctx)
	})

	// The result should be valid YAML.
	var check any
	if err := yaml.Unmarshal([]byte(result), &check); err != nil {
		t.Errorf("resulting YAML is invalid: %v\nYAML:\n%s", err, result)
	}

	// The snippet's actual array values must be inserted, not just the key:
	// the key-only assertion masked the snippet being dropped entirely.
	assert.Contains(t, result, "- critical", "snippet array value missing from result:\n"+result)
	assert.Contains(t, result, "- high", "snippet array value missing from result:\n"+result)
}

// TestApplyToggleAt_snippetForms covers the documented FieldMeta.Snippet
// conventions: "<field>: value", a bare scalar, and an indented list. All of
// them were previously discarded (the field was inserted with an empty value).
func TestApplyToggleAt_snippetForms(t *testing.T) {
	cases := []struct {
		name    string
		field   string
		snippet string
		want    string
	}{
		{"field colon value", "enabled", "enabled: true", "enabled: true"},
		{"bare scalar", "port", "8080", "port: 8080"},
		{"indented list", "levels", "  - critical\n  - high\n", "- critical"},
		{"indented mapping children", "server", "  host: localhost\n  port: 8080\n", "host: localhost"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := withYAMLRoot("cfg:\n  name: x\n", func(root *yaml.Node) bool {
				mapping := root.Content[0].Content[1]
				m := map[string]string{tc.field: tc.snippet}
				ctx := ToggleCtx{Snippets: func(s string) string { return m[s] }}
				return ApplyToggleAt(mapping, []string{}, tc.field, true, ctx)
			})
			var check any
			if err := yaml.Unmarshal([]byte(result), &check); err != nil {
				t.Fatalf("resulting YAML is invalid: %v\nYAML:\n%s", err, result)
			}
			assert.Contains(t, result, tc.want, "snippet value missing from result:\n"+result)
		})
	}
}

func TestPruneEmptyContent(t *testing.T) {
	parse := func(src string) *yaml.Node {
		t.Helper()
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// unwrap DocumentNode → root MappingNode
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			return doc.Content[0]
		}
		return &doc
	}
	serialize := func(n *yaml.Node) string {
		t.Helper()
		out, err := yaml.Marshal(n)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(out)
	}

	t.Run("scalar empty string as mapping value removed", func(t *testing.T) {
		n := parse("key: \"\"")
		PruneEmptyContent(n)
		assert.Empty(t, n.Content)
	})

	t.Run("scalar null as mapping value removed", func(t *testing.T) {
		n := parse("key: null")
		PruneEmptyContent(n)
		assert.Empty(t, n.Content)
	})

	t.Run("empty mapping as mapping value removed", func(t *testing.T) {
		n := parse("key: {}")
		PruneEmptyContent(n)
		assert.Empty(t, n.Content)
	})

	t.Run("empty sequence as mapping value removed", func(t *testing.T) {
		n := parse("key: []")
		PruneEmptyContent(n)
		assert.Empty(t, n.Content)
	})

	t.Run("non-empty scalar mapping value kept", func(t *testing.T) {
		n := parse("key: value")
		PruneEmptyContent(n)
		assert.Len(t, n.Content, 2)
	})

	t.Run("empty scalar sequence item removed (gap 1)", func(t *testing.T) {
		n := parse("tags:\n  - \"\"\n  - hello\n  - \"\"")
		PruneEmptyContent(n)
		got := serialize(n)
		assert.Contains(t, got, "hello")
		assert.NotContains(t, got, `""`)
	})

	t.Run("null scalar sequence item removed (gap 1)", func(t *testing.T) {
		n := parse("tags:\n  - ~\n  - hello")
		PruneEmptyContent(n)
		got := serialize(n)
		assert.Contains(t, got, "hello")
		assert.NotContains(t, got, "null")
	})

	t.Run("all scalar sequence items empty collapses key (gap 1)", func(t *testing.T) {
		n := parse("tags:\n  - \"\"\n  - \"\"")
		PruneEmptyContent(n)
		assert.Empty(t, n.Content)
	})

	t.Run("empty nested sequence item removed (gap 2)", func(t *testing.T) {
		n := parse("matrix:\n  - []\n  - [a, b]")
		PruneEmptyContent(n)
		got := serialize(n)
		assert.NotContains(t, got, "[]")
		assert.Contains(t, got, "a")
	})

	t.Run("all nested sequence items empty collapses key (gap 2)", func(t *testing.T) {
		n := parse("matrix:\n  - []\n  - []")
		PruneEmptyContent(n)
		assert.Empty(t, n.Content)
	})

	t.Run("cascade: mapping whose children all become empty is removed", func(t *testing.T) {
		n := parse("outer:\n  inner:\n    field: \"\"")
		PruneEmptyContent(n)
		assert.Empty(t, n.Content)
	})

	t.Run("partial mapping: non-empty sibling keeps parent", func(t *testing.T) {
		n := parse("outer:\n  a: \"\"\n  b: kept")
		PruneEmptyContent(n)
		got := serialize(n)
		assert.Contains(t, got, "kept")
		assert.NotContains(t, got, `a:`)
	})

	t.Run("struct sequence: entry with all empty fields removed", func(t *testing.T) {
		n := parse("items:\n  - name: \"\"\n    value: \"\"\n  - name: alice\n    value: ok")
		PruneEmptyContent(n)
		got := serialize(n)
		assert.Contains(t, got, "alice")
		assert.NotContains(t, got, "name: \"\"")
	})

	t.Run("struct sequence: entry with one non-empty field survives", func(t *testing.T) {
		n := parse("items:\n  - name: alice\n    value: \"\"")
		PruneEmptyContent(n)
		got := serialize(n)
		assert.Contains(t, got, "alice")
		assert.NotContains(t, got, "value")
	})
}

// ---------------------------------------------------------------------------
// PruneEmptyMappings - null scalar values must be treated as empty
// ---------------------------------------------------------------------------

func TestPruneEmptyMappings_nullScalar(t *testing.T) {
	parse := func(src string) *yaml.Node {
		t.Helper()
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			return doc.Content[0]
		}
		return &doc
	}

	t.Run("null scalar mapping value removed", func(t *testing.T) {
		// "autoscaler:" with no value parses as a null scalar (Tag=="!!null").
		// Drilling into a new empty object field and back out produces exactly
		// this: the child editor serializes "autoscaler:\n" and SetNodeAt writes
		// the null scalar into editRoot. PruneEmptyMappings must remove it so
		// the parent YAML does not show a phantom "autoscaler:" line.
		n := parse("autoscaler:")
		PruneEmptyMappings(n)
		assert.Empty(t, n.Content, "null scalar value should be pruned")
	})

	t.Run("empty mapping value still removed", func(t *testing.T) {
		n := parse("autoscaler: {}")
		PruneEmptyMappings(n)
		assert.Empty(t, n.Content, "empty mapping value should still be pruned")
	})

	t.Run("non-null scalar not removed", func(t *testing.T) {
		n := parse("name: alice")
		PruneEmptyMappings(n)
		assert.Len(t, n.Content, 2, "non-null scalar value must not be pruned")
	})

	t.Run("empty string scalar not removed", func(t *testing.T) {
		// Empty string ("") is a legitimate placeholder for a just-added field
		// (toggle ON creates Tag="" Value=""); it must NOT be pruned.
		n := parse(`name: ""`)
		PruneEmptyMappings(n)
		assert.Len(t, n.Content, 2, "empty string scalar (Tag not !!null) must not be pruned")
	})

	t.Run("sibling with content preserved after null pruned", func(t *testing.T) {
		n := parse("autoscaler:\nname: alice\n")
		PruneEmptyMappings(n)
		got, err := yaml.Marshal(n)
		assert.NoError(t, err)
		assert.NotContains(t, string(got), "autoscaler", "phantom null key must be removed")
		assert.Contains(t, string(got), "alice", "sibling with content must survive")
	})
}
