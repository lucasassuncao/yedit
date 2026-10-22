package validate

import (
	"strings"
	"sync"
	"testing"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/spec"
)

// wiredRequiredFromMetadata wires RequiredFromMetadata with hintsConfig's schema and
// a HintSource marking version (block-level), server.host, workers.name and
// port-attrs.label as required.
func wiredRequiredFromMetadata(t *testing.T) spec.Validator {
	t.Helper()
	required := map[string]bool{
		"version|":         true,
		"server|host":      true,
		"workers|name":     true,
		"port-attrs|label": true,
	}
	return wireMetadataRule(t, RequiredFromMetadata(), func(block, fieldPath string) spec.FieldMeta {
		return spec.FieldMeta{Required: required[block+"|"+fieldPath]}
	})
}

// wireMetadataRule wires any FromMetadata validator with hintsConfig's schema and the
// given hint function.
func wireMetadataRule(t *testing.T, v spec.Validator, hints spec.MetadataFunc) spec.Validator {
	t.Helper()
	rfh := v.(*metadataRuleValidator)
	rfh.defs = schema.Discover(&hintsConfig{})
	rfh.hints = hints
	return v
}

func TestRequiredFromMetadata(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string // exact violation strings, in order
	}{
		{
			name: "empty document - only top-level required reported",
			raw:  "",
			want: []string{"version: required"},
		},
		{
			name: "all satisfied - ok",
			raw: `
version: 1.0.0
server:
  host: localhost
`,
			want: nil,
		},
		{
			name: "optional block absent - its required children not reported",
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := wiredRequiredFromMetadata(t)
			var got []string
			for _, viol := range v.Validate(spec.NewValidationInput([]byte(tc.raw), nil)) {
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

func TestRequiredFromMetadata_inertWithoutWiring(t *testing.T) {
	if errs := RequiredFromMetadata().Validate(spec.NewValidationInput(nil, nil)); len(errs) != 0 {
		t.Errorf("unwired validator should report nothing, got %v", errs)
	}

	// Wired schema but no HintSource: reports nothing.
	v := RequiredFromMetadata()
	v.(*metadataRuleValidator).defs = schema.Discover(&hintsConfig{})
	if errs := v.Validate(spec.NewValidationInput([]byte(""), nil)); len(errs) != 0 {
		t.Errorf("validator without HintSource should report nothing, got %v", errs)
	}
}

// hintsConfig exercises the FromMetadata walk across the structural kinds: a
// top-level scalar, an optional object, a sequence, a dictionary, and a
// scalar list.
type hintsConfig struct {
	Version string   `yaml:"version"`
	Tags    []string `yaml:"tags"`
	Server  *struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	Workers []struct {
		Name string `yaml:"name"`
	} `yaml:"workers"`
	PortAttrs map[string]struct {
		Label string `yaml:"label"`
	} `yaml:"port-attrs"`
}

// runMetadataRule wires v with hintsConfig's schema and hints, validates raw, and
// compares the violation strings against want.
func runMetadataRule(t *testing.T, v spec.Validator, hints spec.MetadataFunc, raw string, want []string) {
	t.Helper()
	wireMetadataRule(t, v, hints)
	var got []string
	for _, viol := range v.Validate(spec.NewValidationInput([]byte(raw), nil)) {
		got = append(got, viol.String())
	}
	if len(got) != len(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("violation[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOneOfFromMetadata(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "server" && fieldPath == "host" {
			return spec.FieldMeta{OneOf: []string{"localhost", "0.0.0.0"}}
		}
		return spec.FieldMeta{}
	})
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"allowed value - ok", "server:\n  host: localhost\n", nil},
		{"absent - ok", "version: 1\n", nil},
		{"empty - ok", "server:\n  host:\n", nil},
		{"not allowed - violation", "server:\n  host: example.com\n",
			[]string{`server.host: value "example.com" is not allowed - use one of: "localhost", "0.0.0.0"`}},
		{"non-scalar - violation", "server:\n  host:\n    a: b\n",
			[]string{"server.host: expected a scalar value"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runMetadataRule(t, OneOfFromMetadata(), hints, tc.raw, tc.want)
		})
	}
}

