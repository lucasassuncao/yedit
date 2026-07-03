package editor

import (
	"fmt"
	"strings"

	"github.com/lucasassuncao/yedit/schema"
)

// collectionBuffer tracks which entry of a collection-nav editor is currently
// shown in the YAML editor. The entry list itself is no longer stored here - it
// is derived structurally from blockEditState.node, the single source of truth.
type collectionBuffer struct {
	key     string
	isMap   bool
	current int // index of the entry shown in yamlEditor (-1 if empty)
}

// collectionDeriveTree refreshes every entry's label, yamlPath, and child
// checkmarks from be.node, preserving the tree's structure (expansion/cursor).
// It is the structural replacement for syncCurrentEntry - and unlike it, derives
// all entries (not just the current one) from the single source of truth.
func (be blockEditState) collectionDeriveTree() treeModel {
	tm := be.tree
	isMap := be.coll.isMap
	nodes := make([]treeNode, len(tm.nodes))
	copy(nodes, tm.nodes)
	for i := 0; i < len(nodes); i++ {
		if nodes[i].kind != treeNodeSeqItem {
			continue
		}
		seqIdx := nodes[i].seqIdx
		label := entryLabel(&be.node, isMap, seqIdx)
		if label != "" {
			nodes[i].label = label
			nodes[i].yamlPath = []string{label}
		}
		var childIdx []int
		for j := i + 1; j < len(nodes) && nodes[j].depth > 0; j++ {
			if label != "" && len(nodes[j].yamlPath) > 0 {
				p := append([]string(nil), nodes[j].yamlPath...)
				p[0] = label
				nodes[j].yamlPath = p
			}
			childIdx = append(childIdx, j)
		}
		sub := make([]treeNode, len(childIdx))
		for k, ci := range childIdx {
			sub[k] = nodes[ci]
		}
		sub = deriveChecked(entryValueNode(&be.node, isMap, seqIdx), sub, true)
		for k, ci := range childIdx {
			nodes[ci] = sub[k]
		}
	}
	tm.nodes = nodes
	return tm
}

// performEntryDelete removes collection entry seqIdx from both the tree and
// the canonical node. saveUndo runs before either is mutated, so the snapshot
// captures the pre-deletion state directly and ctrl+u restores the entry.
func (be blockEditState) performEntryDelete(seqIdx int) blockEditState {
	// Flush the current entry so unsaved edits are not lost when we delete
	// a different entry that shifts the canonical node.
	be = be.flushCurrentEntry()
	be.editorErr = editorError{} // deletion overrides a pending parse error
	be = be.saveUndo()
	be.tree = be.tree.WithDeletedSeqItem(seqIdx)
	removeEntry(&be.node, be.coll.isMap, seqIdx)
	return be.loadEntry(be.tree.NearestSeqItem())
}

// flushAndLoadEntry flushes the current entry into be.node and then loads the
// entry at idx. If the flush fails (invalid YAML), be.editorErr is set and the
// caller should surface it without navigating.
func (be blockEditState) flushAndLoadEntry(idx int) blockEditState {
	be = be.flushCurrentEntry()
	if be.editorErr.kind == errParse {
		return be
	}
	return be.loadEntry(idx)
}

// initialSeqItemContent returns a minimal YAML template for a new sequence item.
// Uses the first child field name so the initial content matches the actual schema.
func (be blockEditState) initialSeqItemContent(label string) string {
	if len(be.childDefs) == 0 {
		return "  - \n"
	}
	first := be.childDefs[0].YAMLName
	if first == "name" {
		return "  - name: \"" + label + "\"\n"
	}
	return "  - " + first + ": \"\"\n"
}

// --- Collection navigator: shared by structured sequences and structured maps ---

// isSeqNav reports whether this block is a structured sequence ([]Struct).
func (be blockEditState) isSeqNav() bool {
	return be.kind == schema.KindList && len(be.childDefs) > 0
}

// isMapNav reports whether this block is a structured map (map[string]Struct).
func (be blockEditState) isMapNav() bool {
	return be.kind == schema.KindDictionary && len(be.childDefs) > 0
}

// isCollectionNav reports whether this block uses the [N] / [+ add new] navigator.
func (be blockEditState) isCollectionNav() bool {
	return be.isSeqNav() || be.isMapNav()
}

