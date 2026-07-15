package editor

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// mappingKeyLine walks m (a MappingNode) along path and returns the 1-based
// buffer line of the final key, or -1 when any step does not resolve (nil or
// non-mapping node, missing key, empty path).
func mappingKeyLine(m *yaml.Node, path []string) int {
	cur := m
	for i, seg := range path {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return -1
		}
		next := -1
		for j := 0; j+1 < len(cur.Content); j += 2 {
			if cur.Content[j].Value == seg {
				if i == len(path)-1 {
					return cur.Content[j].Line
				}
				next = j + 1
				break
			}
		}
		if next < 0 {
			return -1
		}
		cur = cur.Content[next]
	}
	return -1
}

// followTargetLine returns the 1-based buffer line of the field selected in
// the tree, or -1 when it has no line: node without a path, field not present
// in the YAML, or a buffer that does not parse. A fresh parse is used because
// be.node's line info goes stale after tree toggles (the buffer is regenerated
// via nodeToContent).
func (be blockEditState) followTargetLine() int {
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return -1
	}
	node := be.tree.nodes[idx]
	if len(node.yamlPath) == 0 {
		return -1
	}
	v := valueNodeOfSnippet(be.yamlEditor.Value())
	if v == nil {
		return -1
	}
	if !be.isCollectionNav() {
		return mappingKeyLine(v, node.yamlPath)
	}
	return be.collectionTargetLine(v, node)
}

// collectionTargetLine resolves the follow line inside a collection buffer.
// The buffer shows a single entry (entryViewYAML): a one-item sequence or
// one-pair mapping under the block key. yamlPath[0] is the entry label, so
// the walk starts inside the entry's value mapping.
func (be blockEditState) collectionTargetLine(v *yaml.Node, node treeNode) int {
	wantKind, entryIdx := yaml.SequenceNode, 0
	if be.coll.isMap {
		wantKind, entryIdx = yaml.MappingNode, 1
	}
	if v.Kind != wantKind || len(v.Content) <= entryIdx {
		return -1
	}
	if node.kind == treeNodeSeqItem {
		return v.Content[0].Line
	}
	return mappingKeyLine(v.Content[entryIdx], node.yamlPath[1:])
}

// followTreeSelection moves the YAML editor cursor (and the Preview window)
// to the line of the tree node under the cursor. No-op when the node has no
// line in the current buffer, so the editor keeps its position on unchecked
// fields and invalid buffers.
func (be blockEditState) followTreeSelection() blockEditState {
	line := be.followTargetLine()
	if line < 1 {
		return be
	}
	be = be.withEditorCursorAt(line)
	be.previewScroll = line
	return be
}

// withEditorCursorAt moves the textarea cursor to the 1-based buffer line,
// column 0. bubbles v1 exposes no row setter, so the cursor walks with
// CursorUp/CursorDown; progress is tracked as (row, wrap offset) pairs so a
// soft-wrapped line cannot stall the loop, and a clamped end breaks out.
func (be blockEditState) withEditorCursorAt(line int) blockEditState {
	target := line - 1
	for be.yamlEditor.Line() < target {
		prevRow, prevOff := be.yamlEditor.Line(), be.yamlEditor.LineInfo().RowOffset
		be.yamlEditor.CursorDown()
		if be.yamlEditor.Line() == prevRow && be.yamlEditor.LineInfo().RowOffset == prevOff {
			break
		}
	}
	for be.yamlEditor.Line() > target {
		prevRow, prevOff := be.yamlEditor.Line(), be.yamlEditor.LineInfo().RowOffset
		be.yamlEditor.CursorUp()
		if be.yamlEditor.Line() == prevRow && be.yamlEditor.LineInfo().RowOffset == prevOff {
			break
		}
	}
	be.yamlEditor.SetCursor(0)
	if !be.yamlEditor.Focused() {
		// The textarea repositions its viewport only inside a focused Update;
		// direct cursor moves on a blurred editor leave the visible window
		// stale. Render once so the viewport (a shared pointer inside the
		// textarea) holds current content, then run one no-op Update under a
		// temporary focus so the viewport catches up with the cursor.
		_ = be.yamlEditor.View()
		be.yamlEditor.Focus()
		be.yamlEditor, _ = be.yamlEditor.Update(viewportSyncMsg{})
		be.yamlEditor.Blur()
	}
	return be
}

// viewportSyncMsg is a no-op message: the textarea's Update mutates nothing
// for unknown message types but always ends by repositioning its viewport.
type viewportSyncMsg struct{}

// scrollLinesTo returns a window of at most height lines from s that keeps
// targetLine (1-based) visible, roughly centered. targetLine < 1 yields the
// top window, matching clampLines.
func scrollLinesTo(s string, height, targetLine int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s
	}
	offset := targetLine - 1 - height/2
	if offset > len(lines)-height {
		offset = len(lines) - height
	}
	if offset < 0 {
		offset = 0
	}
	return strings.Join(lines[offset:offset+height], "\n")
}
