package yamledit

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// FindDuplicateMappingKey returns the dotted path of the first key repeated
// within one mapping. schema.UnknownKeys cannot see duplicates (yaml.v3 keeps
// the last value), so commit uses this as its final gate.
func FindDuplicateMappingKey(n *yaml.Node) (string, bool) {
	return findDupKeyAt(n, nil)
}

func findDupKeyAt(n *yaml.Node, path []string) (string, bool) {
	if n == nil {
		return "", false
	}
	switch n.Kind {
	case yaml.MappingNode:
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			if seen[key] {
				return strings.Join(append(append([]string{}, path...), key), "."), true
			}
			seen[key] = true
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			child := append(append([]string{}, path...), n.Content[i].Value)
			if p, ok := findDupKeyAt(n.Content[i+1], child); ok {
				return p, true
			}
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, c := range n.Content {
			if p, ok := findDupKeyAt(c, path); ok {
				return p, true
			}
		}
	}
	return "", false
}

// NodeHasContent reports whether a value node carries real content: a null
// scalar, empty string, or empty list/map counts as empty.
func NodeHasContent(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Tag != "!!null" && n.Value != ""
	case yaml.SequenceNode, yaml.MappingNode:
		return len(n.Content) > 0
	case yaml.AliasNode:
		return true
	default:
		return false
	}
}

// BlockValueNode parses "<key>:\n  ..." into the value node mapped to key, the
// canonical node for a block editor. Empty or unparseable content yields an
// empty mapping, so a fresh block always has a writable node.
func BlockValueNode(content string) *yaml.Node {
	if v := ValueNodeOfSnippet(content); v != nil {
		return v
	}
	return &yaml.Node{Kind: yaml.MappingNode}
}

// BlockValueNodeOrNil is BlockValueNode but returns nil when non-empty content
// fails to parse, so callers can tell "empty block" from "corrupt content" and
// surface an error instead of masking it.
func BlockValueNodeOrNil(content string) *yaml.Node {
	if strings.TrimSpace(content) == "" {
		return &yaml.Node{Kind: yaml.MappingNode}
	}
	return ValueNodeOfSnippet(content)
}
