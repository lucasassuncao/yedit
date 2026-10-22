package report

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/spec"
)

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
