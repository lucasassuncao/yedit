package document_test

import (
	"testing"

	"github.com/lucasassuncao/yedit/document"
)

const sampleYAML = `name: mydev
image: ubuntu:22.04
features:
  ghcr.io/devcontainers/features/git:1: {}
forwardPorts:
  - 3000
`

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

func TestRemoveBlock(t *testing.T) {
	raw := []byte(sampleYAML)
	blocks, _ := document.ParseBlocks(raw)
	result, err := document.RemoveBlock(raw, blocks, "image")
	if err != nil {
		t.Fatalf("RemoveBlock: %v", err)
	}
	remaining, _ := document.ParseBlocks(result)
	for _, b := range remaining {
		if b.Key == "image" {
			t.Error("image block still present after removal")
		}
	}
}

func TestRemoveBlock_notFound(t *testing.T) {
	raw := []byte(sampleYAML)
	blocks, _ := document.ParseBlocks(raw)
	_, err := document.RemoveBlock(raw, blocks, "absent")
	if err == nil {
		t.Error("expected error for absent key, got nil")
	}
}

func TestInsertBlock_unordered(t *testing.T) {
	raw := []byte(sampleYAML)
	snippet := "remoteUser: vscode\n"
	// nil order = append at end.
	result, err := document.InsertBlock(raw, snippet, nil)
	if err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	blocks, _ := document.ParseBlocks(result)
	found := false
	for _, b := range blocks {
		if b.Key == "remoteUser" {
			found = true
		}
	}
	if !found {
		t.Error("remoteUser block not found after insert")
	}
}

func TestInsertBlock_ordered(t *testing.T) {
	// File has name + forwardPorts. Inserting "image" should land between them
	// when canonical order puts image between name and forwardPorts.
	base := "name: mydev\nforwardPorts:\n  - 3000\n"
	snippet := "image: ubuntu:22.04\n"
	order := []string{"name", "image", "forwardPorts"}

	result, err := document.InsertBlock([]byte(base), snippet, order)
	if err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}

	blocks, _ := document.ParseBlocks(result)
	got := make([]string, 0, len(blocks))
	for _, b := range blocks {
		got = append(got, b.Key)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 blocks, got %d: %v", len(got), got)
	}
	if got[0] != "name" || got[1] != "image" || got[2] != "forwardPorts" {
		t.Errorf("wrong order: %v, want [name image forwardPorts]", got)
	}
}

func TestInsertBlock_unknownKeyAppends(t *testing.T) {
	base := "name: mydev\n"
	snippet := "novel: value\n"
	order := []string{"name", "image"}

	result, err := document.InsertBlock([]byte(base), snippet, order)
	if err != nil {
		t.Fatalf("InsertBlock: %v", err)
	}
	blocks, _ := document.ParseBlocks(result)
	if len(blocks) != 2 || blocks[1].Key != "novel" {
		t.Errorf("unknown key should append at end; got blocks %+v", blocks)
	}
}

func TestValidateSnippet(t *testing.T) {
	if err := document.ValidateSnippet("remoteUser: vscode\n"); err != nil {
		t.Errorf("expected valid snippet to pass: %v", err)
	}

	if err := document.ValidateSnippet("remoteUser: :\n  broken:"); err == nil {
		t.Error("expected invalid snippet to fail validation")
	}
}
