package editor

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/shell"
	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/render"
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

// withStatus shows transient feedback on the status row; the shell clears it
// after statusMsgDuration unless a newer message replaced it first.
func (m model) withStatus(msg string) (model, tea.Cmd) {
	var cmd tea.Cmd
	m.sh, cmd = m.sh.SetStatus(msg, shell.Info, statusMsgDuration)
	return m, cmd
}

// withStickyError sets an error status that persists until the next status
// change - for errors the user must not miss. Only for the root status row;
// while a block editor is open use withTopBEError.
func (m model) withStickyError(msg string) model {
	m.sh, _ = m.sh.SetStatus(msg, shell.Error, 0)
	return m
}

// collectErrors runs the unknown-key check and all wired validators against
// doc. Callers pass m.doc, or a throwaway copy carrying uncommitted editor
// content (see validateKeys).
func (m model) collectErrors(doc document.Document) []Violation {
	var errs []Violation
	u, err := schema.UnknownKeys(doc.Raw(), m.knownByPath)
	if err != nil {
		errs = append(errs, Violation{Message: fmt.Sprintf("Unknown keys check failed: %v", err), Group: GroupRules})
	}
	if len(u) > 0 {
		var filtered []string
		for _, k := range u {
			if !m.list.IsPassthrough(k) {
				filtered = append(filtered, k)
			}
		}
		for _, k := range filtered {
			errs = append(errs, Violation{Path: k, Group: GroupUnknownKeys})
		}
	}
	for _, v := range RunAll(m.wiredValidators, doc.Raw(), doc.Blocks()) {
		if v.Group == "" {
			v.Group = GroupRules
		}
		errs = append(errs, v)
	}
	return errs
}

// save validates and confirms before writing. then names a SaveAction to run
// once the write succeeds; it travels in the messages, so a cancelled confirm or
// a failed validation leaves nothing pending.
func (m model) save(then string) (tea.Model, tea.Cmd) {
	errs := m.collectErrors(m.doc)
	maxLines := m.height - 12 // reserve space for box border, padding, title, button
	if maxLines < 6 {
		maxLines = 6
	}
	if len(errs) > 0 && !m.cfg.NoValidateOnSave {
		return m.showAlert("Cannot save - fix errors first", render.Violations(errs, maxLines), overlay.Danger)
	}
	doSave := func() tea.Msg { return doSaveMsg{then: then} }
	// An external edit since open is a substantive data-loss risk - always confirm
	// before clobbering it, even under NoSaveConfirm.
	if m.doc.ExternallyChanged() {
		msg := fmt.Sprintf("%s changed on disk since you opened it.\nSaving overwrites those external changes.", m.doc.Path())
		return m.showConfirmAlert("File changed on disk - overwrite?", msg, doSave)
	}
	if len(errs) > 0 {
		// NoValidateOnSave: always confirm - warning is substantive, not routine.
		msg := fmt.Sprintf("Save to %s?\n\nWarnings:\n%s", m.doc.Path(), render.Violations(errs, maxLines))
		return m.showConfirmAlert("Save with warnings?", msg, doSave)
	}
	if m.cfg.NoSaveConfirm {
		return m, doSave
	}
	return m.showConfirmAlert("Save changes?", fmt.Sprintf("Save to %s?", m.doc.Path()), doSave)
}

type doSaveMsg struct{ then string }

func cmdSave(doc document.Document, then string) tea.Cmd {
	return func() tea.Msg {
		saved, err := doc.Save()
		return saveResultMsg{doc: saved, err: err, then: then}
	}
}

// action is the Config.Actions entry bound to key.
func (m model) action(key string) (Action, bool) {
	for _, a := range m.cfg.Actions {
		if a.Key == key {
			return a, true
		}
	}
	return Action{}, false
}

