package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/x/ansi"
	"github.com/lucasassuncao/bezel/overlay"

	tea "charm.land/bubbletea/v2"
)

// TestPreviewIsReadOnly verifies that typing in the preview pane never mutates
// the document - the right panel is a read-only, syntax-highlighted view.
func TestPreviewIsReadOnly(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "ro.yaml")
	must.NoError(os.WriteFile(path, []byte("server:\n  host: localhost\n"), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	// Enter the read-only preview pane via Tab.
	m, _ = pressAction(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	must.Equal(panePreview, m.mode, "expected panePreview after Tab")

	before := string(m.doc.Raw())
	dirtyBefore := m.doc.Dirty()

	// Type characters - a read-only preview must ignore them.
	for _, r := range "xyz: hello" {
		updated, _ = m.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		m = updated.(model)
	}

	is.Equal(before, string(m.doc.Raw()), "preview edited the document")
	is.Equal(dirtyBefore, m.doc.Dirty(), "preview changed dirty state")
}

// TestFilterAcceptsJK guards against j/k being swallowed as navigation while
// the filter input is active - filters like "unknown" (contains k) or
// "worker" (contains k) must be typeable; only the arrow keys navigate.
func TestFilterAcceptsJK(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "filter.yaml")
	seed := "server:\n  host: a\nunknown-key: flagged\n"
	must.NoError(os.WriteFile(path, []byte(seed), 0o600))
	m, err := newModel(Config{Path: path, Schema: &sizeProbeConfig{}})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)

	updated, _ = m.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	m = updated.(model)
	for _, r := range "unknown" {
		updated, _ = m.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		m = updated.(model)
	}
	must.Equal("unknown", m.list.Filter(), "filter text after typing")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(model)

	sel := m.list.SelectedItem()
	must.NotNil(sel, "selected item should not be nil after filter+enter")
	is.Equal("unknown-key", sel.Key, "filter+enter should select unknown-key")
}

// Leaving the preset picker must restore the list screen's layout, or the
// list pane has no rect and the preview takes the whole body.
func TestDismissDocPresetsRestoresListPane(t *testing.T) {
	must := require.New(t)
	m, err := newModel(Config{
		Path:        filepath.Join(t.TempDir(), "probe.yaml"),
		Schema:      &sizeProbeConfig{},
		DocPresets:  stubPresets{data: map[string]string{"/basic": "server:\n  host: a\n"}},
		EnableHints: true,
	})
	must.NoError(err, "newModel")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = updated.(model)
	hintBefore, listBefore := m.sh.Rect("hint"), m.sh.Rect("list")
	must.Positive(hintBefore.H, "hint should be open before the picker")

	updated, _ = m.Update(openDocPresetsMsg{})
	m = updated.(model)
	must.Equal(paneDocPreset, m.mode)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(model)
	must.Equal(paneList, m.mode)
	must.Equal(listBefore, m.sh.Rect("list"), "list pane lost its rect")
	must.Equal(hintBefore, m.sh.Rect("hint"), "hint pane did not come back")
}

// An overlay opened from the preview or the hint leaves that pane framed while
// it is up, and gives the keys back to it when it closes.
func TestAnOverlayGivesTheKeysBackToThePaneUnderIt(t *testing.T) {
	focus := map[string]func(*testing.T, model) model{
		"preview": func(t *testing.T, m model) model {
			m, _ = pressAction(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
			return m
		},
		"hint": func(t *testing.T, m model) model {
			updated, _ := m.Update(focusHintMsg{})
			return updated.(model)
		},
	}
	open := map[string]func(*testing.T, model) model{
		"help": func(t *testing.T, m model) model {
			updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
			return updated.(model)
		},
		"validate": func(t *testing.T, m model) model {
			m, _ = pressAction(t, m, tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
			return m
		},
	}
	for paneName, toPane := range focus {
		for overlayName, toOverlay := range open {
			if paneName == "hint" && overlayName == "validate" {
				continue // the hint is for reading: it offers no document keys
			}
			t.Run(paneName+"/"+overlayName, func(t *testing.T) {
				must := require.New(t)
				m := toPane(t, newHintModel(t, 0))
				want := m.mode
				must.Equal(paneName, m.rootPane())

				m = toOverlay(t, m)
				must.True(m.sh.HasOverlay(), "the overlay is up")
				must.Equal(paneName, m.rootPane(), "the pane keeps its frame under the overlay")
				must.Contains(ansi.Strip(m.View().Content), "[esc] back", "the legend is still the %s's", paneName)

				updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
				if cmd != nil {
					if msg, ok := cmd().(overlay.CloseMsg); ok {
						updated, _ = updated.Update(msg)
					}
				}
				m = updated.(model)
				must.False(m.sh.HasOverlay(), "esc closes the overlay")
				must.Equal(want, m.mode, "the keys go back to the %s", paneName)
			})
		}
	}
}
