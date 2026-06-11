package document_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lucasassuncao/yedit/document"
)

var canonicalOrder = []string{"name", "image", "forwardPorts", "remoteUser"}

func TestLoad_missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")

	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatalf("Load on missing file: unexpected error %v", err)
	}
	if doc == nil {
		t.Fatal("Load returned nil document")
	}
	if len(doc.Raw()) != 0 {
		t.Errorf("expected empty raw, got %q", doc.Raw())
	}
	if doc.Dirty() {
		t.Error("new document should not be dirty")
	}
	if doc.Path() != path {
		t.Errorf("Path() = %q, want %q", doc.Path(), path)
	}
}

func TestLoad_crlf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crlf.yaml")
	content := "name: mydev\r\nimage: ubuntu:22.04\r\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(doc.Raw()) != `name: mydev
image: ubuntu:22.04
` {
		t.Errorf("CRLF not normalised: %q", doc.Raw())
	}
}

func TestDocument_BlockContent(t *testing.T) {
	doc, err := document.New([]byte(`name: mydev
image: ubuntu:22.04
`), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	content, err := doc.BlockContent("image")
	if err != nil {
		t.Fatalf("BlockContent: %v", err)
	}
	if content != "image: ubuntu:22.04\n" {
		t.Errorf("BlockContent = %q, want %q", content, "image: ubuntu:22.04\n")
	}
}

func TestDocument_InsertOrdered(t *testing.T) {
	doc, err := document.New([]byte(`name: mydev
forwardPorts:
  - 3000
`), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("image: ubuntu:22.04\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got := string(doc.Raw())
	want := `name: mydev
image: ubuntu:22.04
forwardPorts:
  - 3000
`
	if got != want {
		t.Errorf("Raw() after Insert =\n%q\nwant\n%q", got, want)
	}
	if !doc.Dirty() {
		t.Error("expected dirty=true after Insert")
	}
	if !doc.CanUndo() {
		t.Error("expected CanUndo=true after Insert")
	}
}

func TestDocument_RemoveNotFound(t *testing.T) {
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	err := doc.Remove("image")
	if err == nil {
		t.Fatal("expected error removing absent key, got nil")
	}
}

func TestDocument_ReplaceRawInvalid(t *testing.T) {
	original := []byte(`name: mydev
