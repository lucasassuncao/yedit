package editor

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/lucasassuncao/bezel/draw"
	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/theme"
)

// Tab is an application screen beside the editor's. Every message reaches
// it, on screen or not, so work it starts goes on while the document is
// edited; keys, mouse and pastes reach only the screen in front.
type Tab interface {
	Name() string
	Init() tea.Cmd
	Update(tea.Msg) (Tab, tea.Cmd)
	View() string // the whole screen, sized by the last WindowSizeMsg; the strip takes the end of its first row
	Busy() string // what quitting now would cut short, "" when nothing
}

// SaveMsg asks the editor to save the document as ctrl+s does, in front,
// through the same checks and question. SavedMsg answers every tab once the
// file is written; a refused or failed save answers nothing.
type SaveMsg struct{}

// SavedMsg tells the tabs the document is on disk at Path, as a SaveMsg asked.
type SavedMsg struct{ Path string }

// QuitMsg asks the editor to quit, as q does there: asking first when the
// document has unsaved changes or a tab is Busy.
type QuitMsg struct{}

// FrontMsg tells every screen which one is in front, "editor" or a tab's
// Name: at the start, and each time another comes in front.
type FrontMsg struct{ Name string }

const (
	tabKey        = "ctrl+t"
	editorTabName = "editor"
	tabSave       = "\x00tab" // the then of a save a tab asked for: no Action key starts with NUL
)

// tabbedModel is the editor with Config.Tabs beside it: the screen in front,
// with the strip at the end of its first row.
type tabbedModel struct {
	editor model
	tabs   []Tab
	active int // 0 is the editor, i is tabs[i-1]
	saver  int // the screen to bring back once a tab's save is written
	width  int
	theme  theme.Resolved
}

// newRoot is the program's model: the editor, wrapped only when it has tabs.
func newRoot(cfg Config) (tea.Model, error) {
	m, err := newModel(cfg)
	if err != nil || len(cfg.Tabs) == 0 {
		return m, err
	}
	t := tabbedModel{editor: m, tabs: cfg.Tabs, theme: m.theme}
	for i, tab := range cfg.Tabs {
		if cfg.FrontTab != "" && tab.Name() == cfg.FrontTab {
			t.active = i + 1
			break
		}
	}
	return t, nil
}

func (t tabbedModel) Init() tea.Cmd {
	cmds := []tea.Cmd{t.editor.Init(), t.announce()}
	for _, tab := range t.tabs {
		cmds = append(cmds, tab.Init())
	}
	return tea.Batch(cmds...)
}

// Update tells every screen when another comes in front, whatever brought it.
func (t tabbedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	front := t.active
	u, cmd := t.update(msg)
	if next := u.(tabbedModel); next.active != front {
		return next, tea.Batch(cmd, next.announce())
	}
	return u, cmd
}

func (t tabbedModel) announce() tea.Cmd {
	name := editorTabName
	if t.active > 0 {
		name = t.tabs[t.active-1].Name()
	}
	return func() tea.Msg { return FrontMsg{Name: name} }
}

func (t tabbedModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		return t.broadcast(msg)
	case tea.BackgroundColorMsg:
		t.theme = theme.Resolve(t.editor.cfg.Theme, msg.IsDark())
		return t.broadcast(msg)
	case tea.KeyPressMsg:
		if msg.String() == tabKey {
			t.active = (t.active + 1) % (len(t.tabs) + 1)
			return t, nil
		}
		return t.toFront(msg)
	case tea.KeyMsg, tea.MouseMsg, tea.PasteMsg, tea.PasteStartMsg, tea.PasteEndMsg:
		return t.toFront(msg)
	case SaveMsg:
		return t.save()
	case saveResultMsg:
		return t.saved(msg)
	case QuitMsg:
		t.active = 0
		t.editor.busy = t.busy()
		u, cmd := t.editor.quitOrConfirm()
		t.editor = u.(model)
		return t, cmd
	}
	return t.broadcast(msg)
}

