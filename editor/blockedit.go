package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/components/alert"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

// blockSpec describes the block being opened for editing.
type blockSpec struct {
	key         string
	defs        []schema.FieldDef
	kind        schema.Kind
	def         schema.FieldDef // the block's own definition; supplies metadata for tree-less blocks
	content     string
	knownByPath map[string]map[string]bool // for schema validation at commit
}

// blockEditPanel identifies which panel has focus during modeEditing.
type blockEditPanel int

const (
	blockEditPanelTree blockEditPanel = iota
	blockEditPanelYAML
)

// blockEditMode is the top-level state of the block-edit screen. Exactly one
// mode is active at a time; helper data fields (confirmAlert, presetNames…)
// are only meaningful in their corresponding mode.
type blockEditMode int

const (
	modeEditing       blockEditMode = iota // editing tree/yaml panels
	modePresetBrowser                      // preset picker overlay
	modeConfirming                         // confirm alert overlay
)

// collectionBuffer tracks which entry of a collection-nav editor is currently
// shown in the YAML editor. The entry list itself is no longer stored here — it
// is derived structurally from blockEditState.node, the single source of truth.
type collectionBuffer struct {
	key     string
	isMap   bool
	current int // index of the entry shown in yamlEditor (-1 if empty)
}

// blockEditState is the full-screen block editing mode that replaces the old
// floating overlayModel. It reuses the same two-panel layout as the root view.
type blockEditState struct {
	cfg Config
	key string // top-level YAML key being edited

	tree        treeModel
	childDefs   []schema.FieldDef
	kind        schema.Kind
	def         schema.FieldDef  // the block's own definition; drives the hint panel for tree-less blocks
	coll        collectionBuffer // non-zero only for collection-nav editors
	knownByPath map[string]map[string]bool

	// node is the block's canonical value node — the single source of truth from
	// which the tree (checkmarks, labels) is projected. For non-collection blocks
	// it mirrors what the YAML editor renders; tree-driven toggles mutate it
	// structurally and the editor is re-rendered from it. Collection blocks still
	// carry their entry list in coll for now.
	node *yaml.Node

	yamlEditor      textarea.Model
	previewRenderer *glamour.TermRenderer // non-nil only for KindObject blocks
	active          blockEditPanel

	isEdit bool // false = add new block, true = edit existing
	dirty  bool // uncommitted changes since last ctrl+s

	// focus is this editor's address within the model's canonical editRoot tree.
	// nil for the top-level editor (whole block); deeper editors carry the indexed
	// path to the drilled-into node. The editor flushes its content back into
	// editRoot at this path on navigation/commit.
	focus []pathSeg

	width, height int
	listW, rightW int

	errMsg        string
	statusMsg     string // neutral feedback (e.g. "Undone."); cleared on next edit action
	currentPreset string

	mode         blockEditMode
	presetCursor int
	presetNames  []string
	confirmAlert *alert.Model // alert data when mode == modeConfirming

	previewFocus  bool // preset browser: right panel has keyboard focus
	previewScroll int  // preset browser: line scroll offset in preview panel

	undoSnap         *blockEditUndoSnap // one-level undo for preset apply/append
	basePresetFields map[string]string  // field name → example from "base" preset; populated once in newBlockEdit
	theme            resolvedTheme
}

// blockEditUndoSnap captures the state of a blockEditState before any
// mutating operation so it can be restored by a single ctrl+u.
type blockEditUndoSnap struct {
	node            *yaml.Node // deep copy of the canonical node at snapshot time
	currentEntryIdx int
	yamlValue       string
	dirty           bool
	preset          string
	// tree state for collection blocks — preserved so restoreUndo keeps
	// the expanded/collapsed view and cursor position intact.
	treeNodes  []treeNode
	treeCursor int
	treeOffset int
}

// blockOwnDef returns the block's own field definition. When the caller did not
// supply metadata (nested editors, unknown keys, or tests), it synthesizes a
// minimal def from the spec so YAMLName and Kind are always set.
func blockOwnDef(spec blockSpec) schema.FieldDef {
	if spec.def.YAMLName != "" {
		return spec.def
	}
	return schema.FieldDef{YAMLName: spec.key, Kind: spec.kind}
}

// newBlockEdit creates the full-screen block editing state.
func newBlockEdit(cfg Config, spec blockSpec, w, h int) blockEditState {
	be := blockEditState{
		cfg:              cfg,
		key:              spec.key,
		childDefs:        spec.defs,
		kind:             spec.kind,
		def:              blockOwnDef(spec),
		knownByPath:      spec.knownByPath,
		currentPreset:    "custom",
		width:            w,
		height:           h,
		theme:            resolveTheme(cfg.Theme),
		basePresetFields: loadBasePresetFields(cfg, spec.key),
	}
	be.relayout()

	be.tree = newTreeModel(spec, be.innerH())

	// Structured collections ([]Struct / map[string]Struct) keep their canonical
	// entry list in be.node; the tree and per-entry editor are projected from it.
	structured := (spec.kind == schema.KindList || spec.kind == schema.KindDictionary) && len(spec.defs) > 0
	if structured {
		raw := spec.content
		if raw == "" {
			raw = spec.key + ":\n"
		}
		be.coll = collectionBuffer{key: spec.key, isMap: be.isMapNav(), current: -1}
		be.node = collValueNode(raw, be.isMapNav())
		be.tree.nodes = be.collectionTreeNodes()
	}

	// If presets are available and this is a new block, try the "base" preset.
	content := spec.content
	trivial := spec.key + ":\n"
	if (content == "" || content == trivial) && !structured {
		if cfg.Presets != nil {
			if y, err := cfg.Presets.PresetYAML(spec.key, "base"); err == nil {
				content = y
				be.currentPreset = "base"
			}
		}
		if content == "" {
			content = trivial
		}
	}

	be.yamlEditor = be.newYAMLEditor(content)

	// Non-collection blocks carry their canonical node from the start; the tree
	// is projected from it and tree edits mutate it. (Collections set be.node
	// above, from the full entry list.) Derive the tree once here so it reflects
	// be.node even when content came from a preset rather than spec.content.
	if !structured {
		be.node = blockValueNode(content)
		be.tree = syncTreeCheckedFromNode(be.tree, be.node)
	}

	// For new struct blocks, pre-check fields listed in cfg.PreCheckedFields.
	newBlock := spec.content == "" || spec.content == spec.key+":\n"
	if newBlock && !structured {
		be = be.withPreCheckedFields()
	}

	// For structured collections: show the first entry (or empty placeholder).
	if structured {
		be = be.loadEntry(0)
	}

	// If there is no tree to show, focus the YAML editor immediately. A map with
	// child defs uses the navigator; a free-form map (no defs) stays raw YAML.
	if len(spec.defs) == 0 || spec.kind == schema.KindPrimitive || spec.kind == schema.KindEnum || (spec.kind == schema.KindDictionary && !structured) {
		be.active = blockEditPanelYAML
		be.yamlEditor.Focus()
	}

	return be
}

