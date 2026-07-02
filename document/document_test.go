package document

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var canonicalOrder = []string{"name", "image", "forwardPorts", "remoteUser"}

func TestLoad_missing(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "missing.yaml")

	doc, err := Load(path, canonicalOrder)
	must.NoError(err, "Load on missing file")
	is.Empty(doc.Raw())
	is.False(doc.Dirty(), "new document should not be dirty")
	is.Equal(path, doc.Path())
}

func TestLoad_crlf(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "crlf.yaml")
	content := "name: mydev\r\nimage: ubuntu:22.04\r\n"
	must.NoError(os.WriteFile(path, []byte(content), 0o600))

	doc, err := Load(path, canonicalOrder)
	must.NoError(err, "Load")
	is.Equal("name: mydev\nimage: ubuntu:22.04\n", string(doc.Raw()), "CRLF not normalised")
}

func TestDocument_BlockContent(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: mydev\nimage: ubuntu:22.04\n"), canonicalOrder)
	must.NoError(err)
	content, err := doc.BlockContent("image")
	must.NoError(err, "BlockContent")
	is.Equal("image: ubuntu:22.04\n", content)
}

func TestDocument_InsertOrdered(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: mydev\nforwardPorts:\n  - 3000\n"), canonicalOrder)
	must.NoError(err)
	doc, err = doc.Insert("image: ubuntu:22.04\n")
	must.NoError(err, "Insert")
	want := "name: mydev\nimage: ubuntu:22.04\nforwardPorts:\n  - 3000\n"
	is.Equal(want, string(doc.Raw()))
	is.True(doc.Dirty(), "expected dirty=true after Insert")
	is.True(doc.CanUndo(), "expected CanUndo=true after Insert")
}

func TestDocument_RemoveNotFound(t *testing.T) {
	is := assert.New(t)
	doc, _ := New([]byte("name: mydev\n"), canonicalOrder)
	_, err := doc.Remove("image")
	is.Error(err, "expected error removing absent key")
}

func TestDocument_ReplaceRawInvalid(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	original := []byte("name: mydev\nimage: ubuntu:22.04\n")
	doc, err := New(original, canonicalOrder)
	must.NoError(err)

	// Invalid YAML: a mapping with a bare ":" value.
	_, err = doc.ReplaceRaw([]byte("name: :\n  broken:"))
	is.Error(err, "expected error for invalid YAML")
	is.Equal(string(original), string(doc.Raw()), "raw changed despite parse failure")
	is.False(doc.Dirty(), "dirty should not be set on parse failure")
	is.False(doc.CanUndo(), "history should not grow on parse failure")
}

func TestDocument_UndoEmpty(t *testing.T) {
	is := assert.New(t)
	doc, _ := New([]byte("name: mydev\n"), canonicalOrder)
	_, ok := doc.Undo()
	is.False(ok, "Undo on empty history should return false")
}

func TestDocument_UndoRestores(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: mydev\n"), canonicalOrder)
	must.NoError(err)
	doc, err = doc.Insert("image: ubuntu:22.04\n")
	must.NoError(err)
	doc, err = doc.Insert("remoteUser: vscode\n")
	must.NoError(err)

	var ok bool
	doc, ok = doc.Undo()
	must.True(ok, "first Undo failed")
	doc, ok = doc.Undo()
	must.True(ok, "second Undo failed")
	is.Equal("name: mydev\n", string(doc.Raw()), "after two undos")
	is.False(doc.Dirty(), "dirty should be false when Undo restores the loaded raw")
}

func TestDocument_UndoStaysDirtyMidStack(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, _ := New([]byte("name: mydev\n"), canonicalOrder)
	doc, _ = doc.Insert("image: ubuntu:22.04\n")
	doc, _ = doc.Insert("remoteUser: vscode\n")
	var ok bool
	doc, ok = doc.Undo()
	must.True(ok, "undo failed")
	is.True(doc.Dirty(), "dirty should be true after one undo (raw still differs from loaded)")
}

