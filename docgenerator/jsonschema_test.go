package docgenerator

import (
	"encoding/json"
	"testing"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/spec"
)

type jsScalars struct {
	Name    string  `yaml:"name"`
	Port    int     `yaml:"port"`
	Ratio   float64 `yaml:"ratio"`
	Debug   bool    `yaml:"debug"`
	Retries uint    `yaml:"retries"`
}

// buildRoot is the in-memory half of emitJSONSchema, shared by these tests.
func buildRoot(t *testing.T, cfg any, meta spec.MetadataSource) map[string]any {
	t.Helper()
	return newJSONSchemaBuilder(meta).root(schema.Discover(cfg))
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}

func TestJSONSchema_scalarTypes(t *testing.T) {
	got := mustJSON(t, buildRoot(t, jsScalars{}, nil))
	want := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "properties": {
    "debug": {
      "type": "boolean"
    },
    "name": {
      "type": "string"
    },
    "port": {
      "type": "integer"
    },
    "ratio": {
      "type": "number"
    },
    "retries": {
      "minimum": 0,
      "type": "integer"
    }
  },
  "type": "object"
}`
	if got != want {
		t.Fatalf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

type jsInner struct {
	Host string `yaml:"host"`
}

type jsNesting struct {
	Server  jsInner            `yaml:"server"`
	Tags    []string           `yaml:"tags"`
	Servers []jsInner          `yaml:"servers"`
	ByName  map[string]jsInner `yaml:"by_name"`
	ByIndex map[int]string     `yaml:"by_index"`
	Free    any                `yaml:"free"`
}

func TestJSONSchema_nestingAndCollections(t *testing.T) {
	got := mustJSON(t, buildRoot(t, jsNesting{}, nil))
	want := `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "properties": {
    "by_index": {
      "additionalProperties": {
        "type": "string"
      },
      "propertyNames": {
        "type": "integer"
      },
      "type": "object"
    },
    "by_name": {
      "additionalProperties": {
        "properties": {
          "host": {
            "type": "string"
          }
        },
        "type": "object"
      },
      "type": "object"
    },
    "free": {},
    "server": {
      "properties": {
        "host": {
          "type": "string"
        }
      },
      "type": "object"
    },
    "servers": {
      "items": {
        "properties": {
          "host": {
            "type": "string"
          }
        },
        "type": "object"
      },
      "type": "array"
    },
    "tags": {
      "items": {
        "type": "string"
      },
      "type": "array"
    }
  },
  "type": "object"
}`
	if got != want {
		t.Fatalf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

type jsTreeNode struct {
	Label    string       `yaml:"label"`
	Children []jsTreeNode `yaml:"children"`
}

type jsTreeRoot struct {
	Tree jsTreeNode `yaml:"tree"`
}

func TestJSONSchema_recursiveTypeBecomesDefRef(t *testing.T) {
	got := mustJSON(t, buildRoot(t, jsTreeRoot{}, nil))
	want := `{
  "$defs": {
    "jsTreeNode": {
      "properties": {
        "children": {
          "items": {
            "$ref": "#/$defs/jsTreeNode"
          },
          "type": "array"
        },
        "label": {
          "type": "string"
        }
      },
      "type": "object"
    }
  },
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "properties": {
    "tree": {
      "$ref": "#/$defs/jsTreeNode"
    }
  },
  "type": "object"
}`
	if got != want {
		t.Fatalf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestJSONSchema_recursionIsIndependentOfRecursionLimit(t *testing.T) {
	strict := newJSONSchemaBuilder(nil).root(schema.Discover(jsTreeRoot{}, 0))
	deep := newJSONSchemaBuilder(nil).root(schema.Discover(jsTreeRoot{}, 3))
	if mustJSON(t, strict) != mustJSON(t, deep) {
		t.Fatalf("recursion limit changed the schema:\nlimit 0:\n%s\n\nlimit 3:\n%s",
			mustJSON(t, strict), mustJSON(t, deep))
	}
}

func TestJSONSchema_noDefsWhenNoRecursion(t *testing.T) {
	doc := buildRoot(t, jsNesting{}, nil)
	if _, ok := doc["$defs"]; ok {
		t.Fatalf("$defs must be omitted when nothing recurses: %s", mustJSON(t, doc))
	}
}

type jsMeta struct {
	Mode     string   `yaml:"mode"`
	Endpoint string   `yaml:"endpoint"`
	Version  string   `yaml:"version"`
	Timeout  string   `yaml:"timeout"`
	Weight   int      `yaml:"weight"`
	Tags     []string `yaml:"tags"`
	Legacy   string   `yaml:"legacy"`
}

// staticMeta answers with a fixed FieldMeta per top-level field name.
type staticMeta map[string]spec.FieldMeta

func (s staticMeta) FieldMeta(blockKey, fieldPath string) spec.FieldMeta {
	if fieldPath != "" {
		return spec.FieldMeta{}
	}
	return s[blockKey]
}

func propOf(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatalf("document has no properties: %s", mustJSON(t, doc))
	}
	node, ok := props[name].(map[string]any)
	if !ok {
		t.Fatalf("property %q missing: %s", name, mustJSON(t, doc))
	}
	return node
}

func TestJSONSchema_fieldMetaKeywords(t *testing.T) {
	meta := staticMeta{
		"mode": {
			Description: "Operating mode.",
			Required:    true,
			Default:     "fast",
			OneOf:       []string{"fast", "safe"},
			NotOneOf:    []string{"legacy"},
		},
		"endpoint": {Formats: []spec.Format{spec.FormatURL}},
		"version":  {Pattern: `^v\d+$`, MinLength: 2, MaxLength: 10, Example: "v1"},
		"weight":   {Min: "1", Max: "100"},
		"tags":     {MinCount: 1, MaxCount: 5, Unique: true},
		"legacy":   {Deprecated: "Use mode instead."},
	}
	doc := buildRoot(t, jsMeta{}, meta)

	required, ok := doc["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "mode" {
		t.Fatalf("want required [mode], got %v", doc["required"])
	}

	mode := propOf(t, doc, "mode")
	if mode["description"] != "Operating mode." {
		t.Fatalf("description: %v", mode["description"])
	}
	if mode["default"] != "fast" {
		t.Fatalf("default: %v", mode["default"])
	}
	if got := mustJSON(t, mode["enum"]); got != "[\n  \"fast\",\n  \"safe\"\n]" {
		t.Fatalf("enum: %s", got)
	}
	if got := mustJSON(t, mode["not"]); got != "{\n  \"enum\": [\n    \"legacy\"\n  ]\n}" {
		t.Fatalf("not: %s", got)
	}

	if got := propOf(t, doc, "endpoint")["format"]; got != "uri" {
		t.Fatalf("format: %v", got)
	}

	version := propOf(t, doc, "version")
	if version["pattern"] != `^v\d+$` {
		t.Fatalf("pattern: %v", version["pattern"])
	}
	if version["minLength"] != 2 || version["maxLength"] != 10 {
		t.Fatalf("length: %v %v", version["minLength"], version["maxLength"])
	}
	if got := mustJSON(t, version["examples"]); got != "[\n  \"v1\"\n]" {
		t.Fatalf("examples: %s", got)
	}

	weight := propOf(t, doc, "weight")
	if weight["minimum"] != float64(1) || weight["maximum"] != float64(100) {
		t.Fatalf("bounds: %v %v", weight["minimum"], weight["maximum"])
	}

	tags := propOf(t, doc, "tags")
	if tags["minItems"] != 1 || tags["maxItems"] != 5 || tags["uniqueItems"] != true {
		t.Fatalf("collection: %v", mustJSON(t, tags))
	}

	legacy := propOf(t, doc, "legacy")
	if legacy["deprecated"] != true || legacy["$comment"] != "Use mode instead." {
		t.Fatalf("deprecated: %v", mustJSON(t, legacy))
	}
}

func TestJSONSchema_nonNumericBoundsFallBackToDescription(t *testing.T) {
	meta := staticMeta{
		"timeout": {Description: "Request timeout.", Min: "30s", Max: "5m"},
	}
	got := propOf(t, buildRoot(t, jsMeta{}, meta), "timeout")
	if _, ok := got["minimum"]; ok {
		t.Fatalf("a duration must not become a numeric bound: %s", mustJSON(t, got))
	}
	want := "Request timeout. Min: 30s Max: 5m"
	if got["description"] != want {
		t.Fatalf("want description %q, got %v", want, got["description"])
	}
}

func TestJSONSchema_unmappedFormatFallsBackToDescription(t *testing.T) {
	meta := staticMeta{
		"version": {Formats: []spec.Format{spec.FormatSemver}},
	}
	got := propOf(t, buildRoot(t, jsMeta{}, meta), "version")
	if _, ok := got["format"]; ok {
		t.Fatalf("semver has no standard format keyword: %s", mustJSON(t, got))
	}
	if got["description"] != "Format: semver" {
		t.Fatalf("want format note in description, got %v", got["description"])
	}
}

func TestJSONSchema_multipleMappedFormatsBecomeAnyOf(t *testing.T) {
	meta := staticMeta{
		"endpoint": {Formats: []spec.Format{spec.FormatIPv4, spec.FormatIPv6}},
	}
	got := propOf(t, buildRoot(t, jsMeta{}, meta), "endpoint")
	want := "[\n  {\n    \"format\": \"ipv4\"\n  },\n  {\n    \"format\": \"ipv6\"\n  }\n]"
	if mustJSON(t, got["anyOf"]) != want {
		t.Fatalf("anyOf: %s", mustJSON(t, got["anyOf"]))
	}
	if _, ok := got["format"]; ok {
		t.Fatalf("format and anyOf are mutually exclusive: %s", mustJSON(t, got))
	}
}
