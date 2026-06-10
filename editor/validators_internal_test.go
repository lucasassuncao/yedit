package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/schema"
)

// rfsConfig exercises RequiredFromSchema across the structural kinds: a
// top-level required scalar, a required field inside an optional object, a
// required field per sequence entry, and one per dictionary value.
type rfsConfig struct {
	Version string `yaml:"version" validate:"required"`
	Server  *struct {
		Host string `yaml:"host" validate:"required"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Workers []struct {
		Name string `yaml:"name" validate:"required"`
	} `yaml:"workers"`
	PortAttrs map[string]struct {
		Label string `yaml:"label" validate:"required"`
	} `yaml:"port-attrs"`
}

func wiredRequiredFromSchema(t *testing.T) Validator {
	t.Helper()
	v := RequiredFromSchema()
	v.(*requiredFromSchemaValidator).defs = schema.Discover(&rfsConfig{})
	return v
}

func TestRequiredFromSchema(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string // exact violation strings, in order
	}{
		{
			name: "unwired validator reports nothing",
			raw:  "",
			want: nil, // overridden below: uses the bare validator
		},
		{
			name: "empty document — only top-level required reported",
			raw:  "",
			want: []string{"version: required"},
		},
		{
			name: "all satisfied — ok",
			raw: `
version: 1.0.0
server:
  host: localhost
`,
			want: nil,
		},
		{
			name: "optional block absent — its required children not reported",
			raw:  "version: 1.0.0\n",
			want: nil,
		},
		{
			name: "optional block present without required child",
			raw: `
version: 1.0.0
server:
  port: 8080
`,
			want: []string{"server.host: required"},
		},
		{
			name: "sequence entries checked individually",
			raw: `
version: 1.0.0
workers:
  - name: a
  - queue: fast
`,
			want: []string{"workers[1].name: required"},
		},
		{
			name: "dictionary values checked individually",
			raw: `
version: 1.0.0
port-attrs:
  "8080":
    label: web
  "9090": {}
`,
			want: []string{`port-attrs.9090.label: required`},
		},
		{
			name: "empty scalar counts as missing",
			raw:  "version:\n",
			want: []string{"version: required"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := wiredRequiredFromSchema(t)
			if tc.name == "unwired validator reports nothing" {
				v = RequiredFromSchema()
			}
			var got []string
			for _, viol := range v.Validate([]byte(tc.raw), nil) {
				got = append(got, viol.String())
			}
			if len(got) != len(tc.want) {
				t.Fatalf("violations = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("violation[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestRequiredFromSchema_wiredByNewModel verifies that newModel injects the
// discovered schema into RequiredFromSchema validators, so a plain
// editor.RequiredFromSchema() in Config.Validators enforces the tags.
func TestRequiredFromSchema_wiredByNewModel(t *testing.T) {
	m, err := newModel(Config{
		Path:       filepath.Join(t.TempDir(), "missing.yaml"), // empty document
		Schema:     &rfsConfig{},
		Validators: []Validator{RequiredFromSchema()},
	})
	if err != nil {
		t.Fatal(err)
	}
	errs := m.collectErrors()
	found := false
	for _, e := range errs {
		if strings.Contains(e.String(), "version: required") {
			found = true
		}
	}
	if !found {
		t.Errorf("collectErrors should report the schema-required field; got %v", errs)
	}
}
