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

// GenerateAllDocs generates one markdown file per section: the root type plus
// each top-level field with nested children. Returns the section names written.
func (g *SchemaGenerator) GenerateAllDocs(v any, docsDir string) ([]string, error) {
	if err := os.MkdirAll(docsDir, 0750); err != nil {
		return nil, fmt.Errorf("create docs dir: %w", err)
	}
	fields := schema.Discover(v)
	root := typeName(v)
	var names []string

	if err := g.writeDocFile(docsDir, root, fields, nil); err != nil {
		return nil, err
	}
	names = append(names, root)

	for _, f := range fields {
		if len(f.Children) == 0 {
			continue
		}
		if err := g.writeDocFile(docsDir, f.YAMLName, f.Children, []string{f.YAMLName}); err != nil {
			return nil, err
		}
		names = append(names, f.YAMLName)
	}
	return names, nil
}

// GenerateDocsInMemory generates markdown for the root type and each top-level
// field with nested children, keyed by name.
func (g *SchemaGenerator) GenerateDocsInMemory(v any) map[string]string {
	fields := schema.Discover(v)
	root := typeName(v)
	result := map[string]string{}

	result[root] = g.generateMarkdown(root, fields, nil)
	for _, f := range fields {
		if len(f.Children) > 0 {
			result[f.YAMLName] = g.generateMarkdown(f.YAMLName, f.Children, []string{f.YAMLName})
		}
	}
	return result
}

// GenerateIndex writes an index.md listing all section names to docsDir.
func GenerateIndex(docsDir string, names []string) error {
	var sb strings.Builder
	sb.WriteString("# Documentation Index\n\n")
	sb.WriteString("This documentation describes all available configuration structures.\n\n")
	sb.WriteString("## Available Configurations\n\n")
	for _, name := range names {
		sb.WriteString(fmt.Sprintf("- [%s](./%s.md)\n", name, strings.ToLower(name)))
	}
	return os.WriteFile(filepath.Join(docsDir, "index.md"), []byte(sb.String()), 0600)
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

func (g *SchemaGenerator) writeDocFile(docsDir, name string, fields []schema.FieldDef, sectionPath []string) error {
	md := g.generateMarkdown(name, fields, sectionPath)
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
