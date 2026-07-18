// Package document provides primitives for editing YAML files structured as
// a flat mapping of top-level keys ("blocks"). It is schema-agnostic - the
// caller supplies the canonical key order when needed for ordered inserts.
package document

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
// Multi-document input and flow-style root mappings are rejected: their line
// ranges cannot be edited block-wise without corrupting the file. An explicit
// empty document ("---") is treated like an empty file.
func ParseBlocks(raw []byte) ([]Block, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}
	var second yaml.Node
	if err := dec.Decode(&second); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("multi-document YAML is not supported")
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	mapping := doc.Content[0]
	if mapping.Kind == yaml.ScalarNode && mapping.Tag == "!!null" {
		return nil, nil
	}
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected mapping at root, got kind %d", mapping.Kind)
	}
	if mapping.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("flow-style root mapping is not supported")
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
		end := totalLines
		if i+1 < len(blocks) {
			end = blocks[i+1].Line - 1
		}
		// Stop this block's range before the trailing blank lines and root-level
		// comments - by convention those belong to the next block (a comment
		// directly above a key documents that key) or, after the last block, to
		// the file tail (trailing newline and trailing comments survive edits).
		// Only lines that are empty or start with '#' at column 0 are reassigned:
		// an indented comment-looking line may be content inside a literal/folded
		// scalar and stays with its block. Limitation: the significant trailing
		// blank lines of a '|+' (keep-chomping) scalar are still attributed to
		// the tail; they are preserved in place by Replace/Remove but omitted
		// from BlockContent.
		for end > blocks[i].Line {
			line := lines[end-1] // end is 1-based
			if line == "" || strings.HasPrefix(line, "#") {
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
