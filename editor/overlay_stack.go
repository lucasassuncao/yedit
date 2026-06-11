package editor

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/internal/alert"
	"github.com/lucasassuncao/yedit/theme"
)

// blockBreadcrumbPrefix returns the breadcrumb segments for all editors in the
// stack except the top one. The top editor appends its own key and tree segments.
func (m model) blockBreadcrumbPrefix() []string {
	n := len(m.blockEdits)
	if n <= 1 {
		return nil
	}
	var segs []string
	for _, be := range m.blockEdits[:n-1] {
		segs = append(segs, be.key)
		segs = append(segs, be.tree.BreadcrumbSegments()...)
	}
	return segs
}

// topBE returns the active (deepest) block editor, or nil when none is open.
func (m *model) topBE() *blockEditState {
	if len(m.blockEdits) == 0 {
		return nil
	}
	return m.blockEdits[len(m.blockEdits)-1]
}

// setTopBE replaces the active block editor in place.
func (m *model) setTopBE(be *blockEditState) {
	if len(m.blockEdits) > 0 {
		m.blockEdits[len(m.blockEdits)-1] = be
	}
}

// --- Screen transitions ---
//
// These are the ONLY functions that assign m.mode. Each one sets the active
// pane together with the data that pane owns, so the two invariants
//
//	m.alert != nil        ⟺  m.mode == paneAlert
//	len(m.blockEdits) > 0  ⟺  m.mode == paneBlockEdit
//
// cannot be violated by a caller that forgets to clear a sibling field. The
func (m model) handleBlockEditDiscarded(msg blockEditDiscardedMsg) (tea.Model, tea.Cmd) {
	if len(m.blockEdits) > 0 {
		m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
	}
	if len(m.blockEdits) == 0 {
		m.enterList()
		if msg.discarded {
			// User threw away uncommitted changes — show explicit feedback.
			m.statusMsg = "Cancelled."
		}
		// else: clean Esc after a commit — preserve the existing status message
		// (e.g. "Block updated (not saved yet)").
	} else {
		m.statusMsg = ""
	}
	return m, nil
}

// handleDrillOut navigates up one level while keeping edits. The current (child)
// editor is flushed into editRoot, popped, and the parent editor is refreshed
// from editRoot so it reflects what the child changed. Editing a child and
// returning to fix a parent field is therefore non-destructive — nothing is
// committed to the document until Ctrl+S. Only fired for nested editors.
func (m model) handleDrillOut() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) <= 1 {
		return m, nil
	}
	childWasDirty := m.topBE().dirty

	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		// Invalid YAML in the child — cannot write it into the canonical tree.
		// The error is already shown; stay so the user can fix it.
		return m, nil
	}
	m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
	// Capture the parent's pre-drill-in state before applying the child's
	// changes, so Ctrl+U on the parent can undo the drill-in as one step.
	if childWasDirty {
		if top := m.topBE(); top != nil {
			be := top.saveUndo()
			m.setTopBE(&be)
		}
	}
	m = m.refreshTopFromRoot(childWasDirty)
	m.statusMsg = ""
	return m, nil
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
		isMap := be.isMapNav()
		oldCount := entryCount(be.node, isMap)
		be.node = node
		// Rebuild the tree only when the entry count changed; otherwise keep it so
		// the expanded/collapsed view and cursor survive the round-trip.
		if entryCount(node, isMap) != oldCount {
			be.tree.nodes = be.collectionTreeNodes()
			if be.coll.current >= entryCount(node, isMap) {
				be.coll.current = entryCount(node, isMap) - 1
			}
		}
		be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
	} else {
		be.node = node
		be.yamlEditor.SetValue(nodeToContent(be.key, node))
	}
	be.tree = be.resyncTreeFromYAML()
	if markDirty {
		be.dirty = true
	}
	m.setTopBE(&be)
	return m
}

