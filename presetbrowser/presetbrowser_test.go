package presetbrowser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
)

// stubPresets implements presets.Source for tests.
type stubPresets struct {
	data map[string]string // key: "field/name" → YAML snippet
}

func (s stubPresets) ListFields() []string { return nil }
func (s stubPresets) ListPresets(field string) []string {
	prefix := field + "/"
	var out []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, strings.TrimPrefix(k, prefix))
		}
	}
	return out
}
func (s stubPresets) PresetYAML(field, name string) (string, error) {
	if y, ok := s.data[field+"/"+name]; ok {
		return y, nil
	}
	return "", fmt.Errorf("not found")
}

// Exercises the preset-picker sub-model: construction, navigation, focus
// toggling, and the apply/append/dismiss actions it reports back.
func TestUpdateAndSelection(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)

	src := stubPresets{data: map[string]string{
		"workers/alpha": "workers:\n  - name: a\n",
		"workers/beta":  "workers:\n  - name: b\n",
	}}

	_, ok := New(nil, "workers", "")
	is.False(ok, "nil source should not open a browser")
	_, ok = New(src, "nothing", "")
	is.False(ok, "field without presets should not open a browser")

	pb, ok := New(src, "workers", "beta")
	must.True(ok, "expected a browser for workers")
	is.Equal("beta", pb.names[pb.cursor], "cursor should pre-select the current preset")

	keyOf := func(s string) tea.KeyMsg {
		switch s {
		case "enter":
			return tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			return tea.KeyPressMsg{Code: tea.KeyEsc}
		case "tab":
			return tea.KeyPressMsg{Code: tea.KeyTab}
		default:
			return tea.KeyPressMsg{Text: s, Code: []rune(s)[0]}
		}
	}

	var (
		action Action
		name   string
	)
	pb, action, _ = pb.Update(tea.KeyPressMsg{Code: tea.KeyUp}, false)
	is.Equal(None, action, "up should not trigger action")
	is.Equal(0, pb.cursor, "up should move cursor to 0")

	pb, action, name = pb.Update(keyOf("enter"), false)
	is.Equal(Applied, action, "enter should apply preset")
	is.Equal(pb.names[0], name, "enter should apply first preset name")

	pb, action, name = pb.Update(keyOf("a"), true)
	is.Equal(Appended, action, "a with allowAppend should append")
	is.Equal(pb.names[0], name, "a should append first preset name")

	pb, action, _ = pb.Update(keyOf("a"), false)
	is.Equal(None, action, "a without allowAppend should be a no-op")

	// Tab moves focus to the preview; esc first returns focus, then dismisses.
	pb, _, _ = pb.Update(keyOf("tab"), false)
	must.True(pb.PreviewFocus, "tab should focus the preview")

	pb, action, _ = pb.Update(keyOf("enter"), false)
	is.Equal(None, action, "enter with preview focused should be a no-op")

	pb, action, _ = pb.Update(keyOf("esc"), false)
	is.Equal(None, action, "first esc should only return focus to the list")
	is.False(pb.PreviewFocus, "first esc should clear previewFocus")

	pb, action, _ = pb.Update(keyOf("esc"), false)
	is.Equal(Dismissed, action, "second esc should dismiss")
}
