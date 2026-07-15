package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/alert"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
	"github.com/lucasassuncao/yedit/yamlnode"
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
	blockEditPanelHint // hint panel focused for scrolling
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

// errKind classifies the origin of an editor error so blocking logic can be precise.
type errKind int

const (
	errNone    errKind = iota
	errParse           // YAML parse failed in flushCurrentEntry; blocks navigation
	errCommit          // validation failed at commit time; blocks commit
	errPreset          // preset I/O failure; display only
	errBlocked         // action rejected (nesting depth, lost focus path); display only
)

// editorError carries a typed error for the block editor's status bar.
type editorError struct {
	kind    errKind
	message string
}

type blockEditState struct {
	cfg Config
	key string // top-level YAML key being edited

	tree        treeModel
	childDefs   []schema.FieldDef
	kind        schema.Kind
	def         schema.FieldDef  // the block's own definition; drives the hint panel for tree-less blocks
	coll        collectionBuffer // non-zero only for collection-nav editors
	knownByPath map[string]map[string]bool

	// node is the block's canonical value node - the single source of truth from
	// which the tree (checkmarks, labels) is projected. For non-collection blocks
	// it mirrors what the YAML editor renders; tree-driven toggles mutate it
	// structurally and the editor is re-rendered from it. Collection blocks still
	// carry their entry list in coll for now.
	node yaml.Node

	yamlEditor      textarea.Model
	previewRenderer *glamour.TermRenderer // non-nil only for KindObject blocks
	active          blockEditPanel
	prevActive      blockEditPanel // panel to return to when leaving hint focus
	hintScroll      int            // scroll offset in hint panel when active == blockEditPanelHint
	previewScroll   int            // 1-based YAML line the Preview keeps visible; 0 = top

	isEdit        bool   // false = add new block, true = edit existing
	dirty         bool   // uncommitted changes since last ctrl+s
	committedYAML string // normalized YAML at last ctrl+s (or open); used to reset dirty when content reverts

	// focus is this editor's address within the model's canonical editRoot tree.
	// nil for the top-level editor (whole block); deeper editors carry the indexed
	// path to the drilled-into node. The editor flushes its content back into
	// editRoot at this path on navigation/commit.
	focus []pathSeg

	width, height int
	listW, rightW int

	editorErr     editorError
	statusMsg     string // neutral feedback (e.g. "Undone."); cleared on next edit action
	currentPreset string

	mode                blockEditMode
	preset              presetBrowser
	confirmAlert        alert.Model
	confirmAlertVisible bool

	undoStack   []blockEditUndoSnap // undo history; each mutating op pushes a snapshot
	redoStack   []blockEditUndoSnap // redo history; populated by restoreUndo, discarded on new mutations
	actionLog   []BlockAction       // in-memory log for debug and replay
	theme       resolvedTheme
	help        help.Model
	legendLines int // lines consumed by the legend bar; updated on resize and init
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
		cfg:           cfg,
		key:           spec.key,
		childDefs:     spec.defs,
		kind:          spec.kind,
		def:           blockOwnDef(spec),
		knownByPath:   spec.knownByPath,
		currentPreset: "custom",
		width:         w,
		height:        h,
		theme:         resolveTheme(cfg.Theme),
	}
	be.help = newHelpModel(be.theme)
	be.help.Width = w - 1
	_, be.legendLines = renderLegend(be.help, be.currentKeyMap(), w-1)
	be = be.relayout()

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
		be.node = *collValueNode(raw, be.isMapNav())
		be.tree.nodes = be.collectionTreeNodes()
	}

	content := spec.content
	if content == "" {
		content = spec.key + ":\n"
	}

	be.yamlEditor = be.newYAMLEditor(content)

	// Non-collection blocks carry their canonical node from the start; the tree
	// is projected from it and tree edits mutate it. (Collections set be.node
	// above, from the full entry list.) Derive the tree once here so it reflects
	// be.node even when content came from a preset rather than spec.content.
	if !structured {
		if v := blockValueNodeOrNil(content); v != nil {
			be.node = *v
		} else {
			be.editorErr = editorError{kind: errParse, message: "Could not parse block content."}
			be.node = yaml.Node{Kind: yaml.MappingNode}
		}
		be.tree = syncTreeCheckedFromNode(be.tree, &be.node)
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
	if len(spec.defs) == 0 || spec.kind == schema.KindPrimitive || (spec.kind == schema.KindDictionary && !structured) {
		be.active = blockEditPanelYAML
		be.yamlEditor.Focus()
	}

	// Baseline for dirty-tracking: the normalized open state, for every block
	// kind. computeDirty compares against it, so dirty is derived rather than
	// maintained flag-by-flag. Non-collection blocks normalize the buffer (an
	// unparseable block on disk must still read as clean until edited).
	if structured {
		be.committedYAML = nodeToContent(be.key, &be.node)
	} else {
		be.committedYAML = normalizeBlockContent(be.key, be.yamlEditor.Value())
	}

	return be
}

