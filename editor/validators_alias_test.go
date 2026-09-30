package editor

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/validate"
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

// TestWire_matchesHeadlessWire pins editor.Wire and validate.Wire to the same
// result for a Config with no Hidden filter. That equivalence is the reason
// validate.Wire exists: a lint command must be able to skip the TUI without
// its rules quietly diverging from the ones the editor enforces. If the two
// ever resolve the schema differently, this fails.
func TestWire_matchesHeadlessWire(t *testing.T) {
	meta := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		return spec.FieldMeta{Required: block == "version" && fieldPath == ""}
	})
	validators := []spec.Validator{RequiredFromMetadata()}

	raw := []byte("server:\n  host: localhost\n")
	blocks, err := document.ParseBlocks(raw)
	if err != nil {
		t.Fatal(err)
	}

	fromEditor := validate.RunAll(
		Wire(validators, Config{Schema: &wireProbeConfig{}, Metadata: meta}),
		raw, blocks)
	fromHeadless := validate.RunAll(
		validate.Wire(validators, &wireProbeConfig{}, 0, meta),
		raw, blocks)

	if len(fromEditor) == 0 {
		t.Fatal("the probe config should produce a violation, otherwise this test proves nothing")
	}
	if !reflect.DeepEqual(fromEditor, fromHeadless) {
		t.Errorf("editor.Wire and validate.Wire disagree:\n editor:   %v\n headless: %v", fromEditor, fromHeadless)
	}
}

// TestWire_respectsRecursionDepth verifies validate.Wire forwards depth the way
// Config.SchemaRecursionDepth does: non-positive means the default, not strict.
func TestWire_respectsRecursionDepth(t *testing.T) {
	meta := spec.MetadataFunc(func(string, string) spec.FieldMeta { return spec.FieldMeta{} })
	validators := []spec.Validator{RequiredFromMetadata()}

	raw := []byte("version: 1\n")
	blocks, err := document.ParseBlocks(raw)
	if err != nil {
		t.Fatal(err)
	}

	zero := validate.RunAll(validate.Wire(validators, &wireProbeConfig{}, 0, meta), raw, blocks)
	explicit := validate.RunAll(validate.Wire(validators, &wireProbeConfig{}, 1, meta), raw, blocks)

	if !reflect.DeepEqual(zero, explicit) {
		t.Errorf("depth 0 should select the default of 1, got %v vs %v", zero, explicit)
	}
}
