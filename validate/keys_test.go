package validate

import (
	"strings"
	"testing"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/spec"
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
			name:          "two keys present - violation",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}},
			wantViolation: true,
			wantContains:  []string{"image", "build"},
		},
		{
			name:          "only one key - ok",
			blocks:        []document.Block{{Key: "image"}},
			wantViolation: false,
		},
		{
			name:          "none of the keys - ok",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput(nil, tc.blocks))
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
		"apps.workers.config.dispatch.push",
		"apps.workers.config.dispatch.pull",
	)

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
	}{
		{
			name: "both keys present - violation",
			raw: `
apps:
  backend:
    workers:
      - name: alpha
        config:
          dispatch:
            push: [x]
            pull: [y]
`,
			wantViolation: true,
		},
		{
			name: "only one key - ok",
			raw: `
apps:
  backend:
    workers:
      - name: alpha
        config:
          dispatch:
            push: [x]
`,
			wantViolation: false,
		},
		{
			name: "multiple workers - second violates",
			raw: `
apps:
  backend:
    workers:
      - name: ok
        config:
          dispatch:
            push: [x]
      - name: bad
        config:
          dispatch:
            push: [x]
            pull: [y]
`,
			wantViolation: true,
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
		})
	}
}

func TestMutuallyExclusive_topLevel_unchanged(t *testing.T) {
	blocks := []document.Block{{Key: "image"}, {Key: "build"}}
	v := MutuallyExclusive("image", "build")
	if errs := v.Validate(spec.NewValidationInput(nil, blocks)); len(errs) != 1 {
		t.Errorf("top-level behavior should be unchanged, got %v", errs)
	}
}

func TestMutuallyExclusive_misconfiguredPaths(t *testing.T) {
	tests := []struct {
		name string
		v    spec.Validator
	}{
		{"diverging parents", MutuallyExclusive("server.tls", "proxy.tls")},
		{"different depths", MutuallyExclusive("server.tls", "server.http.port")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.v.Validate(spec.NewValidationInput([]byte("server:\n  tls: true\n"), nil))
			if len(errs) != 1 {
				t.Fatalf("expected 1 misconfiguration violation, got %v", errs)
			}
			if !strings.Contains(errs[0].Message, "share the same parent prefix") {
				t.Errorf("violation should explain the misconfiguration; got %q", errs[0].Message)
			}
		})
	}
}

