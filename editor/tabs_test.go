package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/blocklist"
)

// fakeTab is a Tab that keeps every message it is sent.
type fakeTab struct {
	name string
	got  *[]tea.Msg
	busy string
}

func (f fakeTab) Name() string  { return f.name }
func (f fakeTab) Init() tea.Cmd { return nil }
func (f fakeTab) View() string  { return "TAB HEAD\nTAB BODY" }
func (f fakeTab) Busy() string  { return f.busy }
func (f fakeTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	*f.got = append(*f.got, msg)
	return f, nil
}

// got is what the tab was sent of type T.
func got[T any](msgs []tea.Msg) []T {
	var out []T
	for _, m := range msgs {
		if v, ok := m.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

var ctrlT = tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}

// tabsModel opens a one-block document with tabs beside the editor.
func tabsModel(t *testing.T, cfg Config, tabs ...Tab) tabbedModel {
	t.Helper()
	cfg.Path = filepath.Join(t.TempDir(), "tabs.yaml")
	require.NoError(t, os.WriteFile(cfg.Path, []byte("server:\n  host: a\n"), 0o600))
	cfg.Schema = &sizeProbeConfig{}
	cfg.Tabs = tabs
	root, err := newRoot(cfg)
	require.NoError(t, err)
	u, _ := root.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return u.(tabbedModel)
}

func sendTabbed(m tabbedModel, msg tea.Msg) (tabbedModel, tea.Cmd) {
	u, cmd := m.Update(msg)
	return u.(tabbedModel), cmd
}

func screen(m tabbedModel) string { return ansi.Strip(m.View().Content) }

func TestTabsSwitchWithCtrlT(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{}, fakeTab{name: "download", got: &msgs})

	out := screen(m)
	strip, _, _ := strings.Cut(out, "\n")
	assert.Contains(t, strip, " editor ")
	assert.Contains(t, strip, " download ")
	assert.Contains(t, strip, "ctrl+t")
	assert.Contains(t, out, "Blocks", "the editor is in front")
	assert.NotContains(t, out, "TAB BODY")

	m, _ = sendTabbed(m, ctrlT)
	out = screen(m)
	assert.Contains(t, out, "TAB BODY")
	assert.NotContains(t, out, "Blocks")

	m, _ = sendTabbed(m, ctrlT)
	assert.Contains(t, screen(m), "Blocks", "ctrl+t comes back round to the editor")
}

// FrontTab opens the program on one of the tabs; a name no tab has opens it
// on the editor.
func TestFrontTabOpensInFront(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{FrontTab: "download"}, fakeTab{name: "download", got: &msgs})
	assert.Equal(t, 1, m.active)
	assert.Contains(t, screen(m), "TAB BODY")

	m = tabsModel(t, Config{FrontTab: "nope"}, fakeTab{name: "download", got: &msgs})
	assert.Equal(t, 0, m.active)
}

// The strip rides on the screen's own first row, so every screen has the
// whole terminal and its rows are the terminal's.
func TestTabsHaveTheWholeTerminal(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{}, fakeTab{name: "download", got: &msgs})

	assert.Equal(t, []tea.WindowSizeMsg{{Width: 120, Height: 40}}, got[tea.WindowSizeMsg](msgs))
	assert.Equal(t, 40, m.editor.height)
	assert.Len(t, strings.Split(screen(m), "\n"), 40)
}

func TestTheStripTakesTheEndOfTheScreensFirstRow(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{FrontTab: "download"}, fakeTab{name: "download", got: &msgs})

	head, body, _ := strings.Cut(screen(m), "\n")

	assert.Regexp(t, `^TAB HEAD +editor   download  ctrl\+t switch$`, head)
	assert.Equal(t, 120, ansi.StringWidth(head))
	assert.True(t, strings.HasPrefix(body, "TAB BODY"), "the screen's other rows stay where they are")
}

