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

// openChildMsg requests drilling into a nested field, pushing a new block editor
// scoped to that field onto the stack. relSegs is the focus-path suffix from the
// parent editor's focus to the drilled-into node (e.g. [segIdx(2), segKey("any")]
// for the "any" field of the parent collection's current item). The model
// resolves the actual content from editRoot at the resulting focus path.
type openChildMsg struct {
	key     string
	defs    []schema.FieldDef
	kind    schema.Kind
	relSegs []pathSeg
}

// commitRequestedMsg is emitted by the block editor (Ctrl+S) to ask the model
// to commit the editor stack into the document. The block layer has no model
// access, so it requests the commit as a message the root Update handles.
type commitRequestedMsg struct{}

// drillOutMsg is sent when the user presses Esc inside a nested editor. Unlike
// blockEditDiscardedMsg (which abandons the whole block edit), it navigates up
// one level while KEEPING edits: the current level is flushed into the canonical
// editRoot, popped, and the parent editor is refreshed to reflect the change.
type drillOutMsg struct{}

// blockEditDiscardedMsg is sent when the user closes a block edit (Esc).
// discarded is true only when uncommitted changes were intentionally thrown away
// (user confirmed the "Discard changes?" dialog). It is false when Esc is pressed
// on a clean editor (no uncommitted changes) - in that case the status message
// from the last commit should be preserved.
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

// validateRequestedMsg is emitted by the block editor (Ctrl+L) to ask the
// model to run the doc-level validation pass. The block layer has no model
// access, so it requests the action as a message the root Update handles.
type validateRequestedMsg struct{}

// clearStatusMsg is sent by a time.Tick to auto-clear the status bar. seq must
// match model.statusSeq; a newer message will have incremented it, so the old
// tick becomes a no-op.
type clearStatusMsg struct{ seq uint }