// computeDirty reports whether the editor's state differs from the committed
// baseline (committedYAML). It is derived at the dispatch boundary instead of
// being maintained by every mutation: content that returns to the baseline -
// a toggle undone by hand, an edit typed and then reverted - reads as clean
// again, for collections too.
func (be blockEditState) computeDirty() bool {
	if be.isCollectionNav() {
		if nodeToContent(be.key, &be.node) != be.committedYAML {
			return true
		}
		// The buffer may hold unflushed edits of the current entry.
		return be.yamlEditor.Value() != be.entryYAML(be.coll.current)
	}
	return normalizeBlockContent(be.key, be.yamlEditor.Value()) != be.committedYAML
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

func (be blockEditState) relayout() blockEditState {
	be.listW, be.rightW = theme.TwoColumnWidths(be.width)
	if be.kind == schema.KindObject {
		be.previewRenderer = newPreviewRenderer(be.rightW - 2)
	}
	return be
}

func (be blockEditState) innerH() int {
	legendLines := be.legendLines
	if legendLines < 1 {
		legendLines = 1
	}
	h := be.height - headerLines - feedbackLines - legendLines - 2
	if h < 1 {
		h = 1
	}
	return h
}

// hintH returns the content height of the hint panel (bottom-right).
// Returns 0 when EnableHints is false (panel is not rendered).
// Otherwise the hint takes ~1/3 of the right column, floored at 5 lines.
func (be blockEditState) hintH() int {
	if !be.cfg.EnableHints {
		return 0
	}
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
	if !be.cfg.EnableHints {
		return be.innerH()
	}
	h := be.innerH() - 2 - be.hintH()
	if h < 0 {
		h = 0
	}
	return h
}

func (be blockEditState) Init() tea.Cmd { return textarea.Blink }

// Update is the blockEditState message router used by unit tests. At runtime
// the model routes all messages through handlePaneBlockEdit/handleBlockEditKey
// (overlay_stack.go), which handles model-level concerns (Ctrl+S save/commit,
// drill navigation, doc writes). New logic belongs there, not here.
func (be blockEditState) Update(msg tea.Msg) (blockEditState, tea.Cmd) {
	// pendingRemoveMsg fires from the "Remove field?" confirm alert as it
	// dismisses, so it crosses the mode boundary and is handled up front.
	if m, ok := msg.(pendingRemoveMsg); ok {
		be.mode = modeEditing
		be.confirmAlertVisible = false
		return be.dispatch(ToggleField{NodeIdx: m.nodeIdx, Checked: false}), nil
	}
	if m, ok := msg.(pendingEntryDeleteMsg); ok {
		be.mode = modeEditing
		be.confirmAlertVisible = false
		return be.dispatch(DeleteEntry{SeqIdx: m.seqIdx}), nil
	}

	if m, ok := msg.(tea.WindowSizeMsg); ok {
		be.width = m.Width
		be.height = m.Height
		be.help.Width = be.width - 1
		_, be.legendLines = renderLegend(be.help, be.currentKeyMap(), be.width-1)
		be = be.relayout()
		be.yamlEditor.SetWidth(be.rightW - 2)
		be.yamlEditor.SetHeight(be.editorH() - 1)
		be.tree.height = be.innerH()
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
		be.confirmAlertVisible = false
		return be, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		// Allow global shortcuts even while the confirm overlay is active so
		// the user is not surprised that Ctrl+S / Ctrl+L are unavailable.
		switch {
		case key.Matches(km, kbCtrlSSaveCh):
			return be, func() tea.Msg { return commitRequestedMsg{} }
		case key.Matches(km, kbCtrlLValid):
			return be, func() tea.Msg { return validateRequestedMsg{} }
		}
		al, cmd := be.confirmAlert.Update(km)
		be.confirmAlert = al
		return be, cmd
	}
	return be, nil
}

func (be blockEditState) updatePresetBrowser(msg tea.Msg) (blockEditState, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return be, nil
	}
	pb, action, name := be.preset.Update(km, be.isCollectionNav())
	be.preset = pb
	switch action {
	case presetApplied:
		if be.cfg.BlockPresets != nil {
			y, err := be.cfg.BlockPresets.PresetYAML(be.key, name)
			if err != nil {
				be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("preset error: %v", err)}
			} else {
				be = be.dispatch(ApplyPreset{Name: name, Content: y})
			}
		}
	case presetAppended:
		if be.cfg.BlockPresets != nil {
			y, err := be.cfg.BlockPresets.PresetYAML(be.key, name)
			if err != nil {
				be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("preset error: %v", err)}
			} else {
				be = be.dispatch(AppendPreset{Name: name, Content: y})
			}
		}
	case presetNone:
		return be, nil
	}
	// presetDismissed, presetApplied, presetAppended all close the browser.
	be.mode = modeEditing
	return be, nil
}

