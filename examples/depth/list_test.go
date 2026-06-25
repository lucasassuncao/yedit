package main

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/editor"
)

func TestListNoDuplicates(t *testing.T) {
	// write seed yaml to a temp file
	f, err := os.CreateTemp(t.TempDir(), "depth-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(seedYAML); err != nil {
		t.Fatal(err)
	}
	f.Close()

	m, err := editor.NewModelForTest(editor.Config{
		Path:        f.Name(),
		Schema:      &DepthConfig{},
		Metadata:    depthMetadata,
		EnableHints: true,
	})
	if err != nil {
		t.Fatalf("newModel: %v", err)
	}

	// set a reasonable terminal size
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m = updated

	// navigate down 4 times (alpha→beta→gamma→delta→epsilon)
	for i := 0; i < 4; i++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated
	}

	out := m.View()
	t.Logf("View:\n%s", out)
}
