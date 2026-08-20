package docgenerator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/docgenerator"
	"github.com/lucasassuncao/yedit/presets"
)

type logConfig struct {
	Output string `yaml:"output"`
	Level  string `yaml:"level"`
}

type categoryConfig struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// generateExamples runs an examples-only generation. Example pages come from a
// presets.Source rather than from the schema, so no entries are needed.
func generateExamples(t *testing.T, dir string, src presets.Source, titles map[string]string) []docgenerator.GeneratedFile {
	t.Helper()
	files, err := docgenerator.Generate(nil, docgenerator.WithExamples(src, dir, titles))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return files
}

func TestExamples_OneFilePerField(t *testing.T) {
	src := presets.Combine(
		presets.ForField("configuration", map[string]*logConfig{
			"console": {Output: "console", Level: "info"},
			"file":    {Output: "file", Level: "warn"},
		}),
		presets.ForField("categories", map[string]*categoryConfig{
			"images": {Name: "photos", Path: "~/Downloads"},
		}),
	)
	titles := map[string]string{
		"configuration": "Configuration",
		"categories":    "Category",
	}

	dir := t.TempDir()
	files := generateExamples(t, dir, src, titles)

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	for _, name := range []string{"configuration.md", "category.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected file %s to exist: %v", name, err)
		}
	}
}

func TestExamples_FileContentContainsPresets(t *testing.T) {
	src := presets.ForField("configuration", map[string]*logConfig{
		"console": {Output: "console", Level: "info"},
		"file":    {Output: "file", Level: "warn"},
	})

	dir := t.TempDir()
	generateExamples(t, dir, src, map[string]string{"configuration": "Configuration"})

	body := readFile(t, filepath.Join(dir, "configuration.md"))

	if !strings.Contains(body, "## Preset: console") {
		t.Error("configuration.md missing preset 'console'")
	}
	if !strings.Contains(body, "## Preset: file") {
		t.Error("configuration.md missing preset 'file'")
	}
	if !strings.Contains(body, "```yaml") {
		t.Error("configuration.md missing yaml code fence")
	}
}

func TestExamples_SkipsFieldNotInTitles(t *testing.T) {
	src := presets.Combine(
		presets.ForField("configuration", map[string]*logConfig{
			"console": {Output: "console", Level: "info"},
		}),
		presets.ForField("categories", map[string]*categoryConfig{
			"images": {Name: "photos", Path: "~/Downloads"},
		}),
	)

	dir := t.TempDir()
	files := generateExamples(t, dir, src, map[string]string{
		"configuration": "Configuration",
		// categories intentionally omitted
	})

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if _, err := os.Stat(filepath.Join(dir, "category.md")); err == nil {
		t.Error("category.md should not have been generated")
	}
}

func TestExamples_SkipsFieldWithNoPresets(t *testing.T) {
	src := presets.ForField("configuration", map[string]*logConfig{})

	dir := t.TempDir()
	files := generateExamples(t, dir, src, map[string]string{"configuration": "Configuration"})

	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestExamples_NoReadmeInExamplesDir(t *testing.T) {
	src := presets.ForField("configuration", map[string]*logConfig{
		"console": {Output: "console", Level: "info"},
	})

	dir := t.TempDir()
	generateExamples(t, dir, src, map[string]string{"configuration": "Configuration"})

	if _, err := os.Stat(filepath.Join(dir, "README.md")); err == nil {
		t.Error("README.md should not be generated inside examplesDir")
	}
}

func TestExamples_FilenameIsLowercasedTitle(t *testing.T) {
	src := presets.ForField("categories", map[string]*categoryConfig{
		"images": {Name: "photos", Path: "~/Downloads"},
	})

	dir := t.TempDir()
	files := generateExamples(t, dir, src, map[string]string{"categories": "Category"})

	if len(files) != 1 || files[0].Name != "Category" {
		t.Fatalf("expected 1 file named 'Category', got %v", files)
	}
	if _, err := os.Stat(filepath.Join(dir, "category.md")); err != nil {
		t.Error("expected file to be named category.md (lowercased title)")
	}
}
