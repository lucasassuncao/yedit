// Package docgenerator generates reference artifacts from a struct-based schema
// (via schema.Discover) and a MetadataSource: markdown reference pages, a JSON
// Schema, preset example pages, and a documentation index.
//
// Every output is opt-in. Generate writes exactly what the With* options select
// and nothing else; calling it with no output option is an error rather than a
// silent no-op.
//
// The package depends on schema, spec, metadata and presets. It does not depend
// on editor, so wiring a doc-generation command costs nothing in TUI
// dependencies.
package docgenerator

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lucasassuncao/yedit/metadata"
	"github.com/lucasassuncao/yedit/presets"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/spec"
)

// ErrNoOutput is returned by Generate when no output option was supplied.
var ErrNoOutput = errors.New("docgenerator: no output configured")

// Entry pairs a struct with the options controlling how it is documented.
//
// SplitStructs gives every field with children its own markdown file instead of
// inlining it; scalar fields are never split. It has no effect on other outputs.
//
// MarkdownDir overrides where this entry's markdown pages are written, for
// layouts that group each documented struct in its own subdirectory. Empty uses
// the directory given to WithMarkdown. It redirects markdown only: it does not
// enable it, so an entry with MarkdownDir set but no WithMarkdown option writes
// nothing. JSON Schema output is unaffected - schemas stay together under
// WithJSONSchema, which is the directory a language server is pointed at.
//
// RecursionLimit is how many extra levels a self-referential type expands beyond
// the first visit. nil uses the schema.Discover default (1); 0 stops expansion
// on the second visit. The JSON Schema output is unaffected: it represents
// recursion with $defs/$ref rather than by expanding.
type Entry struct {
	Config         any
	MarkdownDir    string
	SplitStructs   bool
	RecursionLimit *int
}

// GeneratedFile records one file written by Generate. Path is the file as
// written, so consumers never have to re-derive the naming rule.
type GeneratedFile struct {
	Name string // display title, e.g. "Config"
	Path string // path as written, e.g. "docs/reference/config.md"
}

// config holds a generation run's settings. Each output is enabled by its
// directory being non-empty.
type config struct {
	metadata spec.MetadataSource

	markdownDir   string
	jsonSchemaDir string
	indexDir      string

	examplesSrc   presets.Source
	examplesDir   string
	exampleTitles map[string]string
}

// Option configures a generation run. Passing the same option twice keeps the
// last value.
type Option func(*config)

// WithMetadata uses src as the metadata source for every entry. Without it, each
// entry whose Config implements metadata.Provider gets its own source via
// metadata.New; entries that do not implement it are documented from structure
// alone.
func WithMetadata(src spec.MetadataSource) Option {
	return func(c *config) { c.metadata = src }
}

// WithMarkdown writes markdown reference pages to dir, one per entry plus one
// per split child.
func WithMarkdown(dir string) Option {
	return func(c *config) { c.markdownDir = dir }
}

// WithJSONSchema writes a JSON Schema (draft 2020-12) per entry to dir, named
// "<lowercased type name>.schema.json".
func WithJSONSchema(dir string) Option {
	return func(c *config) { c.jsonSchemaDir = dir }
}

// WithExamples writes one markdown page per preset field to dir, and links to
// them from the matching markdown reference pages.
//
// titles maps a presets.Source field name to its display title (e.g.
// "authorizationpolicies" -> "AuthorizationPolicy"). The page is named after the
// lowercased title so it matches the reference page for the same type, and that
// same lowercased title is what decides which reference pages get an "Examples"
// link. Fields absent from titles, or with no presets, are skipped.
func WithExamples(src presets.Source, dir string, titles map[string]string) Option {
	return func(c *config) {
		c.examplesSrc = src
		c.examplesDir = dir
		c.exampleTitles = titles
	}
}

// WithIndex writes a README.md to dir linking to the generated markdown and
// example pages. Generated JSON Schema files are not listed: the index is a
// readable documentation index, not a manifest of build artifacts.
func WithIndex(dir string) Option {
	return func(c *config) { c.indexDir = dir }
}