func (be blockEditState) updateEditing(msg tea.Msg) (blockEditState, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		if be.active == blockEditPanelYAML {
			prev := be.yamlEditor.Value()
			var cmd tea.Cmd
			be.yamlEditor, cmd = be.yamlEditor.Update(msg)
			if be.yamlEditor.Value() != prev {
				be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value(), Checkpoint: true})
			}
			return be, cmd
		}
		return be, nil
	}
	return be.updateKey(key)
}

// handleHintKey handles ctrl+h (toggle hint focus) and navigation when the hint
// panel is focused. Returns (state, true) when it consumed the key.
func (be blockEditState) handleHintKey(msg tea.KeyMsg) (blockEditState, bool) {
	if key.Matches(msg, kbCtrlHHint) && be.cfg.EnableHints {
		if be.active == blockEditPanelHint {
			be.active = be.prevActive
		} else {
			be.prevActive = be.active
			be.active = blockEditPanelHint
		}
		return be, true
	}
	if be.active != blockEditPanelHint {
		return be, false
	}
	switch {
	case key.Matches(msg, kbUp):
		if be.hintScroll > 0 {
			be.hintScroll--
		}
	case key.Matches(msg, kbDown):
		// Scroll bound is the content height, not the panel height - otherwise
		// the tail of a hint longer than two panel-fulls stays unreachable.
		lines := strings.Count(strings.TrimSuffix(be.hintContent(), "\n"), "\n") + 1
		maxScroll := lines - be.hintH()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if be.hintScroll < maxScroll {
			be.hintScroll++
		}
	case key.Matches(msg, kbTab, kbCtrlHHint):
		be.active = be.prevActive
	}
	return be, true
}

