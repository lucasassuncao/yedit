// Package keys is the bindings the editor's components match against: the
// list, the field tree and the preset browser handle these keys themselves.
// What the legend prints, and every model-level key, is a bezel action
// declared per screen in editor/keyactions.go.
package keys

import "charm.land/bubbles/v2/key"

// Physical keys, each declared exactly once.
const (
	keyUp    = "up"
	keyDown  = "down"
	keyLeft  = "left"
	keyRight = "right"
	keyEnter = "enter"
	keyEsc   = "esc"
	keySlash = "/"
	keyA     = "a"
	keyCtrlS = "ctrl+s"
	keyCtrlL = "ctrl+l"
	keyCtrlD = "ctrl+d"
	keyCtrlC = "ctrl+c"
)

var (
	Up        = key.NewBinding(key.WithKeys(keyUp))
	Down      = key.NewBinding(key.WithKeys(keyDown))
	Left      = key.NewBinding(key.WithKeys(keyLeft))
	Right     = key.NewBinding(key.WithKeys(keyRight))
	Enter     = key.NewBinding(key.WithKeys(keyEnter))
	Esc       = key.NewBinding(key.WithKeys(keyEsc))
	CtrlCQuit = key.NewBinding(key.WithKeys(keyCtrlC))

	CtrlSSave   = key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save"))
	CtrlSSaveCh = key.NewBinding(key.WithKeys(keyCtrlS), key.WithHelp("ctrl+s", "save changes"))
	CtrlDDelete = key.NewBinding(key.WithKeys(keyCtrlD), key.WithHelp("ctrl+d", "delete"))
	CtrlDRemove = key.NewBinding(key.WithKeys(keyCtrlD), key.WithHelp("ctrl+d", "remove"))
	CtrlLValid  = key.NewBinding(key.WithKeys(keyCtrlL), key.WithHelp("ctrl+l", "validate"))

	AAppend = key.NewBinding(key.WithKeys(keyA), key.WithHelp("a", "append"))
	Filter  = key.NewBinding(key.WithKeys(keySlash), key.WithHelp("/", "filter"))
)
