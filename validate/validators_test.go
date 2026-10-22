package validate

import (
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/spec"
)

// runValidator collects the rendered violations so tests can assert on the
// user-visible strings.
func runValidator(t *testing.T, v spec.Validator, raw string, blocks []document.Block) []string {
	t.Helper()
	var out []string
	for _, viol := range v.Validate(spec.NewValidationInput([]byte(raw), blocks)) {
		out = append(out, viol.String())
	}
	return out
}

// TestPathExpansion_acrossValidators verifies that every path-based validator
// expands sequences and dict-style mappings along the path, like
// MutuallyExclusive always did - e.g. "categories.installers.source.type" is
// checked inside every installer of every category.
func TestPathExpansion_acrossValidators(t *testing.T) {
	// categories is a dict; installers is a list - both must be expanded.
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
		validator    spec.Validator
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
			name:      "ValueOneOf - all entries valid",
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

// ---------------------------------------------------------------------------
// parseSize strictness: a numeric prefix followed by garbage must be rejected
// so malformed values are reported as not comparable instead of compared.
// ---------------------------------------------------------------------------

func TestParseSize_rejectsTrailingGarbage(t *testing.T) {
	for _, bad := range []string{"10xB", "1.5junkGB", "x10MB", "MB", ""} {
		if got, ok := parseSize(bad); ok {
			t.Errorf("parseSize(%q) accepted malformed input as %d", bad, got)
		}
	}
	cases := []struct {
		in   string
		want int64
	}{
		{"10MB", 10_000_000},
		{"1.5KB", 1500},
		{"256MiB", 256 << 20},
		{"8B", 8},
		{" 2 GB ", 2_000_000_000},
	}
	for _, tc := range cases {
		got, ok := parseSize(tc.in)
		if !ok || got != tc.want {
			t.Errorf("parseSize(%q) = %d,%v want %d,true", tc.in, got, ok, tc.want)
		}
	}
}
