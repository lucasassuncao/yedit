package legend

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/yedit/theme"
)

// StatusLine wraps text in style and constrains it to width. Used for
// feedback lines (errors, transient messages) — not for the legend.
func StatusLine(width int, style lipgloss.Style, text string) string {
	return lipgloss.NewStyle().Width(width).Render(style.Render(text))
}

// RowKeyMap is implemented by KeyMaps whose legend is split into fixed,
// semantically grouped lines (e.g. navigation vs. document-mutating actions)
// instead of a single ShortHelp list wrapped purely by width.
type RowKeyMap interface {
	help.KeyMap
	Rows() [][]key.Binding
}

// Row renders one row of bindings as styled "key desc" items joined by
// the separator, wrapping onto additional lines only if the row itself still
// exceeds maxWidth. Uses lipgloss.Width for ANSI-aware measurement.
func Row(h help.Model, bindings []key.Binding, maxWidth int) []string {
	sep := h.Styles.ShortSeparator.Render(h.ShortSeparator)
	sepW := lipgloss.Width(sep)

	var items []string
	var widths []int
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		str := h.Styles.ShortKey.Render(b.Help().Key)
		if b.Help().Desc != "" {
			str += " " + h.Styles.ShortDesc.Render(b.Help().Desc)
		}
		items = append(items, str)
		widths = append(widths, lipgloss.Width(str))
	}
	if len(items) == 0 {
		return nil
	}

	var lines []string
	var lineItems []string
	lineW := 0

	for i, item := range items {
		cost := widths[i]
		if len(lineItems) > 0 {
			cost += sepW
		}
		if len(lineItems) > 0 && lineW+cost > maxWidth {
			lines = append(lines, strings.Join(lineItems, sep))
			lineItems = []string{item}
			lineW = widths[i]
		} else {
			lineItems = append(lineItems, item)
			lineW += cost
		}
	}
	if len(lineItems) > 0 {
		lines = append(lines, strings.Join(lineItems, sep))
	}
	return lines
}

// Render renders km's legend and returns the rendered string and the
// number of lines used. A KeyMap implementing RowKeyMap renders one line per
// group, in the group's fixed order, regardless of width (a group still
// wraps onto extra lines if it alone exceeds maxWidth). Any other KeyMap
// wraps its flat ShortHelp list onto new lines purely by width, as before.
func Render(h help.Model, km help.KeyMap, maxWidth int) (string, int) {
	var lines []string
	if rk, ok := km.(RowKeyMap); ok {
		for _, row := range rk.Rows() {
			lines = append(lines, Row(h, row, maxWidth)...)
		}
	} else {
		lines = Row(h, km.ShortHelp(), maxWidth)
	}

	if len(lines) == 0 {
		return "", 1
	}
	return strings.Join(lines, "\n"), len(lines)
}

// HelpLine renders the legend with left padding, filling the full
// terminal width. Wraps onto multiple lines if the bindings exceed width-1.
func HelpLine(width int, h help.Model, km help.KeyMap) string {
	content, _ := Render(h, km, width-1)
	return lipgloss.NewStyle().Width(width).Render(
		lipgloss.NewStyle().PaddingLeft(1).Render(content),
	)
}

// NewHelp builds a help.Model styled to match the editor theme.
func NewHelp(rt theme.Resolved) help.Model {
	h := help.New()
	h.ShowAll = false
	h.Styles.ShortKey = rt.HintKey
	h.Styles.ShortDesc = rt.HintDim
	h.Styles.ShortSeparator = rt.HintDim
	h.Styles.Ellipsis = rt.HintDim
	return h
}