func (be blockEditState) newYAMLEditor(content string) textarea.Model {
	ta := textarea.New()
	ta.SetWidth(be.rightW - 2)
	ta.SetHeight(be.editorH() - 1)
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.Blur()
	if content != "" {
		ta.SetValue(strings.ReplaceAll(content, "\r\n", "\n"))
	}
	return ta
}

func (be *blockEditState) relayout() {
	be.listW, be.rightW = theme.TwoColumnWidths(be.width)
	if be.kind == schema.KindObject {
		be.previewRenderer = newPreviewRenderer(be.rightW - 2)
	}
}

func (be blockEditState) innerH() int {
	h := be.height - headerLines - statusBarLines - 2
	if h < 1 {
		h = 1
	}
	return h
}

// hintH returns the content height of the hint panel (bottom-right).
// The hint takes ~1/3 of the right column, floored at 5 lines.
func (be blockEditState) hintH() int {
	total := be.innerH() - 2 // subtract 2 for the extra border row from stacking
	h := total / 3
	if h < 5 {
		h = 5
	}
	if total-h < 5 {
		h = total - 5
	}
	if h < 0 {
		h = 0
	}
	return h
}

// editorH returns the content height of the top-right panel (editor/preview).
func (be blockEditState) editorH() int {
	h := be.innerH() - 2 - be.hintH()
	if h < 0 {
		h = 0
	}
	return h
}

func (be blockEditState) Init() tea.Cmd { return textarea.Blink }

func (be blockEditState) Update(msg tea.Msg) (blockEditState, tea.Cmd) {
	// pendingRemoveMsg fires from the "Remove field?" confirm alert as it
	// dismisses, so it crosses the mode boundary and is handled up front.
	if m, ok := msg.(pendingRemoveMsg); ok {
		be.mode = modeEditing
		be.confirmAlert = nil
		return be.applyPendingRemove(m.nodeIdx), nil
	}
	if m, ok := msg.(pendingEntryDeleteMsg); ok {
		be.mode = modeEditing
		be.confirmAlert = nil
		return be.performEntryDelete(m.seqIdx), nil
	}

	if m, ok := msg.(tea.WindowSizeMsg); ok {
		be.width = m.Width
		be.height = m.Height
		be.relayout()
		be.yamlEditor.SetWidth(be.rightW - 2)
		be.yamlEditor.SetHeight(be.editorH() - 1)
		be.tree.height = be.innerH()
		if be.confirmAlert != nil {
			be.confirmAlert.Resize(theme.Size{W: m.Width, H: m.Height})
		}
		return be, nil
	}

	switch be.mode {
	case modeConfirming:
		return be.updateConfirming(msg)
	case modePresetBrowser:
		return be.updatePresetBrowser(msg)
	default:
		return be.updateEditing(msg)
	}
}

func (be blockEditState) updateConfirming(msg tea.Msg) (blockEditState, tea.Cmd) {
	if _, ok := msg.(alert.DismissedMsg); ok {
		be.mode = modeEditing
		be.confirmAlert = nil
		return be, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		al, cmd := be.confirmAlert.Update(key)
		be.confirmAlert = &al
		return be, cmd
	}
	return be, nil
}

func (be blockEditState) updatePresetBrowser(msg tea.Msg) (blockEditState, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return be, nil
	}
	switch key.String() {
	case "esc":
		if be.previewFocus {
			be.previewFocus = false
		} else {
			be.mode = modeEditing
		}
	case "tab":
		be.previewFocus = !be.previewFocus
	case "enter":
		if !be.previewFocus {
			if be.presetCursor >= 0 && be.presetCursor < len(be.presetNames) {
				be = be.applyPreset(be.presetNames[be.presetCursor])
			}
			be.mode = modeEditing
		}
	case "a":
		if !be.previewFocus && be.isCollectionNav() {
			if be.presetCursor >= 0 && be.presetCursor < len(be.presetNames) {
				be = be.appendPreset(be.presetNames[be.presetCursor])
			}
			be.mode = modeEditing
		}
	case "up", "k":
		if be.previewFocus {
			if be.previewScroll > 0 {
				be.previewScroll--
			}
		} else {
			if be.presetCursor > 0 {
				be.presetCursor--
				be.previewScroll = 0
			}
		}
	case "down", "j":
		if be.previewFocus {
			be.previewScroll++
		} else if be.presetCursor < len(be.presetNames)-1 {
			be.presetCursor++
			be.previewScroll = 0
		}
	}
	return be, nil
}

func (be blockEditState) updateEditing(msg tea.Msg) (blockEditState, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		if be.active == blockEditPanelYAML {
			var cmd tea.Cmd
			be.yamlEditor, cmd = be.yamlEditor.Update(msg)
			return be, cmd
		}
		return be, nil
	}
	return be.updateKey(key)
}

