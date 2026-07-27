package editor

import (
	"fmt"
	"reflect"

	tea "charm.land/bubbletea/v2"
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

// withTopBE replaces the active block editor, allocating a new slice so prior
// model copies do not share the backing array.
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

// withTopBEError sets a sticky error on the active block editor's feedback line,
// the only one rendered in paneBlockEdit: m.statusMsg would be invisible there
// and could resurface stale back at the list. Falls back to the root sticky
// status when no editor is open.
func (m model) withTopBEError(kind errKind, msg string) model {
	top := m.topBE()
	if top == nil {
		return m.withStickyError(msg)
	}
	be := *top
	be.editorErr = editorError{kind: kind, message: msg}
	return m.withTopBE(be)
}

// The enter* helpers in root.go are the only functions that assign m.mode. Each
// sets the active pane together with the data that pane owns, so the invariants
//
//	alertVisible          ⟹  mode == paneAlert
//	mode == paneBlockEdit ⟹  len(blockEdits) > 0
//
// survive a caller that forgets to clear a sibling field. The arrows are one-way
// on purpose: enterAlert preserves blockEdits so dismissing a root-level alert
// can return to the block editor underneath.

// handleBlockEditDiscarded pops the active block editor, returning to the parent
// editor or, from the top level, to the list. Explicit feedback appears only
// when changes were actually thrown away.
func (m model) handleBlockEditDiscarded(msg blockEditDiscardedMsg) (tea.Model, tea.Cmd) {
	if len(m.blockEdits) > 0 {
		m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
	}
	if len(m.blockEdits) == 0 {
		m = m.enterList()
		if msg.discarded {
			return m.withStatus("Cancelled.")
		}
		// A clean Esc after a commit keeps the existing status message.
	}
	// Intermediate pops preserve any status message the child set, so the user
	// can still read it.
	return m, nil
}

// handleDrillOut navigates up one level while keeping edits: the child editor is
// flushed into editRoot, popped, and the parent refreshed from editRoot. Nothing
// reaches the document until Ctrl+S. Nested editors only.
func (m model) handleDrillOut() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) <= 1 {
		return m, nil
	}
	childWasDirty := m.topBE().dirty
	// Capture child focus before the stack is popped so pruning stays scoped.
	childFocus := append([]pathSeg(nil), m.topBE().focus...)
	// Pruning must never reach the parent editor's own focus node: when the
	// parent held nothing but the drilled-into child, going all the way up would
	// delete it and refreshTopFromRoot would land on a lost path. Everything at
	// or above the parent's focus is the parent's to prune on its own drill-out.
	parentFocusLen := len(m.blockEdits[len(m.blockEdits)-2].focus)

	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		// Invalid YAML cannot be written into the canonical tree. The error is
		// already shown, so stay put and let the user fix it.
		return m, nil
	}
	// Prune the child's own node first, so empty placeholders in sibling fields
	// survive, then drop mapping pairs along the child's path that the flush left
	// empty. The prune stays on this path and never removes sequence items:
	// editors still on the stack address entries by index, so removing one
	// elsewhere would re-point them at a different entry.
	if childNode := nodeAt(m.editRoot, childFocus); childNode != nil {
		pruneEmptyMappings(childNode)
	}
	pruneEmptyAlongFocus(m.editRoot, childFocus, parentFocusLen)

	m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]

	// Refresh the parent first, then snapshot, so Ctrl+U restores the
	// post-drill-out content rather than the stale pre-refresh state.
	m = m.refreshTopFromRoot()
	if childWasDirty {
		if top := m.topBE(); top != nil {
			be := top.saveUndo()
			m = m.withTopBE(be)
		}
	}
	return m, nil
}

// refreshCollectionFromNode updates a collection-nav editor from node, rebuilding
// the tree when the entry count changes and re-anchoring the cursor on the
// previously viewed entry so it survives removals.
func (be blockEditState) refreshCollectionFromNode(node *yaml.Node) blockEditState {
	isMap := be.isMapNav()
	old := be.node
	oldCount := entryCount(&old, isMap)
	be.node = *yamlnode.CloneNode(node)
	newCount := entryCount(&be.node, isMap)
	if newCount != oldCount {
		be.tree.nodes = be.collectionTreeNodes()
		// The rebuilt tree may be shorter than the cursor position, so clamp it
		// back onto a real row.
		be.tree = be.tree.clampCursor()
		be.coll.current = reanchorCollCursor(&old, &be.node, isMap, be.coll.current)
	}
	be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
	return be
}

// reanchorCollCursor locates the entry at index cur of oldNode inside the
// refreshed newNode. Entries can be removed anywhere in the collection, so a
// positional shift would guess wrong; identity matching finds the entry wherever
// it landed. An entry that cannot be found leaves cur clamped to the new bounds.
func reanchorCollCursor(oldNode, newNode *yaml.Node, isMap bool, cur int) int {
	if i := findEntryIndex(oldNode, newNode, isMap, cur); i >= 0 {
		return i
	}
	if newCount := entryCount(newNode, isMap); cur >= newCount {
		return newCount - 1
	}
	return cur
}

