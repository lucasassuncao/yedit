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
		for _, k := range filtered {
			errs = append(errs, Violation{Path: k, Group: GroupUnknownKeys})
		}
	}
	for _, v := range RunAll(m.wiredValidators, m.doc.Raw(), m.doc.Blocks()) {
		if v.Group == "" {
			v.Group = GroupRules
		}
		errs = append(errs, v)
	}
	return errs
}

// groupLoc returns the display location for a grouped Violation entry.
// Paths with dots or brackets are returned as-is; all others (including empty)
// are returned unchanged so the caller omits the location prefix when empty.
func groupLoc(path string) string {
	return path
}

type groupEntry struct{ path, msg string }

// rulesLines renders GroupRules entries as a tree: sections with ├/└ connectors.
func rulesLines(entries []groupEntry) []string {
	type sectionItem struct{ label, msg string }
	sections := make(map[string][]sectionItem)
	var sectionOrder []string
	sectionSeen := make(map[string]bool)

	for _, entry := range entries {
		sec, label := splitRulesPath(entry.path)
		if !sectionSeen[sec] {
			sectionSeen[sec] = true
			sectionOrder = append(sectionOrder, sec)
		}
		sections[sec] = append(sections[sec], sectionItem{label, entry.msg})
	}

	var lines []string
	for _, sec := range sectionOrder {
		items := sections[sec]
		lines = append(lines, "  - "+sec)
		maxW := 0
		for _, it := range items {
			if len(it.label) > maxW {
				maxW = len(it.label)
			}
		}
		for i, it := range items {
			conn := "├"
			if i == len(items)-1 {
				conn = "└"
			}
			if it.label == "" {
				lines = append(lines, fmt.Sprintf("    %s %s", conn, it.msg))
			} else {
				pad := strings.Repeat(" ", maxW-len(it.label))
				lines = append(lines, fmt.Sprintf("    %s %s%s  %s", conn, it.label, pad, it.msg))
			}
		}
	}
	return lines
}

// splitRulesPath splits a dotted/bracketed path into (section, fieldLabel).
// For unsplit paths the section is the path itself and fieldLabel is empty.
func splitRulesPath(path string) (sec, label string) {
	dot := strings.IndexByte(path, '.')
	bracket := strings.IndexByte(path, '[')
	split := dot
	if bracket >= 0 && (split < 0 || bracket < split) {
		split = bracket
	}
	switch {
	case split < 0:
		return path, ""
	case path[split] == '[':
		return path[:split], path[split:]
	default:
		return path[:split], path[split+1:]
	}
}

// formatErrors renders errs as a grouped list. Every violation must carry a
// Group (collectErrors guarantees this). GroupRules uses tree-style rendering
// (sections with ├/└ connectors); all other groups use a bullet list.
// maxLines caps the total output lines; excess is replaced with a summary line.
func formatErrors(errs []Violation, maxLines int) string {
	entries := make(map[group][]groupEntry)
	var groupOrder []group
	groupSeen := make(map[group]bool)

	for _, e := range errs {
		if !groupSeen[e.Group] {
			groupSeen[e.Group] = true
			groupOrder = append(groupOrder, e.Group)
		}
		entries[e.Group] = append(entries[e.Group], groupEntry{e.Path, e.Message})
	}

	var lines []string
	for i, grp := range groupOrder {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "• "+string(grp)+":")
		if grp == GroupRules {
			lines = append(lines, rulesLines(entries[grp])...)
		} else {
			for _, entry := range entries[grp] {
				loc := groupLoc(entry.path)
				switch {
				case entry.msg == "":
					lines = append(lines, "  - "+loc)
				case loc == "":
					lines = append(lines, "  - "+entry.msg)
				default:
					lines = append(lines, "  - "+loc+": "+entry.msg)
				}
			}
		}
	}

	if maxLines > 0 && len(lines) > maxLines {
		remaining := len(lines) - maxLines
		lines = append(lines[:maxLines], fmt.Sprintf("... and %d more error(s).", remaining))
	}
	return strings.Join(lines, "\n")
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
