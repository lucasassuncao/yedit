// Package document provides primitives for editing YAML files structured as
// a flat mapping of top-level keys ("blocks"). It is schema-agnostic — the
// caller supplies the canonical key order when needed for ordered inserts.
package document

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Block represents a top-level YAML key with its line range (1-based).
type Block struct {
	Key     string
	Line    int // line of the key node
	EndLine int // last line occupied by this block (exclusive of next key)
}

// ParseBlocks parses raw YAML bytes and returns top-level blocks.
func ParseBlocks(raw []byte) ([]Block, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping at root, got kind %d", mapping.Kind)
	}

	totalLines := bytes.Count(raw, []byte("\n")) + 1
	lines := strings.Split(string(raw), "\n")
	blocks := make([]Block, 0, len(mapping.Content)/2)

	for i := 0; i < len(mapping.Content)-1; i += 2 {
		keyNode := mapping.Content[i]
		blocks = append(blocks, Block{
			Key:  keyNode.Value,
			Line: keyNode.Line,
		})
	}

	for i := range blocks {
		if i+1 >= len(blocks) {
			blocks[i].EndLine = totalLines
			continue
		}
		// Stop this block's range before the blank lines and comments that precede
		// the next key — by convention those belong to the next block (a comment
		// directly above a key documents that key). Otherwise removing or replacing
		// a block would silently delete the next block's leading comments.
		end := blocks[i+1].Line - 1
		for end > blocks[i].Line {
			trimmed := strings.TrimSpace(lines[end-1]) // end is 1-based
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				end--
			} else {
				break
			}
		}
		blocks[i].EndLine = end
	}

	return blocks, nil
}

// ValidateSnippet returns an error if the YAML text is not parseable.
func ValidateSnippet(text string) error {
	var check any
	return yaml.Unmarshal([]byte(text), &check)
}