func (be blockEditState) updateKey(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	if key.Matches(msg, kbEsc) {
		// Nested editor: Esc navigates up one level, keeping edits (they are
		// flushed into the canonical tree by the model). Nothing is lost, so no
		// discard prompt - that only guards leaving the block edit entirely.
		if len(be.focus) > 0 {
			return be, func() tea.Msg { return drillOutMsg{} }
		}
		// Top-level editor: leaving abandons work not yet committed to the doc.
		if be.dirty {
			al := alert.NewConfirm(
				"Discard changes?",
				"Uncommitted changes will be lost.",
				func() tea.Msg { return blockEditDiscardedMsg{discarded: true} },
			)
			be.confirmAlert = al
			be.confirmAlertVisible = true
			be.mode = modeConfirming
			return be, nil
		}
		return be, func() tea.Msg { return blockEditDiscardedMsg{discarded: false} }
	}

	// Ctrl+S commits the editor stack into the document. That needs model access,
	// so the block layer requests it as a message the root Update handles.
	if key.Matches(msg, kbCtrlSSaveCh) {
		return be, func() tea.Msg { return commitRequestedMsg{} }
	}
	// Ctrl+L triggers doc-level validation (available in every mode).
	if key.Matches(msg, kbCtrlLValid) {
		return be, func() tea.Msg { return validateRequestedMsg{} }
	}

	// Ctrl+U / Ctrl+Y: block-level undo/redo. Empty stacks only report status.
	if key.Matches(msg, kbCtrlUUndo) {
		if len(be.undoStack) == 0 {
			be.statusMsg = "Nothing to undo."
			return be, nil
		}
		return be.dispatch(Undo{}), nil
	}
	if key.Matches(msg, kbCtrlYRedo) {
		if len(be.redoStack) == 0 {
			be.statusMsg = "Nothing to redo."
			return be, nil
		}
		return be.dispatch(Redo{}), nil
	}

	// Ctrl+H toggles the hint panel; when focused it also captures navigation.
	if be2, handled := be.handleHintKey(msg); handled {
		return be2, nil
	}

	if key.Matches(msg, kbTab) {
		return be.switchPanel(), nil
	}

	if be.active == blockEditPanelTree {
		if key.Matches(msg, kbPreset) {
			return be.openPresetPicker(), nil
		}
		return be.updateTreePanel(msg)
	}

	// YAML panel active. The buffer may be transiently invalid while the user
	// types - we never block keystrokes or discard what they wrote. The canonical
	// node is parse-gated below: it (and the tree derived from it) advances only
	// when the buffer parses; while it is invalid the tree freezes at the last good
	// state, so tree and node never disagree. The model's editRoot is touched only
	// at flush (navigation/commit).
	prevValue := be.yamlEditor.Value()
	// Tree-less blocks open with the YAML panel already focused, so the
	// switchPanel checkpoint that normally guards manual editing never fires.
	// Capture the pre-edit state whenever there is nothing to fall back to -
	// including after an undo emptied the stack - and push it below if the
	// keystroke actually mutates the buffer, so ctrl+u can always return to the
	// content before this keystroke. The snapshot must be taken before Update:
	// the textarea shares its buffer internals, so a plain struct copy would
	// alias the post-keystroke content.
	var preSnap *blockEditUndoSnap
	if len(be.undoStack) == 0 {
		snap := be.captureSnap()
		preSnap = &snap
	}
	var cmd tea.Cmd
	be.yamlEditor, cmd = be.yamlEditor.Update(msg)
	// Only re-project when the content actually changed. Cursor moves, selection,
	// and other non-mutating keys leave the tree unchanged, so there is nothing to
	// resync - and no reason to re-parse the buffer.
	if be.yamlEditor.Value() != prevValue {
		if preSnap != nil {
			be.undoStack = appendSnapCapped(nil, *preSnap)
		}
		// Any real edit forks away from the undone states, so pending redo
		// entries are discarded.
		be.redoStack = nil
		be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value(), Checkpoint: false})
	}
	return be, cmd
}

// syncParsedNode is the parse gate called after every YAML editor keystroke. It
// advances the canonical node (and thus the tree) only when content parses
// successfully; an invalid buffer leaves the last good state in place.
// Returns false when the content did not parse and no state was changed.
func (be blockEditState) syncParsedNode(content string) (blockEditState, bool) {
	if be.isCollectionNav() {
		kn, vn, ok := parseEntryFromView(content, be.coll.isMap)
		if !ok {
			return be, false
		}
		return be.applyParsedEntry(kn, vn), true
	}
	if v := valueNodeOfSnippet(content); v != nil {
		be.node = *v
		return be, true
	}
	return be, false
}

// applyParsedEntry writes kn/vn into be.node at the current cursor position.
// When the cursor is valid it updates the existing entry; when the collection
// is empty it appends the first entry so the user's direct YAML edit is
// persisted rather than silently discarded.
func (be blockEditState) applyParsedEntry(kn, vn *yaml.Node) blockEditState {
	cur := be.coll.current
	count := entryCount(&be.node, be.coll.isMap)
	if cur >= 0 && cur < count {
		setEntry(&be.node, be.coll.isMap, cur, kn, vn)
		return be
	}
	if count == 0 {
		if be.coll.isMap {
			be.node.Content = append(be.node.Content, kn, vn)
		} else {
			be.node.Content = append(be.node.Content, vn)
		}
		be.coll.current = 0
		be.tree.nodes = be.collectionTreeNodes()
	}
	return be
}

