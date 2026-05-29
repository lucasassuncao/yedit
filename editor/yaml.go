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

// yamlForSeqItem returns the YAML editor value for a specific sequence item
// (the full "key:\n  - ...\n" form), so the right panel can show just that item.
// It also serves the map navigator: a map entry's Content already carries its
// own "key:" line, so the same "block:\n" + Content assembly applies.
func yamlForSeqItem(key, seqBase string, seqIdx int) string {
	entries := parseSeqEntries(key, seqBase)
	if seqIdx < 0 || seqIdx >= len(entries) {
		return key + ":\n"
	}
	return key + ":\n" + entries[seqIdx].Content
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
func mapEntryKey(line string) string {
	s := strings.TrimSpace(line)
	if strings.HasSuffix(s, ":") {
		s = strings.TrimSuffix(s, ":")
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

// syncTreeCheckedFromYAML re-derives checked states for all treeNodeField leaf
// nodes from the current YAML text (the right panel value), then re-applies
// section grouping for KindObject trees, restoring the cursor position.
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

// applyTreeToggle surgically adds or removes a single leaf field from the
// current YAML of a struct block. Falls back to a simple rebuild on any error.
func applyTreeToggle(ctx toggleCtx, node treeNode, checked bool, current string) string {
	return withYAMLRoot(current, func(root *yaml.Node) bool {
		mapping := root.Content[0]
		if mapping.Kind != yaml.MappingNode || len(mapping.Content) < 2 {
			return false
		}
		valueNode := mapping.Content[1]
		if valueNode.Kind != yaml.MappingNode {
			return false
		}
		path := node.yamlPath
		if !applyToggleAt(valueNode, path[:len(path)-1], path[len(path)-1], checked, ctx, node.depth == 0) {
			return false
		}
		pruneEmptyMappings(valueNode)
		reorderNestedMappingKeys(valueNode, ctx.childDefs)
		return true
	})
}

// applyToggleAt navigates start through navPath (creating mappings as needed),
// then adds or removes leafName at that location. asStruct=true uses the
// snippet as a full subtree (for depth-0 struct fields); asStruct=false treats
// the snippet as a scalar value.
func applyToggleAt(start *yaml.Node, navPath []string, leafName string, checked bool, ctx toggleCtx, asStruct bool) bool {
	cur := start
	for _, k := range navPath {
		cur = findOrCreateMappingChild(cur, k)
		if cur == nil {
			return false
		}
	}
	snippet := ctx.snippets[leafName]
	switch {
	case !checked:
		removeMappingKey(cur, leafName)
	case hasMappingKey(cur, leafName):
		// already present — keep as is
	case asStruct:
		if snippet == "" || !appendFieldFromSnippet(cur, ctx.key, snippet) {
			appendLeafToMapping(cur, leafName, "")
		}
	default:
		appendLeafToMapping(cur, leafName, snippet)
	}
	return true
}

// childByKey returns the value node mapped to key in a MappingNode, or nil.
func childByKey(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// extractSubBlock pulls the nested collection at path out of a parent block's
// editor value and returns it as a standalone "<key>:\n  ...". parentYAML is the
// parent block's value ("<blockKey>:\n  ..."); path is relative to the block
// value (e.g. ["httproutes"] or ["sub", "mymap"]). It returns "<key>:\n" when
// the path is absent or empty so the child opens as a fresh collection.
func extractSubBlock(parentYAML string, path []string) string {
	if len(path) == 0 {
		return parentYAML
	}
	key := path[len(path)-1]
	empty := key + ":\n"

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(parentYAML), &root); err != nil || len(root.Content) == 0 {
		return empty
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode || len(doc.Content) < 2 {
		return empty
	}
	cur := doc.Content[1] // parent block's value mapping
	for _, k := range path {
		cur = childByKey(cur, k)
		if cur == nil {
			return empty
		}
	}
	if cur.Kind != yaml.MappingNode && cur.Kind != yaml.SequenceNode {
		return empty // null/scalar placeholder — treat as empty collection
	}

	wrapper := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: key},
		cur,
	}}
	forceBlockStyle(wrapper)
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(wrapper); err != nil {
		return empty
	}
	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// replaceSubBlock writes childSnippet (a standalone "<key>:\n  ...") back into
// parentYAML at path, creating intermediate mappings as needed. An empty or null
// child value removes the key instead, so deleting every entry prunes the block.
func replaceSubBlock(parentYAML string, path []string, childSnippet string) string {
	if len(path) == 0 {
		return parentYAML
	}
	key := path[len(path)-1]

	var childVal *yaml.Node
	var childRoot yaml.Node
	if err := yaml.Unmarshal([]byte(childSnippet), &childRoot); err == nil && len(childRoot.Content) > 0 {
		if cm := childRoot.Content[0]; cm.Kind == yaml.MappingNode && len(cm.Content) >= 2 {
			childVal = cm.Content[1]
		}
	}
	emptyChild := childVal == nil ||
		(childVal.Kind == yaml.MappingNode && len(childVal.Content) == 0) ||
		(childVal.Kind == yaml.SequenceNode && len(childVal.Content) == 0) ||
		(childVal.Kind == yaml.ScalarNode && (childVal.Tag == "!!null" || childVal.Value == ""))

	return withYAMLRoot(parentYAML, func(root *yaml.Node) bool {
		doc := root.Content[0]
		if doc.Kind != yaml.MappingNode || len(doc.Content) < 2 {
			return false
		}
		cur := doc.Content[1] // parent block's value mapping
		if cur.Kind != yaml.MappingNode {
			// Empty/null block value (e.g. "containerengine:\n") — coerce it into
			// an empty mapping so the nested key can be attached.
			cur.Kind = yaml.MappingNode
			cur.Tag = ""
			cur.Value = ""
			cur.Content = nil
		}
		for _, k := range path[:len(path)-1] {
			cur = findOrCreateMappingChild(cur, k)
			if cur == nil {
				return false
			}
		}
		if emptyChild {
			removeMappingKey(cur, key)
			return true
		}
		for i := 0; i+1 < len(cur.Content); i += 2 {
			if cur.Content[i].Value == key {
				cur.Content[i+1] = childVal
				return true
			}
		}
		cur.Content = append(cur.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, childVal)
		return true
	})
}

// withYAMLRoot parses current as a YAML node, calls fn on it, and re-encodes.
// Returns current unchanged on any parse/encode error or when fn returns false.
func withYAMLRoot(current string, fn func(root *yaml.Node) bool) string {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(current), &root); err != nil || root.Kind == 0 || len(root.Content) == 0 {
		return current
	}
	if !fn(&root) {
		return current
	}
	forceBlockStyle(&root)
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return current
	}
	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// forceBlockStyle clears flow style from every mapping/sequence node so the
// re-encoded YAML is block-style (one field per line). Without this, a value
// that was ever flow ("{}" or "{a: b}") stays inline through later edits.
func forceBlockStyle(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		n.Style &^= yaml.FlowStyle
	}
	for _, c := range n.Content {
		forceBlockStyle(c)
	}
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
		// snippet is a "key: value" line (possibly indented). Parse it directly
		// and lift the value — prepending key+":\n" would nest it under itself.
		var tmp map[string]any
		if err := yaml.Unmarshal([]byte(snippet), &tmp); err == nil {
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
