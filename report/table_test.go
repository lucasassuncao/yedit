package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lucasassuncao/yedit/spec"
)

func TestTableRendersBorders(t *testing.T) {
	var buf bytes.Buffer
	Table(&buf, []spec.Violation{errV("categories[0].name", "is required")}, Options{Width: 100})

	out := buf.String()
	assert.Contains(t, out, "PATH")
	assert.Contains(t, out, "ERROR")
	assert.Contains(t, out, "[0].name")
	assert.True(t, strings.Contains(out, "╭") || strings.Contains(out, "─"), "rounded borders are drawn")
}

func TestTableNarrowTerminalKeepsMessageColumn(t *testing.T) {
	// A width small enough to drive the message column negative must not panic
	// or produce a zero-width column.
	var buf bytes.Buffer
	Table(&buf, []spec.Violation{errV("a.b", "a message that needs room")}, Options{Width: 20})
	assert.Contains(t, buf.String(), "message")
}
