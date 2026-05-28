package editor

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

// toggleCtx bundles the immutable context shared by all YAML toggle helpers.
type toggleCtx struct {
	key       string
	snippets  map[string]string
	childDefs []schema.FieldDef
}

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

// applyToggleToSeqItem surgically adds or removes a field from a single
// sequence item shown in the yamlEditor. yamlContent has the form:
//
//	key:
//	  - name: "foo"
//	    enabled: true
//
// node.yamlPath[0] is the seq item label; the actual field path starts at [1].
func applyToggleToSeqItem(ctx toggleCtx, node treeNode, checked bool, yamlContent string) string {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlContent), &root); err != nil || root.Kind == 0 || len(root.Content) == 0 {
		return yamlContent
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode || len(mapping.Content) < 2 {
		return yamlContent
	}
	seqNode := mapping.Content[1]
	if seqNode.Kind != yaml.SequenceNode || len(seqNode.Content) == 0 {
		return yamlContent
	}
	itemNode := seqNode.Content[0]
	if itemNode.Kind != yaml.MappingNode {
		return yamlContent
	}

	// Strip the seq item label from the path; the rest is the field path.
	if len(node.yamlPath) < 2 {
		return yamlContent
	}
	fieldPath := node.yamlPath[1:] // e.g. ["enabled"] or ["source", "path"]

	// Navigate to the parent mapping of the leaf.
	cur := itemNode
	for i := 0; i < len(fieldPath)-1 && cur != nil; i++ {
		cur = findOrCreateMappingChild(cur, fieldPath[i])
	}
	if cur == nil {
		return yamlContent
	}

	leafName := fieldPath[len(fieldPath)-1]
	if !checked {
		removeMappingKey(cur, leafName)
	} else if !hasMappingKey(cur, leafName) {
		appendLeafToMapping(cur, leafName, ctx.snippets[leafName])
	}

	pruneEmptyMappings(itemNode)
	reorderNestedMappingKeys(itemNode, ctx.childDefs)

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return yamlContent
	}
	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// yamlForSeqItem returns the YAML editor value for a specific sequence item
// (the full "key:\n  - ...\n" form), so the right panel can show just that item.
func yamlForSeqItem(key, seqBase string, seqIdx int) string {
	entries := parseSeqEntries(key, seqBase)
	if seqIdx < 0 || seqIdx >= len(entries) {
		return key + ":\n"
	}
	return key + ":\n" + entries[seqIdx].Content
}

// syncTreeCheckedFromYAML re-derives checked states for all treeNodeField leaf
// nodes from the current YAML text (the right panel value), then re-applies
// section grouping for KindStruct trees, restoring the cursor position.
func syncTreeCheckedFromYAML(tm treeModel, key, yamlContent string) treeModel {
	// Save selected node's path so we can restore the cursor after reorder.
	var selectedPath []string
	if ni := tm.currentNodeIdx(); ni >= 0 && tm.nodes[ni].kind == treeNodeField {
		selectedPath = tm.nodes[ni].yamlPath
	}

	tm.nodes = syncTreeCheckedStates(tm.nodes, key, yamlContent)

	if !tm.isSeq {
		tm.nodes = applySections(tm.nodes)
		tm = tm.restoreCursorToPath(selectedPath)
	}
	return tm
}

// applyDepth0Toggle adds or removes a depth-0 field from a struct block's value mapping.
func applyDepth0Toggle(ctx toggleCtx, fieldName string, checked bool, valueNode *yaml.Node) {
	snippet := ctx.snippets[fieldName]
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
	case snippet != "":
		if !appendFieldFromSnippet(valueNode, ctx.key, snippet) {
			appendLeafToMapping(valueNode, fieldName, "")
		}
	default:
		appendLeafToMapping(valueNode, fieldName, "")
	}
}

// applyNestedToggle adds or removes a deeply-nested leaf field. Returns false
// if a required parent mapping cannot be found.
func applyNestedToggle(ctx toggleCtx, node treeNode, checked bool, valueNode *yaml.Node) bool {
	fieldPath := node.yamlPath
	cur := valueNode
	for i := 0; i < len(fieldPath)-1; i++ {
		cur = findOrCreateMappingChild(cur, fieldPath[i])
		if cur == nil {
			return false
		}
	}
	leafName := fieldPath[len(fieldPath)-1]
	if !checked {
		removeMappingKey(cur, leafName)
	} else if !hasMappingKey(cur, leafName) {
		appendLeafToMapping(cur, leafName, ctx.snippets[leafName])
	}
	return true
}

