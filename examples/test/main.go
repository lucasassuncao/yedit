// Command test is a self-contained yedit example that exercises every schema
// pattern, Config option, and known edge case.
//
// Run from the yedit root:
//
//	go run ./examples/test
//	go run ./examples/test --theme dracula
//	go run ./examples/test --read-only
//	go run ./examples/test --no-save-confirm
//	go run ./examples/test --no-delete-confirm
//
// A test.yaml is created on first run and reused on subsequent runs so undo,
// save, and validate can be exercised across restarts.
//
// # Patterns demonstrated
//
//	Pattern 1 — Primitives (no tree, YAML pane only)
//	  KindPrimitive : app-name (string), debug (bool), version (string required),
//	                  port (int default=8080), ratio (float64), build-timeout (duration)
//	  KindDictionary: labels (map[string]string), settings (map[string]any)
//	  KindList      : tags ([]string), ports ([]int)           — no child defs
//	  KindVariant   : timeout                                   — Provider interface
//
//	Pattern 2 — KindObject (ADDED/AVAILABLE tree with nested structs)
//	  server    : flat struct with []string and map[string]string leaves
//	  database  : struct with nested Pool (3-level nesting)
//	  logging   : struct with KindEnum (level) and bool
//	  deploy    : struct with *bool pointer and KindEnum
//
//	Pattern 3 — KindList with child defs ([N] sequence navigator)
//	  workers   : []Worker  (name required, concurrency, queue, tags []string)
//	  routes    : []Route   (path, method oneof, handler, auth bool)
//	  filters   : []Filter  (self-referential, cycle-detected: any []Filter, all []Filter)
//
//	Pattern 4 — KindDictionary with child defs (map[key]struct navigator)
//	  port-attrs: map[string]PortAttr (label string, on-auto-forward oneof)
//
//	Pattern 5 — Schema edge cases (items 6–10 from robustness audit)
//	  embed     : embeddedMeta anonymous embed → created-by, version-tag promoted to root
//	  inline    : inlineAnnotations yaml:",inline" → team, contact promoted to root
//	  omitempty : replicas (int,omitempty) — FieldDef.OmitEmpty = true
//	  flow      : ips ([]string,flow) — FieldDef.Flow = true
//	  int key   : firewall-rules map[int]PortRule — FieldDef.MapKeyScalar = "int"
//	  marshaler : background (Color via MarshalYAML) — KindPrimitive, no R/G/B sub-fields
//	  any       : extras (interface{}) — KindAny
//
//	Config options exercised
//	  PassthroughKeys  : "import" is preserved as-is, hidden from all sections
//	  Hidden           : "internal-id" is never shown in the UI
//	  PreCheckedFields : opening a new "server" block pre-checks host and port
//	  FieldSnippets    : toggling a struct field inserts a real default value
//	  FieldExamples    : hint panel shows realistic examples per field
//	  Validators       : MutuallyExclusive(server, proxy), RequiredWith(routes, server)
//	  NoDeleteConfirm  : controlled by --no-delete-confirm flag
//	  NoSaveConfirm    : controlled by --no-save-confirm flag
//	  ReadOnly         : controlled by --read-only flag
//
//	Edge cases in seed YAML
//	  unknown-key      : present in file, not in schema → UNKNOWN section + ctrl+l
//	  import           : passthrough key, silently preserved
//	  extensions field : flow-style list ["go","yaml"] inside a worker entry
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

// ── Pattern 1: KindVariant via Provider ──────────────────────────────────────

// TimeoutValue is a union type handled via schema.Provider.
// yedit skips reflection and uses YeditSchema() directly.
type TimeoutValue struct{}

func (TimeoutValue) YeditSchema() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "connect", Kind: schema.KindPrimitive, Default: "5s"},
		{YAMLName: "read", Kind: schema.KindPrimitive, Default: "30s"},
		{YAMLName: "write", Kind: schema.KindPrimitive, Default: "30s"},
	}
}

// ── Pattern 2: KindObject (ADDED/AVAILABLE tree) ──────────────────────────────

// ServerConfig: flat struct with slice and map leaves.
type ServerConfig struct {
	Host       string            `yaml:"host"`
	Port       int               `yaml:"port"        jsonschema:"default=8080"`
	TLS        bool              `yaml:"tls"`
	AllowedIPs []string          `yaml:"allowed-ips"`
	Headers    map[string]string `yaml:"headers"`
}