func TestRangeFromMetadata(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		switch {
		case block == "server" && fieldPath == "port":
			return spec.FieldMeta{Min: "1", Max: "65535"}
		case block == "version" && fieldPath == "": // Min only, no upper bound
			return spec.FieldMeta{Min: "1"}
		}
		return spec.FieldMeta{}
	})
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"in range - ok", "server:\n  port: 8080\n", nil},
		{"absent - ok", "tags:\n  - a\n", nil},
		{"below min - violation", "server:\n  port: 0\n",
			[]string{`server.port: value "0" out of range [1, 65535]`}},
		{"min-only bound - ok above", "version: 5\n", nil},
		{"min-only bound - violation below", "version: 0\n",
			[]string{`version: value "0" out of range [1, ∞]`}},
		{"not comparable - violation", "server:\n  port: abc\n",
			[]string{`server.port: value "abc" is not comparable with range [1, 65535]`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runMetadataRule(t, RangeFromMetadata(), hints, tc.raw, tc.want)
		})
	}
}

func TestRangeFromMetadata_misconfiguredBounds(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "version" {
			return spec.FieldMeta{Min: "10MB", Max: "24h"} // mixed kinds
		}
		return spec.FieldMeta{}
	})
	v := wireMetadataRule(t, RangeFromMetadata(), hints)
	errs := v.Validate(spec.NewValidationInput([]byte("version: 1\n"), nil))
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "invalid range") {
		t.Fatalf("expected 1 invalid-range violation, got %v", errs)
	}
}

func TestPatternFromMetadata(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "version" {
			return spec.FieldMeta{Pattern: `^\d+\.\d+\.\d+$`}
		}
		return spec.FieldMeta{}
	})
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"match - ok", "version: 1.2.3\n", nil},
		{"absent - ok", "server:\n  host: x\n", nil},
		{"mismatch - violation", "version: latest\n",
			// %q in the message escapes the pattern's backslashes.
			[]string{`version: value "latest" does not match pattern "^\\d+\\.\\d+\\.\\d+$"`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runMetadataRule(t, PatternFromMetadata(), hints, tc.raw, tc.want)
		})
	}
}

func TestPatternFromMetadata_invalidPattern(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "version" {
			return spec.FieldMeta{Pattern: `^(\d+$`}
		}
		return spec.FieldMeta{}
	})
	v := wireMetadataRule(t, PatternFromMetadata(), hints)
	errs := v.Validate(spec.NewValidationInput([]byte("version: 1\n"), nil))
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "invalid pattern") {
		t.Fatalf("expected 1 invalid-pattern violation, got %v", errs)
	}
}

func TestCountFromMetadata(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "workers" && fieldPath == "" {
			return spec.FieldMeta{MinCount: 1, MaxCount: 2}
		}
		return spec.FieldMeta{}
	})
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"within range - ok", "workers:\n  - name: a\n", nil},
		{"absent - ok", "version: 1\n", nil},
		{"empty list - violation", "workers: []\n",
			[]string{"workers: has 0 entries - expected between 1 and 2"}},
		{"above max - violation", "workers:\n  - name: a\n  - name: b\n  - name: c\n",
			[]string{"workers: has 3 entries - expected between 1 and 2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runMetadataRule(t, CountFromMetadata(), hints, tc.raw, tc.want)
		})
	}
}

func TestCountFromMetadata_minOnlyHasNoUpperBound(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "workers" && fieldPath == "" {
			return spec.FieldMeta{MinCount: 1}
		}
		return spec.FieldMeta{}
	})
	v := wireMetadataRule(t, CountFromMetadata(), hints)
	raw := "workers:\n  - name: a\n  - name: b\n  - name: c\n  - name: d\n"
	if errs := v.Validate(spec.NewValidationInput([]byte(raw), nil)); len(errs) != 0 {
		t.Errorf("MinCount-only must not cap the list, got %v", errs)
	}
}

