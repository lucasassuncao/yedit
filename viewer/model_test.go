package viewer

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/bezel/bezeltest"
)

type stubSource struct{}

func (stubSource) ListFields() []string { return []string{"image", "volumes"} }
func (stubSource) ListPresets(f string) []string {
	if f == "image" {
		return []string{"alpine", "debian"}
	}
	return []string{"data"}
}
func (stubSource) PresetYAML(f, n string) (string, error) { return f + ": " + n + "\n", nil }

// tallSource returns a preset far taller than any test viewport so the right
// pane has something to scroll.
type tallSource struct{}

func (tallSource) ListFields() []string        { return []string{"alpha", "beta"} }
func (tallSource) ListPresets(string) []string { return []string{"base"} }
func (tallSource) PresetYAML(f, n string) (string, error) {
	var b strings.Builder
	for i := range 100 {
		fmt.Fprintf(&b, "key%d: value\n", i)
	}
	return b.String(), nil
}

type emptySource struct{}

func (emptySource) ListFields() []string                      { return nil }
func (emptySource) ListPresets(string) []string               { return nil }
func (emptySource) PresetYAML(string, string) (string, error) { return "", nil }

func newModel(t *testing.T, src interface {
	ListFields() []string
	ListPresets(string) []string
	PresetYAML(string, string) (string, error)
}) *Model {
	t.Helper()
	m := NewModel(src)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	return &m
}

func TestDrillIntoAFieldAndBack(t *testing.T) {
	m := newModel(t, stubSource{})
	require.Equal(t, "image", m.b.Selected().Label)
	require.Contains(t, ansi.Strip(m.View().Content), "Fields")

	m.Update(bezeltest.Key("enter"))
	require.Equal(t, "image", m.field)
	require.Equal(t, "alpine", m.b.Selected().Label)
	require.Contains(t, ansi.Strip(m.View().Content), "Presets · image")

	m.Update(bezeltest.Key("down"))
	require.Equal(t, "debian", m.b.Selected().Label)

	m.Update(bezeltest.Key("left"))
	require.Equal(t, "", m.field)
	require.Equal(t, "image", m.b.Selected().Label, "back lands on the field just left")
}

func TestPreviewPaneScrollsOnlyWhenFocused(t *testing.T) {
	m := newModel(t, tallSource{})
	top := m.b.PreviewView(5)
	m.Update(bezeltest.Key("down")) // list has focus: moves the cursor, not the preview
	require.Equal(t, top, m.b.PreviewView(5))

	m.Update(bezeltest.Key("tab"))
	require.True(t, m.b.PreviewFocus)
	m.Update(bezeltest.Key("down"))
	require.NotEqual(t, top, m.b.PreviewView(5))
	m.Update(bezeltest.Key("pgdown"))
	m.Update(bezeltest.Key("tab"))
	m.Update(bezeltest.Key("up")) // selection change resets the scroll
	require.Equal(t, top, m.b.PreviewView(5))
}

func TestEmptySourceRendersAMessage(t *testing.T) {
	m := newModel(t, emptySource{})
	require.Contains(t, m.View().Content, "No presets available.")
}

func TestHelpIsShownOpensAndCloses(t *testing.T) {
	m := NewModel(stubSource{})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	rows := strings.Split(ansi.Strip(m.View().Content), "\n")
	require.Contains(t, rows[len(rows)-1], "[?] help")

	m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	require.True(t, m.sh.HasOverlay(), "? opens the help")

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	m.Update(cmd())
	require.False(t, m.sh.HasOverlay(), "esc closes it")
}
