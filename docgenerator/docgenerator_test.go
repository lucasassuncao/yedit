package docgenerator_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/docgenerator"
	"github.com/lucasassuncao/yedit/presets"
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func lsNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestGenerate_noOutputOptionIsAnError(t *testing.T) {
	_, err := docgenerator.Generate([]docgenerator.Entry{{Config: dockerConfig{}}})
	if !errors.Is(err, docgenerator.ErrNoOutput) {
		t.Fatalf("want ErrNoOutput, got %v", err)
	}
}

func TestGenerate_markdownOnlyWritesNoOtherOutput(t *testing.T) {
	dir := t.TempDir()
	files, err := docgenerator.Generate(
		[]docgenerator.Entry{{Config: dockerConfig{}}},
		docgenerator.WithMarkdown(dir),
	)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d: %v", len(files), files)
	}
	if got := filepath.Base(files[0].Path); got != "dockerconfig.md" {
		t.Fatalf("want dockerconfig.md, got %s", got)
	}
	if files[0].Name != "dockerConfig" {
		t.Fatalf("want name dockerConfig, got %s", files[0].Name)
	}
	for _, name := range lsNames(t, dir) {
		if name == "README.md" || strings.HasSuffix(name, ".json") {
			t.Fatalf("unexpected file written: %s", name)
		}
	}
}

func TestGenerate_errorsOnDuplicateMarkdownPage(t *testing.T) {
	dir := t.TempDir()
	_, err := docgenerator.Generate(
		[]docgenerator.Entry{
			{Config: dockerConfig{}, SplitStructs: true},
			{Config: podmanConfig{}, SplitStructs: true},
		},
		docgenerator.WithMarkdown(dir),
	)
	if err == nil {
		t.Fatal("want duplicate page error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate docs page") {
		t.Fatalf("want duplicate docs page error, got %v", err)
	}
}

func TestGenerate_examplesCrossLinkUsesOneDeclaration(t *testing.T) {
	base := t.TempDir()
	docsDir := filepath.Join(base, "reference")
	examplesDir := filepath.Join(base, "examples")

	src := presets.Combine(
		presets.ForField("dockerconfig", map[string]*dockerSettings{
			"minimal": {Image: "alpine"},
		}),
	)

	_, err := docgenerator.Generate(
		[]docgenerator.Entry{{Config: dockerConfig{}}},
		docgenerator.WithMarkdown(docsDir),
		docgenerator.WithExamples(src, examplesDir, map[string]string{"dockerconfig": "dockerConfig"}),
	)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	page := readFile(t, filepath.Join(docsDir, "dockerconfig.md"))
	want := "[dockerConfig presets](../examples/dockerconfig.md)"
	if !strings.Contains(page, want) {
		t.Fatalf("want cross-link %q in page:\n%s", want, page)
	}
	if _, err := os.Stat(filepath.Join(examplesDir, "dockerconfig.md")); err != nil {
		t.Fatalf("example page not written: %v", err)
	}
}

func TestGenerate_markdownWithoutExamplesHasNoExamplesSection(t *testing.T) {
	dir := t.TempDir()
	if _, err := docgenerator.Generate(
		[]docgenerator.Entry{{Config: dockerConfig{}}},
		docgenerator.WithMarkdown(dir),
	); err != nil {
		t.Fatalf("generate: %v", err)
	}
	page := readFile(t, filepath.Join(dir, "dockerconfig.md"))
	if strings.Contains(page, "## Examples") {
		t.Fatalf("unexpected Examples section:\n%s", page)
	}
}

func TestGenerate_indexListsPagesAndExamplesOnly(t *testing.T) {
	base := t.TempDir()
	docsDir := filepath.Join(base, "reference")
	examplesDir := filepath.Join(base, "examples")
	schemaDir := filepath.Join(base, "schema")

	src := presets.Combine(
		presets.ForField("dockerconfig", map[string]*dockerSettings{
			"minimal": {Image: "alpine"},
		}),
	)

	if _, err := docgenerator.Generate(
		[]docgenerator.Entry{{Config: dockerConfig{}}},
		docgenerator.WithMarkdown(docsDir),
		docgenerator.WithJSONSchema(schemaDir),
		docgenerator.WithExamples(src, examplesDir, map[string]string{"dockerconfig": "dockerConfig"}),
		docgenerator.WithIndex(base),
	); err != nil {
		t.Fatalf("generate: %v", err)
	}

	index := readFile(t, filepath.Join(base, "README.md"))
	for _, want := range []string{
		"## Available Configurations",
		"[dockerConfig](./reference/dockerconfig.md)",
		"## Examples",
		"[dockerConfig](./examples/dockerconfig.md)",
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("want %q in index:\n%s", want, index)
		}
	}
	if strings.Contains(index, ".schema.json") {
		t.Fatalf("index must not list schema files:\n%s", index)
	}
}