image: ubuntu:22.04
`)
	doc, err := document.New(original, canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}

	// Invalid YAML: a mapping with a bare ":" value.
	err = doc.ReplaceRaw([]byte("name: :\n  broken:"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if string(doc.Raw()) != string(original) {
		t.Errorf("raw changed despite parse failure: %q", doc.Raw())
	}
	if doc.Dirty() {
		t.Error("dirty should not be set on parse failure")
	}
	if doc.CanUndo() {
		t.Error("history should not grow on parse failure")
	}
}

func TestDocument_UndoEmpty(t *testing.T) {
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	if doc.Undo() {
		t.Error("Undo on empty history should return false")
	}
}

func TestDocument_UndoRestores(t *testing.T) {
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("image: ubuntu:22.04\n"); err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("remoteUser: vscode\n"); err != nil {
		t.Fatal(err)
	}
	// Undo twice should restore original.
	if !doc.Undo() {
		t.Fatal("first Undo failed")
	}
	if !doc.Undo() {
		t.Fatal("second Undo failed")
	}
	if string(doc.Raw()) != "name: mydev\n" {
		t.Errorf("after two undos, Raw = %q, want %q", doc.Raw(), "name: mydev\n")
	}
	if doc.Dirty() {
		t.Error("dirty should be false when Undo restores the loaded raw")
	}
}

func TestDocument_UndoStaysDirtyMidStack(t *testing.T) {
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	_ = doc.Insert("image: ubuntu:22.04\n")
	_ = doc.Insert("remoteUser: vscode\n")
	// One undo back: still differs from loaded → dirty stays true.
	if !doc.Undo() {
		t.Fatal("undo failed")
	}
	if !doc.Dirty() {
		t.Error("dirty should be true after one undo (raw still differs from loaded)")
	}
}

func TestDocument_RedoEmpty(t *testing.T) {
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	if doc.Redo() {
		t.Error("Redo with nothing undone should return false")
	}
	if doc.CanRedo() {
		t.Error("CanRedo should be false with nothing undone")
	}
}

func TestDocument_RedoReappliesUndoneChange(t *testing.T) {
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("image: ubuntu:22.04\n"); err != nil {
		t.Fatal(err)
	}
	withImage := string(doc.Raw())

	if !doc.Undo() {
		t.Fatal("undo failed")
	}
	if !doc.CanRedo() {
		t.Fatal("CanRedo should be true after Undo")
	}
	if !doc.Redo() {
		t.Fatal("redo failed")
	}
	if string(doc.Raw()) != withImage {
		t.Errorf("after redo, Raw = %q, want %q", doc.Raw(), withImage)
	}
	if !doc.Dirty() {
		t.Error("dirty should be true after redo (raw differs from loaded)")
	}
	// The redo itself must be undoable.
	if !doc.Undo() {
		t.Fatal("undo after redo failed")
	}
	if string(doc.Raw()) != "name: mydev\n" {
		t.Errorf("after undoing the redo, Raw = %q, want %q", doc.Raw(), "name: mydev\n")
	}
}

func TestDocument_RedoClearedByNewMutation(t *testing.T) {
	doc, _ := document.New([]byte("name: mydev\n"), canonicalOrder)
	_ = doc.Insert("image: ubuntu:22.04\n")
	if !doc.Undo() {
		t.Fatal("undo failed")
	}
	// A new mutation forks away from the undone state.
	if err := doc.Insert("remoteUser: vscode\n"); err != nil {
		t.Fatal(err)
	}
	if doc.CanRedo() {
		t.Error("redo stack should be cleared by a new mutation")
	}
	if doc.Redo() {
		t.Error("Redo after a new mutation should return false")
	}
}

func TestDocument_HistoryCapsAt50(t *testing.T) {
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	// Perform 60 mutations: toggle image on and off, alternating.
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			_ = doc.Insert("image: ubuntu:22.04\n")
		} else {
			_ = doc.Remove("image")
		}
	}
	// Drain undo and count steps.
	count := 0
	for doc.Undo() {
		count++
		if count > 100 {
			t.Fatal("undo loop did not terminate")
		}
	}
	if count != document.HistoryLimit {
		t.Errorf("expected %d undos available, got %d", document.HistoryLimit, count)
	}
}

func TestDocument_ReplaceAtomic(t *testing.T) {
	doc, err := document.New([]byte(`name: mydev
image: ubuntu:22.04
`), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Replace("image", "image: alpine:latest\n"); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if !doc.Dirty() {
		t.Error("expected dirty=true")
	}
	// A single Undo should restore the original.
	if !doc.Undo() {
		t.Fatal("expected Undo to succeed")
	}
	if string(doc.Raw()) != `name: mydev