func TestAllOrNone_misconfiguredPaths(t *testing.T) {
	v := AllOrNone("server.tls-cert", "proxy.tls-key")
	errs := v.Validate(spec.NewValidationInput([]byte("server:\n  tls-cert: a\n"), nil))
	if len(errs) != 1 {
		t.Fatalf("expected 1 misconfiguration violation, got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "share the same parent prefix") {
		t.Errorf("violation should explain the misconfiguration; got %q", errs[0].Message)
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
			name:          "key present without parent - violation",
			blocks:        []document.Block{{Key: "service"}},
			wantViolation: true,
			wantContains:  []string{"service", "dockerComposeFile"},
		},
		{
			name:          "both present - ok",
			blocks:        []document.Block{{Key: "service"}, {Key: "dockerComposeFile"}},
			wantViolation: false,
		},
		{
			name:          "key absent - ok",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput(nil, tc.blocks))
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

func TestRequiredWith_dottedPath(t *testing.T) {
	v := RequiredWith("server.tls-key", "server.tls-cert")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name: "key present without parent - violation",
			raw: `
server:
  tls-key: /etc/tls/key.pem
`,
			wantViolation: true,
			wantContains:  []string{"server", "tls-key", "tls-cert"},
		},
		{
			name: "both present - ok",
			raw: `
server:
  tls-cert: /etc/tls/cert.pem
  tls-key: /etc/tls/key.pem
`,
		},
		{
			name: "key absent - ok",
			raw: `
server:
  host: localhost
`,
		},
		{
			name: "parent mapping absent - ok",
			raw:  "name: myapp\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, v, tc.raw, nil)
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

func TestRequiredWith_dottedPath_sequenceExpansion(t *testing.T) {
	v := RequiredWith("servers.tls-key", "servers.tls-cert")
	raw := `
servers:
  - name: a
    tls-cert: cert.pem
    tls-key: key.pem
  - name: b
    tls-key: key.pem
`
	got := runValidator(t, v, raw, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation (entry b only), got %v", got)
	}
	if !strings.Contains(got[0], "servers[1]") {
		t.Errorf("violation should point at servers[1]; got %q", got[0])
	}
}

func TestRequiredWith_misconfiguredPaths(t *testing.T) {
	v := RequiredWith("server.tls-key", "proxy.tls-cert")
	errs := v.Validate(spec.NewValidationInput([]byte("server:\n  tls-key: a\n"), nil))
	if len(errs) != 1 {
		t.Fatalf("expected 1 misconfiguration violation, got %v", errs)
	}
	if !strings.Contains(errs[0].Message, "share the same parent prefix") {
		t.Errorf("violation should explain the misconfiguration; got %q", errs[0].Message)
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
			name:          "none present - violation",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: true,
		},
		{
			name:          "one present - ok",
			blocks:        []document.Block{{Key: "image"}},
			wantViolation: false,
		},
		{
			name:          "multiple present - ok",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}},
			wantViolation: false,
		},
		{
			name:          "empty blocks - violation",
			blocks:        nil,
			wantViolation: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput(nil, tc.blocks))
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
			name:          "none present - violation",
			blocks:        []document.Block{{Key: "name"}},
			wantViolation: true,
			wantContains:  []string{"required"},
		},
		{
			name:          "one present - ok",
			blocks:        []document.Block{{Key: "image"}},
			wantViolation: false,
		},
		{
			name:          "two present - violation",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}},
			wantViolation: true,
			wantContains:  []string{"image", "build"},
		},
		{
			name:          "all three present - violation",
			blocks:        []document.Block{{Key: "image"}, {Key: "build"}, {Key: "dockerComposeFile"}},
			wantViolation: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput(nil, tc.blocks))
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
			name: "condition active, key absent - violation",
			raw: `
tls:
  enabled: "true"
`,
			wantViolation: true,
			wantContains:  []string{"tls.cert", "tls.enabled", "true"},
		},
		{
			name: "condition active, key present - ok",
			raw: `
tls:
  enabled: "true"
  cert: /etc/tls/cert.pem
`,
			wantViolation: false,
		},
		{
			name: "condition inactive (different value) - ok",
			raw: `
tls:
  enabled: "false"
`,
			wantViolation: false,
		},
		{
			name: "condition active, key present as mapping - ok",
			raw: `
tls:
  enabled: "true"
  cert:
    path: /etc/tls/cert.pem
`,
			wantViolation: false,
		},
		{
			name: "condition active, key present as empty scalar - violation",
			raw: `
tls:
  enabled: "true"
  cert:
`,
			wantViolation: true,
		},
		{
			name:          "condition path absent - ok",
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

func TestCrossFieldOrdered(t *testing.T) {
	tests := []struct {
		name          string
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "duration: smaller < larger - ok",
			validator: CrossFieldOrdered("limits.start", "limits.end"),
			raw: `
limits:
  start: 24h
  end: 168h
`,
			wantViolation: false,
		},
		{
			name:      "duration: smaller > larger - violation",
			validator: CrossFieldOrdered("limits.start", "limits.end"),
			raw: `
limits:
  start: 720h
  end: 24h
`,
			wantViolation: true,
			wantContains:  []string{"start", "end"},
		},
		{
			name:      "duration: equal values - violation",
			validator: CrossFieldOrdered("limits.start", "limits.end"),
			raw: `
limits:
  start: 24h
  end: 24h
`,
			wantViolation: true,
		},
		{
			name:      "size: smaller < larger - ok",
			validator: CrossFieldOrdered("quota.min", "quota.max"),
			raw: `
quota:
  min: 1MB
  max: 100MB
`,
			wantViolation: false,
		},
		{
			name:      "size: smaller > larger - violation",
			validator: CrossFieldOrdered("quota.min", "quota.max"),
			raw: `
quota:
  min: 500MB
  max: 100MB
`,
			wantViolation: true,
			wantContains:  []string{"min", "max"},
		},
		{
			name:      "size: SI suffixes are decimal (999KB < 1MB) - ok",
			validator: CrossFieldOrdered("quota.min", "quota.max"),
			raw: `
quota:
  min: 999KB
  max: 1MB
`,
			wantViolation: false,
		},
		{
			name:      "size: IEC suffixes are binary (1023KiB < 1MiB) - ok",
			validator: CrossFieldOrdered("quota.min", "quota.max"),
			raw: `
quota:
  min: 1023KiB
  max: 1MiB
`,
			wantViolation: false,
		},
		{
			name:      "size: 1024KiB equals 1MiB - violation",
			validator: CrossFieldOrdered("quota.min", "quota.max"),
			raw: `
quota:
  min: 1024KiB
  max: 1MiB
`,
			wantViolation: true,
		},
		{
			name:      "one field absent - ok",
			validator: CrossFieldOrdered("limits.start", "limits.end"),
			raw: `
limits:
  start: 24h
`,
			wantViolation: false,
		},
		{
			name:      "both absent - ok",
			validator: CrossFieldOrdered("limits.start", "limits.end"),
			raw: `
limits:
  label: "default"
`,
			wantViolation: false,
		},
		{
			name:      "incomparable types (mixed duration and size) - ok",
			validator: CrossFieldOrdered("limits.start", "quota.max"),
			raw: `
limits:
  start: 24h
quota:
  max: 100MB
`,
			wantViolation: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.validator.Validate(spec.NewValidationInput([]byte(tc.raw), nil))
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

func TestRequired(t *testing.T) {
	tests := []struct {
		name      string
		validator spec.Validator
		raw       string
		want      []string // exact violation strings, in order
	}{
		{
			name:      "top-level present - ok",
			validator: Required("version"),
			raw:       "version: 1.0.0\n",
		},
		{
			name:      "top-level absent - violation",
			validator: Required("version"),
			raw:       "name: myapp\n",
			want:      []string{"version: required"},
		},
		{
			name:      "empty document - top-level still required",
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
			name:      "dotted path - parent absent is ok",
			validator: Required("plugins.name"),
			raw:       "version: 1.0.0\n",
		},
		{
			name:      "dotted path - every sequence entry checked",
			validator: Required("plugins.name"),
			raw: `
plugins:
  - name: auth
  - version: 1
`,
			want: []string{"plugins[1].name: required"},
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

func TestAtLeastOneOf_dottedPath(t *testing.T) {
	v := AtLeastOneOf("auth.token", "auth.password")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name: "one present - ok",
			raw: `
auth:
  token: abc
`,
		},
		{
			name: "none present - violation",
			raw: `
auth:
  user: admin
`,
			wantViolation: true,
			wantContains:  []string{"auth", "at least one of", "token", "password"},
		},
		{
			name: "parent absent - ok",
			raw:  "name: myapp\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, v, tc.raw, nil)
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

func TestAtLeastOneOf_dottedPath_sequenceExpansion(t *testing.T) {
	v := AtLeastOneOf("accounts.auth.token", "accounts.auth.password")
	raw := `
accounts:
  - auth:
      token: abc
  - auth:
      user: admin
`
	got := runValidator(t, v, raw, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation (second account), got %v", got)
	}
	if !strings.Contains(got[0], "accounts[1].auth") {
		t.Errorf("violation should point at accounts[1].auth; got %q", got[0])
	}
}

func TestExactlyOneOf_dottedPath(t *testing.T) {
	v := ExactlyOneOf("source.git", "source.local")

	tests := []struct {
		name          string
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name: "exactly one - ok",
			raw: `
source:
  git: https://example.com/repo.git
`,
		},
		{
			name: "none - violation",
			raw: `
source:
  branch: main
`,
			wantViolation: true,
			wantContains:  []string{"source", "exactly one of"},
		},
		{
			name: "both - violation",
			raw: `
source:
  git: https://example.com/repo.git
  local: ./vendor
`,
			wantViolation: true,
			wantContains:  []string{"source", "found:", "git", "local"},
		},
		{
			name: "parent absent - ok",
			raw:  "name: myapp\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runValidator(t, v, tc.raw, nil)
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

func TestAtLeastOneOf_ExactlyOneOf_misconfiguredPaths(t *testing.T) {
	tests := []struct {
		name string
		v    spec.Validator
	}{
		{"AtLeastOneOf diverging parents", AtLeastOneOf("auth.token", "server.password")},
		{"ExactlyOneOf different depths", ExactlyOneOf("source.git", "source.local.path")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.v.Validate(spec.NewValidationInput([]byte("auth:\n  token: a\n"), nil))
			if len(errs) != 1 {
				t.Fatalf("expected 1 misconfiguration violation, got %v", errs)
			}
			if !strings.Contains(errs[0].Message, "share the same parent prefix") {
				t.Errorf("violation should explain the misconfiguration; got %q", errs[0].Message)
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
			name:   "both present - ok",
			blocks: []document.Block{{Key: "tls-cert"}, {Key: "tls-key"}},
		},
		{
			name:   "none present - ok",
			blocks: []document.Block{{Key: "name"}},
		},
		{
			name:          "only one present - violation",
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
			name: "both present - ok",
			raw: `
server:
  tls-cert: /etc/tls/cert.pem
  tls-key: /etc/tls/key.pem
`,
		},
		{
			name: "none present - ok",
			raw: `
server:
  host: localhost
`,
		},
		{
			name: "only one present - violation",
			raw: `
server:
  tls-cert: /etc/tls/cert.pem
`,
			wantViolation: true,
		},
		{
			name: "parent absent - ok",
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

func TestForbiddenIf(t *testing.T) {
	tests := []struct {
		name          string
		validator     spec.Validator
		raw           string
		wantViolation bool
		wantContains  []string
	}{
		{
			name:      "condition active, key absent - ok",
			validator: ForbiddenIf("server.write-token", "server.mode", "readonly"),
			raw:       "server:\n  mode: readonly\n",
		},
		{
			name:          "condition active, key present - violation",
			validator:     ForbiddenIf("server.write-token", "server.mode", "readonly"),
			raw:           "server:\n  mode: readonly\n  write-token: secret\n",
			wantViolation: true,
			wantContains:  []string{"server.write-token", "not allowed", "readonly"},
		},
		{
			name:      "condition inactive - key present - ok",
			validator: ForbiddenIf("server.write-token", "server.mode", "readonly"),
			raw:       "server:\n  mode: readwrite\n  write-token: secret\n",
		},
		{
			name:      "condition path absent - ok",
			validator: ForbiddenIf("server.write-token", "server.mode", "readonly"),
			raw:       "name: myapp\n",
		},
		{
			name:      "empty document - ok",
			validator: ForbiddenIf("server.write-token", "server.mode", "readonly"),
			raw:       "",
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

func TestForbiddenIf_sequenceExpansion(t *testing.T) {
	v := ForbiddenIf("servers.debug-key", "servers.mode", "readonly")
	raw := `
servers:
  - mode: readwrite
    debug-key: ok
  - mode: readonly
    debug-key: forbidden
  - mode: readonly
`
	got := runValidator(t, v, raw, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 violation (servers[1] only), got %v", got)
	}
	if !strings.Contains(got[0], "servers[1]") {
		t.Errorf("violation should point at servers[1]; got %q", got[0])
	}
}

func TestForbiddenIf_unrelatedParents(t *testing.T) {
	v := ForbiddenIf("debug.token", "mode", "production")
	tests := []struct {
		name          string
		raw           string
		wantViolation bool
	}{
		{
			name:          "condition active, key present - violation",
			raw:           "mode: production\ndebug:\n  token: secret\n",
			wantViolation: true,
		},
		{
			name: "condition active, key absent - ok",
			raw:  "mode: production\n",
		},
		{
			name: "condition inactive - ok",
			raw:  "mode: staging\ndebug:\n  token: secret\n",
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

func TestAtLeastOneOfNested(t *testing.T) {
	v := AtLeastOneOfNested("categories.source.auth", "token", "password")

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantInErr []string
	}{
		{
			name: "token present - ok",
			raw: `
categories:
  - source:
      auth:
        token: abc
`,
			wantCount: 0,
		},
		{
			name: "both present - ok",
			raw: `
categories:
  - source:
      auth:
        token: abc
        password: s3cr3t
`,
			wantCount: 0,
		},
		{
			name: "none present - violation",
			raw: `
categories:
  - source:
      auth:
        user: admin
`,
			wantCount: 1,
			wantInErr: []string{"auth", "at least one of", "token", "password"},
		},
		{
			name: "second entry violates",
			raw: `
categories:
  - source:
      auth:
        token: abc
  - source:
      auth:
        user: admin
`,
			wantCount: 1,
			wantInErr: []string{"categories[1]"},
		},
		{
			name:      "auth absent - not checked",
			raw:       "categories:\n  - source:\n      url: http://x\n",
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
			for _, want := range tc.wantInErr {
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("first error should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestExactlyOneOfNested(t *testing.T) {
	v := ExactlyOneOfNested("categories.source", "git", "local")

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantInErr []string
	}{
		{
			name: "exactly one - ok",
			raw: `
categories:
  - source:
      git: https://example.com/repo.git
`,
			wantCount: 0,
		},
		{
			name: "none present - violation",
			raw: `
categories:
  - source:
      url: http://x
`,
			wantCount: 1,
			wantInErr: []string{"source", "exactly one of"},
		},
		{
			name: "both present - violation",
			raw: `
categories:
  - source:
      git: https://example.com/repo.git
      local: ./vendor
`,
			wantCount: 1,
			wantInErr: []string{"source", "found:", "git", "local"},
		},
		{
			name: "second entry violates",
			raw: `
categories:
  - source:
      git: https://example.com/repo.git
  - source:
      git: https://x.com
      local: ./y
`,
			wantCount: 1,
			wantInErr: []string{"categories[1]"},
		},
		{
			name:      "source absent - not checked",
			raw:       "categories:\n  - name: tools\n",
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput([]byte(tc.raw), nil))
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

func TestAllOrNoneNested(t *testing.T) {
	v := AllOrNoneNested("servers.tls", "cert", "key")

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantInErr []string
	}{
		{
			name: "both present - ok",
			raw: `
servers:
  - tls:
      cert: /etc/tls/cert.pem
      key: /etc/tls/key.pem
`,
			wantCount: 0,
		},
		{
			name: "none present - ok",
			raw: `
servers:
  - tls:
      ca: /etc/tls/ca.pem
`,
			wantCount: 0,
		},
		{
			name: "only cert - violation",
			raw: `
servers:
  - tls:
      cert: /etc/tls/cert.pem
`,
			wantCount: 1,
			wantInErr: []string{"tls", "missing", "key"},
		},
		{
			name: "second entry violates",
			raw: `
servers:
  - tls:
      cert: /etc/tls/cert.pem
      key: /etc/tls/key.pem
  - tls:
      cert: /etc/tls/cert.pem
`,
			wantCount: 1,
			wantInErr: []string{"servers[1]"},
		},
		{
			name:      "tls absent - not checked",
			raw:       "servers:\n  - host: localhost\n",
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
			for _, want := range tc.wantInErr {
				if len(errs) > 0 && !strings.Contains(errs[0].String(), want) {
					t.Errorf("first error should contain %q; got %q", want, errs[0].String())
				}
			}
		})
	}
}

func TestMutuallyExclusiveNested(t *testing.T) {
	tests := []struct {
		name      string
		validator spec.Validator
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
			name:      "single key - ok",
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
			name:      "both top-level and nested - two violations",
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
			name:      "scoped path - catches violation inside scope",
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
			name:      "scoped path - ignores filters outside scope",
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
			errs := tc.validator.Validate(spec.NewValidationInput([]byte(tc.raw), nil))
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

func TestMutuallyExclusiveGroupsNested(t *testing.T) {
	// Generic schema: a "rule" block has two groups of mutually exclusive fields.
	// groupA (composite): "union", "intersect"
	// groupB (leaf):      "path", "name"
	v := MutuallyExclusiveGroupsNested(
		"services.rule",
		[]string{"union", "intersect"},
		[]string{"path", "name"},
	)

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantInErr []string
	}{
		{
			name: "groupA with groupB - violation",
			raw: `
services:
  - rule:
      union:
        - path: /a
      path: /b
`,
			wantCount: 1,
			wantInErr: []string{"rule", "union", "path"},
		},
		{
			name: "second groupA key with groupB - violation",
			raw: `
services:
  - rule:
      intersect:
        - name: foo
      name: bar
`,
			wantCount: 1,
			wantInErr: []string{"intersect", "name"},
		},
		{
			name: "only groupA - ok",
			raw: `
services:
  - rule:
      union:
        - path: /a
`,
			wantCount: 0,
		},
		{
			name: "only groupB - ok",
			raw: `
services:
  - rule:
      path: /a
      name: foo
`,
			wantCount: 0,
		},
		{
			// The "rule" scoped validator fires only at "rule"-parented mappings.
			// A violation inside union[i] requires a "rule.union" scoped validator.
			name: "violation inside union[i] - not caught by rule-scoped validator alone",
			raw: `
services:
  - rule:
      union:
        - union:
            - path: /a
          path: /b
`,
			wantCount: 0,
		},
		{
			name: "outside scope - not caught",
			raw: `
other:
  rule:
    union:
      - path: /a
    path: /b
services:
  - rule:
      path: /a
`,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput([]byte(tc.raw), nil))
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

func TestMutuallyExclusiveGroupsNested_threeGroups(t *testing.T) {
	// Three mutually exclusive groups: at most one family may appear at a time.
	v := MutuallyExclusiveGroupsNested(
		"services.rule",
		[]string{"image"},
		[]string{"build"},
		[]string{"compose"},
	)

	tests := []struct {
		name      string
		raw       string
		wantCount int
	}{
		{
			name: "only one group - ok",
			raw: `
services:
  - rule:
      image: ubuntu
`,
			wantCount: 0,
		},
		{
			name: "two groups present - one violation",
			raw: `
services:
  - rule:
      image: ubuntu
      build: ./app
`,
			wantCount: 1,
		},
		{
			name: "all three groups present - three violations (one per pair)",
			raw: `
services:
  - rule:
      image: ubuntu
      build: ./app
      compose: docker-compose.yml
`,
			wantCount: 3,
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
		})
	}
}

func TestMutuallyExclusiveGroupsNested_allLevels(t *testing.T) {
	// Two scoped validators together cover violations at any nesting depth:
	// - "rule" fires at every "rule"-parented mapping
	// - "rule.union" fires at every "union"-parented mapping (including nested ones)
	groupA := []string{"union", "intersect"}
	groupB := []string{"path", "name"}
	validators := []spec.Validator{
		MutuallyExclusiveGroupsNested("services.rule", groupA, groupB),
		MutuallyExclusiveGroupsNested("services.rule.union", groupA, groupB),
	}

	tests := []struct {
		name      string
		raw       string
		wantCount int
	}{
		{
			name: "violation at root rule level",
			raw: `
services:
  - rule:
      union:
        - path: /a
      path: /b
`,
			wantCount: 1,
		},
		{
			name: "violation inside union element",
			raw: `
services:
  - rule:
      union:
        - union:
            - path: /a
          path: /b
`,
			wantCount: 1,
		},
		{
			name: "violation at depth 2 (union inside union)",
			raw: `
services:
  - rule:
      union:
        - union:
            - union:
                - path: /a
              path: /b
`,
			wantCount: 1,
		},
		{
			name: "no violations",
			raw: `
services:
  - rule:
      union:
        - path: /a
        - name: foo
`,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := RunAll(WireNoSchema(validators), []byte(tc.raw), nil)
			if len(errs) != tc.wantCount {
				t.Fatalf("want %d violations, got %v", tc.wantCount, errs)
			}
		})
	}
}

func TestCrossFieldOrderedNested(t *testing.T) {
	// Generic schema: a "rule" block has a "range" sub-block with min/max fields.
	v := CrossFieldOrderedNested("services.rule.range", "min", "max")

	tests := []struct {
		name      string
		raw       string
		wantCount int
		wantInErr []string
	}{
		{
			name: "violation at root level",
			raw: `
services:
  - rule:
      range:
        min: 48
        max: 24
`,
			wantCount: 1,
			wantInErr: []string{"min", "max", "48", "24"},
		},
		{
			name: "violation inside group element",
			raw: `
services:
  - rule:
      group:
        - range:
            min: 72
            max: 1
`,
			wantCount: 1,
			wantInErr: []string{"range"},
		},
		{
			name: "violation at depth 2 (group inside group)",
			raw: `
services:
  - rule:
      group:
        - group:
            - range:
                min: 100
                max: 10
`,
			wantCount: 1,
		},
		{
			name: "properly ordered - ok",
			raw: `
services:
  - rule:
      range:
        min: 24
        max: 48
`,
			wantCount: 0,
		},
		{
			name: "only min set - ok",
			raw: `
services:
  - rule:
      range:
        min: 24
`,
			wantCount: 0,
		},
		{
			name: "outside scope - not caught",
			raw: `
other:
  rule:
    range:
      min: 48
      max: 1
services:
  - rule:
      range:
        min: 1
        max: 48
`,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := v.Validate(spec.NewValidationInput([]byte(tc.raw), nil))
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
