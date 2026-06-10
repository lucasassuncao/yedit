package editor

import (
	"strings"

	"github.com/lucasassuncao/yedit/schema"
)

// selectedHint renders the Hint/Example panel body for the currently selected
// list item. All display data comes from HintSource; the "base" preset is used
// as a fallback example when meta.Example is empty.
func (m model) selectedHint() string {
	it := m.list.SelectedItem()
	if it == nil || it.Separator {
		return m.theme.hintDim.Render("  select a field to see hints")
	}
	if it.Unknown {
		return m.theme.hintDim.Render("  unknown key — not in the schema")
	}
	def := fieldDefByName(m.schemaTree, it.Key)
	if def.YAMLName == "" {
		def.YAMLName = it.Key
	}

	var meta FieldMeta
	if m.cfg.Hints != nil {
		meta = m.cfg.Hints.FieldHint(it.Key, "")
	}
	example := meta.Example
	if example == "" {
		example = m.hintExample(it.Key, def)
	}
	return renderFieldHint(m.theme, meta, example)
}

// hintExample resolves the Example snippet for a top-level block: the "base"
// preset when one exists, otherwise a structural fallback from the schema.
func (m model) hintExample(key string, def schema.FieldDef) string {
	if m.cfg.Presets != nil {
		if y, err := m.cfg.Presets.PresetYAML(key, "base"); err == nil {
			return y
		}
	}
	return generateFallbackExample(def)
}

// anyChildRequired reports whether any direct child field is marked required.
func anyChildRequired(children []schema.FieldDef) bool {
	for _, c := range children {
		if c.Required {
			return true
		}
	}
	return false
}

// renderFieldHint formats a FieldMeta into the Hint/Example panel body.
// Order: description, type, required, default, allowed values, example.
// example is passed separately because the caller may substitute a generated
// fallback when meta.Example is empty.
func renderFieldHint(th resolvedTheme, meta FieldMeta, example string) string {
	var sb strings.Builder

	label := func(s string) string { return th.hintKey.Render(s) }

	if meta.Description != "" {
		sb.WriteString(label("Description:") + " " + meta.Description + "\n")
	}

	if meta.Type != "" {
		sb.WriteString(label("Type:") + " " + meta.Type + "\n")
	}

	if meta.Required {
		sb.WriteString(label("Required:") + " yes\n")
	}

	if meta.Default != "" {
		sb.WriteString(label("Default:") + " " + meta.Default + "\n")
	}

	if len(meta.OneOf) > 0 {
		sb.WriteString(label("Allowed Values:") + "\n")
		for _, v := range meta.OneOf {
			sb.WriteString("  • " + v + "\n")
		}
	}

	if example != "" {
		sb.WriteString(label("Example:") + "\n")
		for _, line := range strings.Split(strings.TrimRight(example, "\n"), "\n") {
			sb.WriteString("  " + line + "\n")
		}
	}

	return sb.String()
}

// generateFallbackExample produces a minimal valid YAML snippet for def when no
// HintSource example is available.
func generateFallbackExample(def schema.FieldDef) string {
	switch def.Kind {
	case schema.KindEnum:
		if len(def.OneOf) > 0 {
			return def.YAMLName + ": " + def.OneOf[0]
		}
		return def.YAMLName + ": \"\""
	case schema.KindList:
		return def.YAMLName + ":\n  - "
	case schema.KindDictionary:
		return def.YAMLName + ":\n  key: value"
	case schema.KindObject:
		if len(def.Children) == 0 {
			return def.YAMLName + ":\n  # ..."
		}
		var sb strings.Builder
		sb.WriteString(def.YAMLName + ":\n")
		for _, child := range def.Children {
			val := "\"\""
			if child.Default != "" {
				val = child.Default
			}
			sb.WriteString("  " + child.YAMLName + ": " + val + "\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case schema.KindVariant:
		return def.YAMLName + ": \"\""
	default: // KindPrimitive
		if def.Default != "" {
			return def.YAMLName + ": " + def.Default
		}
		return def.YAMLName + ": \"\""
	}
}
