// Command test is a self-contained yedit example that exercises every schema
// pattern, Config option, and known edge case.
//
// Run from the yedit root:
//
//	go run ./examples/test               # open the editor
//	go run ./examples/test --theme dracula
//	go run ./examples/test show-docs     # browse schema docs in the TUI
//	go run ./examples/test generate-docs # write docs/ markdown files
//
// A test.yaml is created on first run and reused on subsequent runs so undo,
// save, and validate can be exercised across restarts.
//
// # Patterns demonstrated
//
//	Pattern 1 - Primitives (no tree, YAML pane only)
//	  KindPrimitive : app-name (string), debug (bool), version (string required),
//	                  port (int default=8080), ratio (float64), build-timeout (duration)
//	  KindDictionary: labels (map[string]string), settings (map[string]any)
//	  KindList      : tags ([]string), ports ([]int)           - no child defs
//	  KindVariant   : timeout                                   - Provider interface
//
//	Pattern 2 - KindObject (ADDED/AVAILABLE tree with nested structs)
//	  server    : flat struct with []string and map[string]string leaves
//	  database  : struct with nested Pool (3-level nesting)
//	  logging   : struct with enum-like level (OneOf via hints) and bool
//	  deploy    : struct with *bool pointer and enum-like strategy
//
//	Pattern 3 - KindList with child defs ([N] sequence navigator)
//	  workers   : []Worker  (name required, concurrency, queue, tags []string)
//	  routes    : []Route   (path, method oneof, handler, auth bool)
//	  filters   : []Filter  (self-referential, cycle-detected: any []Filter, all []Filter)
//
//	Pattern 4 - KindDictionary with child defs (map[key]struct navigator)
//	  port-attrs: map[string]PortAttr (label string, on-auto-forward oneof)
//
//	Pattern 5 - Schema edge cases (items 6–10 from robustness audit)
//	  embed     : embeddedMeta anonymous embed → created-by, version-tag promoted to root
//	  inline    : inlineAnnotations yaml:",inline" → team, contact promoted to root
//	  omitempty : replicas (int,omitempty) - FieldDef.OmitEmpty = true
//	  flow      : ips ([]string,flow) - FieldDef.Flow = true
//	  int key   : firewall-rules map[int]PortRule - FieldDef.MapKeyScalar = "int"
//	  marshaler : background (Color via MarshalYAML) - KindPrimitive, no R/G/B sub-fields
//	  any       : extras (interface{}) - KindAny
//
//	Config options exercised
//	  PassthroughKeys  : "import" is preserved as-is, hidden from all sections
//	  Hidden           : "internal-id" is never shown in the UI
//	  PreCheckedFields : opening a new "server" block pre-checks host and port
//	  FieldSnippets    : toggling a struct field inserts a real default value
//	  Presets          : struct-backed presets for "server" and "logging" (presets.ForField + Combine)
//	  Metadata         : each struct implements Metadata() via metadata.MetadataProvider
//	  Validators       : MutuallyExclusive(server, proxy), RequiredWith(routes, server)
//	  NoDeleteConfirm  : controlled by --no-delete-confirm flag
//	  NoSaveConfirm    : controlled by --no-save-confirm flag
//
//	Edge cases in seed YAML
//	  unknown-key      : present in file, not in schema → UNKNOWN section + ctrl+l
//	  import           : passthrough key, silently preserved
//	  extensions field : flow-style list ["go","yaml"] inside a worker entry
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/spf13/cobra"

	"github.com/lucasassuncao/yedit/docgenerator"
	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

// ── Pattern 1: KindVariant via Provider ──────────────────────────────────────

// TimeoutValue is a union type handled via schema.Provider.
// yedit skips reflection and uses YeditSchema() directly.
type TimeoutValue struct{}

func (TimeoutValue) YeditSchema() []schema.FieldDef {
	return []schema.FieldDef{
		{YAMLName: "connect", Kind: schema.KindPrimitive},
		{YAMLName: "read", Kind: schema.KindPrimitive},
		{YAMLName: "write", Kind: schema.KindPrimitive},
	}
}

// ── Pattern 2: KindObject (ADDED/AVAILABLE tree) ──────────────────────────────

// ServerConfig: flat struct with slice and map leaves.
type ServerConfig struct {
	Host       string            `yaml:"host"`
	Port       int               `yaml:"port"`
	TLS        bool              `yaml:"tls"`
	AllowedIPs []string          `yaml:"allowed-ips"`
	Headers    map[string]string `yaml:"headers"`
}

// PoolConfig: nested struct (depth 3 from root).
type PoolConfig struct {
	MinSize int `yaml:"min-size"`
	MaxSize int `yaml:"max-size"`
	Timeout int `yaml:"timeout"`
}

