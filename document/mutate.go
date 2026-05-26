package document

import (
	"bytes"
	"fmt"
	"strings"
)

// BlockContent returns the raw lines for a given block key.
func BlockContent(raw []byte, blocks []Block, key string) (string, error) {
	lines := strings.Split(string(raw), "\n")
	return blockContentFromLines(lines, blocks, key)
}

func blockContentFromLines(lines []string, blocks []Block, key string) (string, error) {
	for _, b := range blocks {
		if b.Key == key {
			start := b.Line - 1
			end := b.EndLine
			if end > len(lines) {
				end = len(lines)
			}
			return strings.Join(lines[start:end], "\n"), nil
		}
	}
	return "", fmt.Errorf("key %q not found", key)
}

// RemoveBlock deletes the lines belonging to key from raw YAML bytes.
func RemoveBlock(raw []byte, blocks []Block, key string) ([]byte, error) {
	var target *Block
	for i := range blocks {
		if blocks[i].Key == key {
			target = &blocks[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("key %q not found in blocks", key)
	}

	lines := strings.Split(string(raw), "\n")
	start := target.Line - 1
	end := target.EndLine // exclusive upper bound (0-based = EndLine)
	lines = append(lines[:start:start], lines[end:]...)
	return []byte(strings.Join(lines, "\n")), nil
}

// InsertBlock inserts a YAML snippet into raw, respecting the canonical key
// order in knownOrder. The snippet is placed before the first existing block
// whose key follows the new key in knownOrder. If the new key is unknown to
// knownOrder, or no later block exists, the snippet is appended at the end.
func InsertBlock(raw []byte, snippet string, knownOrder []string) ([]byte, error) {
	snippetBlocks, err := ParseBlocks([]byte(snippet))
	if err != nil {
		return nil, err
	}
	if len(snippetBlocks) == 0 {
		return appendBlock(raw, snippet), nil
	}
	newKey := snippetBlocks[0].Key

	rank := make(map[string]int, len(knownOrder))
	for i, k := range knownOrder {
		rank[k] = i
	}
	newRank, known := rank[newKey]
	if !known {
		return appendBlock(raw, snippet), nil
	}

	blocks, err := ParseBlocks(raw)
	if err != nil || len(blocks) == 0 {
		return appendBlock(raw, snippet), nil
	}

	insertBeforeLine := -1
	for _, b := range blocks {
		if r, ok := rank[b.Key]; ok && r > newRank {
			insertBeforeLine = b.Line
			break
		}
	}

	if insertBeforeLine == -1 {
		return appendBlock(raw, snippet), nil
	}

	lines := strings.Split(string(raw), "\n")
	idx := insertBeforeLine - 1
	snippetLines := strings.Split(snippet, "\n")
	if len(snippetLines) > 0 && snippetLines[len(snippetLines)-1] == "" {
		snippetLines = snippetLines[:len(snippetLines)-1]
	}
	merged := make([]string, 0, len(lines)+len(snippetLines))
	merged = append(merged, lines[:idx]...)
	merged = append(merged, snippetLines...)
	merged = append(merged, lines[idx:]...)
	return []byte(strings.Join(merged, "\n")), nil
}

func appendBlock(raw []byte, snippet string) []byte {
	trimmed := bytes.TrimRight(raw, "\n")
	if len(trimmed) == 0 {
		return []byte(snippet)
	}
	return append(trimmed, append([]byte("\n"), []byte(snippet)...)...)
}
