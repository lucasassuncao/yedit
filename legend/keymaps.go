// Package legend renders the editor's bottom bar: the key legend and the
// feedback line above it. It also declares the KeyMaps the legend draws, each
// one a fixed grouping of bindings from yedit/keys.
package legend

import (
	"charm.land/bubbles/v2/key"

	"github.com/lucasassuncao/yedit/keys"
)

// KeyMap types implement help.KeyMap in short mode only; FullHelp is unused.

// Dynamic serves modes whose binding list varies at runtime.
type Dynamic []key.Binding

func (d Dynamic) ShortHelp() []key.Binding  { return []key.Binding(d) }
func (d Dynamic) FullHelp() [][]key.Binding { return nil }

// DynamicRows is the row-grouped counterpart of Dynamic, for when a
// RowKeyMap's bindings vary at runtime (the root list inserts keys.Templates only
// when Config.DocPresets is set). ShortHelp flattens the rows for callers that
// just need the binding list; Rows keeps the grouping for legend rendering.
type DynamicRows [][]key.Binding

func (d DynamicRows) ShortHelp() []key.Binding {
	var out []key.Binding
	for _, row := range d {
		out = append(out, row...)
	}
	return out
}
func (d DynamicRows) FullHelp() [][]key.Binding { return nil }
func (d DynamicRows) Rows() [][]key.Binding     { return d }

type SaveTail struct{}

func (SaveTail) ShortHelp() []key.Binding {
	return []key.Binding{keys.Tab, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlSSaveCh, keys.EscBack}
}
func (SaveTail) FullHelp() [][]key.Binding { return nil }

// Rows: see ListExisting.Rows. tab/esc only move focus; undo/redo/save mutate
// or persist the block.
func (SaveTail) Rows() [][]key.Binding {
	return [][]key.Binding{
		{keys.Tab, keys.EscBack},
		{keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlSSaveCh},
	}
}

type ListPreview struct{}

func (ListPreview) ShortHelp() []key.Binding {
	return []key.Binding{keys.Scroll, keys.TabEscList}
}
func (ListPreview) FullHelp() [][]key.Binding { return nil }

type ListFiltering struct{}

func (ListFiltering) ShortHelp() []key.Binding {
	return []key.Binding{keys.TypeFilter, keys.Navigate, keys.EnterSelect, keys.EscClear}
}
func (ListFiltering) FullHelp() [][]key.Binding { return nil }

type ListUnknown struct{ Hint key.Binding }

func (k ListUnknown) ShortHelp() []key.Binding {
	return []key.Binding{keys.Nav, keys.Filter, keys.CtrlDDelete, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlRReload, keys.CtrlSSave, keys.CtrlLValid, k.Hint}
}
func (k ListUnknown) FullHelp() [][]key.Binding { return nil }

// Rows: see ListExisting.Rows.
func (k ListUnknown) Rows() [][]key.Binding {
	return [][]key.Binding{
		{keys.Nav, keys.Filter, k.Hint},
		{keys.CtrlSSave, keys.CtrlDDelete, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlRReload, keys.CtrlLValid},
	}
}

type ListExisting struct{ Hint key.Binding }

func (k ListExisting) ShortHelp() []key.Binding {
	return []key.Binding{keys.Nav, keys.Filter, keys.EnterOpen, keys.CtrlDDelete, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlRReload, keys.CtrlSSave, keys.CtrlLValid, k.Hint}
}
func (k ListExisting) FullHelp() [][]key.Binding { return nil }

// Rows splits the legend by whether a key can change the document: row 0 is
// navigation and inspection, row 1 mutation, persistence, and validation.
// Forcing the split instead of wrapping ShortHelp by width keeps the two
// categories on stable lines at any terminal width.
func (k ListExisting) Rows() [][]key.Binding {
	return [][]key.Binding{
		{keys.Nav, keys.EnterOpen, keys.Filter, k.Hint},
		{keys.CtrlSSave, keys.CtrlDDelete, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlRReload, keys.CtrlLValid},
	}
}

type ListNew struct{ Hint key.Binding }

func (k ListNew) ShortHelp() []key.Binding {
	return []key.Binding{keys.Nav, keys.Filter, keys.EnterAdd, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlRReload, keys.CtrlSSave, keys.CtrlLValid, k.Hint}
}
func (k ListNew) FullHelp() [][]key.Binding { return nil }

// Rows: see ListExisting.Rows.
func (k ListNew) Rows() [][]key.Binding {
	return [][]key.Binding{
		{keys.Nav, keys.EnterAdd, keys.Filter, k.Hint},
		{keys.CtrlSSave, keys.CtrlUUndo, keys.CtrlYRedo, keys.CtrlRReload, keys.CtrlLValid},
	}
}

type PresetPreview struct{}

func (PresetPreview) ShortHelp() []key.Binding {
	return []key.Binding{keys.Scroll, keys.TabPresets, keys.EscBack}
}
func (PresetPreview) FullHelp() [][]key.Binding { return nil }

type PresetListScalar struct{}

func (PresetListScalar) ShortHelp() []key.Binding {
	return []key.Binding{keys.Navigate, keys.TabPreview, keys.EnterApply, keys.EscCancel}
}
func (PresetListScalar) FullHelp() [][]key.Binding { return nil }

type PresetListCollection struct{}

func (PresetListCollection) ShortHelp() []key.Binding {
	return []key.Binding{keys.Navigate, keys.TabPreview, keys.EnterReplace, keys.AAppend, keys.EscCancel}
}
func (PresetListCollection) FullHelp() [][]key.Binding { return nil }

type DocPresetList struct{}

func (DocPresetList) ShortHelp() []key.Binding {
	return []key.Binding{keys.Navigate, keys.TabPreview, keys.EnterApply, keys.EscCancel}
}
func (DocPresetList) FullHelp() [][]key.Binding { return nil }

type DocPresetPreview struct{}

func (DocPresetPreview) ShortHelp() []key.Binding {
	return []key.Binding{keys.Scroll, keys.TabPresets, keys.EscBack}
}
func (DocPresetPreview) FullHelp() [][]key.Binding { return nil }
