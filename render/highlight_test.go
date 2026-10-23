package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// spanText is the text each span of line covers.
func spanText(line string, spans [][]int) []string {
	var out []string
	for _, s := range spans {
		out = append(out, line[s[0]:s[1]])
	}
	return out
}

func TestYAMLHighlightGivesEachLineItsSpans(t *testing.T) {
	src := "name: \"café\" # note\ncount: 3\nlist:\n  - true"
	got := YAMLHighlight(src)
	lines := strings.Split(src, "\n")
	require.Len(t, got, len(lines))

	for i, l := range lines {
		var cover [][]int
		for _, s := range got[i] {
			require.True(t, 0 <= s.Start && s.Start < s.End && s.End <= len(l), "line %d span %v out of bounds", i, s)
			cover = append(cover, []int{s.Start, s.End})
		}
		require.NotEmpty(t, cover, "line %d has no colour", i)
		texts := spanText(l, cover)
		switch i {
		case 0:
			require.Contains(t, texts, "name")
			require.Contains(t, texts, "\"café\"", "the string ends on a byte, not a rune")
			require.Contains(t, texts, "# note")
		case 1:
			require.Contains(t, texts, "3")
		}
	}
}

func TestYAMLHighlightPaintsKeysAndStringsApart(t *testing.T) {
	got := YAMLHighlight("key: \"value\"")[0]
	styleOf := func(text string) string {
		for _, s := range got {
			if "key: \"value\""[s.Start:s.End] == text {
				return s.Style.Render("x")
			}
		}
		t.Fatalf("no span for %q", text)
		return ""
	}
	require.NotEqual(t, styleOf("key"), styleOf("\"value\""))
}

func TestYAMLHighlightOfNothingIsOneEmptyLine(t *testing.T) {
	require.Len(t, YAMLHighlight(""), 1)
}