func (be blockEditState) updateKey(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Nested editor: Esc navigates up one level, keeping edits (they are
		// flushed into the canonical tree by the model). Nothing is lost, so no
		// discard prompt — that only guards leaving the block edit entirely.
		if len(be.focus) > 0 {
			return be, func() tea.Msg { return drillOutMsg{} }
		}
		// Top-level editor: leaving abandons work not yet committed to the doc.
		if be.dirty {
			al := alert.NewConfirm(
				"Discard changes?",
				"Uncommitted changes will be lost.",
				func() tea.Msg { return blockEditDiscardedMsg{discarded: true} },
				theme.Size{W: be.width, H: be.height},
			)
			be.confirmAlert = &al
			be.mode = modeConfirming
			return be, nil
		}
		return be, func() tea.Msg { return blockEditDiscardedMsg{discarded: false} }

	case tea.KeyCtrlS:
		return be.commit()

	case tea.KeyTab:
		return be.switchPanel(), nil
	}

	if be.active == blockEditPanelTree {
		if msg.String() == "p" {
			return be.openPresetPicker(), nil
		}
		return be.updateTreePanel(msg)
	}

	// YAML panel active. The buffer may be transiently invalid while the user
	// types — we never block keystrokes or discard what they wrote. The canonical
	// node is parse-gated below: it (and the tree derived from it) advances only
	// when the buffer parses; while it is invalid the tree freezes at the last good
	// state, so tree and node never disagree. The model's editRoot is touched only
	// at flush (navigation/commit).
	prevValue := be.yamlEditor.Value()
	var cmd tea.Cmd
	be.yamlEditor, cmd = be.yamlEditor.Update(msg)
	// Only re-project when the content actually changed. Cursor moves, selection,
	// and other non-mutating keys leave the tree unchanged, so there is nothing to
	// resync — and no reason to re-parse the buffer.
	if be.yamlEditor.Value() != prevValue {
		be.dirty = true
		be.statusMsg = ""
		be = be.syncParsedNode()
	}
	return be, cmd
}

// syncParsedNode is the parse gate called after every YAML editor keystroke. It
// advances the canonical node (and thus the tree) only when the buffer parses
// successfully; an invalid buffer leaves the last good state in place.
func (be blockEditState) syncParsedNode() blockEditState {
	if be.isCollectionNav() {
		kn, vn, ok := parseEntryFromView(be.yamlEditor.Value(), be.coll.isMap)
		if !ok {
			return be
		}
		if cur := be.coll.current; cur >= 0 && cur < entryCount(be.node, be.coll.isMap) {
			setEntry(be.node, be.coll.isMap, cur, kn, vn)
		}
		be.tree = be.collectionDeriveTree()
		return be
	}
	if v := valueNodeOfSnippet(be.yamlEditor.Value()); v != nil {
		be.node = v
		be.tree = be.resyncTreeFromYAML()
	}
	return be
}

// resyncTreeFromYAML re-derives the tree's checked states from the canonical
// node — for struct blocks via syncTreeCheckedFromNode (with ADDED/AVAILABLE
// sectioning), for collections via collectionDeriveTree (per-entry labels and
// checks). The node is the source of truth, so the tree can never disagree with
// it even while the text buffer is mid-edit.
func (be blockEditState) resyncTreeFromYAML() treeModel {
	if be.isCollectionNav() {
		return be.collectionDeriveTree()
	}
	return syncTreeCheckedFromNode(be.tree, be.node)
}

// collectionDeriveTree refreshes every entry's label, yamlPath, and child
// checkmarks from be.node, preserving the tree's structure (expansion/cursor).
// It is the structural replacement for syncCurrentEntry — and unlike it, derives
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
		label := entryLabel(be.node, isMap, seqIdx)
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
		sub = deriveChecked(entryValueNode(be.node, isMap, seqIdx), sub, true)
		for k, ci := range childIdx {
			nodes[ci] = sub[k]
		}
	}
	tm.nodes = nodes
	return tm
}

// withPreCheckedFields toggles ON the fields listed in cfg.PreCheckedFields for
// this block, inserting their snippets into the YAML editor. Only called for
// new (not yet existing) struct blocks so opening an existing block never
// modifies content.
func (be blockEditState) withPreCheckedFields() blockEditState {
	fields := be.cfg.PreCheckedFields[be.key]
	if len(fields) == 0 {
		return be
	}
	ctx := toggleCtx{key: be.key, snippets: be.cfg.fieldSnippetsFor(be.key), childDefs: be.childDefs}
	nodeByLabel := make(map[string]treeNode, len(be.tree.nodes))
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeField && n.depth == 0 {
			nodeByLabel[n.label] = n
		}
	}
	changed := false
	for _, fieldName := range fields {
		if n, ok := nodeByLabel[fieldName]; ok && !n.checked {
			toggleNodeField(be.node, ctx, n, true)
			changed = true
		}
	}
	if !changed {
		return be
	}
	be.yamlEditor.SetValue(nodeToContent(be.key, be.node))
	be.tree = syncTreeCheckedFromNode(be.tree, be.node)
	return be
}

// fieldHasContent reports whether the field at node.yamlPath has a non-empty
// value in the current YAML editor content. For structured sequences the
// editor shows a single item under the block key, so doc[key] is a []any with
// one element and node.yamlPath[0] is the seq-item label (skipped).
func (be blockEditState) fieldHasContent(node treeNode) bool {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(be.yamlEditor.Value()), &doc); err != nil {
		return false
	}
	path := node.yamlPath
	if len(path) == 0 {
		return false
	}
	var sub map[string]any
	startIdx := 0
	if items, ok := doc[be.key].([]any); ok {
		if len(items) == 0 {
			return false
		}
		sub, _ = items[0].(map[string]any)
		startIdx = 1 // skip seq item label
	} else {
		sub, _ = doc[be.key].(map[string]any)
	}
	if sub == nil || len(path) <= startIdx {
		return false
	}
	cur := sub
	for i := startIdx; i < len(path)-1; i++ {
		cur, _ = cur[path[i]].(map[string]any)
		if cur == nil {
			return false
		}
	}
	val, exists := cur[path[len(path)-1]]
	if !exists || val == nil {
		return false
	}
	switch v := val.(type) {
	case string:
		return v != ""
	case map[string]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	default:
		return true
	}
}