// busy is what every Busy tab says quitting would cut short, one per line.
func (t tabbedModel) busy() string {
	var lines []string
	for _, tab := range t.tabs {
		if b := tab.Busy(); b != "" {
			lines = append(lines, b)
		}
	}
	return strings.Join(lines, "\n")
}

// save is a tab's SaveMsg: the editor in front, where the save asks and
// reports, unless there is nothing to write.
func (t tabbedModel) save() (tea.Model, tea.Cmd) {
	t.saver, t.active = t.active, 0
	if len(t.editor.blockEdits) > 0 {
		u, cmd := t.editor.showAlert("Close the block editor first",
			"It holds edits the file does not have yet: leave it (esc), then save from the tab again.", overlay.Info)
		t.editor = u.(model)
		return t, cmd
	}
	if !t.editor.needsSave() {
		t.active = t.saver
		return t.broadcast(SavedMsg{Path: t.editor.doc.Path()})
	}
	u, cmd := t.editor.save(tabSave)
	t.editor = u.(model)
	return t, cmd
}

// saved hands a save's outcome to the editor and, when a tab asked for it and
// it was written, brings that tab back and tells every tab.
func (t tabbedModel) saved(msg saveResultMsg) (tea.Model, tea.Cmd) {
	u, cmd := t.toEditor(msg)
	t = u.(tabbedModel)
	if msg.then != tabSave || msg.err != nil {
		return t, cmd
	}
	t.active = t.saver
	u, told := t.broadcast(SavedMsg{Path: t.editor.doc.Path()})
	return u, tea.Batch(cmd, told)
}

// toFront hands msg to the screen in front alone.
func (t tabbedModel) toFront(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.active == 0 {
		return t.toEditor(msg)
	}
	tabs := append([]Tab(nil), t.tabs...) // copies share the backing array
	var cmd tea.Cmd
	tabs[t.active-1], cmd = tabs[t.active-1].Update(msg)
	t.tabs = tabs
	return t, cmd
}

func (t tabbedModel) toEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	t.editor.busy = t.busy() // for the quit question, wherever the editor asks it
	u, cmd := t.editor.Update(msg)
	t.editor = u.(model)
	return t, cmd
}

// broadcast hands msg to the editor and to every tab.
func (t tabbedModel) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	u, cmd := t.toEditor(msg)
	t = u.(tabbedModel)
	cmds := []tea.Cmd{cmd}
	tabs := make([]Tab, len(t.tabs))
	for i, tab := range t.tabs {
		var c tea.Cmd
		tabs[i], c = tab.Update(msg)
		cmds = append(cmds, c)
	}
	t.tabs = tabs
	return t, tea.Batch(cmds...)
}

func (t tabbedModel) View() tea.View {
	body := t.editor.viewContent()
	if t.active > 0 {
		body = t.tabs[t.active-1].View()
	}
	head, rest, more := strings.Cut(body, "\n")
	content := t.header(head)
	if more {
		content += "\n" + rest
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// header is the screen's first row with the strip at its end: the tabs'
// names, the one in front marked, and the key that steps through them. Short
// of room, the hint goes first, then the end of the screen's own text.
func (t tabbedModel) header(head string) string {
	names := []string{editorTabName}
	for _, tab := range t.tabs {
		names = append(names, tab.Name())
	}
	tabsW := len(names) - 1
	for _, n := range names {
		tabsW += ansi.StringWidth(n) + 2
	}
	strip := draw.Tabs(tabsW, names, t.active, t.theme.Chrome)
	text := strings.TrimRight(head, " ")
	hint := t.theme.Chrome.Dim.Render(tabKey + " switch")
	if ansi.StringWidth(text)+2+tabsW+1+ansi.StringWidth(hint) <= t.width {
		strip += " " + hint
	}
	room := max(0, t.width-ansi.StringWidth(strip)-1)
	return draw.Fit(draw.Fit(text, room)+" "+strip, t.width)
}
