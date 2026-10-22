package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
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

// A block with no presets of its own has nothing to browse.
func TestOpenPresetPickerWithoutPresetsIsNoop(t *testing.T) {
	stub := stubPresets{data: map[string]string{"other/x": "other: 1\n"}}
	be := newBlockEdit(Config{BlockPresets: stub}, structSpec(), 100, 40)

	be = be.openPresetPicker()
	assert.Equal(t, modeEditing, be.mode)

	stub.data["configuration/quiet"] = "configuration:\n  output: none\n"
	be = be.openPresetPicker()
	assert.Equal(t, modePresetBrowser, be.mode)
}

// applyPreset replaces the block with the preset, and the change is undoable.
func TestApplyPreset_replacesTheBlock(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)
	before := be.yamlEditor.Value()

	be = be.dispatch(ApplyPreset{Name: "quiet", Content: "configuration:\n  output: none\n"})

	is.Equal("configuration:\n  output: none\n", be.yamlEditor.Value())
	is.Equal("quiet", be.currentPreset)
	is.Equal(errNone, be.editorErr.kind)
	is.Equal(before, be.restoreUndo().yamlEditor.Value(), "the replaced content comes back on undo")
}

// A preset that does not parse resets the block and says so, instead of
// clearing it without explanation.
func TestApplyPreset_invalidYAMLResetsWithAnError(t *testing.T) {
	is := assert.New(t)
	be := newBlockEdit(Config{}, structSpec(), 100, 40)

	be = be.dispatch(ApplyPreset{Name: "broken", Content: "configuration:\n  output: [unclosed\n"})

	is.Equal(errPreset, be.editorErr.kind)
	is.Empty(be.node.Content, "the block is reset to an empty mapping")

	coll := newBlockEdit(Config{}, seqSpec("categories:\n  - name: a\n"), 100, 40)
	coll = coll.dispatch(ApplyPreset{Name: "broken", Content: "categories: [unclosed\n"})
	is.Equal(errPreset, coll.editorErr.kind, "a collection says so too")
	is.Zero(yamledit.EntryCount(&coll.node, false))
}

// appendPreset appends every preset entry after the existing ones and leaves the
// cursor on the last entry.
func TestAppendPreset_addsEntriesToExisting(t *testing.T) {
	is := assert.New(t)
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n",
	}}
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)

	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "extra")
	be = be.dispatch(AppendPreset{Name: "extra", Content: y})

	base := yamledit.NodeToContent("categories", &be.node)
	is.Contains(base, "name: existing", "entries missing original entry")
	is.Contains(base, "name: appended", "entries missing appended entry")

	seqCount := 0
	for _, n := range be.tree.Nodes {
		if n.Kind == fieldtree.KindSeqItem {
			seqCount++
		}
	}
	is.Equal(2, seqCount, "tree should have 2 seq items")
	is.Equal(1, be.tree.Cursor, "cursor should be on last entry")
	is.Contains(be.yamlEditor.Value(), "appended", "yamlEditor not showing appended entry")
	is.True(be.dirty, "dirty should be true after appendPreset")
}

// Append must work when the existing entries and the preset YAML are indented
// differently.
func TestAppendPreset_indentMismatch(t *testing.T) {
	is := assert.New(t)
	// existing uses 4-space, preset uses 2-space
	stub := stubPresets{data: map[string]string{
		"categories/extra": "categories:\n  - name: appended\n    enabled: null\n",
	}}
	spec := blockSpec{
		key:  "categories",
		kind: schema.KindList,
		defs: []schema.FieldDef{{YAMLName: "name", Kind: schema.KindPrimitive}},
		content: `categories:
    - name: existing
      enabled: true
`,
	}
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "extra")
	be = be.dispatch(AppendPreset{Name: "extra", Content: y})

	base2 := yamledit.NodeToContent("categories", &be.node)
	is.Contains(base2, "name: existing", "entries missing original entry")
	is.Contains(base2, "name: appended", "entries missing appended entry")
	is.Contains(be.yamlEditor.Value(), "appended", "yamlEditor not showing appended entry")
}

// A preset with multiple entries adds all of them.
func TestAppendPreset_multiEntryPreset(t *testing.T) {
	is := assert.New(t)
	stub := stubPresets{data: map[string]string{
		"categories/multi": "categories:\n  - name: alpha\n  - name: beta\n",
	}}
	spec := seqSpec("categories:\n  - name: existing\n")
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)
	be = be.openPresetPicker()
	y, _ := stub.PresetYAML("categories", "multi")
	be = be.dispatch(AppendPreset{Name: "multi", Content: y})

	seqCount := 0
	for _, n := range be.tree.Nodes {
		if n.Kind == fieldtree.KindSeqItem {
			seqCount++
		}
	}
	is.Equal(3, seqCount, "tree should have 3 seq items (1 existing + 2 from preset)")
	is.Equal(2, be.tree.Cursor, "cursor should be on last entry")
}

// Appending a preset whose entry key already exists must be rejected, not
// spliced in as a duplicate mapping key.
func TestAppendPreset_duplicateMapKeyRejected(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	stub := stubPresets{data: map[string]string{
		"moves/dup": "moves:\n  alpha:\n    path: /from-preset\n",
	}}
	spec := blockSpec{
		key:  "moves",
		kind: schema.KindDictionary,
		defs: []schema.FieldDef{
			{YAMLName: "path", Kind: schema.KindPrimitive},
		},
		content: "moves:\n  alpha:\n    path: /original\n",
	}
	be := newBlockEdit(Config{BlockPresets: stub}, spec, 100, 40)
	must.True(be.isMapNav(), "spec must build a map-nav block")

	y, _ := stub.PresetYAML("moves", "dup")
	be = be.dispatch(AppendPreset{Name: "dup", Content: y})

	is.Equal(errPreset, be.editorErr.kind, "colliding append must set a preset error")
	count := 0
	for i := 0; i+1 < len(be.node.Content); i += 2 {
		if be.node.Content[i].Value == "alpha" {
			count++
		}
	}
	is.Equal(1, count, "'alpha' must remain a single entry")
	base := yamledit.NodeToContent("moves", &be.node)
	is.Contains(base, "path: /original", "original entry value must be preserved")
}
