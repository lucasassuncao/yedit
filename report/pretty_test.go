package report

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/lucasassuncao/yedit/spec"
)

func TestPrettyGroupsBySection(t *testing.T) {
	var buf bytes.Buffer
	Pretty(&buf, []spec.Violation{
		errV("categories[0].name", "is required"),
		errV("logging.level", "not allowed"),
		warnV("categories[1].source", "overlaps"),
	}, Options{WarningNote: "valid but lossy"})

	out := buf.String()
	assert.Contains(t, out, "categories")
	assert.Contains(t, out, "logging")
	assert.Contains(t, out, "[0].name", "the section prefix is stripped under its heading")
	assert.Contains(t, out, "warnings")
	assert.Contains(t, out, "valid but lossy")
	assert.Contains(t, out, "└─")
}

func TestPrettyOK(t *testing.T) {
	var buf bytes.Buffer
	Pretty(&buf, nil, Options{})
	assert.Contains(t, buf.String(), "configuration is valid")
}

func TestPrettySummaryOnlyKeepsCounts(t *testing.T) {
	var buf bytes.Buffer
	Pretty(&buf, []spec.Violation{
		errV("categories[0].name", "is required"),
		warnV("logging.level", "careful"),
	}, Options{SummaryOnly: true})

	out := buf.String()
	assert.NotContains(t, out, "is required")
	assert.Contains(t, out, "error(s)")
	assert.Contains(t, out, "warning(s)")
}
