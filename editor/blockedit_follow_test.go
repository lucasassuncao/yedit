package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
	"github.com/lucasassuncao/yedit/schema"
)

// nestedStructSpec builds a KindObject blockSpec with a scalar field and an inline
// struct child, so nested paths can be exercised.
func nestedStructSpec(content string) blockSpec {
	return blockSpec{
		key:  "settings",
		kind: schema.KindObject,
		defs: []schema.FieldDef{
			{YAMLName: "retries", Kind: schema.KindPrimitive},
			{YAMLName: "source", Kind: schema.KindObject, Children: []schema.FieldDef{
				{YAMLName: "filter", Kind: schema.KindPrimitive},
			}},
		},
		content: content,
	}
}

// cursorToVisibleNode positions the tree cursor on the first visible node with
// the given label. Fails the test when the label is not visible.
func cursorToVisibleNode(t *testing.T, be blockEditState, label string) blockEditState {
	t.Helper()
	for vi, ni := range be.tree.visibleNodes() {
		if be.tree.nodes[ni].label == label {
			be.tree.cursor = vi
			return be
		}
	}
	t.Fatalf("node %q not visible in tree", label)
	return be
}

func TestMappingKeyLine(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	content := "settings:\n  retries: 3\n  source:\n    filter: \"x\"\n"
	v := valueNodeOfSnippet(content)
	must.NotNil(v)

	is.Equal(2, mappingKeyLine(v, []string{"retries"}), "top-level key")
	is.Equal(3, mappingKeyLine(v, []string{"source"}), "parent key")
	is.Equal(4, mappingKeyLine(v, []string{"source", "filter"}), "nested key")
	is.Equal(-1, mappingKeyLine(v, []string{"absent"}), "missing key")
	is.Equal(-1, mappingKeyLine(v, []string{"retries", "child"}), "descend into scalar")
	is.Equal(-1, mappingKeyLine(v, nil), "empty path")
	is.Equal(-1, mappingKeyLine(nil, []string{"x"}), "nil node")
}

func TestFollowTargetLine_StructBlock(t *testing.T) {
	is := assert.New(t)

	be := newBlockEdit(Config{}, nestedStructSpec("settings:\n  retries: 3\n  source:\n    filter: \"x\"\n"), 100, 40)

	be = cursorToVisibleNode(t, be, "retries")
	is.Equal(2, be.followTargetLine())

	be = cursorToVisibleNode(t, be, "source")
	is.Equal(3, be.followTargetLine())
}

func TestFollowTargetLine_AbsentFieldAndInvalidBuffer(t *testing.T) {
	is := assert.New(t)

	// Only retries present: "source" exists in the tree but not in the YAML.
	be := newBlockEdit(Config{}, nestedStructSpec("settings:\n  retries: 3\n"), 100, 40)
	be = cursorToVisibleNode(t, be, "source")
	is.Equal(-1, be.followTargetLine(), "absent field has no line")

	// Invalid buffer: lookup must fail without panicking.
	be = cursorToVisibleNode(t, be, "retries")
	be.yamlEditor.SetValue("settings:\n  retries: [broken\n")
	is.Equal(-1, be.followTargetLine(), "invalid buffer has no line")
}

func TestWithEditorCursorAt(t *testing.T) {
	is := assert.New(t)

	// SetValue leaves the textarea cursor at the end of the content, so the
	// first jump exercises the upward walk.
	be := newBlockEdit(Config{}, nestedStructSpec("settings:\n  retries: 3\n  source:\n    filter: \"x\"\n"), 100, 40)

	be = be.withEditorCursorAt(4)
	is.Equal(3, be.yamlEditor.Line(), "moved down to line 4 (0-based row 3)")

	be = be.withEditorCursorAt(2)
	is.Equal(1, be.yamlEditor.Line(), "moved back up to line 2")

	be = be.withEditorCursorAt(99)
	is.Equal(be.yamlEditor.LineCount()-1, be.yamlEditor.Line(), "clamped at last line")
}

func TestScrollLinesTo(t *testing.T) {
	is := assert.New(t)

	s := "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8"

	is.Equal(s, scrollLinesTo(s, 10, 4), "fits: unchanged")
	is.Equal("l1\nl2\nl3", scrollLinesTo(s, 3, 0), "no target: top window")
	is.Equal("l4\nl5\nl6", scrollLinesTo(s, 3, 5), "target centered")
	is.Equal("l6\nl7\nl8", scrollLinesTo(s, 3, 8), "target at end: clamped")
	is.Equal("", scrollLinesTo(s, 0, 5), "zero height")
}