// entryCtx is one entry resolved once: discovered structure plus the metadata
// source that applies to it. Emitters read it; none of them re-discovers.
type entryCtx struct {
	entry  Entry
	name   string
	fields []schema.FieldDef
	meta   spec.MetadataSource // nil means "no metadata declared"
}

// Generate writes every output selected by opts and returns the files written,
// ordered markdown, JSON Schema, examples, index.
func Generate(entries []Entry, opts ...Option) ([]GeneratedFile, error) {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.markdownDir == "" && cfg.jsonSchemaDir == "" && cfg.examplesDir == "" && cfg.indexDir == "" {
		return nil, ErrNoOutput
	}

	ctxs, err := resolveEntries(cfg, entries)
	if err != nil {
		return nil, err
	}

	var mdFiles, schemaFiles, exampleFiles []GeneratedFile
	if cfg.markdownDir != "" {
		if mdFiles, err = emitMarkdown(cfg, ctxs); err != nil {
			return nil, err
		}
	}
	if cfg.jsonSchemaDir != "" {
		if schemaFiles, err = emitJSONSchema(cfg, ctxs); err != nil {
			return nil, err
		}
	}
	if cfg.examplesDir != "" {
		if exampleFiles, err = emitExamples(cfg); err != nil {
			return nil, err
		}
	}

	files := make([]GeneratedFile, 0, len(mdFiles)+len(schemaFiles)+len(exampleFiles)+1)
	files = append(files, mdFiles...)
	files = append(files, schemaFiles...)
	files = append(files, exampleFiles...)

	if cfg.indexDir != "" {
		indexFile, err := emitIndex(cfg.indexDir, mdFiles, exampleFiles)
		if err != nil {
			return nil, err
		}
		files = append(files, indexFile)
	}
	return files, nil
}

// resolveEntries discovers each entry's structure and settles its metadata
// source: the WithMetadata override when set, otherwise a source composed from
// the struct itself when it implements metadata.Provider, otherwise none.
func resolveEntries(cfg *config, entries []Entry) ([]entryCtx, error) {
	ctxs := make([]entryCtx, 0, len(entries))
	for _, e := range entries {
		meta := cfg.metadata
		if meta == nil {
			if _, ok := e.Config.(metadata.Provider); ok {
				src, err := metadata.New(e.Config)
				if err != nil {
					return nil, fmt.Errorf("build metadata for %T: %w", e.Config, err)
				}
				meta = src
			}
		}
		ctxs = append(ctxs, entryCtx{
			entry:  e,
			name:   typeName(e.Config),
			fields: discoverEntry(e),
			meta:   meta,
		})
	}
	return ctxs, nil
}

// markdownDirFor returns where an entry's markdown pages go: its own override
// when set, otherwise the directory given to WithMarkdown.
func (c *config) markdownDirFor(e Entry) string {
	if e.MarkdownDir != "" {
		return e.MarkdownDir
	}
	return c.markdownDir
}

// exampleLinks derives the markdown cross-link inputs from WithExamples: the
// path from fromDir to the examples directory, and the set of page names that
// have an example page. Both come from the single titles map, so the two can
// never disagree.
//
// It takes fromDir rather than reading c.markdownDir because entries may write
// to different directories, and each one's link back to the examples has to be
// relative to where its own pages landed.
func (c *config) exampleLinks(fromDir string) (relDir string, pages map[string]bool) {
	if fromDir == "" || c.examplesDir == "" || len(c.exampleTitles) == 0 {
		return "", nil
	}
	rel, err := filepath.Rel(fromDir, c.examplesDir)
	if err != nil {
		return "", nil
	}
	pages = make(map[string]bool, len(c.exampleTitles))
	for _, title := range c.exampleTitles {
		pages[strings.ToLower(title)] = true
	}
	return filepath.ToSlash(rel), pages
}
