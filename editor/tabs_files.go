package editor

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/bezel/draw"
	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/theme"
)

// fileTab is a file opened from a PathField entry, edited by an editor of its
// own after the application's tabs. Its messages come back as fileMsg, so a
// save or a dialog there never reaches another document's editor.
type fileTab struct {
	id int
	m  model
}

func (f fileTab) name() string { return filepath.Base(f.m.doc.Path()) }

// fileMsg is a file tab's own message on its way back to it.
type fileMsg struct {
	id  int
	msg tea.Msg
}

// closeFileRequestedMsg is esc on a file tab's block list; closeFileMsg is the
// answer once nothing would be lost, or the user said so.
type (
	closeFileRequestedMsg struct{}
	closeFileMsg          struct{}
)

// tagFile sends cmd's messages back to file tab id. A batch stays a batch so
// the runtime still runs it, and quitting stays the whole program's. The
// editor uses no tea.Sequence, whose message could not be told apart.
func tagFile(id int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			out := make(tea.BatchMsg, 0, len(msg))
			for _, c := range msg {
				out = append(out, tagFile(id, c))
			}
			return out
		case tea.QuitMsg, QuitMsg:
			return QuitMsg{}
		default:
			return fileMsg{id: id, msg: msg}
		}
	}
}

// opensFiles reports whether an entry of cfg's PathFields can open a file, which
// needs the tabs to show it in.
func opensFiles(cfg Config) bool {
	for _, pf := range cfg.PathFields {
		if pf.Open != nil {
			return true
		}
	}
	return false
}

// samePath reports whether a and b name one file, as the platform compares names.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// fileScreen is the screen index of file tab i.
func (t tabbedModel) fileScreen(i int) int { return 1 + len(t.tabs) + i }

// openFile brings the file cfg edits to front, opening it in a tab of its own
// when no screen has it yet. The tab gets no Init: the program already asked
// the terminal for its background, and t.dark has the answer.
func (t tabbedModel) openFile(cfg Config) (tea.Model, tea.Cmd) {
	if samePath(cfg.Path, t.editor.doc.Path()) {
		t.active = 0
		return t, nil
	}
	for i, f := range t.files {
		if samePath(cfg.Path, f.m.doc.Path()) {
			t.active = t.fileScreen(i)
			return t, nil
		}
	}
	// Tabs and Actions are the program's: Exec could not be routed back here.
	cfg.Tabs, cfg.Actions = nil, nil
	m, err := newModel(cfg)
	if err != nil {
		return t.alertFront("Cannot open "+filepath.Base(cfg.Path), err.Error())
	}
	m.inFileTab, m.tabbed = true, true // a file tab is never the only screen
	m = m.withTheme(theme.Resolve(cfg.Theme, t.dark))
	t.nextFile++
	f := fileTab{id: t.nextFile, m: m}
	u, cmd := f.m.Update(tea.WindowSizeMsg{Width: t.width, Height: t.height - stripRows})
	f.m = u.(model)
	t.files = append(slices.Clone(t.files), f)
	t.active = t.fileScreen(len(t.files) - 1)
	return t, tagFile(f.id, cmd)
}

// alertFront shows an error on the screen in front, which asked for it.
func (t tabbedModel) alertFront(title, message string) (tea.Model, tea.Cmd) {
	alert := overlay.NewAlert(draw.Danger, title, message, t.theme)
	if i := t.active - t.fileScreen(0); i >= 0 {
		files := slices.Clone(t.files)
		files[i].m = files[i].m.enterAlert(alert)
		t.files = files
		return t, nil
	}
	t.editor = t.editor.enterAlert(alert)
	return t, nil
}

// fromFile delivers a file tab's own message: closing it, opening another file,
// or anything else for the tab itself.
func (t tabbedModel) fromFile(msg fileMsg) (tea.Model, tea.Cmd) {
	i := slices.IndexFunc(t.files, func(f fileTab) bool { return f.id == msg.id })
	if i < 0 {
		return t, nil // the tab closed while its message was on the way
	}
	switch inner := msg.msg.(type) {
	case closeFileMsg:
		t.files = slices.Delete(slices.Clone(t.files), i, i+1)
		t.active = 0
		return t, nil
	case openFileMsg:
		return t.openFile(inner.cfg)
	}
	return t.toFile(i, msg.msg)
}

// toFile hands msg to file tab i; what it answers comes back to it alone.
func (t tabbedModel) toFile(i int, msg tea.Msg) (tea.Model, tea.Cmd) {
	files := slices.Clone(t.files)
	u, cmd := files[i].m.Update(msg)
	files[i].m = u.(model)
	t.files = files
	return t, tagFile(files[i].id, cmd)
}

// toFiles hands msg to every file tab, as the terminal's size and colours are
// every screen's.
func (t tabbedModel) toFiles(msg tea.Msg) (tabbedModel, tea.Cmd) {
	cmds := make([]tea.Cmd, 0, len(t.files))
	for i := range t.files {
		u, cmd := t.toFile(i, msg)
		t = u.(tabbedModel)
		cmds = append(cmds, cmd)
	}
	return t, tea.Batch(cmds...)
}

// unsavedFiles is a line for every file tab that has changes to lose.
func (t tabbedModel) unsavedFiles() []string {
	var out []string
	for _, f := range t.files {
		if f.m.unsaved() {
			out = append(out, f.name()+" has unsaved changes")
		}
	}
	return out
}
