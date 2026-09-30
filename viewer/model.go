// Package viewer is a read-only TUI that browses the presets exposed by a
// presets.Source. Use it to ship a "show-examples" sub-command alongside an
// editor built on yedit/editor.
package viewer

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"

	"github.com/lucasassuncao/bezel/browser"
	"github.com/lucasassuncao/bezel/draw"
	"github.com/lucasassuncao/bezel/layout"
	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/shell"
	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/render"
)

// Model is the Bubble Tea root for the viewer TUI: a browser over the fields,
// and once one is opened, a browser over its presets.
type Model struct {
	src    presets.Source
	fields []string
	field  string // the field whose presets are listed; "" while browsing fields

	b        browser.Model
	keys     browser.Keys
	sh       shell.Shell
	th       theme.Resolved
	renderer *glamour.TermRenderer
}

// NewModel constructs the TUI from a presets.Source.
func NewModel(src presets.Source) Model {
	th := theme.Resolve(theme.ThemePlain, true)
	m := Model{src: src, fields: src.ListFields(), th: th}
	m.sh = shell.New(shell.Config{
		Layout: layout.Columns(layout.Fixed("fields", layout.Ratio(1, 3), layout.Min(30), layout.Max(60)), layout.Fill("body")),
		Theme:  th,
		Title:  "yedit", Subtitle: "presets",
	})
	// ← and → walk the two levels the way esc and enter do.
	m.keys = browser.DefaultKeys()
	m.keys.Enter = key.NewBinding(key.WithKeys("enter", "right"))
	m.keys.Esc = key.NewBinding(key.WithKeys("esc", "left"))
	m.b = browser.New(m.fieldItems(), "").WithKeys(m.keys)
	return m
}

// Init asks the terminal for its background so the theme can match it.
func (m *Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.sh, _, _ = m.sh.Update(msg)
		m.relayout()
		return m, nil
	case tea.BackgroundColorMsg:
		m.th = theme.Resolve(theme.ThemePlain, msg.IsDark())
		m.sh = m.sh.SetTheme(m.th)
		return m, nil
	case overlay.CloseMsg, overlay.PushMsg:
		var cmd tea.Cmd
		m.sh, _, cmd = m.sh.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// The shell runs help and quit and, while the help is open, takes every key.
		if sh, handled, cmd := m.sh.SetActions(m.actions()...).Update(msg); handled {
			m.sh = sh
			return m, cmd
		}
		var action browser.Action
		m.b, action = m.b.Update(msg)
		switch {
		case action == browser.Chosen && m.field == "":
			m.field = m.b.Selected().Label
			m.b = m.b.SetItems(m.presetItems(m.field))
		case action == browser.Dismissed && m.field != "":
			// Back to the fields, cursor on the one just left.
			current := m.field
			m.field = ""
			m.b = browser.New(m.fieldItems(), current).WithKeys(m.keys)
			m.relayout()
		}
		m.sh = m.sh.SetActions(m.actions()...)
		return m, nil
	}
	return m, nil
}

func (m *Model) relayout() {
	m.sh = m.sh.SetActions(m.actions()...)
	body := draw.InnerRect(m.sh.Rect("body"))
	m.b = m.b.SetPreviewHeight(body.H)
	if r, err := glamour.NewTermRenderer(glamour.WithStylePath("dark"), glamour.WithWordWrap(body.W)); err == nil {
		m.renderer = r
	}
}

// fieldItems lists the fields, each previewing its first preset.
func (m *Model) fieldItems() []browser.Item {
	items := make([]browser.Item, 0, len(m.fields))
	for _, f := range m.fields {
		items = append(items, browser.Item{Label: f, Detail: func() string {
			if ps := m.src.ListPresets(f); len(ps) > 0 {
				return m.rendered(f, ps[0])
			}
			return ""
		}})
	}
	return items
}

func (m *Model) presetItems(field string) []browser.Item {
	names := m.src.ListPresets(field)
	items := make([]browser.Item, 0, len(names))
	for _, n := range names {
		items = append(items, browser.Item{Label: n, Detail: func() string { return m.rendered(field, n) }})
	}
	return items
}

// rendered is a preset's YAML through glamour, or the error in its place.
func (m *Model) rendered(field, preset string) string {
	y, err := m.src.PresetYAML(field, preset)
	if err != nil {
		y = "# error: " + err.Error()
	}
	return render.YAMLFence(y, m.renderer)
}

// actions is the keys for where the user is: the fields, a field's presets,
// or the document pane. The browser handles every key but help and quit.
func (m *Model) actions() []shell.Action {
	out := []shell.Action{shell.Help(), shell.ChangePane(shell.DisplayOnly())}
	shown := func(label, desc string, keys ...string) shell.Action {
		return shell.Custom(label, desc, nil, shell.WithKey(keys...), shell.DisplayOnly())
	}
	switch {
	case m.b.PreviewFocus:
		return append(out, shell.Scroll(), shell.Quit(), shown("pgup/pgdn", "half-page", "pgup", "pgdown"), shown("esc", "back to list", "esc"))
	case m.field != "":
		return append(out, shell.Move(), shell.Quit(), shown("esc/←", "back to fields", "esc", "left"))
	}
	return append(out, shell.Move(), shell.Quit(), shown("enter/→", "open", "enter", "right"))
}

func (m *Model) View() tea.View {
	w, _ := m.sh.Size()
	if w == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}
	if len(m.fields) == 0 {
		v := tea.NewView("No presets available.")
		v.AltScreen = true
		return v
	}

	listTitle, bodyTitle := "Fields", "Preset"
	if m.field != "" {
		listTitle = "Presets · " + m.field
		bodyTitle = fmt.Sprintf("%s · %s", m.field, m.b.Selected().Label)
	}
	focus := "fields"
	if m.b.PreviewFocus {
		focus = "body"
	}
	content := m.sh.SetActions(m.actions()...).SetFocus(focus).View(map[string]shell.Pane{
		"fields": {Title: listTitle, Body: func(r layout.Rect) string { return m.b.ListView(m.th, r.H) }},
		"body":   {Title: bodyTitle, Body: func(r layout.Rect) string { return m.b.PreviewView(r.H) }},
	})
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// Run starts the viewer TUI as a blocking call.
func Run(src presets.Source) error {
	m := NewModel(src)
	p := tea.NewProgram(&m)
	_, err := p.Run()
	return err
}
