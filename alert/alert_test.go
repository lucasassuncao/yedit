package alert

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type confirmedMsg struct{}

func keyY() tea.KeyMsg     { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}} }
func keyEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }

func TestConfirmNilCmdDismissesOnY(t *testing.T) {
	a := NewConfirm("Title", "Message", nil)
	_, cmd := a.Update(keyY())
	if cmd == nil {
		t.Fatal("Update(y) with nil confirmCmd returned nil cmd; modal would be stuck")
	}
	if _, ok := cmd().(DismissedMsg); !ok {
		t.Errorf("cmd() = %T, want DismissedMsg", cmd())
	}
}

func TestConfirmNilCmdDismissesOnEnter(t *testing.T) {
	a := NewConfirm("Title", "Message", nil)
	_, cmd := a.Update(keyEnter())
	if cmd == nil {
		t.Fatal("Update(enter) with nil confirmCmd returned nil cmd; modal would be stuck")
	}
	if _, ok := cmd().(DismissedMsg); !ok {
		t.Errorf("cmd() = %T, want DismissedMsg", cmd())
	}
}

func TestConfirmRunsProvidedCmd(t *testing.T) {
	a := NewConfirm("Title", "Message", func() tea.Msg { return confirmedMsg{} })
	_, cmd := a.Update(keyY())
	if cmd == nil {
		t.Fatal("Update(y) returned nil cmd, want the confirm cmd")
	}
	if _, ok := cmd().(confirmedMsg); !ok {
		t.Errorf("cmd() = %T, want confirmedMsg", cmd())
	}
}
