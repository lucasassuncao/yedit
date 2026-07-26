package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/spec"
)

// wireProbeConfig is the schema this test wires validators against. It lives
// here rather than in validate because the test drives newModel, an editor
// internal: the rules moved to yedit/validate, but the wiring that discovers a
// schema from an editor.Config did not.
type wireProbeConfig struct {
	Version string `yaml:"version"`
	Server  *struct {
		Host string `yaml:"host"`
	} `yaml:"server"`
}

// TestRequiredFromMetadata_wiredByNewModel verifies that newModel injects the
// discovered schema and the MetadataSource into FromMetadata validators, so a
// plain RequiredFromMetadata() in Config.Validators enforces the metadata
// markers without the caller wiring anything by hand.
func TestRequiredFromMetadata_wiredByNewModel(t *testing.T) {
	m, err := newModel(Config{
		Path:   filepath.Join(t.TempDir(), "missing.yaml"), // empty document
		Schema: &wireProbeConfig{},
		Metadata: spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
			return spec.FieldMeta{Required: block == "version" && fieldPath == ""}
		}),
		Validators: []spec.Validator{RequiredFromMetadata()},
	})
	if err != nil {
		t.Fatal(err)
	}
	errs := m.collectErrors(m.doc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.String(), "version: required") {
			found = true
		}
	}
	if !found {
		t.Errorf("collectErrors should report the metadata-required field; got %v", errs)
	}
}