func TestUniqueFromMetadata(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "tags" {
			return spec.FieldMeta{Unique: true}
		}
		return spec.FieldMeta{}
	})
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"unique - ok", "tags:\n  - a\n  - b\n", nil},
		{"absent - ok", "version: 1\n", nil},
		{"duplicate - violation", "tags:\n  - a\n  - b\n  - a\n",
			[]string{`tags[2]: duplicate value "a" (first seen at tags[0])`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runMetadataRule(t, UniqueFromMetadata(), hints, tc.raw, tc.want)
		})
	}
}

func TestDeprecatedFromMetadata(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "version" {
			return spec.FieldMeta{Deprecated: "use api-version instead"}
		}
		return spec.FieldMeta{}
	})
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"absent - ok", "tags:\n  - a\n", nil},
		{"present - violation", "version: 1\n",
			[]string{"version: deprecated - use api-version instead"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runMetadataRule(t, DeprecatedFromMetadata(), hints, tc.raw, tc.want)
		})
	}
}

func TestFormatFromMetadata(t *testing.T) {
	type S struct {
		URL  string `yaml:"url"`
		Addr string `yaml:"addr"`
	}
	defs := schema.Discover(&S{})

	wire := func(formats ...spec.Format) spec.Validator {
		v := FormatFromMetadata()
		v.(*metadataRuleValidator).hints = spec.MetadataFunc(func(block, path string) spec.FieldMeta {
			return spec.FieldMeta{Formats: formats}
		})
		v.(*metadataRuleValidator).defs = defs
		return v
	}

	validate := func(v spec.Validator, src string) []spec.Violation {
		return v.Validate(spec.NewValidationInput([]byte(src), nil))
	}

	// valid URL: no violation
	v := wire(spec.FormatURL)
	if errs := validate(v, "url: \"https://example.com\"\n"); len(errs) != 0 {
		t.Errorf("valid URL: want 0 violations, got %v", errs)
	}
	// invalid URL: violation
	if errs := validate(v, "url: \"not-a-url\"\n"); len(errs) != 1 || errs[0].Path != "url" {
		t.Errorf("invalid URL: want 1 violation at 'url', got %v", errs)
	}
	if errs := validate(v, "url: \"not-a-url\"\n"); !strings.Contains(errs[0].Message, "url") {
		t.Errorf("message should contain format label 'url': %q", errs[0].Message)
	}
	// empty value: no violation
	if errs := validate(v, "url: \"\"\n"); len(errs) != 0 {
		t.Errorf("empty value should skip format check, got %v", errs)
	}
	// OR semantics: IPv4 matches url|ipv4
	type Addr struct {
		Addr string `yaml:"addr"`
	}
	vOr := FormatFromMetadata()
	vOr.(*metadataRuleValidator).hints = spec.MetadataFunc(func(block, path string) spec.FieldMeta {
		return spec.FieldMeta{Formats: []spec.Format{spec.FormatURL, spec.FormatIPv4}}
	})
	vOr.(*metadataRuleValidator).defs = schema.Discover(&Addr{})
	if errs := vOr.Validate(spec.NewValidationInput([]byte("addr: \"192.168.1.1\"\n"), nil)); len(errs) != 0 {
		t.Errorf("IPv4 should match url|ipv4, got %v", errs)
	}
	if errs := vOr.Validate(spec.NewValidationInput([]byte("addr: \"ftp://bad\"\n"), nil)); len(errs) != 1 {
		t.Errorf("neither format: want 1 violation, got %v", errs)
	} else if !strings.Contains(errs[0].Message, "url | ipv4") {
		t.Errorf("message should list both labels: %q", errs[0].Message)
	}
}

