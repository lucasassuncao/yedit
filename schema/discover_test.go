package schema_test

import (
	"fmt"
	"testing"
	"time"

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
		{YAMLName: "type", Kind: schema.KindPrimitive, Required: true},
		{YAMLName: "target", Kind: schema.KindPrimitive, Required: true},
	}
}

type configWithUnion struct {
	Items []unionItem `yaml:"items"`
}

// minimalConfig has only yaml tags — no validate, no jsonschema_description.
// Discover should still produce usable FieldDefs with zero-valued optional fields.
type minimalConfig struct {
	Name    string         `yaml:"name"`
	Port    int            `yaml:"port"`
	Nested  *minimalNested `yaml:"nested"`
	Skipped string         // no yaml tag — must be omitted
}

type minimalNested struct {
	Host string `yaml:"host"`
	Tls  bool   `yaml:"tls"`
}

func TestDiscover_yamlTagOnly(t *testing.T) {
	fields := schema.Discover(&minimalConfig{})

	got := schema.TopLevelOrder(fields)
	want := []string{"name", "port", "nested"}
	if len(got) != len(want) {
		t.Fatalf("TopLevelOrder = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("TopLevelOrder[%d] = %q, want %q", i, got[i], w)
		}
	}

	// Every optional attribute must be zero-valued when its tag is absent.
	for _, f := range fields {
		if f.Required {
			t.Errorf("%s.Required = true; expected false without validate tag", f.YAMLName)
		}
		if f.Description != "" {
			t.Errorf("%s.Description = %q; expected empty without jsonschema_description", f.YAMLName, f.Description)
		}
		if f.Default != "" {
			t.Errorf("%s.Default = %q; expected empty without jsonschema default", f.YAMLName, f.Default)
		}
		if len(f.OneOf) != 0 {
			t.Errorf("%s.OneOf = %v; expected empty without validate oneof", f.YAMLName, f.OneOf)
		}
	}

	// Nested struct still descends.
	var nested schema.FieldDef
	for _, f := range fields {
		if f.YAMLName == "nested" {
			nested = f
		}
	}
	if len(nested.Children) != 2 || nested.Children[0].YAMLName != "host" || nested.Children[1].YAMLName != "tls" {
		t.Errorf("nested children = %+v, want [host tls]", nested.Children)
	}
}

func TestDiscover_providerOverridesReflection(t *testing.T) {
	fields := schema.Discover(&configWithUnion{})
	if len(fields) != 1 || fields[0].YAMLName != "items" {
		t.Fatalf("expected single field 'items', got %v", fields)
	}
	if fields[0].Kind != schema.KindVariant {
		t.Errorf("items Kind = %v, want KindVariant", fields[0].Kind)
	}
	if len(fields[0].Children) != 2 {
		t.Fatalf("expected 2 children from Provider, got %d", len(fields[0].Children))
	}
	if fields[0].Children[0].YAMLName != "type" || fields[0].Children[1].YAMLName != "target" {
		t.Errorf("Provider children = %+v, want [type target]", fields[0].Children)
	}
}

type scalarConfig struct {
	Str  string        `yaml:"str"`
	Num  int           `yaml:"num"`
	Flag bool          `yaml:"flag"`
	Rate float64       `yaml:"rate"`
	Size uint          `yaml:"size"`
	TTL  time.Duration `yaml:"ttl"`
	Ptr  *bool         `yaml:"ptr"`
	Mode string        `yaml:"mode" validate:"oneof=a b"`
	Tags []string      `yaml:"tags"`
}

func TestDiscover_scalarType(t *testing.T) {
	got := map[string]string{}
	for _, f := range schema.Discover(&scalarConfig{}) {
		got[f.YAMLName] = f.Scalar
	}
	want := map[string]string{
		"str":  "string",
		"num":  "int",
		"flag": "bool",
		"rate": "float",
		"size": "uint",
		"ttl":  "duration",
		"ptr":  "bool",
		"mode": "string", // enum reclassification keeps the underlying scalar
		"tags": "",       // a slice is not a scalar
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s.Scalar = %q, want %q", name, got[name], w)
		}
	}
}

// ── item 11: cycle detection ──────────────────────────────────────────────────

type selfRefNode struct {
	Name     string        `yaml:"name"`
	Children []selfRefNode `yaml:"children"`
}
type selfRefRoot struct {
	Tree selfRefNode `yaml:"tree"`
}

func TestDiscover_recursiveTypeStopsAtCycle(t *testing.T) {
	// Default limit=1: one extra recursive level beyond the first visit.
	// tree → selfRefNode (visit 1): has children [name, children]
	// tree.children → selfRefNode (visit 2, shallow): has children [name, children]
	// tree.children.children → selfRefNode (visit 3 > limit=1): nil
	fields := schema.Discover(&selfRefRoot{})
	if len(fields) != 1 || fields[0].YAMLName != "tree" {
		t.Fatalf("expected single field 'tree', got %v", fields)
	}
	tree := fields[0]
	if len(tree.Children) != 2 {
		t.Fatalf("tree.Children = %d, want 2 (name + children)", len(tree.Children))
	}
	// The "children" field at the first level has one shallow level of children.
	childrenField := tree.Children[1]
	if len(childrenField.Children) != 2 {
		t.Errorf("children at depth 1 should have 2 children (shallow level), got %d", len(childrenField.Children))
	}
	// The "children" field at the second level (shallow) must be nil — cycle stopped.
	if len(childrenField.Children) >= 2 {
		deepChildrenField := childrenField.Children[1]
		if len(deepChildrenField.Children) != 0 {
			t.Errorf("children at depth 2 should have no children (cycle blocked), got %d", len(deepChildrenField.Children))
		}
	}
}

