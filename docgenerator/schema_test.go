package docgenerator

import (
	"strings"
	"testing"
)

type dockerSettings struct {
	Image string `yaml:"image"`
}

type dockerConfig struct {
	Settings dockerSettings `yaml:"settings"`
}

type podmanSettings struct {
	Runtime string `yaml:"runtime"`
}

type podmanConfig struct {
	Settings podmanSettings `yaml:"settings"`
}

func TestGenerateDocsForEachErrorsOnDuplicateOutputFile(t *testing.T) {
	dir := t.TempDir()
	g := NewSchemaGenerator()
	_, err := g.GenerateDocsForEach([]Entry{
		{Config: dockerConfig{}, DocsDir: dir, SplitStructs: true},
		{Config: podmanConfig{}, DocsDir: dir, SplitStructs: true},
	})
	if err == nil {
		t.Fatal("expected duplicate output file error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate docs page") {
		t.Errorf("error = %q, want it to mention the duplicate page", err)
	}
}