image: ubuntu:22.04
` {
		t.Errorf("after one Undo, Raw =\n%q", doc.Raw())
	}
	// No further undo (single snapshot was recorded by Replace).
	if doc.CanUndo() {
		t.Error("Replace should have recorded exactly one snapshot")
	}
}

func TestDocument_Save(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.yaml")
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("name: mydev\n"); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() {
		t.Fatal("doc should be dirty before save")
	}
	if err := doc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if doc.Dirty() {
		t.Error("doc should not be dirty after Save")
	}

	// Verify file written with mode 0600 (POSIX only; Windows ignores POSIX
	// permission bits and creates files with mode 0666 by default).
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("file mode = %v, want 0600", perm)
		}
	}

	data, _ := os.ReadFile(path)
	if string(data) != "name: mydev\n" {
		t.Errorf("file content = %q, want %q", data, "name: mydev\n")
	}
}

func TestDocument_DirtyLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.yaml")
	doc, _ := document.Load(path, canonicalOrder)
	if doc.Dirty() {
		t.Error("freshly loaded missing-file document should not be dirty")
	}
	if err := doc.Insert("name: mydev\n"); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() {
		t.Error("dirty should be true after Insert")
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	if doc.Dirty() {
		t.Error("dirty should be false after Save")
	}
}

func TestSave_noPath(t *testing.T) {
	doc, err := document.New([]byte("name: mydev\n"), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err == nil {
		t.Error("expected error saving in-memory document, got nil")
	}
}

// TestSave_preservesCRLF: a file loaded with CRLF endings is written back with
// CRLF, so editing one block on Windows does not rewrite every line to LF.
func TestSave_preservesCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crlf.yaml")
	if err := os.WriteFile(path, []byte("name: web\r\nimage: alpine\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Replace("image", "image: ubuntu\n"); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("\r\n")) {
		t.Errorf("CRLF not preserved on save:\n%q", data)
	}
	// No bare LF should remain once CRLF pairs are stripped.
	if bytes.Contains(bytes.ReplaceAll(data, []byte("\r\n"), nil), []byte("\n")) {
		t.Errorf("found bare LF in a CRLF file:\n%q", data)
	}
	if !bytes.Contains(data, []byte("image: ubuntu")) {
		t.Errorf("edit not applied:\n%q", data)
	}
}

// TestSave_preservesFileMode: an existing file's permission bits survive a save
// (atomic write must not reset the mode to 0600). POSIX only.
func TestSave_preservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "perm.yaml")
	if err := os.WriteFile(path, []byte("name: web\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { // defeat umask
		t.Fatal(err)
	}
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Replace("name", "name: api\n"); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("file mode = %v after save, want 0644 (must be preserved)", perm)
	}
}

// TestSave_noLeftoverTempFiles: the atomic write must not leave .tmp droppings
// in the target directory.
func TestSave_noLeftoverTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("name: web\n"); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "out.yaml" {
			t.Errorf("unexpected leftover file after atomic save: %q", e.Name())
		}
	}
}

// TestExternallyChanged: a file modified on disk after load is detected, and a
// Save resets the baseline so a subsequent check is clean.
func TestExternallyChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ext.yaml")
	if err := os.WriteFile(path, []byte("name: web\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ExternallyChanged() {
		t.Fatal("freshly loaded file should not look externally changed")
	}

	// Another process rewrites the file (different size → reliably detected).
	if err := os.WriteFile(path, []byte("name: web\nimage: alpine\nremoteUser: root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !doc.ExternallyChanged() {
		t.Error("external modification not detected")
	}

	// Saving our own content re-establishes the baseline.
	if err := doc.Insert("image: ubuntu\n"); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	if doc.ExternallyChanged() {
		t.Error("baseline not reset after our own Save")
	}
}

// TestReplace_preservesSurroundingComments: editing one block must not drop
// comments and blank lines that live outside the replaced block.
func TestReplace_preservesSurroundingComments(t *testing.T) {
	src := `# top of file
name: web

# the image to use
image: alpine
# trailing note
`
	path := filepath.Join(t.TempDir(), "comments.yaml")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Replace("name", "name: api\n"); err != nil {
		t.Fatal(err)
	}
	got := string(doc.Raw())
	for _, want := range []string{"# top of file", "# the image to use", "# trailing note", "name: api"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Errorf("comment/line %q lost after editing a sibling block:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// YAML edge cases: anchors, multi-document, tab indentation
// ---------------------------------------------------------------------------

func TestParseBlocks_withAnchors(t *testing.T) {
	// Anchors must not crash ParseBlocks; the resolved content is what matters.
	raw := `base: &anchor
  value: x
derived:
  <<: *anchor