func TestDocument_RedoEmpty(t *testing.T) {
	is := assert.New(t)
	doc, _ := New([]byte("name: mydev\n"), canonicalOrder)
	_, ok := doc.Redo()
	is.False(ok, "Redo with nothing undone should return false")
	is.False(doc.CanRedo(), "CanRedo should be false with nothing undone")
}

func TestDocument_RedoReappliesUndoneChange(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: mydev\n"), canonicalOrder)
	must.NoError(err)
	doc, err = doc.Insert("image: ubuntu:22.04\n")
	must.NoError(err)
	withImage := string(doc.Raw())

	var ok bool
	doc, ok = doc.Undo()
	must.True(ok, "undo failed")
	must.True(doc.CanRedo(), "CanRedo should be true after Undo")
	doc, ok = doc.Redo()
	must.True(ok, "redo failed")
	is.Equal(withImage, string(doc.Raw()), "after redo")
	is.True(doc.Dirty(), "dirty should be true after redo (raw differs from loaded)")

	// The redo itself must be undoable.
	doc, ok = doc.Undo()
	must.True(ok, "undo after redo failed")
	is.Equal("name: mydev\n", string(doc.Raw()), "after undoing the redo")
}

func TestDocument_RedoClearedByNewMutation(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, _ := New([]byte("name: mydev\n"), canonicalOrder)
	doc, _ = doc.Insert("image: ubuntu:22.04\n")
	var ok bool
	doc, ok = doc.Undo()
	must.True(ok, "undo failed")

	// A new mutation forks away from the undone state.
	doc, err := doc.Insert("remoteUser: vscode\n")
	must.NoError(err)
	is.False(doc.CanRedo(), "redo stack should be cleared by a new mutation")
	_, ok = doc.Redo()
	is.False(ok, "Redo after a new mutation should return false")
}

func TestDocument_HistoryCapsAt50(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: mydev\n"), canonicalOrder)
	must.NoError(err)
	// Perform 60 mutations: toggle image on and off, alternating.
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			doc, _ = doc.Insert("image: ubuntu:22.04\n")
		} else {
			doc, _ = doc.Remove("image")
		}
	}
	// Drain undo and count steps.
	count := 0
	var ok bool
	for {
		doc, ok = doc.Undo()
		if !ok {
			break
		}
		count++
		must.LessOrEqual(count, 100, "undo loop did not terminate")
	}
	is.Equal(HistoryLimit, count, "expected HistoryLimit undos available")
}

func TestDocument_ReplaceAtomic(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: mydev\nimage: ubuntu:22.04\n"), canonicalOrder)
	must.NoError(err)
	doc, err = doc.Replace("image", "image: alpine:latest\n")
	must.NoError(err, "Replace")
	is.True(doc.Dirty())

	// A single Undo should restore the original.
	var ok bool
	doc, ok = doc.Undo()
	must.True(ok, "expected Undo to succeed")
	is.Equal("name: mydev\nimage: ubuntu:22.04\n", string(doc.Raw()), "after one Undo")
	// No further undo (single snapshot was recorded by Replace).
	is.False(doc.CanUndo(), "Replace should have recorded exactly one snapshot")
}

func TestDocument_Save(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "out.yaml")
	doc, err := Load(path, canonicalOrder)
	must.NoError(err)
	doc, err = doc.Insert("name: mydev\n")
	must.NoError(err)
	must.True(doc.Dirty(), "doc should be dirty before save")
	doc, err = doc.Save()
	must.NoError(err, "Save")
	is.False(doc.Dirty(), "doc should not be dirty after Save")

	// Verify file written with mode 0600 (POSIX only; Windows ignores POSIX
	// permission bits and creates files with mode 0666 by default).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		must.NoError(err)
		is.Equal(os.FileMode(0o600), info.Mode().Perm(), "file mode")
	}

	data, _ := os.ReadFile(path)
	is.Equal("name: mydev\n", string(data), "file content")
}

