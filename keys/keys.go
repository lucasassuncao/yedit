// Package keys is the editor's key-binding vocabulary: the physical keys and
// the bindings derived from them. Handlers match against these bindings and the
// legend renders their descriptions, so rebinding a key here changes behaviour
// and help text together.
package keys

import "charm.land/bubbles/v2/key"

// Physical keys, each declared exactly once. The bindings below derive from them
// and the handlers match against those bindings, so rebinding a key here changes
// the behavior and the legend together.
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

// Matcher-only bindings, for keys whose wording varies by mode or that carry no
// legend entry at all. The display variants share the same key constants, so
// behavior and legend cannot drift apart.
var (
	Up        = key.NewBinding(key.WithKeys(keyUp))
	Down      = key.NewBinding(key.WithKeys(keyDown))
	Left      = key.NewBinding(key.WithKeys(keyLeft))
	Right     = key.NewBinding(key.WithKeys(keyRight))
	Enter     = key.NewBinding(key.WithKeys(keyEnter))
	Esc       = key.NewBinding(key.WithKeys(keyEsc))
	CtrlCQuit = key.NewBinding(key.WithKeys(keyCtrlC))
	CtrlHHint = key.NewBinding(key.WithKeys(keyCtrlH))
)

// Key bindings — one var per distinct key/description pair.
var (
	Nav      = key.NewBinding(key.WithKeys(keyUp, keyDown), key.WithHelp("↑/↓", "nav"))
	Navigate = key.NewBinding(key.WithKeys(keyUp, keyDown), key.WithHelp("↑/↓", "navigate"))
	Scroll   = key.NewBinding(key.WithKeys(keyUp, keyDown), key.WithHelp("↑/↓", "scroll"))
	Expand   = key.NewBinding(key.WithKeys(keyRight, keyLeft), key.WithHelp("→/←", "expand"))

	Tab        = key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "change pane"))
	TabPreview = key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "preview"))
	TabPresets = key.NewBinding(key.WithKeys(keyTab), key.WithHelp("tab", "presets"))
	TabEscList = key.NewBinding(key.WithKeys(keyTab, keyEsc), key.WithHelp("tab/esc", "back to list"))

	CtrlSSave   = key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save"))
	CtrlSSaveCh = key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save changes"))
	CtrlDDelete = key.NewBinding(key.WithKeys(keyCtrlD), key.WithHelp("ctrl+d", "delete"))
	CtrlDRemove = key.NewBinding(key.WithKeys(keyCtrlD), key.WithHelp("ctrl+d", "remove"))
	CtrlUUndo   = key.NewBinding(key.WithKeys(keyCtrlU), key.WithHelp("ctrl+u", "undo"))
	CtrlYRedo   = key.NewBinding(key.WithKeys(keyCtrlY), key.WithHelp("ctrl+y", "redo"))
	CtrlRReload = key.NewBinding(key.WithKeys(keyCtrlR), key.WithHelp("ctrl+r", "reload"))
	CtrlLValid  = key.NewBinding(key.WithKeys(keyCtrlL), key.WithHelp("ctrl+l", "validate"))

	EscBack   = key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "back"))
	EscCancel = key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "cancel"))
	EscClear  = key.NewBinding(key.WithKeys(keyEsc), key.WithHelp("esc", "clear"))

	EnterAdd     = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "add"))
	EnterApply   = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "apply"))
	EnterOpen    = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "open"))
	EnterReplace = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "replace"))
	EnterSelect  = key.NewBinding(key.WithKeys(keyEnter), key.WithHelp("enter", "select"))

	AAppend = key.NewBinding(key.WithKeys(keyA), key.WithHelp("a", "append"))

	Filter     = key.NewBinding(key.WithKeys(keySlash), key.WithHelp("/", "filter"))
	TypeFilter = key.NewBinding(key.WithHelp("type", "filter"))
	Preset     = key.NewBinding(key.WithKeys(keyP), key.WithHelp("p", "preset"))

	Hint     = key.NewBinding(key.WithKeys(keyH), key.WithHelp("h", "show hint"))
	HintHide = key.NewBinding(key.WithKeys(keyH), key.WithHelp("h", "hide hint"))

	Templates = key.NewBinding(key.WithKeys(keyP), key.WithHelp("p", "templates"))
)
