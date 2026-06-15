// Package viewer is a read-only TUI that browses the presets exposed by a
// presets.Source. Use it to ship a "show-examples" sub-command alongside an
// editor built on yedit/editor.
package viewer

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/theme"
)

type pane int

const (
	paneList pane = iota
	paneViewport
)

// Model is the Bubble Tea root for the viewer TUI.
type Model struct {
	src    presets.Source
	fields []string
	list   listModel

	width  int
	height int
	listW  int
	vpW    int

	active pane

	renderer     *glamour.TermRenderer
	renderedPane string
}

// NewModel constructs the TUI from a presets.Source.
func NewModel(src presets.Source) Model {
	fields := src.ListFields()
	presetsByField := make(map[string][]string, len(fields))
	for _, f := range fields {
		presetsByField[f] = src.ListPresets(f)
	}
	return Model{
		src:    src,
		fields: fields,
		list:   newListModel(fields, presetsByField),
		active: paneList,
	}
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		m.refreshRendered()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.active == paneList {
				m.active = paneViewport
			} else {
				m.active = paneList
			}
			return m, nil
		case "up", "k":
			if m.active == paneList {
				m.list.MoveUp()
				m.refreshRendered()
			}
			return m, nil
		case "down", "j":
			if m.active == paneList {
				m.list.MoveDown()
				m.refreshRendered()
			}
			return m, nil
		case "enter", "l", "right":
			if m.active == paneList {
				if m.list.Mode() == modeFields {
					m.list.DrillIn()
					m.refreshRendered()
				}
			}
			return m, nil
		case "esc", "h", "left":
			if m.active == paneList {
				if m.list.Mode() == modePresets {
					m.list.Back()
					m.refreshRendered()
				}
			}
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) relayout() {
	m.listW, m.vpW = theme.TwoColumnWidths(m.width)
	innerH := m.height - 5 // 1 header + 2 status + 2 panel borders
	if innerH < 3 {
		innerH = 3
	}
	m.list.SetSize(m.listW-2, innerH)

	if r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(m.vpW-2),
	); err == nil {
		m.renderer = r
	}
	m.renderedPane = ""
}

func (m *Model) refreshRendered() {
	field, preset := m.list.Selected()
	yaml := ""
	if preset != "" {
		if y, err := m.src.PresetYAML(field, preset); err == nil {
			yaml = y
		} else {
			yaml = "# error: " + err.Error()
		}
	}
	m.renderedPane = renderYAML(yaml, m.renderer)
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	if len(m.fields) == 0 {
		return "No presets available."
	}

	field, preset := m.list.Selected()
	rightTitle := "Preset"
	if field != "" && preset != "" {
		rightTitle = fmt.Sprintf("%s · %s", field, preset)
	}

	innerH := m.height - 5
	if innerH < 3 {
		innerH = 3
	}

	leftPanel := theme.RenderTitledPanel("Fields", theme.Size{W: m.listW, H: innerH + 2}, m.active == paneList, m.list.View())
	rightPanel := theme.RenderTitledPanel(rightTitle, theme.Size{W: m.vpW, H: innerH + 2}, m.active == paneViewport, m.renderedPane)

	hintText := "[↑/↓] navigate • [Enter/→] open • [Esc/←] back • [Tab] panel • [q] quit"
	if m.list.Mode() == modePresets {
		hintText = "[↑/↓] navigate • [Esc/←] back to fields • [Tab] panel • [q] quit"
	}
	header := theme.RenderHeader("yedit", "presets", "", m.width)
	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: "", Hint: theme.StatusBar.Render(hintText)})
}

// Run starts the viewer TUI as a blocking call.
func Run(src presets.Source) error {
	m := NewModel(src)
	p := tea.NewProgram(&m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
