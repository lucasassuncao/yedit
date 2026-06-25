package editor

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

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

// --- Screen transitions ---
//
// The enter* helpers (root.go) are the only functions that assign m.mode. Each
// sets the active pane together with the data that pane owns, so the invariants
//
//	alertVisible           ⟺  mode == paneAlert
//	len(blockEdits) > 0     ⟺  mode == paneBlockEdit
//
// cannot be violated by a caller that forgets to clear a sibling field. The
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
	} else {
		m.statusMsg = ""
	}
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

	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		// Invalid YAML in the child - cannot write it into the canonical tree.
		// The error is already shown; stay so the user can fix it.
		return m, nil
	}
	m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
	// Capture the parent's pre-drill-in state before applying the child's
	// changes, so Ctrl+U on the parent can undo the drill-in as one step.
	if childWasDirty {
		if top := m.topBE(); top != nil {
			be := top.saveUndo()
			m = m.withTopBE(be)
		}
	}
	m = m.refreshTopFromRoot(childWasDirty)
	m.statusMsg = ""
	return m, nil
}

// refreshCollectionFromNode updates a collection-nav editor in-place from node,
// rebuilding the tree when the entry count changes and adjusting the cursor so
// the previously viewed entry stays in view after removals.
func (be *blockEditState) refreshCollectionFromNode(node *yaml.Node) {
	isMap := be.isMapNav()
	oldCount := entryCount(&be.node, isMap)
	be.node = *yamlnode.CloneNode(node)
	newCount := entryCount(&be.node, isMap)
	if newCount != oldCount {
		be.tree.nodes = be.collectionTreeNodes()
		be.coll.current = clampCollCursor(be.coll.current, oldCount, newCount)
	}
	be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
}

// clampCollCursor adjusts the cursor after a collection's entry count changes.
// When entries were removed it shifts the cursor down by the removed count so
// the viewed entry is preserved; then clamps to [0, newCount-1].
func clampCollCursor(current, oldCount, newCount int) int {
	if newCount < oldCount {
		if current >= oldCount-newCount {
			current -= oldCount - newCount
		} else {
			current = 0
		}
	}
	if current >= newCount {
		return newCount - 1
	}
	return current
}

// refreshTopFromRoot rebuilds the active editor's content from the node at its
// focus path in editRoot, preserving tree cursor/expansion and the current
// collection entry. markDirty propagates uncommitted-changes state up from a
// child so the top-level "Discard changes?" guard still fires.
func (m model) refreshTopFromRoot(markDirty bool) model {
	top := m.topBE()
	if top == nil {
		return m
	}
	node := nodeAt(m.editRoot, top.focus)
	if node == nil {
		return m
	}
	be := *top
	if be.isCollectionNav() {
		be.refreshCollectionFromNode(node)
	} else {
		be.node = *yamlnode.CloneNode(node)
		be.yamlEditor.SetValue(nodeToContent(be.key, &be.node))
	}
	be.tree = be.resyncTreeFromYAML()
	if markDirty {
		be.dirty = true
	}
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
	var initial string
	if it.Existing {
		current, err := m.doc.BlockContent(it.Key)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error reading %s: %v", it.Key, err)
			return m, nil
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
	committed, snippet, ok := top.commit()
	m = m.withTopBE(committed)
	if !ok {
		m.statusMsg = committed.editorErr.message
		return m, false
	}
	val := valueNodeOfSnippet(snippet)
	if val == nil {
		m.statusMsg = "internal error: could not write editor into canonical tree"
		return m, false
	}
	rootSnap := yamlnode.CloneNode(m.editRoot)
	if !setNodeAt(m.editRoot, committed.focus, val) {
		*m.editRoot = *rootSnap
		m.statusMsg = "internal error: could not write editor into canonical tree"
		return m, false
	}
	return m, true
}

// handleOpenChild drills into a nested field. It flushes the parent editor into
// the canonical editRoot, then builds the child editor from the node living at
// the child's focus path within that same tree - no substring copy. Unknown-key
// validation is left to the parent, so the child uses a nil knownByPath (its
// root key is the field name, which would otherwise read as an unknown key).
func (m model) handleOpenChild(msg openChildMsg) (tea.Model, tea.Cmd) {
	const maxNestingDepth = 10
	if len(m.blockEdits) >= maxNestingDepth {
		m.statusMsg = fmt.Sprintf("Maximum nesting depth (%d) reached.", maxNestingDepth)
		return m, nil
	}

	// Flush the parent into editRoot so the child reads the parent's live state.
	parentFocus := append([]pathSeg(nil), m.topBE().focus...)
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
	// For map-nav collections, relSegs[0] is the runtime map entry key (not a
	// schema field name) and must be excluded from the metadata lookup prefix.
	metaPrefix := focusToStringPath(childFocus)
	if m.topBE().isCollectionNav() && m.topBE().coll.isMap && len(msg.relSegs) > 0 {
		metaPrefix = focusToStringPath(append(append([]pathSeg(nil), parentFocus...), msg.relSegs[1:]...))
	}
	defs := applyPresentation(msg.defs, m.cfg.Metadata, m.editBlockKey, metaPrefix)
	be := newBlockEdit(m.cfg, blockSpec{key: msg.key, defs: defs, kind: msg.kind, content: content, knownByPath: nil}, m.width, m.height)
	be.isEdit = true
	be.focus = childFocus

	m.blockEdits = append(m.blockEdits, be)
	m = m.enterBlockEdit()
	return m, be.Init()
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
	switch {
	case blockIsEmpty && isEdit:
		m.doc, err = m.doc.Remove(m.editBlockKey)
	case !blockIsEmpty:
		final := nodeToContent(m.editBlockKey, m.editRoot)
		if isEdit {
			if current, _ := m.doc.BlockContent(m.editBlockKey); normalizeBlockContent(m.editBlockKey, current) != final {
				m.doc, err = m.doc.Replace(m.editBlockKey, final)
			}
		} else {
			m.doc, err = m.doc.Insert(final)
		}
	}
	if err != nil {
		m.statusMsg = fmt.Sprintf("Apply error: %v", err)
		return m, nil
	}
	m = m.syncView()
	m = m.enterList()
	return m.withStatus("Changes committed (not saved yet) - ctrl+s to save.")
}
