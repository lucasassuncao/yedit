package editor

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/schema"
)

// collectionBuffer tracks which entry of a collection-nav editor is shown in the
// YAML editor. The entry list itself is derived from blockEditState.node.
type collectionBuffer struct {
	key     string
	isMap   bool
	current int // index of the entry shown in yamlEditor (-1 if empty)
}

// collectionDeriveTree refreshes every entry's label, yamlPath, and child
// checkmarks from be.node, preserving expansion and cursor.
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
		// A map entry keyed by the empty string still exists and must refresh the
		// row, or its label/yamlPath would point at a key that no longer exists.
		hasLabel := label != "" || (isMap && seqIdx < entryCount(&be.node, isMap))
		if hasLabel {
			display := label
			if display == "" {
				display = `""` // visible placeholder for an empty map key
			}
			nodes[i].label = display
			nodes[i].yamlPath = []string{label}
		}
		var childIdx []int
		for j := i + 1; j < len(nodes) && nodes[j].depth > 0; j++ {
			if hasLabel && len(nodes[j].yamlPath) > 0 {
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

// performEntryDelete removes collection entry seqIdx from both the tree and the
// canonical node. saveUndo runs first, so ctrl+u restores the entry.
func (be blockEditState) performEntryDelete(seqIdx int) blockEditState {
	// Deleting a different entry flushes the current one so its edits survive the
	// index shift; a flush failure means those edits are invalid and deleting
	// would revert them silently. Deleting the current entry discards its buffer
	// by design - that is the remedy the flush errors point the user to.
	if seqIdx != be.coll.current {
		be = be.flushCurrentEntry()
		if be.editorErr.kind == errParse {
			return be
		}
	}
	be.editorErr = editorError{}
	be = be.saveUndo()
	be.tree = be.tree.WithDeletedSeqItem(seqIdx)
	removeEntry(&be.node, be.coll.isMap, seqIdx)
	return be.loadEntry(be.tree.NearestSeqItem())
}

// flushAndLoadEntry flushes the current entry into be.node, then loads idx. A
// failed flush sets be.editorErr; the caller must surface it without navigating.
func (be blockEditState) flushAndLoadEntry(idx int) blockEditState {
	be = be.flushCurrentEntry()
	if be.editorErr.kind == errParse {
		return be
	}
	return be.loadEntry(idx)
}

// initialSeqItemContent is a minimal template for a new sequence item, keyed on
// the first child field so it matches the schema.
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
// canonical node, and is a no-op on an untouched placeholder. Text that will not
// parse into an entry sets be.editorErr so callers block navigation or commit:
// this is the parse gate that keeps the node valid.
func (be blockEditState) flushCurrentEntry() blockEditState {
	cur := be.coll.current
	view := be.yamlEditor.Value()
	if cur < 0 || cur >= entryCount(&be.node, be.coll.isMap) {
		// No current entry: a blank buffer or pristine placeholder is a clean
		// no-op. Anything else never parsed into a first entry (applyParsedEntry
		// appends it the moment it does), so committing would drop it.
		if strings.TrimSpace(view) == "" || view == be.entryYAML(-1) {
			be.editorErr = editorError{}
			return be
		}
		be.editorErr = editorError{kind: errParse, message: "Invalid YAML - fix this entry before committing."}
		return be
	}
	if strings.TrimSpace(view) == "" {
		// An emptied buffer treated as a no-op would resurrect the old content on
		// the next load, so require an explicit action.
		be.editorErr = editorError{kind: errParse, message: "Entry is empty - press ctrl+d on it in the tree to delete it, or restore its content."}
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
	// A rename onto a key that exists elsewhere would produce a mapping with two
	// identical keys, so reject the flush.
	if be.coll.isMap && duplicateMapKey(&be.node, cur, kn.Value) {
		be.editorErr = editorError{kind: errParse, message: fmt.Sprintf("Duplicate map key %q - rename it to a unique key first.", kn.Value)}
		return be
	}
	setEntry(&be.node, be.coll.isMap, cur, kn, vn)
	be.editorErr = editorError{}
	return be
}

// duplicateMapKey reports whether key exists at an index other than except.
// Shared by the flush and per-keystroke duplicate guards.
func duplicateMapKey(node *yaml.Node, except int, key string) bool {
	count := entryCount(node, true)
	for i := 0; i < count; i++ {
		if i != except && entryLabel(node, true, i) == key {
			return true
		}
	}
	return false
}

// loadEntry shows entry idx, clamped to [0, entryCount-1]; an empty collection
// sets current=-1. Always flushCurrentEntry first when switching entries.
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
// maps, which the user renames in the YAML pane, or "" for sequences (auto
// "item N").
func (be blockEditState) newEntryLabel() string {
	if !be.isMapNav() {
		return ""
	}
	// Existing keys come from the canonical node, which stays correct even when
	// the tree is stale after an undo/redo.
	count := entryCount(&be.node, be.coll.isMap)
	existing := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		existing[entryLabel(&be.node, true, i)] = true
	}
	// Start at count+1 for predictable positional labels, incrementing past any
	// key that already exists.
	for n := count + 1; ; n++ {
		label := fmt.Sprintf("key%d", n)
		if !existing[label] {
			return label
		}
	}
}
