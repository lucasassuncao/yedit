package editor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A CR in the preview sends the terminal cursor to column 0 and draws over the
// panel beside it, so a preset from a CRLF file must preview with LF endings.
func TestPresetPreviewDropsCarriageReturns(t *testing.T) {
	src := stubPresets{data: map[string]string{"/golang": "name: go\r\nimage: golang\r\n"}}

	items := presetItems(src, "")

	require.Len(t, items, 1)
	require.False(t, strings.Contains(items[0].Detail(), "\r"), "preview still carries a CR")
	require.Equal(t, "name: go\nimage: golang\n", items[0].Detail())
}
