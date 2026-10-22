package editor

import (
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	View() string // the whole screen, sized by the last WindowSizeMsg; the strip is drawn under its first row
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

// tabbedModel is the editor with Config.Tabs and the files opened from its
// entries beside it: the screen in front, with the strip under its first row
// while there is another screen to switch to.
type tabbedModel struct {
	editor        model
	tabs          []Tab
	files         []fileTab
	active        int // 0 is the editor, i is tabs[i-1], then the files in order
	saver         int // the screen to bring back once a tab's save is written
	width, height int
	dark          bool // the terminal's background, for the files opened later
	nextFile      int  // the id the next file tab gets
	strip         bool // the strip is drawn, and every screen is a row shorter for it
	theme         theme.Resolved
}

// stripRows is the height the strip takes from every screen.
const stripRows = 1

// newRoot is the program's model: the editor, wrapped only when it has tabs or
// can open files in tabs of their own.
func newRoot(cfg Config) (tea.Model, error) {
	m, err := newModel(cfg)
	if err != nil || (len(cfg.Tabs) == 0 && !opensFiles(cfg)) {
		return m, err
	}
	t := tabbedModel{editor: m, tabs: cfg.Tabs, theme: m.theme, dark: true}
	for i, tab := range cfg.Tabs {
		if cfg.FrontTab != "" && tab.Name() == cfg.FrontTab {
			t.active = i + 1
			break
		}
	}
	t, _ = t.restrip()
	return t, nil
}

func (t tabbedModel) Init() tea.Cmd {
	cmds := []tea.Cmd{t.editor.Init(), t.announce()}
	for _, tab := range t.tabs {
		cmds = append(cmds, tab.Init())
	}
	return tea.Batch(cmds...)
}

// Update tells every screen when another comes in front, whatever brought it,
// and makes room for the strip when a screen comes or goes.
func (t tabbedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	front := t.active
	u, cmd := t.update(msg)
	next, resized := u.(tabbedModel).restrip()
	cmd = tea.Batch(cmd, resized)
	if next.active != front {
		return next, tea.Batch(cmd, next.announce())
	}
	return next, cmd
}

// restrip draws the strip while there is more than one screen: every screen
// gives it a row, and the editors' legends name ctrl+t.
func (t tabbedModel) restrip() (tabbedModel, tea.Cmd) {
	on := len(t.names()) > 1
	if on == t.strip {
		return t, nil
	}
	t.strip = on
	t.editor.tabbed = on
	files := slices.Clone(t.files)
	for i := range files {
		files[i].m.tabbed = on
	}
	t.files = files
	if t.width == 0 {
		return t, nil // no size yet: the first WindowSizeMsg leaves room for the strip
	}
	u, cmd := t.everyScreen(t.screenSize())
	return u.(tabbedModel), cmd
}

// screenSize is the terminal less the strip: what every screen gets.
func (t tabbedModel) screenSize() tea.WindowSizeMsg {
	h := t.height
	if t.strip {
		h -= stripRows
	}
	return tea.WindowSizeMsg{Width: t.width, Height: h}
}

func (t tabbedModel) announce() tea.Cmd {
	name := t.names()[t.active]
	return func() tea.Msg { return FrontMsg{Name: name} }
}

// names is every screen's name, in the order ctrl+t steps through them.
func (t tabbedModel) names() []string {
	names := []string{editorTabName}
	for _, tab := range t.tabs {
		names = append(names, tab.Name())
	}
	for _, f := range t.files {
		names = append(names, f.name())
	}
	return names
}

func (t tabbedModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width, t.height = msg.Width, msg.Height
		return t.everyScreen(t.screenSize())
	case tea.BackgroundColorMsg:
		t.theme = theme.Resolve(t.editor.cfg.Theme, msg.IsDark())
		t.dark = msg.IsDark()
		return t.everyScreen(msg)
	case fileMsg:
		return t.fromFile(msg)
	case openFileMsg:
		return t.openFile(msg.cfg)
	case tea.KeyPressMsg:
		if msg.String() == tabKey {
			t.active = (t.active + 1) % len(t.names())
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

// busy is what every Busy tab and every edited file says quitting would cut
// short, one per line.
func (t tabbedModel) busy() string {
	var lines []string
	for _, tab := range t.tabs {
		if b := tab.Busy(); b != "" {
			lines = append(lines, b)
		}
	}
	return strings.Join(append(lines, t.unsavedFiles()...), "\n")
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
	if i := t.active - t.fileScreen(0); i >= 0 {
		return t.toFile(i, msg)
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

// everyScreen hands msg to the editor, every tab and every file tab. Other
// messages are broadcast without the files, whose own come back as fileMsg.
func (t tabbedModel) everyScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	u, cmd := t.broadcast(msg)
	t, files := u.(tabbedModel).toFiles(msg)
	return t, tea.Batch(cmd, files)
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
	if i := t.active - t.fileScreen(0); i >= 0 {
		body = t.files[i].m.viewContent()
	} else if t.active > 0 {
		body = t.tabs[t.active-1].View()
	}
	if t.strip {
		head, rest, _ := strings.Cut(body, "\n")
		body = head + "\n" + t.stripRow() + "\n" + rest
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

// stripRow names every screen, the one in front filled; the editor's by its
// file, like the files beside it. An application tab's legend is its own, so
// ctrl+t is named here while one is in front.
func (t tabbedModel) stripRow() string {
	names := t.names()
	names[0] = filepath.Base(t.editor.doc.Path())
	tabsW := len(names) - 1
	for _, n := range names {
		tabsW += ansi.StringWidth(n) + 2
	}
	row := draw.Tabs(tabsW, names, t.active, t.tabStyle())
	if t.active > 0 && t.active < t.fileScreen(0) {
		row += "  " + t.theme.Chrome.Dim.Render(tabKey+" change tab")
	}
	return draw.Fit(row, t.width)
}

// tabStyle fills the tab in front with the accent: a filled block reads as
// selected at a glance, where the theme's underline has to be looked for.
func (t tabbedModel) tabStyle() draw.ChromeStyle {
	c, st := t.theme.Colors, t.theme.Chrome
	st.TabActive = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(c.OnAccent)).Background(lipgloss.Color(c.Accent))
	st.Tab = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Dim))
	return st
}
