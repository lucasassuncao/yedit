package editor

import (
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
)

// BlockAction is a pure synchronous mutation of blockEditState. Every
// block-editor mutation passes through blockEditState.dispatch.
type BlockAction interface{ blockAction() }

// ModelAction is handled by model.dispatch and may produce a tea.Cmd only for
// tea.Quit.
type ModelAction interface{ modelAction() }

// ToggleField checks or unchecks the field at NodeIdx in the tree.
type ToggleField struct {
	NodeIdx int
	Checked bool
}

// SyncYAML advances be.node from new YAML content (parse-gated). Checkpoint
// saves an undo snapshot first: set it for pastes, not for single keystrokes.
type SyncYAML struct {
	Content    string
	Checkpoint bool
}

// AddEntry appends a new entry to a collection-nav block.
type AddEntry struct{}

// DeleteEntry removes the collection entry at SeqIdx.
type DeleteEntry struct{ SeqIdx int }

// NavigateEntry moves the collection cursor to Idx (flush + load).
type NavigateEntry struct{ Idx int }

// ApplyPreset replaces the block content with the named preset. Content is the
// already-fetched YAML so dispatch stays pure.
type ApplyPreset struct{ Name, Content string }

// AppendPreset appends preset entries to a collection-nav block. Content is the
// already-fetched YAML so dispatch stays pure.
type AppendPreset struct{ Name, Content string }

// Undo restores the previous block snapshot.
type Undo struct{}

// Redo re-applies the most recently undone block snapshot.
type Redo struct{}

func (ToggleField) blockAction()   {}
func (SyncYAML) blockAction()      {}
func (AddEntry) blockAction()      {}
func (DeleteEntry) blockAction()   {}
func (NavigateEntry) blockAction() {}
func (ApplyPreset) blockAction()   {}
func (AppendPreset) blockAction()  {}
func (Undo) blockAction()          {}
func (Redo) blockAction()          {}

type OpenBlock struct{ Key string }
type CommitBlock struct{}
type DeleteBlock struct{ Key string }
type DrillIn struct {
	Key     string
	Defs    []schema.FieldDef
	Kind    schema.Kind
	RelSegs []yamledit.PathSeg
}
type DrillOut struct{}
type DocUndo struct{}
type DocRedo struct{}
type Save struct{}
type Reload struct{}
type ToggleHints struct{}
type ApplyDocPreset struct{ Name, Content string }

func (OpenBlock) modelAction()      {}
func (CommitBlock) modelAction()    {}
func (DeleteBlock) modelAction()    {}
func (DrillIn) modelAction()        {}
func (DrillOut) modelAction()       {}
func (DocUndo) modelAction()        {}
func (DocRedo) modelAction()        {}
func (Save) modelAction()           {}
func (Reload) modelAction()         {}
func (ToggleHints) modelAction()    {}
func (ApplyDocPreset) modelAction() {}