// runAppAction starts the Action bound to key, saving first when it asks to. A
// clean document already written to its destination is not saved again.
func (m model) runAppAction(key string) (tea.Model, tea.Cmd) {
	a, ok := m.action(key)
	if !ok || len(m.blockEdits) > 0 {
		return m, nil
	}
	if m.sh.IsBusy() {
		return m.withStatus(a.Help + ": another action is still running.")
	}
	written := m.cfg.SavePath == "" || m.saved
	if !a.SaveFirst || (!m.doc.Dirty() && written && !m.doc.ExternallyChanged()) {
		return m.startAction(a)
	}
	return m.save(key)
}

// startAction hands the document to a: Exec gets the terminal, Run runs in the
// background under the shell's spinner.
func (m model) startAction(a Action) (tea.Model, tea.Cmd) {
	ctx := ActionContext{Path: m.doc.Path(), Raw: append([]byte(nil), m.doc.Raw()...)}
	if a.Exec != nil {
		c := a.Exec(ctx)
		if c == nil {
			return m, nil
		}
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return actionExecDoneMsg{help: a.Help, err: err} })
	}
	if a.Run == nil {
		return m, nil
	}
	var spin tea.Cmd
	m.sh, spin = m.sh.Busy(a.Help + "…")
	run := func() tea.Msg {
		res, err := a.Run(ctx)
		return actionResultMsg{help: a.Help, res: res, err: err}
	}
	return m, tea.Batch(spin, run)
}

func (m model) handleActionResult(msg actionResultMsg) (tea.Model, tea.Cmd) {
	m.sh = m.sh.Idle()
	title, kind, head := msg.help, overlay.Success, msg.res.Message
	if msg.err != nil {
		title, kind, head = msg.help+" failed", overlay.Danger, msg.err.Error()
	}
	if msg.res.Output == "" {
		return m.showAlert(title, head, kind)
	}
	text := msg.res.Output
	if head != "" {
		text = head + "\n\n" + text
	}
	m = m.enterAlert(overlay.NewPager(title, text, m.theme.ModalFor(kind), m.theme.Legend))
	return m, nil
}

// handleActionExecDone reports a failed program, and reloads a document the
// program changed on disk unless that would throw away unsaved edits.
func (m model) handleActionExecDone(msg actionExecDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.showAlert(msg.help+" failed", msg.err.Error(), overlay.Danger)
	}
	if !m.doc.ExternallyChanged() {
		return m, nil
	}
	if m.doc.Dirty() {
		return m.withStatus(m.doc.Path() + " changed on disk - ctrl+r reloads it, discarding your edits.")
	}
	return m.execReload()
}

func cmdReload(doc document.Document) tea.Cmd {
	return func() tea.Msg {
		reloaded, err := doc.Reload()
		return reloadResultMsg{doc: reloaded, err: err}
	}
}

func (m model) execSave(then string) (tea.Model, tea.Cmd) {
	return m, cmdSave(m.doc, then)
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

// validateKeys runs the document-level validation pass (ctrl+l). When a block
// editor stack is open, the current (uncommitted) editor content is applied to
// a throwaway copy of the document first, so validation reflects what is on
// screen instead of the last committed state.
func (m model) validateKeys() (tea.Model, tea.Cmd) {
	doc := m.doc
	if len(m.blockEdits) > 0 {
		var ok bool
		if m, ok = m.flushTopToRoot(); !ok {
			// The editor content does not commit (parse/unknown-key error); the
			// editor's feedback line shows the detail. Validating the stale
			// document would only mislead, so stop here.
			return m, nil
		}
		var err error
		if doc, err = m.docWithEditorContent(); err != nil {
			return m.showAlert("Validation failed", fmt.Sprintf("Could not apply editor content: %v", err), overlay.Danger)
		}
	}
	maxLines := m.height - 12
	if maxLines < 6 {
		maxLines = 6
	}
	if errs := m.collectErrors(doc); len(errs) > 0 {
		return m.showAlert("Validation errors", render.Violations(errs, maxLines), overlay.Danger)
	}
	return m.showAlert("Validation passed", "All keys are valid and no conflicts were found.", overlay.Success)
}