// applyTreeToggle surgically adds or removes a single leaf field from the
// current YAML of a struct block. Falls back to a simple rebuild on any error.
func applyTreeToggle(ctx toggleCtx, node treeNode, checked bool, current string) string {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(current), &root); err != nil || root.Kind == 0 || len(root.Content) == 0 {
		return current
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode || len(mapping.Content) < 2 {
		return current
	}
	valueNode := mapping.Content[1]
	if valueNode.Kind != yaml.MappingNode {
		return current
	}

	if node.depth == 0 {
		applyDepth0Toggle(ctx, node.label, checked, valueNode)
	} else if !applyNestedToggle(ctx, node, checked, valueNode) {
		return current
	}

	pruneEmptyMappings(valueNode)
	reorderNestedMappingKeys(valueNode, ctx.childDefs)

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return current
	}
	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// findOrCreateMappingChild finds a child mapping node by key, creating it if absent.
func findOrCreateMappingChild(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	// Create the key with an empty mapping value.
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valNode := &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content, keyNode, valNode)
	return valNode
}

// removeMappingKey removes a key-value pair from a mapping node.
func removeMappingKey(mapping *yaml.Node, key string) {
	if mapping.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

// pruneEmptyMappings removes key-value pairs whose value is an empty mapping
// node ({}), recursing into nested mappings first so the cleanup propagates
// upward (e.g. source becomes empty after all its children are toggled off).
func pruneEmptyMappings(mapping *yaml.Node) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	for i := 1; i < len(mapping.Content); i += 2 {
		pruneEmptyMappings(mapping.Content[i])
	}
	i := 0
	for i < len(mapping.Content)-1 {
		val := mapping.Content[i+1]
		if val.Kind == yaml.MappingNode && len(val.Content) == 0 {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
		} else {
			i += 2
		}
	}
}

// hasMappingKey reports whether a mapping node contains the given key.
func hasMappingKey(mapping *yaml.Node, key string) bool {
	if mapping.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return true
		}
	}
	return false
}

// reorderMappingKeys sorts the key-value pairs of a MappingNode to match the
// field order defined by defs. Unknown keys keep their relative position after
// all known keys.
func reorderMappingKeys(mapping *yaml.Node, defs []schema.FieldDef) {
	if mapping == nil || mapping.Kind != yaml.MappingNode || len(defs) == 0 {
		return
	}
	n := len(mapping.Content) / 2
	if n < 2 {
		return
	}
	order := make(map[string]int, len(defs))
	for i, d := range defs {
		order[d.YAMLName] = i
	}
	type kv struct{ k, v *yaml.Node }
	pairs := make([]kv, n)
	for i := range pairs {
		pairs[i] = kv{mapping.Content[i*2], mapping.Content[i*2+1]}
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		oi, okI := order[pairs[i].k.Value]
		oj, okJ := order[pairs[j].k.Value]
		if okI && okJ {
			return oi < oj
		}
		return okI && !okJ
	})
	for i, p := range pairs {
		mapping.Content[i*2] = p.k
		mapping.Content[i*2+1] = p.v
	}
}

// reorderNestedMappingKeys recursively sorts a MappingNode's keys to match
// schema order, descending into nested struct mappings using def.Children.
func reorderNestedMappingKeys(mapping *yaml.Node, defs []schema.FieldDef) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}
	reorderMappingKeys(mapping, defs)
	// Build a lookup from field name → children defs for nested structs.
	childrenOf := make(map[string][]schema.FieldDef, len(defs))
	for _, d := range defs {
		if len(d.Children) > 0 {
			childrenOf[d.YAMLName] = d.Children
		}
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		key := mapping.Content[i].Value
		val := mapping.Content[i+1]
		if val.Kind == yaml.MappingNode {
			if children, ok := childrenOf[key]; ok {
				reorderNestedMappingKeys(val, children)
			}
		}
	}
}

// appendLeafToMapping appends a scalar key with empty value (or snippet) to a mapping.
func appendLeafToMapping(mapping *yaml.Node, key, snippet string) {
	if mapping.Kind != yaml.MappingNode {
		return
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: ""}
	if snippet != "" {
		// Parse the snippet to extract the value.
		var tmp map[string]any
		if err := yaml.Unmarshal([]byte(key+":\n"+snippet), &tmp); err == nil {
			if v, ok := tmp[key]; ok {
				valNode.Value = fmt.Sprintf("%v", v)
			}
		}
	}
	mapping.Content = append(mapping.Content, keyNode, valNode)
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
