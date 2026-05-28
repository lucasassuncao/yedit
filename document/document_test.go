package document_test

import (
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
	if string(doc.Raw()) != "name: mydev\nimage: ubuntu:22.04\n" {
		t.Errorf("CRLF not normalised: %q", doc.Raw())
	}
}

func TestDocument_BlockContent(t *testing.T) {
	doc, err := document.New([]byte("name: mydev\nimage: ubuntu:22.04\n"), canonicalOrder)
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
	doc, err := document.New([]byte("name: mydev\nforwardPorts:\n  - 3000\n"), canonicalOrder)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Insert("image: ubuntu:22.04\n"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got := string(doc.Raw())
	want := "name: mydev\nimage: ubuntu:22.04\nforwardPorts:\n  - 3000\n"
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
	original := []byte("name: mydev\nimage: ubuntu:22.04\n")
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
	doc, err := document.New([]byte("name: mydev\nimage: ubuntu:22.04\n"), canonicalOrder)
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
	if string(doc.Raw()) != "name: mydev\nimage: ubuntu:22.04\n" {
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