// Short of room, the hint goes first, then the end of the screen's text.
func TestTheScreensHeaderGivesWayToTheStrip(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{FrontTab: "download"}, fakeTab{name: "download", got: &msgs})

	m, _ = sendTabbed(m, tea.WindowSizeMsg{Width: 40, Height: 40})
	head, _, _ := strings.Cut(screen(m), "\n")
	assert.Regexp(t, `^TAB HEAD +editor   download $`, head)

	m, _ = sendTabbed(m, tea.WindowSizeMsg{Width: 25, Height: 40})
	head, _, _ = strings.Cut(screen(m), "\n")
	assert.Equal(t, "TAB …  editor   download ", head)
}

func TestFrontMsgSaysWhichScreenIsInFront(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{}, fakeTab{name: "download", got: &msgs})
	m = runTabbed(t, m, m.Init())
	assert.Equal(t, []FrontMsg{{Name: "editor"}}, got[FrontMsg](msgs), "at the start")

	m, cmd := sendTabbed(m, ctrlT)
	m = runTabbed(t, m, cmd)
	assert.Equal(t, []FrontMsg{{Name: "editor"}, {Name: "download"}}, got[FrontMsg](msgs))

	_, cmd = sendTabbed(m, QuitMsg{})
	assert.Contains(t, drainBatch(cmd()), tea.Msg(FrontMsg{Name: "editor"}), "the editor asks in front")
}

type ping struct{}

