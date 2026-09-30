package blocklist

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/bezel/bezeltest"
	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/yedit/document"
)

var th = theme.Resolve(theme.ThemePlain, true)

// TestBuildItemsGroupsKeysBySection pins the four sections and their order.
func TestBuildItemsGroupsKeysBySection(t *testing.T) {
	is := assert.New(t)
	existing := []document.Block{{Key: "b"}, {Key: "x"}, {Key: "p"}}
	items := BuildItems([]string{"a", "b"}, existing, map[string]bool{"p": true})
	var labels []string
	for _, it := range items {
		labels = append(labels, it.Key)
	}
	is.Equal([]string{"ADDED", "b", "", "AVAILABLE", "a", "", "UNKNOWN", "x", "", "PASSTHROUGH", "p"}, labels)
	is.True(items[7].Unknown && !items[7].Passthrough)
	is.True(items[10].Unknown && items[10].Passthrough)
}

// TestEnterOpensKnownKeysAndDeleteAsksForExisting pins the two messages the
// list emits and which rows may emit them.
func TestEnterOpensKnownKeysAndDeleteAsksForExisting(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	lm := New([]string{"a", "b"}, []document.Block{{Key: "b"}, {Key: "zzz"}}, nil, 10, th)
	must.Equal("b", lm.SelectedItem().Key, "cursor starts on the first added key")

	_, cmd := lm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	must.NotNil(cmd)
	is.Equal(OpenItemMsg{Item: Item{Key: "b", Existing: true}}, cmd())

	_, cmd = lm.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	must.NotNil(cmd)
	is.Equal(DeleteItemMsg{Key: "b"}, cmd())

	lm, _ = lm.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // "a", available
	_, cmd = lm.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	is.Nil(cmd, "an available key has nothing to delete")

	lm, _ = lm.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // "zzz", unknown
	must.Equal("zzz", lm.SelectedItem().Key)
	_, cmd = lm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.Nil(cmd, "an unknown key has no schema to open")
}

// TestListFilterByTyping verifies the "/" filter narrows the list as the user
// types, and that enter on the match opens it.
func TestListFilterByTyping(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	lm := New([]string{"alpha", "beta", "gamma"}, nil, nil, 10, th)
	must.False(lm.IsFiltering(), "should not start in filtering mode")
	lm, _ = lm.Update(bezeltest.Key("/"))
	must.True(lm.IsFiltering(), `"/" should enter filtering mode`)
	for _, r := range "be" {
		lm, _ = lm.Update(bezeltest.Key(string(r)))
	}
	is.Equal("be", lm.Filter())
	must.NotNil(lm.SelectedItem())
	is.Equal("beta", lm.SelectedItem().Key, `filter "be" should match beta`)

	lm, cmd := lm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	is.False(lm.IsFiltering())
	must.NotNil(cmd)
	is.Equal("beta", cmd().(OpenItemMsg).Item.Key)
}

// TestRebuildKeepsTheCursorAndCounts pins the counter and cursor after a
// document change.
func TestRebuildKeepsTheCursorAndCounts(t *testing.T) {
	is := assert.New(t)
	lm := New([]string{"a", "b", "c"}, []document.Block{{Key: "a"}}, nil, 10, th)
	lm, _ = lm.Update(tea.KeyPressMsg{Code: tea.KeyDown}) // "b", available
	is.Equal(1, lm.AddedCount())
	is.Equal(3, lm.KnownCount())

	lm = lm.Rebuild([]document.Block{{Key: "a"}, {Key: "b"}}, th)
	is.Equal(2, lm.AddedCount())
	is.Equal("b", lm.SelectedItem().Key, "cursor follows the key across the rebuild")
	is.Contains(ansi.Strip(lm.View(th)), "▶ ●  b")
	is.Equal(Item{Key: "c"}, lm.ItemByKey("c"))
	is.Equal(Item{Key: "ADDED"}, lm.ItemByKey("ADDED"), "a heading is not an item")
}
