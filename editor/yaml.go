package editor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lucasassuncao/yedit/yamlnode"

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
		// Use leafName (not ctx.key) as the parent key so the snippet is parsed
		// as "leafName:\n  <snippet>" — the correct wrapping for depth-0 struct fields.
		if snippet == "" || !appendFieldFromSnippet(cur, leafName, snippet) {
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
//
// isMapEntry marks a key that is a runtime map-entry key (user data, e.g. the
// "web" in httproutes.web) rather than a schema field name. Both navigate the
// node tree the same way; the distinction matters only for schema/metadata
// lookups, which must see field names exclusively.
type pathSeg struct {
	key        string
	idx        int
	isIndex    bool
	isMapEntry bool
}

func segKey(k string) pathSeg    { return pathSeg{key: k} }
func segMapKey(k string) pathSeg { return pathSeg{key: k, isMapEntry: true} }
func segIdx(i int) pathSeg       { return pathSeg{idx: i, isIndex: true} }

// focusToStringPath converts a focus path to a slice of key strings, dropping
// index segments and runtime map-entry keys - neither is a schema field name,
// at any nesting depth. Used to build metadata lookup prefixes for nested editors.
func focusToStringPath(focus []pathSeg) []string {
	out := make([]string, 0, len(focus))
	for _, s := range focus {
		if !s.isIndex && !s.isMapEntry {
			out = append(out, s.key)
		}
	}
	return out
}

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

// parseBlockText parses the block editor's buffer, which must contain exactly
// one top-level key equal to key, and returns that key's value node. The
// returned message is empty on success and user-facing on failure. Unlike
// valueNodeOfSnippet - which silently returns the first key's value - this is
// the strict variant for commit paths, where a stray second key or a renamed
// key would otherwise be dropped without warning.
func parseBlockText(key, text string) (*yaml.Node, string) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(text), &root); err != nil {
		return nil, fmt.Sprintf("Invalid YAML: %v", err)
	}
	missingHeader := fmt.Sprintf("Missing %q header - the editor must contain the block's top-level key.", key+":")
	if root.Kind == 0 || len(root.Content) == 0 {
		return nil, missingHeader
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode || len(doc.Content) < 2 {
		return nil, missingHeader
	}
	if len(doc.Content) > 2 {
		return nil, fmt.Sprintf("Only one top-level key is allowed here - remove %q or commit it as its own block.", doc.Content[2].Value)
	}
	if got := doc.Content[0].Value; got != key {
		return nil, fmt.Sprintf("Top-level key must be %q (found %q) - renaming it here is not supported.", key, got)
	}
	return doc.Content[1], ""
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

// coerceToMapping coerces a null scalar node (kind==Scalar, value=="") to a
// MappingNode so path traversal can continue through it. Returns false when
// the node is some other non-mapping kind.
func coerceToMapping(n *yaml.Node) bool {
	if n.Kind == yaml.MappingNode {
		return true
	}
	if n.Kind == yaml.ScalarNode && n.Value == "" {
		n.Kind = yaml.MappingNode
		n.Tag = ""
		n.Value = ""
		n.Content = nil
		return true
	}
	return false
}

// advanceSeg advances parent by one path segment, creating intermediate mapping
// keys as needed. Returns a non-nil error when the segment cannot be traversed.
func advanceSeg(parent *yaml.Node, s pathSeg) (*yaml.Node, error) {
	if s.isIndex {
		if parent.Kind != yaml.SequenceNode || s.idx < 0 || s.idx >= len(parent.Content) {
			return nil, fmt.Errorf("sequence index %d out of range (kind=%v, len=%d)", s.idx, parent.Kind, len(parent.Content))
		}
		return parent.Content[s.idx], nil
	}
	if !coerceToMapping(parent) {
		return nil, fmt.Errorf("cannot traverse key %q: node is not a mapping (kind=%v)", s.key, parent.Kind)
	}
	child := yamlnode.ChildByKey(parent, s.key)
	switch {
	case child == nil:
		child = &yaml.Node{Kind: yaml.MappingNode}
		parent.Content = append(parent.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: s.key}, child)
	case child.Kind == yaml.ScalarNode && child.Value != "":
		// Non-null scalar: cannot be traversed by any subsequent segment.
		return nil, fmt.Errorf("cannot traverse key %q: value is a non-null scalar %q", s.key, child.Value)
	default:
		coerceToMapping(child) // coerces null scalar; no-op for sequences and mappings
	}
	return child, nil
}

