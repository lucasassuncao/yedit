package trace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/cursor"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterRecordsKeysMessagesAndActionsInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	w, err := New(path)
	require.NoError(t, err)
	w.Msg("list", tea.KeyPressMsg{Code: tea.KeyEnter})
	w.Msg("list", tea.WindowSizeMsg{Width: 80, Height: 24})
	w.Action("model", "", struct{ Key string }{"cats"})
	require.NoError(t, w.Close())
	assert.Equal(t, path, w.Path())

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	var events []map[string]any
	for sc := bufio.NewScanner(f); sc.Scan(); {
		var ev map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &ev))
		events = append(events, ev)
	}
	require.Len(t, events, 3)
	assert.Equal(t, "key", events[0]["scope"], "a key message goes to the key scope")
	assert.Equal(t, "enter", events[0]["key"])
	assert.Equal(t, "msg", events[1]["scope"])
	assert.Equal(t, "tea.WindowSizeMsg", events[1]["type"])
	assert.Equal(t, "model", events[2]["scope"])
	assert.EqualValues(t, 3, events[2]["seq"])
}

func TestIsNoiseDropsCursorBlinks(t *testing.T) {
	assert.True(t, IsNoise(cursor.BlinkMsg{}))
	assert.False(t, IsNoise(tea.WindowSizeMsg{}))
}
