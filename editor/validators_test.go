package editor_test

import (
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/editor"
)

func TestMutuallyExclusive_violation(t *testing.T) {
	blocks := []document.Block{{Key: "image"}, {Key: "build"}}
	v := editor.MutuallyExclusive("image", "build", "dockerComposeFile")
	errs := v.Validate(nil, blocks)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if !strings.Contains(errs[0], "image") || !strings.Contains(errs[0], "build") {
		t.Errorf("violation message should name both keys; got %q", errs[0])
	}
}

func TestMutuallyExclusive_single_ok(t *testing.T) {
	blocks := []document.Block{{Key: "image"}}
	v := editor.MutuallyExclusive("image", "build", "dockerComposeFile")
	if errs := v.Validate(nil, blocks); len(errs) != 0 {
		t.Errorf("expected no violations, got %v", errs)
	}
}

func TestMutuallyExclusive_none_ok(t *testing.T) {
	blocks := []document.Block{{Key: "name"}}
	v := editor.MutuallyExclusive("image", "build", "dockerComposeFile")
	if errs := v.Validate(nil, blocks); len(errs) != 0 {
		t.Errorf("expected no violations when neither key is present, got %v", errs)
	}
}

func TestRequiredWith_violation(t *testing.T) {
	blocks := []document.Block{{Key: "service"}}
	v := editor.RequiredWith("service", "dockerComposeFile")
	errs := v.Validate(nil, blocks)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if !strings.Contains(errs[0], "service") || !strings.Contains(errs[0], "dockerComposeFile") {
		t.Errorf("violation message should reference both keys; got %q", errs[0])
	}
}

func TestRequiredWith_satisfied(t *testing.T) {
	blocks := []document.Block{{Key: "service"}, {Key: "dockerComposeFile"}}
	v := editor.RequiredWith("service", "dockerComposeFile")
	if errs := v.Validate(nil, blocks); len(errs) != 0 {
		t.Errorf("expected no violations, got %v", errs)
	}
}

func TestRequiredWith_absent_key_ok(t *testing.T) {
	// service is not present; RequiredWith should not complain about parent.
	blocks := []document.Block{{Key: "name"}}
	v := editor.RequiredWith("service", "dockerComposeFile")
	if errs := v.Validate(nil, blocks); len(errs) != 0 {
		t.Errorf("expected no violations when key absent, got %v", errs)
	}
}
