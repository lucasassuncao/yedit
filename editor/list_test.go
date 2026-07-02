package editor

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

// buildLM creates a listModel with the given items and a small viewport.
func buildLM(keys []string, height int) listModel {
	var items []listItem
	for _, k := range keys {
		items = append(items, listItem{Key: k})
	}
	return listModel{items: items, height: height}
}

// TestFilterEnter_clampScrollApplied guards BUG-001: pressing enter in filter
// mode must update lm.offset so the selected item is actually visible. Before
// the fix, clampScroll() returned a new value that was discarded.
func TestFilterEnter_clampScrollApplied(t *testing.T) {
	is := assert.New(t)

	// 10 items, viewport of 3 rows. Start in filter mode with fCursor on item[7].
	keys := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	lm := buildLM(keys, 3)
	lm.filtering = true
	lm.fCursor = 7 // "h" is at index 7 in the full list (no separators)

	lm, _ = lm.updateFilter(tea.KeyMsg{Type: tea.KeyEnter})

	is.False(lm.filtering, "filtering should be cleared after enter")
	is.Equal(7, lm.cursor, "cursor should point to the selected item")
	// With height=3 the visible window is rows [offset, offset+3). cursor=7
	// must be inside that window, so offset must be >= 5.
	is.GreaterOrEqual(lm.offset, 5, "offset must have been adjusted so the cursor is visible")
}

// TestFilterBackspace_removesWholeRune guards the filter against multibyte
// input: backspace must drop the last rune, not the last byte, or a typed
// "ç" would leave invalid UTF-8 behind and break matching.
func TestFilterBackspace_removesWholeRune(t *testing.T) {
	is := assert.New(t)

	lm := buildLM([]string{"config"}, 3)
	lm.filtering = true
	lm.filter = "conç"

	lm, _ = lm.updateFilter(tea.KeyMsg{Type: tea.KeyBackspace})

	is.Equal("con", lm.filter, "backspace must remove the whole multibyte rune")
}