func TestGenerate_indexOmitsEmptySections(t *testing.T) {
	base := t.TempDir()
	if _, err := docgenerator.Generate(
		[]docgenerator.Entry{{Config: dockerConfig{}}},
		docgenerator.WithMarkdown(filepath.Join(base, "reference")),
		docgenerator.WithIndex(base),
	); err != nil {
		t.Fatalf("generate: %v", err)
	}
	index := readFile(t, filepath.Join(base, "README.md"))
	if strings.Contains(index, "## Examples") {
		t.Fatalf("empty Examples section must be omitted:\n%s", index)
	}
}

func TestGenerate_examplesWithoutSourceIsAnError(t *testing.T) {
	_, err := docgenerator.Generate(
		[]docgenerator.Entry{{Config: dockerConfig{}}},
		docgenerator.WithExamples(nil, t.TempDir(), map[string]string{"a": "A"}),
	)
	if !errors.Is(err, docgenerator.ErrNoPresetSource) {
		t.Fatalf("want ErrNoPresetSource, got %v", err)
	}
}

func TestGenerate_jsonSchemaOnlyWritesNoMarkdown(t *testing.T) {
	dir := t.TempDir()
	files, err := docgenerator.Generate(
		[]docgenerator.Entry{{Config: dockerConfig{}}},
		docgenerator.WithJSONSchema(dir),
	)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d: %v", len(files), files)
	}
	if got := filepath.Base(files[0].Path); got != "dockerconfig.schema.json" {
		t.Fatalf("want dockerconfig.schema.json, got %s", got)
	}
	for _, name := range lsNames(t, dir) {
		if strings.HasSuffix(name, ".md") {
			t.Fatalf("unexpected markdown written: %s", name)
		}
	}

	content := readFile(t, files[0].Path)
	if !strings.HasSuffix(content, "\n") {
		t.Fatal("generated schema must end with a newline")
	}
	if !strings.Contains(content, `"$schema": "https://json-schema.org/draft/2020-12/schema"`) {
		t.Fatalf("missing dialect declaration:\n%s", content)
	}
	if strings.Contains(content, `"additionalProperties": false`) {
		t.Fatalf("additionalProperties must not be emitted:\n%s", content)
	}
}

func TestGenerate_errorsOnDuplicateJSONSchema(t *testing.T) {
	dir := t.TempDir()
	_, err := docgenerator.Generate(
		[]docgenerator.Entry{
			{Config: dockerConfig{}},
			{Config: dockerConfig{}},
		},
		docgenerator.WithJSONSchema(dir),
	)
	if err == nil || !strings.Contains(err.Error(), "duplicate json schema") {
		t.Fatalf("want duplicate json schema error, got %v", err)
	}
}

