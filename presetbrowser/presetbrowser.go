// Package presetbrowser is the preset-picker overlay shared by the block
// editor and the root document-template picker.
package presetbrowser

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/theme"

	"github.com/lucasassuncao/yedit/keys"
)

// Model is the preset-picker overlay: a list of preset names on the left
// and a scrollable YAML preview on the right. It owns only browsing state;
// applying or appending the choice stays with the caller.
type Model struct {
	source presets.Source
	field  string
	names  []string
	cursor int

	PreviewFocus  bool // right panel has keyboard focus
	previewScroll int  // line scroll offset in the preview panel
}

// New builds the overlay for field, pre-selecting current when it
// is one of the presets. Reports false when source is nil or the field has no
// presets, and the picker simply does not open.
func New(source presets.Source, field, current string) (Model, bool) {
	if source == nil {
		return Model{}, false
	}
	names := source.ListPresets(field)
	if len(names) == 0 {
		return Model{}, false
	}
	pb := Model{source: source, field: field, names: names}
	for i, n := range names {
		if n == current {
			pb.cursor = i
			break
		}
	}
	return pb, true
}

// SelectedName returns the preset name under the cursor. The browser only opens
// with a non-empty names slice and Update clamps the cursor, so "" is a
// defensive fallback rather than an expected value.
func (pb Model) SelectedName() string {
	if pb.cursor < 0 || pb.cursor >= len(pb.names) {
		return ""
	}
	return pb.names[pb.cursor]
}

// Action is the outcome of a key handled by the preset browser.
type Action int

const (
	None      Action = iota
	Dismissed        // close the browser without choosing
	Applied          // replace the block content with the selection
	Appended         // append the selection's entries (collections only)
)

// Update handles one key press. allowAppend enables the "a" append action, for
// collection-nav blocks only, and the returned name carries the selected preset
// for Applied/Appended.
func (pb Model) Update(msg tea.KeyMsg, allowAppend bool) (Model, Action, string) {
	switch {
	case key.Matches(msg, keys.Esc):
		if pb.PreviewFocus {
			pb.PreviewFocus = false
			return pb, None, ""
		}
		return pb, Dismissed, ""
	case key.Matches(msg, keys.Tab):
		pb.PreviewFocus = !pb.PreviewFocus
	case key.Matches(msg, keys.Enter):
		if !pb.PreviewFocus {
			return pb, Applied, pb.SelectedName()
		}
	case key.Matches(msg, keys.AAppend):
		if !pb.PreviewFocus && allowAppend {
			return pb, Appended, pb.SelectedName()
		}
	case key.Matches(msg, keys.Up):
		if pb.PreviewFocus {
			if pb.previewScroll > 0 {
				pb.previewScroll--
			}
		} else if pb.cursor > 0 {
			pb.cursor--
			pb.previewScroll = 0
		}
	case key.Matches(msg, keys.Down):
		if pb.PreviewFocus {
			pb.previewScroll++
		} else if pb.cursor < len(pb.names)-1 {
			pb.cursor++
			pb.previewScroll = 0
		}
	}
	return pb, None, ""
}

// ListView renders the preset name list with the cursor row highlighted.
func (pb Model) ListView(th theme.Resolved) string {
	var sb strings.Builder
	for i, name := range pb.names {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i == pb.cursor {
			sb.WriteString(th.SelectedItem.Render("▶  " + name))
		} else {
			sb.WriteString(th.AvailableItem.Render("   " + name))
		}
	}
	return sb.String()
}

// PreviewView renders the selected preset's YAML clipped to height lines at the
// current scroll offset.
func (pb Model) PreviewView(height int) string {
	full := pb.PreviewYAML()
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

// PreviewYAML returns the raw YAML of the preset under the cursor, or an
// inline error comment when the source fails to resolve it.
func (pb Model) PreviewYAML() string {
	y, err := pb.source.PresetYAML(pb.field, pb.SelectedName())
	if err != nil {
		return fmt.Sprintf("# error: %v", err)
	}
	return y
}
