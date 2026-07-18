package editor

import (
	"fmt"
	"reflect"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/yamlnode"
)

// topBE returns a copy of the active (deepest) block editor, or nil when none
// is open. Callers read or mutate it freely and persist changes via withTopBE.
func (m model) topBE() *blockEditState {
	if len(m.blockEdits) == 0 {
		return nil
	}
	be := m.blockEdits[len(m.blockEdits)-1]
	return &be
}

// withTopBE returns a new model with be replacing the active block editor.
// It allocates a new slice so the caller's model and any prior copies do not
// share the same backing array.
func (m model) withTopBE(be blockEditState) model {
	if len(m.blockEdits) == 0 {
		return m
	}
	updated := make([]blockEditState, len(m.blockEdits))
	copy(updated, m.blockEdits)
	updated[len(updated)-1] = be
	m.blockEdits = updated
	return m
}

// withTopBEError sets a sticky error on the active block editor's feedback
// line - the channel actually rendered while a block editor is open (the root
// status line is not drawn in paneBlockEdit, so m.statusMsg would be invisible
// there and could resurface stale once back at the list). Falls back to the
// root sticky status when no editor is open.
func (m model) withTopBEError(kind errKind, msg string) model {
	top := m.topBE()
	if top == nil {
		return m.withStickyError(msg)
	}
	be := *top
	be.editorErr = editorError{kind: kind, message: msg}
	return m.withTopBE(be)
}

// --- Screen transitions ---
//
// The enter* helpers (root.go) are the only functions that assign m.mode. Each
// sets the active pane together with the data that pane owns, so the invariants
//
//	alertVisible          ⟹  mode == paneAlert
//	mode == paneBlockEdit ⟹  len(blockEdits) > 0
//
// cannot be violated by a caller that forgets to clear a sibling field. The
// arrows are one-way on purpose: enterAlert preserves blockEdits so that
// dismissing a root-level alert can return to the block editor underneath.

// handleBlockEditDiscarded pops the active block editor after the user closed
// it with Esc, returning to the parent editor or - from the top level - to the
// list, with explicit feedback only when changes were actually thrown away.
func (m model) handleBlockEditDiscarded(msg blockEditDiscardedMsg) (tea.Model, tea.Cmd) {
	if len(m.blockEdits) > 0 {
		m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
	}
	if len(m.blockEdits) == 0 {
		m = m.enterList()
		if msg.discarded {
			// User threw away uncommitted changes - show explicit feedback.
			return m.withStatus("Cancelled.")
		}
		// else: clean Esc after a commit - preserve the existing status message
		// (e.g. "Block updated (not saved yet)").
	}
	// Intermediate pops (returning to a parent editor) intentionally preserve
	// any status message the child may have set so the user can read it.
	return m, nil
}

// handleDrillOut navigates up one level while keeping edits. The current (child)
// editor is flushed into editRoot, popped, and the parent editor is refreshed
// from editRoot so it reflects what the child changed. Editing a child and
// returning to fix a parent field is therefore non-destructive - nothing is
// committed to the document until Ctrl+S. Only fired for nested editors.
func (m model) handleDrillOut() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) <= 1 {
		return m, nil
	}
	childWasDirty := m.topBE().dirty
	// Capture child focus before the stack is popped so we can scope pruning.
	childFocus := append([]pathSeg(nil), m.topBE().focus...)
	// The cascade below must never remove the parent editor's own focus node:
	// when the parent's mapping contained nothing but the drilled-into child,
	// pruning all the way up would delete it and refreshTopFromRoot would land
	// on a lost focus path. Everything at or above the parent's focus is the
	// parent's to prune on its own drill-out.
	parentFocusLen := len(m.blockEdits[len(m.blockEdits)-2].focus)

	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		// Invalid YAML in the child - cannot write it into the canonical tree.
		// The error is already shown; stay so the user can fix it.
		return m, nil
	}
	// Narrow prune: target the child's own node first so we don't accidentally
	// remove empty placeholders the user left in sibling fields, then remove any
	// mapping pairs along the child's path that the flush left empty (e.g. the
	// phantom "<key>:" of a drilled-into field the user never filled). The prune
	// must stay on this path and must never remove sequence items: editors still
	// on the stack address entries by index (segIdx), so removing an empty entry
	// elsewhere in a collection would silently re-point them at a different entry.
	if childNode := nodeAt(m.editRoot, childFocus); childNode != nil {
		pruneEmptyMappings(childNode)
	}
	pruneEmptyAlongFocus(m.editRoot, childFocus, parentFocusLen)

	m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]

	// Refresh the parent FIRST, then snapshot the refreshed state so Ctrl+U
	// restores the post-drill-out content (not the stale pre-refresh snapshot).
	m = m.refreshTopFromRoot()
	if childWasDirty {
		if top := m.topBE(); top != nil {
			be := top.saveUndo()
			m = m.withTopBE(be)
		}
	}
	return m, nil
}