func TestLengthFromMetadata(t *testing.T) {
	type S struct {
		Name string `yaml:"name"`
	}
	defs := schema.Discover(&S{})

	check := func(src string, min, max int, wantViolation bool) {
		t.Helper()
		v := LengthFromMetadata()
		v.(*metadataRuleValidator).hints = spec.MetadataFunc(func(block, path string) spec.FieldMeta {
			return spec.FieldMeta{MinLength: min, MaxLength: max}
		})
		v.(*metadataRuleValidator).defs = defs
		errs := v.Validate(spec.NewValidationInput([]byte(src), nil))
		if (len(errs) > 0) != wantViolation {
			t.Errorf("src=%q min=%d max=%d: wantViolation=%v got %v", src, min, max, wantViolation, errs)
		}
	}

	check("name: \"hello\"\n", 3, 10, false)
	check("name: \"hi\"\n", 3, 10, true)                // too short
	check("name: \"this is too long!\"\n", 3, 10, true) // too long
	check("name: \"hello\"\n", 0, 10, false)
	check("name: \"hello world!\"\n", 0, 10, true) // over max
	check("name: \"hi\"\n", 3, 0, true)            // under min, no upper bound
	check("name: \"hello\"\n", 3, 0, false)
	check("name: \"\"\n", 3, 10, false) // empty value skipped
}

func TestNotOneOfFromMetadata(t *testing.T) {
	type S struct {
		Proto string `yaml:"proto"`
	}
	defs := schema.Discover(&S{})

	check := func(src string, denied []string, wantViolation bool) {
		t.Helper()
		v := NotOneOfFromMetadata()
		v.(*metadataRuleValidator).hints = spec.MetadataFunc(func(block, path string) spec.FieldMeta {
			return spec.FieldMeta{NotOneOf: denied}
		})
		v.(*metadataRuleValidator).defs = defs
		errs := v.Validate(spec.NewValidationInput([]byte(src), nil))
		if (len(errs) > 0) != wantViolation {
			t.Errorf("src=%q denied=%v: wantViolation=%v got %v", src, denied, wantViolation, errs)
		}
	}

	check("proto: \"ftp\"\n", []string{"ftp", "ws"}, true)
	check("proto: \"https\"\n", []string{"ftp", "ws"}, false)
	check("proto: \"FTP\"\n", []string{"ftp"}, false) // case-sensitive
	check("proto: \"\"\n", []string{"ftp"}, false)    // empty skipped
}

// ---------------------------------------------------------------------------
// Explicit null handling: "key: null" / "key: ~" must behave like an absent
// or empty value, never as the literal string "null"/"~".
// ---------------------------------------------------------------------------

func TestPatternFromMetadata_explicitNullSkipped(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "server" && fieldPath == "host" {
			return spec.FieldMeta{Pattern: `^[a-z.]+$`}
		}
		return spec.FieldMeta{}
	})
	runMetadataRule(t, PatternFromMetadata(), hints, "server:\n  host: null\n", nil)
	runMetadataRule(t, PatternFromMetadata(), hints, "server:\n  host: ~\n", nil)
}

func TestCountFromMetadata_explicitNullIsEmptyCollection(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "tags" && fieldPath == "" {
			return spec.FieldMeta{MinCount: 1}
		}
		return spec.FieldMeta{}
	})
	v := wireMetadataRule(t, CountFromMetadata(), hints)
	errs := v.Validate(spec.NewValidationInput([]byte("tags: null\n"), nil))
	for _, e := range errs {
		if strings.Contains(e.Message, "expected a list or mapping") {
			t.Errorf("null must count as an empty collection, not a shape error: %v", e)
		}
	}
}

// ---------------------------------------------------------------------------
// Wire's goroutine-safety promise: concurrent RunAll over validators wired
// from the same PatternFromMetadata instance must not race on the shared
// regex cache (run with -race).
// ---------------------------------------------------------------------------

func TestPatternFromMetadata_concurrentRunAll(t *testing.T) {
	hints := spec.MetadataFunc(func(block, fieldPath string) spec.FieldMeta {
		if block == "server" && fieldPath == "host" {
			return spec.FieldMeta{Pattern: `^[a-z.]+$`}
		}
		return spec.FieldMeta{}
	})
	base := []spec.Validator{PatternFromMetadata()}
	tree := schema.Discover(&hintsConfig{})
	raw := []byte("server:\n  host: localhost\n")

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wired := WireWithSchema(base, tree, hints)
			for i := 0; i < 50; i++ {
				RunAll(wired, raw, nil)
			}
		}()
	}
	wg.Wait()
}
