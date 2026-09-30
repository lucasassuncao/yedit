// Command demo is the small editor docs/demo.gif is recorded from, and the one
// the README's Quick Demo shows. Run it from the yedit root:
//
//	go run ./cmd/demo                   # edits demo.yaml, seeded on first run
//	go run ./cmd/demo -config path.yaml
//	go run ./cmd/demo -theme grape
//
// # Schema
//
//	Config
//	  app-name (string, required) - primitive, no tree
//	  debug    (bool)             - primitive, no tree
//	  server   (ServerConfig)     - KindObject, nested drill-in target
//	    host, port
//	    pool (PoolConfig)         - nested one level deeper: min-size, max-size
//	  workers  ([]Worker)         - KindList with child defs, [N] navigator
//	    name (required), concurrency
//
// Metadata is declared via MetadataProvider (metadata.New) - each struct
// documents only its own fields; nested structs compose automatically.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/metadata"
	"github.com/lucasassuncao/yedit/presets"
)

// ── Schema ────────────────────────────────────────────────────────────────────

type PoolConfig struct {
	MinSize int `yaml:"min-size"`
	MaxSize int `yaml:"max-size"`
}

func (PoolConfig) Metadata() map[string]any {
	return map[string]any{
		"min-size": map[string]any{"description": "Minimum pool size.", "default": "2"},
		"max-size": map[string]any{"description": "Maximum pool size.", "default": "10"},
	}
}

type ServerConfig struct {
	Host string     `yaml:"host"`
	Port int        `yaml:"port"`
	Pool PoolConfig `yaml:"pool"`
}

func (ServerConfig) Metadata() map[string]any {
	return map[string]any{
		"host": map[string]any{"description": "Address to bind.", "default": "localhost"},
		"port": map[string]any{"description": "Port to listen on.", "default": "8080"},
		// no children needed - PoolConfig.Metadata() is composed automatically
	}
}

type Worker struct {
	Name        string `yaml:"name"`
	Concurrency int    `yaml:"concurrency"`
}

func (Worker) Metadata() map[string]any {
	return map[string]any{
		"name":        map[string]any{"description": "Worker name.", "required": true},
		"concurrency": map[string]any{"description": "Number of concurrent jobs.", "default": "1"},
	}
}

type Config struct {
	AppName string       `yaml:"app-name"`
	Debug   bool         `yaml:"debug"`
	Server  ServerConfig `yaml:"server"`
	Workers []Worker     `yaml:"workers"`
}

func (Config) Metadata() map[string]any {
	return map[string]any{
		"app-name": map[string]any{"description": "Application display name.", "required": true},
		"debug":    map[string]any{"description": "Enable debug logging.", "default": "false"},
		"server":   map[string]any{"description": "HTTP server configuration."},
		"workers":  map[string]any{"description": "Background worker pools."},
	}
}

var testMetadata = mustBuildMetadata()

func mustBuildMetadata() editor.MetadataSource {
	src, err := metadata.New(Config{})
	if err != nil {
		panic(fmt.Sprintf("testMetadata: %v", err))
	}
	return src
}

// ── Presets ───────────────────────────────────────────────────────────────────

var testPresets = presets.ForField("server", serverPresetsMap())

func serverPresetsMap() map[string]ServerConfig {
	return map[string]ServerConfig{
		"minimal":    {Host: "localhost", Port: 8080, Pool: PoolConfig{MinSize: 2, MaxSize: 5}},
		"production": {Host: "0.0.0.0", Port: 443, Pool: PoolConfig{MinSize: 10, MaxSize: 100}},
	}
}

// ── Theme ─────────────────────────────────────────────────────────────────────

func appTheme(name string) theme.Theme {
	if t, ok := theme.All()[name]; ok {
		return theme.Theme{Base: &t}
	}
	return theme.Theme{}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "YAML file to edit (default: demo.yaml, seeded on first run)")
	themeName := flag.String("theme", "plain", "theme preset (-theme grape, -theme sonic, …)")
	noSaveConfirm := flag.Bool("no-save-confirm", false, "skip save confirmation dialog")
	noDeleteConfirm := flag.Bool("no-delete-confirm", false, "skip delete confirmation dialog")
	noValidate := flag.Bool("no-validate", false, "allow saving with validation errors")
	flag.Parse()

	path := *configPath
	if path == "" {
		path = "demo.yaml"
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(path, []byte(seedYAML), 0o600); err != nil {
				return err
			}
		}
	}

	res, err := editor.Run(editor.Config{
		Theme:  appTheme(*themeName),
		Path:   path,
		Schema: &Config{},
		Title:  "yedit demo",

		NoSaveConfirm:    *noSaveConfirm,
		NoDeleteConfirm:  *noDeleteConfirm,
		NoValidateOnSave: *noValidate,

		EnableHints: true,

		BlockPresets: testPresets,
		Metadata:     testMetadata,

		Validators: []editor.Validator{
			editor.RequiredFromMetadata(),
		},
	})
	if err != nil {
		return err
	}
	if res.Saved {
		fmt.Println("changes saved to", path)
	}
	return nil
}