// PoolConfig: nested struct (depth 3 from root).
type PoolConfig struct {
	MinSize int `yaml:"min-size" jsonschema:"default=2"`
	MaxSize int `yaml:"max-size" jsonschema:"default=10"`
	Timeout int `yaml:"timeout"  jsonschema:"default=30"`
}

// DatabaseConfig: nested struct + oneof + required.
type DatabaseConfig struct {
	Driver   string     `yaml:"driver"    validate:"required,oneof=postgres mysql sqlite"`
	DSN      string     `yaml:"dsn"       validate:"required"`
	MaxConns int        `yaml:"max-conns" jsonschema:"default=10"`
	Pool     PoolConfig `yaml:"pool"`
}

// LoggingConfig: flat struct with bool and enum.
type LoggingConfig struct {
	Level      string `yaml:"level"       validate:"oneof=debug info warn error" jsonschema:"default=info"`
	File       string `yaml:"file"`
	ShowCaller bool   `yaml:"show-caller"`
}

// DeployConfig: struct with *bool pointer and enum — from movelooper patterns.
type DeployConfig struct {
	Enabled    *bool  `yaml:"enabled"`
	Strategy   string `yaml:"strategy"   validate:"oneof=rolling blue-green canary"`
	Replicas   int    `yaml:"replicas"   jsonschema:"default=1"`
	AutoRevert bool   `yaml:"auto-revert"`
}

// ── Pattern 3: KindList with child defs ──────────────────────────────────────

// Worker: seq-item struct with []string leaf.
type Worker struct {
	Name        string   `yaml:"name"        validate:"required"`
	Concurrency int      `yaml:"concurrency" jsonschema:"default=1"`
	Queue       string   `yaml:"queue"`
	Extensions  []string `yaml:"extensions"` // edge: flow-style ["go","yaml"] in seed
	Tags        []string `yaml:"tags"`
}

// Route: seq-item with oneof.
type Route struct {
	Path    string `yaml:"path"    validate:"required"`
	Method  string `yaml:"method"  validate:"required,oneof=GET POST PUT DELETE PATCH"`
	Handler string `yaml:"handler" validate:"required"`
	Auth    bool   `yaml:"auth"`
}

// Filter: self-referential type (any/all contain []Filter).
// With depth limit = 10, up to 10 levels of nesting are discoverable.
type Filter struct {
	Regex         string   `yaml:"regex"`
	Glob          string   `yaml:"glob"`
	Include       []string `yaml:"include"`
	Ignore        []string `yaml:"ignore"`
	CaseSensitive bool     `yaml:"case-sensitive"`
	Any           []Filter `yaml:"any"`
	All           []Filter `yaml:"all"`
}

// ── Pattern 4: KindDictionary with child defs ────────────────────────────────

// PortAttr: the value-struct for map[string]PortAttr (port-attrs).
// Demonstrates the KindDictionary + child defs navigator.
type PortAttr struct {
	Label         string `yaml:"label"`
	OnAutoForward string `yaml:"on-auto-forward" validate:"oneof=notify openBrowser openPreview ignore silent"`
	Protocol      string `yaml:"protocol"        validate:"oneof=http https tcp udp"`
}

// ── Pattern 5: Schema edge cases ─────────────────────────────────────────────

// embeddedMeta: promoted via anonymous embed (item 6a).
type embeddedMeta struct {
	CreatedBy  string `yaml:"created-by"`
	VersionTag string `yaml:"version-tag"`
}

// inlineAnnotations: promoted via yaml:",inline" (item 6b).
type inlineAnnotations struct {
	Team    string `yaml:"team"`
	Contact string `yaml:"contact"`
}

// Color implements yaml.Marshaler — Discover classifies it as KindPrimitive (item 9).
// The editor shows it as a text scalar ("#rrggbb"), not as R/G/B sub-fields.
type Color struct{ R, G, B uint8 }

func (c Color) MarshalYAML() (any, error) { //nolint:unparam
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B), nil
}

func (c *Color) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return fmt.Errorf("invalid color %q: want #rrggbb", "#"+s)
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	c.R, c.G, c.B = uint8(r), uint8(g), uint8(b)
	return nil
}

