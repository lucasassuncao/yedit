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

func TestAtLeastOneOf(t *testing.T) {
	v := editor.AtLeastOneOf("image", "build", "dockerComposeFile")

	tests := []struct {
		name          string
		blocks        []document.Block
		wantViolation bool
	}{
		{
			name:          "none present — violation",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: true,
		},
		{
			name:          "one present — ok",
			blocks:        []document.Block{{Key: "image"}},
			wantViolation: false,
		},
		{
			name:          "multiple present — ok",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}},
			wantViolation: false,
		},
		{
			name:          "empty blocks — violation",
			blocks:        nil,
			wantViolation: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(nil, tc.blocks)
			if tc.wantViolation && len(errs) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(errs) != 0 {
				t.Errorf("expected no violations, got %v", errs)
			}
			if tc.wantViolation && len(errs) > 0 {
				for _, want := range []string{"image", "build", "dockerComposeFile"} {
					if !strings.Contains(errs[0], want) {
						t.Errorf("violation should mention %q; got %q", want, errs[0])
					}
				}
			}
		})
	}
}

func TestExactlyOneOf(t *testing.T) {
	v := editor.ExactlyOneOf("image", "build", "dockerComposeFile")

	tests := []struct {
		name          string
		blocks        []document.Block
		wantViolation bool
		wantContains  []string
	}{
		{
			name:          "none present — violation",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: true,
			wantContains:  []string{"required"},
		},
		{
			name:          "one present — ok",
			blocks:        []document.Block{{Key: "image"}},
			wantViolation: false,
		},
		{
			name:          "two present — violation",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}},
			wantViolation: true,
			wantContains:  []string{"image", "build"},
		},
		{
			name:          "all three present — violation",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}, {Key: "dockerComposeFile"}},
			wantViolation: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(nil, tc.blocks)
			if tc.wantViolation && len(errs) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(errs) != 0 {
				t.Errorf("expected no violations, got %v", errs)
			}
			for _, want := range tc.wantContains {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0])
				}
			}
		})
	}
}

func TestRequiredIf(t *testing.T) {
	v := editor.RequiredIf("tls.cert", "tls.enabled", "true")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name: "condition active, key absent — violation",
			raw: `
tls:
  enabled: "true"
`,
			wantViolation: true,
			wantContains:  []string{"tls.cert", "tls.enabled", "true"},
		},
		{
			name: "condition active, key present — ok",
			raw: `
tls:
  enabled: "true"
  cert: /etc/tls/cert.pem
`,
			wantViolation: false,
		},
		{
			name: "condition inactive (different value) — ok",
			raw: `
tls:
  enabled: "false"
`,
			wantViolation: false,
		},
		{
			name:          "condition path absent — ok",
			raw:           `name: myapp`,
			wantViolation: false,
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
			for _, want := range tc.wantContains {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0])
				}
			}
		})
	}
}

func TestValueOneOf(t *testing.T) {
	v := editor.ValueOneOf("configuration.log-level", "trace", "debug", "info", "warn", "error", "fatal")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name: "allowed value — ok",
			raw: `
configuration:
  log-level: info
`,
			wantViolation: false,
		},
		{
			name: "disallowed value — violation",
			raw: `
configuration:
  log-level: verbose
`,
			wantViolation: true,
			wantContains:  []string{"log-level", "verbose", "trace", "fatal"},
		},
		{
			name: "field absent — ok",
			raw: `
configuration:
  output: console
`,
			wantViolation: false,
		},
		{
			name:          "path absent — ok",
			raw:           `name: myapp`,
			wantViolation: false,
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
			for _, want := range tc.wantContains {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0])
				}
			}
		})
	}
}

func TestCrossFieldOrdered(t *testing.T) {
	tests := []struct {
		name          string
		validator     editor.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "duration: smaller < larger — ok",
			validator: editor.CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  min-age: 24h
  max-age: 168h
`,
			wantViolation: false,
		},
		{
			name:      "duration: smaller > larger — violation",
			validator: editor.CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  min-age: 720h
  max-age: 24h
`,
			wantViolation: true,
			wantContains:  []string{"min-age", "max-age"},
		},
		{
			name:      "duration: equal values — violation",
			validator: editor.CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  min-age: 24h
  max-age: 24h
`,
			wantViolation: true,
		},
		{
			name:      "size: smaller < larger — ok",
			validator: editor.CrossFieldOrdered("filter.min-size", "filter.max-size"),
			raw: `
filter:
  min-size: 1MB
  max-size: 100MB
`,
			wantViolation: false,
		},
		{
			name:      "size: smaller > larger — violation",
			validator: editor.CrossFieldOrdered("filter.min-size", "filter.max-size"),
			raw: `
filter:
  min-size: 500MB
  max-size: 100MB
`,
			wantViolation: true,
			wantContains:  []string{"min-size", "max-size"},
		},
		{
			name:      "one field absent — ok",
			validator: editor.CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  min-age: 24h
`,
			wantViolation: false,
		},
		{
			name:      "both absent — ok",
			validator: editor.CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  regex: "^foo"
`,
			wantViolation: false,
		},
		{
			name:      "incomparable types (mixed duration and size) — ok",
			validator: editor.CrossFieldOrdered("filter.min-age", "filter.max-size"),
			raw: `
filter:
  min-age: 24h
  max-size: 100MB
`,
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.validator.Validate([]byte(tc.raw), nil)
			if tc.wantViolation && len(errs) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(errs) != 0 {
				t.Errorf("expected no violations, got %v", errs)
			}
			for _, want := range tc.wantContains {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0])
				}
			}
		})
	}
}

func TestNoDuplicates(t *testing.T) {
	v := editor.NoDuplicates("categories", "name")

	tests := []struct {
		name         string
		raw          string
		wantCount    int
		wantContains []string
	}{
		{
			name: "no duplicates — ok",
			raw: `
categories:
  - name: images
  - name: videos
  - name: documents
`,
			wantCount: 0,
		},
		{
			name: "one duplicate — one violation",
			raw: `
categories:
  - name: images
  - name: videos
  - name: images
`,
			wantCount:    1,
			wantContains: []string{"categories[2]", "images", "categories[0]"},
		},
		{
			name: "two distinct duplicates — two violations",
			raw: `
categories:
  - name: images
  - name: videos
  - name: images
  - name: videos
`,
			wantCount: 2,
		},
		{
			name: "item without the field — skipped",
			raw: `
categories:
  - name: images
  - source: /tmp
  - name: images
`,
			wantCount:    1,
			wantContains: []string{"categories[2]"},
		},
		{
			name:      "empty sequence — ok",
			raw:       `categories: []`,
			wantCount: 0,
		},
		{
			name:      "path not a sequence — ok",
			raw:       `categories: images`,
			wantCount: 0,
		},
		{
			name:      "path absent — ok",
			raw:       `name: myapp`,
			wantCount: 0,
		},
		{
			name:      "empty document — ok",
			raw:       "",
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate([]byte(tc.raw), nil)
			if len(errs) != tc.wantCount {
				t.Fatalf("want %d violations, got %v", tc.wantCount, errs)
			}
			for _, want := range tc.wantContains {
				if len(errs) > 0 && !strings.Contains(errs[0], want) {
					t.Errorf("first violation should contain %q; got %q", want, errs[0])
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