func TestDocument_DirtyLifecycle(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "doc.yaml")
	doc, _ := Load(path, canonicalOrder)
	is.False(doc.Dirty(), "freshly loaded missing-file document should not be dirty")
	var err error
	doc, err = doc.Insert("name: mydev\n")
	must.NoError(err)
	is.True(doc.Dirty(), "dirty should be true after Insert")
	doc, err = doc.Save()
	must.NoError(err)
	is.False(doc.Dirty(), "dirty should be false after Save")
}

func TestSave_noPath(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	doc, err := New([]byte("name: mydev\n"), canonicalOrder)
	must.NoError(err)
	_, err = doc.Save()
	is.Error(err, "expected error saving in-memory document")
}

// TestSave_preservesCRLF: a file loaded with CRLF endings is written back with
// CRLF, so editing one block on Windows does not rewrite every line to LF.
func TestSave_preservesCRLF(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "crlf.yaml")
	must.NoError(os.WriteFile(path, []byte("name: web\r\nimage: alpine\r\n"), 0o644))
	doc, err := Load(path, canonicalOrder)
	must.NoError(err)
	doc, err = doc.Replace("image", "image: ubuntu\n")
	must.NoError(err)
	_, err = doc.Save()
	must.NoError(err)
	data, _ := os.ReadFile(path)
	is.Contains(string(data), "\r\n", "CRLF not preserved on save")
	// No bare LF should remain once CRLF pairs are stripped.
	is.NotContains(string(bytes.ReplaceAll(data, []byte("\r\n"), nil)), "\n", "found bare LF in a CRLF file")
	is.Contains(string(data), "image: ubuntu", "edit not applied")
}

// TestSave_preservesFileMode: an existing file's permission bits survive a save
// (atomic write must not reset the mode to 0600). POSIX only.
func TestSave_preservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "perm.yaml")
	must.NoError(os.WriteFile(path, []byte("name: web\n"), 0o644))
	must.NoError(os.Chmod(path, 0o644)) // defeat umask
	doc, err := Load(path, canonicalOrder)
	must.NoError(err)
	doc, err = doc.Replace("name", "name: api\n")
	must.NoError(err)
	_, err = doc.Save()
	must.NoError(err)
	info, _ := os.Stat(path)
	is.Equal(os.FileMode(0o644), info.Mode().Perm(), "file mode must be preserved after save")
}

// TestSave_noLeftoverTempFiles: the atomic write must not leave .tmp droppings
// in the target directory.
func TestSave_noLeftoverTempFiles(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")
	doc, err := Load(path, canonicalOrder)
	must.NoError(err)
	doc, err = doc.Insert("name: web\n")
	must.NoError(err)
	_, err = doc.Save()
	must.NoError(err)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		is.Equal("out.yaml", e.Name(), "unexpected leftover file after atomic save")
	}
}

// TestExternallyChanged: a file modified on disk after load is detected, and a
// Save resets the baseline so a subsequent check is clean.
func TestExternallyChanged(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "ext.yaml")
	must.NoError(os.WriteFile(path, []byte("name: web\n"), 0o644))
	doc, err := Load(path, canonicalOrder)
	must.NoError(err)
	must.False(doc.ExternallyChanged(), "freshly loaded file should not look externally changed")

	// Another process rewrites the file (different size → reliably detected).
	must.NoError(os.WriteFile(path, []byte("name: web\nimage: alpine\nremoteUser: root\n"), 0o644))
	is.True(doc.ExternallyChanged(), "external modification not detected")

	// Saving our own content re-establishes the baseline.
	doc, err = doc.Insert("image: ubuntu\n")
	must.NoError(err)
	doc, err = doc.Save()
	must.NoError(err)
	is.False(doc.ExternallyChanged(), "baseline not reset after our own Save")
}

