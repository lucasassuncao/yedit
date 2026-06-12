package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/internal/alert"
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
	errNone   errKind = iota
	errParse          // YAML parse failed in flushCurrentEntry; blocks navigation
	errCommit         // validation failed at commit time; blocks commit
	errPreset         // preset I/O failure; display only
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
	node *yaml.Node

	yamlEditor      textarea.Model
	previewRenderer *glamour.TermRenderer // non-nil only for KindObject blocks
	active          blockEditPanel
	prevActive      blockEditPanel // panel to return to when leaving hint focus
	hintScroll      int            // scroll offset in hint panel when active == blockEditPanelHint

	isEdit bool // false = add new block, true = edit existing
	dirty  bool // uncommitted changes since last ctrl+s

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

	mode         blockEditMode
	preset       *presetBrowser // preset-picker overlay state when mode == modePresetBrowser
	confirmAlert *alert.Model   // alert data when mode == modeConfirming

	undoStack []*blockEditUndoSnap // undo history; each mutating op pushes a snapshot
	redoStack []*blockEditUndoSnap // redo history; populated by restoreUndo, discarded on new mutations
	actionLog []BlockAction        // in-memory log for debug and replay
	theme     resolvedTheme
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
	if len(spec.defs) == 0 || spec.kind == schema.KindPrimitive || (spec.kind == schema.KindDictionary && !structured) {
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
	h := be.innerH() - 2 - be.hintH()
	if h < 0 {
		h = 0
	}
	return h
}

func (be blockEditState) Init() tea.Cmd { return textarea.Blink }

// forwardMsg passes bubbletea messages to sub-components (textarea, alert,
// preset browser, resize). Contains no editor logic - all semantic mutations go
// through dispatch. pendingRemoveMsg and pendingEntryDeleteMsg are converted to
// dispatch calls here because they arrive after a confirmation dialog clears.
func (be blockEditState) forwardMsg(msg tea.Msg) (blockEditState, tea.Cmd) {
	if m, ok := msg.(pendingRemoveMsg); ok {
		be.mode = modeEditing
		be.confirmAlert = nil
		be = be.dispatch(ToggleField{NodeIdx: m.nodeIdx, Checked: false})
		return be, nil
	}
	if m, ok := msg.(pendingEntryDeleteMsg); ok {
		be.mode = modeEditing
		be.confirmAlert = nil
		be = be.dispatch(DeleteEntry{SeqIdx: m.seqIdx})
		return be, nil
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
	case modePresetBrowser:
		return be.updatePresetBrowser(msg)
	default:
		// modeEditing: forward non-key messages to the textarea when it has focus.
		if be.active == blockEditPanelYAML {
			if _, ok := msg.(tea.KeyMsg); !ok {
				var cmd tea.Cmd
				be.yamlEditor, cmd = be.yamlEditor.Update(msg)
				return be, cmd
			}
		}
	}
	return be, nil
}

