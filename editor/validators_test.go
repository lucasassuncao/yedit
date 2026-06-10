package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/schema"
)

func TestMutuallyExclusive(t *testing.T) {
	v := MutuallyExclusive("image", "build", "dockerComposeFile")

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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("violation message should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestMutuallyExclusive_dottedPath(t *testing.T) {
	v := MutuallyExclusive(
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
	v := MutuallyExclusive("image", "build")
	if errs := v.Validate(nil, blocks); len(errs) != 1 {
		t.Errorf("top-level behavior should be unchanged, got %v", errs)
	}
}

func TestRequiredWith(t *testing.T) {
	v := RequiredWith("service", "dockerComposeFile")

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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("violation message should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestAtLeastOneOf(t *testing.T) {
	v := AtLeastOneOf("image", "build", "dockerComposeFile")

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
					if !strings.Contains(errs[0].String(), want) {
						t.Errorf("violation should mention %q; got %q", want, errs[0].String())
					}
				}
			}
		})
	}
}

func TestExactlyOneOf(t *testing.T) {
	v := ExactlyOneOf("image", "build", "dockerComposeFile")

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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestRequiredIf(t *testing.T) {
	v := RequiredIf("tls.cert", "tls.enabled", "true")

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
			name: "condition active, key present as mapping — ok",
			raw: `
tls:
  enabled: "true"
  cert:
    path: /etc/tls/cert.pem
`,
			wantViolation: false,
		},
		{
			name: "condition active, key present as empty scalar — violation",
			raw: `
tls:
  enabled: "true"
  cert:
`,
			wantViolation: true,
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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestValueOneOf(t *testing.T) {
	v := ValueOneOf("configuration.log-level", "trace", "debug", "info", "warn", "error", "fatal")

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
			name: "non-scalar value — violation",
			raw: `
configuration:
  log-level:
    file: debug
`,
			wantViolation: true,
			wantContains:  []string{"log-level", "scalar"},
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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestCrossFieldOrdered(t *testing.T) {
	tests := []struct {
		name          string
		validator     Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "duration: smaller < larger — ok",
			validator: CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  min-age: 24h
  max-age: 168h
`,
			wantViolation: false,
		},
		{
			name:      "duration: smaller > larger — violation",
			validator: CrossFieldOrdered("filter.min-age", "filter.max-age"),
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
			validator: CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  min-age: 24h
  max-age: 24h
`,
			wantViolation: true,
		},
		{
			name:      "size: smaller < larger — ok",
			validator: CrossFieldOrdered("filter.min-size", "filter.max-size"),
			raw: `
filter:
  min-size: 1MB
  max-size: 100MB
`,
			wantViolation: false,
		},
		{
			name:      "size: smaller > larger — violation",
			validator: CrossFieldOrdered("filter.min-size", "filter.max-size"),
			raw: `
filter:
  min-size: 500MB
  max-size: 100MB
`,
			wantViolation: true,
			wantContains:  []string{"min-size", "max-size"},
		},
		{
			name:      "size: SI suffixes are decimal (999KB < 1MB) — ok",
			validator: CrossFieldOrdered("filter.min-size", "filter.max-size"),
			raw: `
filter:
  min-size: 999KB
  max-size: 1MB
`,
			wantViolation: false,
		},
		{
			name:      "size: IEC suffixes are binary (1023KiB < 1MiB) — ok",
			validator: CrossFieldOrdered("filter.min-size", "filter.max-size"),
			raw: `
filter:
  min-size: 1023KiB
  max-size: 1MiB
`,
			wantViolation: false,
		},
		{
			name:      "size: 1024KiB equals 1MiB — violation",
			validator: CrossFieldOrdered("filter.min-size", "filter.max-size"),
			raw: `
filter:
  min-size: 1024KiB
  max-size: 1MiB
`,
			wantViolation: true,
		},
		{
			name:      "one field absent — ok",
			validator: CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  min-age: 24h
`,
			wantViolation: false,
		},
		{
			name:      "both absent — ok",
			validator: CrossFieldOrdered("filter.min-age", "filter.max-age"),
			raw: `
filter:
  regex: "^foo"
`,
			wantViolation: false,
		},
		{
			name:      "incomparable types (mixed duration and size) — ok",
			validator: CrossFieldOrdered("filter.min-age", "filter.max-size"),
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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("violation should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestNoDuplicates(t *testing.T) {
	v := NoDuplicates("categories", "name")

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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("first violation should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

// runValidator collects the rendered violations so tests can assert on the
// user-visible strings.
func runValidator(t *testing.T, v Validator, raw string, blocks []document.Block) []string {
	t.Helper()
	var out []string
	for _, viol := range v.Validate([]byte(raw), blocks) {
		out = append(out, viol.String())
	}
	return out
}

func TestRequired(t *testing.T) {
	tests := []struct {
		name      string
		validator Validator
		raw       string
		want      []string // exact violation strings, in order
	}{
		{
			name:      "top-level present — ok",
			validator: Required("version"),
			raw:       "version: 1.0.0\n",
		},
		{
			name:      "top-level absent — violation",
			validator: Required("version"),
			raw:       "name: myapp\n",
			want:      []string{"version: required"},
		},
		{
			name:      "empty document — top-level still required",
			validator: Required("version"),
			raw:       "",
			want:      []string{"version: required"},
		},
		{
			name:      "null scalar counts as missing",
			validator: Required("version"),
			raw:       "version:\n",
			want:      []string{"version: required"},
		},
		{
			name:      "non-scalar counts as present",
			validator: Required("build"),
			raw:       "build:\n  context: .\n",
		},
		{
			name:      "dotted path — parent absent is ok",
			validator: Required("categories.name"),
			raw:       "version: 1.0.0\n",
		},
		{
			name:      "dotted path — every sequence entry checked",
			validator: Required("categories.name"),
			raw: `
categories:
  - name: images
  - source: /tmp
`,
			want: []string{"categories[1].name: required"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, tc.validator, tc.raw, nil)
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

func TestValueInRange(t *testing.T) {
	tests := []struct {
		name          string
		validator     Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "number within range — ok",
			validator: ValueInRange("server.port", "1", "65535"),
			raw:       "server:\n  port: 8080\n",
		},
		{
			name:          "number out of range — violation",
			validator:     ValueInRange("server.port", "1", "65535"),
			raw:           "server:\n  port: 70000\n",
			wantViolation: true,
			wantContains:  []string{"server.port", "70000", "out of range"},
		},
		{
			name:      "duration within range — ok",
			validator: ValueInRange("filter.max-age", "1h", "8760h"),
			raw:       "filter:\n  max-age: 24h\n",
		},
		{
			name:          "duration below range — violation",
			validator:     ValueInRange("filter.max-age", "1h", "8760h"),
			raw:           "filter:\n  max-age: 30m\n",
			wantViolation: true,
		},
		{
			name:      "size within range — ok",
			validator: ValueInRange("filter.max-size", "1MB", "1GB"),
			raw:       "filter:\n  max-size: 100MB\n",
		},
		{
			name:      "absent path — ok",
			validator: ValueInRange("server.port", "1", "65535"),
			raw:       "name: myapp\n",
		},
		{
			name:      "empty value — ok",
			validator: ValueInRange("server.port", "1", "65535"),
			raw:       "server:\n  port:\n",
		},
		{
			name:          "non-scalar value — violation",
			validator:     ValueInRange("server.port", "1", "65535"),
			raw:           "server:\n  port:\n    internal: 8080\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "value not comparable with range — violation",
			validator:     ValueInRange("server.port", "1", "65535"),
			raw:           "server:\n  port: eighty\n",
			wantViolation: true,
			wantContains:  []string{"not comparable"},
		},
		{
			name:          "mixed-kind bounds — misconfiguration reported",
			validator:     ValueInRange("server.port", "1h", "65535"),
			raw:           "server:\n  port: 8080\n",
			wantViolation: true,
			wantContains:  []string{"invalid range"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, tc.validator, tc.raw, nil)
			if tc.wantViolation && len(got) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(got) != 0 {
				t.Errorf("expected no violations, got %v", got)
			}
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

func TestValueMatches(t *testing.T) {
	tests := []struct {
		name          string
		validator     Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "match — ok",
			validator: ValueMatches("version", `^\d+\.\d+\.\d+$`),
			raw:       "version: 1.2.3\n",
		},
		{
			name:          "mismatch — violation",
			validator:     ValueMatches("version", `^\d+\.\d+\.\d+$`),
			raw:           "version: latest\n",
			wantViolation: true,
			wantContains:  []string{"version", "latest", "does not match"},
		},
		{
			name:      "absent path — ok",
			validator: ValueMatches("version", `^\d+$`),
			raw:       "name: myapp\n",
		},
		{
			name:      "empty value — ok",
			validator: ValueMatches("version", `^\d+$`),
			raw:       "version:\n",
		},
		{
			name:          "non-scalar value — violation",
			validator:     ValueMatches("version", `^\d+$`),
			raw:           "version:\n  major: 1\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "invalid pattern — misconfiguration reported",
			validator:     ValueMatches("version", `^(\d+$`),
			raw:           "version: 1\n",
			wantViolation: true,
			wantContains:  []string{"invalid pattern"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, tc.validator, tc.raw, nil)
			if tc.wantViolation && len(got) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(got) != 0 {
				t.Errorf("expected no violations, got %v", got)
			}
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

func TestAllOrNone_topLevel(t *testing.T) {
	v := AllOrNone("tls-cert", "tls-key")

	tests := []struct {
		name          string
		blocks        []document.Block
		wantViolation bool
		wantContains  []string
	}{
		{
			name:   "both present — ok",
			blocks: []document.Block{{Key: "tls-cert"}, {Key: "tls-key"}},
		},
		{
			name:   "none present — ok",
			blocks: []document.Block{{Key: "name"}},
		},
		{
			name:          "only one present — violation",
			blocks:        []document.Block{{Key: "tls-cert"}},
			wantViolation: true,
			wantContains:  []string{"tls-cert", "tls-key", "missing"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, v, "", tc.blocks)
			if tc.wantViolation && len(got) != 1 {
				t.Fatalf("expected 1 violation, got %v", got)
			}
			if !tc.wantViolation && len(got) != 0 {
				t.Errorf("expected no violations, got %v", got)
			}
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

func TestAllOrNone_dottedPath(t *testing.T) {
	v := AllOrNone("server.tls-cert", "server.tls-key")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
	}{
		{
			name: "both present — ok",
			raw: `
server:
  tls-cert: /etc/tls/cert.pem
  tls-key: /etc/tls/key.pem
`,
		},
		{
			name: "none present — ok",
			raw: `
server:
  host: localhost
`,
		},
		{
			name: "only one present — violation",
			raw: `
server:
  tls-cert: /etc/tls/cert.pem
`,
			wantViolation: true,
		},
		{
			name: "parent absent — ok",
			raw:  "name: myapp\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, v, tc.raw, nil)
			if tc.wantViolation && len(got) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(got) != 0 {
				t.Errorf("expected no violations, got %v", got)
			}
		})
	}
}

func TestCountRange(t *testing.T) {
	tests := []struct {
		name          string
		validator     Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "within range — ok",
			validator: CountRange("workers", 1, 10),
			raw:       "workers:\n  - name: a\n  - name: b\n",
		},
		{
			name:          "below min — violation",
			validator:     CountRange("workers", 1, 10),
			raw:           "workers: []\n",
			wantViolation: true,
			wantContains:  []string{"workers", "0 entries", "between 1 and 10"},
		},
		{
			name:          "above max — violation",
			validator:     CountRange("workers", 0, 1),
			raw:           "workers:\n  - name: a\n  - name: b\n",
			wantViolation: true,
		},
		{
			name:      "no upper bound — ok",
			validator: CountRange("workers", 1, -1),
			raw:       "workers:\n  - name: a\n  - name: b\n  - name: c\n",
		},
		{
			name:          "no upper bound, below min — violation",
			validator:     CountRange("workers", 2, -1),
			raw:           "workers:\n  - name: a\n",
			wantViolation: true,
			wantContains:  []string{"at least 2"},
		},
		{
			name:      "mapping counts keys — ok",
			validator: CountRange("port-attrs", 1, 2),
			raw:       "port-attrs:\n  \"8080\":\n    label: web\n",
		},
		{
			name:      "absent path — ok",
			validator: CountRange("workers", 1, 10),
			raw:       "name: myapp\n",
		},
		{
			name:          "scalar at path — violation",
			validator:     CountRange("workers", 1, 10),
			raw:           "workers: many\n",
			wantViolation: true,
			wantContains:  []string{"list or mapping"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, tc.validator, tc.raw, nil)
			if tc.wantViolation && len(got) == 0 {
				t.Fatal("expected a violation, got none")
			}
			if !tc.wantViolation && len(got) != 0 {
				t.Errorf("expected no violations, got %v", got)
			}
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

func TestUniqueValues(t *testing.T) {
	v := UniqueValues("tags")

	tests := []struct {
		name         string
		raw          string
		wantCount    int
		wantContains []string
	}{
		{
			name:      "unique — ok",
			raw:       "tags: [go, yaml, tui]\n",
			wantCount: 0,
		},
		{
			name:         "duplicate — violation with indices",
			raw:          "tags: [go, yaml, go]\n",
			wantCount:    1,
			wantContains: []string{"tags[2]", `"go"`, "tags[0]"},
		},
		{
			name:      "two distinct duplicates — two violations",
			raw:       "tags: [a, b, a, b]\n",
			wantCount: 2,
		},
		{
			name:      "non-scalar items skipped",
			raw:       "tags:\n  - name: x\n  - name: x\n",
			wantCount: 0,
		},
		{
			name:      "absent path — ok",
			raw:       "name: myapp\n",
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, v, tc.raw, nil)
			if len(got) != tc.wantCount {
				t.Fatalf("want %d violations, got %v", tc.wantCount, got)
			}
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

func TestDeprecated(t *testing.T) {
	tests := []struct {
		name          string
		validator     Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:          "present — violation with hint",
			validator:     Deprecated("dockerFile", "use build.dockerfile instead"),
			raw:           "dockerFile: Dockerfile\n",
			wantViolation: true,
			wantContains:  []string{"dockerFile", "deprecated", "use build.dockerfile instead"},
		},
		{
			name:      "absent — ok",
			validator: Deprecated("dockerFile", "use build.dockerfile instead"),
			raw:       "build:\n  dockerfile: Dockerfile\n",
		},
		{
			name:          "present with null value — still deprecated",
			validator:     Deprecated("dockerFile", "use build.dockerfile instead"),
			raw:           "dockerFile:\n",
			wantViolation: true,
		},
		{
			name:          "nested path",
			validator:     Deprecated("server.insecure", "use server.tls instead"),
			raw:           "server:\n  insecure: true\n",
			wantViolation: true,
			wantContains:  []string{"server.insecure"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, tc.validator, tc.raw, nil)
			if tc.wantViolation && len(got) != 1 {
				t.Fatalf("expected 1 violation, got %v", got)
			}
			if !tc.wantViolation && len(got) != 0 {
				t.Errorf("expected no violations, got %v", got)
			}
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

// TestPathExpansion_acrossValidators verifies that every path-based validator
// expands sequences and dict-style mappings along the path, like
// MutuallyExclusive always did — e.g. "categories.installers.source.type" is
// checked inside every installer of every category.
func TestPathExpansion_acrossValidators(t *testing.T) {
	// categories is a dict; installers is a list — both must be expanded.
	const nested = `
categories:
  media:
    installers:
      - name: ok
        source:
          type: winget
      - name: bad
        source:
          type: floppy
  tools:
    installers:
      - name: also-ok
        source:
          type: scoop
`

	tests := []struct {
		name         string
		validator    Validator
		raw          string
		wantCount    int
		wantContains []string // checked against the first violation
	}{
		{
			name:         "ValueOneOf finds the bad entry through dict and list",
			validator:    ValueOneOf("categories.installers.source.type", "winget", "scoop"),
			raw:          nested,
			wantCount:    1,
			wantContains: []string{"categories.media.installers[1].source.type", "floppy"},
		},
		{
			name:      "ValueOneOf — all entries valid",
			validator: ValueOneOf("categories.installers.source.type", "winget", "scoop", "floppy"),
			raw:       nested,
			wantCount: 0,
		},
		{
			name:      "ValueMatches checks every entry",
			validator: ValueMatches("workers.name", `^[a-z]+$`),
			raw: `
workers:
  - name: alpha
  - name: Beta9
`,
			wantCount:    1,
			wantContains: []string{"workers[1].name", "Beta9"},
		},
		{
			name:      "ValueInRange checks every entry",
			validator: ValueInRange("workers.concurrency", "1", "8"),
			raw: `
workers:
  - concurrency: 4
  - concurrency: 99
`,
			wantCount:    1,
			wantContains: []string{"workers[1].concurrency", "out of range"},
		},
		{
			name:      "CrossFieldOrdered compares each entry's own pair",
			validator: CrossFieldOrdered("categories.installers.source.filter.min-age", "categories.installers.source.filter.max-age"),
			raw: `
categories:
  media:
    installers:
      - source:
          filter:
            min-age: 24h
            max-age: 168h
      - source:
          filter:
            min-age: 720h
            max-age: 24h
`,
			wantCount:    1,
			wantContains: []string{"installers[1]", "min-age", "max-age"},
		},
		{
			name:      "RequiredIf with shared parent checks each entry's condition",
			validator: RequiredIf("servers.tls-cert", "servers.protocol", "https"),
			raw: `
servers:
  - protocol: http
  - protocol: https
    tls-cert: /etc/tls/cert.pem
  - protocol: https
`,
			wantCount:    1,
			wantContains: []string{"servers[2].tls-cert", "required when"},
		},
		{
			name:         "Required reaches leaves through dicts and lists",
			validator:    Required("categories.installers.name"),
			raw:          "categories:\n  media:\n    installers:\n      - source: {}\n",
			wantCount:    1,
			wantContains: []string{"categories.media.installers[0].name: required"},
		},
		{
			name:         "UniqueValues checks each entry's own sequence",
			validator:    UniqueValues("workers.tags"),
			raw:          "workers:\n  - tags: [a, b]\n  - tags: [c, c]\n",
			wantCount:    1,
			wantContains: []string{"workers[1].tags[1]", `"c"`},
		},
		{
			name:         "CountRange checks each dict value",
			validator:    CountRange("categories.installers", 1, -1),
			raw:          "categories:\n  media:\n    installers:\n      - name: x\n  tools:\n    installers: []\n",
			wantCount:    1,
			wantContains: []string{"categories.tools.installers", "at least 1"},
		},
		{
			name:         "Deprecated flags every occurrence",
			validator:    Deprecated("servers.insecure", "use tls instead"),
			raw:          "servers:\n  - insecure: true\n  - host: x\n  - insecure: false\n",
			wantCount:    2,
			wantContains: []string{"servers[0].insecure", "deprecated"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, tc.validator, tc.raw, nil)
			if len(got) != tc.wantCount {
				t.Fatalf("want %d violations, got %v", tc.wantCount, got)
			}
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("first violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

func TestViolation_PathAndString(t *testing.T) {
	if got := (Violation{Message: "msg"}).String(); got != "msg" {
		t.Errorf("String without Path = %q, want %q", got, "msg")
	}
	if got := (Violation{Path: "a.b", Message: "msg"}).String(); got != "a.b: msg" {
		t.Errorf("String with Path = %q, want %q", got, "a.b: msg")
	}

	v := ValueOneOf("configuration.log-level", "info")
	errs := v.Validate([]byte("configuration:\n  log-level: verbose\n"), nil)
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if errs[0].Path != "configuration.log-level" {
		t.Errorf("Path = %q, want %q", errs[0].Path, "configuration.log-level")
	}
}

func TestMutuallyExclusiveNested(t *testing.T) {
	tests := []struct {
		name      string
		validator Validator
		raw       string
		wantCount int
		wantInErr []string
	}{
		{
			name:      "top-level filter violation",
			validator: MutuallyExclusiveNested("filter", "any", "all"),
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
			validator: MutuallyExclusiveNested("filter", "any", "all"),
			raw: `
filter:
  any:
    - categories: [foo]
`,
			wantCount: 0,
		},
		{
			name:      "nested filter violation",
			validator: MutuallyExclusiveNested("filter", "any", "all"),
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
			validator: MutuallyExclusiveNested("filter", "any", "all"),
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
			validator: MutuallyExclusiveNested("filter", "any", "all"),
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
			validator: MutuallyExclusiveNested("categories.installers.source.filter", "any", "all"),
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
			validator: MutuallyExclusiveNested("categories.installers.source.filter", "any", "all"),
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
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("first error should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

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
