package document_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/document"
)

var canonicalOrder = []string{"name", "image", "forwardPorts", "remoteUser"}

func TestLoad_missing(t *testing.T) {
	is := assert.New(t)
	must := require.New(t)
	path := filepath.Join(t.TempDir(), "missing.yaml")

	doc, err := document.Load(path, canonicalOrder)
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

	doc, err := document.Load(path, canonicalOrder)
	must.NoError(err, "Load")
	is.Equal("name: mydev\nimage: ubuntu:22.04\n", string(doc.Raw()), "CRLF not normalised")
}

func TestDocument_BlockContent(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := document.New([]byte("name: mydev\nimage: ubuntu:22.04\n"), canonicalOrder)
	must.NoError(err)
	content, err := doc.BlockContent("image")
	must.NoError(err, "BlockContent")
	is.Equal("image: ubuntu:22.04\n", content)
}

func TestDocument_InsertOrdered(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := document.New([]byte("name: mydev\nforwardPorts:\n  - 3000\n"), canonicalOrder)
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
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	_, err := doc.Remove("image")
	is.Error(err, "expected error removing absent key")
}

func TestDocument_ReplaceRawInvalid(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	original := []byte("name: mydev\nimage: ubuntu:22.04\n")
	doc, err := document.New(original, canonicalOrder)
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
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	_, ok := doc.Undo()
	is.False(ok, "Undo on empty history should return false")
}

func TestDocument_UndoRestores(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
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
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	doc, _ = doc.Insert("image: ubuntu:22.04\n")
	doc, _ = doc.Insert("remoteUser: vscode\n")
	var ok bool
	doc, ok = doc.Undo()
	must.True(ok, "undo failed")
	is.True(doc.Dirty(), "dirty should be true after one undo (raw still differs from loaded)")
}

func TestDocument_RedoEmpty(t *testing.T) {
	is := assert.New(t)
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	_, ok := doc.Redo()
	is.False(ok, "Redo with nothing undone should return false")
	is.False(doc.CanRedo(), "CanRedo should be false with nothing undone")
}

func TestDocument_RedoReappliesUndoneChange(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
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
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
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
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
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
	is.Equal(document.HistoryLimit, count, "expected HistoryLimit undos available")
}

func TestDocument_ReplaceAtomic(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := document.New([]byte("name: mydev\nimage: ubuntu:22.04\n"), canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
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
	doc, _ := document.Load(path, canonicalOrder)
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
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
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
func TestReplace_preservesSurroundingComments(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	src := "# top of file\nname: web\n\n# the image to use\nimage: alpine\n# trailing note\n"
	path := filepath.Join(t.TempDir(), "comments.yaml")
	must.NoError(os.WriteFile(path, []byte(src), 0o644))
	doc, err := document.Load(path, canonicalOrder)
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
	blocks, err := document.ParseBlocks([]byte(raw))
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
	blocks, err := document.ParseBlocks([]byte(raw))
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
	_, err := document.ParseBlocks([]byte(raw))
	is.Error(err, "expected error for tab-indented YAML")
}

func TestDocument_ReplaceRoundTrip(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	// After Replace, the stored block must be semantically equal to the snippet.
	doc, err := document.New([]byte("name: mydev\nimage: ubuntu:22.04\n"), []string{"name", "image"})
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
	doc, err := document.New([]byte("name: mydev\n"), []string{"name", "image"})
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

	doc, err := document.Load(path, canonicalOrder)
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
	doc2, err := document.Load(path, canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
	must.NoError(err, "Load")
	is.False(bytes.HasPrefix(doc.Raw(), bom), "Raw() still starts with UTF-8 BOM - should be stripped on load")
}

// TestRemoveBlock_staleRangeNoPanic ensures RemoveBlock clamps a block range that
// runs past the current line slice (e.g. blocks stale relative to raw) instead of
// panicking on lines[end:].
func TestRemoveBlock_staleRangeNoPanic(t *testing.T) {
	must := require.New(t)
	raw := []byte("a: 1\nb: 2\n")
	blocks := []document.Block{{Key: "a", Line: 1, EndLine: 99}}
	_, err := document.RemoveBlock(raw, blocks, "a")
	must.NoError(err, "RemoveBlock must not error on a stale range")
}

// TestBlockContent_staleRangeNoPanic ensures BlockContent clamps a start index
// past the line slice (a previously unclamped panic path) instead of slicing
// lines[start:end] with start > end.
func TestBlockContent_staleRangeNoPanic(t *testing.T) {
	must := require.New(t)
	raw := []byte("a: 1\n")
	blocks := []document.Block{{Key: "a", Line: 9, EndLine: 99}}
	_, err := document.BlockContent(raw, blocks, "a")
	must.NoError(err, "BlockContent must not error on a stale range")
}

// TestDocument_InsertStripsTrailingBlankLines ensures a snippet with trailing
// blank lines does not leave a blank line wedged between blocks.
func TestDocument_InsertStripsTrailingBlankLines(t *testing.T) {
	must := require.New(t)
	is := assert.New(t)
	doc, err := document.New([]byte("name: x\n"), canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
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
	doc, err := document.Load(path, canonicalOrder)
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
	doc, err := document.New([]byte("name: x\n"), canonicalOrder)
	must.NoError(err)
	_, err = doc.Reload()
	is.Error(err, "Reload without a path should return an error")
}
