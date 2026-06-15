package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMapEntries(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	base := `portsAttributes:
  "3000":
    label: web
    onAutoForward: notify
  "8080":
    label: api
`
	entries := parseMapEntries("portsAttributes", base)
	must.Len(entries, 2, "expected 2 entries")
	is.Equal("3000", entries[0].Label)
	is.Equal("8080", entries[1].Label)
	is.Equal(`  "3000":
    label: web
    onAutoForward: notify
`, entries[0].Content)
	// Round-trip back to the full block.
	is.Equal(base, seqEntriesToBase("portsAttributes", entries), "round-trip mismatch")
}

func TestParseMapEntries_notMap(t *testing.T) {
	is := assert.New(t)
	entries := parseMapEntries("x", `y:
  a: 1
`)
	is.Nil(entries, "wrong prefix should yield nil")
}

func TestApplyToggleToMapEntry_remove(t *testing.T) {
	is := assert.New(t)
	view := `portsAttributes:
  "3000":
    label: web
    onAutoForward: notify
`
	node := treeNode{yamlPath: []string{"3000", "onAutoForward"}}
	got := applyToggleToMapEntry(toggleCtx{key: "portsAttributes"}, node, false, view)
	want := `portsAttributes:
  "3000":
    label: web
`
	is.Equal(want, got, "after removing onAutoForward")
}

func TestApplyToggleToMapEntry_add(t *testing.T) {
	is := assert.New(t)
	view := `portsAttributes:
  "3000":
    label: web
`
	node := treeNode{yamlPath: []string{"3000", "onAutoForward"}}
	got := applyToggleToMapEntry(toggleCtx{key: "portsAttributes"}, node, true, view)
	// An empty field renders as a null value ("onAutoForward:"), same as the
	// sequence navigator's appendLeafToMapping.
	want := `portsAttributes:
  "3000":
    label: web
    onAutoForward:
`
	is.Equal(want, got, "after adding onAutoForward")
}

func TestApplyToggleToMapEntry_addWithSnippet(t *testing.T) {
	is := assert.New(t)
	view := `portsAttributes:
  "3000":
    label: web
`
	node := treeNode{yamlPath: []string{"3000", "onAutoForward"}}
	m := map[string]string{"onAutoForward": "    onAutoForward: notify\n"}
	ctx := toggleCtx{
		key:      "portsAttributes",
		snippets: func(s string) string { return m[s] },
	}
	got := applyToggleToMapEntry(ctx, node, true, view)
	// The snippet's value must be lifted, not nested as map[onAutoForward:notify].
	want := `portsAttributes:
  "3000":
    label: web
    onAutoForward: notify
`
	is.Equal(want, got, "snippet value not lifted")
}

func TestApplyToggleToMapEntry_normalizesFlowToBlock(t *testing.T) {
	is := assert.New(t)
	// The entry arrived in flow style (e.g. an emptied {} that regained fields).
	view := "portsAttributes:\n  lucas: {label: '', onAutoForward: notify}\n"
	node := treeNode{yamlPath: []string{"lucas", "protocol"}}
	ctx := toggleCtx{key: "portsAttributes", snippets: func(s string) string { return map[string]string{"protocol": "    protocol: http\n"}[s] }}
	got := applyToggleToMapEntry(ctx, node, true, view)
	is.NotContains(got, "{", "entry should be block style, got flow")
	is.Contains(got, "\n    onAutoForward: notify\n", "fields should be one per line")
	is.Contains(got, "\n    protocol: http\n", "fields should be one per line")
}

// ---------------------------------------------------------------------------
// YAML edge cases: tab indentation, anchors in collection content
// ---------------------------------------------------------------------------

func TestParseSeqEntries_tabIndented_doesNotPanic(t *testing.T) {
	// Tab-indented YAML is invalid; parseSeqEntries must not panic.
	seqBase := "categories:\n\t- name: foo\n"
	entries := parseSeqEntries("categories", seqBase)
	// May return nil (invalid YAML) or an empty slice - both are acceptable.
	_ = entries
}

func TestParseSeqEntries_anchorInEntry_doesNotPanic(t *testing.T) {
	// Anchors inside entries are unusual but should not cause a panic.
	seqBase := "categories:\n  - &ref\n    name: images\n  - *ref\n"
	entries := parseSeqEntries("categories", seqBase)
	// At least one entry should be parsed if the YAML is syntactically valid for gopkg.
	_ = entries
}

func TestParseMapEntries_colonInKey(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	// Map keys that contain colons (e.g. devcontainer feature keys) must round-trip.
	mapBase := `portsAttributes:
  "3000:80":
    label: web
  "8080":
    label: api
`
	entries := parseMapEntries("portsAttributes", mapBase)
	must.Len(entries, 2, "expected 2 map entries")
	is.Equal("3000:80", entries[0].Label)
	is.Equal("8080", entries[1].Label)
}

// ---------------------------------------------------------------------------
// mapEntryKey - keys containing colons are preserved
// ---------------------------------------------------------------------------

func TestMapEntryKey_withColon(t *testing.T) {
	is := assert.New(t)
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
		is.Equal(c.want, mapEntryKey(c.line), "mapEntryKey(%q)", c.line)
	}
}
