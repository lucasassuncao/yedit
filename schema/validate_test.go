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

func known(t *testing.T) map[string]map[string]bool {
	t.Helper()
	return schema.KnownChildren(schema.Discover(&sampleConfig{}))
}

func TestUnknownKeys_clean(t *testing.T) {
	raw := []byte("name: mydev\nimage: ubuntu:22.04\n")
	if u := schema.UnknownKeys(raw, known(t)); len(u) != 0 {
		t.Errorf("expected no unknown keys, got %v", u)
	}
}

func TestUnknownKeys_topLevelTypo(t *testing.T) {
	raw := []byte("name: mydev\ncustomization: bad\n")
	u := schema.UnknownKeys(raw, known(t))
	if len(u) != 1 || u[0] != "customization" {
		t.Errorf("expected [customization], got %v", u)
	}
}

func TestUnknownKeys_subKeyTypo(t *testing.T) {
	raw := []byte("customizations:\n  vscod:\n    extensions:\n      - foo.bar\n")
	u := schema.UnknownKeys(raw, known(t))
	if len(u) != 1 || u[0] != "customizations.vscod" {
		t.Errorf("expected [customizations.vscod], got %v", u)
	}
}

func TestUnknownKeys_validSubKey(t *testing.T) {
	raw := []byte("customizations:\n  vscode:\n    extensions:\n      - foo.bar\n")
	if u := schema.UnknownKeys(raw, known(t)); len(u) != 0 {
		t.Errorf("expected no unknown keys, got %v", u)
	}
}

func TestUnknownKeys_freeFormSettings(t *testing.T) {
	// Settings is a map, so its nested keys must not be validated.
	raw := []byte("customizations:\n  vscode:\n    extensions:\n      - foo.bar\n    settings:\n      editor.formatOnSave: true\n      any.arbitrary.key: 42\n")
	if u := schema.UnknownKeys(raw, known(t)); len(u) != 0 {
		t.Errorf("expected no errors for free-form settings, got %v", u)
	}
}

func TestUnknownKeys_freeFormArgs(t *testing.T) {
	// Args is a map[string]string — its keys must be accepted.
	raw := []byte("build:\n  dockerfile: Dockerfile\n  args:\n    MY_ARG: value\n    OTHER_ARG: x\n")
	if u := schema.UnknownKeys(raw, known(t)); len(u) != 0 {
		t.Errorf("expected no errors for free-form args, got %v", u)
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
	raw := []byte("portsAttributes:\n  \"3000\":\n    label: web\n  lucas:\n    onAutoForward: notify\n")
	if u := schema.UnknownKeys(raw, known); len(u) != 0 {
		t.Errorf("map keys must be free-form; got unknown: %v", u)
	}
}

func TestUnknownKeys_buildSubKeyTypo(t *testing.T) {
	raw := []byte("build:\n  dockerfilee: Dockerfile\n  context: .\n")
	u := schema.UnknownKeys(raw, known(t))
	if len(u) != 1 || u[0] != "build.dockerfilee" {
		t.Errorf("expected [build.dockerfilee], got %v", u)
	}
}
