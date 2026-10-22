package report

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