// PortRule: value type for map[int]PortRule — demonstrates non-string map key (item 8).
type PortRule struct {
	Proto   string `yaml:"proto"   validate:"oneof=tcp udp"`
	Allowed bool   `yaml:"allowed"`
}

// SchemaEdgeCases exercises items 6, 7, 8, 9, 10 in a single block.
type SchemaEdgeCases struct {
	embeddedMeta                       // item 6a: anonymous embed → created-by, version-tag promoted
	inlineAnnotations `yaml:",inline"` // item 6b: yaml:",inline" → team, contact promoted

	Replicas int      `yaml:"replicas,omitempty"` // item 7: OmitEmpty=true
	IPs      []string `yaml:"ips,flow"`           // item 7: Flow=true

	FirewallRules map[int]PortRule `yaml:"firewall-rules"` // item 8: MapKeyScalar="int"

	Background Color `yaml:"background"` // item 9: MarshalYAML → KindPrimitive, no sub-fields

	Extras any `yaml:"extras"` // item 10: interface{} → KindAny
}

// ── Root config ───────────────────────────────────────────────────────────────

type TestConfig struct {
	// Hidden by Config.Hidden — never appears in the UI.
	InternalID string `yaml:"internal-id"`

	// Pattern 1 — KindPrimitive
	AppName      string        `yaml:"app-name"`
	Debug        bool          `yaml:"debug"`
	Version      string        `yaml:"version"       validate:"required" jsonschema:"default=0.1.0"`
	Port         int           `yaml:"port"          jsonschema:"default=8080"`
	Ratio        float64       `yaml:"ratio"         jsonschema:"default=1.0"`
	BuildTimeout time.Duration `yaml:"build-timeout"`

	// Pattern 1 — KindDictionary (free-form, no child defs)
	Labels   map[string]string `yaml:"labels"`
	Settings map[string]any    `yaml:"settings"`

	// Pattern 1 — KindList, no child defs
	Tags  []string `yaml:"tags"`
	Ports []int    `yaml:"ports"`

	// Pattern 1 — KindVariant via Provider
	Timeout TimeoutValue `yaml:"timeout"`

	// Pattern 2 — KindObject structs
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logging  LoggingConfig  `yaml:"logging"`
	Deploy   DeployConfig   `yaml:"deploy"`

	// Pattern 3 — KindList with child defs
	Workers []Worker `yaml:"workers"`
	Routes  []Route  `yaml:"routes"`
	Filters []Filter `yaml:"filters"`

	// Pattern 4 — KindDictionary with child defs
	PortAttrs map[string]PortAttr `yaml:"port-attrs"`

	// Pattern 5 — Schema edge cases
	EdgeCases SchemaEdgeCases `yaml:"edge-cases"`
}

// ── Seed YAML ─────────────────────────────────────────────────────────────────

const seedYAML = `import: shared.yaml

app-name: "my-app"
debug: false
version: "0.1.0"
port: 8080
ratio: 1.5
build-timeout: 30s
tags:
  - go
  - tui
settings:
  cache: true
  max-upload: 10mb
server:
  host: localhost
  port: 8080
  tls: false
  allowed-ips:
    - 127.0.0.1
    - 10.0.0.0/8
  headers:
    X-Request-ID: ""
    X-Service-Name: "my-app"
logging:
  level: info
  show-caller: false
workers:
  - name: "default"
    concurrency: 2
    queue: "main"
    extensions: ["go", "yaml"]
    tags:
      - critical
  - name: "background"
    concurrency: 1
    queue: "low"
    tags: []
  - name: "heavy"
    concurrency: 4
    queue: "batch"
port-attrs:
  "3000":
    label: "frontend"
    on-auto-forward: openBrowser
    protocol: http
  "8080":
    label: "api"
    on-auto-forward: notify
    protocol: http
filters:
  - glob: "*.go"
    case-sensitive: true
  - regex: ".*_test\\.go$"
    ignore:
      - vendor
edge-cases:
  created-by: "alice"
  version-tag: "v1.0"
  team: "platform"
  contact: "platform@example.com"
  replicas: 3
  ips: [10.0.0.1, 10.0.0.2]
  firewall-rules:
    8080:
      proto: tcp
      allowed: true
    443:
      proto: tcp
      allowed: true
  background: "#1e1e2e"
  extras: "free-form string"
unknown-key: "flagged by ctrl+l validate"
`

