package editor

import "github.com/charmbracelet/bubbles/key"

// msgUncommittedChanges is shown in the feedback line when there are uncommitted changes.
const msgUncommittedChanges = "Uncommitted changes - ctrl+s to commit"

// Physical keys - each key name is declared exactly once. The bindings below
// derive from these and the handlers match against the bindings (key.Matches),
// so rebinding a key here changes the behavior and the legend together.
const (
	keyUp    = "up"
	keyDown  = "down"
	keyLeft  = "left"
	keyRight = "right"
	keyEnter = "enter"
	keyEsc   = "esc"
	keyTab   = "tab"
	keySlash = "/"
	keyP     = "p"
	keyH     = "h"
	keyA     = "a"
	keyCtrlS = "ctrl+s"
	keyCtrlL = "ctrl+l"
	keyCtrlD = "ctrl+d"
	keyCtrlU = "ctrl+u"
	keyCtrlY = "ctrl+y"
	keyCtrlR = "ctrl+r"
	keyCtrlH = "ctrl+h"
	keyCtrlC = "ctrl+c"
)

// Matcher-only bindings, for keys whose display wording varies by mode (the
// kbEnter*/kbEsc*/directional variants below) or that carry no legend entry
// (quit, hint focus). Handlers match against these; the display variants share
// the same key constants, so behavior and legend cannot drift apart.
var (
	kbUp        = key.NewBinding(key.WithKeys(keyUp))
	kbDown      = key.NewBinding(key.WithKeys(keyDown))
	kbLeft      = key.NewBinding(key.WithKeys(keyLeft))
	kbRight     = key.NewBinding(key.WithKeys(keyRight))
	kbEnter     = key.NewBinding(key.WithKeys(keyEnter))
	kbEsc       = key.NewBinding(key.WithKeys(keyEsc))
	kbCtrlCQuit = key.NewBinding(key.WithKeys(keyCtrlC))
	kbCtrlHHint = key.NewBinding(key.WithKeys(keyCtrlH))
)

// Key bindings — one var per distinct key/description pair.
var (
	kbNav      = key.NewBinding(key.WithKeys(keyUp, keyDown), key.WithHelp("↑/↓", "nav"))
	kbNavigate = key.NewBinding(key.WithKeys(keyUp, keyDown), key.WithHelp("↑/↓", "navigate"))
	kbScroll   = key.NewBinding(key.WithKeys(keyUp, keyDown), key.WithHelp("↑/↓", "scroll"))
	kbExpand   = key.NewBinding(key.WithKeys(keyRight, keyLeft), key.WithHelp("→/←", "expand"))

	kbTab        = key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "change pane"))
	kbTabPreview = key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "preview"))
	kbTabPresets = key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "presets"))
	kbTabEscList = key.NewBinding(key.WithKeys(keyTab, keyEsc), key.WithHelp("tab/esc", "back to list"))

	kbCtrlSSave   = key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save"))
	kbCtrlSSaveCh = key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save changes"))
	kbCtrlDDelete = key.NewBinding(key.WithKeys(keyCtrlD), key.WithHelp("ctrl+d", "delete"))
	kbCtrlDRemove = key.NewBinding(key.WithKeys(keyCtrlD), key.WithHelp("ctrl+d", "remove"))
	kbCtrlUUndo   = key.NewBinding(key.WithKeys(keyCtrlU), key.WithHelp("ctrl+u", "undo"))
	kbCtrlYRedo   = key.NewBinding(key.WithKeys(keyCtrlY), key.WithHelp("ctrl+y", "redo"))
	kbCtrlRReload = key.NewBinding(key.WithKeys(keyCtrlR), key.WithHelp("ctrl+r", "reload"))
	kbCtrlLValid  = key.NewBinding(key.WithKeys(keyCtrlL), key.WithHelp("ctrl+l", "validate"))

	kbEscBack   = key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "back"))
	kbEscCancel = key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "cancel"))
	kbEscClear  = key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "clear"))

	kbEnterAdd     = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "add"))
	kbEnterApply   = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "apply"))
	kbEnterOpen    = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "open"))
	kbEnterReplace = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "replace"))
	kbEnterSelect  = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "select"))

	kbAAppend = key.NewBinding(key.WithKeys(keyA), key.WithHelp("a", "append"))

	kbFilter     = key.NewBinding(key.WithKeys(keySlash), key.WithHelp("/", "filter"))
	kbTypeFilter = key.NewBinding(key.WithHelp("type", "filter"))
	kbPreset     = key.NewBinding(key.WithKeys(keyP), key.WithHelp("p", "preset"))

	kbHint     = key.NewBinding(key.WithKeys(keyH), key.WithHelp("h", "hint"))
	kbHintHide = key.NewBinding(key.WithKeys(keyH), key.WithHelp("h", "hide hint"))

	kbTemplates = key.NewBinding(key.WithKeys(keyP), key.WithHelp("p", "templates"))
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

type docPresetListKeyMap struct{}

func (docPresetListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{kbNavigate, kbTabPreview, kbEnterApply, kbEscCancel}
}
func (docPresetListKeyMap) FullHelp() [][]key.Binding { return nil }

type docPresetPreviewKeyMap struct{}

func (docPresetPreviewKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{kbScroll, kbTabPresets, kbEscBack}
}
func (docPresetPreviewKeyMap) FullHelp() [][]key.Binding { return nil }