func TestDiscover_recursiveTypeLimit0(t *testing.T) {
	// Explicit limit=0: no recursive expansion (original strict cycle detection).
	fields := schema.Discover(&selfRefRoot{}, 0)
	tree := fields[0]
	childrenField := tree.Children[1]
	if len(childrenField.Children) != 0 {
		t.Errorf("with limit=0, recursive children should be nil, got %d", len(childrenField.Children))
	}
}

func TestDiscover_recursiveTypeLimit2(t *testing.T) {
	// Explicit limit=2: two extra levels of recursion.
	fields := schema.Discover(&selfRefRoot{}, 2)
	tree := fields[0]
	depth1 := tree.Children[1]   // first "children" field
	depth2 := depth1.Children[1] // second "children" field (shallow level 1)
	depth3 := depth2.Children[1] // third "children" field (shallow level 2)
	if len(depth3.Children) != 0 {
		t.Errorf("with limit=2, depth-3 children should be nil, got %d", len(depth3.Children))
	}
}

// ── item 6: embedded / inline promotion ──────────────────────────────────────

type embeddedBase struct {
	CreatedBy  string `yaml:"created-by"`
	VersionTag string `yaml:"version-tag"`
}

type inlineBase struct {
	Team    string `yaml:"team"`
	Contact string `yaml:"contact"`
}

type anonymousEmbedConfig struct {
	embeddedBase
	Port int `yaml:"port"`
}

type inlineEmbedConfig struct {
	inlineBase `yaml:",inline"`
	Port       int `yaml:"port"`
}

func TestDiscover_anonymousEmbed(t *testing.T) {
	fields := schema.Discover(&anonymousEmbedConfig{})
	got := schema.TopLevelOrder(fields)
	want := []string{"created-by", "version-tag", "port"}
	if len(got) != len(want) {
		t.Fatalf("TopLevelOrder = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestDiscover_inlineEmbed(t *testing.T) {
	fields := schema.Discover(&inlineEmbedConfig{})
	got := schema.TopLevelOrder(fields)
	want := []string{"team", "contact", "port"}
	if len(got) != len(want) {
		t.Fatalf("TopLevelOrder = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// ── item 7: omitempty / flow flags ───────────────────────────────────────────

type omitFlowConfig struct {
	Replicas int      `yaml:"replicas,omitempty"`
	Tags     []string `yaml:"tags,flow"`
	Name     string   `yaml:"name"`
}

func TestDiscover_omitEmpty(t *testing.T) {
	fields := schema.Discover(&omitFlowConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	if !m["replicas"].OmitEmpty {
		t.Error("replicas.OmitEmpty should be true")
	}
	if m["tags"].OmitEmpty {
		t.Error("tags.OmitEmpty should be false")
	}
	if m["name"].OmitEmpty {
		t.Error("name.OmitEmpty should be false")
	}
}

func TestDiscover_flow(t *testing.T) {
	fields := schema.Discover(&omitFlowConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	if !m["tags"].Flow {
		t.Error("tags.Flow should be true")
	}
	if m["replicas"].Flow {
		t.Error("replicas.Flow should be false")
	}
	if m["name"].Flow {
		t.Error("name.Flow should be false")
	}
}

// ── item 8: map key scalar ────────────────────────────────────────────────────

type intKeyConfig struct {
	ByPort map[int]string    `yaml:"by-port"`
	Labels map[string]string `yaml:"labels"`
}

func TestDiscover_mapKeyScalar(t *testing.T) {
	fields := schema.Discover(&intKeyConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	if m["by-port"].MapKeyScalar != "int" {
		t.Errorf("by-port.MapKeyScalar = %q, want %q", m["by-port"].MapKeyScalar, "int")
	}
	if m["labels"].MapKeyScalar != "string" {
		t.Errorf("labels.MapKeyScalar = %q, want %q", m["labels"].MapKeyScalar, "string")
	}
}

// ── item 9: MarshalYAML / TextMarshaler → KindPrimitive ──────────────────────

type colorType struct{ R, G, B uint8 }

func (c colorType) MarshalYAML() (any, error) {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B), nil
}

type ipType [4]byte

func (ip ipType) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])), nil
}

type marshalerConfig struct {
	Background colorType `yaml:"background"`
	Gateway    ipType    `yaml:"gateway"`
}

func TestDiscover_marshalerIsKindPrimitive(t *testing.T) {
	fields := schema.Discover(&marshalerConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	if m["background"].Kind != schema.KindPrimitive {
		t.Errorf("background.Kind = %v, want KindPrimitive", m["background"].Kind)
	}
	if len(m["background"].Children) != 0 {
		t.Errorf("background should have no children, got %d", len(m["background"].Children))
	}
	if m["gateway"].Kind != schema.KindPrimitive {
		t.Errorf("gateway.Kind = %v, want KindPrimitive", m["gateway"].Kind)
	}
	if len(m["gateway"].Children) != 0 {
		t.Errorf("gateway should have no children, got %d", len(m["gateway"].Children))
	}
}

// ── item 10: interface{}/any → KindAny ───────────────────────────────────────

type anyFieldConfig struct {
	Extras any    `yaml:"extras"`
	Name   string `yaml:"name"`
}

func TestDiscover_anyIsKindAny(t *testing.T) {
	fields := schema.Discover(&anyFieldConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	if m["extras"].Kind != schema.KindAny {
		t.Errorf("extras.Kind = %v, want KindAny", m["extras"].Kind)
	}
	if m["name"].Kind != schema.KindPrimitive {
		t.Errorf("name.Kind = %v, want KindPrimitive", m["name"].Kind)
	}
}