func (m model) handlePaneBlockEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	top := m.topBE()
	if top == nil {
		m.enterList()
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		return m.handleBlockEditKey(top, key)
	}

	// Non-key messages (window resize, pending confirms, etc.): forward to sub-components.
	prevYAML := top.yamlEditor.Value()
	be, cmd := top.forwardMsg(msg)

	// After forwarding, sync the node if the YAML content changed.
	if be.active == blockEditPanelYAML && be.yamlEditor.Value() != prevYAML {
		be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value()})
	}

	m.setTopBE(&be)
	return m, cmd
}

func (m model) handleBlockEditKey(top *blockEditState, key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Esc at root with dirty state: show discard-changes confirmation.
	if key.Type == tea.KeyEsc && len(top.focus) == 0 && top.dirty {
		be := *top
		al := alert.NewConfirm(
			"Discard changes?",
			"Uncommitted changes will be lost.",
			func() tea.Msg { return blockEditDiscardedMsg{discarded: true} },
			theme.Size{W: m.width, H: m.height},
		)
		be.confirmAlert = &al
		be.mode = modeConfirming
		m.setTopBE(&be)
		return m, nil
	}

	// ctrl+u / ctrl+y with empty stacks: show status, never fall through to doc-level undo.
	if key.String() == "ctrl+u" && len(top.undoStack) == 0 {
		top.statusMsg = "Nothing to undo."
		m.setTopBE(top)
		return m, nil
	}
	if key.String() == "ctrl+y" && len(top.redoStack) == 0 {
		top.statusMsg = "Nothing to redo."
		m.setTopBE(top)
		return m, nil
	}

	// Semantic keys: resolved by keymap → dispatch.
	if ea, ok := blockKeymap(top, key); ok {
		if ea.Block != nil {
			be := top.dispatch(ea.Block)
			m.setTopBE(&be)
			return m, nil
		}
		if ea.Model != nil {
			return m.dispatch(ea.Model)
		}
	}

	// Tab: switch panel focus (pure UI, not dispatched).
	if key.Type == tea.KeyTab && top.mode == modeEditing {
		be := top.switchPanel()
		m.setTopBE(&be)
		return m, nil
	}

	// p: open preset picker (pure UI mode change, not dispatched).
	if key.String() == "p" && top.active == blockEditPanelTree && top.mode == modeEditing {
		be := top.openPresetPicker()
		m.setTopBE(&be)
		return m, nil
	}

	// Tree panel: semantic tree actions go through updateTreePanel.
	if top.active == blockEditPanelTree && top.mode == modeEditing {
		be, cmd := top.updateTreePanel(key)
		m.setTopBE(&be)
		return m, cmd
	}

	// Non-tree key with no special handling: forward to YAML editor.
	prevYAML := top.yamlEditor.Value()
	be, cmd := top.forwardMsg(key)
	if be.active == blockEditPanelYAML && be.yamlEditor.Value() != prevYAML {
		be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value()})
	}
	m.setTopBE(&be)
	return m, cmd
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

	children := m.childrenOf[it.Key]
	kind := fieldKind(m.schemaTree, it.Key)
	// Unknown items have no schema, so skip unknown-key validation inside the overlay.
	knownByPath := m.knownByPath
	if it.Unknown {
		knownByPath = nil
	}
	be := newBlockEdit(m.cfg, blockSpec{key: it.Key, defs: children, kind: kind, def: fieldDefByName(m.schemaTree, it.Key), content: initial, knownByPath: knownByPath}, m.width, m.height)
	be.isEdit = it.Existing
	be.focus = nil // top-level editor edits the whole block
	m.blockEdits = []*blockEditState{&be}
	m.editBlockKey = it.Key
	// Canonical tree, refreshed from the top editor on every flush (drill-in /
	// commit). A non-nil placeholder is enough; the first flush populates it.
	m.editRoot = &yaml.Node{Kind: yaml.MappingNode}
	m.enterBlockEdit()
	return m, be.Init()
}

