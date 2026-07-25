// Package themebrowser provides a small, inline (not full-screen) terminal
// table listing yedit's built-in theme names next to their category.
package themebrowser

import (
	"fmt"
	"image/color"
	"strconv"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/yedit/theme"
)

const colID = "ID"

// maxVisibleRows caps the table's height before it scrolls, so the ~50
// built-in themes don't grow the box past a reasonable size.
const maxVisibleRows = 15

type browserModel struct {
	tbl    table.Model
	colors theme.Colors
}

func newBrowserModel(colors theme.Colors) *browserModel {
	const (
		colTheme    = "Theme"
		colCategory = "Category"
	)
	themeW := len(colTheme)
	catW := len(colCategory)

	var names [][2]string // theme name, category name
	for _, cat := range theme.Categories() {
		for _, name := range cat.Themes {
			names = append(names, [2]string{name, cat.Name})
			if len(name) > themeW {
				themeW = len(name)
			}
			if len(cat.Name) > catW {
				catW = len(cat.Name)
			}
		}
	}

	idW := len(strconv.Itoa(len(names)))
	if idW < len(colID) {
		idW = len(colID)
	}
	rows := make([]table.Row, len(names))
	for i, n := range names {
		rows[i] = table.Row{strconv.Itoa(i + 1), n[0], n[1]}
	}

	visibleRows := len(rows)
	if visibleRows > maxVisibleRows {
		visibleRows = maxVisibleRows
	}
	if visibleRows < 1 {
		visibleRows = 1
	}

	tbl := table.New(
		table.WithColumns([]table.Column{
			{Title: colID, Width: idW},
			{Title: colTheme, Width: themeW},
			{Title: colCategory, Width: catW},
		}),
		table.WithRows(rows),
		table.WithStyles(tableStyles(colors)),
		table.WithWidth((idW+2)+(themeW+2)+(catW+2)),
		table.WithHeight(visibleRows+1), // +1: WithHeight subtracts the 1-line header row internally
	)

	return &browserModel{tbl: tbl, colors: colors}
}

// tableStyles must not put Padding on Selected: table.renderRow renders each
// cell through Cell (which already pads) and only then wraps the whole,
// already-padded row string in Selected - adding padding there stacks a
// second layer on top and makes the cursor row wider than every other row.
// bubbles' own table.DefaultStyles follows the same rule.
//
// Selected does carry a Background, unlike every other "selected" style
// elsewhere in yedit (which use Foreground only): a text-color-only cue is
// easy to miss on the cursor row of a plain list, but on a wide table row
// it's the difference between an obvious highlight bar and looking like
// navigation silently does nothing.
func tableStyles(c theme.Colors) table.Styles {
	selected := lipgloss.NewStyle().Bold(true).
		Background(lipgloss.Color(c.SelectionColor)).
		Foreground(contrastText(c.SelectionColor))
	return table.Styles{
		Header: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(c.InactiveBorderColor)).Padding(0, 1),
		// Cell intentionally carries no Foreground: table.renderRow renders
		// each cell through Cell first (self-contained, with its own reset
		// code) and only wraps the *result* in Selected for the cursor row.
		// A colored Cell would reset Selected's Background midway through
		// the row, at every cell boundary, leaving only the first column
		// highlighted instead of the full-width bar the row is supposed to
		// have. Padding-only styling emits no ANSI codes to reset, so
		// Selected's Background/Foreground apply cleanly across the whole
		// row.
		Cell:     lipgloss.NewStyle().Padding(0, 1),
		Selected: selected,
	}
}

// contrastText picks black or white text to stay readable against bg,
// using perceived luminance. SelectionColor ranges from near-black-on-white
// themes to neon accents to pale creams across the ~50 built-in themes, so a
// single fixed text color would be unreadable on roughly half of them.
// Falls back to white for non-hex values (only theme.ThemePlain's ANSI
// codes, which are bright enough - "6" cyan, "212" pink - for white text).
func contrastText(bg string) color.Color {
	if len(bg) == 7 && bg[0] == '#' {
		r, rErr := strconv.ParseInt(bg[1:3], 16, 64)
		g, gErr := strconv.ParseInt(bg[3:5], 16, 64)
		b, bErr := strconv.ParseInt(bg[5:7], 16, 64)
		if rErr == nil && gErr == nil && bErr == nil {
			luminance := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
			if luminance > 140 {
				return lipgloss.Color("#000000")
			}
		}
	}
	return lipgloss.Color("#FFFFFF")
}

func (m *browserModel) Init() tea.Cmd { return nil }

// Update handles navigation directly via table.Model's MoveUp/MoveDown
// rather than forwarding to table.Model.Update - one less layer (and its
// focus-state gate) between a keypress and the cursor actually moving.
func (m *browserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "up":
		m.tbl.MoveUp(1)
	case "down":
		m.tbl.MoveDown(1)
	}
	return m, nil
}

// View renders inline - no AltScreen, so it doesn't take over the terminal
// just to show a small table. lipgloss.Border sizes itself to the table's
// own content, so there is no manual width/height math to keep in sync (the
// prior version's bugs all traced back to that).
func (m *browserModel) View() tea.View {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.colors.InactiveBorderColor))
	return tea.NewView(box.Render(m.tbl.View()))
}

// BrowseInTerminal renders an inline, scrollable table (↑/↓ to
// navigate, q/esc/ctrl+c to quit) of every built-in theme name next to its
// theme.Categories() category. It does not take over the full screen. An
// optional theme.Theme controls colors; zero value resolves to
// theme.ThemePlain.
func BrowseInTerminal(t ...theme.Theme) error {
	th := theme.Theme{}
	if len(t) > 0 {
		th = t[0]
	}
	m := newBrowserModel(theme.ResolveColors(th))
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run theme browser: %w", err)
	}
	return nil
}