// TestGenerate_perEntryMarkdownDir pins the layout movelooper depends on: each
// documented struct in its own subdirectory, with the index above them and the
// example cross-links resolved relative to each entry's own directory.
func TestGenerate_perEntryMarkdownDir(t *testing.T) {
	base := t.TempDir()
	attributesDir := filepath.Join(base, "attributes")
	examplesDir := filepath.Join(base, "examples")

	src := presets.Combine(
		presets.ForField("dockerconfig", map[string]*dockerSettings{
			"minimal": {Image: "alpine"},
		}),
	)

	files, err := docgenerator.Generate(
		[]docgenerator.Entry{
			{Config: dockerConfig{}, MarkdownDir: filepath.Join(attributesDir, "docker")},
			{Config: podmanConfig{}, MarkdownDir: filepath.Join(attributesDir, "podman"), SplitStructs: true},
		},
		docgenerator.WithMarkdown(attributesDir),
		docgenerator.WithExamples(src, examplesDir, map[string]string{"dockerconfig": "dockerConfig"}),
		docgenerator.WithIndex(base),
	)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	for _, want := range []string{
		filepath.Join(attributesDir, "docker", "dockerconfig.md"),
		filepath.Join(attributesDir, "podman", "podmanconfig.md"),
		filepath.Join(attributesDir, "podman", "settings.md"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("expected %s: %v (got %v)", want, err, files)
		}
	}
	if _, err := os.Stat(filepath.Join(attributesDir, "dockerconfig.md")); err == nil {
		t.Fatal("entry with MarkdownDir must not also write to the default dir")
	}

	// Both entries sit one level below attributesDir, so each link back to the
	// examples directory must climb two levels.
	page := readFile(t, filepath.Join(attributesDir, "docker", "dockerconfig.md"))
	want := "[dockerConfig presets](../../examples/dockerconfig.md)"
	if !strings.Contains(page, want) {
		t.Fatalf("want cross-link %q relative to the entry's own dir:\n%s", want, page)
	}

	index := readFile(t, filepath.Join(base, "README.md"))
	for _, link := range []string{
		"(./attributes/docker/dockerconfig.md)",
		"(./attributes/podman/podmanconfig.md)",
	} {
		if !strings.Contains(index, link) {
			t.Fatalf("want %q in index:\n%s", link, index)
		}
	}
}

// TestGenerate_perEntryMarkdownDirAtDifferentDepths pins that each entry's
// example link is computed from its own directory, not from a single shared one.
func TestGenerate_perEntryMarkdownDirAtDifferentDepths(t *testing.T) {
	base := t.TempDir()
	examplesDir := filepath.Join(base, "examples")

	src := presets.Combine(
		presets.ForField("dockerconfig", map[string]*dockerSettings{
			"minimal": {Image: "alpine"},
		}),
		presets.ForField("podmanconfig", map[string]*podmanSettings{
			"minimal": {Runtime: "crun"},
		}),
	)

	if _, err := docgenerator.Generate(
		[]docgenerator.Entry{
			{Config: dockerConfig{}, MarkdownDir: filepath.Join(base, "flat")},
			{Config: podmanConfig{}, MarkdownDir: filepath.Join(base, "deep", "nested", "here")},
		},
		docgenerator.WithMarkdown(base),
		docgenerator.WithExamples(src, examplesDir, map[string]string{
			"dockerconfig": "dockerConfig",
			"podmanconfig": "podmanConfig",
		}),
	); err != nil {
		t.Fatalf("generate: %v", err)
	}

	shallow := readFile(t, filepath.Join(base, "flat", "dockerconfig.md"))
	if !strings.Contains(shallow, "(../examples/dockerconfig.md)") {
		t.Fatalf("shallow entry link wrong:\n%s", shallow)
	}
	deep := readFile(t, filepath.Join(base, "deep", "nested", "here", "podmanconfig.md"))
	if !strings.Contains(deep, "(../../../examples/podmanconfig.md)") {
		t.Fatalf("deep entry link wrong:\n%s", deep)
	}
}