// ── Theme ─────────────────────────────────────────────────────────────────────

func appTheme(name string) theme.Theme {
	all := theme.All()
	if t, ok := all[name]; ok {
		return theme.Theme{Base: &t}
	}
	return theme.Theme{} // default dark
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	themeName := flag.String("theme", "dark", "theme preset (run --list-themes for options)")
	readOnly := flag.Bool("read-only", false, "open in read-only mode")
	noSaveConfirm := flag.Bool("no-save-confirm", false, "skip save confirmation dialog")
	noDeleteConfirm := flag.Bool("no-delete-confirm", false, "skip delete confirmation dialog")
	noValidate := flag.Bool("no-validate", false, "allow saving with validation errors")
	flag.Parse()

	const path = "test.yaml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(seedYAML), 0600); err != nil {
			panic(err)
		}
	}

	if err := editor.Run(editor.Config{
		Theme:  appTheme(*themeName),
		Path:   path,
		Schema: &TestConfig{},
		Title:  "yedit test",

		// Config.Hidden: fields never shown in the UI.
		Hidden: []string{"internal-id"},

		// Config.PassthroughKeys: preserved as-is, hidden from all sections,
		// excluded from unknown-key validation (ctrl+l).
		PassthroughKeys: []string{"import"},

		// Config.ReadOnly / NoSaveConfirm / NoDeleteConfirm / NoValidateOnSave.
		ReadOnly:         *readOnly,
		NoSaveConfirm:    *noSaveConfirm,
		NoDeleteConfirm:  *noDeleteConfirm,
		NoValidateOnSave: *noValidate,

		// Config.PreCheckedFields: fields toggled ON automatically when opening a
		// new (not yet existing) block. Opening an existing block is unaffected.
		PreCheckedFields: map[string][]string{
			"server":  {"host", "port"},
			"logging": {"level"},
			"deploy":  {"strategy", "replicas"},
		},

		// Config.FieldSnippets: YAML inserted when a struct field is toggled ON.
		FieldSnippets: map[string]map[string]string{
			"server": {
				"host":        "  host: localhost\n",
				"port":        "  port: 8080\n",
				"tls":         "  tls: false\n",
				"allowed-ips": "  allowed-ips:\n    - 127.0.0.1\n",
				"headers":     "  headers:\n    X-Request-ID: \"\"\n",
			},
			"database": {
				"driver":    "  driver: postgres\n",
				"dsn":       "  dsn: \"postgres://localhost/mydb\"\n",
				"max-conns": "  max-conns: 10\n",
				"pool":      "  pool:\n    min-size: 2\n    max-size: 10\n    timeout: 30\n",
			},
			"logging": {
				"level":       "  level: info\n",
				"file":        "  file: \"/var/log/app.log\"\n",
				"show-caller": "  show-caller: false\n",
			},
			"deploy": {
				"strategy": "  strategy: rolling\n",
				"replicas": "  replicas: 1\n",
				"enabled":  "  enabled: true\n",
			},
		},

		// Config.FieldExamples: shown in the hint panel when a field is selected.
		FieldExamples: map[string]map[string]string{
			"server": {
				"allowed-ips": "allowed-ips:\n  - 127.0.0.1\n  - 192.168.0.0/24\n  - 10.0.0.0/8\n",
				"headers":     "headers:\n  X-Request-ID: \"\"\n  X-Forwarded-For: \"\"\n",
			},
			"database": {
				// #nosec G101 -- example DSN for the demo hint panel; not a real credential
				"dsn":    "dsn: \"postgres://user:pass@localhost:5432/mydb?sslmode=disable\"\n",
				"driver": "driver: postgres\n",
			},
			"deploy": {
				"enabled": "enabled: true\n",
			},
			"workers": {
				"extensions": "extensions: [\"go\", \"yaml\", \"json\"]\n",
				"tags":       "tags:\n  - critical\n  - high-priority\n",
			},
		},

		Validators: []editor.Validator{
			// MutuallyExclusive: server and proxy cannot coexist.
			// Add "proxy: true" manually to test.yaml to trigger this.
			editor.MutuallyExclusive("server", "proxy"),
			// RequiredWith: routes require server to be present.
			editor.RequiredWith("routes", "server"),
		},
	}); err != nil {
		panic(err)
	}
}
