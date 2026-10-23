package render

import (
	"fmt"
	"runtime"
	"strings"
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

// One line changed in a long file costs memory in proportion to the file, not
// to its square: saving a big config must not run the editor out of memory.
func TestDiffOfOneChangeInALongFileStaysSmall(t *testing.T) {
	var sb strings.Builder
	for i := range 20000 {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	before := sb.String()
	after := strings.Replace(before, "line 10000\n", "LINE 10000\n", 1)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	start := ms.TotalAlloc
	got := Diff([]byte(before), []byte(after), DiffStyles{})
	runtime.ReadMemStats(&ms)

	assert.Equal(t, []string{
		"@@ -9999,5 +9999,5 @@",
		" 9999  9999   line 9998 ", "10000 10000   line 9999 ", "10001       - line 10000",
		"      10001 + LINE 10000", "10002 10002   line 10001", "10003 10003   line 10002",
	}, got.Rows)
	assert.Less(t, ms.TotalAlloc-start, uint64(64<<20), "a 20000-line table would be 3 GB")
}

// Lines alike at both ends stay kept; only what lies between them is diffed.
func TestDiffOfARepeatedLineAddsOne(t *testing.T) {
	got := Diff([]byte("a\na\n"), []byte("a\na\na\n"), DiffStyles{})

	assert.Equal(t, []string{"@@ -1,2 +1,3 @@", "1 1   a", "2 2   a", "  3 + a"}, got.Rows)
}