// TestReplace_preservesSurroundingComments: editing one block must not drop
// comments and blank lines that live outside the replaced block.
// TestReplace_preservesPositionAndBlankLines reproduces a real-world bug: a
// document whose on-disk top-level order doesn't match knownOrder (here,
// "extra" sits between "name" and "image" but has no rank in knownOrder, like
// a PassthroughKey). Editing "name" must not relocate it to its canonical
// position, and the blank lines separating blocks must survive untouched.
func TestReplace_preservesPositionAndBlankLines(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	src := "name: web\n\nextra:\n  - one\n\nimage: alpine\n"
	doc, err := New([]byte(src), canonicalOrder) // canonicalOrder: []string{"name", "image"}
	must.NoError(err)
	doc, err = doc.Replace("name", "name: api\n")
	must.NoError(err)
	want := "name: api\n\nextra:\n  - one\n\nimage: alpine\n"
	is.Equal(want, string(doc.Raw()), "Replace must not reorder blocks or drop blank-line separators")
}

func TestReplace_preservesSurroundingComments(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	src := "# top of file\nname: web\n\n# the image to use\nimage: alpine\n# trailing note\n"
	path := filepath.Join(t.TempDir(), "comments.yaml")
	must.NoError(os.WriteFile(path, []byte(src), 0o644))
	doc, err := Load(path, canonicalOrder)
	must.NoError(err)
	doc, err = doc.Replace("name", "name: api\n")
	must.NoError(err)
	got := string(doc.Raw())
	for _, want := range []string{"# top of file", "# the image to use", "# trailing note", "name: api"} {
		is.Contains(got, want, "comment/line lost after editing a sibling block")
	}
}

// ---------------------------------------------------------------------------
// YAML edge cases: anchors, multi-document, tab indentation
// ---------------------------------------------------------------------------

func TestParseBlocks_withAnchors(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	// Anchors must not crash ParseBlocks; the resolved content is what matters.
	raw := "base: &anchor\n  value: x\nderived:\n  <<: *anchor\n"
	blocks, err := ParseBlocks([]byte(raw))
	must.NoError(err, "ParseBlocks with anchors")
	must.Len(blocks, 2, "expected 2 blocks")
	is.Equal("base", blocks[0].Key)
	is.Equal("derived", blocks[1].Key)
}

func TestParseBlocks_multiDocument(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	// Only the first YAML document should be returned; no panic.
	raw := "key1: val1\n---\nkey2: val2\n"
	blocks, err := ParseBlocks([]byte(raw))
	must.NoError(err, "ParseBlocks with multi-document")
	// yaml.Unmarshal into a single node only reads the first document.
	for _, b := range blocks {
		is.NotEqual("key2", b.Key, "ParseBlocks returned a block from the second document")
	}
}

func TestParseBlocks_tabIndented(t *testing.T) {
	is := assert.New(t)
	// Tab-indented YAML is invalid; ParseBlocks should return an error, not panic.
	raw := "key:\n\tvalue: x\n"
	_, err := ParseBlocks([]byte(raw))
	is.Error(err, "expected error for tab-indented YAML")
}

func TestDocument_ReplaceRoundTrip(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	// After Replace, the stored block must be semantically equal to the snippet.
	doc, err := New([]byte("name: mydev\nimage: ubuntu:22.04\n"), []string{"name", "image"})
	must.NoError(err)
	doc, err = doc.Replace("image", "image: debian:12\n")
	must.NoError(err, "Replace failed")
	is.True(doc.Dirty(), "expected dirty=true after Replace")
	is.Contains(string(doc.Raw()), "debian:12", "replaced content not found in document")
}

func TestDocument_InsertRoundTrip(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	// After Insert, the stored block must be semantically equal to the snippet.
	doc, err := New([]byte("name: mydev\n"), []string{"name", "image"})
	must.NoError(err)
	doc, err = doc.Insert("image: ubuntu:22.04\n")
	must.NoError(err, "Insert failed")
	is.Contains(string(doc.Raw()), "ubuntu:22.04", "inserted content not found in document")
}