// Update is the blockEditState message router used by unit tests. At runtime
// the model routes all messages through handlePaneBlockEdit/handleBlockEditKey
// (overlay_stack.go), which handles model-level concerns (commitAll, drill
// navigation, doc writes). New logic belongs there, not here.
func (be blockEditState) Update(msg tea.Msg) (blockEditState, tea.Cmd) {
	// pendingRemoveMsg fires from the "Remove field?" confirm alert as it
	// dismisses, so it crosses the mode boundary and is handled up front.
	if m, ok := msg.(pendingRemoveMsg); ok {
		be.mode = modeEditing
		be.confirmAlert = nil
		return be.dispatch(ToggleField{NodeIdx: m.nodeIdx, Checked: false}), nil
	}
	if m, ok := msg.(pendingEntryDeleteMsg); ok {
		be.mode = modeEditing
		be.confirmAlert = nil
		return be.dispatch(DeleteEntry{SeqIdx: m.seqIdx}), nil
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
	action, name := be.preset.Update(key, be.isCollectionNav())
	switch action {
	case presetApplied:
		if be.cfg.Presets != nil {
			y, err := be.cfg.Presets.PresetYAML(be.key, name)
			if err != nil {
				be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("preset error: %v", err)}
			} else {
				be = be.applyPreset(name, y)
			}
		}
	case presetAppended:
		if be.cfg.Presets != nil {
			y, err := be.cfg.Presets.PresetYAML(be.key, name)
			if err != nil {
				be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("preset error: %v", err)}
			} else {
				be = be.appendPreset(name, y)
			}
		}
	case presetNone:
		return be, nil
	}
	// presetDismissed, presetApplied, presetAppended all close the browser.
	be.mode = modeEditing
	be.preset = nil
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
	// types - we never block keystrokes or discard what they wrote. The canonical
	// node is parse-gated below: it (and the tree derived from it) advances only
	// when the buffer parses; while it is invalid the tree freezes at the last good
	// state, so tree and node never disagree. The model's editRoot is touched only
	// at flush (navigation/commit).
	prevValue := be.yamlEditor.Value()
	var cmd tea.Cmd
	be.yamlEditor, cmd = be.yamlEditor.Update(msg)
	// Only re-project when the content actually changed. Cursor moves, selection,
	// and other non-mutating keys leave the tree unchanged, so there is nothing to
	// resync - and no reason to re-parse the buffer.
	if be.yamlEditor.Value() != prevValue {
		be.dirty = true
		be.statusMsg = ""
		be = be.syncParsedNode(be.yamlEditor.Value())
	}
	return be, cmd
}

// syncParsedNode is the parse gate called after every YAML editor keystroke. It
// advances the canonical node (and thus the tree) only when content parses
// successfully; an invalid buffer leaves the last good state in place.
func (be blockEditState) syncParsedNode(content string) blockEditState {
	if be.isCollectionNav() {
		kn, vn, ok := parseEntryFromView(content, be.coll.isMap)
		if !ok {
			return be
		}
		if cur := be.coll.current; cur >= 0 && cur < entryCount(be.node, be.coll.isMap) {
			setEntry(be.node, be.coll.isMap, cur, kn, vn)
		}
		be.tree = be.collectionDeriveTree()
		return be
	}
	if v := valueNodeOfSnippet(content); v != nil {
		be.node = v
		be.tree = be.resyncTreeFromYAML()
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
	return syncTreeCheckedFromNode(be.tree, be.node)
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

func (be blockEditState) commit() (blockEditState, tea.Cmd) {
	var snippet string
	if be.isCollectionNav() {
		be = be.flushCurrentEntry()
		if be.editorErr.kind != errNone {
			return be, nil
		}
		snippet = nodeToContent(be.key, be.node)
	} else {
		be.editorErr = editorError{}
		snippet = be.yamlEditor.Value()
	}

	if err := validateSnippetText(snippet); err != nil {
		be.editorErr = editorError{kind: errCommit, message: fmt.Sprintf("Invalid YAML: %v", err)}
		return be, nil
	}
	if be.knownByPath != nil {
		if unknown := schema.UnknownKeys([]byte(snippet), be.knownByPath); len(unknown) > 0 {
			be.editorErr = editorError{kind: errCommit, message: fmt.Sprintf("Unknown keys: %s", strings.Join(unknown, ", "))}
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

	rightPanel := topPanel
	if be.cfg.EnableHints {
		hintActive := be.active == blockEditPanelHint
		hintPanel := theme.RenderTitledPanelWith("Hint/Example", theme.Size{W: be.rightW, H: be.hintH() + 2}, hintActive, be.scrolledHintContent(), be.theme.colors)
		rightPanel = lipgloss.JoinVertical(lipgloss.Left, topPanel, hintPanel)
	}

	hintText := be.currentHint()

	var feedback string
	switch {
	case be.editorErr.kind != errNone:
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.errorText.Render(be.editorErr.message))
	case be.dirty:
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.status.Render(msgUncommittedChanges))
	case be.statusMsg != "":
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.status.Render(be.statusMsg))
	}
	hint := lipgloss.NewStyle().Width(be.width).Render(be.theme.status.Render(hintText))

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Hint: hint})
	if be.height > 0 {
		if lines := strings.Split(out, "\n"); len(lines) > be.height {
			out = strings.Join(lines[:be.height], "\n")
		}
	}
	return out
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
	parts = append(parts, keyCtrlUUndo, keyCtrlYRedo, keyTabPane, keyCtrlSSaveChg, keyEscBack)
	return strings.Join(parts, hintSep)
}

