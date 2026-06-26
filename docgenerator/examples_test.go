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

func TestGenerateExampleDocs_OneFilePerField(t *testing.T) {
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
	files, err := docgenerator.GenerateExampleDocs(dir, src, titles)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	for _, name := range []string{"configuration.md", "category.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected file %s to exist: %v", name, err)
		}
	}
}

func TestGenerateExampleDocs_FileContentContainsPresets(t *testing.T) {
	src := presets.ForField("configuration", map[string]*logConfig{
		"console": {Output: "console", Level: "info"},
		"file":    {Output: "file", Level: "warn"},
	})

	dir := t.TempDir()
	_, err := docgenerator.GenerateExampleDocs(dir, src, map[string]string{
		"configuration": "Configuration",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "configuration.md"))
	if err != nil {
		t.Fatalf("configuration.md not found: %v", err)
	}
	body := string(content)

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

func TestGenerateExampleDocs_SkipsFieldNotInTitles(t *testing.T) {
	src := presets.Combine(
		presets.ForField("configuration", map[string]*logConfig{
			"console": {Output: "console", Level: "info"},
		}),
		presets.ForField("categories", map[string]*categoryConfig{
			"images": {Name: "photos", Path: "~/Downloads"},
		}),
	)

	dir := t.TempDir()
	files, err := docgenerator.GenerateExampleDocs(dir, src, map[string]string{
		"configuration": "Configuration",
		// categories intentionally omitted
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if _, err := os.Stat(filepath.Join(dir, "category.md")); err == nil {
		t.Error("category.md should not have been generated")
	}
}

func TestGenerateExampleDocs_SkipsFieldWithNoPresets(t *testing.T) {
	src := presets.ForField("configuration", map[string]*logConfig{})

	dir := t.TempDir()
	files, err := docgenerator.GenerateExampleDocs(dir, src, map[string]string{
		"configuration": "Configuration",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestGenerateExampleDocs_NoReadmeInExamplesDir(t *testing.T) {
	src := presets.ForField("configuration", map[string]*logConfig{
		"console": {Output: "console", Level: "info"},
	})

	dir := t.TempDir()
	if _, err := docgenerator.GenerateExampleDocs(dir, src, map[string]string{
		"configuration": "Configuration",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "README.md")); err == nil {
		t.Error("README.md should not be generated inside examplesDir")
	}
}

func TestGenerateExampleDocs_FilenameIsLowercasedTitle(t *testing.T) {
	src := presets.ForField("categories", map[string]*categoryConfig{
		"images": {Name: "photos", Path: "~/Downloads"},
	})

	dir := t.TempDir()
	files, err := docgenerator.GenerateExampleDocs(dir, src, map[string]string{
		"categories": "Category",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 1 || files[0].Name != "Category" {
		t.Fatalf("expected 1 file named 'Category', got %v", files)
	}
	if _, err := os.Stat(filepath.Join(dir, "category.md")); err != nil {
		t.Error("expected file to be named category.md (lowercased title)")
	}
}
