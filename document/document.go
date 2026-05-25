package document

import (
	"fmt"
	"os"
	"strings"
)

// HistoryLimit caps the undo stack.
const HistoryLimit = 50

// Document owns the YAML editing state. All mutations are atomic and snapshot
// for undo automatically. Single-threaded — no concurrent use.
//
// knownOrder defines the canonical key order used by Insert/Replace to place
// blocks. Pass nil for unordered append behaviour.
type Document struct {
	path       string
	raw        []byte
	blocks     []Block
	history    [][]byte
	dirty      bool
	knownOrder []string
}

// Load reads a YAML file from path. A non-existent file is not an error — the
// returned Document is empty, dirty=false, and Save writes to path.
//
// knownOrder is the canonical key order for ordered Insert/Replace.
func Load(path string, knownOrder []string) (*Document, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is supplied by the embedding application
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if raw == nil {
		raw = []byte{}
	}
	raw = []byte(strings.ReplaceAll(string(raw), "\r\n", "\n"))

	blocks, err := ParseBlocks(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &Document{path: path, raw: raw, blocks: blocks, knownOrder: knownOrder}, nil
}

// New builds a Document from raw bytes. Intended for tests and in-memory use;
// the resulting document has no file path.
func New(raw []byte, knownOrder []string) (*Document, error) {
	raw = []byte(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	blocks, err := ParseBlocks(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing raw: %w", err)
	}
	return &Document{raw: raw, blocks: blocks, knownOrder: knownOrder}, nil
}

func (d *Document) Raw() []byte     { return d.raw }
func (d *Document) Blocks() []Block { return d.blocks }
func (d *Document) Path() string    { return d.path }
func (d *Document) Dirty() bool     { return d.dirty }
func (d *Document) CanUndo() bool   { return len(d.history) > 0 }

// BlockContent returns the raw lines for a given block key.
func (d *Document) BlockContent(key string) (string, error) {
	return BlockContent(d.raw, d.blocks, key)
}

// snapshot pushes the current raw onto the history stack, capping at HistoryLimit.
func (d *Document) snapshot() {
	snap := make([]byte, len(d.raw))
	copy(snap, d.raw)
	d.history = append(d.history, snap)
	if len(d.history) > HistoryLimit {
		d.history = d.history[len(d.history)-HistoryLimit:]
	}
}

// Insert adds snippet to the document, positioned by the canonical key order.
// Snapshots history and sets dirty on success.
func (d *Document) Insert(snippet string) error {
	newRaw, err := InsertBlock(d.raw, snippet, d.knownOrder)
	if err != nil {
		return err
	}
	d.snapshot()
	d.raw = newRaw
	blocks, err := ParseBlocks(newRaw)
	if err != nil {
		return fmt.Errorf("reparsing after insert: %w", err)
	}
	d.blocks = blocks
	d.dirty = true
	return nil
}

// Remove deletes the block with the given key. Returns an error if the key is
// not present.
func (d *Document) Remove(key string) error {
	newRaw, err := RemoveBlock(d.raw, d.blocks, key)
	if err != nil {
		return err
	}
	d.snapshot()
	d.raw = newRaw
	blocks, err := ParseBlocks(newRaw)
	if err != nil {
		return fmt.Errorf("reparsing after remove: %w", err)
	}
	d.blocks = blocks
	d.dirty = true
	return nil
}

// Replace removes the block at key and inserts snippet in its schema-ordered
// position. Records a single history snapshot for the combined operation.
func (d *Document) Replace(key, snippet string) error {
	removed, err := RemoveBlock(d.raw, d.blocks, key)
	if err != nil {
		return err
	}
	inserted, err := InsertBlock(removed, snippet, d.knownOrder)
	if err != nil {
		return err
	}
	d.snapshot()
	d.raw = inserted
	blocks, err := ParseBlocks(inserted)
	if err != nil {
		return fmt.Errorf("reparsing after replace: %w", err)
	}
	d.blocks = blocks
	d.dirty = true
	return nil
}

// ReplaceRaw replaces the document content with raw, normalising CRLF.
// If raw fails to parse, the document is left untouched and the error is returned.
// Does NOT snapshot — direct YAML editing is not tracked in the undo history;
// only committed block operations (Insert, Replace, Remove) are undoable.
func (d *Document) ReplaceRaw(raw []byte) error {
	raw = []byte(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	blocks, err := ParseBlocks(raw)
	if err != nil {
		return err
	}
	d.raw = raw
	d.blocks = blocks
	d.dirty = true
	return nil
}

// Undo restores the previous raw from history. Returns false if history is empty.
// Does not push a new snapshot; keeps dirty=true.
func (d *Document) Undo() bool {
	if len(d.history) == 0 {
		return false
	}
	prev := d.history[len(d.history)-1]
	d.history = d.history[:len(d.history)-1]
	d.raw = prev
	if blocks, err := ParseBlocks(prev); err == nil {
		d.blocks = blocks
	}
	d.dirty = true
	return true
}

// Save writes the current raw to disk at d.path with mode 0600 and clears dirty.
// Returns an error if d.path is empty.
func (d *Document) Save() error {
	if d.path == "" {
		return fmt.Errorf("document has no path; Load requires a path")
	}
	if err := os.WriteFile(d.path, d.raw, 0o600); err != nil {
		return err
	}
	d.dirty = false
	return nil
}
