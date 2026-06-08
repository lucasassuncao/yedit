package editor_test

import (
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/editor"
)

func TestMutuallyExclusive(t *testing.T) {
	v := editor.MutuallyExclusive("image", "build", "dockerComposeFile")

	tests := []struct {
		name          string
		blocks        []document.Block
		wantViolation bool
		wantContains  []string
	}{
		{
			name:          "two keys present — violation",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}},
			wantViolation: true,
			wantContains:  []string{"image", "build"},
		},
		{
			name:          "only one key — ok",
			blocks:        []document.Block{{Key: "image"}},
			wantViolation: false,
		},
		{
			name:          "none of the keys — ok",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(nil, tc.blocks)
			if tc.wantViolation && len(errs) != 1 {
				t.Fatalf("expected 1 violation, got %v", errs)
			}
			if !tc.wantViolation && len(errs) != 0 {
				t.Errorf("expected no violations, got %v", errs)
			}
			for _, want := range tc.wantContains {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("violation message should contain %q; got %q", want, errs[0])
				}
			}
		})
	}
}

func TestMutuallyExclusive_dottedPath(t *testing.T) {
	v := editor.MutuallyExclusive(
		"categories.installers.source.filter.any",
		"categories.installers.source.filter.all",
	)

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
	}{
		{
			name: "both keys in filter — violation",
			raw: `
categories:
  foo:
    installers:
      - name: bar
        source:
          filter:
            any:
              - categories: [x]
            all:
              - categories: [y]
`,
			wantViolation: true,
		},
		{
			name: "only one key — ok",
			raw: `
categories:
  foo:
    installers:
      - name: bar
        source:
          filter:
            any:
              - categories: [x]
`,
			wantViolation: false,
		},
		{
			name: "multiple installers — second violates",
			raw: `
categories:
  foo:
    installers:
      - name: ok
        source:
          filter:
            any:
              - categories: [x]
      - name: bad
        source:
          filter:
            any:
              - categories: [x]
            all:
              - categories: [y]
`,
			wantViolation: true,
		},
		{
			name:          "empty document — ok",
			raw:           "",
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate([]byte(tc.raw), nil)
			if tc.wantViolation && len(errs) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(errs) != 0 {
				t.Errorf("expected no violations, got %v", errs)
			}
		})
	}
}

func TestMutuallyExclusive_topLevel_unchanged(t *testing.T) {
	blocks := []document.Block{{Key: "image"}, {Key: "build"}}
	v := editor.MutuallyExclusive("image", "build")
	if errs := v.Validate(nil, blocks); len(errs) != 1 {
		t.Errorf("top-level behavior should be unchanged, got %v", errs)
	}
}

func TestRequiredWith(t *testing.T) {
	v := editor.RequiredWith("service", "dockerComposeFile")

	tests := []struct {
		name          string
		blocks        []document.Block
		wantViolation bool
		wantContains  []string
	}{
		{
			name:          "key present without parent — violation",
			blocks:        []document.Block{{Key: "service"}},
			wantViolation: true,
			wantContains:  []string{"service", "dockerComposeFile"},
		},
		{
			name:          "both present — ok",
			blocks:        []document.Block{{Key: "service"}, {Key: "dockerComposeFile"}},
			wantViolation: false,
		},
		{
			name:          "key absent — ok",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(nil, tc.blocks)
			if tc.wantViolation && len(errs) != 1 {
				t.Fatalf("expected 1 violation, got %v", errs)
			}
			if !tc.wantViolation && len(errs) != 0 {
				t.Errorf("expected no violations, got %v", errs)
			}
			for _, want := range tc.wantContains {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("violation message should contain %q; got %q", want, errs[0])
				}
			}
		})
	}
}

func TestMutuallyExclusiveNested(t *testing.T) {
	tests := []struct {
		name      string
		validator editor.Validator
		raw       string
		wantCount int
		wantInErr []string
	}{
		{
			name:      "top-level filter violation",
			validator: editor.MutuallyExclusiveNested("filter", "any", "all"),
			raw: `
filter:
  any:
    - categories: [foo]
  all:
    - categories: [bar]
`,
			wantCount: 1,
			wantInErr: []string{"any", "all", "filter"},
		},
		{
			name:      "single key — ok",
			validator: editor.MutuallyExclusiveNested("filter", "any", "all"),
			raw: `
filter:
  any:
    - categories: [foo]
`,
			wantCount: 0,
		},
		{
			name:      "nested filter violation",
			validator: editor.MutuallyExclusiveNested("filter", "any", "all"),
			raw: `
filter:
  any:
    - filter:
        any:
          - categories: [foo]
        all:
          - categories: [bar]
`,
			wantCount: 1,
			wantInErr: []string{"any[0].filter"},
		},
		{
			name:      "both top-level and nested — two violations",
			validator: editor.MutuallyExclusiveNested("filter", "any", "all"),
			raw: `
filter:
  any:
    - filter:
        any:
          - categories: [foo]
        all:
          - categories: [bar]
  all:
    - categories: [baz]
`,
			wantCount: 2,
		},
		{
			name:      "deeply nested path without scope",
			validator: editor.MutuallyExclusiveNested("filter", "any", "all"),
			raw: `
categories:
  foo:
    installers:
      - name: bar
        source:
          filter:
            any:
              - categories: [x]
            all:
              - categories: [y]
`,
			wantCount: 1,
			wantInErr: []string{"filter"},
		},
		{
			name:      "scoped path — catches violation inside scope",
			validator: editor.MutuallyExclusiveNested("categories.installers.source.filter", "any", "all"),
			raw: `
categories:
  foo:
    installers:
      - name: bar
        source:
          filter:
            any:
              - categories: [x]
            all:
              - categories: [y]
`,
			wantCount: 1,
			wantInErr: []string{"filter"},
		},
		{
			name:      "scoped path — ignores filters outside scope",
			validator: editor.MutuallyExclusiveNested("categories.installers.source.filter", "any", "all"),
			raw: `
categories:
  foo:
    installers:
      - name: bar
        source:
          filter:
            any:
              - categories: [x]
some:
  other:
    object:
      filter:
        any:
          - categories: [x]
        all:
          - categories: [y]
`,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.validator.Validate([]byte(tc.raw), nil)
			if len(errs) != tc.wantCount {
				t.Fatalf("want %d violations, got %v", tc.wantCount, errs)
			}
			for _, want := range tc.wantInErr {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("first error should contain %q; got %q", want, errs[0])
				}
			}
		})
	}
}