// findEntryIndex locates oldNode's entry cur inside newNode, by key for maps and
// by structural equality for sequences, or -1 when it is gone.
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
	// Prefer the same position: among structurally identical entries a first-match
	// scan would re-anchor onto a different one than the user was viewing.
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

// refreshTopFromRoot rebuilds the active editor from the node at its focus path
// in editRoot, preserving tree cursor, expansion, and current entry. dirty is
// recomputed from the refreshed content, so uncommitted child edits reach the
// top-level "Discard changes?" guard without explicit plumbing.
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
	// One router: the block editor's Update handles every message and emits
	// model-level concerns as messages the root Update routes.
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
	// Unknown items have no schema, so skip unknown-key validation.
	knownByPath := m.knownByPath
	if it.Unknown {
		knownByPath = nil
	}
	be := newBlockEdit(m.cfg, blockSpec{key: it.Key, defs: children, kind: kind, def: fieldDefByName(m.schemaTree, it.Key), content: initial, knownByPath: knownByPath}, m.width, m.height)
	be.isEdit = it.Existing
	be.focus = nil // top-level editor edits the whole block
	m.blockEdits = []blockEditState{be}
	m.editBlockKey = it.Key
	// Canonical tree, refreshed from the top editor on every flush. A non-nil
	// placeholder is enough; the first flush populates it.
	m.editRoot = &yaml.Node{Kind: yaml.MappingNode}
	m = m.enterBlockEdit()
	return m, be.Init()
}

// flushTopToRoot commits the active editor and writes its value node into
// editRoot at the editor's focus path. A validation error sets the editor's
// error and returns false, so the caller aborts the navigation or commit.
func (m model) flushTopToRoot() (model, bool) {
	top := m.topBE()
	if top == nil {
		return m, false
	}
	committed, val, ok := top.commit()
	m = m.withTopBE(committed)
	if !ok {
		// committed.editorErr carries the detail and the editor's feedback line
		// renders it; the root status line is not visible in this mode.
		return m, false
	}
	rootSnap := yamlnode.CloneNode(m.editRoot)
	if err := setNodeAt(m.editRoot, committed.focus, val); err != nil {
		*m.editRoot = *rootSnap
		return m.withTopBEError(errCommit, fmt.Sprintf("internal error: could not write editor into canonical tree: %v", err)), false
	}
	return m, true
}

// handleOpenChild drills into a nested field: it flushes the parent into
// editRoot, then builds the child editor from the node at the child's focus path
// in that same tree, copying no substring. Unknown-key validation stays with the
// parent, so the child passes a nil knownByPath - its root key is the field
// name, which would otherwise read as an unknown key.
func (m model) handleOpenChild(msg openChildMsg) (tea.Model, tea.Cmd) {
	// Guard against a stale openChildMsg arriving with an empty stack.
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
	// focusToStringPath drops index segments and runtime map-entry keys, so the
	// prefix holds only schema field names.
	metaPrefix := focusToStringPath(childFocus)
	defs := applyPresentation(msg.defs, m.cfg.Metadata, m.editBlockKey, metaPrefix)
	be := newBlockEdit(m.cfg, blockSpec{key: msg.key, defs: defs, kind: msg.kind, content: content, knownByPath: nil}, m.width, m.height)
	be.isEdit = true
	be.focus = childFocus

	m.blockEdits = append(m.blockEdits, be)
	m = m.enterBlockEdit()
	return m, be.Init()
}

// docWithEditorContent returns the document that committing now would produce,
// so ctrl+l inside an editor validates the on-screen content. The caller must
// have run flushTopToRoot first; editRoot is cloned here so pruning never
// mutates the live session. Mirrors commitAll's serialization without the
// effects.
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

// saveAll is the Ctrl+S handler: it commits the open editor stack into m.doc, or
// saves the file when no editor is open. Writing the file is a separate action,
// triggered by Ctrl+S from the list view.
func (m model) saveAll() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) > 0 {
		return m.commitAll()
	}
	return m.save()
}

// commitAll commits the open editor stack into m.doc and returns to the list
// without writing the file. Every drill-in already flushed its parent into
// editRoot, so only the top editor is still live: flush it, then serialize the
// canonical tree once, with no per-level string splicing.
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
		// Nothing was added, so return to the list without dirtying the document.
		m = m.syncView()
		m = m.enterList()
		return m.withStatus("Nothing added.")
	case !blockIsEmpty:
		final := nodeToContent(m.editBlockKey, m.editRoot)
		if isEdit {
			current, readErr := m.doc.BlockContent(m.editBlockKey)
			if readErr != nil {
				// A failed read must not read as "content changed": Replace would
				// then run against unknown document state.
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
