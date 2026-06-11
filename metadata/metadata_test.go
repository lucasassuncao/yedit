package metadata_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/metadata"
)

type filter struct {
	Regex  string        `yaml:"regex"`
	MinAge time.Duration `yaml:"min-age"`
	Any    []filter      `yaml:"any"`
}

type source struct {
	Path       string   `yaml:"path"`
	Extensions []string `yaml:"extensions"`
	Filter     filter   `yaml:"filter"`
}

type category struct {
	Name   string  `yaml:"name"`
	Source *source `yaml:"source"`
}

type config struct {
	Output     string         `yaml:"output"`
	Categories []category     `yaml:"categories"`
	Labels     map[string]int `yaml:"labels"`
}

// tree builds a valid metadata tree with a recursive shared-pointer child.
func tree() map[string]*metadata.Node {
	filterChildren := map[string]*metadata.Node{
		"regex":   {FieldMeta: editor.FieldMeta{Description: "regex"}},
		"min-age": {FieldMeta: editor.FieldMeta{Min: "0s", Max: "87600h"}},
	}
	anyNode := &metadata.Node{FieldMeta: editor.FieldMeta{Description: "OR"}}
	anyNode.Children = filterChildren
	filterChildren["any"] = anyNode
	return map[string]*metadata.Node{
		"output": {FieldMeta: editor.FieldMeta{OneOf: []string{"console", "file"}}},
		"categories": {
			FieldMeta: editor.FieldMeta{Required: true},
			Children: map[string]*metadata.Node{
				"name": {FieldMeta: editor.FieldMeta{Required: true}},
				"source": {Children: map[string]*metadata.Node{
					"path":       {FieldMeta: editor.FieldMeta{Required: true}},
					"extensions": {FieldMeta: editor.FieldMeta{MinCount: 1, Unique: true}},
					"filter":     {Children: filterChildren},
				}},
			},
		},
	}
}

func TestBuild_resolvesFieldHints(t *testing.T) {
	src, err := metadata.Build(&config{}, tree())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !src.FieldMeta("categories", "").Required {
		t.Error("block-level meta should resolve")
	}
	if !src.FieldMeta("categories", "source.path").Required {
		t.Error("nested meta should resolve")
	}
	if got := src.FieldMeta("categories", "source.filter.any.min-age").Max; got != "87600h" {
		t.Errorf("recursive meta Max = %q, want 87600h", got)
	}
	if meta := src.FieldMeta("categories", "nope"); meta.Required || meta.Description != "" {
		t.Error("miss should return zero FieldMeta")
	}
	if meta := src.FieldMeta("unknown-block", ""); meta.Required {
		t.Error("unknown block should return zero FieldMeta")
	}
}

func TestBuild_derivesTypes(t *testing.T) {
	src, err := metadata.Build(&config{}, tree())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	for path, want := range map[string]string{
		"":                      "[]object", // the "categories" block itself
		"name":                  "string",
		"source.extensions":     "[]string",
		"source.filter.min-age": "duration",
	} {
		if got := src.FieldMeta("categories", path).Type; got != want {
			t.Errorf("Type(categories, %q) = %q, want %q", path, got, want)
		}
	}
	if got := src.FieldMeta("output", "").Type; got != "string" {
		t.Errorf("Type(output) = %q, want string", got)
	}
}

func TestBuild_explicitTypeWins(t *testing.T) {
	tr := map[string]*metadata.Node{
		"output": {FieldMeta: editor.FieldMeta{Type: "custom-label"}},
	}
	src, err := metadata.Build(&config{}, tr)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := src.FieldMeta("output", "").Type; got != "custom-label" {
		t.Errorf("explicit Type overwritten: got %q", got)
	}
}

func TestBuild_unknownTopLevelKey(t *testing.T) {
	tr := map[string]*metadata.Node{"outptu": {}}
	if _, err := metadata.Build(&config{}, tr); err == nil || !strings.Contains(err.Error(), `"outptu"`) {
		t.Fatalf("expected unknown-key error naming the key, got %v", err)
	}
}

func TestBuild_unknownNestedKey(t *testing.T) {
	tr := map[string]*metadata.Node{
		"categories": {Children: map[string]*metadata.Node{
			"sourc": {},
		}},
	}
	if _, err := metadata.Build(&config{}, tr); err == nil || !strings.Contains(err.Error(), "categories.sourc") {
		t.Fatalf("expected error naming the full path, got %v", err)
	}
}