// resyncTreeFromYAML re-derives the tree's checked states from the canonical
// node - for struct blocks via syncTreeCheckedFromNode (with ADDED/AVAILABLE
// sectioning), for collections via collectionDeriveTree (per-entry labels and
// checks). The node is the source of truth, so the tree can never disagree with
// it even while the text buffer is mid-edit.
func (be blockEditState) resyncTreeFromYAML() treeModel {
	if be.isCollectionNav() {
		return be.collectionDeriveTree()
	}
	return syncTreeCheckedFromNode(be.tree, &be.node)
}

// snippetsFn returns a lookup function for snippets scoped to be.key.
// Reads FieldMeta.Snippet from the MetadataSource when configured.
// Returns nil when no MetadataSource is configured.
func (be blockEditState) snippetsFn() func(string) string {
	if be.cfg.Metadata == nil {
		return nil
	}
	return func(fieldName string) string {
		return be.cfg.Metadata.FieldMeta(be.key, fieldName).Snippet
	}
}

// withPreCheckedFields toggles ON fields where FieldMeta.PreChecked is true,
// inserting their snippets into the YAML editor. Only called for new (not yet
// existing) struct blocks so opening an existing block never modifies content.
func (be blockEditState) withPreCheckedFields() blockEditState {
	if be.cfg.Metadata == nil {
		return be
	}
	ctx := toggleCtx{key: be.key, snippets: be.snippetsFn(), childDefs: be.childDefs}
	changed := false
	for _, n := range be.tree.nodes {
		if n.kind != treeNodeField || n.depth != 0 || n.checked {
			continue
		}
		meta := be.cfg.Metadata.FieldMeta(be.key, n.label)
		if meta.PreChecked {
			be.node = *toggleNodeField(&be.node, ctx, n, true)
			changed = true
		}
	}
	if !changed {
		return be
	}
	be.yamlEditor.SetValue(nodeToContent(be.key, &be.node))
	be.tree = syncTreeCheckedFromNode(be.tree, &be.node)
	return be
}

// resyncAfterCommit reloads the editor from the freshly committed block so a
// repeated Ctrl+S is idempotent. For collection blocks it re-derives the entry
// list (and tree, when the entry count changed); otherwise it reloads the raw
// YAML. The committed baseline is reset either way, so dirty reads clean.
//
// Currently unused at runtime: commitAll returns to the list and discards the
// editor stack instead of keeping the editor open. Kept (with its test) for a
// future commit-in-place flow.
func (be blockEditState) resyncAfterCommit(fresh string) blockEditState {
	if !be.isCollectionNav() {
		if v := blockValueNodeOrNil(fresh); v != nil {
			be.node = *v
		} else {
			be.node = yaml.Node{Kind: yaml.MappingNode}
		}
		be.yamlEditor.SetValue(fresh)
		be.committedYAML = nodeToContent(be.key, &be.node)
		be.dirty = be.computeDirty()
		return be
	}
	isMap := be.isMapNav()
	oldCount := entryCount(&be.node, isMap)
	be.node = *collValueNode(fresh, isMap)
	if entryCount(&be.node, isMap) != oldCount {
		// Entry count changed: rebuild the tree from scratch (expansion is lost,
		// but the structure must match the new node).
		be.tree.nodes = be.collectionTreeNodes()
		if be.coll.current >= entryCount(&be.node, isMap) {
			be.coll.current = entryCount(&be.node, isMap) - 1
		}
	}
	be.tree = be.collectionDeriveTree()
	be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
	be.committedYAML = nodeToContent(be.key, &be.node)
	be.dirty = be.computeDirty()
	return be
}

func (be blockEditState) switchPanel() blockEditState {
	if be.active == blockEditPanelTree {
		// Checkpoint before YAML editing so manual changes are undoable with ctrl+u.
		be = be.saveUndo()
		be.active = blockEditPanelYAML
		be.yamlEditor.Focus()
	} else {
		be.active = blockEditPanelTree
		be.yamlEditor.Blur()
	}
	return be
}

