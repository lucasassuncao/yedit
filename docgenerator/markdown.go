package docgenerator

import (
	"fmt"
	"strings"

	"github.com/lucasassuncao/yedit/schema"
)

// generateRootMarkdown generates a summary page for a root type that links to
// per-field files instead of inlining nested sections. Used by GenerateDocsForEach
// when splitStructs is true.
func (g *SchemaGenerator) generateRootMarkdown(title string, fields []schema.FieldDef) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", title))
	sb.WriteString("## Arguments\n\n")
	sb.WriteString("The following arguments are supported:\n\n")
	g.writeFieldsTableLinked(&sb, fields)
	return sb.String()
}

func (g *SchemaGenerator) writeFieldsTableLinked(sb *strings.Builder, fields []schema.FieldDef) {
	sb.WriteString("| Name | Type | Description | Required | Default |\n")
	sb.WriteString("|------|------|-------------|----------|---------|\n")
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
		fmt.Fprintf(sb, "| %s | %s | %s | %s | %s |\n",
			displayName, typeLabel(f), description, required, defaultValue)
	}
	sb.WriteString("\n")
}

func (g *SchemaGenerator) generateMarkdown(typeName string, fields []schema.FieldDef, sectionPath []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s\n\n", typeName))
	sb.WriteString("## Arguments\n\n")
	sb.WriteString("The following arguments are supported:\n\n")
	g.writeFieldsTable(&sb, fields, sectionPath)
	g.writeNestedSections(&sb, fields, sectionPath, 3)

	return sb.String()
}

func (g *SchemaGenerator) writeFieldsTable(sb *strings.Builder, fields []schema.FieldDef, sectionPath []string) {
	sb.WriteString("| Name | Type | Description | Required | Default |\n")
	sb.WriteString("|------|------|-------------|----------|---------|\n")

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

		fmt.Fprintf(sb, "| %s | %s | %s | %s | %s |\n",
			displayName, typeLabel(f), description, required, defaultValue)
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