// applyPendingRemove carries out the field removal that was held pending
// confirmation. It sets the node to unchecked and applies the YAML change.
func (be blockEditState) applyPendingRemove(nodeIdx int) blockEditState {
	if nodeIdx < 0 || nodeIdx >= len(be.tree.nodes) {
		return be
	}
	nodes := make([]treeNode, len(be.tree.nodes))
	copy(nodes, be.tree.nodes)
	nodes[nodeIdx].checked = false
	be.tree.nodes = nodes

	node := be.tree.nodes[nodeIdx]
	ctx := toggleCtx{key: be.key, snippets: be.cfg.fieldSnippetsFor(be.key), childDefs: be.childDefs}
	be.applyToggle(ctx, node, false)
	be.dirty = true
	be.tree = be.resyncTreeFromYAML()
	return be
}

func (be blockEditState) updateTreePanel(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	prevSeqIdx := be.tree.NearestSeqItem()

	tree, action := be.tree.Update(msg)
	be.tree = tree

	switch action {
	case treeOpenChild:
		return be.handleTreeOpenChild()
	case treeToggled:
		return be.handleTreeToggled()
	case treeAddNew:
		return be.handleTreeAddNew(), nil
	case treeDeleted:
		return be.handleTreeDeleted(), nil
	}

	// Collection entries are shown one at a time; moving to a different entry
	// requires flushing the current buffer and loading the new entry.
	if be.isCollectionNav() && (action == treeNoAction || action == treeExpanded || action == treeCollapsed) {
		newSeqIdx := be.tree.NearestSeqItem()
		if newSeqIdx != prevSeqIdx {
			be = be.flushCurrentEntry()
			if be.errMsg == "" {
				be = be.loadEntry(newSeqIdx)
			}
			// If errMsg is set, navigation is blocked; the error is visible in the status bar.
		}
	}

	return be, nil
}

// handleTreeOpenChild drills into the openable field under the cursor by
// emitting an openChildMsg carrying the focus-path suffix from this editor to
// the drilled-into node. The model resolves the actual content from the
// canonical editRoot, so no substring is copied here.
func (be blockEditState) handleTreeOpenChild() (blockEditState, tea.Cmd) {
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be, nil
	}
	node := be.tree.nodes[idx]

	// relSegs addresses the field relative to this editor's focus.
	var relSegs []pathSeg
	if be.isCollectionNav() {
		// node.yamlPath[0] is the current item's label (not a real key); the live
		// item is be.coll.current. node.yamlPath[1:] are the field keys below it.
		relSegs = append(relSegs, segIdx(be.coll.current))
		for _, k := range node.yamlPath[1:] {
			relSegs = append(relSegs, segKey(k))
		}
	} else {
		// Struct block: node.yamlPath is the key path from this block's mapping.
		for _, k := range node.yamlPath {
			relSegs = append(relSegs, segKey(k))
		}
	}

	return be, func() tea.Msg {
		return openChildMsg{
			key:     node.def.YAMLName,
			defs:    node.def.Children,
			kind:    node.def.Kind,
			relSegs: relSegs,
		}
	}
}

// handleTreeToggled applies or reverts a field toggle. Toggling a field OFF when
// it already has content requires confirmation to avoid silent data loss.
func (be blockEditState) handleTreeToggled() (blockEditState, tea.Cmd) {
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be, nil
	}
	node := be.tree.nodes[idx]
	be = be.saveUndo()
	if !node.checked && be.fieldHasContent(node) {
		// Revert the toggle in the tree while waiting for the user to confirm.
		nodes := make([]treeNode, len(be.tree.nodes))
		copy(nodes, be.tree.nodes)
		nodes[idx].checked = true
		be.tree.nodes = nodes
		capturedIdx := idx
		al := alert.NewConfirm(
			"Remove field?",
			fmt.Sprintf("Remove %q? Its content will be lost.", node.label),
			func() tea.Msg { return pendingRemoveMsg{nodeIdx: capturedIdx} },
			theme.Size{W: be.width, H: be.height},
		)
		be.confirmAlert = &al
		be.mode = modeConfirming
		return be, nil
	}
	be.dirty = true
	ctx := toggleCtx{key: be.key, snippets: be.cfg.fieldSnippetsFor(be.key), childDefs: be.childDefs}
	be.applyToggle(ctx, node, node.checked)
	be.tree = be.resyncTreeFromYAML()
	return be, nil
}

// applyToggle adds or removes the field at node within the canonical node, then
// re-renders the editor from it. For collections it targets the current entry's
// value mapping; for struct blocks the block's own mapping. Either way the tree
// (derived from the same node) stays in agreement.
func (be *blockEditState) applyToggle(ctx toggleCtx, node treeNode, checked bool) {
	if be.isCollectionNav() {
		be.toggleEntryField(ctx, node, checked)
		be.yamlEditor.SetValue(entryViewYAML(be.node, be.key, be.coll.isMap, be.coll.current))
		return
	}
	toggleNodeField(be.node, ctx, node, checked)
	be.yamlEditor.SetValue(nodeToContent(be.key, be.node))
}

// toggleEntryField mutates the current collection entry's value mapping. It
// mirrors applyToggleToEntry but operates on the live node instead of re-parsed
// text: yamlPath[0] is the entry label (skipped), the field path starts at [1].
func (be *blockEditState) toggleEntryField(ctx toggleCtx, node treeNode, checked bool) {
	if len(node.yamlPath) < 2 {
		return
	}
	entryNode := entryValueNode(be.node, be.coll.isMap, be.coll.current)
	if entryNode == nil {
		return
	}
	fieldPath := node.yamlPath[1:]
	if !applyToggleAt(entryNode, fieldPath[:len(fieldPath)-1], fieldPath[len(fieldPath)-1], checked, ctx, false) {
		return
	}
	pruneEmptyMappings(entryNode)
	reorderNestedMappingKeys(entryNode, ctx.childDefs)
}

