package schema_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

type basicConfig struct {
	Name    string       `yaml:"name"`
	Image   string       `yaml:"image"`
	Mode    string       `yaml:"mode"`
	Build   *buildConfig `yaml:"build"`
	Skipped string       // no yaml tag
	Hidden  string       `yaml:"-"`
	Meta    string       `yaml:"$schema"`
}

func TestDiscover_topLevelFields(t *testing.T) {
	is := assert.New(t)
	fields := schema.Discover(&basicConfig{})
	got := schema.TopLevelOrder(fields)
	want := []string{"name", "image", "mode", "build"}
	is.Equal(want, got)
}

func TestDiscover_descents(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	fields := schema.Discover(&basicConfig{})
	var build schema.FieldDef
	for _, f := range fields {
		if f.YAMLName == "build" {
			build = f
		}
	}
	must.NotEmpty(build.Children, "build should have children discovered from buildConfig")
	names := make([]string, len(build.Children))
	for i, c := range build.Children {
		names[i] = c.YAMLName
	}
	is.Equal([]string{"dockerfile", "context", "args"}, names)
}

// unionItem opts into Provider to declare its own schema.
type unionItem struct{}

func (unionItem) YeditSchema() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "type", Kind: schema.KindPrimitive},
		{YAMLName: "target", Kind: schema.KindPrimitive},
	}
}

type configWithUnion struct {
	Items []unionItem `yaml:"items"`
}

// minimalConfig has only yaml tags.
type minimalConfig struct {
	Name    string         `yaml:"name"`
	Port    int            `yaml:"port"`
	Nested  *minimalNested `yaml:"nested"`
	Skipped string         // no yaml tag - must be omitted
}

type minimalNested struct {
	Host string `yaml:"host"`
	Tls  bool   `yaml:"tls"`
}

func TestDiscover_yamlTagOnly(t *testing.T) {
	is := assert.New(t)
	fields := schema.Discover(&minimalConfig{})

	got := schema.TopLevelOrder(fields)
	want := []string{"name", "port", "nested"}
	is.Equal(want, got)

	// Nested struct still descends.
	var nested schema.FieldDef
	for _, f := range fields {
		if f.YAMLName == "nested" {
			nested = f
		}
	}
	if is.Len(nested.Children, 2) {
		is.Equal("host", nested.Children[0].YAMLName)
		is.Equal("tls", nested.Children[1].YAMLName)
	}
}

