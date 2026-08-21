package editor

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/presetbrowser"
	"github.com/lucasassuncao/yedit/yamledit"
)

// openPresetPicker enters preset-browser mode, a no-op when this block has no
// presets.
func (be blockEditState) openPresetPicker() blockEditState {
	pb, ok := presetbrowser.New(be.cfg.BlockPresets, be.key, be.currentPreset)
	if !ok {
		return be
	}
	be.preset = pb
	be.mode = modePresetBrowser
	return be
}

func (be blockEditState) applyPreset(name, y string) blockEditState {
	be = be.saveUndo()
	be.currentPreset = name
	be.editorErr = editorError{}

	if be.isCollectionNav() {
		// yamledit.CollValueNode coerces unparseable YAML to an empty collection, so tell
		// the user instead of clearing the block without explanation.
		if strings.TrimSpace(y) != "" && yamledit.ValueNodeOfSnippet(y) == nil {
			be.editorErr = editorError{kind: errPreset, message: "Preset YAML is invalid; block reset to empty."}
		}
		be.node = *yamledit.CollValueNode(y, be.isMapNav())
		be.tree.Nodes = be.collectionTreeNodes()
		be.tree.Cursor = 0
		be.tree.Offset = 0
		return be.loadEntry(0)
	}

	be.yamlEditor.SetValue(y)
	if v := yamledit.BlockValueNodeOrNil(y); v != nil {
		be.node = *v
	} else {
		// Reset to an empty mapping and say so, rather than clearing the block
		// without explanation.
		be.node = yaml.Node{Kind: yaml.MappingNode}
		be.editorErr = editorError{kind: errPreset, message: "Preset YAML is invalid - block reset to empty."}
	}
	return be
}

func (be blockEditState) appendPreset(name, y string) blockEditState {
	if !be.isCollectionNav() {
		return be
	}
	presetNode := yamledit.CollValueNode(y, be.isMapNav())
	if yamledit.EntryCount(presetNode, be.isMapNav()) == 0 {
		return be
	}
	be = be.saveUndo()

	be = be.flushCurrentEntry()
	be.editorErr = editorError{} // appending overrides an in-progress invalid entry; don't block
	// A preset entry key that already exists would splice a duplicate mapping key
	// into the node, the same corruption flushCurrentEntry guards on rename, so
	// reject the whole append.
	if be.isMapNav() {
		existing := make(map[string]bool, len(be.node.Content)/2)
		for i := 0; i+1 < len(be.node.Content); i += 2 {
			existing[be.node.Content[i].Value] = true
		}
		for i := 0; i+1 < len(presetNode.Content); i += 2 {
			k := presetNode.Content[i].Value
			if existing[k] {
				be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("Preset entry %q already exists - append cancelled.", k)}
				return be
			}
			existing[k] = true
		}
	}
	// Indentation is irrelevant: entries are spliced as nodes and re-encoded.
	be.node.Content = append(be.node.Content, presetNode.Content...)

	be.tree.Nodes = be.collectionTreeNodes()
	be.tree.Offset = 0
	be.tree.Cursor = yamledit.EntryCount(&be.node, be.isMapNav()) - 1

	be = be.loadEntry(yamledit.EntryCount(&be.node, be.isMapNav()) - 1)
	be.currentPreset = name
	be.editorErr = editorError{}
	return be
}