// TestLoad_utf8BOM verifies that files starting with a UTF-8 BOM load correctly:
// the BOM must not appear in any block key, editing must work, and a save+reload
// cycle must not corrupt the file.
func TestLoad_utf8BOM(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	raw := append([]byte{0xEF, 0xBB, 0xBF}, "name: mydev\nimage: ubuntu:22.04\n"...)
	path := filepath.Join(t.TempDir(), "bom.yaml")
	must.NoError(os.WriteFile(path, raw, 0o600))

	doc, err := Load(path, canonicalOrder)
	must.NoError(err, "Load with UTF-8 BOM")
	for _, b := range doc.Blocks() {
		is.Contains([]string{"name", "image"}, b.Key, "BOM leaking into key name?")
	}

	// Editing must work.
	doc, err = doc.Replace("name", "name: api\n")
	must.NoError(err, "Replace after BOM load")
	_, err = doc.Save()
	must.NoError(err, "Save after BOM edit")

	// Reload must also parse correctly - no BOM duplication or corruption.
	doc2, err := Load(path, canonicalOrder)
	must.NoError(err, "reload after BOM save")
	for _, b := range doc2.Blocks() {
		is.Contains([]string{"name", "image"}, b.Key, "after save+reload, unexpected block key")
	}
	want2 := "name: api\nimage: ubuntu:22.04\n"
	is.Equal(want2, string(doc2.Raw()), "Raw after BOM round-trip")
}

// TestLoad_utf8BOM_strip verifies that the raw bytes returned by a BOM-prefixed
// file do not start with the BOM sequence - stripping it prevents it from
// leaking into block content or being re-inserted on partial edits.
func TestLoad_utf8BOM_strip(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	bom := []byte{0xEF, 0xBB, 0xBF}
	raw := append([]byte{0xEF, 0xBB, 0xBF}, "name: mydev\n"...)
	path := filepath.Join(t.TempDir(), "bom.yaml")
	must.NoError(os.WriteFile(path, raw, 0o600))
	doc, err := Load(path, canonicalOrder)
	must.NoError(err, "Load")
	is.False(bytes.HasPrefix(doc.Raw(), bom), "Raw() still starts with UTF-8 BOM - should be stripped on load")
}

// TestRemoveBlock_staleRangeNoPanic ensures RemoveBlock clamps a block range that
// runs past the current line slice (e.g. blocks stale relative to raw) instead of
// panicking on lines[end:].
func TestRemoveBlock_staleRangeNoPanic(t *testing.T) {
	must := require.New(t)
	raw := []byte("a: 1\nb: 2\n")
	blocks := []Block{{Key: "a", Line: 1, EndLine: 99}}
	_, err := RemoveBlock(raw, blocks, "a")
	must.NoError(err, "RemoveBlock must not error on a stale range")
}

// TestBlockContent_staleRangeNoPanic ensures BlockContent clamps a start index
// past the line slice (a previously unclamped panic path) instead of slicing
// lines[start:end] with start > end.
func TestBlockContent_staleRangeNoPanic(t *testing.T) {
	must := require.New(t)
	raw := []byte("a: 1\n")
	blocks := []Block{{Key: "a", Line: 9, EndLine: 99}}
	_, err := BlockContent(raw, blocks, "a")
	must.NoError(err, "BlockContent must not error on a stale range")
}

// TestDocument_InsertStripsTrailingBlankLines ensures a snippet with trailing
// blank lines does not leave a blank line wedged between blocks.
func TestDocument_InsertStripsTrailingBlankLines(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: x\n"), canonicalOrder)
	must.NoError(err)
	doc, err = doc.Insert("image: y\n\n")
	must.NoError(err, "Insert")
	is.Equal("name: x\nimage: y\n", string(doc.Raw()), "Insert left blank lines")
}

