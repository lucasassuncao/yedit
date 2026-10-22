package document_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/document"
)

func TestParseBlocks(t *testing.T) {
	blocks, err := document.ParseBlocks([]byte(sampleYAML))
	if err != nil {
		t.Fatalf("ParseBlocks: %v", err)
	}
	if len(blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(blocks))
	}
	if blocks[0].Key != "name" {
		t.Errorf("first block key = %q, want \"name\"", blocks[0].Key)
	}
	if blocks[2].Key != "features" {
		t.Errorf("third block key = %q, want \"features\"", blocks[2].Key)
	}
}

func TestParseBlocks_empty(t *testing.T) {
	blocks, err := document.ParseBlocks(nil)
	if err != nil {
		t.Fatalf("ParseBlocks(nil): %v", err)
	}
	if blocks != nil {
		t.Errorf("expected nil blocks for empty input, got %v", blocks)
	}
}

// YAML edge cases: anchors, multi-document, tab indentation.

func TestParseBlocks_withAnchors(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	// Anchors must not crash ParseBlocks; the resolved content is what matters.
	raw := "base: &anchor\n  value: x\nderived:\n  <<: *anchor\n"
	blocks, err := document.ParseBlocks([]byte(raw))
	must.NoError(err, "ParseBlocks with anchors")
	must.Len(blocks, 2, "expected 2 blocks")
	is.Equal("base", blocks[0].Key)
	is.Equal("derived", blocks[1].Key)
}

func TestParseBlocks_multiDocument(t *testing.T) {
	is := assert.New(t)
	// Block ranges span the whole file but only the first document is decoded, so
	// mutations would destroy the rest. Must be rejected with a clear error.
	raw := "key1: val1\n---\nkey2: val2\n"
	_, err := document.ParseBlocks([]byte(raw))
	is.ErrorContains(err, "multi-document", "ParseBlocks must reject multi-document YAML")
}

// A comment-looking line indented inside a literal scalar is content and must
// stay with its block; trimming it truncated the scalar and made Replace fail
// its round-trip check with a bogus error.
func TestParseBlocks_indentedCommentInLiteralScalar(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := document.New([]byte("a: |\n  line1\n  # not a comment\nb: 2\n"), nil)
	must.NoError(err)

	content, err := doc.BlockContent("a")
	must.NoError(err, "BlockContent")
	is.Equal("a: |\n  line1\n  # not a comment\n", content, "indented comment-looking line must belong to the block")

	doc, err = doc.Replace("a", "a: |\n  line1\n  # still not a comment\n")
	must.NoError(err, "Replace of a literal-scalar block must round-trip")
	is.Equal("a: |\n  line1\n  # still not a comment\nb: 2\n", string(doc.Raw()))
}

// TestParseBlocks_rootLevelTrailingCommentStillTrimmed pins the counterpart of
// the indented-comment fix: a column-0 comment above the next key still
// belongs to the next block.
func TestParseBlocks_rootLevelTrailingCommentStillTrimmed(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	blocks, err := document.ParseBlocks([]byte("a: 1\n\n# documents b\nb: 2\n"))
	must.NoError(err)
	must.Len(blocks, 2)
	is.Equal(1, blocks[0].EndLine, "blank line and root-level comment belong to the next block")
}

// TestParseBlocks_flowRootRejected: a flow-style root mapping has no per-block
// line ranges, so block mutations would corrupt it. It must be rejected.
func TestParseBlocks_flowRootRejected(t *testing.T) {
	is := assert.New(t)
	_, err := document.ParseBlocks([]byte("{a: 1, b: 2}\n"))
	is.ErrorContains(err, "flow-style", "ParseBlocks must reject a flow-style root mapping")
}

// TestParseBlocks_explicitEmptyDocument: "---" is an explicit null document
// and must load like an empty file, not fail with "expected mapping at root".
func TestParseBlocks_explicitEmptyDocument(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	blocks, err := document.ParseBlocks([]byte("---\n"))
	must.NoError(err, "explicit empty document must parse")
	is.Empty(blocks)
}

func TestParseBlocks_tabIndented(t *testing.T) {
	is := assert.New(t)
	// Tab-indented YAML is invalid; ParseBlocks should return an error, not panic.
	raw := "key:\n\tvalue: x\n"
	_, err := document.ParseBlocks([]byte(raw))
	is.Error(err, "expected error for tab-indented YAML")
}

func TestValidateSnippet(t *testing.T) {
	if err := document.ValidateSnippet("remoteUser: vscode\n"); err != nil {
		t.Errorf("expected valid snippet to pass: %v", err)
	}

	if err := document.ValidateSnippet("remoteUser: :\n  broken:"); err == nil {
		t.Error("expected invalid snippet to fail validation")
	}
}
