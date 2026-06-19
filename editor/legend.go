package editor

import "github.com/charmbracelet/bubbles/key"

// msgUncommittedChanges is shown in the feedback line when there are uncommitted changes.
const msgUncommittedChanges = "Uncommitted changes - ctrl+s to commit"

// Key bindings — one var per distinct key/description pair.
var (
	kbNav      = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "nav"))
	kbNavigate = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "navigate"))
	kbScroll   = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "scroll"))
	kbExpand   = key.NewBinding(key.WithKeys("right", "left"), key.WithHelp("→/←", "expand"))

	kbTab        = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "change pane"))
	kbTabPreview = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "preview"))
	kbTabPresets = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "presets"))
	kbTabEscList = key.NewBinding(key.WithKeys("tab", "esc"), key.WithHelp("tab/esc", "back to list"))

	kbCtrlSSave   = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save"))
	kbCtrlSSaveCh = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save changes"))
	kbCtrlDDelete = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete"))
	kbCtrlDRemove = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "remove"))
	kbCtrlUUndo   = key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "undo"))
	kbCtrlYRedo   = key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "redo"))
	kbCtrlRReload = key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reload"))
	kbCtrlLValid  = key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "validate"))

	kbEscBack   = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	kbEscCancel = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	kbEscClear  = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear"))

	kbEnterAdd     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add"))
	kbEnterApply   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply"))
	kbEnterOpen    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	kbEnterReplace = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "replace"))
	kbEnterSelect  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select"))

	kbAAppend = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "append"))

	kbFilter     = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	kbTypeFilter = key.NewBinding(key.WithHelp("type", "filter"))
	kbPreset     = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "preset"))

	kbHint     = key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hint"))
	kbHintHide = key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide hint"))
)

// KeyMap types implement help.KeyMap (short mode only — FullHelp is unused).

// dynamicKeyMap is used for modes whose binding list varies at runtime.
type dynamicKeyMap []key.Binding

func (d dynamicKeyMap) ShortHelp() []key.Binding  { return []key.Binding(d) }
func (d dynamicKeyMap) FullHelp() [][]key.Binding { return nil }

type saveTailMap struct{}

func (saveTailMap) ShortHelp() []key.Binding {
	return []key.Binding{kbTab, kbCtrlUUndo, kbCtrlYRedo, kbCtrlSSaveCh, kbEscBack}
}
func (saveTailMap) FullHelp() [][]key.Binding { return nil }

type listPreviewMap struct{}

func (listPreviewMap) ShortHelp() []key.Binding {
	return []key.Binding{kbScroll, kbTabEscList}
}
func (listPreviewMap) FullHelp() [][]key.Binding { return nil }

type listFilteringMap struct{}

func (listFilteringMap) ShortHelp() []key.Binding {
	return []key.Binding{kbTypeFilter, kbNavigate, kbEnterSelect, kbEscClear}
}
func (listFilteringMap) FullHelp() [][]key.Binding { return nil }

type listUnknownMap struct{ hint key.Binding }

func (k listUnknownMap) ShortHelp() []key.Binding {
	return []key.Binding{kbNav, kbFilter, kbCtrlDDelete, kbCtrlUUndo, kbCtrlYRedo, kbCtrlRReload, kbCtrlSSave, kbCtrlLValid, k.hint}
}
func (k listUnknownMap) FullHelp() [][]key.Binding { return nil }

type listExistingMap struct{ hint key.Binding }

func (k listExistingMap) ShortHelp() []key.Binding {
	return []key.Binding{kbNav, kbFilter, kbEnterOpen, kbCtrlDDelete, kbCtrlUUndo, kbCtrlYRedo, kbCtrlRReload, kbCtrlSSave, kbCtrlLValid, k.hint}
}
func (k listExistingMap) FullHelp() [][]key.Binding { return nil }

type listNewMap struct{ hint key.Binding }

func (k listNewMap) ShortHelp() []key.Binding {
	return []key.Binding{kbNav, kbFilter, kbEnterAdd, kbCtrlUUndo, kbCtrlYRedo, kbCtrlRReload, kbCtrlSSave, kbCtrlLValid, k.hint}
}
func (k listNewMap) FullHelp() [][]key.Binding { return nil }

type presetPreviewMap struct{}

func (presetPreviewMap) ShortHelp() []key.Binding {
	return []key.Binding{kbScroll, kbTabPresets, kbEscBack}
}
func (presetPreviewMap) FullHelp() [][]key.Binding { return nil }

type presetListScalarMap struct{}

func (presetListScalarMap) ShortHelp() []key.Binding {
	return []key.Binding{kbNavigate, kbTabPreview, kbEnterApply, kbEscCancel}
}
func (presetListScalarMap) FullHelp() [][]key.Binding { return nil }

type presetListCollectionMap struct{}

func (presetListCollectionMap) ShortHelp() []key.Binding {
	return []key.Binding{kbNavigate, kbTabPreview, kbEnterReplace, kbAAppend, kbEscCancel}
}
func (presetListCollectionMap) FullHelp() [][]key.Binding { return nil }
