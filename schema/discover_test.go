package schema_test

import (
	"testing"

	"github.com/lucasassuncao/yedit/schema"
)

type taggedConfig struct {
	Name    string       `yaml:"name" validate:"required" jsonschema_description:"Project name."`
	Image   string       `yaml:"image" jsonschema_description:"Docker image."`
	Mode    string       `yaml:"mode" validate:"omitempty,oneof=dev prod"`
	Build   *buildConfig `yaml:"build"`
	Skipped string       // no yaml tag
	Hidden  string       `yaml:"-"`
	Meta    string       `yaml:"$schema"`
}

func TestDiscover_topLevelFields(t *testing.T) {
	fields := schema.Discover(&taggedConfig{})
	got := schema.TopLevelOrder(fields)
	want := []string{"name", "image", "mode", "build"}
	if len(got) != len(want) {
		t.Fatalf("TopLevelOrder = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("TopLevelOrder[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestDiscover_required(t *testing.T) {
	fields := schema.Discover(&taggedConfig{})
	for _, f := range fields {
		switch f.YAMLName {
		case "name":
			if !f.Required {
				t.Errorf("name should be Required")
			}
		case "image":
			if f.Required {
				t.Errorf("image should not be Required")
			}
		}
	}
}

func TestDiscover_oneOf(t *testing.T) {
	fields := schema.Discover(&taggedConfig{})
	for _, f := range fields {
		if f.YAMLName == "mode" {
			want := []string{"dev", "prod"}
			if len(f.OneOf) != len(want) || f.OneOf[0] != want[0] || f.OneOf[1] != want[1] {
				t.Errorf("mode.OneOf = %v, want %v", f.OneOf, want)
			}
		}
	}
}

func TestDiscover_descents(t *testing.T) {
	fields := schema.Discover(&taggedConfig{})
	var build schema.FieldDef
	for _, f := range fields {
		if f.YAMLName == "build" {
			build = f
		}
	}
	if len(build.Children) == 0 {
		t.Fatal("build should have children discovered from buildConfig")
	}
	names := make([]string, len(build.Children))
	for i, c := range build.Children {
		names[i] = c.YAMLName
	}
	if names[0] != "dockerfile" || names[1] != "context" || names[2] != "args" {
		t.Errorf("build children = %v, want [dockerfile context args]", names)
	}
}

// unionItem opts into Provider to declare its own schema.
type unionItem struct{}

func (unionItem) YeditSchema() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "type", Kind: schema.KindScalar, Required: true},
		{YAMLName: "target", Kind: schema.KindScalar, Required: true},
	}
}

type configWithUnion struct {
	Items []unionItem `yaml:"items"`
}

func TestDiscover_providerOverridesReflection(t *testing.T) {
	fields := schema.Discover(&configWithUnion{})
	if len(fields) != 1 || fields[0].YAMLName != "items" {
		t.Fatalf("expected single field 'items', got %v", fields)
	}
	if fields[0].Kind != schema.KindUnion {
		t.Errorf("items Kind = %v, want KindUnion", fields[0].Kind)
	}
	if len(fields[0].Children) != 2 {
		t.Fatalf("expected 2 children from Provider, got %d", len(fields[0].Children))
	}
	if fields[0].Children[0].YAMLName != "type" || fields[0].Children[1].YAMLName != "target" {
		t.Errorf("Provider children = %+v, want [type target]", fields[0].Children)
	}
}
