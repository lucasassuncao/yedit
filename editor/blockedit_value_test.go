package editor

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
)

var ctrlO = tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}

// valueMeta opts level into the picker; format declares OneOf but does not.
func valueMeta() Config {
	return Config{Metadata: MetadataFunc(func(block, fieldPath string) FieldMeta {
		switch fieldPath {
		case "level", "":
			return FieldMeta{OneOf: []string{"debug", "info", "warn"}, Default: "info", EnablePicker: true}
		case "format":
			return FieldMeta{OneOf: []string{"pretty", "json"}}
		}
		return FieldMeta{}
	})}
}

func loggingBlock(cfg Config, content string) blockEditState {
	defs := []schema.FieldDef{{YAMLName: "level", Kind: schema.KindPrimitive}, {YAMLName: "format", Kind: schema.KindPrimitive}}
	be := newBlockEdit(cfg, blockSpec{key: "logging", defs: defs, kind: schema.KindObject, content: content}, 100, 30)
	be.active = blockEditPanelTree
	return be
}

func TestValuePickerIsOfferedOnlyWhereTheMetadataEnablesIt(t *testing.T) {
	be := loggingBlock(valueMeta(), "logging:\n")
	require.Contains(t, legendKeys(cursorToLabel(be, "level")), "ctrl+o")
	require.NotContains(t, legendKeys(cursorToLabel(be, "format")), "ctrl+o", "OneOf alone keeps typing")

	typing := cursorToLabel(be, "level")
	typing.active = blockEditPanelYAML
	require.NotContains(t, legendKeys(typing), "ctrl+o")

	require.NotContains(t, legendKeys(loggingBlock(Config{}, "logging:\n")), "ctrl+o", "no metadata, no picker")
}

// The whole flow: the picker opens on the default of an absent field, the
// pick adds the field with that value, and it is one undo step.
func TestValuePickerAddsTheFieldWithThePick(t *testing.T) {
	be := cursorToLabel(loggingBlock(valueMeta(), "logging:\n  format: json\n"), "level")

	be = pathKey(t, be, ctrlO)
	require.Equal(t, modePicking, be.mode)
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyDown}) // info → warn
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEnter})

	require.Equal(t, modeEditing, be.mode, "picking closes the picker")
	require.False(t, be.sh.HasOverlay())
	require.Equal(t, "logging:\n  level: warn\n  format: json\n", be.yamlEditor.Value())
	require.True(t, be.dirty)

	be = be.dispatch(Undo{})
	require.Equal(t, "logging:\n  format: json\n", be.yamlEditor.Value(), "a pick is one undo step")
}

// The picker starts on the current value, so enter alone keeps it.
func TestValuePickerStartsOnTheCurrentValue(t *testing.T) {
	be := cursorToLabel(loggingBlock(valueMeta(), "logging:\n  level: debug\n"), "level")
	be = pathKey(t, be, ctrlO)
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyDown})
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, "logging:\n  level: info\n", be.yamlEditor.Value())
}

func TestValuePickerCancels(t *testing.T) {
	be := cursorToLabel(loggingBlock(valueMeta(), "logging:\n  level: debug\n"), "level")
	be = pathKey(t, be, ctrlO)
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, modeEditing, be.mode)
	require.False(t, be.sh.HasOverlay())
	require.Equal(t, "logging:\n  level: debug\n", be.yamlEditor.Value())
}

// In a collection, the pick lands in the entry being edited.
func TestValuePickerWritesIntoTheCurrentEntry(t *testing.T) {
	defs := []schema.FieldDef{{YAMLName: "name", Kind: schema.KindPrimitive}, {YAMLName: "level", Kind: schema.KindPrimitive}}
	content := "categories:\n  - name: a\n  - name: b\n"
	be := newBlockEdit(valueMeta(), blockSpec{key: "categories", defs: defs, kind: schema.KindList, content: content}, 100, 30)
	be.active = blockEditPanelTree
	be = be.dispatch(NavigateEntry{Idx: 1})
	for i := range be.tree.Nodes() {
		be.tree = be.tree.WithNodeMutated(i, func(n *fieldtree.Node) { n.Expanded = true })
	}
	be.tree = be.tree.CursorToSeqItem(1)
	be = cursorToLabel(be, "level")

	be = pathKey(t, be, ctrlO)
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, "categories:\n  - name: a\n  - name: b\n    level: info\n", yamledit.NodeToContent(be.key, &be.node))
}

// A block that is itself a scalar takes the pick as its value.
func TestValuePickerSetsALoneScalarBlock(t *testing.T) {
	be := newBlockEdit(valueMeta(), blockSpec{key: "level", kind: schema.KindPrimitive, content: "level: debug\n"}, 100, 30)
	require.Contains(t, legendKeys(be), "ctrl+o")
	be = pathKey(t, be, ctrlO)
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyDown})
	be = pathKey(t, be, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, "level: info\n", be.yamlEditor.Value())
}