// refreshCollectionFromNode updates a collection-nav editor in-place from node,
// rebuilding the tree when the entry count changes and re-anchoring the cursor
// on the previously viewed entry so it stays in view after removals.
func (be blockEditState) refreshCollectionFromNode(node *yaml.Node) blockEditState {
	isMap := be.isMapNav()
	old := be.node
	oldCount := entryCount(&old, isMap)
	be.node = *yamlnode.CloneNode(node)
	newCount := entryCount(&be.node, isMap)
	if newCount != oldCount {
		be.tree.nodes = be.collectionTreeNodes()
		// The rebuilt tree may be shorter than the cursor position (e.g. entries
		// pruned during drill-out); clamp so the cursor stays on a real row.
		be.tree = be.tree.clampCursor()
		be.coll.current = reanchorCollCursor(&old, &be.node, isMap, be.coll.current)
	}
	be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
	return be
}

// reanchorCollCursor locates the entry the user was viewing (index cur in
// oldNode) inside the refreshed newNode: map entries are matched by key,
// sequence entries by structural equality. Entries can be removed anywhere in
// the collection (e.g. pruning of empty mappings), so a positional shift would
// guess wrong; identity matching finds the entry wherever it landed. When it
// cannot be found (removed, or its content was edited), cur is clamped to the
// new bounds.
func reanchorCollCursor(oldNode, newNode *yaml.Node, isMap bool, cur int) int {
	if i := findEntryIndex(oldNode, newNode, isMap, cur); i >= 0 {
		return i
	}
	if newCount := entryCount(newNode, isMap); cur >= newCount {
		return newCount - 1
	}
	return cur
}

// findEntryIndex returns the index in newNode of the entry at index cur in
// oldNode - matched by key for maps, by structural equality for sequences -
// or -1 when the entry cannot be located.
func findEntryIndex(oldNode, newNode *yaml.Node, isMap bool, cur int) int {
	if cur < 0 || cur >= entryCount(oldNode, isMap) {
		return -1
	}
	newCount := entryCount(newNode, isMap)
	if isMap {
		key := entryLabel(oldNode, true, cur)
		for i := 0; i < newCount; i++ {
			if entryLabel(newNode, true, i) == key {
				return i
			}
		}
		return -1
	}
	val := entryValueNode(oldNode, false, cur)
	if val == nil {
		return -1
	}
	// Prefer the same position: with structurally identical duplicate entries
	// (e.g. right after duplicating one) a first-match scan would re-anchor the
	// cursor onto a different entry than the one the user was viewing.
	if cur < newCount && reflect.DeepEqual(entryValueNode(newNode, false, cur), val) {
		return cur
	}
	for i := 0; i < newCount; i++ {
		if reflect.DeepEqual(entryValueNode(newNode, false, i), val) {
			return i
		}
	}
	return -1
}

// refreshTopFromRoot rebuilds the active editor's content from the node at its
// focus path in editRoot, preserving tree cursor/expansion and the current
// collection entry. The dirty flag is recomputed from the refreshed content,
// so uncommitted child edits reach the top-level "Discard changes?" guard
// without explicit plumbing.
func (m model) refreshTopFromRoot() model {
	top := m.topBE()
	if top == nil {
		return m
	}
	node := nodeAt(m.editRoot, top.focus)
	if node == nil {
		return m.withTopBEError(errBlocked, "internal: focus path lost after drill-out; editor may show stale content")
	}
	be := *top
	if be.isCollectionNav() {
		be = be.refreshCollectionFromNode(node)
	} else {
		be.node = *yamlnode.CloneNode(node)
		be.yamlEditor.SetValue(nodeToContent(be.key, &be.node))
	}
	be.tree = be.resyncTreeFromYAML()
	be.dirty = be.computeDirty()
	return m.withTopBE(be)
}

