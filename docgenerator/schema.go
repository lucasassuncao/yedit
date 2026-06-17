// Package docgenerator generates markdown documentation from a struct-based
// schema (via schema.Discover) and a MetadataSource, and provides a TUI viewer
// for browsing the generated docs.
package docgenerator

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/metadata"
	"github.com/lucasassuncao/yedit/schema"
)

// Option configures a SchemaGenerator.
type Option func(*SchemaGenerator)

// WithMetadata configures the generator to use src for field descriptions,
// required flags, and defaults.
func WithMetadata(src editor.MetadataSource) Option {
	return func(g *SchemaGenerator) { g.metadata = src }
}

// SchemaGenerator generates markdown documentation from a Go struct using
// schema.Discover for structure and a MetadataSource for field descriptions.
type SchemaGenerator struct {
	metadata editor.MetadataSource
}

// NewSchemaGenerator creates a SchemaGenerator. All configuration is passed
// via options.
func NewSchemaGenerator(opts ...Option) *SchemaGenerator {
	g := &SchemaGenerator{}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// DocSet holds the generated pages and their parent-child relationships.
// Children maps a parent page name to its child page names in schema order.
type DocSet struct {
	Pages    map[string]string   // name → raw markdown
	Children map[string][]string // parent → children in schema order (split-struct entries only)
}

// GenerateDocsInMemory generates markdown for each entry.
// DocsDir is ignored; only Config and SplitStructs are used.
// For SplitStructs entries the parent-child relationship is recorded in DocSet.Children.
func (g *SchemaGenerator) GenerateDocsInMemory(entries []Entry) DocSet {
	ds := DocSet{Pages: map[string]string{}, Children: map[string][]string{}}
	for _, e := range entries {
		fields := discoverEntry(e)
		name := typeName(e.Config)
		if !e.SplitStructs {
			ds.Pages[name] = g.generateMarkdown(name, fields, nil)
			continue
		}
		ds.Pages[name] = g.generateRootMarkdown(name, fields)
		for _, f := range fields {
			if f.YAMLName == "" || f.YAMLName == "-" || len(f.Children) == 0 {
				continue
			}
			ds.Pages[f.YAMLName] = g.generateMarkdown(f.YAMLName, f.Children, []string{f.YAMLName})
			ds.Children[name] = append(ds.Children[name], f.YAMLName)
		}
	}
	return ds
}

// Entry pairs a struct with the directory where its documentation should be written.
// When SplitStructs is true, each field with children gets its own file instead of
// being inlined. Scalar fields are not split. Default is false.
//
// RecursionLimit controls how many extra levels a self-referential type expands
// beyond the first visit during schema discovery. nil uses the schema.Discover
// default (1). Set to 0 to stop expansion on the second visit, which prevents
// recursive structs from generating repeated sub-sections in the output.
type Entry struct {
	Config         any
	DocsDir        string
	SplitStructs   bool
	RecursionLimit *int
}

func discoverEntry(e Entry) []schema.FieldDef {
	if e.RecursionLimit != nil {
		return schema.Discover(e.Config, *e.RecursionLimit)
	}
	return schema.Discover(e.Config)
}

// GeneratedFile records the name and directory of a file produced by GenerateDocsForEach.
type GeneratedFile struct {
	Name    string
	DocsDir string
}

// GenerateDocsForEach generates markdown documentation for each entry.
// When Entry.SplitStructs is false (default), one file is written per entry containing
// all fields and nested sub-sections inline. When true, each field with children gets
// its own file; scalar fields are not split. Metadata is resolved using the field's
// yaml name as the block key.
func (g *SchemaGenerator) GenerateDocsForEach(entries []Entry) ([]GeneratedFile, error) {
	var files []GeneratedFile
	for _, e := range entries {
		if err := os.MkdirAll(e.DocsDir, 0750); err != nil {
			return nil, fmt.Errorf("create docs dir: %w", err)
		}
		fields := discoverEntry(e)
		name := typeName(e.Config)
		if !e.SplitStructs {
			md := g.generateMarkdown(name, fields, nil)
			if err := g.writeRaw(e.DocsDir, name, md); err != nil {
				return nil, err
			}
			files = append(files, GeneratedFile{Name: name, DocsDir: e.DocsDir})
			continue
		}
		md := g.generateRootMarkdown(name, fields)
		if err := g.writeRaw(e.DocsDir, name, md); err != nil {
			return nil, err
		}
		files = append(files, GeneratedFile{Name: name, DocsDir: e.DocsDir})
		generated, err := g.generateSplitChildren(e.DocsDir, fields)
		if err != nil {
			return nil, err
		}
		files = append(files, generated...)
	}
	return files, nil
}

func (g *SchemaGenerator) generateSplitChildren(docsDir string, fields []schema.FieldDef) ([]GeneratedFile, error) {
	var files []GeneratedFile
	for _, f := range fields {
		if f.YAMLName == "" || f.YAMLName == "-" || len(f.Children) == 0 {
			continue
		}
		childMD := g.generateMarkdown(f.YAMLName, f.Children, []string{f.YAMLName})
		if err := g.writeRaw(docsDir, f.YAMLName, childMD); err != nil {
			return nil, err
		}
		files = append(files, GeneratedFile{Name: f.YAMLName, DocsDir: docsDir})
	}
	return files, nil
}

// GenerateInMemory builds the MetadataSource for each entry and generates a DocSet
// in memory. Each Entry.Config must implement metadata.MetadataProvider.
// DocsDir is ignored.
func GenerateInMemory(entries []Entry) (DocSet, error) {
	ds := DocSet{Pages: map[string]string{}, Children: map[string][]string{}}
	for _, e := range entries {
		src, err := metadata.New(e.Config)
		if err != nil {
			return DocSet{}, fmt.Errorf("build metadata for %T: %w", e.Config, err)
		}
		partial := NewSchemaGenerator(WithMetadata(src)).GenerateDocsInMemory([]Entry{e})
		for k, v := range partial.Pages {
			ds.Pages[k] = v
		}
		for k, v := range partial.Children {
			ds.Children[k] = v
		}
	}
	return ds, nil
}

// Generate builds the MetadataSource for each entry, generates documentation,
// and writes an index.md to indexPath. Each Entry.Config must implement
// metadata.MetadataProvider.
func Generate(indexPath string, entries []Entry) error {
	var allFiles []GeneratedFile
	for _, e := range entries {
		src, err := metadata.New(e.Config)
		if err != nil {
			return fmt.Errorf("build metadata for %T: %w", e.Config, err)
		}
		files, err := NewSchemaGenerator(WithMetadata(src)).GenerateDocsForEach([]Entry{e})
		if err != nil {
			return err
		}
		allFiles = append(allFiles, files...)
	}
	return GenerateIndex(indexPath, allFiles)
}

// GenerateIndex writes an index.md to baseDir linking to all generated files.
// Links are computed as paths relative to baseDir so the index works correctly
// when entries were written to different subdirectories.
func GenerateIndex(baseDir string, files []GeneratedFile) error {
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		return fmt.Errorf("create index dir: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("# Documentation Index\n\n")
	sb.WriteString("This documentation describes all available configuration structures.\n\n")
	sb.WriteString("## Available Configurations\n\n")
	for _, f := range files {
		abs := filepath.Join(f.DocsDir, strings.ToLower(f.Name)+".md")
		rel, err := filepath.Rel(baseDir, abs)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", f.Name, err)
		}
		sb.WriteString(fmt.Sprintf("- [%s](./%s)\n", f.Name, filepath.ToSlash(rel)))
	}
	return os.WriteFile(filepath.Join(baseDir, "README.md"), []byte(sb.String()), 0600)
}

// fieldMeta translates the (sectionPath, fieldName) docgenerator coordinates to
// the (blockKey, fieldPath) MetadataSource coordinates:
//
//   - sectionPath empty → blockKey = fieldName, fieldPath = ""
//   - sectionPath ["build"] → blockKey = "build", fieldPath = fieldName
//   - sectionPath ["categories","source"] → blockKey = "categories", fieldPath = "source.fieldName"
func (g *SchemaGenerator) fieldMeta(sectionPath []string, fieldName string) editor.FieldMeta {
	if g.metadata == nil {
		return editor.FieldMeta{}
	}
	if len(sectionPath) == 0 {
		return g.metadata.FieldMeta(fieldName, "")
	}
	blockKey := sectionPath[0]
	fieldPath := fieldName
	if len(sectionPath) > 1 {
		fieldPath = strings.Join(sectionPath[1:], ".") + "." + fieldName
	}
	return g.metadata.FieldMeta(blockKey, fieldPath)
}

func (g *SchemaGenerator) writeRaw(docsDir, name, md string) error {
	out := filepath.Join(docsDir, strings.ToLower(name)+".md")
	valid, ok := validatePathWithinBase(docsDir, out)
	if !ok {
		return fmt.Errorf("invalid docs path: %s", out)
	}
	return os.WriteFile(valid, []byte(md), 0600)
}

func typeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

func validatePathWithinBase(baseDir, targetPath string) (string, bool) {
	cleanBase := filepath.Clean(baseDir)
	cleanTarget := filepath.Clean(targetPath)
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return cleanBase, true
	}
	if strings.HasPrefix(rel, "..") {
		return "", false
	}
	return cleanTarget, true
}