// handleTreeAddNew appends a fresh entry to the collection and moves the cursor
// to it so the user can start filling in its fields immediately.
func (be blockEditState) handleTreeAddNew() blockEditState {
	be = be.saveUndo()
	be.dirty = true
	be = be.flushCurrentEntry()
	be.errMsg = "" // adding overrides an in-progress invalid entry; don't block on it
	label := be.newEntryLabel()
	be.tree = be.tree.WithNewSeqItem(be.childDefs, label)
	// Build the new entry node from the schema template and append it.
	kn, vn, ok := parseEntryFromView(be.key+":\n"+be.initialEntryContent(label), be.coll.isMap)
	if !ok {
		vn = &yaml.Node{Kind: yaml.MappingNode}
		kn = &yaml.Node{Kind: yaml.ScalarNode, Value: label}
	}
	if be.coll.isMap {
		be.node.Content = append(be.node.Content, kn, vn)
	} else {
		be.node.Content = append(be.node.Content, vn)
	}
	be = be.loadEntry(be.tree.NearestSeqItem())
	be.tree = be.resyncTreeFromYAML()
	return be
}

// handleTreeDeleted fires when ctrl+d targets a collection entry. Dropping a whole
// entry is the most destructive tree action, so it confirms first (unless
// NoDeleteConfirm); the actual removal runs in performEntryDelete.
func (be blockEditState) handleTreeDeleted() blockEditState {
	idx := be.tree.currentNodeIdx()
	if idx < 0 || be.tree.nodes[idx].kind != treeNodeSeqItem {
		return be
	}
	seqIdx := be.tree.nodes[idx].seqIdx
	if be.cfg.NoDeleteConfirm {
		return be.performEntryDelete(seqIdx)
	}
	label := be.tree.nodes[idx].label
	al := alert.NewConfirm(
		"Remove entry?",
		fmt.Sprintf("Remove %q? Its content will be lost.", label),
		func() tea.Msg { return pendingEntryDeleteMsg{seqIdx: seqIdx} },
		theme.Size{W: be.width, H: be.height},
	)
	be.confirmAlert = &al
	be.mode = modeConfirming
	return be
}

// performEntryDelete removes collection entry seqIdx from both the tree and the
// buffer, saving undo first. saveUndo runs before the tree is mutated, so the
// snapshot captures the pre-deletion tree directly and ctrl+u restores the entry.
func (be blockEditState) performEntryDelete(seqIdx int) blockEditState {
	be = be.saveUndo()
	be.dirty = true
	be.tree = be.tree.WithDeletedSeqItem(seqIdx)
	removeEntry(be.node, be.coll.isMap, seqIdx)
	be = be.loadEntry(be.tree.NearestSeqItem())
	// Re-derive so positional ("item N") labels of unnamed entries stay in sync
	// with their new index in the node after the surviving entries shift up.
	be.tree = be.collectionDeriveTree()
	return be
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
	if be.isMapNav() {
		return buildMapNodesFromNode(be.childDefs, be.node)
	}
	return buildSeqNodesFromNode(be.childDefs, be.node)
}

// resyncAfterCommit reloads the editor from the freshly committed block so a
// repeated Ctrl+S is idempotent. For collection blocks it re-derives the entry
// list (and tree, when the entry count changed); otherwise it reloads the raw
// YAML. Clears the dirty flag either way.
func (be blockEditState) resyncAfterCommit(fresh string) blockEditState {
	if !be.isCollectionNav() {
		be.node = blockValueNode(fresh)
		be.yamlEditor.SetValue(fresh)
		be.dirty = false
		return be
	}
	isMap := be.isMapNav()
	oldCount := entryCount(be.node, isMap)
	be.node = collValueNode(fresh, isMap)
	if entryCount(be.node, isMap) != oldCount {
		// Entry count changed: rebuild the tree from scratch (expansion is lost,
		// but the structure must match the new node).
		be.tree.nodes = be.collectionTreeNodes()
		if be.coll.current >= entryCount(be.node, isMap) {
			be.coll.current = entryCount(be.node, isMap) - 1
		}
	}
	be.tree = be.collectionDeriveTree()
	be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
	be.dirty = false
	return be
}

// flushCurrentEntry parses the current entry's editor text back into the
// canonical node. It is a no-op when there is no current entry or the editor is
// empty. When the text cannot be parsed into an entry (e.g. the user deleted the
// "key:" header, or it is mid-edit invalid), be.errMsg is set so callers block
// navigation or commit — the parse gate that keeps the node valid.
func (be blockEditState) flushCurrentEntry() blockEditState {
	cur := be.coll.current
	if cur < 0 || cur >= entryCount(be.node, be.coll.isMap) {
		be.errMsg = ""
		return be
	}
	view := be.yamlEditor.Value()
	if strings.TrimSpace(view) == "" {
		be.errMsg = ""
		return be
	}
	kn, vn, ok := parseEntryFromView(view, be.coll.isMap)
	if !ok {
		if itemContentFrom(be.key, view) == "" {
			be.errMsg = "Missing '" + be.key + ":' header — restore it before navigating."
		} else {
			be.errMsg = "Invalid YAML — fix this entry before leaving it."
		}
		return be
	}
	setEntry(be.node, be.coll.isMap, cur, kn, vn)
	be.errMsg = ""
	return be
}

// loadEntry shows entry idx in the editor.
// Always call flushCurrentEntry before loadEntry when switching entries.
func (be blockEditState) loadEntry(idx int) blockEditState {
	be.coll.current = idx
	be.yamlEditor.SetValue(be.entryYAML(idx))
	return be
}

