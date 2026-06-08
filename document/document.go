package document

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
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
	loaded     []byte // raw at last load/save, used to clear dirty on Undo-to-original
	blocks     []Block
	history    [][]byte
	dirty      bool
	knownOrder []string

	usedCRLF bool // file on disk used CRLF line endings; restored on Save

	// diskModTime/diskSize record the on-disk file state at the last load/save so
	// Save can detect an external modification before overwriting it.
	diskModTime time.Time
	diskSize    int64
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
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // strip UTF-8 BOM
	usedCRLF := bytes.Contains(raw, []byte("\r\n"))
	raw = []byte(strings.ReplaceAll(string(raw), "\r\n", "\n"))

	blocks, err := ParseBlocks(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	d := &Document{path: path, raw: raw, loaded: copyBytes(raw), blocks: blocks, knownOrder: knownOrder, usedCRLF: usedCRLF}
	d.recordDiskState()
	return d, nil
}

// New builds a Document from raw bytes. Intended for tests and in-memory use;
// the resulting document has no file path.
func New(raw []byte, knownOrder []string) (*Document, error) {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // strip UTF-8 BOM
	usedCRLF := bytes.Contains(raw, []byte("\r\n"))
	raw = []byte(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	blocks, err := ParseBlocks(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing raw: %w", err)
	}
	return &Document{raw: raw, loaded: copyBytes(raw), blocks: blocks, knownOrder: knownOrder, usedCRLF: usedCRLF}, nil
}

func copyBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (d *Document) Raw() []byte     { return d.raw }
func (d *Document) Blocks() []Block { return d.blocks }
func (d *Document) Path() string    { return d.path }
func (d *Document) Dirty() bool     { return d.dirty }
func (d *Document) CanUndo() bool   { return len(d.history) > 0 }

// SetPath overrides the path used by Save. Call after Load when the save
// destination differs from the source (e.g. writing a template to a new file).
func (d *Document) SetPath(path string) { d.path = path }

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
// Snapshots history and sets dirty on success. Returns an error (and rolls back)
// if a post-write round-trip check detects that the stored block diverges from
// the submitted snippet.
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

	if sBlocks, err2 := ParseBlocks([]byte(snippet)); err2 == nil && len(sBlocks) > 0 {
		key := sBlocks[0].Key
		if recovered, err2 := BlockContent(d.raw, d.blocks, key); err2 == nil {
			if !blockSemanticEqual(snippet, recovered) {
				d.Undo()
				return fmt.Errorf("round-trip verification failed after insert of %q", key)
			}
		}
	}
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
// Returns an error (and rolls back) if a post-write round-trip check detects
// that the stored block diverges from the submitted snippet.
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

	if recovered, err2 := BlockContent(d.raw, d.blocks, key); err2 == nil {
		if !blockSemanticEqual(snippet, recovered) {
			d.Undo()
			return fmt.Errorf("round-trip verification failed after replace of %q", key)
		}
	}
	return nil
}

// blockSemanticEqual reports whether two YAML strings are semantically equivalent —
// same structure and values regardless of formatting, key order, or quoting style.
// Returns true on any parse error so callers always fall through to the happy path.
func blockSemanticEqual(a, b string) bool {
	var va, vb any
	if err := yaml.Unmarshal([]byte(a), &va); err != nil {
		return false
	}
	if err := yaml.Unmarshal([]byte(b), &vb); err != nil {
		return false
	}
	return reflect.DeepEqual(va, vb)
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
// Does not push a new snapshot; dirty is set based on whether the restored raw
// matches the last-loaded/saved content.
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
	d.dirty = !bytes.Equal(d.raw, d.loaded)
	return true
}

// Save writes the current content to disk at d.path and clears dirty. The write
// is atomic (temp file + rename) so a crash mid-write never truncates the
// original. The file's existing mode is preserved (new files are created 0600),
// and CRLF line endings are restored when the loaded file used them. Returns an
// error if d.path is empty.
func (d *Document) Save() error {
	if d.path == "" {
		return fmt.Errorf("document has no path; Load requires a path")
	}
	out := d.raw
	if d.usedCRLF {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	if err := atomicWrite(d.path, out); err != nil {
		return err
	}
	d.loaded = copyBytes(d.raw)
	d.dirty = false
	d.recordDiskState()
	return nil
}

// ExternallyChanged reports whether the file on disk was modified since this
// Document last loaded or saved it — e.g. another process or a git operation
// edited it. Returns false when there is no path or the file is absent (a save
// would create it, clobbering nothing). Callers should confirm with the user
// before overwriting when this returns true.
func (d *Document) ExternallyChanged() bool {
	if d.path == "" {
		return false
	}
	info, err := os.Stat(d.path)
	if err != nil {
		return false
	}
	return info.ModTime() != d.diskModTime || info.Size() != d.diskSize
}

// recordDiskState captures the on-disk mtime/size so a later Save can detect an
// external modification. A missing file records the zero state.
func (d *Document) recordDiskState() {
	if d.path == "" {
		return
	}
	if info, err := os.Stat(d.path); err == nil {
		d.diskModTime = info.ModTime()
		d.diskSize = info.Size()
	} else {
		d.diskModTime = time.Time{}
		d.diskSize = 0
	}
}

// atomicWrite durably writes data to path: it writes a temp file in the same
// directory, fsyncs it, then renames over path (atomic on the same filesystem,
// and REPLACE_EXISTING on Windows). The destination's existing mode is preserved;
// new files are created 0600. On any failure before the rename the temp file is
// removed and path is left untouched.
func atomicWrite(path string, data []byte) error {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".yedit-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