func (m model) handlePaneBlockEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	top := m.topBE()
	if top == nil {
		m = m.enterList()
		return m, nil
	}
	// One router: the block editor's own Update handles every message (mode
	// switch, keys, resize) and emits model-level concerns (commit, drill,
	// discard) as messages that the root Update routes.
	be, cmd := top.Update(msg)
	return m.withTopBE(be), cmd
}

func (m model) handleOpenItem(it listItem) (tea.Model, tea.Cmd) {
	if m.mode == paneBlockEdit {
		return m, nil // stale Cmd: editor is already open, discard
	}
	var initial string
	if it.Existing {
		current, err := m.doc.BlockContent(it.Key)
		if err != nil {
			return m.withStickyError(fmt.Sprintf("Error reading %s: %v", it.Key, err)), nil
		}
		initial = current
	} else {
		initial = it.Key + ":\n"
	}

	children := applyPresentation(m.childrenOf[it.Key], m.cfg.Metadata, it.Key, nil)
	kind := fieldKind(m.schemaTree, it.Key)
	// Unknown items have no schema, so skip unknown-key validation inside the overlay.
	knownByPath := m.knownByPath
	if it.Unknown {
		knownByPath = nil
	}
	be := newBlockEdit(m.cfg, blockSpec{key: it.Key, defs: children, kind: kind, def: fieldDefByName(m.schemaTree, it.Key), content: initial, knownByPath: knownByPath}, m.width, m.height)
	be.isEdit = it.Existing
	be.focus = nil // top-level editor edits the whole block
	m.blockEdits = []blockEditState{be}
	m.editBlockKey = it.Key
	// Canonical tree, refreshed from the top editor on every flush (drill-in /
	// commit). A non-nil placeholder is enough; the first flush populates it.
	m.editRoot = &yaml.Node{Kind: yaml.MappingNode}
	m = m.enterBlockEdit()
	return m, be.Init()
}

// flushTopToRoot commits the active editor and writes its value node into
// editRoot at the editor's focus path. Returns (updatedModel, true) on success;
// on a validation error it sets the editor's error and returns false so the
// caller aborts the navigation/commit.
func (m model) flushTopToRoot() (model, bool) {
	top := m.topBE()
	if top == nil {
		return m, false
	}
	committed, val, ok := top.commit()
	m = m.withTopBE(committed)
	if !ok {
		// committed.editorErr carries the detail and the editor's own feedback
		// line renders it - the root status line is not visible in this mode.
		return m, false
	}
	rootSnap := yamlnode.CloneNode(m.editRoot)
	if err := setNodeAt(m.editRoot, committed.focus, val); err != nil {
		*m.editRoot = *rootSnap
		return m.withTopBEError(errCommit, fmt.Sprintf("internal error: could not write editor into canonical tree: %v", err)), false
	}
	return m, true
}

// handleOpenChild drills into a nested field. It flushes the parent editor into
// the canonical editRoot, then builds the child editor from the node living at
// the child's focus path within that same tree - no substring copy. Unknown-key
// validation is left to the parent, so the child uses a nil knownByPath (its
// root key is the field name, which would otherwise read as an unknown key).
func (m model) handleOpenChild(msg openChildMsg) (tea.Model, tea.Cmd) {
	// Guard against stale openChildMsg arriving with an empty blockEdits stack.
	top := m.topBE()
	if top == nil {
		return m, nil
	}

	const maxNestingDepth = 10
	if len(m.blockEdits) >= maxNestingDepth {
		return m.withTopBEError(errBlocked, fmt.Sprintf("Maximum nesting depth (%d) reached.", maxNestingDepth)), nil
	}

	// Flush the parent into editRoot so the child reads the parent's live state.
	parentFocus := append([]pathSeg(nil), top.focus...)
	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		return m, nil
	}

	childFocus := append([]pathSeg(nil), parentFocus...)
	childFocus = append(childFocus, msg.relSegs...)
	content := msg.key + ":\n"
	if node := nodeAt(m.editRoot, childFocus); node != nil {
		content = nodeToContent(msg.key, node)
	}
	// focusToStringPath drops index segments and runtime map-entry keys (marked
	// by segMapKey at emit time), so the prefix holds only schema field names -
	// including entry keys of map-nav ancestors further up the focus path.
	metaPrefix := focusToStringPath(childFocus)
	defs := applyPresentation(msg.defs, m.cfg.Metadata, m.editBlockKey, metaPrefix)
	be := newBlockEdit(m.cfg, blockSpec{key: msg.key, defs: defs, kind: msg.kind, content: content, knownByPath: nil}, m.width, m.height)
	be.isEdit = true
	be.focus = childFocus

	m.blockEdits = append(m.blockEdits, be)
	m = m.enterBlockEdit()
	return m, be.Init()
}

