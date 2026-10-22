package report

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lucasassuncao/yedit/spec"
)

func TestPlain(t *testing.T) {
	var buf bytes.Buffer
	Plain(&buf, []spec.Violation{
		errV("categories[0].name", "is required"),
		warnV("categories[0].destination.path", "same as source"),
	}, Options{})

	out := buf.String()
	assert.Contains(t, out, "categories[0].name")
	assert.Contains(t, out, "is required")
	assert.Contains(t, out, "1 error(s)")
	assert.Contains(t, out, "1 warning(s)")
}

func TestPlainOK(t *testing.T) {
	var buf bytes.Buffer
	Plain(&buf, nil, Options{})
	assert.Equal(t, "ok\n", buf.String())
}

func TestPlainSummaryOnly(t *testing.T) {
	var buf bytes.Buffer
	Plain(&buf, []spec.Violation{errV("a.b", "boom")}, Options{SummaryOnly: true})

	assert.NotContains(t, buf.String(), "boom")
	assert.Contains(t, buf.String(), "1 error(s)")
}