// TestDocument_Reload replaces the in-memory state with the on-disk content,
// resetting dirty and the undo history.
func TestDocument_Reload(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "reload.yaml")
	must.NoError(os.WriteFile(path, []byte("name: original\n"), 0o600))
	doc, err := Load(path, canonicalOrder)
	must.NoError(err, "Load")

	// Local edit, then an external rewrite of the file.
	doc, err = doc.Insert("image: ubuntu:22.04\n")
	must.NoError(err, "Insert")
	external := "name: rewritten\nremoteUser: root\n"
	must.NoError(os.WriteFile(path, []byte(external), 0o600))

	doc, err = doc.Reload()
	must.NoError(err, "Reload")
	is.Equal(external, string(doc.Raw()), "Raw after reload")
	is.False(doc.Dirty(), "reloaded document should not be dirty")
	is.False(doc.CanUndo() || doc.CanRedo(), "reload should reset the undo/redo history")
	is.Len(doc.Blocks(), 2, "expected 2 blocks after reload")
}

// TestDocument_Reload_missingFile mirrors Load: a deleted file reloads as an
// empty document rather than erroring.
func TestDocument_Reload_missingFile(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "gone.yaml")
	must.NoError(os.WriteFile(path, []byte("name: x\n"), 0o600))
	doc, err := Load(path, canonicalOrder)
	must.NoError(err, "Load")
	must.NoError(os.Remove(path))

	doc, err = doc.Reload()
	must.NoError(err, "Reload after delete")
	is.Empty(doc.Raw())
}

// TestDocument_Reload_noPath errors on in-memory documents.
func TestDocument_Reload_noPath(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := New([]byte("name: x\n"), canonicalOrder)
	must.NoError(err)
	_, err = doc.Reload()
	is.Error(err, "Reload without a path should return an error")
}

// TestBlockSemanticEqual_roundtripComparison guards the round-trip verification
// in Insert/Replace. The check used to compare snippet against
// key+":\n"+recovered, but recovered (from BlockContent) already includes the
// key line - so the prefix produced a duplicate-key YAML that fails to parse,
// and blockSemanticEqual fail-opens to true. A real divergence was therefore
// never caught. The fix compares snippet against recovered directly.
func TestBlockSemanticEqual_roundtripComparison(t *testing.T) {
	is := assert.New(t)
	snippet := "image: ubuntu:22.04\n"
	recovered := "image: ubuntu:22.04\n" // BlockContent includes the key line

	is.True(blockSemanticEqual(snippet, recovered), "identical blocks must compare equal")

	diverged := "image: SOMETHING-ELSE\n"
	is.False(blockSemanticEqual(snippet, diverged), "divergent blocks must compare NOT equal")

	// When b is a malformed duplicate-key document (e.g. the old code produced
	// key+":\n"+recovered, creating two "image" keys), it fails to parse and
	// blockSemanticEqual must return false so the round-trip check triggers a
	// rollback rather than silently accepting corrupted content.
	is.False(blockSemanticEqual(snippet, "image:\n"+diverged), "malformed b must fail-closed (false) so corruption triggers rollback")

	// When a (the original snippet) fails to parse, the function must also
	// fail-closed - it must not silently accept an unverifiable round-trip.
	is.False(blockSemanticEqual("image:\n"+snippet, snippet), "malformed a must fail-closed (false) - symmetric with malformed b")
}

// TestSnapshot_clearsFuture documents the precondition for the rollback fix:
// snapshot() always sets future to nil before the round-trip check runs.
func TestSnapshot_clearsFuture(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)
	d.future = [][]byte{[]byte("redo-state\n")}

	d = d.snapshot()
	is.Nil(d.future, "snapshot clears the redo stack")
}

// TestRollback_doesNotRestoreFuture shows that rollback() alone leaves the redo
// stack empty -- which is why Insert/Replace must explicitly restore savedFuture.
func TestRollback_doesNotRestoreFuture(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)
	d.history = [][]byte{copyBytes(d.raw)}
	d.future = [][]byte{[]byte("redo-state\n")}

	d = d.snapshot()
	d = d.rollback()
	is.Empty(d.future, "rollback alone does not restore future (expectedpre-fix behavior)")
}

