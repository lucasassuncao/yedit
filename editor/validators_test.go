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

func TestMutuallyExclusive_dottedPath_violation(t *testing.T) {
	raw := []byte("categories:\n  foo:\n    installers:\n      - name: bar\n        source:\n          filter:\n            any:\n              - categories: [x]\n            all:\n              - categories: [y]\n")
	v := editor.MutuallyExclusive(
		"categories.installers.source.filter.any",
		"categories.installers.source.filter.all",
	)
	errs := v.Validate(raw, nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if !strings.Contains(errs[0], "any") || !strings.Contains(errs[0], "all") {
		t.Errorf("violation should name both keys; got %q", errs[0])
	}
}

func TestMutuallyExclusive_dottedPath_ok(t *testing.T) {
	raw := []byte("categories:\n  foo:\n    installers:\n      - name: bar\n        source:\n          filter:\n            any:\n              - categories: [x]\n")
	v := editor.MutuallyExclusive(
		"categories.installers.source.filter.any",
		"categories.installers.source.filter.all",
	)
	if errs := v.Validate(raw, nil); len(errs) != 0 {
		t.Errorf("expected no violations, got %v", errs)
	}
}

func TestMutuallyExclusive_dottedPath_multipleInstallers(t *testing.T) {
	// Two installer entries; only the second violates the rule.
	raw := []byte(strings.Join([]string{
		"categories:",
		"  foo:",
		"    installers:",
		"      - name: ok",
		"        source:",
		"          filter:",
		"            any:",
		"              - categories: [x]",
		"      - name: bad",
		"        source:",
		"          filter:",
		"            any:",
		"              - categories: [x]",
		"            all:",
		"              - categories: [y]",
	}, "\n") + "\n")
	v := editor.MutuallyExclusive(
		"categories.installers.source.filter.any",
		"categories.installers.source.filter.all",
	)
	errs := v.Validate(raw, nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation (second installer only), got %v", errs)
	}
}

func TestMutuallyExclusive_topLevel_unchanged(t *testing.T) {
	blocks := []document.Block{{Key: "image"}, {Key: "build"}}
	v := editor.MutuallyExclusive("image", "build")
	if errs := v.Validate(nil, blocks); len(errs) != 1 {
		t.Errorf("top-level behavior should be unchanged, got %v", errs)
	}
}

func TestMutuallyExclusiveNested_topLevel(t *testing.T) {
	raw := []byte("filter:\n  any:\n    - categories: [foo]\n  all:\n    - categories: [bar]\n")
	v := editor.MutuallyExclusiveNested("filter", "any", "all")
	errs := v.Validate(raw, nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if !strings.Contains(errs[0], "any") || !strings.Contains(errs[0], "all") {
		t.Errorf("violation message should name both keys; got %q", errs[0])
	}
	if !strings.Contains(errs[0], "filter") {
		t.Errorf("violation message should include path; got %q", errs[0])
	}
}

func TestMutuallyExclusiveNested_ok_single(t *testing.T) {
	raw := []byte("filter:\n  any:\n    - categories: [foo]\n")
	v := editor.MutuallyExclusiveNested("filter", "any", "all")
	if errs := v.Validate(raw, nil); len(errs) != 0 {
		t.Errorf("expected no violations with only one key, got %v", errs)
	}
}

func TestMutuallyExclusiveNested_recursive(t *testing.T) {
	// Top-level filter is OK (only "any"); nested filter inside any[0] is invalid.
	raw := []byte(strings.Join([]string{
		"filter:",
		"  any:",
		"    - filter:",
		"        any:",
		"          - categories: [foo]",
		"        all:",
		"          - categories: [bar]",
	}, "\n") + "\n")
	v := editor.MutuallyExclusiveNested("filter", "any", "all")
	errs := v.Validate(raw, nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation (nested only), got %v", errs)
	}
	if !strings.Contains(errs[0], "any[0].filter") {
		t.Errorf("path in error should point to nested filter; got %q", errs[0])
	}
}

func TestMutuallyExclusiveNested_multipleViolations(t *testing.T) {
	// Both the top-level filter and a nested filter violate the rule.
	raw := []byte(strings.Join([]string{
		"filter:",
		"  any:",
		"    - filter:",
		"        any:",
		"          - categories: [foo]",
		"        all:",
		"          - categories: [bar]",
		"  all:",
		"    - categories: [baz]",
	}, "\n") + "\n")
	v := editor.MutuallyExclusiveNested("filter", "any", "all")
	errs := v.Validate(raw, nil)
	if len(errs) != 2 {
		t.Fatalf("expected 2 violations, got %v", errs)
	}
}

func TestMutuallyExclusiveNested_deeplyNestedPath(t *testing.T) {
	// filter buried under categories.installers[0].source
	raw := []byte(strings.Join([]string{
		"categories:",
		"  foo:",
		"    installers:",
		"      - name: bar",
		"        source:",
		"          filter:",
		"            any:",
		"              - categories: [x]",
		"            all:",
		"              - categories: [y]",
	}, "\n") + "\n")
	v := editor.MutuallyExclusiveNested("filter", "any", "all")
	errs := v.Validate(raw, nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if !strings.Contains(errs[0], "filter") {
		t.Errorf("path should mention filter; got %q", errs[0])
	}
}

func TestMutuallyExclusiveNested_scopedPath_violation(t *testing.T) {
	// Scoped to categories.installers.source.filter — should catch the violation there.
	raw := []byte(strings.Join([]string{
		"categories:",
		"  foo:",
		"    installers:",
		"      - name: bar",
		"        source:",
		"          filter:",
		"            any:",
		"              - categories: [x]",
		"            all:",
		"              - categories: [y]",
	}, "\n") + "\n")
	v := editor.MutuallyExclusiveNested("categories.installers.source.filter", "any", "all")
	errs := v.Validate(raw, nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if !strings.Contains(errs[0], "filter") {
		t.Errorf("path should mention filter; got %q", errs[0])
	}
}

func TestMutuallyExclusiveNested_scopedPath_ignoresOtherFilters(t *testing.T) {
	// A "filter" under some.other.object should NOT be caught when scoped to
	// categories.installers.source.filter.
	raw := []byte(strings.Join([]string{
		"categories:",
		"  foo:",
		"    installers:",
		"      - name: bar",
		"        source:",
		"          filter:",
		"            any:",
		"              - categories: [x]",
		"some:",
		"  other:",
		"    object:",
		"      filter:",
		"        any:",
		"          - categories: [x]",
		"        all:",
		"          - categories: [y]",
	}, "\n") + "\n")
	v := editor.MutuallyExclusiveNested("categories.installers.source.filter", "any", "all")
	// Only the categories.installers.source.filter is checked; some.other.object.filter is ignored.
	if errs := v.Validate(raw, nil); len(errs) != 0 {
		t.Errorf("scoped validator must not fire on filters outside the scope; got %v", errs)
	}
}
