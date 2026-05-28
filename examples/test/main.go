// Command test is a self-contained yedit example that exercises every schema
// pattern and known edge case.
//
// Run from the yedit root:
//
//	go run ./examples/test
//
// A test.yaml file is created in the working directory on first run and reused
// on subsequent runs so you can see undo/save/validate in action.
//
// # Patterns demonstrated
//
//	Pattern 1 — (no fields) + YAML pane
//	  KindScalar  : app-name, debug, version, port
//	  KindMap     : labels, settings
//	  KindSlice   : tags, ports          ([]string / []int, no child defs)
//	  KindUnion   : timeout              (implements schema.Provider)
//
//	Pattern 2 — ADDED/AVAILABLE struct tree
//	  server      : flat struct          (host, port, tls)
//	  database    : nested struct        (driver, dsn, pool.min-size, pool.max-size)
//	  logging     : simple scalars       (level, file, show-caller)
//
//	Pattern 3 — [N] sequence navigator
//	  workers     : []Worker             (name, concurrency, queue, tags []string)
//	  routes      : []Route              (path, method oneof, handler, auth)
//
// # Edge cases demonstrated
//
//	[]string inside KindStruct           → server.allowed-ips (leaf, no sub-tree)
//	map[string]string inside KindStruct  → server.headers     (leaf, no sub-tree)
//	[]string inside a seq-item struct    → workers[N].tags    (leaf, no sub-tree)
//	KindMap at top level                 → labels, settings
//	KindUnion (Provider)                 → timeout            → (no fields) + YAML pane
//	Deep nesting (3 levels)              → database.pool.*
//	oneof validation                     → database.driver, route.method
//	required + MutuallyExclusive         → validators wired via editor.Config
//	Unknown YAML key in seed file        → "unknown-key" flagged by ctrl+l
package main

import (
	"os"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/schema"
)

// ── Pattern 1: KindScalar / KindMap / KindSlice (no defs) / KindUnion ────────

// TimeoutValue is a union type: in YAML it can be a plain duration string
// ("30s") or a structured object. Implementing schema.Provider signals yedit
// to skip reflection and instead show whatever YeditSchema() returns.
// Because Kind is set to KindUnion in the editor, the overlay falls through to
// Pattern 1: the left panel shows "(no fields)" and the YAML pane gets focus.
type TimeoutValue struct{}

func (TimeoutValue) YeditSchema() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "connect", Kind: schema.KindScalar},
		{YAMLName: "read", Kind: schema.KindScalar},
		{YAMLName: "write", Kind: schema.KindScalar},
	}
}

// ── Pattern 2: KindStruct (ADDED/AVAILABLE tree) ──────────────────────────────

// ServerConfig exercises a flat struct.
// Edge cases:
//   - allowed-ips is []string → shows as a togglable leaf; content via YAML pane
//   - headers is map[string]string → same
type ServerConfig struct {
	Host       string            `yaml:"host"`
	Port       int               `yaml:"port"`
	TLS        bool              `yaml:"tls"`
	AllowedIPs []string          `yaml:"allowed-ips"` // edge: []string inside struct
	Headers    map[string]string `yaml:"headers"`     // edge: map inside struct
}

// PoolConfig is a nested struct (depth 2 inside Database → depth 3 total).
// Edge case: 3-level nesting is the deepest the schema discoverer traverses
// (hardcoded at depth ≤ 3 in schema/discover.go).
type PoolConfig struct {
	MinSize int `yaml:"min-size" jsonschema:"default=2"`
	MaxSize int `yaml:"max-size" jsonschema:"default=10"`
}

// DatabaseConfig exercises nested struct + oneof + required.
type DatabaseConfig struct {
	Driver   string     `yaml:"driver"    validate:"required,oneof=postgres mysql sqlite"`
	DSN      string     `yaml:"dsn"       validate:"required"`
	MaxConns int        `yaml:"max-conns" jsonschema:"default=10"`
	Pool     PoolConfig `yaml:"pool"` // edge: nested struct
}

