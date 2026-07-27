package editor

import (
	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/schema"
)

// saveResultMsg carries the outcome of an async Save.
type saveResultMsg struct {
	doc document.Document
	err error
}

// reloadResultMsg carries the outcome of an async Reload.
type reloadResultMsg struct {
	doc document.Document
	err error
}

// openChildMsg pushes a new block editor scoped to a nested field. relSegs is
// the focus-path suffix from the parent editor to that node (e.g. [segIdx(2),
// segKey("any")]); the model resolves the content from editRoot at the resulting
// focus path.
type openChildMsg struct {
	key     string
	defs    []schema.FieldDef
	kind    schema.Kind
	relSegs []pathSeg
}

// commitRequestedMsg asks the model to commit the editor stack into the
// document. The block layer has no model access, so Ctrl+S requests it as a
// message the root Update handles.
type commitRequestedMsg struct{}

// drillOutMsg navigates up one level while keeping edits: the current level is
// flushed into editRoot, popped, and the parent refreshed. Unlike
// blockEditDiscardedMsg, which abandons the whole block edit.
type drillOutMsg struct{}

// blockEditDiscardedMsg is sent when Esc closes a block edit. discarded is true
// only when the user confirmed the "Discard changes?" dialog; on a clean editor
// it is false and the last commit's status message is preserved.
type blockEditDiscardedMsg struct{ discarded bool }

// pendingRemoveMsg is dispatched by the "Remove field?" confirm alert when the
// user chooses Yes. nodeIdx is the index into blockEditState.tree.nodes.
type pendingRemoveMsg struct{ nodeIdx int }

// pendingEntryDeleteMsg is dispatched by the "Remove entry?" confirm alert when
// the user confirms deleting a whole collection entry. seqIdx indexes the entry.
type pendingEntryDeleteMsg struct{ seqIdx int }

// confirmedDeleteMsg is dispatched by the "Remove block?" confirm alert when
// the user confirms deleting a top-level block from the main list.
type confirmedDeleteMsg struct{ Key string }

// confirmedReloadMsg is dispatched by the "Reload from disk?" confirm alert
// when the user confirms discarding local edits in favour of the on-disk file.
type confirmedReloadMsg struct{}

// confirmedDocPresetMsg is dispatched by the "Apply document preset?" confirm
// alert when the user confirms replacing the entire document with a preset.
type confirmedDocPresetMsg struct {
	Name    string
	Content string
}

// validateRequestedMsg asks the model to run the doc-level validation pass,
// mirroring commitRequestedMsg.
type validateRequestedMsg struct{}

// clearStatusMsg auto-clears the status bar. seq must match model.statusSeq: a
// newer message increments it, so the stale tick becomes a no-op.
type clearStatusMsg struct{ seq uint }
