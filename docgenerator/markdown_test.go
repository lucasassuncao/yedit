package docgenerator

import (
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/spec"
)

func TestFormatLabelsDoesNotEmitTablePipes(t *testing.T) {
	meta := spec.FieldMeta{Formats: []spec.Format{spec.FormatURL, spec.FormatUUID}}
	got := formatLabels(meta)
	if got != "url, uuid" {
		t.Errorf("formatLabels = %q, want %q", got, "url, uuid")
	}
	if strings.Contains(got, "|") {
		t.Errorf("formatLabels = %q, must not contain a raw pipe", got)
	}
}

func TestCellTextEscapesPipesAndCollapsesNewlines(t *testing.T) {
	got := cellText("a|b\nsecond   line")
	want := "a\\|b second line"
	if got != want {
		t.Errorf("cellText = %q, want %q", got, want)
	}
}

type pipeConfig struct {
	Mode string `yaml:"mode"`
}

func TestFieldsTableEscapesMetadataCells(t *testing.T) {
	src := spec.MetadataFunc(func(blockKey, fieldPath string) spec.FieldMeta {
		return spec.FieldMeta{
			Description: "either a\nor b",
			Default:     "a|b",
			Formats:     []spec.Format{spec.FormatURL},
		}
	})
	g := &mdRenderer{metadata: src}
	page := g.generateMarkdown("pipeConfig", schema.Discover(pipeConfig{}), nil)
	if !strings.Contains(page, "| a\\|b |") {
		t.Errorf("default cell not pipe-escaped:\n%s", page)
	}
	if !strings.Contains(page, "either a or b") {
		t.Errorf("description newlines not collapsed:\n%s", page)
	}
}

type upperChild struct {
	Image string `yaml:"image"`
}

type upperConfig struct {
	Settings upperChild `yaml:"Settings"`
}

func TestLinkedFieldsTableLowercasesLinkTarget(t *testing.T) {
	root := (&mdRenderer{}).generateRootMarkdown("upperConfig", schema.Discover(upperConfig{}))
	if !strings.Contains(root, "[Settings](./settings.md)") {
		t.Errorf("link target not lowercased:\n%s", root)
	}
}