`
	blocks, err := document.ParseBlocks([]byte(raw))
	if err != nil {
		t.Fatalf("ParseBlocks with anchors: unexpected error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Key != "base" || blocks[1].Key != "derived" {
		t.Errorf("unexpected block keys: %v", blocks)
	}
}

func TestParseBlocks_multiDocument(t *testing.T) {
	// Only the first YAML document should be returned; no panic.
	raw := `key1: val1
---
key2: val2
`
	blocks, err := document.ParseBlocks([]byte(raw))
	if err != nil {
		t.Fatalf("ParseBlocks with multi-document: unexpected error: %v", err)
	}
	// yaml.Unmarshal into a single node only reads the first document.
	for _, b := range blocks {
		if b.Key == "key2" {
			t.Error("ParseBlocks returned a block from the second document")
		}
	}
}

func TestParseBlocks_tabIndented(t *testing.T) {
	// Tab-indented YAML is invalid; ParseBlocks should return an error, not panic.
	raw := "key:\n\tvalue: x\n"
	_, err := document.ParseBlocks([]byte(raw))
	if err == nil {
		t.Error("expected error for tab-indented YAML, got nil")
	}
}

func TestDocument_ReplaceRoundTrip(t *testing.T) {
	// After Replace, the stored block must be semantically equal to the snippet.
	raw := []byte(`name: mydev
image: ubuntu:22.04
`)
	doc, err := document.New(raw, []string{"name", "image"})
	if err != nil {
		t.Fatal(err)
	}
	snippet := "image: debian:12\n"
	if err := doc.Replace("image", snippet); err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if !doc.Dirty() {
		t.Error("expected dirty=true after Replace")
	}
	got := string(doc.Raw())
	if !containsSubstring(got, "debian:12") {
		t.Errorf("replaced content not found in document:\n%s", got)
	}
}

func TestDocument_InsertRoundTrip(t *testing.T) {
	// After Insert, the stored block must be semantically equal to the snippet.
	raw := []byte("name: mydev\n")
	doc, err := document.New(raw, []string{"name", "image"})
	if err != nil {
		t.Fatal(err)
	}
	snippet := "image: ubuntu:22.04\n"
	if err := doc.Insert(snippet); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	got := string(doc.Raw())
	if !containsSubstring(got, "ubuntu:22.04") {
		t.Errorf("inserted content not found in document:\n%s", got)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstringSearch(s, sub))
}

func containsSubstringSearch(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestLoad_utf8BOM verifies that files starting with a UTF-8 BOM load correctly:
// the BOM must not appear in any block key, editing must work, and a save+reload
// cycle must not corrupt the file.
func TestLoad_utf8BOM(t *testing.T) {
	raw := append([]byte{0xEF, 0xBB, 0xBF}, "name: mydev\nimage: ubuntu:22.04\n"...)
	path := filepath.Join(t.TempDir(), "bom.yaml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatalf("Load with UTF-8 BOM: %v", err)
	}
	for _, b := range doc.Blocks() {
		if b.Key != "name" && b.Key != "image" {
			t.Errorf("unexpected block key %q - BOM leaking into key name?", b.Key)
		}
	}

	// Editing must work.
	if err := doc.Replace("name", "name: api\n"); err != nil {
		t.Fatalf("Replace after BOM load: %v", err)
	}
	if err := doc.Save(); err != nil {
		t.Fatalf("Save after BOM edit: %v", err)
	}

	// Reload must also parse correctly - no BOM duplication or corruption.
	doc2, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatalf("reload after BOM save: %v", err)
	}
	for _, b := range doc2.Blocks() {
		if b.Key != "name" && b.Key != "image" {
			t.Errorf("after save+reload, unexpected block key %q", b.Key)
		}
	}
	want2 := `name: api
