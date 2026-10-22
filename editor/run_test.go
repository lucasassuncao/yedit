package editor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A config the editor cannot open fails before any terminal is touched.
func TestRunContextRejectsAnUnusableConfig(t *testing.T) {
	res, err := RunContext(context.Background(), Config{Path: filepath.Join(t.TempDir(), "x.yaml")})
	require.ErrorContains(t, err, "Config.Schema is required")
	assert.Equal(t, Result{}, res)
}

func TestRunContextReportsADumpFileItCannotCreate(t *testing.T) {
	cfg := Config{
		Path:   filepath.Join(t.TempDir(), "x.yaml"),
		Schema: &sizeProbeConfig{},
		Trace:  Trace{Dump: true, DumpPath: filepath.Join(t.TempDir(), "missing", "dump.jsonl")},
	}
	_, err := RunContext(context.Background(), cfg)
	require.ErrorContains(t, err, "creating session dump file")
}
