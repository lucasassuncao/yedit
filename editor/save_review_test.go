package editor

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/bezel/layout"
	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/theme"
)

type savedForTest struct{}

func review(before, after string, h int) saveReview {
	onSave := func() tea.Msg { return savedForTest{} }
	r := newSaveReview(theme.Resolve(theme.Theme{}, true), "conf.yaml", []byte(before), []byte(after), onSave)
	return r.Resize(layout.Rect{W: 100, H: h}).(saveReview)
}

func reviewScreen(o overlay.Overlay, h int) string {
	return ansi.Strip(o.View(layout.Rect{W: 100, H: h}))
}

func TestSaveReviewShowsTheDiffAndItsCounts(t *testing.T) {
	screen := reviewScreen(review("host: a\nport: 1\n", "host: b\nport: 1\n", 30), 30)

	assert.Contains(t, screen, "Save changes?")
	assert.Contains(t, screen, "Save to conf.yaml")
	assert.Contains(t, screen, "- host: a")
	assert.Contains(t, screen, "+ host: b")
	assert.Contains(t, screen, "+1 -1")
	assert.Contains(t, screen, "[enter] save")
}

func TestSaveReviewSaysWhenNothingChanged(t *testing.T) {
	assert.Contains(t, reviewScreen(review("a: 1\n", "a: 1\n", 30), 30), "No changes.")
}

func TestSaveReviewSavesOnEnter(t *testing.T) {
	_, cmd := review("a: 1\n", "a: 2\n", 30).Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.NotNil(t, cmd)
	var got []tea.Msg
	for _, c := range cmd().(tea.BatchMsg) {
		got = append(got, c())
	}
	assert.Contains(t, got, savedForTest{})
	assert.Contains(t, got, overlay.CloseMsg{})
}

func TestSaveReviewScrollsALongDiff(t *testing.T) {
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	var o overlay.Overlay = review("", strings.Join(lines, "\n")+"\n", 20)
	first := reviewScreen(o, 20)
	assert.Contains(t, first, "line-08")
	assert.NotContains(t, first, "line-09", "only what fits is drawn")

	o, _ = o.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	o, _ = o.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	screen := reviewScreen(o, 20)
	assert.Contains(t, screen, "line-18")
	assert.NotContains(t, screen, "line-09")
	assert.Contains(t, screen, "lines 11-19 of 51")
}
