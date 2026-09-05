package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/spec"
)

func errV(path, msg string) spec.Violation {
	return spec.Violation{Path: path, Message: msg}
}

func warnV(path, msg string) spec.Violation {
	return spec.Violation{Path: path, Message: msg, Severity: spec.SeverityWarning}
}

func TestPartitionDefaultsToError(t *testing.T) {
	// The zero Severity must keep behaving like an error, or every validator
	// written before the field existed would silently become non-fatal.
	errs, warnings := Partition([]spec.Violation{
		errV("a", "boom"),
		warnV("b", "careful"),
		{Path: "c", Message: "no severity named"},
	})

	assert.Len(t, errs, 2)
	assert.Len(t, warnings, 1)
	assert.Equal(t, "c", errs[1].Path)
}

func TestSection(t *testing.T) {
	assert.Equal(t, "categories", Section("categories[0].source.path"))
	assert.Equal(t, "configuration", Section("configuration.defaults.action"))
	assert.Equal(t, "logging", Section("logging"))
	assert.Equal(t, "(general)", Section(""))
	assert.Equal(t, "(general)", Section("[0].name"))
}

func TestSubPath(t *testing.T) {
	assert.Equal(t, "[0].source.path", SubPath("categories[0].source.path"))
	assert.Equal(t, "defaults.action", SubPath("configuration.defaults.action"))
	assert.Equal(t, "logging", SubPath("logging"), "a path with no separator stays whole")
}

func TestGroupBySectionIsSorted(t *testing.T) {
	sections, bySection := GroupBySection([]spec.Violation{
		errV("logging.level", "bad"),
		errV("categories[0].name", "missing"),
		errV("categories[1].name", "missing"),
	})

	assert.Equal(t, []string{"categories", "logging"}, sections)
	assert.Len(t, bySection["categories"], 2)
	assert.Len(t, bySection["logging"], 1)
}

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

func TestJSONShape(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, JSON(&buf, []spec.Violation{
		errV("categories[0].name", "is required"),
		errV("logging.level", "not allowed"),
		warnV("categories[0].destination.path", "same as source"),
	}, Options{}))

	var out JSONOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	assert.False(t, out.Valid)
	assert.Equal(t, 2, out.ErrorCount)
	assert.Equal(t, 1, out.WarningCount)
	assert.Equal(t, map[string]int{"categories": 1, "logging": 1}, out.Summary)
	assert.Len(t, out.Errors, 2)
	assert.Len(t, out.Warnings, 1)
}

func TestJSONWarningsDoNotInvalidate(t *testing.T) {
	// The whole point of severity: a warning is reported but never fails.
	var buf bytes.Buffer
	require.NoError(t, JSON(&buf, []spec.Violation{warnV("a.b", "careful")}, Options{}))

	var out JSONOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))

	assert.True(t, out.Valid)
	assert.Equal(t, 0, out.ErrorCount)
	assert.Equal(t, 1, out.WarningCount)
}

func TestJSONSummaryAlwaysPresent(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, JSON(&buf, nil, Options{}))
	assert.Contains(t, buf.String(), `"summary"`)
}

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