func TestDiscover_providerOverridesReflection(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	fields := schema.Discover(&configWithUnion{})
	must.Len(fields, 1, "expected single field 'items'")
	is.Equal("items", fields[0].YAMLName)
	is.Equal(schema.KindVariant, fields[0].Kind)
	if is.Len(fields[0].Children, 2, "expected 2 children from Provider") {
		is.Equal("type", fields[0].Children[0].YAMLName)
		is.Equal("target", fields[0].Children[1].YAMLName)
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
	Mode string        `yaml:"mode"`
	Tags []string      `yaml:"tags"`
}

func TestDiscover_scalarType(t *testing.T) {
	is := assert.New(t)
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
		"mode": "string",
		"tags": "", // a slice is not a scalar
	}
	for name, w := range want {
		is.Equal(w, got[name], "field %q Scalar", name)
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
	is := assert.New(t)
	must := require.New(t)
	// Default limit=1: one extra recursive level beyond the first visit.
	// tree → selfRefNode (visit 1): has children [name, children]
	// tree.children → selfRefNode (visit 2, shallow): has children [name, children]
	// tree.children.children → selfRefNode (visit 3 > limit=1): nil
	fields := schema.Discover(&selfRefRoot{})
	must.Len(fields, 1, "expected single field 'tree'")
	is.Equal("tree", fields[0].YAMLName)
	tree := fields[0]
	must.Len(tree.Children, 2, "tree.Children want 2 (name + children)")
	// The "children" field at the first level has one shallow level of children.
	childrenField := tree.Children[1]
	is.Len(childrenField.Children, 2, "children at depth 1 should have 2 children (shallow level)")
	// The "children" field at the second level (shallow) must be nil - cycle stopped.
	if len(childrenField.Children) >= 2 {
		deepChildrenField := childrenField.Children[1]
		is.Empty(deepChildrenField.Children, "children at depth 2 should have no children (cycle blocked)")
	}
}

func TestDiscover_recursiveTypeLimit0(t *testing.T) {
	is := assert.New(t)
	// Explicit limit=0: no recursive expansion (original strict cycle detection).
	fields := schema.Discover(&selfRefRoot{}, 0)
	tree := fields[0]
	childrenField := tree.Children[1]
	is.Empty(childrenField.Children, "with limit=0, recursive children should be nil")
}

func TestDiscover_recursiveTypeLimit2(t *testing.T) {
	is := assert.New(t)
	// Explicit limit=2: two extra levels of recursion.
	fields := schema.Discover(&selfRefRoot{}, 2)
	tree := fields[0]
	depth1 := tree.Children[1]   // first "children" field
	depth2 := depth1.Children[1] // second "children" field (shallow level 1)
	depth3 := depth2.Children[1] // third "children" field (shallow level 2)
	is.Empty(depth3.Children, "with limit=2, depth-3 children should be nil")
}

// ── item 6: embedded / inline promotion ──────────────────────────────────────

// EmbeddedBase is exported: yaml.v3 marshals a bare exported embed under the
// lowercased type name instead of inlining its fields.
type EmbeddedBase struct {
	CreatedBy  string `yaml:"created-by"`
	VersionTag string `yaml:"version-tag"`
}

type embeddedBase struct {
	CreatedBy  string `yaml:"created-by"`
	VersionTag string `yaml:"version-tag"`
}

type inlineBase struct {
	Team    string `yaml:"team"`
	Contact string `yaml:"contact"`
}

type anonymousEmbedConfig struct {
	EmbeddedBase
	Port int `yaml:"port"`
}

type unexportedEmbedConfig struct {
	embeddedBase     // presence is the point: yaml.v3 cannot marshal a bare unexported embed
	Port         int `yaml:"port"`
}

type inlineEmbedConfig struct {
	inlineBase `yaml:",inline"`
	Port       int `yaml:"port"`
}

func TestDiscover_anonymousEmbed(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	fields := schema.Discover(&anonymousEmbedConfig{})
	got := schema.TopLevelOrder(fields)
	// yaml.v3 does not inline a bare anonymous embed: it marshals it under
	// the lowercased type name, like a named field. Cross-checked below.
	want := []string{"embeddedbase", "port"}
	is.Equal(want, got)
	must.NotEmpty(fields)
	is.Equal([]string{"created-by", "version-tag"}, schema.TopLevelOrder(fields[0].Children))

	out, err := yaml.Marshal(anonymousEmbedConfig{EmbeddedBase{"me", "v1"}, 8080})
	must.NoError(err)
	var doc map[string]any
	must.NoError(yaml.Unmarshal(out, &doc))
	is.Contains(doc, "embeddedbase", "yaml.v3 must key the embed by its lowercased type name")
	is.Contains(doc, "port")
}

func TestDiscover_unexportedBareEmbedOmitted(t *testing.T) {
	is := assert.New(t)
	// yaml.v3 panics when marshaling a bare unexported embed, so the schema
	// must omit it entirely.
	fields := schema.Discover(&unexportedEmbedConfig{})
	is.Equal([]string{"port"}, schema.TopLevelOrder(fields))
}

func TestDiscover_inlineEmbed(t *testing.T) {
	is := assert.New(t)
	fields := schema.Discover(&inlineEmbedConfig{})
	got := schema.TopLevelOrder(fields)
	want := []string{"team", "contact", "port"}
	is.Equal(want, got)
}

// ── item 7: omitempty / flow flags ───────────────────────────────────────────

type omitFlowConfig struct {
	Replicas int      `yaml:"replicas,omitempty"`
	Tags     []string `yaml:"tags,flow"`
	Name     string   `yaml:"name"`
}

func TestDiscover_omitEmpty(t *testing.T) {
	is := assert.New(t)
	fields := schema.Discover(&omitFlowConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	is.True(m["replicas"].OmitEmpty, "replicas.OmitEmpty should be true")
	is.False(m["tags"].OmitEmpty, "tags.OmitEmpty should be false")
	is.False(m["name"].OmitEmpty, "name.OmitEmpty should be false")
}

func TestDiscover_flow(t *testing.T) {
	is := assert.New(t)
	fields := schema.Discover(&omitFlowConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	is.True(m["tags"].Flow, "tags.Flow should be true")
	is.False(m["replicas"].Flow, "replicas.Flow should be false")
	is.False(m["name"].Flow, "name.Flow should be false")
}

// ── item 8: map key scalar ────────────────────────────────────────────────────

type intKeyConfig struct {
	ByPort map[int]string    `yaml:"by-port"`
	Labels map[string]string `yaml:"labels"`
}

func TestDiscover_mapKeyScalar(t *testing.T) {
	is := assert.New(t)
	fields := schema.Discover(&intKeyConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	is.Equal("int", m["by-port"].MapKeyScalar)
	is.Equal("string", m["labels"].MapKeyScalar)
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
	is := assert.New(t)
	fields := schema.Discover(&marshalerConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	is.Equal(schema.KindPrimitive, m["background"].Kind)
	is.Empty(m["background"].Children, "background should have no children")
	is.Equal(schema.KindPrimitive, m["gateway"].Kind)
	is.Empty(m["gateway"].Children, "gateway should have no children")
}

// ── map-wrapped Provider elements ─────────────────────────────────────────────

// mapUnionItem implements Provider with a value receiver, so calling
// YeditSchema on a typed nil pointer would panic.
type mapUnionItem struct{}

func (mapUnionItem) YeditSchema() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "kind", Kind: schema.KindPrimitive},
	}
}

type mapProviderConfig struct {
	ByName map[string]*mapUnionItem  `yaml:"by-name"`
	Groups map[string][]mapUnionItem `yaml:"groups"`
}

func TestDiscover_mapOfProviderPointer(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	// map[string]*T where T implements Provider with a value receiver used to
	// panic (typed nil receiver). It must resolve the provider's schema.
	fields := schema.Discover(&mapProviderConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	is.Equal(schema.KindVariant, m["by-name"].Kind)
	must.Len(m["by-name"].Children, 1)
	is.Equal("kind", m["by-name"].Children[0].YAMLName)
}

func TestDiscover_mapOfProviderSlice(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	// map[string][]T used to never reach T (silent false negative).
	fields := schema.Discover(&mapProviderConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	is.Equal(schema.KindVariant, m["groups"].Kind)
	must.Len(m["groups"].Children, 1)
	is.Equal("kind", m["groups"].Children[0].YAMLName)
}

// ── name-less tag with options ───────────────────────────────────────────────

type namelessTagConfig struct {
	Replicas int    `yaml:",omitempty"`
	Name     string `yaml:"name"`
}

func TestDiscover_namelessTagFallsBackToFieldName(t *testing.T) {
	is := assert.New(t)
	// yaml.v3 keys a tag with options but no name (yaml:",omitempty") by the
	// lowercased field name; it must not be dropped from the schema.
	fields := schema.Discover(&namelessTagConfig{})
	got := schema.TopLevelOrder(fields)
	is.Equal([]string{"replicas", "name"}, got)
	is.True(fields[0].OmitEmpty, "replicas.OmitEmpty should be true")
}

// ── item 10: interface{}/any → KindAny ───────────────────────────────────────

type anyFieldConfig struct {
	Extras any    `yaml:"extras"`
	Name   string `yaml:"name"`
}

func TestDiscover_anyIsKindAny(t *testing.T) {
	is := assert.New(t)
	fields := schema.Discover(&anyFieldConfig{})
	m := map[string]schema.FieldDef{}
	for _, f := range fields {
		m[f.YAMLName] = f
	}
	is.Equal(schema.KindAny, m["extras"].Kind)
	is.Equal(schema.KindPrimitive, m["name"].Kind)
}
