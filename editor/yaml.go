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
	snippets  func(string) string
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
	var snippet string
	if ctx.snippets != nil {
		snippet = ctx.snippets(leafName)
	}
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
		// Try to append as a structured field first (for complex snippets like arrays/maps).
		// Fall back to a simple scalar if the snippet is empty or not a valid structure.
		if snippet != "" && appendFieldFromSnippet(cur, leafName, snippet) {
			// Successfully appended the complex structure
		} else {
			appendLeafToMapping(cur, leafName, "")
		}
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

// pathSeg is one step in a focus path through a YAML node tree: either a mapping
// key (isIndex == false) or a sequence index (isIndex == true). A focus path is
// the canonical, unambiguous address of a node. Rooted at the filters sequence,
// filters[0].any[1] is [segIdx(0), segKey("any"), segIdx(1)].
type pathSeg struct {
	key     string
	idx     int
	isIndex bool
}

func segKey(k string) pathSeg { return pathSeg{key: k} }
func segIdx(i int) pathSeg    { return pathSeg{idx: i, isIndex: true} }

// nodeToContent serializes a value node as a standalone "<key>:\n  ..." block,
// forcing block style so the result renders one field per line. Returns
// "<key>:\n" when encoding fails. This is the inverse of valueNodeOfSnippet.
func nodeToContent(key string, value *yaml.Node) string {
	wrapper := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: key},
		value,
	}}
	forceBlockStyle(wrapper)
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(wrapper); err != nil {
		return key + ":\n"
	}
	return strings.TrimRight(buf.String(), "\n") + "\n"
}

// valueNodeOfSnippet parses a standalone "<key>:\n  ..." block and returns the
// value node mapped to that key (the inverse of nodeToContent), or nil on a
// parse error or unexpected shape. The returned node is detached and safe to
// splice into another tree via setNodeAt.
func valueNodeOfSnippet(snippet string) *yaml.Node {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(snippet), &root); err != nil || len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode || len(doc.Content) < 2 {
		return nil
	}
	return doc.Content[1]
}

// nodeAt returns the node reached by following segs from node, or nil when any
// step fails to resolve (wrong kind, missing key, index out of range). It never
// descends implicitly — every step is explicit, so it can address a sequence
// node itself as well as an element inside it.
func nodeAt(node *yaml.Node, segs []pathSeg) *yaml.Node {
	for _, s := range segs {
		if node == nil {
			return nil
		}
		if s.isIndex {
			if node.Kind != yaml.SequenceNode || s.idx < 0 || s.idx >= len(node.Content) {
				return nil
			}
			node = node.Content[s.idx]
		} else {
			if node.Kind != yaml.MappingNode {
				return nil
			}
			node = childByKey(node, s.key)
		}
	}
	return node
}

// setNodeAt replaces the node addressed by segs within root with newVal,
// creating intermediate mapping keys as needed. Returns false when a sequence
// index is out of range or an intermediate node has a conflicting kind. This is
// structurally safe: it operates on live nodes, so it can never turn a sequence
// into a mapping the way string splicing could.
func setNodeAt(root *yaml.Node, segs []pathSeg, newVal *yaml.Node) bool {
	if len(segs) == 0 {
		*root = *cloneNode(newVal)
		return true
	}
	parent := root
	for _, s := range segs[:len(segs)-1] {
		if s.isIndex {
			if parent.Kind != yaml.SequenceNode || s.idx < 0 || s.idx >= len(parent.Content) {
				return false
			}
			parent = parent.Content[s.idx]
		} else {
			if parent.Kind != yaml.MappingNode {
				return false
			}
			child := childByKey(parent, s.key)
			if child == nil {
				child = &yaml.Node{Kind: yaml.MappingNode}
				parent.Content = append(parent.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: s.key}, child)
			}
			parent = child
		}
	}
	last := segs[len(segs)-1]
	if last.isIndex {
		if parent.Kind != yaml.SequenceNode || last.idx < 0 || last.idx >= len(parent.Content) {
			return false
		}
		parent.Content[last.idx] = newVal
		return true
	}
	if parent.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == last.key {
			parent.Content[i+1] = newVal
			return true
		}
	}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: last.key}, newVal)
	return true
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

// forceBlockStyle clears flow style from mapping nodes so the re-encoded YAML
// is block-style (one field per line). Without this, a mapping that was ever
// flow ("{}" or "{a: b}") stays inline through later edits.
// Sequence nodes are intentionally left untouched: flow sequences on leaf fields
// (e.g. extensions: ["pdf", "txt"]) are an accepted style and must be preserved.
func forceBlockStyle(n *yaml.Node) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
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
			child := mapping.Content[i+1]
			// An existing key with a null/empty value (e.g. "source:\n") parses as
			// a scalar node, not a mapping. Coerce it so children can be added —
			// without this, appendLeafToMapping silently no-ops on the scalar.
			if child.Kind != yaml.MappingNode && child.Value == "" {
				child.Kind = yaml.MappingNode
				child.Tag = ""
				child.Value = ""
				child.Content = nil
			}
			return child
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
	if v := snippetValueNode(snippet, key); v != nil {
		valNode = v
	}
	mapping.Content = append(mapping.Content, keyNode, valNode)
}

// snippetValueNode parses snippet as YAML and returns the value node for key,
// preserving its type tag (!!bool, !!int, etc.). Returns nil when the snippet
// is empty, unparseable, or does not contain key.
func snippetValueNode(snippet, key string) *yaml.Node {
	if snippet == "" {
		return nil
	}
	var tmp yaml.Node
	if err := yaml.Unmarshal([]byte(snippet), &tmp); err != nil || len(tmp.Content) == 0 {
		return nil
	}
	m := tmp.Content[0]
	if m.Kind != yaml.MappingNode {
		return nil
	}
	return childByKey(m, key)
}

// appendFieldFromSnippet parses snippet under parentKey, extracts all child
// key/value pairs from the snippet's struct value, and appends them to valueNode.
// Returns false if the snippet is malformed or has no fields.
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
	valueNode.Content = append(valueNode.Content, tValue.Content...)
	return true
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