func (be blockEditState) presetView(parentSegs []string) string {
	segs := append(append(parentSegs, be.key), be.tree.BreadcrumbSegments()...)
	header := theme.RenderHeaderWith(be.cfg.Title, strings.Join(segs, " › "), "", be.width, be.theme.colors)

	leftPanel := theme.RenderTitledPanelWith("Available Presets", theme.Size{W: be.listW, H: be.innerH() + 2}, !be.preset.previewFocus, be.preset.listView(be.theme), be.theme.colors)
	rightPanel := theme.RenderTitledPanelWith("Preset Preview", theme.Size{W: be.rightW, H: be.innerH() + 2}, be.preset.previewFocus, be.preset.previewView(be.innerH()), be.theme.colors)

	hintStr := hintPresetListScalar
	if be.preset.previewFocus {
		hintStr = hintPresetPreviewFocused
	} else if be.isCollectionNav() {
		hintStr = hintPresetListCollection
	}
	hint := lipgloss.NewStyle().Width(be.width).Render(be.theme.status.Render(hintStr))

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Hint: hint})
	if be.height > 0 {
		if lines := strings.Split(out, "\n"); len(lines) > be.height {
			out = strings.Join(lines[:be.height], "\n")
		}
	}
	return out
}

// validateSnippetText checks that text is valid YAML.
func validateSnippetText(text string) error {
	var check any
	return yaml.Unmarshal([]byte(text), &check)
}

// --- Hint panel ----------------------------------------------------------

// fieldItemView renders the left panel for a tree-less block (primitive, enum,
// or free-form collection): a single non-toggleable row naming the field being
// edited. There are no sub-fields to navigate, so the row is just an anchor -
// the field's metadata lives in the Hint/Example panel.
func (be blockEditState) fieldItemView() string {
	return be.theme.existingItem.Render(" ▸ " + be.key)
}

// hintContent returns the rendered string for the bottom-right hint panel.
// scrolledHintContent returns the hint content clipped to hintH() lines,
// starting at hintScroll. Used when hint panel has focus for scrolling.
func (be blockEditState) scrolledHintContent() string {
	content := be.hintContent()
	if content == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	h := be.hintH()
	maxScroll := len(lines) - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := be.hintScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + h
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
}

func (be blockEditState) hintContent() string {
	// Tree-less blocks (primitive/enum/free-form collection) have no field nodes;
	// show the block's own metadata instead of the "select a field" placeholder.
	if be.tree.isEmpty() {
		return be.fieldHintFor(be.def.YAMLName)
	}
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be.theme.hintDim.Render("  select a field to see hints")
	}
	node := be.tree.nodes[idx]
	if node.kind != treeNodeField {
		return be.theme.hintDim.Render("  select a field to see hints")
	}
	fieldPath := strings.Join(node.yamlPath, ".")
	if be.isCollectionNav() && len(node.yamlPath) > 0 {
		fieldPath = strings.Join(node.yamlPath[1:], ".")
	}
	return be.fieldHintFor(fieldPath)
}

// fieldHintFor builds the hint text for a single field definition.
// fieldPath is the dot-joined path from the block root (e.g. "source.path").
func (be blockEditState) fieldHintFor(fieldPath string) string {
	if be.cfg.Metadata == nil {
		return be.theme.hintDim.Render("  Config.Metadata is not set - no metadata source configured")
	}
	meta := be.cfg.Metadata.FieldMeta(be.key, fieldPath)
	ex := meta.Example
	if ex == "" && meta.Multiline {
		fieldName := fieldPath
		if i := strings.LastIndex(fieldPath, "."); i >= 0 {
			fieldName = fieldPath[i+1:]
		}
		ex = fieldName + ": |\n  line 1\n  line 2\n"
	}
	if out := renderFieldHint(be.theme, meta, ex); out != "" {
		return out
	}
	return be.theme.hintDim.Render("  no metadata declared for this field")
}
