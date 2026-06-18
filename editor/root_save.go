package editor

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/internal/alert"
	"github.com/lucasassuncao/yedit/schema"
)

func (m model) undo() (tea.Model, tea.Cmd) {
	var ok bool
	m.doc, ok = m.doc.Undo()
	if !ok {
		return m.withStatus("Nothing to undo.")
	}
	m = m.syncView()
	return m.withStatus("Undone.")
}

func (m model) redo() (tea.Model, tea.Cmd) {
	var ok bool
	m.doc, ok = m.doc.Redo()
	if !ok {
		return m.withStatus("Nothing to redo.")
	}
	m = m.syncView()
	return m.withStatus("Redone.")
}

const statusMsgDuration = 4 * time.Second

// withStatus sets statusMsg and schedules a tick to clear it after statusMsgDuration.
// The tick carries the current statusSeq; if a newer message has been set by the
// time the tick fires, the seq will not match and the clear is a no-op.
func (m model) withStatus(msg string) (model, tea.Cmd) {
	m.statusSeq++
	m.statusMsg = msg
	seq := m.statusSeq
	return m, tea.Tick(statusMsgDuration, func(time.Time) tea.Msg {
		return clearStatusMsg{seq: seq}
	})
}

func (m model) collectErrors() []Violation {
	var errs []Violation
	if u := schema.UnknownKeys(m.doc.Raw(), m.knownByPath); len(u) > 0 {
		var filtered []string
		for _, k := range u {
			if !m.list.passthrough[k] {
				filtered = append(filtered, k)
			}
		}
		if len(filtered) > 0 {
			errs = append(errs, Violation{Message: "Unknown key(s): " + strings.Join(filtered, ", ")})
		}
	}
	errs = append(errs, RunAll(m.cfg.Validators, m.doc.Raw(), m.doc.Blocks())...)
	return errs
}

// formatErrors formats errs into a bullet list. maxLines caps the total
// rendered line count; when exceeded the remaining count is appended as a
// summary line. Pass 0 to disable the cap.
func formatErrors(errs []Violation, maxLines int) string {
	var sb strings.Builder
	usedLines := 0
	for i, e := range errs {
		msg := "• " + e.String()
		cost := strings.Count(msg, "\n") + 1
		if i > 0 {
			cost += 2 // blank separator
		}
		if maxLines > 0 && usedLines+cost > maxLines {
			remaining := len(errs) - i
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(fmt.Sprintf("... and %d more error(s).", remaining))
			break
		}
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(msg)
		usedLines += cost
	}
	return sb.String()
}

func (m model) save() (tea.Model, tea.Cmd) {
	errs := m.collectErrors()
	maxLines := m.height - 12 // reserve space for box border, padding, title, button
	if maxLines < 6 {
		maxLines = 6
	}
	if len(errs) > 0 && !m.cfg.NoValidateOnSave {
		return m.showAlert("Cannot save - fix errors first", formatErrors(errs, maxLines), alert.KindError)
	}
	doSave := func() tea.Msg { return doSaveMsg{} }
	// An external edit since open is a substantive data-loss risk - always confirm
	// before clobbering it, even under NoSaveConfirm.
	if m.doc.ExternallyChanged() {
		msg := fmt.Sprintf("%s changed on disk since you opened it.\nSaving overwrites those external changes.", m.doc.Path())
		return m.showConfirmAlert("File changed on disk - overwrite?", msg, doSave)
	}
	if len(errs) > 0 {
		// NoValidateOnSave: always confirm - warning is substantive, not routine.
		msg := fmt.Sprintf("Save to %s?\n\nWarnings:\n%s", m.doc.Path(), formatErrors(errs, maxLines))
		return m.showConfirmAlert("Save with warnings?", msg, doSave)
	}
	if m.cfg.NoSaveConfirm {
		return m, doSave
	}
	return m.showConfirmAlert("Save changes?", fmt.Sprintf("Save to %s?", m.doc.Path()), doSave)
}

type doSaveMsg struct{}

func cmdSave(doc document.Document) tea.Cmd {
	return func() tea.Msg {
		saved, err := doc.Save()
		return saveResultMsg{doc: saved, err: err}
	}
}

func cmdReload(doc document.Document) tea.Cmd {
	return func() tea.Msg {
		reloaded, err := doc.Reload()
		return reloadResultMsg{doc: reloaded, err: err}
	}
}

func (m model) execSave() (tea.Model, tea.Cmd) {
	return m, cmdSave(m.doc)
}

// reload re-reads the file from disk, discarding local edits. Unsaved changes
// are a substantive loss, so they require confirmation; a clean document
// reloads immediately.
func (m model) reload() (tea.Model, tea.Cmd) {
	if m.doc.Dirty() {
		msg := fmt.Sprintf("Re-read %s from disk?\nUnsaved changes will be lost.", m.doc.Path())
		return m.showConfirmAlert("Reload from disk?", msg, func() tea.Msg { return confirmedReloadMsg{} })
	}
	return m.execReload()
}

func (m model) execReload() (tea.Model, tea.Cmd) {
	return m, cmdReload(m.doc)
}

func (m model) validateKeys() (tea.Model, tea.Cmd) {
	maxLines := m.height - 12
	if maxLines < 6 {
		maxLines = 6
	}
	if errs := m.collectErrors(); len(errs) > 0 {
		return m.showAlert("Validation errors", formatErrors(errs, maxLines), alert.KindError)
	}
	return m.showAlert("Validation passed", "All keys are valid and no conflicts were found.", alert.KindSuccess)
}
