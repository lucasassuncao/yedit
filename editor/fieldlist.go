package editor

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
)

// fieldState pairs a discovered FieldDef with its toggle state in the overlay.
type fieldState struct {
	Def     schema.FieldDef
	Checked bool
}

// fieldListModel renders the left panel of the overlay: a scrollable,
// toggleable list of sub-field definitions for a given top-level key.
type fieldListModel struct {
	fields []fieldState
	cursor int
	offset int
	height int
}

// newFieldList builds the field list from discovered child defs and a set of
// pre-checked field names (typically taken from Config.PreCheckedFields).
func newFieldList(defs []schema.FieldDef, preChecked map[string]bool, h int) fieldListModel {
	states := make([]fieldState, len(defs))
	for i, d := range defs {
		states[i] = fieldState{Def: d, Checked: preChecked[d.YAMLName]}
	}
	return fieldListModel{fields: states, height: h}
}

func (fl fieldListModel) Fields() []fieldState { return fl.fields }

func (fl *fieldListModel) SetFields(fields []fieldState) { fl.fields = fields }

// Update returns the updated model and whether a toggle happened.
func (fl fieldListModel) Update(msg tea.KeyMsg) (fieldListModel, bool) {
	n := len(fl.fields)
	switch msg.String() {
	case "up", "k":
		if fl.cursor > 0 {
			fl.cursor--
			if fl.cursor < fl.offset {
				fl.offset = fl.cursor
			}
		}
	case "down", "j":
		if fl.cursor < n-1 {
			fl.cursor++
			if fl.cursor >= fl.offset+fl.height {
				fl.offset = fl.cursor - fl.height + 1
			}
		}
	case " ":
		if fl.cursor < n {
			fl.fields[fl.cursor].Checked = !fl.fields[fl.cursor].Checked
			return fl, true
		}
	}
	return fl, false
}

// ToggledField returns the field at the cursor; call after Update returns toggled=true.
func (fl fieldListModel) ToggledField() fieldState {
	if fl.cursor >= 0 && fl.cursor < len(fl.fields) {
		return fl.fields[fl.cursor]
	}
	return fieldState{}
}

func (fl fieldListModel) View() string {
	if len(fl.fields) == 0 {
		return availableItemStyle.Render("  (no sub-fields)")
	}

	var sb strings.Builder
	end := fl.offset + fl.height
	if end > len(fl.fields) {
		end = len(fl.fields)
	}

	for i := fl.offset; i < end; i++ {
		fs := fl.fields[i]
		mark := "○"
		if fs.Checked {
			mark = "●"
		}
		req := ""
		if fs.Def.Required {
			req = " *"
		}
		label := fmt.Sprintf("%s %-16s%s", mark, fs.Def.YAMLName, req)

		var line string
		switch {
		case i == fl.cursor:
			line = selectedItemStyle.Render("▶ " + label)
		case fs.Checked:
			line = existingItemStyle.Render("  " + label)
		default:
			line = availableItemStyle.Render("  " + label)
		}
		sb.WriteString(line + "\n")
	}

	rendered := end - fl.offset
	for i := rendered; i < fl.height; i++ {
		sb.WriteByte('\n')
	}

	return sb.String()
}