image: ubuntu:22.04
`
	if got := string(doc2.Raw()); got != want2 {
		t.Errorf("Raw after BOM round-trip =\n%q\nwant\n%q", got, want2)
	}
}

// TestLoad_utf8BOM_strip verifies that the raw bytes returned by a BOM-prefixed
// file do not start with the BOM sequence - stripping it prevents it from
// leaking into block content or being re-inserted on partial edits.
func TestLoad_utf8BOM_strip(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	raw := append([]byte{0xEF, 0xBB, 0xBF}, "name: mydev\n"...)
	path := filepath.Join(t.TempDir(), "bom.yaml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bytes.HasPrefix(doc.Raw(), bom) {
		t.Error("Raw() still starts with UTF-8 BOM - should be stripped on load")
	}
}

// TestRemoveBlock_staleRangeNoPanic ensures RemoveBlock clamps a block range that
// runs past the current line slice (e.g. blocks stale relative to raw) instead of
// panicking on lines[end:].
func TestRemoveBlock_staleRangeNoPanic(t *testing.T) {
	raw := []byte("a: 1\nb: 2\n")
	blocks := []document.Block{{Key: "a", Line: 1, EndLine: 99}}
	if _, err := document.RemoveBlock(raw, blocks, "a"); err != nil {
		t.Fatalf("RemoveBlock must not error on a stale range: %v", err)
	}
}

// TestBlockContent_staleRangeNoPanic ensures BlockContent clamps a start index
// past the line slice (a previously unclamped panic path) instead of slicing
// lines[start:end] with start > end.
func TestBlockContent_staleRangeNoPanic(t *testing.T) {
	raw := []byte("a: 1\n")
	blocks := []document.Block{{Key: "a", Line: 9, EndLine: 99}}
	if _, err := document.BlockContent(raw, blocks, "a"); err != nil {
		t.Fatalf("BlockContent must not error on a stale range: %v", err)
	}
}

// TestDocument_InsertStripsTrailingBlankLines ensures a snippet with trailing
// blank lines does not leave a blank line wedged between blocks.
func TestDocument_InsertStripsTrailingBlankLines(t *testing.T) {
	doc, err := document.New([]byte("name: x\n"), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("image: y\n\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got := string(doc.Raw())
	want := "name: x\nimage: y\n"
	if got != want {
		t.Errorf("Insert left blank lines: got %q want %q", got, want)
	}
}

// TestDocument_Reload replaces the in-memory state with the on-disk content,
// resetting dirty and the undo history.
func TestDocument_Reload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reload.yaml")
	if err := os.WriteFile(path, []byte("name: original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Local edit, then an external rewrite of the file.
	if err := doc.Insert("image: ubuntu:22.04\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	external := "name: rewritten\nremoteUser: root\n"
	if err := os.WriteFile(path, []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := doc.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := string(doc.Raw()); got != external {
		t.Errorf("Raw after reload = %q, want %q", got, external)
	}
	if doc.Dirty() {
		t.Error("reloaded document should not be dirty")
	}
	if doc.CanUndo() || doc.CanRedo() {
		t.Error("reload should reset the undo/redo history")
	}
	if len(doc.Blocks()) != 2 {
		t.Errorf("expected 2 blocks after reload, got %d", len(doc.Blocks()))
	}
}

// TestDocument_Reload_missingFile mirrors Load: a deleted file reloads as an
// empty document rather than erroring.
func TestDocument_Reload_missingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone.yaml")
	if err := os.WriteFile(path, []byte("name: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := document.Load(path, canonicalOrder)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if err := doc.Reload(); err != nil {
		t.Fatalf("Reload after delete: %v", err)
	}
	if len(doc.Raw()) != 0 {
		t.Errorf("expected empty raw, got %q", doc.Raw())
	}
}

// TestDocument_Reload_noPath errors on in-memory documents.
func TestDocument_Reload_noPath(t *testing.T) {
	doc, err := document.New([]byte("name: x\n"), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Reload(); err == nil {
		t.Error("Reload without a path should return an error")
	}
}
