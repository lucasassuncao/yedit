package editor

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"go.yaml.in/yaml/v3"

	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
)

// pathField is the PathField for this block, when it is one the picker can
// fill: a top-level scalar, or a top-level list of scalars.
func (be blockEditState) pathField() (PathField, bool) {
	pf, ok := be.cfg.PathFields[be.key]
	if !ok || len(be.focus) > 0 {
		return PathField{}, false
	}
	scalarList := be.kind == schema.KindList && len(be.childDefs) == 0
	return pf, scalarList || be.kind == schema.KindPrimitive
}

// docPath is where the document will be written, absolute: a relative path in
// it has to hold there, not where it was loaded from.
func (be blockEditState) docPath() string {
	p := be.cfg.Path
	if be.cfg.SavePath != "" {
		p = be.cfg.SavePath
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// openPathPicker pushes a file picker over the editor, starting in the
// document's folder; its answer comes back as a pathPickedMsg.
func (be blockEditState) openPathPicker() (blockEditState, tea.Cmd) {
	pf, ok := be.pathField()
	if !ok {
		return be, nil
	}
	onPick := func(abs string) tea.Cmd { return func() tea.Msg { return pathPickedMsg{abs: abs} } }
	fp := overlay.NewFilePicker("Pick a path for "+be.key, filepath.Dir(be.docPath()), onPick, be.theme).
		WithAllowedTypes(pf.Types...)
	if pf.Dirs {
		fp = fp.WithDirs()
	}
	be.sh = be.sh.Push(fp)
	be.mode = modePicking
	return be, fp.Init()
}

// updatePicking hands every message to the picker: its keys, and the
// directory listings it reads, which come back as overlay.TopMsg.
func (be blockEditState) updatePicking(msg tea.Msg) (blockEditState, tea.Cmd) {
	var cmd tea.Cmd
	be.sh, _, cmd = be.sh.Update(msg)
	return be, cmd
}

// pickedPath shapes abs with the field's Format and writes it in, or reports
// why it cannot be used and leaves the block as it was.
func (be blockEditState) pickedPath(abs string) blockEditState {
	value := abs
	if f := be.cfg.PathFields[be.key].Format; f != nil {
		v, err := f(abs, be.docPath())
		if err != nil {
			be.editorErr = editorError{kind: errBlocked, message: fmt.Sprintf("cannot use %s: %v", abs, err)}
			return be
		}
		value = v
	}
	return be.dispatch(PickPath{Value: value, Replace: be.entryUnderCursor()})
}

// entryUnderCursor is the list entry on the YAML cursor's line, or -1 when the
// cursor is elsewhere or the buffer and the node disagree on the entries.
func (be blockEditState) entryUnderCursor() int {
	if be.active != blockEditPanelYAML || be.kind != schema.KindList || be.node.Kind != yaml.SequenceNode {
		return -1
	}
	lines := strings.Split(be.yamlEditor.Value(), "\n")
	row := be.yamlEditor.Line()
	isItem := func(l string) bool { return strings.HasPrefix(strings.TrimLeft(l, " "), "- ") }
	if row >= len(lines) || !isItem(lines[row]) {
		return -1
	}
	items, idx := 0, -1
	for i, l := range lines {
		if isItem(l) {
			if i <= row {
				idx = items
			}
			items++
		}
	}
	if items != len(be.node.Content) {
		return -1
	}
	return idx
}

// applyPick writes value into the node: the scalar itself, or an entry of the
// list, replacing the one at replace or appended when replace is -1.
func (be blockEditState) applyPick(value string, replace int) blockEditState {
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if be.kind == schema.KindPrimitive {
		be.node = *scalar
	} else {
		if be.node.Kind != yaml.SequenceNode {
			be.node = yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		}
		// Copies of the state share the backing array: never write through it.
		content := slices.Clone(be.node.Content)
		if replace >= 0 && replace < len(content) {
			content[replace] = scalar
		} else {
			content = append(content, scalar)
		}
		be.node.Content = content
	}
	be.yamlEditor.SetValue(yamledit.NodeToContent(be.key, &be.node))
	return be
}
