package editor

// hintSep is the separator between hint segments.
const hintSep = " • "

// Atomic key hints — one per key or action.
const (
	keyNav      = "[↑/↓] nav"
	keyNavigate = "[↑/↓] navigate"
	keyScroll   = "[↑/↓] scroll"
	keyExpand   = "[→/←] expand"

	keyTabPane    = "[Tab] change pane"
	keyTabPreview = "[Tab] preview"
	keyTabPresets = "[Tab] presets"
	keyTabEscList = "[Tab] / [Esc] back to list"

	keyCtrlSSave     = "[ctrl+s] save"
	keyCtrlSSaveChg  = "[ctrl+s] save changes"
	keyCtrlDDelete   = "[ctrl+d] delete"
	keyCtrlDRemove   = "[ctrl+d] remove"
	keyCtrlUUndo     = "[ctrl+u] undo"
	keyCtrlLValidate = "[ctrl+l] validate"

	keyEscBack   = "[Esc] back"
	keyEscCancel = "[Esc] cancel"
	keyEscClear  = "[Esc] clear"

	keyEnterAdd     = "[Enter] add"
	keyEnterApply   = "[Enter] apply"
	keyEnterOpen    = "[Enter] open"
	keyEnterReplace = "[Enter] replace"
	keyEnterSelect  = "[Enter] select"

	keyAAppend = "[a] append"

	keyFilter     = "[/] filter"
	keyTypeFilter = "[type] filter"
	keyPreset     = "[p] preset"

	keyHint     = "[h] hint"
	keyHintHide = "[h] hide hint"
)

// Composite hints built by concatenating atoms with hintSep.
const (
	hintSaveTail = keyTabPane + hintSep + keyCtrlUUndo + hintSep + keyCtrlSSaveChg + hintSep + keyEscBack

	hintPresetPreviewFocused = keyScroll + hintSep + keyTabPresets + hintSep + keyEscBack
	hintPresetListScalar     = keyNavigate + hintSep + keyTabPreview + hintSep + keyEnterApply + hintSep + keyEscCancel
	hintPresetListCollection = keyNavigate + hintSep + keyTabPreview + hintSep + keyEnterReplace + hintSep + keyAAppend + hintSep + keyEscCancel

	hintModelPreviewFocused = keyScroll + hintSep + keyTabEscList
	hintModelFiltering      = keyTypeFilter + hintSep + keyNavigate + hintSep + keyEnterSelect + hintSep + keyEscClear
	hintModelExisting       = keyNav + hintSep + keyFilter + hintSep + keyEnterOpen + hintSep + keyCtrlDDelete + hintSep + keyCtrlUUndo + hintSep + keyCtrlSSave + hintSep + keyCtrlLValidate
	hintModelNew            = keyNav + hintSep + keyFilter + hintSep + keyEnterAdd + hintSep + keyCtrlUUndo + hintSep + keyCtrlSSave + hintSep + keyCtrlLValidate

	msgUncommittedChanges = "Uncommitted changes — ctrl+s to commit"
)