// docWithEditorContent returns a copy of m.doc with the open editor stack's
// current content applied - the document that WOULD result from committing
// now. Used by validation so ctrl+l inside an editor reflects the on-screen
// content. The caller must have flushed the top editor into editRoot first
// (flushTopToRoot); editRoot is cloned here so the pruning never mutates the
// live edit session. Mirrors commitAll's serialization, minus the effects.
func (m model) docWithEditorContent() (document.Document, error) {
	if len(m.blockEdits) == 0 {
		return m.doc, nil
	}
	root := yamlnode.CloneNode(m.editRoot)
	pruneEmptyContent(root)
	blockIsEmpty := len(root.Content) == 0 &&
		(root.Kind == yaml.MappingNode || root.Kind == yaml.SequenceNode)
	isEdit := m.blockEdits[0].isEdit
	switch {
	case blockIsEmpty && isEdit:
		return m.doc.Remove(m.editBlockKey)
	case blockIsEmpty:
		return m.doc, nil
	case isEdit:
		return m.doc.Replace(m.editBlockKey, nodeToContent(m.editBlockKey, root))
	default:
		return m.doc.Insert(nodeToContent(m.editBlockKey, root))
	}
}

// saveAll is the Ctrl+S handler. When block editors are open it commits all
// stacked editors into m.doc and returns to the list - file save is a separate
// action triggered by Ctrl+S from the list view. When no editors are open it
// saves the file directly.
func (m model) saveAll() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) > 0 {
		return m.commitAll()
	}
	return m.save()
}

// commitAll commits the open editor stack into m.doc and returns to the list
// without writing the file. Because every drill-in already flushed its parent
// into editRoot, only the active (top) editor is still live: flush it, then
// serialize the whole canonical tree once. No per-level string splicing.
func (m model) commitAll() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) == 0 {
		return m, nil
	}
	isEdit := m.blockEdits[0].isEdit

	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		return m, nil
	}

	pruneEmptyContent(m.editRoot)
	blockIsEmpty := len(m.editRoot.Content) == 0 &&
		(m.editRoot.Kind == yaml.MappingNode || m.editRoot.Kind == yaml.SequenceNode)
	var err error
	unchanged := false
	switch {
	case blockIsEmpty && isEdit:
		m.doc, err = m.doc.Remove(m.editBlockKey)
	case blockIsEmpty && !isEdit:
		// Nothing was added — return to list without dirtying the document.
		m = m.syncView()
		m = m.enterList()
		return m.withStatus("Nothing added.")
	case !blockIsEmpty:
		final := nodeToContent(m.editBlockKey, m.editRoot)
		if isEdit {
			current, readErr := m.doc.BlockContent(m.editBlockKey)
			if readErr != nil {
				// A failed read must not be treated as "content changed" - the
				// Replace below would run against unknown document state.
				return m.withTopBEError(errCommit, fmt.Sprintf("Apply error: %v", readErr)), nil
			}
			if normalizeBlockContent(m.editBlockKey, current) != final {
				m.doc, err = m.doc.Replace(m.editBlockKey, final)
			} else {
				unchanged = true
			}
		} else {
			m.doc, err = m.doc.Insert(final)
		}
	}
	if err != nil {
		return m.withTopBEError(errCommit, fmt.Sprintf("Apply error: %v", err)), nil
	}
	m = m.syncView()
	m = m.enterList()
	if unchanged {
		return m.withStatus("No changes to commit.")
	}
	return m.withStatus("Changes committed (not saved yet) - ctrl+s to save.")
}