// flushTopToRoot commits the active editor and writes its value node into
// editRoot at the editor's focus path. Returns (updatedModel, true) on success;
// on a validation error it sets the editor's error and returns false so the
// caller aborts the navigation/commit.
func (m model) flushTopToRoot() (model, bool) {
	top := m.topBE()
	committed, cmd := top.commit()
	m.setTopBE(&committed)
	if cmd == nil {
		m.statusMsg = committed.editorErr.message
		return m, false
	}
	snippet := cmd().(blockEditCommittedMsg).Snippet
	val := valueNodeOfSnippet(snippet)
	if val == nil || !setNodeAt(m.editRoot, committed.focus, val) {
		m.statusMsg = "internal error: could not write editor into canonical tree"
		return m, false
	}
	return m, true
}

// handleOpenChild drills into a nested field. It flushes the parent editor into
// the canonical editRoot, then builds the child editor from the node living at
// the child's focus path within that same tree — no substring copy. Unknown-key
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
	be := newBlockEdit(m.cfg, blockSpec{key: msg.key, defs: msg.defs, kind: msg.kind, content: content, knownByPath: nil}, m.width, m.height)
	be.isEdit = true
	be.focus = childFocus

	m.blockEdits = append(m.blockEdits, &be)
	m.enterBlockEdit()
	return m, be.Init()
}

// handleOverlayConfirmed handles a blockEditCommittedMsg: the editor committing
// itself to the document while staying open. The live Ctrl+S flow uses commitAll
// (canonical-tree flush) instead; this path remains for direct top-level commits
// such as tests seeding content. Nested commits are never produced this way.
func (m model) handleOverlayConfirmed(snippet string) (tea.Model, tea.Cmd) {
	if len(m.blockEdits) != 1 {
		return m, nil
	}

	// Top-level block commit → write to the document.
	be := m.blockEdits[0]
	isEdit := be.isEdit
	var err error
	if isEdit {
		err = m.doc.Replace(be.key, snippet)
	} else {
		err = m.doc.Insert(snippet)
	}
	if err != nil {
		m.statusMsg = fmt.Sprintf("Apply error: %v", err)
		return m, nil
	}
	m.syncView()
	// Re-sync after commit so repeated Ctrl+S is idempotent.
	if fresh, err := m.doc.BlockContent(be.key); err == nil {
		*be = be.resyncAfterCommit(fresh)
	}
	// Keep blockEdit open — user stays in editing mode after commit.
	if isEdit {
		m.statusMsg = "Block updated (not saved yet) — Esc to return."
	} else {
		// First commit transitions the block edit to edit mode.
		be.isEdit = true
		m.statusMsg = "Block added (not saved yet) — Esc to return."
	}
	return m, nil
}

// saveAll is the Ctrl+S handler. When block editors are open it commits all
// stacked editors into m.doc and returns to the list — file save is a separate
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

	pruneEmptyMappings(m.editRoot)
	blockIsEmpty := m.editRoot.Kind == yaml.MappingNode && len(m.editRoot.Content) == 0
	var err error
	switch {
	case blockIsEmpty && isEdit:
		err = m.doc.Remove(m.editBlockKey)
	case !blockIsEmpty:
		final := nodeToContent(m.editBlockKey, m.editRoot)
		if isEdit {
			if current, _ := m.doc.BlockContent(m.editBlockKey); normalizeBlockContent(m.editBlockKey, current) != final {
				err = m.doc.Replace(m.editBlockKey, final)
			}
		} else {
			err = m.doc.Insert(final)
		}
	}
	if err != nil {
		m.statusMsg = fmt.Sprintf("Apply error: %v", err)
		return m, nil
	}
	m.syncView()
	m.enterList()
	m.statusMsg = "Changes committed (not saved yet) — ctrl+s to save."
	return m, nil
}