// entryYAML returns the single-entry editor view for index idx.
func (be blockEditState) entryYAML(idx int) string {
	return entryViewYAML(be.node, be.key, be.coll.isMap, idx)
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
func (be blockEditState) newEntryLabel() string {
	if !be.isMapNav() {
		return ""
	}
	existing := make(map[string]bool)
	for _, node := range be.tree.nodes {
		if node.kind == treeNodeSeqItem {
			existing[node.label] = true
		}
	}
	// Start at count+1 for predictable positional labels, but increment past
	// any key that already exists so we never produce a duplicate map key.
	for n := len(existing) + 1; ; n++ {
		label := fmt.Sprintf("key%d", n)
		if !existing[label] {
			return label
		}
	}
}

func (be blockEditState) switchPanel() blockEditState {
	if be.active == blockEditPanelTree {
		be.active = blockEditPanelYAML
		be.yamlEditor.Focus()
	} else {
		be.active = blockEditPanelTree
		be.yamlEditor.Blur()
	}
	return be
}

// openPresetPicker enters preset-browser mode if there are any presets for
// this block. It's a no-op when Presets is nil or the field has none.
func (be blockEditState) openPresetPicker() blockEditState {
	if be.cfg.Presets == nil {
		return be
	}
	names := be.cfg.Presets.ListPresets(be.key)
	if len(names) == 0 {
		return be
	}
	be.mode = modePresetBrowser
	be.presetNames = names
	be.presetCursor = 0
	be.previewFocus = false
	be.previewScroll = 0
	for i, n := range names {
		if n == be.currentPreset {
			be.presetCursor = i
			break
		}
	}
	return be
}

func (be blockEditState) saveUndo() blockEditState {
	treeNodes := make([]treeNode, len(be.tree.nodes))
	copy(treeNodes, be.tree.nodes)
	be.undoSnap = &blockEditUndoSnap{
		node:            cloneNode(be.node),
		currentEntryIdx: be.coll.current,
		yamlValue:       be.yamlEditor.Value(),
		dirty:           be.dirty,
		preset:          be.currentPreset,
		treeNodes:       treeNodes,
		treeCursor:      be.tree.cursor,
		treeOffset:      be.tree.offset,
	}
	return be
}

func (be blockEditState) restoreUndo() blockEditState {
	snap := be.undoSnap
	if snap == nil {
		return be
	}
	be.undoSnap = nil
	be.currentPreset = snap.preset
	be.dirty = snap.dirty
	be.errMsg = ""

	be.node = cloneNode(snap.node)

	if be.isCollectionNav() {
		be.coll.current = snap.currentEntryIdx
		if len(snap.treeNodes) > 0 {
			treeNodes := make([]treeNode, len(snap.treeNodes))
			copy(treeNodes, snap.treeNodes)
			be.tree.nodes = treeNodes
			be.tree.cursor = snap.treeCursor
			be.tree.offset = snap.treeOffset
		} else {
			be.tree.nodes = be.collectionTreeNodes()
			be.tree.cursor = 0
			be.tree.offset = 0
		}
		be = be.loadEntry(be.coll.current)
		// The node is authoritative; snap.yamlValue restores any in-progress (even
		// unparseable) text the user had typed into the entry at snapshot time.
		be.yamlEditor.SetValue(snap.yamlValue)
		be.tree = be.resyncTreeFromYAML()
		return be
	}
	be.yamlEditor.SetValue(snap.yamlValue)
	be.tree = syncTreeCheckedFromNode(be.tree, be.node)
	return be
}

func (be blockEditState) applyPreset(name string) blockEditState {
	if be.cfg.Presets == nil {
		return be
	}
	y, err := be.cfg.Presets.PresetYAML(be.key, name)
	if err != nil {
		be.errMsg = fmt.Sprintf("preset error: %v", err)
		return be
	}
	be = be.saveUndo()
	be.currentPreset = name
	be.errMsg = ""
	be.dirty = true

	if be.isCollectionNav() {
		be.node = collValueNode(y, be.isMapNav())
		be.tree.nodes = be.collectionTreeNodes()
		be.tree.cursor = 0
		be.tree.offset = 0
		be = be.loadEntry(0)
		return be
	}

	be.yamlEditor.SetValue(y)
	be.node = blockValueNode(y)
	be.tree = syncTreeCheckedFromNode(be.tree, be.node)
	return be
}

func (be blockEditState) appendPreset(name string) blockEditState {
	if be.cfg.Presets == nil || !be.isCollectionNav() {
		return be
	}
	y, err := be.cfg.Presets.PresetYAML(be.key, name)
	if err != nil {
		be.errMsg = fmt.Sprintf("preset error: %v", err)
		return be
	}
	be = be.saveUndo()

	presetNode := collValueNode(y, be.isMapNav())
	if entryCount(presetNode, be.isMapNav()) == 0 {
		return be
	}

	be = be.flushCurrentEntry()
	be.errMsg = "" // appending overrides an in-progress invalid entry; don't block
	// Indentation is irrelevant now: the entries are spliced as nodes and re-encoded.
	be.node.Content = append(be.node.Content, presetNode.Content...)

	be.tree.nodes = be.collectionTreeNodes()
	be.tree.offset = 0
	be.tree.cursor = entryCount(be.node, be.isMapNav()) - 1

	be = be.loadEntry(entryCount(be.node, be.isMapNav()) - 1)
	be.currentPreset = name
	be.errMsg = ""
	be.dirty = true
	return be
}

func (be blockEditState) commit() (blockEditState, tea.Cmd) {
	var snippet string
	if be.isCollectionNav() {
		be = be.flushCurrentEntry()
		if be.errMsg != "" {
			return be, nil
		}
		snippet = nodeToContent(be.key, be.node)
	} else {
		be.errMsg = ""
		snippet = be.yamlEditor.Value()
	}

	if err := validateSnippetText(snippet); err != nil {
		be.errMsg = fmt.Sprintf("Invalid YAML: %v", err)
		return be, nil
	}
	if be.knownByPath != nil {
		if unknown := schema.UnknownKeys([]byte(snippet), be.knownByPath); len(unknown) > 0 {
			be.errMsg = fmt.Sprintf("Unknown keys: %s", strings.Join(unknown, ", "))
			return be, nil
		}
	}
	if !strings.HasSuffix(snippet, "\n") {
		snippet += "\n"
	}
	be.dirty = false

	return be, func() tea.Msg { return blockEditCommittedMsg{Snippet: snippet} }
}

// View renders the block editor. parentSegs is the breadcrumb path from all
// ancestor editors in the stack, computed by model.blockBreadcrumbPrefix().
func (be blockEditState) View(parentSegs []string) string {
	switch be.mode {
	case modeConfirming:
		return be.confirmAlert.View()
	case modePresetBrowser:
		return be.presetView(parentSegs)
	}

	segs := append(append(parentSegs, be.key), be.tree.BreadcrumbSegments()...)
	header := theme.RenderHeaderWith(be.cfg.Title, strings.Join(segs, " › "), "", be.width, be.theme.colors)

	treeActive := be.active == blockEditPanelTree
	leftTitle, leftContent := "Fields", be.tree.View(be.theme)
	if be.tree.isEmpty() {
		leftTitle, leftContent = "Field", be.fieldItemView()
	}
	leftPanel := theme.RenderTitledPanelWith(leftTitle, theme.Size{W: be.listW, H: be.innerH() + 2}, treeActive, leftContent, be.theme.colors)

	yamlActive := be.active == blockEditPanelYAML
	var topTitle, topContent string
	if !yamlActive && be.kind == schema.KindObject {
		topTitle = "Preview"
		topContent = renderPreviewYAML(be.yamlEditor.Value(), be.previewRenderer)
	} else {
		topTitle = "Editing YAML"
		topContent = be.yamlEditor.View()
	}
	topPanel := theme.RenderTitledPanelWith(topTitle, theme.Size{W: be.rightW, H: be.editorH() + 2}, yamlActive, clampLines(topContent, be.editorH()), be.theme.colors)

	hintPanel := theme.RenderTitledPanelWith("Hint/Example", theme.Size{W: be.rightW, H: be.hintH() + 2}, false, clampLines(be.hintContent(), be.hintH()), be.theme.colors)
	rightPanel := lipgloss.JoinVertical(lipgloss.Left, topPanel, hintPanel)

	hintText := be.currentHint()

	var feedback string
	switch {
	case be.errMsg != "":
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.errorText.Render(be.errMsg))
	case be.dirty:
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.status.Render(msgUncommittedChanges))
	case be.statusMsg != "":
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.status.Render(be.statusMsg))
	}
	hint := lipgloss.NewStyle().Width(be.width).Render(be.theme.status.Render(hintText))

	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Hint: hint})
}