// collectionTreeNodes rebuilds the tree nodes for the current collection entries,
// picking the map or sequence layout from the block kind.
func (be blockEditState) collectionTreeNodes() []treeNode {
	return buildCollectionNodesFromNode(be.childDefs, &be.node, be.isMapNav())
}

// flushCurrentEntry parses the current entry's editor text back into the
// canonical node. It is a no-op when there is no current entry or the editor is
// empty. When the text cannot be parsed into an entry (e.g. the user deleted the
// "key:" header, or it is mid-edit invalid), be.editorErr is set so callers block
// navigation or commit - the parse gate that keeps the node valid.
func (be blockEditState) flushCurrentEntry() blockEditState {
	cur := be.coll.current
	if cur < 0 || cur >= entryCount(&be.node, be.coll.isMap) {
		be.editorErr = editorError{}
		return be
	}
	view := be.yamlEditor.Value()
	if strings.TrimSpace(view) == "" {
		be.editorErr = editorError{}
		return be
	}
	if !be.coll.isMap && viewHasMultipleSeqItems(view) {
		be.editorErr = editorError{kind: errParse, message: "One entry per editor - use [+ add new] to create additional entries."}
		return be
	}
	kn, vn, ok := parseEntryFromView(view, be.coll.isMap)
	if !ok {
		msg := "Invalid YAML - fix this entry before leaving it."
		if !strings.HasPrefix(view, be.key+":") {
			msg = "Missing '" + be.key + ":' header - restore it before navigating."
		}
		be.editorErr = editorError{kind: errParse, message: msg}
		return be
	}
	// Guard against map key renames that would create a duplicate: if the new
	// key already exists at a different position in the canonical node, reject
	// the flush to prevent a corrupt YAML mapping with two identical keys.
	if be.coll.isMap {
		newKey := kn.Value
		count := entryCount(&be.node, be.coll.isMap)
		for i := 0; i < count; i++ {
			if i != cur && entryLabel(&be.node, true, i) == newKey {
				be.editorErr = editorError{kind: errParse, message: fmt.Sprintf("Duplicate map key %q - rename it to a unique key first.", newKey)}
				return be
			}
		}
	}
	setEntry(&be.node, be.coll.isMap, cur, kn, vn)
	be.editorErr = editorError{}
	return be
}

// loadEntry shows entry idx in the editor.
// Always call flushCurrentEntry before loadEntry when switching entries.
// idx is clamped to [0, entryCount-1]; an empty collection sets current=-1.
func (be blockEditState) loadEntry(idx int) blockEditState {
	count := entryCount(&be.node, be.coll.isMap)
	if count == 0 {
		be.coll.current = -1
		be.yamlEditor.SetValue(be.entryYAML(-1))
		return be
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= count {
		idx = count - 1
	}
	be.coll.current = idx
	be.yamlEditor.SetValue(be.entryYAML(idx))
	return be
}

// entryYAML returns the single-entry editor view for index idx.
func (be blockEditState) entryYAML(idx int) string {
	return entryViewYAML(&be.node, be.key, be.coll.isMap, idx)
}

// initialEntryContent returns the YAML template for a freshly added entry.
func (be blockEditState) initialEntryContent(label string) string {
	if be.isMapNav() {
		return "  " + label + ":\n    " + be.childDefs[0].YAMLName + ": \"\"\n"
	}
	return be.initialSeqItemContent(label)
}

// newEntryLabel is the label for a freshly added entry: a placeholder key for
// maps (the user renames it in the YAML pane), or "" for sequences (auto "item N").
// For maps, uniqueness is checked against the canonical node (not the tree) so
// stale tree labels after an undo cannot generate duplicate keys.
func (be blockEditState) newEntryLabel() string {
	if !be.isMapNav() {
		return ""
	}
	// Build the set of existing keys from the canonical node, which is always
	// up to date even when the tree is stale after an undo/redo.
	count := entryCount(&be.node, be.coll.isMap)
	existing := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		existing[entryLabel(&be.node, true, i)] = true
	}
	// Start at count+1 for predictable positional labels, but increment past
	// any key that already exists so we never produce a duplicate map key.
	for n := count + 1; ; n++ {
		label := fmt.Sprintf("key%d", n)
		if !existing[label] {
			return label
		}
	}
}
