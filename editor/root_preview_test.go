package editor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "charm.land/bubbletea/v2"
)

// followCfg is a flat schema used to exercise preview-follows-selection.
type followCfg struct {
	A string `yaml:"a"`
	B string `yaml:"b"`
	C string `yaml:"c"`
	D string `yaml:"d"`
	E string `yaml:"e"`
	F string `yaml:"f"`
}

// TestPreviewFollowsSelectedBlock verifies that navigating the ADDED list
// scrolls the read-only preview to the selected block's line.
func TestPreviewFollowsSelectedBlock(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "f.yaml")
	must.NoError(os.WriteFile(path, []byte(`a: 1
b: 2
c: 3
d: 4
e: 5
f: 6
`), 0o600))
	m, err := newModel(Config{Path: path, Schema: &followCfg{}})
	must.NoError(err, "newModel")
	// Short viewport so the 6-line document overflows and can actually scroll.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 8})
	m = updated.(model)
	must.Equal(0, m.preview.YOffset(), "initial YOffset should be 0")
	// Navigate down to "c" (third block, line 3).
	for i := 0; i < 2; i++ {
		updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = updated.(model)
	}
	it := m.list.SelectedItem()
	must.NotNil(it, "selected item should not be nil")
	must.Equal("c", it.Key, "expected selected item c")
	is.Equal(2, m.preview.YOffset(), "YOffset following \"c\" should be 2 (block at line 3)")
}