// LoggingConfig is a simple flat struct with only scalar leaves.
type LoggingConfig struct {
	Level      string `yaml:"level"       validate:"oneof=debug info warn error"`
	File       string `yaml:"file"`
	ShowCaller bool   `yaml:"show-caller"`
}

// ── Pattern 3: KindSlice with child defs ([N] sequence navigator) ─────────────

// Worker exercises a seq-item struct with a nested []string leaf.
type Worker struct {
	Name        string   `yaml:"name"        validate:"required"`
	Concurrency int      `yaml:"concurrency" jsonschema:"default=1"`
	Queue       string   `yaml:"queue"`
	Tags        []string `yaml:"tags"` // edge: []string inside seq item
}

// Route exercises oneof validation on a seq-item field.
type Route struct {
	Path    string `yaml:"path"    validate:"required"`
	Method  string `yaml:"method"  validate:"required,oneof=GET POST PUT DELETE PATCH"`
	Handler string `yaml:"handler" validate:"required"`
	Auth    bool   `yaml:"auth"`
}

// ── Root config — all patterns in one struct ──────────────────────────────────

type TestConfig struct {
	// Pattern 1 — KindScalar
	AppName string `yaml:"app-name"`
	Debug   bool   `yaml:"debug"`
	Version string `yaml:"version" validate:"required" jsonschema:"default=0.1.0"`
	Port    int    `yaml:"port"    jsonschema:"default=8080"`

	// Pattern 1 — KindMap (free-form; left panel shows "(no fields)")
	Labels   map[string]string `yaml:"labels"`
	Settings map[string]any    `yaml:"settings"`

	// Pattern 1 — KindSlice, no child defs (left panel shows "(no fields)")
	Tags  []string `yaml:"tags"`
	Ports []int    `yaml:"ports"`

	// Pattern 1 — KindUnion via Provider (left panel shows "(no fields)")
	Timeout TimeoutValue `yaml:"timeout"`

	// Pattern 2 — KindStruct (ADDED/AVAILABLE tree)
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logging  LoggingConfig  `yaml:"logging"`

	// Pattern 3 — KindSlice + child defs ([N] navigator + add new)
	Workers []Worker `yaml:"workers"`
	Routes  []Route  `yaml:"routes"`
}

// ── Seed YAML ─────────────────────────────────────────────────────────────────

// seedYAML is written to test.yaml on first run so the editor opens with
// representative content. It intentionally includes an unknown key
// ("unknown-key") to demonstrate ctrl+l validation feedback.
const seedYAML = `app-name: "my-app"
debug: false
version: "0.1.0"
port: 8080
tags:
  - go
  - tui
server:
  host: localhost
  port: 8080
  tls: false
  allowed-ips:
    - 127.0.0.1
    - 10.0.0.0/8
logging:
  level: info
  show-caller: false
workers:
  - name: "default"
    concurrency: 2
    queue: "main"
    tags:
      - critical
  - name: "background"
    concurrency: 1
    queue: "low"
unknown-key: "flagged by ctrl+l validate"
`

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	const path = "test.yaml"

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(seedYAML), 0600); err != nil {
			panic(err)
		}
	}

	if err := editor.Run(editor.Config{
		Path:   path,
		Schema: &TestConfig{},
		Title:  "yedit test",

		Validators: []editor.Validator{
			// Demonstrates MutuallyExclusive: can't set both server and a
			// hypothetical "proxy" at the same time (add proxy manually to YAML
			// to trigger this).
			editor.MutuallyExclusive("server", "proxy"),
			// Demonstrates RequiredWith: routes require server to be present.
			editor.RequiredWith("routes", "server"),
		},

		// FieldSnippets: default YAML inserted when a struct sub-field is
		// toggled ON for the first time (Pattern 2 tree).
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
			},
			"logging": {
				"level":       "  level: info\n",
				"file":        "  file: \"/var/log/app.log\"\n",
				"show-caller": "  show-caller: false\n",
			},
		},
	}); err != nil {
		panic(err)
	}
}
