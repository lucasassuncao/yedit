package validate

import (
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/spec"
)

func TestValueOneOf(t *testing.T) {
	v := ValueOneOf("configuration.log-level", "trace", "debug", "info", "warn", "error", "fatal")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name: "allowed value - ok",
			raw: `
configuration:
  log-level: info
`,
			wantViolation: false,
		},
		{
			name: "disallowed value - violation",
			raw: `
configuration:
  log-level: verbose
`,
			wantViolation: true,
			wantContains:  []string{"log-level", "verbose", "trace", "fatal"},
		},
		{
			name: "non-scalar value - violation",
			raw: `
configuration:
  log-level:
    file: debug
`,
			wantViolation: true,
			wantContains:  []string{"log-level", "scalar"},
		},
		{
			name: "field absent - ok",
			raw: `
configuration:
  output: console
`,
			wantViolation: false,
		},
		{
			name:          "path absent - ok",
			raw:           `name: myapp`,
			wantViolation: false,
		},
		{
			name:          "empty document - ok",
			raw:           "",
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput([]byte(tc.raw), nil))
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
	v := NoDuplicates("plugins", "name")

	tests := []struct {
		name         string
		raw          string
		wantCount    int
		wantContains []string
	}{
		{
			name: "no duplicates - ok",
			raw: `
plugins:
  - name: auth
  - name: cache
  - name: logger
`,
			wantCount: 0,
		},
		{
			name: "one duplicate - one violation",
			raw: `
plugins:
  - name: auth
  - name: cache
  - name: auth
`,
			wantCount:    1,
			wantContains: []string{"plugins[2]", "auth", "plugins[0]"},
		},
		{
			name: "two distinct duplicates - two violations",
			raw: `
plugins:
  - name: auth
  - name: cache
  - name: auth
  - name: cache
`,
			wantCount: 2,
		},
		{
			name: "item without the field - skipped",
			raw: `
plugins:
  - name: auth
  - version: 1
  - name: auth
`,
			wantCount:    1,
			wantContains: []string{"plugins[2]"},
		},
		{
			name:      "empty sequence - ok",
			raw:       `plugins: []`,
			wantCount: 0,
		},
		{
			name:      "path not a sequence - ok",
			raw:       `plugins: auth`,
			wantCount: 0,
		},
		{
			name:      "path absent - ok",
			raw:       `name: myapp`,
			wantCount: 0,
		},
		{
			name:      "empty document - ok",
			raw:       "",
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput([]byte(tc.raw), nil))
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

func TestValueInRange(t *testing.T) {
	tests := []struct {
		name          string
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "number within range - ok",
			validator: ValueInRange("server.port", "1", "65535"),
			raw:       "server:\n  port: 8080\n",
		},
		{
			name:          "number out of range - violation",
			validator:     ValueInRange("server.port", "1", "65535"),
			raw:           "server:\n  port: 70000\n",
			wantViolation: true,
			wantContains:  []string{"server.port", "70000", "out of range"},
		},
		{
			name:      "duration within range - ok",
			validator: ValueInRange("schedule.timeout", "1h", "8760h"),
			raw:       "schedule:\n  timeout: 24h\n",
		},
		{
			name:          "duration below range - violation",
			validator:     ValueInRange("schedule.timeout", "1h", "8760h"),
			raw:           "schedule:\n  timeout: 30m\n",
			wantViolation: true,
		},
		{
			name:      "size within range - ok",
			validator: ValueInRange("storage.quota", "1MB", "1GB"),
			raw:       "storage:\n  quota: 100MB\n",
		},
		{
			name:      "absent path - ok",
			validator: ValueInRange("server.port", "1", "65535"),
			raw:       "name: myapp\n",
		},
		{
			name:      "empty value - ok",
			validator: ValueInRange("server.port", "1", "65535"),
			raw:       "server:\n  port:\n",
		},
		{
			name:          "non-scalar value - violation",
			validator:     ValueInRange("server.port", "1", "65535"),
			raw:           "server:\n  port:\n    internal: 8080\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "value not comparable with range - violation",
			validator:     ValueInRange("server.port", "1", "65535"),
			raw:           "server:\n  port: eighty\n",
			wantViolation: true,
			wantContains:  []string{"not comparable"},
		},
		{
			name:          "mixed-kind bounds - misconfiguration reported",
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
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "match - ok",
			validator: ValueMatches("version", `^\d+\.\d+\.\d+$`),
			raw:       "version: 1.2.3\n",
		},
		{
			name:          "mismatch - violation",
			validator:     ValueMatches("version", `^\d+\.\d+\.\d+$`),
			raw:           "version: latest\n",
			wantViolation: true,
			wantContains:  []string{"version", "latest", "does not match"},
		},
		{
			name:      "absent path - ok",
			validator: ValueMatches("version", `^\d+$`),
			raw:       "name: myapp\n",
		},
		{
			name:      "empty value - ok",
			validator: ValueMatches("version", `^\d+$`),
			raw:       "version:\n",
		},
		{
			name:          "non-scalar value - violation",
			validator:     ValueMatches("version", `^\d+$`),
			raw:           "version:\n  major: 1\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "invalid pattern - misconfiguration reported",
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

func TestNoDuplicates_expandedPath(t *testing.T) {
	v := NoDuplicates("categories.installers", "name")
	raw := `
categories:
  - name: tools
    installers:
      - name: git
      - name: git
  - name: apps
    installers:
      - name: git
`
	got := runValidator(t, v, raw, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation (duplicate within first category only), got %v", got)
	}
	if !strings.Contains(got[0], "categories[0].installers[1].name") {
		t.Errorf("violation should point at categories[0].installers[1].name; got %q", got[0])
	}
}

func TestNoDuplicates_dottedField(t *testing.T) {
	v := NoDuplicates("servers", "meta.name")
	raw := `
servers:
  - meta:
      name: a
  - meta:
      name: a
`
	got := runValidator(t, v, raw, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation, got %v", got)
	}
	if !strings.Contains(got[0], `duplicate value "a"`) {
		t.Errorf("violation should report the duplicate value; got %q", got[0])
	}
}

func TestValueHasPrefixSuffix(t *testing.T) {
	tests := []struct {
		name          string
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "prefix match - ok",
			validator: ValueHasPrefix("image", "registry.example.com/"),
			raw:       "image: registry.example.com/app:1.0\n",
		},
		{
			name:          "prefix mismatch - violation",
			validator:     ValueHasPrefix("image", "registry.example.com/"),
			raw:           "image: docker.io/app:1.0\n",
			wantViolation: true,
			wantContains:  []string{"image", "docker.io/app:1.0", "does not start with"},
		},
		{
			name:      "suffix match - ok",
			validator: ValueHasSuffix("output", ".yaml"),
			raw:       "output: result.yaml\n",
		},
		{
			name:          "suffix mismatch - violation",
			validator:     ValueHasSuffix("output", ".yaml"),
			raw:           "output: result.json\n",
			wantViolation: true,
			wantContains:  []string{"output", "result.json", "does not end with"},
		},
		{
			name:      "absent path - ok",
			validator: ValueHasPrefix("image", "registry/"),
			raw:       "name: myapp\n",
		},
		{
			name:      "empty value - ok",
			validator: ValueHasSuffix("output", ".yaml"),
			raw:       "output:\n",
		},
		{
			name:          "non-scalar value - violation",
			validator:     ValueHasPrefix("image", "registry/"),
			raw:           "image:\n  name: app\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "sequence expansion - violation points at item",
			validator:     ValueHasPrefix("mirrors.url", "https://"),
			raw:           "mirrors:\n  - url: https://a.example.com\n  - url: http://b.example.com\n",
			wantViolation: true,
			wantContains:  []string{"mirrors[1].url", "http://b.example.com"},
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

func TestCountRange(t *testing.T) {
	tests := []struct {
		name          string
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "within range - ok",
			validator: CountRange("workers", 1, 10),
			raw:       "workers:\n  - name: a\n  - name: b\n",
		},
		{
			name:          "below min - violation",
			validator:     CountRange("workers", 1, 10),
			raw:           "workers: []\n",
			wantViolation: true,
			wantContains:  []string{"workers", "0 entries", "between 1 and 10"},
		},
		{
			name:          "above max - violation",
			validator:     CountRange("workers", 0, 1),
			raw:           "workers:\n  - name: a\n  - name: b\n",
			wantViolation: true,
		},
		{
			name:      "no upper bound - ok",
			validator: CountRange("workers", 1, -1),
			raw:       "workers:\n  - name: a\n  - name: b\n  - name: c\n",
		},
		{
			name:          "no upper bound, below min - violation",
			validator:     CountRange("workers", 2, -1),
			raw:           "workers:\n  - name: a\n",
			wantViolation: true,
			wantContains:  []string{"at least 2"},
		},
		{
			name:      "mapping counts keys - ok",
			validator: CountRange("port-attrs", 1, 2),
			raw:       "port-attrs:\n  \"8080\":\n    label: web\n",
		},
		{
			name:      "absent path - ok",
			validator: CountRange("workers", 1, 10),
			raw:       "name: myapp\n",
		},
		{
			name:          "scalar at path - violation",
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
			name:      "unique - ok",
			raw:       "tags: [go, yaml, tui]\n",
			wantCount: 0,
		},
		{
			name:         "duplicate - violation with indices",
			raw:          "tags: [go, yaml, go]\n",
			wantCount:    1,
			wantContains: []string{"tags[2]", `"go"`, "tags[0]"},
		},
		{
			name:      "two distinct duplicates - two violations",
			raw:       "tags: [a, b, a, b]\n",
			wantCount: 2,
		},
		{
			name:      "non-scalar items skipped",
			raw:       "tags:\n  - name: x\n  - name: x\n",
			wantCount: 0,
		},
		{
			name:      "absent path - ok",
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
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:          "present - violation with hint",
			validator:     Deprecated("dockerFile", "use build.dockerfile instead"),
			raw:           "dockerFile: Dockerfile\n",
			wantViolation: true,
			wantContains:  []string{"dockerFile", "deprecated", "use build.dockerfile instead"},
		},
		{
			name:      "absent - ok",
			validator: Deprecated("dockerFile", "use build.dockerfile instead"),
			raw:       "build:\n  dockerfile: Dockerfile\n",
		},
		{
			name:          "present with null value - still deprecated",
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

func TestValueNotOneOf(t *testing.T) {
	v := ValueNotOneOf("server.protocol", "ftp", "telnet")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name: "allowed value - ok",
			raw:  "server:\n  protocol: https\n",
		},
		{
			name:          "denied value - violation",
			raw:           "server:\n  protocol: ftp\n",
			wantViolation: true,
			wantContains:  []string{"server.protocol", `"ftp"`, "not allowed"},
		},
		{
			name:          "second denied value - violation",
			raw:           "server:\n  protocol: telnet\n",
			wantViolation: true,
		},
		{
			name: "absent path - ok",
			raw:  "name: myapp\n",
		},
		{
			name: "empty value - ok",
			raw:  "server:\n  protocol:\n",
		},
		{
			name:          "non-scalar value - violation (scalar expected)",
			raw:           "server:\n  protocol:\n    sub: ftp\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "sequence expansion - flags denied item",
			raw:           "server:\n  - protocol: https\n  - protocol: ftp\n",
			wantViolation: true,
			wantContains:  []string{"server[1].protocol"},
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
			for _, want := range tc.wantContains {
				if len(got) > 0 && !strings.Contains(got[0], want) {
					t.Errorf("violation should contain %q; got %q", want, got[0])
				}
			}
		})
	}
}

func TestValueHasLength(t *testing.T) {
	tests := []struct {
		name          string
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "within range - ok",
			validator: ValueHasLength("name", 3, 10),
			raw:       "name: hello\n",
		},
		{
			name:          "too short - violation",
			validator:     ValueHasLength("name", 3, 10),
			raw:           "name: hi\n",
			wantViolation: true,
			wantContains:  []string{"name", "between 3 and 10"},
		},
		{
			name:          "too long - violation",
			validator:     ValueHasLength("name", 3, 10),
			raw:           "name: this-is-too-long\n",
			wantViolation: true,
			wantContains:  []string{"name", "between 3 and 10"},
		},
		{
			name:      "max only - ok",
			validator: ValueHasLength("name", 0, 10),
			raw:       "name: hello\n",
		},
		{
			name:          "max only - violation",
			validator:     ValueHasLength("name", 0, 5),
			raw:           "name: too-long\n",
			wantViolation: true,
			wantContains:  []string{"at most 5"},
		},
		{
			name:          "min only - violation",
			validator:     ValueHasLength("name", 5, 0),
			raw:           "name: hi\n",
			wantViolation: true,
			wantContains:  []string{"at least 5"},
		},
		{
			name:      "min only - ok",
			validator: ValueHasLength("name", 3, 0),
			raw:       "name: hello world\n",
		},
		{
			name:      "absent path - ok",
			validator: ValueHasLength("name", 3, 10),
			raw:       "version: 1\n",
		},
		{
			name:      "empty value - ok",
			validator: ValueHasLength("name", 3, 10),
			raw:       "name:\n",
		},
		{
			name:          "non-scalar - violation (scalar expected)",
			validator:     ValueHasLength("name", 3, 10),
			raw:           "name:\n  first: a\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "unicode code points counted correctly",
			validator:     ValueHasLength("label", 3, 5),
			raw:           "label: héllo\n", // 5 code points
			wantViolation: false,
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

func TestValueMatchesFormat(t *testing.T) {
	tests := []struct {
		name          string
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "valid URL - ok",
			validator: ValueMatchesFormat("endpoint", spec.FormatURL),
			raw:       "endpoint: https://example.com\n",
		},
		{
			name:          "invalid URL - violation",
			validator:     ValueMatchesFormat("endpoint", spec.FormatURL),
			raw:           "endpoint: not-a-url\n",
			wantViolation: true,
			wantContains:  []string{"endpoint", "url"},
		},
		{
			name:      "OR semantics - second format matches - ok",
			validator: ValueMatchesFormat("addr", spec.FormatURL, spec.FormatIPv4),
			raw:       "addr: 192.168.1.1\n",
		},
		{
			name:          "OR semantics - neither matches - violation lists both",
			validator:     ValueMatchesFormat("addr", spec.FormatURL, spec.FormatIPv4),
			raw:           "addr: not-valid\n",
			wantViolation: true,
			wantContains:  []string{"addr", "url", "ipv4"},
		},
		{
			name:      "absent path - ok",
			validator: ValueMatchesFormat("endpoint", spec.FormatURL),
			raw:       "name: myapp\n",
		},
		{
			name:      "empty value - ok",
			validator: ValueMatchesFormat("endpoint", spec.FormatURL),
			raw:       "endpoint:\n",
		},
		{
			name:          "non-scalar - violation (scalar expected)",
			validator:     ValueMatchesFormat("endpoint", spec.FormatURL),
			raw:           "endpoint:\n  host: example.com\n",
			wantViolation: true,
			wantContains:  []string{"scalar"},
		},
		{
			name:          "sequence expansion - flags invalid item",
			validator:     ValueMatchesFormat("mirrors.url", spec.FormatURL),
			raw:           "mirrors:\n  - url: https://a.com\n  - url: ftp://bad\n",
			wantViolation: true,
			wantContains:  []string{"mirrors[1].url"},
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

func TestValueOneOf_violationPath(t *testing.T) {
	v := ValueOneOf("configuration.log-level", "info")
	errs := v.Validate(spec.NewValidationInput([]byte("configuration:\n  log-level: verbose\n"), nil))
	if len(errs) != 1 {
		t.Fatalf("expected 1 violation, got %v", errs)
	}
	if errs[0].Path != "configuration.log-level" {
		t.Errorf("Path = %q, want %q", errs[0].Path, "configuration.log-level")
	}
}

// ---------------------------------------------------------------------------
// Explicit null handling: "key: null" / "key: ~" must behave like an absent
// or empty value, never as the literal string "null"/"~".
// ---------------------------------------------------------------------------

func TestValueMatches_explicitNullSkipped(t *testing.T) {
	v := ValueMatches("server.host", `^[a-z.]+$`)
	for _, raw := range []string{"server:\n  host: null\n", "server:\n  host: ~\n"} {
		if errs := v.Validate(spec.NewValidationInput([]byte(raw), nil)); len(errs) != 0 {
			t.Errorf("raw=%q: explicit null must be skipped, got %v", raw, errs)
		}
	}
}

func TestCountRange_explicitNullIsEmptyCollection(t *testing.T) {
	v := CountRange("tags", 1, -1)
	errs := v.Validate(spec.NewValidationInput([]byte("tags: null\n"), nil))
	if len(errs) != 1 {
		t.Fatalf("want exactly one count violation, got %v", errs)
	}
	if strings.Contains(errs[0].Message, "expected a list or mapping") {
		t.Errorf("null must count as an empty collection, not a shape error: %v", errs[0])
	}
}
