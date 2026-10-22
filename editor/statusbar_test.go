package editor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// An error outranks the unsaved notice, which outranks a transient status.
func TestFeedbackLinePriority(t *testing.T) {
	var be blockEditState
	text, isErr := be.feedbackLine()
	assert.Empty(t, text)
	assert.False(t, isErr)

	be.statusMsg = "Undone."
	text, isErr = be.feedbackLine()
	assert.Equal(t, "Undone.", text)
	assert.False(t, isErr)

	be.dirty = true
	text, isErr = be.feedbackLine()
	assert.Equal(t, msgUncommittedChanges, text)
	assert.False(t, isErr)

	be.editorErr = editorError{kind: errParse, message: "bad indent"}
	text, isErr = be.feedbackLine()
	assert.Equal(t, "bad indent", text)
	assert.True(t, isErr)
}
