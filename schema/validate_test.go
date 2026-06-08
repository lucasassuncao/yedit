package schema_test

import (
	"testing"

	"github.com/lucasassuncao/yedit/schema"
)

type vscodeCustomization struct {
	Extensions []string       `yaml:"extensions"`
	Settings   map[string]any `yaml:"settings"`
}

type customizationsBlock struct {
	Vscode *vscodeCustomization `yaml:"vscode"`
}

type buildConfig struct {
	Dockerfile string            `yaml:"dockerfile"`
	Context    string            `yaml:"context"`
	Args       map[string]string `yaml:"args"`
}

type sampleConfig struct {
	Name           string               `yaml:"name"`
	Image          string               `yaml:"image"`
	Build          *buildConfig         `yaml:"build"`
	Customizations *customizationsBlock `yaml:"customizations"`
}

func TestUnknownKeys(t *testing.T) {
	known := schema.KnownChildren(schema.Discover(&sampleConfig{}))

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "clean doc",
			raw: `
name: mydev
image: ubuntu:22.04
`,
			want: nil,
		},
		{
			name: "top-level typo",
			raw: `
name: mydev
customization: bad
`,
			want: []string{"customization"},
		},
		{
			name: "sub-key typo",
			raw: `
customizations:
  vscod:
    extensions:
      - foo.bar
`,
			want: []string{"customizations.vscod"},
		},
		{
			name: "valid sub-key",
			raw: `
customizations:
  vscode:
    extensions:
      - foo.bar
`,
			want: nil,
		},
		{
			name: "free-form settings keys",
			raw: `
customizations:
  vscode:
    extensions:
      - foo.bar
    settings:
      editor.formatOnSave: true
      any.arbitrary.key: 42
`,
			want: nil,
		},
		{
			name: "free-form args keys",
			raw: `
build:
  dockerfile: Dockerfile
  args:
    MY_ARG: value
    OTHER_ARG: x
`,
			want: nil,
		},
		{
			name: "build sub-key typo",
			raw: `
build:
  dockerfilee: Dockerfile
  context: .
`,
			want: []string{"build.dockerfilee"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := schema.UnknownKeys([]byte(tc.raw), known)
			if len(got) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Errorf("[%d] want %q, got %q", i, w, got[i])
				}
			}
		})
	}
}

type portAttr struct {
	Label         string `yaml:"label"`
	OnAutoForward string `yaml:"onAutoForward"`
}

type mapOfStructConfig struct {
	PortsAttributes map[string]*portAttr `yaml:"portsAttributes"`
}

// TestUnknownKeys_mapOfStructKeysAreFreeForm verifies that the keys of a
// map[string]*Struct field are not validated against the value-struct's field
// names (they are user-chosen, e.g. port specs).
func TestUnknownKeys_mapOfStructKeysAreFreeForm(t *testing.T) {
	known := schema.KnownChildren(schema.Discover(&mapOfStructConfig{}))
	raw := `
portsAttributes:
  "3000":
    label: web
  lucas:
    onAutoForward: notify
`
	if u := schema.UnknownKeys([]byte(raw), known); len(u) != 0 {
		t.Errorf("map keys must be free-form; got unknown: %v", u)
	}
}
