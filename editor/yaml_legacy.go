package editor

// This file holds the pre-SOT, string-splicing YAML helpers still used by the
// collection-entry flow. New code should prefer the node-based helpers in
// yaml.go; these become deletion candidates once the entry flow is ported.

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// seqEntry holds the display label and the raw indented YAML lines for one
// sequence item.
type seqEntry struct {
	Label   string
	Content string
}

// parseSeqEntries parses the YAML sequence in seqBase using text-based splitting
// to preserve the original formatting of each item. The indentation width is
// detected from the first item line, so both 2-space and 4-space styles work.
func parseSeqEntries(key, seqBase string) []seqEntry {
	prefix := key + ":\n"
	if !strings.HasPrefix(seqBase, prefix) {
		// The value may be on the same line (flow style: `key: [{...}]`).
		// Attempt conversion to block style before giving up.
		if converted := flowToBlockSeq(seqBase); converted != "" {
			return parseSeqEntries(key, converted)
		}
		return nil
	}
	body := strings.TrimPrefix(seqBase, prefix)
	lines := strings.Split(body, "\n")

	// Detect the indentation level from the first sequence item line.
	itemPrefix := ""
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			itemPrefix = line[:len(line)-len(trimmed)] + "- "
			break
		}
	}
	if itemPrefix == "" {
		// Block prefix present but no items found — also try flow-style conversion
		// for the case `key:\n[{...}]` (unlikely but defensive).
		if converted := flowToBlockSeq(seqBase); converted != "" {
			return parseSeqEntries(key, converted)
		}
		return nil
	}
	bareMarker := strings.TrimSuffix(itemPrefix, " ") // e.g. "  -"

	var entries []seqEntry
	var cur []string

	flush := func() {
		for len(cur) > 0 && strings.TrimSpace(cur[len(cur)-1]) == "" {
			cur = cur[:len(cur)-1]
		}
		if len(cur) == 0 {
			return
		}
		content := strings.Join(cur, "\n") + "\n"
		label := labelFromContent(content)
		if label == "" {
			label = fmt.Sprintf("item %d", len(entries)+1)
		}
		entries = append(entries, seqEntry{Label: label, Content: content})
		cur = nil
	}

	for _, line := range lines {
		if strings.HasPrefix(line, itemPrefix) || line == bareMarker {
			flush()
			cur = []string{line}
		} else if len(cur) > 0 {
			cur = append(cur, line)
		}
	}
	flush()
	return entries
}

// labelFromContent extracts the "name" field value from an item's indented YAML
// lines (e.g. "  - name: images\n    enabled: true\n"). Returns "" when absent.
func labelFromContent(content string) string {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte("x:\n"+content), &doc); err != nil {
		return ""
	}
	xVal, ok := doc["x"]
	if !ok {
		return ""
	}
	items, ok := xVal.([]any)
	if !ok || len(items) == 0 {
		return ""
	}
	m, ok := items[0].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := m["name"].(string)
	return name
}

// seqEntriesToBase assembles the full sequence YAML from key and entries.
func seqEntriesToBase(key string, entries []seqEntry) string {
	var sb strings.Builder
	sb.WriteString(key + ":\n")
	for _, e := range entries {
		sb.WriteString(e.Content)
	}
	return sb.String()
}

// itemContentFrom strips the "key:" header from a yamlEditor value and returns
// the item's indented lines. Tolerates optional trailing spaces after the colon.
// Returns "" when the value doesn't start with the expected key header.
func itemContentFrom(key, value string) string {
	rest, ok := strings.CutPrefix(value, key+":")
	if !ok {
		return ""
	}
	rest = strings.TrimLeft(rest, " \t")
	if !strings.HasPrefix(rest, "\n") {
		return ""
	}
	return rest[1:]
}

// applyToggleToEntry is the shared implementation for surgically adding or
// removing a leaf field within a single collection entry shown in the yamlEditor.
// resolveEntry extracts the target mapping node (the entry's value) from the
// block's value node; it differs between sequences and maps.
func applyToggleToEntry(ctx toggleCtx, node treeNode, checked bool, yamlContent string,
	resolveEntry func(blockValue *yaml.Node) *yaml.Node,
) string {
	if len(node.yamlPath) < 2 {
		return yamlContent
	}
	return withYAMLRoot(yamlContent, func(root *yaml.Node) bool {
		mapping := root.Content[0]
		if mapping.Kind != yaml.MappingNode || len(mapping.Content) < 2 {
			return false
		}
		entryNode := resolveEntry(mapping.Content[1])
		if entryNode == nil {
			return false
		}
		fieldPath := node.yamlPath[1:]
		if !applyToggleAt(entryNode, fieldPath[:len(fieldPath)-1], fieldPath[len(fieldPath)-1], checked, ctx, false) {
			return false
		}
		pruneEmptyMappings(entryNode)
		reorderNestedMappingKeys(entryNode, ctx.childDefs)
		return true
	})
}

