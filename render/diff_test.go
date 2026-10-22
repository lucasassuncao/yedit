package render

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestDiffShowsAChangeWithTwoLinesOfContext(t *testing.T) {
	got := Diff([]byte("a\nb\nc\nd\ne\nf\ng\n"), []byte("a\nb\nc\nD\ne\nf\ng\n"), DiffStyles{})

	assert.Equal(t, []string{"@@ -2,5 +2,5 @@", "2 2   b", "3 3   c", "4   - d", "  4 + D", "5 5   e", "6 6   f"}, got.Rows)
	assert.Equal(t, 1, got.Added)
	assert.Equal(t, 1, got.Removed)
}

func TestDiffIsEmptyWhenNothingChanged(t *testing.T) {
	got := Diff([]byte("a\nb\n"), []byte("a\nb\n"), DiffStyles{})

	assert.Empty(t, got.Rows)
	assert.Zero(t, got.Added+got.Removed)
}

func TestDiffOfANewFileAddsEveryLine(t *testing.T) {
	got := Diff(nil, []byte("a\nb\n"), DiffStyles{})

	assert.Equal(t, []string{"@@ -0,0 +1,2 @@", "  1 + a", "  2 + b"}, got.Rows)
	assert.Equal(t, 2, got.Added)
}

func TestDiffSplitsDistantChangesIntoHunks(t *testing.T) {
	got := Diff([]byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n"), []byte("X\n2\n3\n4\n5\n6\n7\n8\nY\n"), DiffStyles{})

	assert.Equal(t, []string{
		"@@ -1,3 +1,3 @@", "1   - 1", "  1 + X", "2 2   2", "3 3   3",
		"@@ -7,3 +7,3 @@", "7 7   7", "8 8   8", "9   - 9", "  9 + Y",
	}, got.Rows)
}

func TestDiffMergesChangesWhoseContextTouches(t *testing.T) {
	got := Diff([]byte("1\n2\n3\n4\n5\n6\n"), []byte("X\n2\n3\n4\n5\nY\n"), DiffStyles{})

	assert.Equal(t, []string{
		"@@ -1,6 +1,6 @@", "1   - 1", "  1 + X", "2 2   2", "3 3   3", "4 4   4", "5 5   5", "6   - 6", "  6 + Y",
	}, got.Rows)
}

// Rows share one width, so a row style paints them edge to edge.
func TestDiffPadsRowsToOneWidth(t *testing.T) {
	got := Diff([]byte("a\n"), []byte("abc\n"), DiffStyles{})

	assert.Equal(t, []string{"@@ -1,1 +1,1 @@", "1   - a  ", "  1 + abc"}, got.Rows)
}

func TestDiffPaintsOnlyChangedRows(t *testing.T) {
	plain := Diff([]byte("a\nb\n"), []byte("a\nB\n"), DiffStyles{}).Rows
	st := DiffStyles{
		AddedRow:   lipgloss.NewStyle().Background(lipgloss.Color("#123123")),
		RemovedRow: lipgloss.NewStyle().Background(lipgloss.Color("#321321")),
	}

	got := Diff([]byte("a\nb\n"), []byte("a\nB\n"), st).Rows

	for i := range got {
		assert.Equal(t, plain[i], ansi.Strip(got[i]), "styles change colours, not text")
	}
	assert.Equal(t, plain[1], got[1], "a kept row is not painted")
	assert.NotEqual(t, plain[2], got[2], "a removed row is painted")
	assert.NotEqual(t, plain[3], got[3], "an added row is painted")
}