// DatabaseConfig: nested struct + oneof + required.
type DatabaseConfig struct {
	Driver   string     `yaml:"driver"`
	DSN      string     `yaml:"dsn"`
	MaxConns int        `yaml:"max-conns"`
	Pool     PoolConfig `yaml:"pool"`
}

// LoggingConfig: flat struct with bool and enum.
type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	ShowCaller bool   `yaml:"show-caller"`
}

// DeployConfig: struct with *bool pointer and enum - from movelooper patterns.
type DeployConfig struct {
	Enabled    *bool  `yaml:"enabled"`
	Strategy   string `yaml:"strategy"`
	Replicas   int    `yaml:"replicas"`
	AutoRevert bool   `yaml:"auto-revert"`
}

// ── Pattern 6: new FieldMeta capabilities ────────────────────────────────────

// NetworkConfig exercises Formats, MinLength/MaxLength, NotOneOf.
type NetworkConfig struct {
	Endpoint  string `yaml:"endpoint"`   // FormatURL
	Host      string `yaml:"host"`       // FormatHost | FormatIPv4 (OR semantics)
	CIDR      string `yaml:"cidr"`       // FormatCIDR
	UUID      string `yaml:"uuid"`       // FormatUUID
	Tag       string `yaml:"tag"`        // FormatSemver + MinLength=5
	Protocol  string `yaml:"protocol"`   // NotOneOf=["ftp","telnet"]
	NoteName  string `yaml:"note-name"`  // MinLength=3, MaxLength=64
	AnyIP     string `yaml:"any-ip"`     // FormatIP
	Listen    string `yaml:"listen"`     // FormatHostPort
	HTTPPort  string `yaml:"http-port"`  // FormatPort
	Timeout   string `yaml:"timeout"`    // FormatDuration
	Expiry    string `yaml:"expiry"`     // FormatDate
}

// SecurityConfig exercises remaining built-in formats.
type SecurityConfig struct {
	IPv6Addr   string `yaml:"ipv6-addr"`   // FormatIPv6
	PublicKey  string `yaml:"public-key"`  // FormatPublicKey
	PrivateKey string `yaml:"private-key"` // FormatPrivateKey
	FQDN       string `yaml:"fqdn"`        // FormatFQDN
	Email      string `yaml:"email"`       // FormatEmail
}

// DeployExtConfig exercises Multiline, Snippet, PreChecked, FormatTerraformSource, FormatGitRef, FormatDirectoryPath.
type DeployExtConfig struct {
	Source    string `yaml:"source"`     // FormatTerraformSource, Snippet, PreChecked
	Script    string `yaml:"script"`     // Multiline:true, no Example (auto-gen)
	Readme    string `yaml:"readme"`     // Multiline:true, explicit Example
	GitRef    string `yaml:"git-ref"`    // FormatGitRef
	DirPath   string `yaml:"dir-path"`   // FormatDirectoryPath
}

// ── Pattern 3: KindList with child defs ──────────────────────────────────────

// Worker: seq-item struct with []string leaf.
type Worker struct {
	Name        string   `yaml:"name"`
	Concurrency int      `yaml:"concurrency"`
	Queue       string   `yaml:"queue"`
	Extensions  []string `yaml:"extensions"` // edge: flow-style ["go","yaml"] in seed
	Tags        []string `yaml:"tags"`
}

