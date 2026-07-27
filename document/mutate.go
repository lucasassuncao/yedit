package document

import (
	"bytes"
	"fmt"
	"strings"
)

// BlockContent returns the raw lines for a given block key. The content always
// ends with a single trailing newline: a literal scalar parsed without its final
// line break would silently lose it.
func BlockContent(raw []byte, blocks []Block, key string) (string, error) {
	lines := strings.Split(string(raw), "\n")
	return blockContentFromLines(lines, blocks, key)
}

func blockContentFromLines(lines []string, blocks []Block, key string) (string, error) {
	for _, b := range blocks {
		if b.Key == key {
			start := b.Line - 1
			end := b.EndLine
			start, end = clampRange(start, end, len(lines))
			return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n") + "\n", nil
		}
	}
	return "", fmt.Errorf("key %q not found", key)
}

// ReplaceBlock substitutes the lines belonging to key with snippet, in place.
// Unlike RemoveBlock+InsertBlock, only the block's own line range changes.
func ReplaceBlock(raw []byte, blocks []Block, key, snippet string) ([]byte, error) {
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
	start, end = clampRange(start, end, len(lines))

	snippet = strings.ReplaceAll(snippet, "\r\n", "\n")
	snippetLines := strings.Split(strings.TrimRight(snippet, "\n"), "\n")
	merged := make([]string, 0, len(lines)-(end-start)+len(snippetLines))
	merged = append(merged, lines[:start]...)
	merged = append(merged, snippetLines...)
	merged = append(merged, lines[end:]...)
	return []byte(strings.Join(merged, "\n")), nil
}

// RemoveBlock deletes the lines belonging to key from raw YAML bytes, together
// with the comment lines that document it (see leadingCommentStart).
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
	start, end = clampRange(start, end, len(lines))
	start = leadingCommentStart(lines, start)
	lines = append(lines[:start:start], lines[end:]...)
	return []byte(strings.Join(lines, "\n")), nil
}

// leadingCommentStart walks back from keyIdx over the comment lines documenting
// the block and returns the index removal should start at. Mirrors ParseBlocks'
// convention: only '#' at column 0 qualifies, since an indented comment-looking
// line may be content inside a literal scalar.
//
// A blank line ends the run, so a section header separated from the key by an
// empty line survives the removal:
//
//	# ---- Network ----   <- kept (blank line below)
//
//	# listen port         <- removed with the block
//	port: 8080
//
// Limitation: a header glued directly to the first key with no blank line is
// indistinguishable from that key's own comment and is removed with it.
func leadingCommentStart(lines []string, keyIdx int) int {
	start := keyIdx
	for start > 0 && strings.HasPrefix(lines[start-1], "#") {
		start--
	}
	return start
}

// clampRange guarantees 0 <= start <= end <= n so slicing never panics on blocks
// whose recorded line numbers are stale or inverted.
func clampRange(start, end, n int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start > end {
		start = end
	}
	return start, end
}

// InsertBlock places snippet before the first existing block whose key follows
// the new key in knownOrder. A key unknown to knownOrder, or the absence of a
// later block, appends at the end.
func InsertBlock(raw []byte, snippet string, knownOrder []string) ([]byte, error) {
	// Collapse trailing blank lines to a single newline so neither the append nor
	// the ordered path wedges a blank line between blocks.
	snippet = strings.ReplaceAll(snippet, "\r\n", "\n")
	snippet = strings.TrimRight(snippet, "\n") + "\n"
	snippetBlocks, err := ParseBlocks([]byte(snippet))
	if err != nil {
		return nil, err
	}
	blocks, blocksErr := ParseBlocks(raw)
	if blocksErr == nil {
		// A duplicate key would produce an invalid document or a misleading
		// round-trip failure, so reject it up front.
		for _, sb := range snippetBlocks {
			for _, b := range blocks {
				if b.Key == sb.Key {
					return nil, fmt.Errorf("key %q already exists", sb.Key)
				}
			}
		}
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

	if blocksErr != nil || len(blocks) == 0 {
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
	// Land above the following block's leading comments, not between them and the
	// key they document. A blank line separates comment groups and stops the walk.
	for idx > 0 {
		if strings.HasPrefix(strings.TrimSpace(lines[idx-1]), "#") {
			idx--
		} else {
			break
		}
	}
	snippetLines := strings.Split(snippet, "\n")
	for len(snippetLines) > 0 && snippetLines[len(snippetLines)-1] == "" {
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
	// Build a fresh slice; appending onto trimmed would alias raw's backing array.
	out := make([]byte, 0, len(trimmed)+1+len(snippet))
	out = append(out, trimmed...)
	out = append(out, '\n')
	out = append(out, snippet...)
	return out
}