// TestRollback_savedFutureRestoresRedoStack verifies the fix used in Insert/Replace:
// capture savedFuture before snapshot, restore it in all rollback paths.
func TestRollback_savedFutureRestoresRedoStack(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)
	d.history = [][]byte{copyBytes(d.raw)}
	d.future = [][]byte{[]byte("redo-state\n")}
	want := d.future

	savedFuture := d.future
	d = d.snapshot()
	d = d.rollback()
	d.future = savedFuture

	is.Equal(want, d.future, "savedFuture pattern restores the redo stack after rollback")
	is.Equal("a: 1\n", string(d.raw), "rollback restores raw")
}

// TestRollback_restoresConsistencyAfterRawMutation verifies that rollback()
// restores both raw and blocks when called after d.raw was set to invalid content.
func TestRollback_restoresConsistencyAfterRawMutation(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	d, err := New([]byte("a: 1\n"), nil)
	must.NoError(err)

	// Simulate what Insert does: snapshot, then set raw to content that would
	// make ParseBlocks fail, then rollback to restore consistency.
	d = d.snapshot()
	d.raw = []byte("invalid: [\n") // unclosed flow sequence - ParseBlocks would fail on this

	// Pre-fix: caller received d with d.raw=invalid, d.blocks=stale (inconsistent)
	// Post-fix: rollback is called, restoring both raw and blocks.
	d = d.rollback()
	is.Equal("a: 1\n", string(d.raw), "rollback restored raw to pre-mutation state")
	must.Len(d.blocks, 1, "rollback restored blocks via re-parse")
	is.Equal("a", d.blocks[0].Key, "restored block key matches original content")
}

// TestSetPath_reloadReadsSourcePath guards the SavePath flow: Reload must
// re-read the file the document was loaded from, not the save destination
// (which may not even exist yet), and the destination must survive the reload.
func TestSetPath_reloadReadsSourcePath(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "template.yaml")
	dst := filepath.Join(dir, "new.yaml")
	must.NoError(os.WriteFile(src, []byte("name: mydev\n"), 0o600))

	doc, err := Load(src, canonicalOrder)
	must.NoError(err)
	doc = doc.SetPath(dst)

	reloaded, err := doc.Reload()
	must.NoError(err, "Reload")
	is.Equal("name: mydev\n", string(reloaded.Raw()), "Reload must re-read the load path, not the save path")
	is.Equal(dst, reloaded.Path(), "save destination must survive a reload")
}

// TestSetPath_externallyChangedNoFalsePositive guards that SetPath records the
// destination's on-disk state: an untouched, pre-existing destination must not
// read as externally changed on the first save.
func TestSetPath_externallyChangedNoFalsePositive(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "template.yaml")
	dst := filepath.Join(dir, "existing.yaml")
	must.NoError(os.WriteFile(src, []byte("name: mydev\n"), 0o600))
	must.NoError(os.WriteFile(dst, []byte("name: old\n"), 0o600))

	doc, err := Load(src, canonicalOrder)
	must.NoError(err)
	doc = doc.SetPath(dst)

	is.False(doc.ExternallyChanged(), "untouched destination must not read as externally changed")
}

// TestMarkSaved_preservesNewerEdits guards the async-save flow: applying a
// completed save's outcome onto a document that gained edits meanwhile must
// keep those edits (and the dirty flag), updating only the persistence state.
func TestMarkSaved_preservesNewerEdits(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	path := filepath.Join(t.TempDir(), "doc.yaml")
	must.NoError(os.WriteFile(path, []byte("name: mydev\nimage: ubuntu:22.04\n"), 0o600))

	doc, err := Load(path, canonicalOrder)
	must.NoError(err)

	saved, err := doc.Save() // the snapshot a background command would write
	must.NoError(err)

	live, err := doc.Remove("image") // newer edit racing with the save
	must.NoError(err)

	live = live.MarkSaved(saved)
	is.True(live.Dirty(), "newer edits must keep the document dirty after MarkSaved")
	is.NotContains(string(live.Raw()), "image:", "MarkSaved must not clobber newer edits")

	clean := doc.MarkSaved(saved)
	is.False(clean.Dirty(), "a document matching the saved content must be clean")
}