// Route: seq-item with oneof.
type Route struct {
	Path    string `yaml:"path"`
	Method  string `yaml:"method"`
	Handler string `yaml:"handler"`
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
	OnAutoForward string `yaml:"on-auto-forward"`
	Protocol      string `yaml:"protocol"`
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

// Color implements yaml.Marshaler - Discover classifies it as KindPrimitive (item 9).
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

// PortRule: value type for map[int]PortRule - demonstrates non-string map key (item 8).
type PortRule struct {
	Proto   string `yaml:"proto"`
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
	// Hidden by Config.Hidden - never appears in the UI.
	InternalID string `yaml:"internal-id"`

	// Pattern 1 - KindPrimitive
	AppName      string        `yaml:"app-name"`
	Debug        bool          `yaml:"debug"`
	Version      string        `yaml:"version"`
	Port         int           `yaml:"port"`
	Ratio        float64       `yaml:"ratio"`
	BuildTimeout time.Duration `yaml:"build-timeout"`

	// Pattern 1 - KindDictionary (free-form, no child defs)
	Labels   map[string]string `yaml:"labels"`
	Settings map[string]any    `yaml:"settings"`

	// Pattern 1 - KindList, no child defs
	Tags  []string `yaml:"tags"`
	Ports []int    `yaml:"ports"`

	// Pattern 1 - KindVariant via Provider
	Timeout TimeoutValue `yaml:"timeout"`

	// Pattern 2 - KindObject structs
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logging  LoggingConfig  `yaml:"logging"`
	Deploy   DeployConfig   `yaml:"deploy"`

	// Pattern 3 - KindList with child defs
	Workers []Worker `yaml:"workers"`
	Routes  []Route  `yaml:"routes"`
	Filters []Filter `yaml:"filters"`

	// Pattern 4 - KindDictionary with child defs
	PortAttrs map[string]PortAttr `yaml:"port-attrs"`

	// Pattern 5 - Schema edge cases
	EdgeCases SchemaEdgeCases `yaml:"edge-cases"`

	// Pattern 6 - new FieldMeta capabilities
	Network    NetworkConfig    `yaml:"network"`
	Security   SecurityConfig   `yaml:"security"`
	DeployExt  DeployExtConfig  `yaml:"deploy-ext"`
}

// ── Theme ─────────────────────────────────────────────────────────────────────

func appTheme(name string) theme.Theme {
	all := theme.All()
	if t, ok := all[name]; ok {
		return theme.Theme{Base: &t}
	}
	return theme.Theme{} // default dark
}

// ── Presets ───────────────────────────────────────────────────────────────────

var testPresets = presets.Combine(
	presets.ForField("server", serverPresetsMap()),
	presets.ForField("logging", loggingPresetsMap()),
)

func serverPresetsMap() map[string]ServerConfig {
	return map[string]ServerConfig{
		"minimal":    {Host: "localhost", Port: 8080},
		"production": {Host: "0.0.0.0", Port: 443, TLS: true, AllowedIPs: []string{"10.0.0.0/8", "172.16.0.0/12"}},
	}
}

func loggingPresetsMap() map[string]LoggingConfig {
	return map[string]LoggingConfig{
		"development": {Level: "debug", ShowCaller: true},
		"production":  {Level: "warn", File: "/var/log/app.log"},
	}
}

// ── Commands ──────────────────────────────────────────────────────────────────

func main() {
	root := buildEditCmd()
	root.AddCommand(buildShowDocsCmd())
	root.AddCommand(buildGenerateDocsCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildEditCmd() *cobra.Command {
	var themeName string
	var noSaveConfirm, noDeleteConfirm, noValidate bool

	cmd := &cobra.Command{
		Use:   "test",
		Short: "yedit test - exercises every schema pattern and Config option",
		RunE: func(cmd *cobra.Command, args []string) error {
			const path = "test.yaml"
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.WriteFile(path, []byte(seedYAML), 0600); err != nil {
					return err
				}
			}

			res, err := editor.Run(editor.Config{
				Theme:  appTheme(themeName),
				Path:   path,
				Schema: &TestConfig{},
				Title:  "yedit test",

				Hidden:          []string{"internal-id"},
				PassthroughKeys: []string{"import"},

				NoSaveConfirm:    noSaveConfirm,
				NoDeleteConfirm:  noDeleteConfirm,
				NoValidateOnSave: noValidate,

				Presets:  testPresets,
				Metadata: testMetadata,

				Validators: []editor.Validator{
					editor.MutuallyExclusive("server", "proxy"),
					editor.RequiredWith("routes", "server"),
					editor.FormatFromMetadata(),
					editor.LengthFromMetadata(),
					editor.NotOneOfFromMetadata(),
				},
			})
			if err != nil {
				return err
			}
			if res.Saved {
				fmt.Println("changes saved to", path)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&themeName, "theme", "dark", "theme preset (--theme dracula, --theme light, …)")
	cmd.Flags().BoolVar(&noSaveConfirm, "no-save-confirm", false, "skip save confirmation dialog")
	cmd.Flags().BoolVar(&noDeleteConfirm, "no-delete-confirm", false, "skip delete confirmation dialog")
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "allow saving with validation errors")
	return cmd
}

func buildShowDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-docs",
		Short: "Browse schema documentation in the TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			gen := docgenerator.NewSchemaGenerator(docgenerator.WithMetadata(testMetadata))
			ds := gen.GenerateDocsInMemory([]docgenerator.Entry{
				{Config: TestConfig{}},
			})
			return docgenerator.RenderMarkdownDocsInTerminal(ds, "yedit test")
		},
	}
}

func buildGenerateDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "generate-docs",
		Short:  "Write markdown documentation to docs/",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			gen := docgenerator.NewSchemaGenerator(docgenerator.WithMetadata(testMetadata))
			files, err := gen.GenerateDocsForEach([]docgenerator.Entry{
				{Config: TestConfig{}, DocsDir: "docs"},
			})
			if err != nil {
				return err
			}
			if err := docgenerator.GenerateIndex("docs", files); err != nil {
				return err
			}
			fmt.Println("documentation written to docs/")
			return nil
		},
	}
}
