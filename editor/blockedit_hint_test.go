package editor

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

// A list of scalars has no fields to select, so every entry shows the hint of
// the block itself.
func TestScalarListEntryShowsTheBlockHint(t *testing.T) {
	cfg := Config{EnableHints: true, Metadata: MetadataFunc(func(block, fieldPath string) FieldMeta {
		if fieldPath == "" {
			return FieldMeta{Description: "Files merged into this one."}
		}
		return FieldMeta{}
	})}
	be := newBlockEdit(cfg, scalarListSpec("import:\n  - ./a.yaml\n"), 100, 40)

	assert.Contains(t, ansi.Strip(be.hintContent()), "Files merged into this one.")
}