// setNodeAt replaces the node addressed by segs within root with newVal,
// creating intermediate mapping keys as needed. Returns an error when a sequence
// index is out of range or an intermediate node has a conflicting kind. This is
// structurally safe: it operates on live nodes, so it can never turn a sequence
// into a mapping the way string splicing could.
func setNodeAt(root *yaml.Node, segs []pathSeg, newVal *yaml.Node) error {
	if len(segs) == 0 {
		*root = *yamlnode.CloneNode(newVal)
		return nil
	}
	parent := root
	for _, s := range segs[:len(segs)-1] {
		next, err := advanceSeg(parent, s)
		if err != nil {
			return err
		}
		parent = next
	}
	last := segs[len(segs)-1]
	if last.isIndex {
		if parent.Kind != yaml.SequenceNode || last.idx < 0 || last.idx >= len(parent.Content) {
			return fmt.Errorf("sequence index %d out of range (kind=%v, len=%d)", last.idx, parent.Kind, len(parent.Content))
		}
		parent.Content[last.idx] = newVal
		return nil
	}
	if !coerceToMapping(parent) {
		return fmt.Errorf("cannot set key %q: node is not a mapping (kind=%v)", last.key, parent.Kind)
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == last.key {
			parent.Content[i+1] = newVal
			return nil
		}
	}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: last.key}, newVal)
	return nil
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
			// Guard on ScalarNode explicitly: sequences also have Value=="" but
			// must not be turned into mappings.
			if child.Kind == yaml.ScalarNode && child.Value == "" {
				child.Kind = yaml.MappingNode
				child.Tag = ""
				child.Value = ""
				child.Content = nil
			}
			// Non-empty scalars are returned unchanged; applyToggleAt returns false
			// when it tries to navigate further into them, which is the correct behavior.
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
// ({}), empty sequence ([]), or null scalar (key with no value, Tag=="!!null"),
// and removes empty mapping items from sequences, recursing into nested nodes
// first so the cleanup propagates upward.
//
// Null scalars arise when drilling into an object field that does not yet exist
// in the YAML: the child editor serializes "<key>:\n" which parses back as a
// null scalar value. Without pruning them here, drilling out would leave the
// parent's YAML with a phantom "<key>:" line even though no content was added.
// Toggle operations use Tag=="" for freshly-added empty values, so they are
// not affected by this check.
func pruneEmptyMappings(node *yaml.Node) {
	pruneEmpty(node,
		func(v *yaml.Node) bool { return emptyCollection(v) || v.Kind == yaml.ScalarNode && v.Tag == "!!null" },
		func(v *yaml.Node) bool { return v.Kind == yaml.MappingNode && len(v.Content) == 0 },
	)
}

// pruneEmptyContent is like pruneEmptyMappings but also removes mapping values
// and sequence items that are null or empty scalars. Called on commit so that
// scaffold fields the user never filled in (e.g. hooks.before.shell: "") are
// stripped before the block is written to the document. Not used after
// individual toggles, where "" is the legitimate placeholder for a just-added
// field.
func pruneEmptyContent(node *yaml.Node) {
	emptyVal := func(v *yaml.Node) bool {
		return emptyCollection(v) || v.Kind == yaml.ScalarNode && (v.Tag == "!!null" || v.Value == "")
	}
	pruneEmpty(node, emptyVal, emptyVal)
}

// emptyCollection reports whether v is a mapping or sequence with no content.
func emptyCollection(v *yaml.Node) bool {
	return (v.Kind == yaml.MappingNode || v.Kind == yaml.SequenceNode) && len(v.Content) == 0
}

// pruneEmpty is the shared depth-first traversal behind pruneEmptyMappings and
// pruneEmptyContent: it recurses into nested nodes first (so the cleanup
// propagates upward), then removes mapping pairs whose value emptyPair reports
// as empty and sequence items emptyItem reports as empty.
func pruneEmpty(node *yaml.Node, emptyPair, emptyItem func(*yaml.Node) bool) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 1; i < len(node.Content); i += 2 {
			pruneEmpty(node.Content[i], emptyPair, emptyItem)
		}
		i := 0
		for i < len(node.Content)-1 {
			if emptyPair(node.Content[i+1]) {
				node.Content = append(node.Content[:i], node.Content[i+2:]...)
			} else {
				i += 2
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			pruneEmpty(item, emptyPair, emptyItem)
		}
		i := 0
		for i < len(node.Content) {
			if emptyItem(node.Content[i]) {
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

// appendFieldFromSnippet parses snippet under parentKey and appends the
// (parentKey, value) key-value pair to valueNode, so the field is correctly
// nested under parentKey rather than flattened as siblings.  Returns false if
// the snippet is malformed, the value has no fields, or the key already exists
// in valueNode (duplicate-key guard — callers should check hasMappingKey first
// when they want an update-or-insert semantic instead).
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
	// Guard against duplicate keys: if parentKey already exists in valueNode,
	// do not append a second copy.
	if hasMappingKey(valueNode, parentKey) {
		return false
	}
	// Append the (parentKey, value) pair — nesting the snippet under parentKey
	// rather than splicing its children directly into valueNode.
	valueNode.Content = append(valueNode.Content, tMapping.Content[0], tMapping.Content[1])
	return true
}