// pressDown drives one down-arrow through the tree panel handler.
func pressDown(be blockEditState) blockEditState {
	be2, _ := be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyDown})
	return be2
}

func TestTreeNavigationFollowsInEditor(t *testing.T) {
	is := assert.New(t)

	be := newBlockEdit(Config{}, nestedStructSpec("settings:\n  retries: 3\n  source:\n    filter: \"x\"\n"), 100, 40)

	// Every down-arrow that lands on a field present in the YAML must leave
	// the editor cursor on that field's line (self-consistent with the lookup).
	for i := 0; i < 4; i++ {
		be = pressDown(be)
		if line := be.followTargetLine(); line > 0 {
			is.Equal(line-1, be.yamlEditor.Line(), "editor row follows tree selection")
			is.Equal(line, be.previewScroll, "preview scroll follows tree selection")
		}
	}
}

func TestFollowKeepsPositionOnAbsentField(t *testing.T) {
	is := assert.New(t)

	be := newBlockEdit(Config{}, nestedStructSpec("settings:\n  retries: 3\n"), 100, 40)
	be = cursorToVisibleNode(t, be, "retries")
	be = be.followTreeSelection()
	is.Equal(1, be.yamlEditor.Line(), "on retries (line 2, row 1)")

	// Move the cursor to the absent field and follow again: the editor
	// position must not change.
	be = cursorToVisibleNode(t, be, "source")
	prevRow := be.yamlEditor.Line()
	be = be.followTreeSelection()
	is.Equal(prevRow, be.yamlEditor.Line(), "absent field keeps editor position")
}

func TestToggleOnFollowsInsertedField(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	be := newBlockEdit(Config{}, nestedStructSpec("settings:\n  retries: 3\n"), 100, 40)
	be = cursorToVisibleNode(t, be, "source")

	// Expand the inline struct parent, then check its leaf via Enter.
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyRight})
	be = cursorToVisibleNode(t, be, "filter")
	be, _ = be.updateTreePanel(tea.KeyPressMsg{Code: tea.KeyEnter})

	line := be.followTargetLine()
	must.Greater(line, 0, "toggled-on field must exist in the buffer now")
	is.Equal(line-1, be.yamlEditor.Line(), "editor jumped to the inserted field")
}

// tallSeqSpec builds a KindList blockSpec whose single entry has many fields,
// so the entry view is taller than the editor panel.
func tallSeqSpec(fields []string) blockSpec {
	var defs []schema.FieldDef
	content := "items:\n  - "
	for i, f := range fields {
		defs = append(defs, schema.FieldDef{YAMLName: f, Kind: schema.KindPrimitive})
		if i > 0 {
			content += "    "
		}
		content += f + ": value-" + f + "\n"
	}
	return blockSpec{key: "items", kind: schema.KindList, defs: defs, content: content}
}

// TestBlurredEditorScrollsToFollowedField reproduces the collection-block bug:
// while the tree panel has focus the textarea is blurred, and moving its cursor
// does not reposition its viewport. The rendered panel must still show the
// followed field.
func TestBlurredEditorScrollsToFollowedField(t *testing.T) {
	is := assert.New(t)

	fields := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliett", "kilo", "lima", "mike", "november", "oscar", "papa", "quebec", "romeo", "sierra", "tango"}
	// Small terminal: the 21-line entry view exceeds the editor panel height.
	be := newBlockEdit(Config{}, tallSeqSpec(fields), 100, 16)

	// Expand the entry and follow its last field.
	be = cursorToFieldExpanded(be, "tango")
	be = be.followTreeSelection()

	is.False(be.yamlEditor.Focused(), "textarea must stay blurred while tree has focus")
	view := be.yamlEditor.View()
	is.Contains(view, "tango", "blurred editor viewport must scroll to the followed field")
}

func TestFollowTargetLine_CollectionEntry(t *testing.T) {
	is := assert.New(t)

	// Entry view buffer is "categories:\n  - name: a\n": name is on line 2.
	be := newBlockEdit(Config{}, seqSpec("categories:\n  - name: a\n  - name: b\n"), 100, 40)
	be = cursorToFieldExpanded(be, "name")
	is.Equal(2, be.followTargetLine(), "field inside current entry")

	// The seq item row itself points at the entry's first line.
	be = cursorToVisibleNode(t, be, "a")
	is.Equal(2, be.followTargetLine(), "entry row points at entry start")
}
