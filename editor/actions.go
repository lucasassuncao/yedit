package editor

import (
	"fmt"
	"strings"

	"github.com/lucasassuncao/yedit/schema"
)

// BlockAction is a pure synchronous mutation of blockEditState.
// All block-editor mutations pass through blockEditState.dispatch.
type BlockAction interface{ blockAction() }

// ModelAction is handled by model.dispatch.
// May produce tea.Cmd only for tea.Quit.
type ModelAction interface{ modelAction() }

// EditorAction is a discriminated union returned by blockKeymap.
// Exactly one field is non-nil.
type EditorAction struct {
	Block BlockAction
	Model ModelAction
}

// ── BlockAction types ────────────────────────────────────────────────────────

// ToggleField checks or unchecks the field at NodeIdx in the tree.
type ToggleField struct {
	NodeIdx int
	Checked bool
}

// SyncYAML advances be.node from new YAML content (parse-gated).
type SyncYAML struct{ Content string }

// AddEntry appends a new entry to a collection-nav block.
type AddEntry struct{}

// DeleteEntry removes the collection entry at SeqIdx.
type DeleteEntry struct{ SeqIdx int }

// NavigateEntry moves the collection cursor to Idx (flush + load).
type NavigateEntry struct{ Idx int }

// ApplyPreset replaces the block content with the named preset.
// Content is the already-fetched YAML so dispatch stays pure.
type ApplyPreset struct{ Name, Content string }

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
func (Undo) blockAction()          {}
func (Redo) blockAction()          {}

// ── ModelAction types ────────────────────────────────────────────────────────

type OpenBlock struct{ Key string }
type CommitBlock struct{}
type DiscardBlock struct{}
type DeleteBlock struct{ Key string }
type DrillIn struct {
	Key     string
	Defs    []schema.FieldDef
	Kind    schema.Kind
	RelSegs []pathSeg
}
type DrillOut struct{}
type DocUndo struct{}
type DocRedo struct{}
type Save struct{}
type Reload struct{}
type ToggleHints struct{}

func (OpenBlock) modelAction()    {}
func (CommitBlock) modelAction()  {}
func (DiscardBlock) modelAction() {}
func (DeleteBlock) modelAction()  {}
func (DrillIn) modelAction()      {}
func (DrillOut) modelAction()     {}
func (DocUndo) modelAction()      {}
func (DocRedo) modelAction()      {}
func (Save) modelAction()         {}
func (Reload) modelAction()       {}
func (ToggleHints) modelAction()  {}

// DumpBlockActionLog returns a Go-syntax representation of the action log,
// suitable for pasting into a test or bug report.
func DumpBlockActionLog(log []BlockAction) string {
	if len(log) == 0 {
		return "[]BlockAction{}"
	}
	var b strings.Builder
	b.WriteString("[]BlockAction{\n")
	for _, a := range log {
		switch act := a.(type) {
		case ToggleField:
			fmt.Fprintf(&b, "\tToggleField{NodeIdx: %d, Checked: %v},\n", act.NodeIdx, act.Checked)
		case SyncYAML:
			fmt.Fprintf(&b, "\tSyncYAML{Content: %q},\n", act.Content)
		case AddEntry:
			b.WriteString("\tAddEntry{},\n")
		case DeleteEntry:
			fmt.Fprintf(&b, "\tDeleteEntry{SeqIdx: %d},\n", act.SeqIdx)
		case NavigateEntry:
			fmt.Fprintf(&b, "\tNavigateEntry{Idx: %d},\n", act.Idx)
		case ApplyPreset:
			fmt.Fprintf(&b, "\tApplyPreset{Name: %q, Content: %q},\n", act.Name, act.Content)
		case Undo:
			b.WriteString("\tUndo{},\n")
		case Redo:
			b.WriteString("\tRedo{},\n")
		}
	}
	b.WriteString("}")
	return b.String()
}
