package editor

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// rebuildYAML constructs the YAML body for a block by concatenating the
// pre-indented snippets of every checked field. Falls back to "<field>: \n"
// when no snippet is provided for a field.
func rebuildYAML(key string, fields []fieldState, snippets map[string]string) string {
	var sb strings.Builder
	sb.WriteString(key + ":\n")
	for _, fs := range fields {
		if !fs.Checked {
			continue
		}
		if snip, ok := snippets[fs.Def.YAMLName]; ok && snip != "" {
			sb.WriteString(snip)
		} else {
			sb.WriteString("  " + fs.Def.YAMLName + ": \n")
		}
	}
	return sb.String()
}

// syncFieldsFromYAML updates Checked on each field to reflect what is present
// in content (the current textarea value for the given key). Unparseable
// content is treated as "no change".
func syncFieldsFromYAML(key string, fields []fieldState, content string) []fieldState {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return fields
	}
	sub, _ := doc[key].(map[string]any)
	out := make([]fieldState, len(fields))
	copy(out, fields)
	for i := range out {
		_, out[i].Checked = sub[out[i].Def.YAMLName]
	}
	return out
}

// applyFieldToggle surgically adds or removes a single sub-field from current
// (the textarea YAML value), preserving manual edits to other fields.
//
// snippet is the indented YAML chunk to insert when checked=true (taken from
// Config.FieldSnippets). When checked=false, snippet is ignored.
//
// If the surgery fails for any reason (unparseable YAML, unexpected shape,
// missing snippet), falls back to rebuildYAML so the UI never desynchronises
// from the field state.
func applyFieldToggle(key string, fields []fieldState, fieldName, snippet string, checked bool, current string, allSnippets map[string]string) string {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(current), &root); err != nil || root.Kind == 0 || len(root.Content) == 0 {
		return rebuildYAML(key, fields, allSnippets)
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode || len(mapping.Content) < 2 {
		return rebuildYAML(key, fields, allSnippets)
	}
	valueNode := mapping.Content[1]
	if valueNode.Kind != yaml.MappingNode {
		return rebuildYAML(key, fields, allSnippets)
	}

	idx := -1
	for i := 0; i < len(valueNode.Content)-1; i += 2 {
		if valueNode.Content[i].Value == fieldName {
			idx = i
			break
		}
	}

	switch {
	case !checked:
		if idx >= 0 {
			valueNode.Content = append(valueNode.Content[:idx], valueNode.Content[idx+2:]...)
		}
	case idx >= 0:
		// already present — keep as is
	case snippet == "":
		return rebuildYAML(key, fields, allSnippets)
	default:
		if !appendFieldFromSnippet(valueNode, key, snippet) {
			return rebuildYAML(key, fields, allSnippets)
		}
	}

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return rebuildYAML(key, fields, allSnippets)
	}
	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// appendFieldFromSnippet parses snippet under parentKey, extracts the first
// child key/value pair, and appends it to valueNode. Returns false if the
// snippet is malformed.
func appendFieldFromSnippet(valueNode *yaml.Node, parentKey, snippet string) bool {
	var templateRoot yaml.Node
	if err := yaml.Unmarshal([]byte(parentKey+":\n"+snippet), &templateRoot); err != nil {
		return false
	}
	if templateRoot.Kind == 0 || len(templateRoot.Content) == 0 {
		return false
	}
	tMapping := templateRoot.Content[0]
	if tMapping.Kind != yaml.MappingNode || len(tMapping.Content) < 2 {
		return false
	}
	tValue := tMapping.Content[1]
	if tValue.Kind != yaml.MappingNode || len(tValue.Content) < 2 {
		return false
	}
	valueNode.Content = append(valueNode.Content, tValue.Content[0], tValue.Content[1])
	return true
}
