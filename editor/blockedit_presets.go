package editor

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// presetBrowser is the preset-picker overlay shown inside a block editor: a
// list of preset names on the left and a scrollable YAML preview on the right.
// It owns only browsing state (cursor, focus, scroll); applying or appending
// the chosen preset stays in blockEditState.
type presetBrowser struct {
	source PresetSource
	field  string
	names  []string
	cursor int

	previewFocus  bool // right panel has keyboard focus
	previewScroll int  // line scroll offset in the preview panel
}

// newPresetBrowser builds the overlay for field, pre-selecting current when it
// is one of the available presets. Returns nil when source is nil or the field
// has no presets - the picker simply does not open.
func newPresetBrowser(source PresetSource, field, current string) *presetBrowser {
	if source == nil {
		return nil
	}
	names := source.ListPresets(field)
	if len(names) == 0 {
		return nil
	}
	pb := &presetBrowser{source: source, field: field, names: names}
	for i, n := range names {
		if n == current {
			pb.cursor = i
			break
		}
	}
	return pb
}

// presetAction is the outcome of a key handled by the preset browser.
type presetAction int

const (
	presetNone      presetAction = iota
	presetDismissed              // close the browser without choosing
	presetApplied                // replace the block content with the selection
	presetAppended               // append the selection's entries (collections only)
)

// Update handles one key press, mutating the browser in place. allowAppend
// enables the "a" append action (collection-nav blocks only). name carries the
// selected preset for presetApplied/presetAppended.
func (pb *presetBrowser) Update(key tea.KeyMsg, allowAppend bool) (action presetAction, name string) {
	switch key.String() {
	case "esc":
		if pb.previewFocus {
			pb.previewFocus = false
			return presetNone, ""
		}
		return presetDismissed, ""
	case "tab":
		pb.previewFocus = !pb.previewFocus
	case "enter":
		if !pb.previewFocus {
			return presetApplied, pb.names[pb.cursor]
		}
	case "a":
		if !pb.previewFocus && allowAppend {
			return presetAppended, pb.names[pb.cursor]
		}
	case "up", "k":
		if pb.previewFocus {
			if pb.previewScroll > 0 {
				pb.previewScroll--
			}
		} else if pb.cursor > 0 {
			pb.cursor--
			pb.previewScroll = 0
		}
	case "down", "j":
		if pb.previewFocus {
			pb.previewScroll++
		} else if pb.cursor < len(pb.names)-1 {
			pb.cursor++
			pb.previewScroll = 0
		}
	}
	return presetNone, ""
}

// listView renders the preset name list with the cursor row highlighted.
func (pb *presetBrowser) listView(th resolvedTheme) string {
	var sb strings.Builder
	for i, name := range pb.names {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i == pb.cursor {
			sb.WriteString(th.selectedItem.Render("▶  " + name))
		} else {
			sb.WriteString(th.availableItem.Render("   " + name))
		}
	}
	return sb.String()
}

// previewView renders the selected preset's YAML clipped to height lines,
// honouring the current scroll offset.
func (pb *presetBrowser) previewView(height int) string {
	full := pb.previewYAML()
	if full == "" {
		return ""
	}
	lines := strings.Split(full, "\n")
	if height < 1 {
		height = 1
	}
	maxScroll := len(lines) - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := pb.previewScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + height
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
}

// previewYAML returns the raw YAML of the preset under the cursor, or an
// inline error comment when the source fails to resolve it.
func (pb *presetBrowser) previewYAML() string {
	y, err := pb.source.PresetYAML(pb.field, pb.names[pb.cursor])
	if err != nil {
		return fmt.Sprintf("# error: %v", err)
	}
	return y
}

// openPresetPicker enters preset-browser mode if there are any presets for
// this block. It's a no-op when Presets is nil or the field has none.
func (be blockEditState) openPresetPicker() blockEditState {
	pb := newPresetBrowser(be.cfg.Presets, be.key, be.currentPreset)
	if pb == nil {
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
	be.dirty = true

	if be.isCollectionNav() {
		be.node = collValueNode(y, be.isMapNav())
		be.tree.nodes = be.collectionTreeNodes()
		be.tree.cursor = 0
		be.tree.offset = 0
		be = be.loadEntry(0)
		be.tree = be.resyncTreeFromYAML()
		return be
	}

	be.yamlEditor.SetValue(y)
	be.node = blockValueNode(y)
	be.tree = syncTreeCheckedFromNode(be.tree, be.node)
	return be
}

func (be blockEditState) appendPreset(name, y string) blockEditState {
	if !be.isCollectionNav() {
		return be
	}
	be = be.saveUndo()

	presetNode := collValueNode(y, be.isMapNav())
	if entryCount(presetNode, be.isMapNav()) == 0 {
		return be
	}

	be = be.flushCurrentEntry()
	be.editorErr = editorError{} // appending overrides an in-progress invalid entry; don't block
	// Indentation is irrelevant now: the entries are spliced as nodes and re-encoded.
	be.node.Content = append(be.node.Content, presetNode.Content...)

	be.tree.nodes = be.collectionTreeNodes()
	be.tree.offset = 0
	be.tree.cursor = entryCount(be.node, be.isMapNav()) - 1

	be = be.loadEntry(entryCount(be.node, be.isMapNav()) - 1)
	be.tree = be.resyncTreeFromYAML()
	be.currentPreset = name
	be.editorErr = editorError{}
	be.dirty = true
	return be
}
