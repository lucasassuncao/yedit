package document

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// HistoryLimit caps the undo stack.
const HistoryLimit = 50

// Document owns the YAML editing state. Mutations are atomic and snapshot for
// undo automatically. Single-threaded - no concurrent use.
//
// knownOrder is the canonical key order Insert/Replace place blocks by; nil
// means append.
type Document struct {
	path       string
	loadPath   string // file the content was loaded from; Reload re-reads this even after SetPath
	raw        []byte
	loaded     []byte // raw at last load/save; Dirty() is computed against it
	blocks     []Block
	history    [][]byte
	future     [][]byte // redo stack; populated by Undo, discarded on new mutations
	knownOrder []string

	usedCRLF bool // file on disk used CRLF line endings; restored on Save

	// diskModTime/diskSize record the on-disk file state at the last load/save so
	// Save can detect an external modification before overwriting it.
	diskModTime time.Time
	diskSize    int64
}

// Load reads a YAML file from path. A non-existent file is not an error: the
// returned Document is empty and clean, and Save creates the file.
func Load(path string, knownOrder []string) (Document, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is supplied by the embedding application
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Document{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if raw == nil {
		raw = []byte{}
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // strip UTF-8 BOM
	usedCRLF := bytes.Contains(raw, []byte("\r\n"))
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))

	blocks, err := ParseBlocks(raw)
	if err != nil {
		return Document{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	d := Document{path: path, loadPath: path, raw: raw, loaded: copyBytes(raw), blocks: blocks, knownOrder: knownOrder, usedCRLF: usedCRLF}
	d = d.recordDiskState()
	return d, nil
}

// New builds a Document from raw bytes, with no file path. For tests and
// in-memory use.
func New(raw []byte, knownOrder []string) (Document, error) {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) // strip UTF-8 BOM
	usedCRLF := bytes.Contains(raw, []byte("\r\n"))
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	blocks, err := ParseBlocks(raw)
	if err != nil {
		return Document{}, fmt.Errorf("parsing raw: %w", err)
	}
	return Document{raw: raw, loaded: copyBytes(raw), blocks: blocks, knownOrder: knownOrder, usedCRLF: usedCRLF}, nil
}

func copyBytes(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

func (d Document) Raw() []byte     { return d.raw }
func (d Document) Blocks() []Block { return d.blocks }
func (d Document) Path() string    { return d.path }
func (d Document) CanUndo() bool   { return len(d.history) > 0 }
func (d Document) CanRedo() bool   { return len(d.future) > 0 }

// Dirty reports whether the content differs from what was last loaded or saved.
// Computed, not stored: reverting an edit reads as clean and no mutation path
// can forget to update a flag.
func (d Document) Dirty() bool { return !bytes.Equal(d.raw, d.loaded) }

// SetPath overrides the path used by Save; Reload keeps re-reading the original
// load path. The new path's on-disk state is recorded so ExternallyChanged
// compares against the save destination instead of reporting a false positive.
func (d Document) SetPath(path string) Document {
	d.path = path
	return d.recordDiskState()
}

// BlockContent returns the raw lines for a given block key.
func (d Document) BlockContent(key string) (string, error) {
	return BlockContent(d.raw, d.blocks, key)
}

// snapshot pushes the current raw onto the history stack and discards the redo
// entries: a new mutation forks away from the undone states.
func (d Document) snapshot() Document {
	d.history = appendCapped(d.history, copyBytes(d.raw))
	d.future = nil
	return d
}

// appendCapped appends snap, dropping the oldest entries beyond HistoryLimit.
// The result never shares a backing array with stack: Document is copied by
// value, so an in-place append would corrupt sibling copies.
func appendCapped(stack [][]byte, snap []byte) [][]byte {
	start := 0
	if len(stack)+1 > HistoryLimit {
		start = len(stack) + 1 - HistoryLimit
	}
	out := make([][]byte, 0, len(stack)-start+1)
	out = append(out, stack[start:]...)
	out = append(out, snap)
	return out
}

// cloneStack copies the stack so a pop never mutates a backing array shared
// with another Document copy.
func cloneStack(stack [][]byte) [][]byte {
	if len(stack) == 0 {
		return nil
	}
	out := make([][]byte, len(stack))
	copy(out, stack)
	return out
}

// Insert adds snippet, positioned by the canonical key order, and snapshots
// history. Rolls back with an error when the round-trip check finds the stored
// block diverging from the snippet.
func (d Document) Insert(snippet string) (Document, error) {
	snippet = strings.ReplaceAll(snippet, "\r\n", "\n")
	newRaw, err := InsertBlock(d.raw, snippet, d.knownOrder)
	if err != nil {
		return d, err
	}
	savedFuture := d.future
	d = d.snapshot()
	d.raw = newRaw
	blocks, err := ParseBlocks(newRaw)
	if err != nil {
		d = d.rollback()
		d.future = savedFuture
		return d, fmt.Errorf("reparsing after insert: %w", err)
	}
	d.blocks = blocks

	// Verify per key: a multi-key snippet lands as one block per key, so
	// comparing the whole snippet against a single block would falsely reject it.
	if sBlocks, err2 := ParseBlocks([]byte(snippet)); err2 == nil {
		for _, sb := range sBlocks {
			part, errPart := BlockContent([]byte(snippet), sBlocks, sb.Key)
			recovered, errRec := BlockContent(d.raw, d.blocks, sb.Key)
			if errPart != nil || errRec != nil || !blockSemanticEqual(part, recovered) {
				d = d.rollback()
				d.future = savedFuture
				return d, fmt.Errorf("round-trip verification failed after insert of %q", sb.Key)
			}
		}
	}
	return d, nil
}

// Remove deletes the block with the given key. Returns an error if the key is
// not present.
func (d Document) Remove(key string) (Document, error) {
	newRaw, err := RemoveBlock(d.raw, d.blocks, key)
	if err != nil {
		return d, err
	}
	savedFuture := d.future
	d = d.snapshot()
	d.raw = newRaw
	blocks, err := ParseBlocks(newRaw)
	if err != nil {
		d = d.rollback()
		d.future = savedFuture
		return d, fmt.Errorf("reparsing after remove: %w", err)
	}
	d.blocks = blocks
	return d, nil
}

// Replace substitutes the block at key in place, leaving its position and the
// surrounding blank lines and comments untouched, and records one history
// snapshot. Rolls back with an error when the round-trip check finds the stored
// block diverging from the snippet.
func (d Document) Replace(key, snippet string) (Document, error) {
	snippet = strings.ReplaceAll(snippet, "\r\n", "\n")
	replaced, err := ReplaceBlock(d.raw, d.blocks, key, snippet)
	if err != nil {
		return d, err
	}
	savedFuture := d.future
	d = d.snapshot()
	d.raw = replaced
	blocks, err := ParseBlocks(replaced)
	if err != nil {
		d = d.rollback()
		d.future = savedFuture
		return d, fmt.Errorf("reparsing after replace: %w", err)
	}
	d.blocks = blocks

	if recovered, err2 := BlockContent(d.raw, d.blocks, key); err2 == nil {
		if !blockSemanticEqual(snippet, recovered) {
			d = d.rollback()
			d.future = savedFuture
			return d, fmt.Errorf("round-trip verification failed after replace of %q", key)
		}
	}
	return d, nil
}

// blockSemanticEqual compares structure and values, ignoring formatting, key
// order, and quoting. Fails closed: a parse error returns false, so an
// unverifiable round-trip rolls back instead of accepting possibly corrupted
// content (see TestBlockSemanticEqual_roundtripComparison).
func blockSemanticEqual(a, b string) bool {
	var va, vb any
	if err := yaml.Unmarshal([]byte(a), &va); err != nil {
		return false
	}
	if err := yaml.Unmarshal([]byte(b), &vb); err != nil {
		return false
	}
	ra, err := yaml.Marshal(va)
	if err != nil {
		return false
	}
	rb, err := yaml.Marshal(vb)
	if err != nil {
		return false
	}
	return bytes.Equal(ra, rb)
}

// ReplaceRaw swaps the whole document content, normalising CRLF, and leaves it
// untouched when raw fails to parse. Does not snapshot: only committed block
// operations (Insert, Replace, Remove) are undoable.
func (d Document) ReplaceRaw(raw []byte) (Document, error) {
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	blocks, err := ParseBlocks(raw)
	if err != nil {
		return d, err
	}
	d.raw = raw
	d.blocks = blocks
	return d, nil
}

// Undo restores the previous raw and pushes the undone state onto the redo
// stack. Returns false when history is empty or the snapshot no longer parses,
// keeping the current consistent state. The pop is copy-on-write: sibling
// Document copies share the stack's backing array.
func (d Document) Undo() (Document, bool) {
	if len(d.history) == 0 {
		return d, false
	}
	prev := d.history[len(d.history)-1]
	blocks, err := ParseBlocks(prev)
	if err != nil {
		return d, false
	}
	d.future = appendCapped(d.future, copyBytes(d.raw))
	d.history = cloneStack(d.history[:len(d.history)-1])
	d.raw = prev
	d.blocks = blocks
	return d, true
}

// Redo re-applies the most recently undone change, pushing the current state
// onto the undo history so the redo itself can be undone. Returns false when
// there is nothing to redo or the snapshot no longer parses. Copy-on-write,
// mirroring Undo.
func (d Document) Redo() (Document, bool) {
	if len(d.future) == 0 {
		return d, false
	}
	prev := d.future[len(d.future)-1]
	blocks, err := ParseBlocks(prev)
	if err != nil {
		return d, false
	}
	d.history = appendCapped(d.history, copyBytes(d.raw))
	d.future = cloneStack(d.future[:len(d.future)-1])
	d.raw = prev
	d.blocks = blocks
	return d, true
}

// rollback undoes the last snapshot without recording a redo entry: a state
// rejected by the round-trip check must not be reachable via Redo. An
// unparseable snapshot keeps the current state rather than leaving raw and
// blocks divergent. Copy-on-write, mirroring Undo.
func (d Document) rollback() Document {
	if len(d.history) == 0 {
		return d
	}
	snap := d.history[len(d.history)-1]
	blocks, err := ParseBlocks(snap)
	if err != nil {
		return d
	}
	d.history = cloneStack(d.history[:len(d.history)-1])
	d.raw = snap
	d.blocks = blocks
	return d
}

// Save atomically writes the current content to d.path and clears dirty. The
// file's existing mode is preserved (new files get 0600) and CRLF line endings
// are restored when the loaded file used them. Errors when d.path is empty.
func (d Document) Save() (Document, error) {
	if d.path == "" {
		return d, fmt.Errorf("document has no path; Load requires a path")
	}
	out := d.raw
	if d.usedCRLF {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	if err := atomicWrite(d.path, out); err != nil {
		return d, err
	}
	d.loaded = copyBytes(d.raw)
	d = d.recordDiskState()
	return d, nil
}

// Reload re-reads the source file, resetting raw, blocks, dirty, and the
// undo/redo history as if the document had just been loaded. The source is the
// load path even when SetPath pointed Save elsewhere; the save destination
// survives. A missing file reloads as empty, mirroring Load. On error the
// in-memory state is left untouched.
func (d Document) Reload() (Document, error) {
	src := d.loadPath
	if src == "" {
		src = d.path
	}
	if src == "" {
		return d, fmt.Errorf("document has no path; Load requires a path")
	}
	nd, err := Load(src, d.knownOrder)
	if err != nil {
		return d, err
	}
	if d.path != src {
		nd = nd.SetPath(d.path)
	}
	return nd, nil
}

// MarkSaved applies a completed Save onto d. Save runs on a snapshot, so by the
// time its result arrives d may carry newer edits; replacing d wholesale would
// drop them. Only the persistence state (loaded, mtime/size) is copied, and
// Dirty() follows from the current content.
func (d Document) MarkSaved(saved Document) Document {
	d.loaded = copyBytes(saved.loaded)
	d.diskModTime = saved.diskModTime
	d.diskSize = saved.diskSize
	return d
}

// ExternallyChanged reports whether the file on disk was modified since this
// Document last loaded or saved it. False when there is no path or the file is
// absent: a save would create it, clobbering nothing. Callers should confirm
// with the user before overwriting.
func (d Document) ExternallyChanged() bool {
	if d.path == "" {
		return false
	}
	info, err := os.Stat(d.path)
	if err != nil {
		return false
	}
	return info.ModTime() != d.diskModTime || info.Size() != d.diskSize
}

// recordDiskState captures the on-disk mtime/size for ExternallyChanged. A
// missing file records the zero state.
func (d Document) recordDiskState() Document {
	if d.path == "" {
		return d
	}
	if info, err := os.Stat(d.path); err == nil {
		d.diskModTime = info.ModTime()
		d.diskSize = info.Size()
	} else {
		d.diskModTime = time.Time{}
		d.diskSize = 0
	}
	return d
}

// atomicWrite writes a temp file in the same directory, fsyncs it, then renames
// over path, so a crash mid-write never truncates the original. The
// destination's mode is preserved; new files get 0600. Any failure before the
// rename removes the temp file and leaves path untouched.
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
