// Command test is a small, self-contained yedit example used both for manual
// testing and for recording the demo GIFs under examples/.
//
// Run from the yedit root:
//
//	go run ./examples/test                 # open the editor (seeds demo.yaml on first run)
//	go run ./examples/test --config path.yaml
//	go run ./examples/test --theme dracula
//	go run ./examples/test show-docs       # browse schema docs in the TUI
//	go run ./examples/test generate-docs   # write docs/ markdown files
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
	"fmt"
	"io/fs"
	"os"

	"github.com/spf13/cobra"

	"github.com/lucasassuncao/yedit/docgenerator"
	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/metadata"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/theme"
)

// ── Schema ────────────────────────────────────────────────────────────────────

type PoolConfig struct {
	MinSize int `yaml:"min-size"`
	MaxSize int `yaml:"max-size"`
}

func (PoolConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"min-size": {FieldMeta: editor.FieldMeta{Description: "Minimum pool size.", Default: "2"}},
		"max-size": {FieldMeta: editor.FieldMeta{Description: "Maximum pool size.", Default: "10"}},
	}
}

type ServerConfig struct {
	Host string     `yaml:"host"`
	Port int        `yaml:"port"`
	Pool PoolConfig `yaml:"pool"`
}

func (ServerConfig) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"host": {FieldMeta: editor.FieldMeta{Description: "Address to bind.", Default: "localhost"}},
		"port": {FieldMeta: editor.FieldMeta{Description: "Port to listen on.", Default: "8080"}},
		// no Children needed - PoolConfig.Metadata() is composed automatically
	}
}

type Worker struct {
	Name        string `yaml:"name"`
	Concurrency int    `yaml:"concurrency"`
}

func (Worker) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"name":        {FieldMeta: editor.FieldMeta{Description: "Worker name.", Required: true}},
		"concurrency": {FieldMeta: editor.FieldMeta{Description: "Number of concurrent jobs.", Default: "1"}},
	}
}

type Config struct {
	AppName string       `yaml:"app-name"`
	Debug   bool         `yaml:"debug"`
	Server  ServerConfig `yaml:"server"`
	Workers []Worker     `yaml:"workers"`
}

func (Config) Metadata() map[string]*metadata.Node {
	return map[string]*metadata.Node{
		"app-name": {FieldMeta: editor.FieldMeta{Description: "Application display name.", Required: true}},
		"debug":    {FieldMeta: editor.FieldMeta{Description: "Enable debug logging.", Default: "false"}},
		"server":   {FieldMeta: editor.FieldMeta{Description: "HTTP server configuration."}},
		"workers":  {FieldMeta: editor.FieldMeta{Description: "Background worker pools."}},
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
	all := theme.All()
	if t, ok := all[name]; ok {
		return theme.Theme{Base: &t}
	}
	return theme.Theme{} // default dark
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
	var themeName, configPath string
	var noSaveConfirm, noDeleteConfirm, noValidate bool

	cmd := &cobra.Command{
		Use:   "test",
		Short: "yedit test - a small example editor used for demos",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := configPath
			if path == "" {
				path = "demo.yaml"
				if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
					if err := os.WriteFile(path, []byte(seedYAML), 0600); err != nil {
						return err
					}
				}
			}

			res, err := editor.Run(editor.Config{
				Theme:  appTheme(themeName),
				Path:   path,
				Schema: &Config{},
				Title:  "yedit test",

				NoSaveConfirm:    noSaveConfirm,
				NoDeleteConfirm:  noDeleteConfirm,
				NoValidateOnSave: noValidate,

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
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "YAML file to edit (default: demo.yaml, seeded on first run)")
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
				{Config: Config{}},
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
				{Config: Config{}, DocsDir: "docs"},
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