// currentHint returns the hint bar text for the current panel and cursor state.
func (be blockEditState) currentHint() string {
	if be.active != blockEditPanelTree {
		return hintSaveTail
	}
	parts := []string{keyNav, keyExpand}
	if be.cfg.Presets != nil && len(be.cfg.Presets.ListPresets(be.key)) > 0 {
		parts = append(parts, keyPreset)
	}
	if be.isCollectionNav() {
		parts = append(parts, keyEnterAdd, keyCtrlDDelete)
	} else {
		parts = append(parts, keyEnterAdd, keyCtrlDRemove)
	}
	parts = append(parts, keyCtrlUUndo, keyTabPane, keyCtrlSSaveChg, keyEscBack)
	return strings.Join(parts, hintSep)
}

func (be blockEditState) presetView(parentSegs []string) string {
	segs := append(append(parentSegs, be.key), be.tree.BreadcrumbSegments()...)
	header := theme.RenderHeaderWith(be.cfg.Title, strings.Join(segs, " › "), "", be.width, be.theme.colors)

	leftPanel := theme.RenderTitledPanelWith("Available Presets", theme.Size{W: be.listW, H: be.innerH() + 2}, !be.previewFocus, be.renderPresetList(), be.theme.colors)
	rightPanel := theme.RenderTitledPanelWith("Preset Preview", theme.Size{W: be.rightW, H: be.innerH() + 2}, be.previewFocus, be.scrolledPreview(), be.theme.colors)

	hintStr := hintPresetListScalar
	if be.previewFocus {
		hintStr = hintPresetPreviewFocused
	} else if be.isCollectionNav() {
		hintStr = hintPresetListCollection
	}
	hint := lipgloss.NewStyle().Width(be.width).Render(be.theme.status.Render(hintStr))

	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Hint: hint})
}

func (be blockEditState) scrolledPreview() string {
	full := be.presetPreviewYAML()
	if full == "" {
		return ""
	}
	lines := strings.Split(full, "\n")
	visibleH := be.innerH()
	if visibleH < 1 {
		visibleH = 1
	}
	total := len(lines)
	maxScroll := total - visibleH
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := be.previewScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + visibleH
	if end > total {
		end = total
	}
	return strings.Join(lines[scroll:end], "\n")
}

func (be blockEditState) renderPresetList() string {
	var sb strings.Builder
	for i, name := range be.presetNames {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i == be.presetCursor {
			sb.WriteString(be.theme.selectedItem.Render("▶  " + name))
		} else {
			sb.WriteString(be.theme.availableItem.Render("   " + name))
		}
	}
	return sb.String()
}

func (be blockEditState) presetPreviewYAML() string {
	if be.cfg.Presets == nil || len(be.presetNames) == 0 {
		return ""
	}
	if be.presetCursor < 0 || be.presetCursor >= len(be.presetNames) {
		return ""
	}
	y, err := be.cfg.Presets.PresetYAML(be.key, be.presetNames[be.presetCursor])
	if err != nil {
		return fmt.Sprintf("# error: %v", err)
	}
	return y
}

// validateSnippetText checks that text is valid YAML.
func validateSnippetText(text string) error {
	var check any
	return yaml.Unmarshal([]byte(text), &check)
}

// --- Hint panel ----------------------------------------------------------

