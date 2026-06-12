package editor

import (
	"strconv"
	"strings"
)

// selectedHint renders the Hint/Example panel body for the currently selected
// list item. All display data comes from MetadataSource.
func (m model) selectedHint() string {
	if m.cfg.Metadata == nil {
		return m.theme.hintDim.Render("  Config.Metadata is not set - no metadata source configured")
	}
	it := m.list.SelectedItem()
	if it == nil || it.Separator {
		return m.theme.hintDim.Render("  select a field to see hints")
	}
	if it.Unknown {
		return m.theme.hintDim.Render("  unknown key - not in the schema")
	}
	def := fieldDefByName(m.schemaTree, it.Key)
	if def.YAMLName == "" {
		def.YAMLName = it.Key
	}
	meta := m.cfg.Metadata.FieldMeta(it.Key, "")
	ex := meta.Example
	if ex == "" && meta.Multiline {
		ex = it.Key + ": |\n  line 1\n  line 2\n"
	}
	if out := renderFieldHint(m.theme, meta, ex); out != "" {
		return out
	}
	return m.theme.hintDim.Render("  no metadata declared for this field")
}

// renderFieldHint formats a FieldMeta into the Hint/Example panel body.
// Order: description, type, format, required, default, allowed values, range,
// length, denied values, pattern, entries, unique, deprecated, example.
// example is passed separately because the caller may substitute a generated
// fallback when meta.Example is empty.
func renderFieldHint(th resolvedTheme, meta FieldMeta, example string) string {
	var sb strings.Builder
	label := func(s string) string { return th.hintKey.Render(s) }

	if meta.Description != "" {
		sb.WriteString(label("Description:") + " " + meta.Description + "\n")
	}
	if typeStr := hintTypeStr(meta); typeStr != "" {
		sb.WriteString(label("Type:") + " " + typeStr + "\n")
	}
	if line := hintFormatLine(meta); line != "" {
		sb.WriteString(label("Format:") + " " + line + "\n")
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
	if line := hintRangeLine(meta); line != "" {
		sb.WriteString(label("Range:") + " " + line + "\n")
	}
	if line := hintLengthLine(meta); line != "" {
		sb.WriteString(label("Length:") + " " + line + "\n")
	}
	if len(meta.NotOneOf) > 0 {
		sb.WriteString(label("Denied:") + "\n")
		for _, v := range meta.NotOneOf {
			sb.WriteString("  • " + v + "\n")
		}
	}
	if meta.Pattern != "" {
		sb.WriteString(label("Pattern:") + " " + meta.Pattern + "\n")
	}
	if meta.MinCount > 0 || meta.MaxCount > 0 {
		upper := "∞"
		if meta.MaxCount > 0 {
			upper = strconv.Itoa(meta.MaxCount)
		}
		sb.WriteString(label("Entries:") + " " + strconv.Itoa(meta.MinCount) + " – " + upper + "\n")
	}
	if meta.Unique {
		sb.WriteString(label("Unique:") + " yes\n")
	}
	if meta.Deprecated != "" {
		sb.WriteString(label("Deprecated:") + " " + meta.Deprecated + "\n")
	}
	if example != "" {
		sb.WriteString(label("Example:") + "\n")
		for _, line := range strings.Split(strings.TrimRight(example, "\n"), "\n") {
			sb.WriteString("  " + line + "\n")
		}
	}
	return sb.String()
}

func hintTypeStr(meta FieldMeta) string {
	if meta.Type != "" {
		return meta.Type
	}
	if meta.Multiline {
		return "multiline string"
	}
	return ""
}

func hintFormatLine(meta FieldMeta) string {
	var labels []string
	for _, f := range meta.Formats {
		if !f.IsZero() {
			labels = append(labels, f.Label())
		}
	}
	return strings.Join(labels, " | ")
}

func hintRangeLine(meta FieldMeta) string {
	switch {
	case meta.Min != "" && meta.Max != "":
		return meta.Min + " – " + meta.Max
	case meta.Min != "":
		return "≥ " + meta.Min
	case meta.Max != "":
		return "≤ " + meta.Max
	}
	return ""
}

func hintLengthLine(meta FieldMeta) string {
	switch {
	case meta.MinLength > 0 && meta.MaxLength > 0:
		return strconv.Itoa(meta.MinLength) + "–" + strconv.Itoa(meta.MaxLength) + " chars"
	case meta.MinLength > 0:
		return "min " + strconv.Itoa(meta.MinLength) + " chars"
	case meta.MaxLength > 0:
		return "max " + strconv.Itoa(meta.MaxLength) + " chars"
	}
	return ""
}
