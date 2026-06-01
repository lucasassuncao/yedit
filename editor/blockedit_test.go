package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/schema"
)

// stubPresets implements presets.Source for tests.
type stubPresets struct {
	data map[string]string // key: "field/name" → YAML snippet
}

func (s stubPresets) ListFields() []string { return nil }
func (s stubPresets) ListPresets(field string) []string {
	prefix := field + "/"
	var out []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, strings.TrimPrefix(k, prefix))
		}
	}
	return out
}
func (s stubPresets) PresetYAML(field, name string) (string, error) {
	if y, ok := s.data[field+"/"+name]; ok {
		return y, nil
	}
	return "", fmt.Errorf("not found")
}

// seqSpec builds a KindList blockSpec with one child field ("name").
func seqSpec(key, content string) blockSpec {
	return blockSpec{
		key:  key,
		kind: schema.KindList,
		defs: []schema.FieldDef{
			{YAMLName: "name", Kind: schema.KindPrimitive},
		},
		content: content,
	}
}

// TestAppendPreset_addsEntriesToExisting verifies that appendPreset appends
// all entries from the preset after the existing entries and positions the
// cursor on the last entry.
func TestAppendPreset_addsEntriesToExisting(t *testing.T) {
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n",
	}}
	spec := seqSpec("categories", "categories:\n  - name: existing\n")
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)

	be = be.openPresetPicker()
	be = be.appendPreset("extra")

	if !strings.Contains(be.seqBase, "name: existing") {
		t.Errorf("seqBase missing original entry:\n%s", be.seqBase)
	}
	if !strings.Contains(be.seqBase, "name: appended") {
		t.Errorf("seqBase missing appended entry:\n%s", be.seqBase)
	}

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	if seqCount != 2 {
		t.Errorf("tree has %d seq items, want 2", seqCount)
	}

	if be.tree.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (last entry)", be.tree.cursor)
	}

	if !strings.Contains(be.yamlEditor.Value(), "appended") {
		t.Errorf("yamlEditor not showing appended entry:\n%s", be.yamlEditor.Value())
	}

	if !be.dirty {
		t.Error("dirty should be true after appendPreset")
	}
}

// TestAppendPreset_indentMismatch verifies that append works correctly when the
// existing seqBase and the preset YAML use different indentation levels.
func TestAppendPreset_indentMismatch(t *testing.T) {
	// existing uses 4-space, preset uses 2-space
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n    enabled: null\n",
	}}
	spec := blockSpec{
		key:     "categories",
		kind:    schema.KindList,
		defs:    []schema.FieldDef{{YAMLName: "name", Kind: schema.KindPrimitive}},
		content: "categories:\n    - name: existing\n      enabled: true\n",
	}
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	be = be.appendPreset("extra")

	if !strings.Contains(be.seqBase, "name: existing") {
		t.Errorf("seqBase missing original entry:\n%s", be.seqBase)
	}
	if !strings.Contains(be.seqBase, "name: appended") {
		t.Errorf("seqBase missing appended entry:\n%s", be.seqBase)
	}
	if !strings.Contains(be.yamlEditor.Value(), "appended") {
		t.Errorf("yamlEditor not showing appended entry:\n%s", be.yamlEditor.Value())
	}
}

// TestAppendPreset_multiEntryPreset verifies that a preset with multiple
// entries adds all of them.
func TestAppendPreset_multiEntryPreset(t *testing.T) {
	stub := stubPresets{data: map[string]string{
		"categories/multi": "categories:\n  - name: alpha\n  - name: beta\n",
	}}
	spec := seqSpec("categories", "categories:\n  - name: existing\n")
	be := newBlockEdit(Config{Presets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	be = be.appendPreset("multi")

	seqCount := 0
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqCount++
		}
	}
	if seqCount != 3 {
		t.Errorf("tree has %d seq items, want 3 (1 existing + 2 from preset)", seqCount)
	}
	if be.tree.cursor != 2 {
		t.Errorf("cursor = %d, want 2", be.tree.cursor)
	}
}