// typeLabel returns the human type shown in the hint panel. For primitive fields
// it prefers the concrete scalar type ("string", "int", "bool", …); for
// everything else it falls back to the kind ("object", "list", "enum", …).
func typeLabel(def schema.FieldDef) string {
	if def.Kind == schema.KindPrimitive && def.Scalar != "" {
		return def.Scalar
	}
	return kindHumanLabel(def.Kind)
}

func kindHumanLabel(k schema.Kind) string {
	switch k {
	case schema.KindPrimitive:
		return "primitive"
	case schema.KindObject:
		return "object"
	case schema.KindList:
		return "list"
	case schema.KindDictionary:
		return "dictionary"
	case schema.KindVariant:
		return "variant"
	case schema.KindEnum:
		return "enum"
	default:
		return "unknown"
	}
}

// fieldItemView renders the left panel for a tree-less block (primitive, enum,
// or free-form collection): a single non-toggleable row naming the field being
// edited. There are no sub-fields to navigate, so the row is just an anchor —
// the field's metadata lives in the Hint/Example panel.
func (be blockEditState) fieldItemView() string {
	return be.theme.existingItem.Render(" ▸ " + be.key)
}

// hintContent returns the rendered string for the bottom-right hint panel.
func (be blockEditState) hintContent() string {
	// Tree-less blocks (primitive/enum/free-form collection) have no field nodes;
	// show the block's own metadata instead of the "select a field" placeholder.
	if be.tree.isEmpty() {
		return be.fieldHintFor(be.def)
	}
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be.theme.hintDim.Render("  select a field to see hints")
	}
	node := be.tree.nodes[idx]
	if node.kind != treeNodeField {
		return be.theme.hintDim.Render("  select a field to see hints")
	}
	return be.fieldHintFor(node.def)
}

// fieldHintFor builds the hint text for a single field definition.
func (be blockEditState) fieldHintFor(def schema.FieldDef) string {
	return renderFieldHint(be.theme, def, be.fieldExample(def))
}

// renderFieldHint formats a field's metadata into the Hint/Example panel body:
// type, required, default, allowed values, description, and example. The example
// is resolved by the caller so both the block editor and the root view can reuse
// this with their own example source.
func renderFieldHint(th resolvedTheme, def schema.FieldDef, example string) string {
	var sb strings.Builder

	sb.WriteString(th.hintKey.Render("type") + "  " + typeLabel(def) + "\n")

	if def.Required {
		sb.WriteString(th.hintKey.Render("required") + "\n")
	}
	if def.Default != "" {
		sb.WriteString(th.hintKey.Render("default") + "  " + def.Default + "\n")
	}
	if len(def.OneOf) > 0 {
		sb.WriteString("\n" + th.hintKey.Render("values") + "\n")
		for _, v := range def.OneOf {
			sb.WriteString("  • " + v + "\n")
		}
	}
	if def.Description != "" {
		sb.WriteString("\n" + def.Description + "\n")
	}

	if example != "" {
		sb.WriteString("\n" + th.hintKey.Render("Example") + "\n")
		for _, line := range strings.Split(strings.TrimRight(example, "\n"), "\n") {
			sb.WriteString("  " + line + "\n")
		}
	}

	return sb.String()
}

// fieldExample returns a YAML snippet for the field: Config.FieldExamples takes
// precedence, then the "base" preset, then a structural fallback is generated
// from the FieldDef so there is always something useful to show.
func (be blockEditState) fieldExample(def schema.FieldDef) string {
	if be.cfg.FieldExamples != nil {
		if ex := be.cfg.FieldExamples[be.key][def.YAMLName]; ex != "" {
			return ex
		}
	}
	if ex := be.extractFromBasePreset(def.YAMLName); ex != "" {
		return ex
	}
	return generateFallbackExample(def)
}

// generateFallbackExample produces a minimal valid YAML snippet for def when no
// explicit example or preset value is available.
func generateFallbackExample(def schema.FieldDef) string {
	switch def.Kind {
	case schema.KindEnum:
		if len(def.OneOf) > 0 {
			return def.YAMLName + ": " + def.OneOf[0]
		}
		return def.YAMLName + ": \"\""
	case schema.KindList:
		return def.YAMLName + ":\n  - "
	case schema.KindDictionary:
		return def.YAMLName + ":\n  key: value"
	case schema.KindObject:
		if len(def.Children) == 0 {
			return def.YAMLName + ":\n  # ..."
		}
		var sb strings.Builder
		sb.WriteString(def.YAMLName + ":\n")
		for _, child := range def.Children {
			val := "\"\""
			if child.Default != "" {
				val = child.Default
			}
			sb.WriteString("  " + child.YAMLName + ": " + val + "\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	case schema.KindVariant:
		return def.YAMLName + ": \"\""
	default: // KindPrimitive
		if def.Default != "" {
			return def.YAMLName + ": " + def.Default
		}
		return def.YAMLName + ": \"\""
	}
}

// extractFromBasePreset returns the YAML snippet for fieldName from the
// pre-parsed base preset cache. It is a map lookup — the actual parsing
// happens once in newBlockEdit via loadBasePresetFields.
func (be blockEditState) extractFromBasePreset(fieldName string) string {
	return be.basePresetFields[fieldName]
}

// loadBasePresetFields parses the "base" preset for blockKey and returns a map
// of field name → YAML snippet for every top-level child field. Called once
// during newBlockEdit so the per-frame render path does no I/O or parsing.
func loadBasePresetFields(cfg Config, blockKey string) map[string]string {
	if cfg.Presets == nil {
		return nil
	}
	presetYAML, err := cfg.Presets.PresetYAML(blockKey, "base")
	if err != nil {
		return nil
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(presetYAML), &doc); err != nil {
		return nil
	}
	root, _ := doc[blockKey].(map[string]any)
	if root == nil {
		return nil
	}
	fields := make(map[string]string, len(root))
	for k, v := range root {
		out, err := yaml.Marshal(map[string]any{k: v})
		if err != nil {
			continue
		}
		fields[k] = strings.TrimRight(string(out), "\n")
	}
	return fields
}
