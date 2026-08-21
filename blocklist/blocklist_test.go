package blocklist

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildLM creates a Model with the given items and a small viewport.
func buildLM(keys []string, height int) Model {
	var items []Item
	for _, k := range keys {
		items = append(items, Item{Key: k})
	}
	return Model{items: items, height: height}
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

	lm, _ = lm.updateFilter(tea.KeyPressMsg{Code: tea.KeyEnter})

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

	lm, _ = lm.updateFilter(tea.KeyPressMsg{Code: tea.KeyBackspace})

	is.Equal("con", lm.filter, "backspace must remove the whole multibyte rune")
}

// TestListMoveCursorClampsAtBounds verifies the main list clamps at top/bottom
// instead of wrapping around, matching the tree panel.
func TestListMoveCursorClampsAtBounds(t *testing.T) {
	is := assert.New(t)
	lm := New([]string{"a", "b", "c"}, nil, nil, 10)
	first := lm.cursor
	lm = lm.moveCursor(-1) // already at the top - must not wrap to the bottom
	is.Equal(first, lm.cursor, "moveCursor(-1) at top should clamp, not wrap")
	for i := 0; i < len(lm.items); i++ {
		lm = lm.moveCursor(1) // walk to the bottom; clamps once there
	}
	last := lm.cursor
	lm = lm.moveCursor(1) // at the bottom - must not wrap to the top
	is.Equal(last, lm.cursor, "moveCursor(+1) at bottom should clamp, not wrap")
}

// TestListFilterByTyping verifies the "/" filter narrows the list as the user types.
func TestListFilterByTyping(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	lm := New([]string{"alpha", "beta", "gamma"}, nil, nil, 10)
	must.False(lm.IsFiltering(), "should not start in filtering mode")
	lm, _ = lm.Update(tea.KeyPressMsg{Text: "/", Code: '/'})
	must.True(lm.IsFiltering(), `"/" should enter filtering mode`)
	for _, r := range "be" {
		lm, _ = lm.Update(tea.KeyPressMsg{Text: string(r), Code: r})
	}
	got := lm.filteredItems()
	if is.Len(got, 1, `filter "be" should match exactly one item`) {
		is.Equal("beta", got[0].Key, `filter "be" should match beta`)
	}
}
