package editor

// legendSep is the separator between legend segments.
const legendSep = " • "

// Atomic key legend pieces - one per key or action.
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
	keyCtrlYRedo     = "[ctrl+y] redo"
	keyCtrlRReload   = "[ctrl+r] reload"
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

// Composite legends built by concatenating atoms with legendSep.
const (
	legendSaveTail = keyTabPane + legendSep + keyCtrlUUndo + legendSep + keyCtrlYRedo + legendSep + keyCtrlSSaveChg + legendSep + keyEscBack

	legendPresetPreviewFocused = keyScroll + legendSep + keyTabPresets + legendSep + keyEscBack
	legendPresetListScalar     = keyNavigate + legendSep + keyTabPreview + legendSep + keyEnterApply + legendSep + keyEscCancel
	legendPresetListCollection = keyNavigate + legendSep + keyTabPreview + legendSep + keyEnterReplace + legendSep + keyAAppend + legendSep + keyEscCancel

	legendModelPreviewFocused = keyScroll + legendSep + keyTabEscList
	legendModelFiltering      = keyTypeFilter + legendSep + keyNavigate + legendSep + keyEnterSelect + legendSep + keyEscClear
	legendModelExisting       = keyNav + legendSep + keyFilter + legendSep + keyEnterOpen + legendSep + keyCtrlDDelete + legendSep + keyCtrlUUndo + legendSep + keyCtrlYRedo + legendSep + keyCtrlRReload + legendSep + keyCtrlSSave + legendSep + keyCtrlLValidate
	legendModelNew            = keyNav + legendSep + keyFilter + legendSep + keyEnterAdd + legendSep + keyCtrlUUndo + legendSep + keyCtrlYRedo + legendSep + keyCtrlRReload + legendSep + keyCtrlSSave + legendSep + keyCtrlLValidate

	msgUncommittedChanges = "Uncommitted changes - ctrl+s to commit"
)
