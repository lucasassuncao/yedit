// Package keys is the bindings the editor's components match against: the
// list, the field tree and the preset browser handle these keys themselves.
// What the legend prints, and every model-level key, is a bezel action
// declared per screen in editor/keyactions.go.
package keys

import "github.com/lucasassuncao/bezel/key"

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
	keyCtrlH = "ctrl+h"
)

var (
	Up        = key.New(keyUp)
	Down      = key.New(keyDown)
	Left      = key.New(keyLeft)
	Right     = key.New(keyRight)
	Enter     = key.New(keyEnter)
	Esc       = key.New(keyEsc)
	CtrlCQuit = key.New(keyCtrlC)

	CtrlSSave   = key.New(keyCtrlS).WithHelp("ctrl+s", "save")
	CtrlSSaveCh = key.New(keyCtrlS).WithHelp("ctrl+s", "save changes")
	CtrlDDelete = key.New(keyCtrlD).WithHelp("ctrl+d", "delete")
	CtrlDRemove = key.New(keyCtrlD).WithHelp("ctrl+d", "remove")
	CtrlLValid  = key.New(keyCtrlL).WithHelp("ctrl+l", "validate")
	// CtrlHHint leaves the focused hint panel; the legend does not offer it there.
	CtrlHHint = key.New(keyCtrlH)

	AAppend = key.New(keyA).WithHelp("a", "append")
	Filter  = key.New(keySlash).WithHelp("/", "filter")
)
