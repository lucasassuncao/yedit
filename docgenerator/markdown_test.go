package docgenerator

import (
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/schema"
)

func TestFormatLabelsDoesNotEmitTablePipes(t *testing.T) {
	meta := editor.FieldMeta{Formats: []editor.Format{editor.FormatURL, editor.FormatUUID}}
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
	src := editor.MetadataFunc(func(blockKey, fieldPath string) editor.FieldMeta {
		return editor.FieldMeta{
			Description: "either a\nor b",
			Default:     "a|b",
			Formats:     []editor.Format{editor.FormatURL},
		}
	})
	g := NewSchemaGenerator(WithMetadata(src))
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
	root := NewSchemaGenerator().generateRootMarkdown("upperConfig", schema.Discover(upperConfig{}))
	if !strings.Contains(root, "[Settings](./settings.md)") {
		t.Errorf("link target not lowercased:\n%s", root)
	}
}
