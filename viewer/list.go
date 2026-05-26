package viewer

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

var (
	styleListActive = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	styleListDim    = lipgloss.NewStyle().Foreground(theme.Dim)
	styleListHeader = lipgloss.NewStyle().Bold(true).Foreground(theme.AccentBright)
)

type listMode int

const (
	modeFields listMode = iota
	modePresets
)

// listModel provides two-level navigation: fields → presets.
// In modeFields the user browses top-level field names.
// Pressing Enter (DrillIn) switches to modePresets for that field.
// Pressing Esc (Back) returns to modeFields.
type listModel struct {
	fields         []string
	presetsByField map[string][]string

	mode         listMode
	fieldCursor  int
	presetCursor int
	activeField  string // field whose presets are being shown

	scroll int
	height int
	width  int
}

func newListModel(fields []string, presetsByField map[string][]string) listModel {
	l := listModel{
		fields:         fields,
		presetsByField: presetsByField,
		mode:           modeFields,
	}
	if len(fields) > 0 {
		l.activeField = fields[0]
	}
	return l
}

// Selected returns the (field, preset) pair for the right-panel preview.
// In modeFields the first preset of the hovered field is used as a preview.
// In modePresets the actually-highlighted preset is returned.
func (l *listModel) Selected() (string, string) {
	if l.mode == modeFields {
		if l.fieldCursor < 0 || l.fieldCursor >= len(l.fields) {
			return "", ""
		}
		field := l.fields[l.fieldCursor]
		presets := l.presetsByField[field]
		if len(presets) > 0 {
			return field, presets[0]
		}
		return field, ""
	}
	// modePresets
	presets := l.presetsByField[l.activeField]
	if l.presetCursor >= 0 && l.presetCursor < len(presets) {
		return l.activeField, presets[l.presetCursor]
	}
	return l.activeField, ""
}

// Mode returns the current navigation level.
func (l *listModel) Mode() listMode { return l.mode }

func (l *listModel) MoveDown() {
	if l.mode == modeFields {
		if l.fieldCursor < len(l.fields)-1 {
			l.fieldCursor++
			l.activeField = l.fields[l.fieldCursor]
			l.ensureCursorVisible()
		}
	} else {
		presets := l.presetsByField[l.activeField]
		if l.presetCursor < len(presets)-1 {
			l.presetCursor++
			l.ensureCursorVisible()
		}
	}
}

func (l *listModel) MoveUp() {
	if l.mode == modeFields {
		if l.fieldCursor > 0 {
			l.fieldCursor--
			l.activeField = l.fields[l.fieldCursor]
			l.ensureCursorVisible()
		}
	} else {
		if l.presetCursor > 0 {
			l.presetCursor--
			l.ensureCursorVisible()
		}
	}
}

// DrillIn switches from modeFields to modePresets for the hovered field.
func (l *listModel) DrillIn() {
	if l.mode != modeFields || l.fieldCursor >= len(l.fields) {
		return
	}
	l.activeField = l.fields[l.fieldCursor]
	l.presetCursor = 0
	l.mode = modePresets
	l.scroll = 0
}

// Back returns from modePresets to modeFields.
func (l *listModel) Back() {
	if l.mode == modePresets {
		l.mode = modeFields
		l.scroll = 0
		l.ensureCursorVisible()
	}
}

// JumpFieldForward / JumpFieldBackward move the field cursor by one page (modeFields only).
func (l *listModel) JumpFieldForward() {
	if l.mode != modeFields {
		return
	}
	step := l.height
	if step < 1 {
		step = 1
	}
	l.fieldCursor += step
	if l.fieldCursor >= len(l.fields) {
		l.fieldCursor = len(l.fields) - 1
	}
	l.activeField = l.fields[l.fieldCursor]
	l.ensureCursorVisible()
}

func (l *listModel) JumpFieldBackward() {
	if l.mode != modeFields {
		return
	}
	step := l.height
	if step < 1 {
		step = 1
	}
	l.fieldCursor -= step
	if l.fieldCursor < 0 {
		l.fieldCursor = 0
	}
	l.activeField = l.fields[l.fieldCursor]
	l.ensureCursorVisible()
}

func (l *listModel) SetSize(w, h int) {
	l.width = w
	l.height = h
	l.ensureCursorVisible()
}

func (l *listModel) ensureCursorVisible() {
	cur := l.fieldCursor
	if l.mode == modePresets {
		cur = l.presetCursor
	}
	if cur < l.scroll {
		l.scroll = cur
	}
	if l.height > 0 && cur >= l.scroll+l.height {
		l.scroll = cur - l.height + 1
	}
	if l.scroll < 0 {
		l.scroll = 0
	}
}

func (l *listModel) View() string {
	if l.height <= 0 {
		return ""
	}
	var lines []string
	if l.mode == modeFields {
		lines = l.viewFields()
	} else {
		lines = l.viewPresets()
	}
	for len(lines) < l.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (l *listModel) viewFields() []string {
	end := l.scroll + l.height
	if end > len(l.fields) {
		end = len(l.fields)
	}
	lines := make([]string, 0, end-l.scroll)
	for i := l.scroll; i < end; i++ {
		f := l.fields[i]
		if i == l.fieldCursor {
			lines = append(lines, styleListActive.Render("▸ "+f))
		} else {
			lines = append(lines, styleListDim.Render("  "+f))
		}
	}
	return lines
}

func (l *listModel) viewPresets() []string {
	header := styleListHeader.Render("← " + l.activeField)
	lines := []string{header, strings.Repeat("─", l.width)}

	presets := l.presetsByField[l.activeField]
	start := l.scroll
	end := start + l.height - 2 // -2 for header rows
	if end > len(presets) {
		end = len(presets)
	}
	for i := start; i < end; i++ {
		p := presets[i]
		if i == l.presetCursor {
			lines = append(lines, styleListActive.Render("  ▸ "+p))
		} else {
			lines = append(lines, styleListDim.Render("    "+p))
		}
	}
	return lines
}
