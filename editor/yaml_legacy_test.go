package editor

import (
	"strings"
	"testing"
)

func TestParseMapEntries(t *testing.T) {
	base := `portsAttributes:
  "3000":
    label: web
    onAutoForward: notify
  "8080":
    label: api
`
	entries := parseMapEntries("portsAttributes", base)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Label != "3000" || entries[1].Label != "8080" {
		t.Errorf("labels = [%q %q], want [3000 8080]", entries[0].Label, entries[1].Label)
	}
	if entries[0].Content != `  "3000":
    label: web
    onAutoForward: notify
` {
		t.Errorf("entry[0].Content = %q", entries[0].Content)
	}
	// Round-trip back to the full block.
	if got := seqEntriesToBase("portsAttributes", entries); got != base {
		t.Errorf("round-trip mismatch:\n got %q\nwant %q", got, base)
	}
}

func TestParseMapEntries_notMap(t *testing.T) {
	if entries := parseMapEntries("x", `y:
  a: 1
`); entries != nil {
		t.Errorf("wrong prefix should yield nil, got %v", entries)
	}
}

func TestApplyToggleToMapEntry_remove(t *testing.T) {
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
	if got != want {
		t.Errorf("after removing onAutoForward:\n got %q\nwant %q", got, want)
	}
}

func TestApplyToggleToMapEntry_add(t *testing.T) {
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
	if got != want {
		t.Errorf("after adding onAutoForward:\n got %q\nwant %q", got, want)
	}
}

func TestApplyToggleToMapEntry_addWithSnippet(t *testing.T) {
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
	if got != want {
		t.Errorf("snippet value not lifted:\n got %q\nwant %q", got, want)
	}
}

func TestApplyToggleToMapEntry_normalizesFlowToBlock(t *testing.T) {
	// The entry arrived in flow style (e.g. an emptied {} that regained fields).
	view := "portsAttributes:\n  lucas: {label: '', onAutoForward: notify}\n"
	node := treeNode{yamlPath: []string{"lucas", "protocol"}}
	ctx := toggleCtx{key: "portsAttributes", snippets: func(s string) string { return map[string]string{"protocol": "    protocol: http\n"}[s] }}
	got := applyToggleToMapEntry(ctx, node, true, view)
	if strings.Contains(got, "{") {
		t.Errorf("entry should be block style, got flow: %q", got)
	}
	if !strings.Contains(got, "\n    onAutoForward: notify\n") || !strings.Contains(got, "\n    protocol: http\n") {
		t.Errorf("fields should be one per line: %q", got)
	}
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
	// Map keys that contain colons (e.g. devcontainer feature keys) must round-trip.
	mapBase := `portsAttributes:
  "3000:80":
    label: web
  "8080":
    label: api
`
	entries := parseMapEntries("portsAttributes", mapBase)
	if len(entries) != 2 {
		t.Fatalf("expected 2 map entries, got %d", len(entries))
	}
	if entries[0].Label != "3000:80" {
		t.Errorf("entry[0].Label = %q, want %q", entries[0].Label, "3000:80")
	}
	if entries[1].Label != "8080" {
		t.Errorf("entry[1].Label = %q, want %q", entries[1].Label, "8080")
	}
}

// ---------------------------------------------------------------------------
// mapEntryKey - keys containing colons are preserved
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
