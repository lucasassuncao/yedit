package editor

const msgUncommittedChanges = "Uncommitted changes - ctrl+s to commit"

// feedbackLine picks the block editor's feedback: an error takes priority,
// then the unsaved-changes notice, then any transient status message.
func (be blockEditState) feedbackLine() (text string, isError bool) {
	switch {
	case be.editorErr.kind != errNone:
		return be.editorErr.message, true
	case be.dirty:
		return msgUncommittedChanges, false
	case be.statusMsg != "":
		return be.statusMsg, false
	}
	return "", false
}