// commit validates the editor's current content and returns its canonical
// value node. It returns (newState, node, true) on success; on a validation
// error it returns (newState, nil, false) with the detail in be.editorErr. The
// node is detached data - the caller decides what to do with it (write it into
// the canonical tree, serialize it for the document), so commit performs no
// effect itself. Returning the node instead of a serialized snippet spares the
// caller a lossy parse-back: parseBlockText already rejected stray or renamed
// top-level keys with a user-facing message.
func (be blockEditState) commit() (blockEditState, *yaml.Node, bool) {
	var val *yaml.Node
	if be.isCollectionNav() {
		be = be.flushCurrentEntry()
		if be.editorErr.kind != errNone {
			return be, nil, false
		}
		val = yamlnode.CloneNode(&be.node)
	} else {
		be.editorErr = editorError{}
		v, errMsg := parseBlockText(be.key, be.yamlEditor.Value())
		if errMsg != "" {
			be.editorErr = editorError{kind: errCommit, message: errMsg}
			return be, nil, false
		}
		val = v
	}

	if be.knownByPath != nil {
		unknown, err := schema.UnknownKeys([]byte(nodeToContent(be.key, val)), be.knownByPath)
		if err != nil {
			be.editorErr = editorError{kind: errCommit, message: fmt.Sprintf("Unknown keys check failed: %v", err)}
			return be, nil, false
		}
		if len(unknown) > 0 {
			be.editorErr = editorError{kind: errCommit, message: fmt.Sprintf("Unknown keys: %s", strings.Join(unknown, ", "))}
			return be, nil, false
		}
	}
	// NOTE: dirty is intentionally NOT cleared here. commit() is called by
	// flushTopToRoot during drill-in/out, where the edits have only reached
	// editRoot — not the document. Clearing dirty here would bypass the
	// "Discard changes?" guard if the user later Escs from the parent editor.
	// A successful document commit (commitAll) returns to the list and discards
	// the editor stack, so the flag dies with it.

	return be, val, true
}

// View renders the block editor. parentSegs is the breadcrumb path from all
// ancestor editors in the stack, computed by model.blockBreadcrumbPrefix().
func (be blockEditState) View(parentSegs []string) string {
	if be.mode == modePresetBrowser {
		return be.presetView(parentSegs)
	}

	header := be.breadcrumbHeader(parentSegs)

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
		// The preview window follows the tree selection: scrollLinesTo keeps
		// previewScroll visible (rendered preview lines map ~1:1 to YAML lines).
		topContent = scrollLinesTo(renderPreviewYAML(be.yamlEditor.Value(), be.previewRenderer), be.editorH(), be.previewScroll)
	} else {
		topTitle = "Editing YAML"
		topContent = clampLines(be.yamlEditor.View(), be.editorH())
	}
	topPanel := theme.RenderTitledPanelWith(topTitle, theme.Size{W: be.rightW, H: be.editorH() + 2}, yamlActive, topContent, be.theme.colors)

	rightPanel := topPanel
	if be.cfg.EnableHints {
		hintActive := be.active == blockEditPanelHint
		hintPanel := theme.RenderTitledPanelWith("Hint/Example", theme.Size{W: be.rightW, H: be.hintH() + 2}, hintActive, be.scrolledHintContent(), be.theme.colors)
		rightPanel = lipgloss.JoinVertical(lipgloss.Left, topPanel, hintPanel)
	}

	feedback := be.feedbackLine()
	legend := renderHelpLine(be.width, be.help, be.currentKeyMap())

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Legend: legend})
	if be.height > 0 {
		out = clampLines(out, be.height)
	}
	if be.confirmAlertVisible {
		out = theme.CompositeCenter(be.confirmAlert.Box(), out)
	}
	return out
}

func (be blockEditState) presetView(parentSegs []string) string {
	header := be.breadcrumbHeader(parentSegs)

	leftPanel := theme.RenderTitledPanelWith("Available Presets", theme.Size{W: be.listW, H: be.innerH() + 2}, !be.preset.previewFocus, be.preset.listView(be.theme), be.theme.colors)
	rightPanel := theme.RenderTitledPanelWith("Preset Preview", theme.Size{W: be.rightW, H: be.innerH() + 2}, be.preset.previewFocus, be.preset.previewView(be.innerH()), be.theme.colors)

	var presetKM help.KeyMap
	switch {
	case be.preset.previewFocus:
		presetKM = presetPreviewMap{}
	case be.isCollectionNav():
		presetKM = presetListCollectionMap{}
	default:
		presetKM = presetListScalarMap{}
	}
	legend := renderHelpLine(be.width, be.help, presetKM)

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Legend: legend})
	if be.height > 0 {
		out = clampLines(out, be.height)
	}
	return out
}

// validateSnippetText checks that text is valid YAML.
func validateSnippetText(text string) error {
	var check any
	return yaml.Unmarshal([]byte(text), &check)
}
