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

func TestGenerateDocsInMemoryQualifiesCollidingChildKeys(t *testing.T) {
	g := NewSchemaGenerator()
	ds := g.GenerateDocsInMemory([]Entry{
		{Config: dockerConfig{}, SplitStructs: true},
		{Config: podmanConfig{}, SplitStructs: true},
	})

	if _, ok := ds.Pages["settings"]; !ok {
		t.Fatal("first child page 'settings' missing")
	}
	second, ok := ds.Pages["podmanConfig.settings"]
	if !ok {
		t.Fatalf("colliding child page not re-keyed; pages: %v", pageKeys(ds))
	}
	if !strings.Contains(second, "runtime") {
		t.Errorf("re-keyed page does not hold the second entry's fields:\n%s", second)
	}
	if first := ds.Pages["settings"]; !strings.Contains(first, "image") {
		t.Errorf("first entry's child page was overwritten:\n%s", first)
	}

	if got := ds.Children["podmanConfig"]; len(got) != 1 || got[0] != "podmanConfig.settings" {
		t.Errorf("Children[podmanConfig] = %v, want [podmanConfig.settings]", got)
	}

	root := ds.Pages["podmanConfig"]
	if !strings.Contains(root, "(./podmanconfig.settings.md)") {
		t.Errorf("second root page link not rewritten to the qualified key:\n%s", root)
	}
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

func TestExtractTopicLinksResolvesLowercasedTargets(t *testing.T) {
	known := map[string]string{
		"podmanConfig.settings": "",
		"settings":              "",
	}
	raw := "see [settings](./podmanconfig.settings.md) and [other](./settings.md)"
	got := extractTopicLinks(raw, known)
	if len(got) != 2 || got[0] != "podmanConfig.settings" || got[1] != "settings" {
		t.Errorf("extractTopicLinks = %v, want [podmanConfig.settings settings]", got)
	}
}

func pageKeys(ds DocSet) []string {
	keys := make([]string, 0, len(ds.Pages))
	for k := range ds.Pages {
		keys = append(keys, k)
	}
	return keys
}
