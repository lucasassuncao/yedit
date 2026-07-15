package docgenerator

import (
	"fmt"
	"strings"

	"github.com/lucasassuncao/yedit/editor"
	"github.com/lucasassuncao/yedit/schema"
)

// generateRootMarkdown generates a summary page for a root type that links to
// per-field files instead of inlining nested sections. Used by GenerateDocsForEach
// when splitStructs is true.
func (g *SchemaGenerator) generateRootMarkdown(title string, fields []schema.FieldDef) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", title)
	g.writeExamplesLink(&sb, title)
	sb.WriteString("## Arguments\n\n")
	sb.WriteString("The following arguments are supported:\n\n")
	g.writeFieldsTableLinked(&sb, fields)
	return sb.String()
}

func (g *SchemaGenerator) writeFieldsTableLinked(sb *strings.Builder, fields []schema.FieldDef) {
	hasFormat := g.anyHasFormat(fields, nil)
	if hasFormat {
		sb.WriteString("| Name | Type | Format | Description | Required | Default |\n")
		sb.WriteString("|------|------|--------|-------------|----------|---------|\n")
	} else {
		sb.WriteString("| Name | Type | Description | Required | Default |\n")
		sb.WriteString("|------|------|-------------|----------|---------|\n")
	}
	for _, f := range fields {
		name := f.YAMLName
		if name == "" || name == "-" {
			continue
		}
		meta := g.fieldMeta(nil, name)
		description := strings.ReplaceAll(meta.Description, "|", "\\|")
		description = strings.Join(strings.Fields(description), " ")
		required := "No"
		if meta.Required {
			required = "Yes"
		}
		defaultValue := "-"
		if meta.Default != "" {
			defaultValue = meta.Default
		}
		displayName := name
		if len(f.Children) > 0 {
			displayName = fmt.Sprintf("[%s](./%s.md)", name, name)
		}
		if hasFormat {
			fmt.Fprintf(sb, "| %s | %s | %s | %s | %s | %s |\n",
				displayName, docTypeLabel(f, meta), formatLabels(meta), description, required, defaultValue)
		} else {
			fmt.Fprintf(sb, "| %s | %s | %s | %s | %s |\n",
				displayName, docTypeLabel(f, meta), description, required, defaultValue)
		}
	}
	sb.WriteString("\n")
}

func (g *SchemaGenerator) generateMarkdown(typeName string, fields []schema.FieldDef, sectionPath []string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# %s\n\n", typeName)
	g.writeExamplesLink(&sb, typeName)
	sb.WriteString("## Arguments\n\n")
	sb.WriteString("The following arguments are supported:\n\n")
	g.writeFieldsTable(&sb, fields, sectionPath)
	g.writeNestedSections(&sb, fields, sectionPath, 3)

	return sb.String()
}

// writeExamplesLink emits an "Examples" section linking to the preset example
// page for title when the generator was configured with WithExamples and an
// example page exists for it. The lookup is by lowercased title so both the
// root page (e.g. "yedit") and split-child pages (e.g. "categories")
// resolve to "<title>.md" in the examples directory.
func (g *SchemaGenerator) writeExamplesLink(sb *strings.Builder, title string) {
	if g.examplesRelDir == "" {
		return
	}
	key := strings.ToLower(title)
	if !g.examplePages[key] {
		return
	}
	sb.WriteString("## Examples\n\n")
	fmt.Fprintf(sb, "For usage examples, see [%s presets](%s/%s.md).\n\n", title, g.examplesRelDir, key)
}

func (g *SchemaGenerator) writeFieldsTable(sb *strings.Builder, fields []schema.FieldDef, sectionPath []string) {
	hasFormat := g.anyHasFormat(fields, sectionPath)
	if hasFormat {
		sb.WriteString("| Name | Type | Format | Description | Required | Default |\n")
		sb.WriteString("|------|------|--------|-------------|----------|---------|\n")
	} else {
		sb.WriteString("| Name | Type | Description | Required | Default |\n")
		sb.WriteString("|------|------|-------------|----------|---------|\n")
	}

	for _, f := range fields {
		name := f.YAMLName
		if name == "" || name == "-" {
			continue
		}

		meta := g.fieldMeta(sectionPath, name)

		description := strings.ReplaceAll(meta.Description, "|", "\\|")
		description = strings.Join(strings.Fields(description), " ")

		required := "No"
		if meta.Required {
			required = "Yes"
		}

		defaultValue := "-"
		if meta.Default != "" {
			defaultValue = meta.Default
		}

		displayName := name

		if hasFormat {
			fmt.Fprintf(sb, "| %s | %s | %s | %s | %s | %s |\n",
				displayName, docTypeLabel(f, meta), formatLabels(meta), description, required, defaultValue)
		} else {
			fmt.Fprintf(sb, "| %s | %s | %s | %s | %s |\n",
				displayName, docTypeLabel(f, meta), description, required, defaultValue)
		}
	}
	sb.WriteString("\n")
}

func (g *SchemaGenerator) writeNestedSections(sb *strings.Builder, fields []schema.FieldDef, sectionPath []string, level int) {
	hashes := strings.Repeat("#", level)

	for _, f := range fields {
		if len(f.Children) == 0 {
			continue
		}
		name := f.YAMLName
		childPath := append(append([]string(nil), sectionPath...), name)

		fmt.Fprintf(sb, "%s %s\n\n", hashes, name)
		sb.WriteString("The following arguments are supported:\n\n")
		g.writeFieldsTable(sb, f.Children, childPath)
		g.writeNestedSections(sb, f.Children, childPath, level+1)
	}
}

// anyHasFormat reports whether any field in fields has a non-empty Formats slice.
func (g *SchemaGenerator) anyHasFormat(fields []schema.FieldDef, sectionPath []string) bool {
	for _, f := range fields {
		name := f.YAMLName
		if name == "" || name == "-" {
			continue
		}
		meta := g.fieldMeta(sectionPath, name)
		if len(meta.Formats) > 0 {
			return true
		}
	}
	return false
}

// formatLabels returns the joined format labels for a field, or "-" when none.
func formatLabels(meta editor.FieldMeta) string {
	var labels []string
	for _, f := range meta.Formats {
		if !f.IsZero() {
			labels = append(labels, f.Label())
		}
	}
	if len(labels) == 0 {
		return "-"
	}
	return strings.Join(labels, " | ")
}

// docTypeLabel returns the type label for the markdown table, honouring
// FieldMeta.Multiline and FieldMeta.Type overrides.
func docTypeLabel(f schema.FieldDef, meta editor.FieldMeta) string {
	if meta.Multiline && meta.Type == "" {
		return "multiline string"
	}
	if meta.Type != "" {
		return meta.Type
	}
	return typeLabel(f)
}

func typeLabel(f schema.FieldDef) string {
	switch f.Kind {
	case schema.KindPrimitive:
		if f.Scalar != "" {
			return f.Scalar
		}
		return "any"
	case schema.KindObject, schema.KindVariant:
		return "object"
	case schema.KindList:
		if len(f.Children) > 0 {
			return "array[object]"
		}
		if f.Scalar != "" {
			return "array[" + f.Scalar + "]"
		}
		return "array"
	case schema.KindDictionary:
		if len(f.Children) > 0 {
			return "map[string]object"
		}
		if f.Scalar != "" {
			return "map[string]" + f.Scalar
		}
		return "map"
	case schema.KindAny:
		return "any"
	default:
		return "-"
	}
}
