// Package hint renders field metadata into the body of the editor's
// Hint/Example panel.
package hint

import (
	"strconv"
	"strings"

	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/theme"
)

// Render formats a FieldMeta into the Hint/Example panel body. example is
// passed separately because the caller may substitute a generated fallback
// when meta.Example is empty. An all-zero FieldMeta renders the empty string,
// which callers use to fall back to a "no metadata" placeholder.
func Render(th theme.Resolved, meta spec.FieldMeta, example string) string {
	label := func(s string) string { return th.HintKey.Render(s) }

	var lines []string
	field := func(name, value string) {
		lines = append(lines, label(name)+" "+value)
	}
	list := func(name string, values []string) {
		lines = append(lines, label(name))
		for _, v := range values {
			lines = append(lines, "  • "+v)
		}
	}

	if meta.Description != "" {
		field("Description:", meta.Description)
	}
	if t := typeStr(meta); t != "" {
		field("Type:", t)
	}
	if line := formatLine(meta); line != "" {
		field("Format:", line)
	}
	if meta.Required {
		field("Required:", "yes")
	}
	if meta.Default != "" {
		field("Default:", meta.Default)
	}
	if len(meta.OneOf) > 0 {
		list("Allowed Values:", meta.OneOf)
	}
	if line := rangeLine(meta); line != "" {
		field("Range:", line)
	}
	if line := lengthLine(meta); line != "" {
		field("Length:", line)
	}
	if len(meta.NotOneOf) > 0 {
		list("Denied:", meta.NotOneOf)
	}
	if meta.Pattern != "" {
		field("Pattern:", meta.Pattern)
	}
	if meta.MinCount > 0 || meta.MaxCount > 0 {
		upper := "∞"
		if meta.MaxCount > 0 {
			upper = strconv.Itoa(meta.MaxCount)
		}
		field("Entries:", strconv.Itoa(meta.MinCount)+" – "+upper)
	}
	if meta.Unique {
		field("Unique:", "yes")
	}
	if meta.Deprecated != "" {
		field("Deprecated:", meta.Deprecated)
	}
	if example != "" {
		lines = append(lines, label("Example:"))
		for _, line := range strings.Split(strings.TrimSuffix(example, "\n"), "\n") {
			lines = append(lines, "  "+line)
		}
	}

	if len(lines) == 0 {
		return ""
	}
	// Every line is newline-terminated, including the last, so the panel body
	// concatenates cleanly with whatever the caller appends.
	return strings.Join(lines, "\n") + "\n"
}

func typeStr(meta spec.FieldMeta) string {
	if meta.Type != "" {
		return meta.Type
	}
	if meta.Multiline {
		return "multiline string"
	}
	return ""
}

func formatLine(meta spec.FieldMeta) string {
	var labels []string
	for _, f := range meta.Formats {
		if !f.IsZero() {
			labels = append(labels, f.Label())
		}
	}
	return strings.Join(labels, " | ")
}

func rangeLine(meta spec.FieldMeta) string {
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

func lengthLine(meta spec.FieldMeta) string {
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
