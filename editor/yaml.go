package editor

import (
	"github.com/lucasassuncao/yedit/internal/yamlnode"
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
		// already present - keep as is
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

// normalizeBlockContent parses a raw block snippet and re-serializes it through
// nodeToContent so the result can be compared against another nodeToContent
// output without false mismatches from formatting differences. Returns raw
// unchanged if the snippet cannot be parsed.
func normalizeBlockContent(key, raw string) string {
	val := valueNodeOfSnippet(raw)
	if val == nil {
		return raw
	}
	return nodeToContent(key, val)
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
// descends implicitly - every step is explicit, so it can address a sequence
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
			node = yamlnode.ChildByKey(node, s.key)
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
		*root = *yamlnode.CloneNode(newVal)
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
			child := yamlnode.ChildByKey(parent, s.key)
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
			// a scalar node, not a mapping. Coerce it so children can be added -
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
// ({}) or empty sequence ([]), and removes empty mapping items from sequences,
// recursing into nested nodes first so the cleanup propagates upward.
func pruneEmptyMappings(node *yaml.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 1; i < len(node.Content); i += 2 {
			pruneEmptyMappings(node.Content[i])
		}
		i := 0
		for i < len(node.Content)-1 {
			val := node.Content[i+1]
			empty := (val.Kind == yaml.MappingNode || val.Kind == yaml.SequenceNode) && len(val.Content) == 0
			if empty {
				node.Content = append(node.Content[:i], node.Content[i+2:]...)
			} else {
				i += 2
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			pruneEmptyMappings(item)
		}
		i := 0
		for i < len(node.Content) {
			item := node.Content[i]
			if item.Kind == yaml.MappingNode && len(item.Content) == 0 {
				node.Content = append(node.Content[:i], node.Content[i+1:]...)
			} else {
				i++
			}
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
	return yamlnode.ChildByKey(m, key)
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