// Work a tab started goes on behind the editor, but a key only reaches the
// screen in front: q on the tab must not quit the editor.
func TestTabsGetEveryMessageButOtherScreensKeys(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{}, fakeTab{name: "download", got: &msgs})

	m, _ = sendTabbed(m, ping{})
	m, _ = sendTabbed(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	assert.Len(t, got[ping](msgs), 1, "the tab hears its messages while the editor is in front")
	assert.Empty(t, got[tea.KeyPressMsg](msgs), "a key goes to the editor alone")

	m, _ = sendTabbed(m, ctrlT)
	_, cmd := sendTabbed(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	assert.Len(t, got[tea.KeyPressMsg](msgs), 1)
	assert.Nil(t, cmd, "the editor never saw q")
}

// runTabbed runs cmd and feeds what it answers back into m, as the program
// would; a command that takes longer than a timer's tick is left alone.
func runTabbed(t *testing.T, m tabbedModel, cmd tea.Cmd) tabbedModel {
	t.Helper()
	if cmd == nil {
		return m
	}
	answer := make(chan tea.Msg, 1)
	go func() { answer <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-answer:
	case <-time.After(200 * time.Millisecond):
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runTabbed(t, m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	m, cmd = sendTabbed(m, msg)
	return runTabbed(t, m, cmd)
}

// asking is a tab model with the download tab in front and an edited document.
func asking(t *testing.T, cfg Config, msgs *[]tea.Msg) tabbedModel {
	t.Helper()
	m := tabsModel(t, cfg, fakeTab{name: "download", got: msgs})
	var err error
	m.editor.doc, err = m.editor.doc.ReplaceRaw([]byte("server:\n  host: b\n"))
	require.NoError(t, err)
	m, _ = sendTabbed(m, ctrlT)
	return m
}

func TestSaveMsgSavesAndAnswersTheTabs(t *testing.T) {
	var msgs []tea.Msg
	m := asking(t, Config{NoSaveConfirm: true}, &msgs)

	m, cmd := sendTabbed(m, SaveMsg{})
	assert.Equal(t, 0, m.active, "the editor comes in front to save")
	m = runTabbed(t, m, cmd)

	assert.Equal(t, []SavedMsg{{Path: m.editor.doc.Path()}}, got[SavedMsg](msgs))
	assert.Equal(t, 1, m.active, "back to the tab that asked")
	assert.NotEqual(t, paneAlert, m.editor.mode, "no Saved alert to dismiss")
	on, err := os.ReadFile(m.editor.doc.Path())
	require.NoError(t, err)
	assert.Equal(t, "server:\n  host: b\n", string(on))
}

func TestSaveMsgOnASavedDocumentAnswersAtOnce(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{}, fakeTab{name: "download", got: &msgs})
	m, _ = sendTabbed(m, ctrlT)

	m, _ = sendTabbed(m, SaveMsg{})

	assert.Len(t, got[SavedMsg](msgs), 1)
	assert.Equal(t, 1, m.active, "nothing to ask, so the tab stays in front")
}

// An open block editor holds edits the document does not have yet: the tab
// waits for it rather than save without them.
func TestSaveMsgWithABlockEditorOpenWaits(t *testing.T) {
	var msgs []tea.Msg
	m := asking(t, Config{NoSaveConfirm: true}, &msgs)
	m.active = 0
	m = runTabbed(t, m, func() tea.Msg { return blocklist.OpenItemMsg{Item: blocklist.Item{Key: "server", Existing: true}} })
	require.Equal(t, paneBlockEdit, m.editor.mode)
	m, _ = sendTabbed(m, ctrlT)

	m, cmd := sendTabbed(m, SaveMsg{})
	m = runTabbed(t, m, cmd)

	assert.Empty(t, got[SavedMsg](msgs))
	assert.Equal(t, 0, m.active)
	assert.Contains(t, screen(m), "block editor")
}

func TestCancelledSaveAnswersNothing(t *testing.T) {
	var msgs []tea.Msg
	m := asking(t, Config{}, &msgs)

	m, cmd := sendTabbed(m, SaveMsg{})
	m = runTabbed(t, m, cmd)
	require.Contains(t, screen(m), "Save changes?")
	m, cmd = sendTabbed(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTabbed(t, m, cmd)

	assert.Empty(t, got[SavedMsg](msgs))
	m, cmd = sendTabbed(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = runTabbed(t, m, cmd)
	m, cmd = sendTabbed(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	runTabbed(t, m, cmd)
	assert.Empty(t, got[SavedMsg](msgs), "the editor's own save answers no tab")
}

// quitting presses q on the list and returns the screen and what confirming
// the question it asks sends.
func quitting(t *testing.T, m tabbedModel) (string, []tea.Msg) {
	t.Helper()
	m, cmd := sendTabbed(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	require.NotNil(t, cmd)
	m, _ = sendTabbed(m, cmd())
	out := screen(m)
	_, cmd = sendTabbed(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "the question is confirmed")
	return out, drainBatch(cmd())
}

func TestQuitAsksWhileATabIsBusy(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{}, fakeTab{name: "download", got: &msgs, busy: "A download is running."})

	out, sent := quitting(t, m)

	assert.Contains(t, out, "A download is running.")
	assert.Contains(t, sent, tea.Msg(tea.QuitMsg{}))
}

func TestQuitSaysBothWhatWouldBeLost(t *testing.T) {
	var msgs []tea.Msg
	m := asking(t, Config{}, &msgs)
	m.tabs = []Tab{fakeTab{name: "download", got: &msgs, busy: "A download is running."}}
	m.active = 0

	out, _ := quitting(t, m)

	assert.Contains(t, out, "Unsaved changes will be lost.")
	assert.Contains(t, out, "A download is running.")
}

func TestQuitMsgFromATabQuitsThroughTheEditor(t *testing.T) {
	var msgs []tea.Msg
	m := tabsModel(t, Config{}, fakeTab{name: "download", got: &msgs})
	m, _ = sendTabbed(m, ctrlT)

	m, cmd := sendTabbed(m, QuitMsg{})

	assert.Equal(t, 0, m.active, "the editor asks, if it has to")
	require.NotNil(t, cmd)
	assert.Contains(t, drainBatch(cmd()), tea.Msg(tea.QuitMsg{}), "nothing to lose: it quits")
}

func TestNoTabsLeavesTheEditorAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plain.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  host: a\n"), 0o600))

	root, err := newRoot(Config{Path: path, Schema: &sizeProbeConfig{}})

	require.NoError(t, err)
	assert.IsType(t, model{}, root)
}