// applyToggleToSeqItem surgically adds or removes a field from a single
// sequence item shown in the yamlEditor. yamlContent has the form:
//
//	key:
//	  - name: "foo"
//	    enabled: true
//
// node.yamlPath[0] is the seq item label; the actual field path starts at [1].
func applyToggleToSeqItem(ctx toggleCtx, node treeNode, checked bool, yamlContent string) string {
	return applyToggleToEntry(ctx, node, checked, yamlContent, func(blockValue *yaml.Node) *yaml.Node {
		if blockValue.Kind != yaml.SequenceNode || len(blockValue.Content) == 0 {
			return nil
		}
		if blockValue.Content[0].Kind != yaml.MappingNode {
			return nil
		}
		return blockValue.Content[0]
	})
}

// parseMapEntries parses the YAML mapping in mapBase ("block:\n  <entry>:\n
// ...") into one entry per top-level map key, preserving each entry's raw lines.
// The entry indentation is detected from the first non-blank line. It is the map
// navigator's analogue of parseSeqEntries; the entry key becomes the Label.
func parseMapEntries(key, mapBase string) []seqEntry {
	prefix := key + ":\n"
	if !strings.HasPrefix(mapBase, prefix) {
		return nil
	}
	body := strings.TrimPrefix(mapBase, prefix)
	lines := strings.Split(body, "\n")

	entryIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		entryIndent = len(line) - len(strings.TrimLeft(line, " "))
		break
	}
	if entryIndent < 0 {
		return nil
	}

	var entries []seqEntry
	var cur []string
	var curKey string

	flush := func() {
		for len(cur) > 0 && strings.TrimSpace(cur[len(cur)-1]) == "" {
			cur = cur[:len(cur)-1]
		}
		if len(cur) == 0 {
			return
		}
		label := curKey
		if label == "" {
			label = fmt.Sprintf("entry %d", len(entries)+1)
		}
		entries = append(entries, seqEntry{Label: label, Content: strings.Join(cur, "\n") + "\n"})
		cur = nil
	}

	for _, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if strings.TrimSpace(line) != "" && indent == entryIndent {
			flush()
			curKey = mapEntryKey(line)
			cur = []string{line}
		} else if len(cur) > 0 {
			cur = append(cur, line)
		}
	}
	flush()
	return entries
}

// mapEntryKey extracts the map key from an entry's first line, dropping the
// trailing colon and surrounding quotes (`  "3000":` → "3000").
// Uses ": " as the separator before falling back to ":" so keys that contain
// colons (e.g. "ghcr.io/features/git:1") are preserved intact.
func mapEntryKey(line string) string {
	s := strings.TrimSpace(line)
	if strings.HasSuffix(s, ":") {
		s = strings.TrimSuffix(s, ":")
	} else if i := strings.Index(s, ": "); i >= 0 {
		s = s[:i]
	} else if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(strings.TrimSpace(s), `"'`)
}

// applyToggleToMapEntry surgically adds or removes a field from the single map
// entry shown in the yamlEditor. yamlContent has the form:
//
//	block:
//	  "3000":
//	    label: web
//
// node.yamlPath[0] is the entry key; the field path starts at [1].
func applyToggleToMapEntry(ctx toggleCtx, node treeNode, checked bool, yamlContent string) string {
	return applyToggleToEntry(ctx, node, checked, yamlContent, func(blockValue *yaml.Node) *yaml.Node {
		// blockValue is the map: entry-key → struct. Take the first entry's value.
		if blockValue.Kind != yaml.MappingNode || len(blockValue.Content) < 2 {
			return nil
		}
		if blockValue.Content[1].Kind != yaml.MappingNode {
			return nil
		}
		return blockValue.Content[1]
	})
}

// flowToBlockSeq attempts to convert a flow-style sequence (e.g.
// `categories: [{name: foo}]`) to block style so parseSeqEntries can handle it.
// Returns the converted string, or "" if conversion fails or is unnecessary.
func flowToBlockSeq(seqBase string) string {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(seqBase), &doc); err != nil {
		return ""
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return ""
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode || len(mapping.Content) < 2 {
		return ""
	}
	seqNode := mapping.Content[1]
	if seqNode.Kind != yaml.SequenceNode || seqNode.Style == 0 {
		return "" // already block style or not a sequence
	}
	// Force block style on the sequence and all child nodes.
	setBlockStyle(&doc)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return ""
	}
	return string(out)
}

// setBlockStyle recursively clears flow style flags so yaml.Marshal emits block style.
func setBlockStyle(n *yaml.Node) {
	n.Style = 0
	for _, child := range n.Content {
		setBlockStyle(child)
	}
}
