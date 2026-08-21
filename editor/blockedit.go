package editor

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/alert"
	"github.com/lucasassuncao/yedit/animation"
	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/legend"
	"github.com/lucasassuncao/yedit/presetbrowser"
	"github.com/lucasassuncao/yedit/render"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
	"github.com/lucasassuncao/yedit/yamledit"
	"github.com/lucasassuncao/yedit/yamlnode"

	"github.com/lucasassuncao/yedit/keys"
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
// mode is active at a time, and the helper fields (confirmAlert, preset…) are
// only meaningful in their own mode.
type blockEditMode int

const (
	modeEditing       blockEditMode = iota // editing tree/yaml panels
	modePresetBrowser                      // preset picker overlay
	modeConfirming                         // confirm alert overlay
)

// errKind classifies an editor error so blocking logic can be precise.
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

	tree        fieldtree.Model
	childDefs   []schema.FieldDef
	kind        schema.Kind
	def         schema.FieldDef  // the block's own definition; drives the hint panel for tree-less blocks
	coll        collectionBuffer // non-zero only for collection-nav editors
	knownByPath map[string]map[string]bool

	// node is the block's canonical value node, the single source of truth the
	// tree is projected from. Tree-driven toggles mutate it structurally and the
	// YAML editor is re-rendered from it. Collection blocks still carry their
	// entry list in coll for now.
	node yaml.Node

	yamlEditor      textarea.Model
	previewRenderer *glamour.TermRenderer
	active          blockEditPanel
	prevActive      blockEditPanel  // panel to return to when leaving hint focus
	showHint        bool            // split the right column to show the Hint/Example panel
	hintAnim        animation.Tween // in-flight show/hide transition for the hint panel; inactive unless Config.AnimationDuration is set
	hintScroll      int             // scroll offset in hint panel when active == blockEditPanelHint
	previewScroll   int             // 1-based YAML line the Preview keeps visible; 0 = top

	isEdit        bool   // false = add new block, true = edit existing
	dirty         bool   // uncommitted changes since last ctrl+s
	committedYAML string // normalized YAML at last ctrl+s (or open); used to reset dirty when content reverts

	// focus is this editor's address within the model's canonical editRoot tree:
	// nil for the top-level editor, otherwise the indexed path to the drilled-into
	// node. Content is flushed back into editRoot here on navigation/commit.
	focus []yamledit.PathSeg

	width, height int
	listW, rightW int

	editorErr     editorError
	statusMsg     string // neutral feedback (e.g. "Undone."); cleared on next edit action
	currentPreset string

	mode                blockEditMode
	preset              presetbrowser.Model
	confirmAlert        alert.Model
	confirmAlertVisible bool

	undoStack   []blockEditUndoSnap // undo history; each mutating op pushes a snapshot
	redoStack   []blockEditUndoSnap // redo history; populated by restoreUndo, discarded on new mutations
	actionLog   []BlockAction       // in-memory log for debug and replay
	theme       theme.Resolved
	help        help.Model
	legendLines int // lines consumed by the legend bar; updated on resize and init
}

// blockOwnDef returns the block's own field definition, synthesizing a minimal
// one from the spec when the caller supplied no metadata (nested editors,
// unknown keys, tests) so YAMLName and Kind are always set.
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
		theme:         theme.Resolve(cfg.Theme),
		showHint:      cfg.EnableHints,
	}
	be.help = legend.NewHelp(be.theme)
	be.help.SetWidth(w - 1)
	_, be.legendLines = legend.Render(be.help, be.currentKeyMap(), w-1)
	be = be.relayout()

	be.tree = fieldtree.New(spec.kind, spec.defs, spec.content, be.innerH())

	// Structured collections ([]Struct / map[string]Struct) keep their canonical
	// entry list in be.node; the tree and per-entry editor are projected from it.
	structured := (spec.kind == schema.KindList || spec.kind == schema.KindDictionary) && len(spec.defs) > 0
	if structured {
		raw := spec.content
		if raw == "" {
			raw = spec.key + ":\n"
		}
		be.coll = collectionBuffer{key: spec.key, isMap: be.isMapNav(), current: -1}
		be.node = *yamledit.CollValueNode(raw, be.isMapNav())
		be.tree.Nodes = be.collectionTreeNodes()
	}

	content := spec.content
	if content == "" {
		content = spec.key + ":\n"
	}

	be.yamlEditor = be.newYAMLEditor(content)

	// Non-collection blocks carry their canonical node from the start. Derive the
	// tree once here so it reflects be.node even when content came from a preset
	// rather than spec.content.
	if !structured {
		if v := yamledit.BlockValueNodeOrNil(content); v != nil {
			be.node = *v
		} else {
			be.editorErr = editorError{kind: errParse, message: "Could not parse block content."}
			be.node = yaml.Node{Kind: yaml.MappingNode}
		}
		be.tree = fieldtree.SyncCheckedFromNode(be.tree, &be.node)
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

	// Baseline for dirty-tracking: the normalized open state. Non-collection
	// blocks normalize the buffer, so an unparseable block on disk still reads as
	// clean until edited.
	if structured {
		be.committedYAML = yamledit.NodeToContent(be.key, &be.node)
	} else {
		be.committedYAML = yamledit.NormalizeBlockContent(be.key, be.yamlEditor.Value())
	}

	return be
}

// computeDirty reports whether the editor differs from committedYAML. Derived at
// the dispatch boundary rather than maintained per mutation, so content that
// returns to the baseline reads as clean again.
func (be blockEditState) computeDirty() bool {
	if be.isCollectionNav() {
		if yamledit.NodeToContent(be.key, &be.node) != be.committedYAML {
			return true
		}
		// The buffer may hold unflushed edits of the current entry.
		return be.yamlEditor.Value() != be.entryYAML(be.coll.current)
	}
	return yamledit.NormalizeBlockContent(be.key, be.yamlEditor.Value()) != be.committedYAML
}

func (be blockEditState) newYAMLEditor(content string) textarea.Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	// A custom prompt replaces the built-in line-number gutter so it matches the
	// Preview panel's "%4d │ " gutter (numberPreviewLines) exactly.
	rt := be.theme
	ta.SetPromptFunc(render.PreviewGutterWidth, func(info textarea.PromptInfo) string {
		return rt.HintDim.Render(fmt.Sprintf("%4d │ ", info.LineNumber+1))
	})
	// The library's MaxHeight default of 99 silently caps both the viewport and
	// the number of logical lines the buffer accepts, truncating YAML blocks
	// longer than 99 lines. We manage height via SetHeight, so disable the cap.
	ta.MaxHeight = 0
	ta.SetWidth(be.rightW - 2)
	ta.SetHeight(be.editorH() - 1)
	ta.CharLimit = 0
	ta.Blur()
	if content != "" {
		ta.SetValue(strings.ReplaceAll(content, "\r\n", "\n"))
	}
	return ta
}

func (be blockEditState) relayout() blockEditState {
	be.listW, be.rightW = theme.TwoColumnWidths(be.width)
	be.previewRenderer = render.NewPreviewRenderer(be.rightW - 2 - render.PreviewGutterWidth)
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

// hintVisible reports whether the hint panel is drawn. It stays true
// mid-animation but goes false the moment the eased height reaches zero - see
// model.hintVisible for why a zero-height panel must not be rendered.
func (be blockEditState) hintVisible() bool { return be.hintH() > 0 }

// hintTargetH is the height the hint panel settles at once shown: ~1/3 of the
// right column, floored at 5 lines and never squeezing the editor below 5.
func (be blockEditState) hintTargetH() int {
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

// hintH is the hint panel's content height as drawn right now: the interpolated
// height while a tween is in flight, hintTargetH once settled, 0 when hidden.
//
// hintTargetH's floors describe the resting size only; reapplying them every
// frame would make the panel jump straight to 5 lines instead of growing from 0.
func (be blockEditState) hintH() int {
	if !be.cfg.EnableHints {
		return 0
	}
	if be.hintAnim.Active() {
		return be.hintAnim.Cur
	}
	if !be.showHint {
		return 0
	}
	return be.hintTargetH()
}

// editorH returns the content height of the top-right panel (editor/preview).
func (be blockEditState) editorH() int {
	if !be.hintVisible() {
		return be.innerH()
	}
	h := be.innerH() - 2 - be.hintH()
	if h < 0 {
		h = 0
	}
	return h
}

// startHintAnim eases the hint panel from its current drawn height towards the
// height implied by the new be.showHint, reporting whether a tick loop must
// start. from must be sampled before be.showHint is flipped.
func (be blockEditState) startHintAnim(from int) (blockEditState, bool) {
	running := be.hintAnim.Active()
	target := 0
	if be.showHint {
		target = be.hintTargetH()
	}
	be.hintAnim = animation.New(from, target, be.cfg.AnimationDuration)
	// A rapid double toggle retargets the existing tween instead of stacking a
	// second ticker.
	return be, be.hintAnim.Active() && !running
}

func (be blockEditState) Init() tea.Cmd { return textarea.Blink }

// enterConfirmAlert is the single entry point for the confirm modal, so every
// dialog opens the same way.
func (be blockEditState) enterConfirmAlert(al alert.Model) blockEditState {
	be.confirmAlert = al
	be.confirmAlertVisible = true
	be.mode = modeConfirming
	return be
}

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

	// Animation frames advance regardless of mode, so they precede the mode
	// switch. At runtime handleHintAnimTick routes them here; this case keeps
	// be.Update self-contained for tests that drive it directly.
	if _, ok := msg.(hintAnimTickMsg); ok {
		return be.advanceHintAnim()
	}

	if m, ok := msg.(tea.WindowSizeMsg); ok {
		be.width = m.Width
		be.height = m.Height
		be.help.SetWidth(be.width - 1)
		_, be.legendLines = legend.Render(be.help, be.currentKeyMap(), be.width-1)
		be = be.relayout()
		be.yamlEditor.SetWidth(be.rightW - 2)
		be.yamlEditor.SetHeight(be.editorH() - 1)
		be.tree.Height = be.innerH()
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
		// Global shortcuts stay live under the overlay so Ctrl+S / Ctrl+L never
		// appear to be unavailable.
		switch {
		case key.Matches(km, keys.CtrlSSaveCh):
			return be, func() tea.Msg { return commitRequestedMsg{} }
		case key.Matches(km, keys.CtrlLValid):
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
	case presetbrowser.Applied:
		if be.cfg.BlockPresets != nil {
			y, err := be.cfg.BlockPresets.PresetYAML(be.key, name)
			if err != nil {
				be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("preset error: %v", err)}
			} else {
				be = be.dispatch(ApplyPreset{Name: name, Content: y})
			}
		}
	case presetbrowser.Appended:
		if be.cfg.BlockPresets != nil {
			y, err := be.cfg.BlockPresets.PresetYAML(be.key, name)
			if err != nil {
				be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("preset error: %v", err)}
			} else {
				be = be.dispatch(AppendPreset{Name: name, Content: y})
			}
		}
	case presetbrowser.None:
		return be, nil
	}
	// presetbrowser.Dismissed, presetbrowser.Applied, presetbrowser.Appended all close the browser.
	be.mode = modeEditing
	return be, nil
}

func (be blockEditState) updateEditing(msg tea.Msg) (blockEditState, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		if be.active == blockEditPanelYAML {
			return be.updateNonKeyBuffer(msg)
		}
		return be, nil
	}
	return be.updateKey(key)
}

// updateNonKeyBuffer applies a non-key message (e.g. a clipboard paste) to the
// YAML editor. The undo checkpoint must pair the pre-change buffer with the
// pre-change node, or undo would restore the node and leave the pasted text in
// place. The textarea update leaves the node untouched, so capture after it and
// rewind only the buffer before pushing.
func (be blockEditState) updateNonKeyBuffer(msg tea.Msg) (blockEditState, tea.Cmd) {
	prev := be.yamlEditor.Value()
	var cmd tea.Cmd
	be.yamlEditor, cmd = be.yamlEditor.Update(msg)
	if be.yamlEditor.Value() == prev {
		return be, cmd
	}
	snap := be.captureSnap()
	snap.yamlValue = prev
	if n := len(be.undoStack); n == 0 || !snapEqual(be.undoStack[n-1], snap) {
		be.undoStack = appendSnapCapped(be.undoStack, snap)
		be.redoStack = nil
	}
	be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value(), Checkpoint: false})
	return be, cmd
}

// handleHintKey handles ctrl+h (toggle hint focus) and navigation when the hint
// panel is focused. Returns (state, true) when it consumed the key.
func (be blockEditState) handleHintKey(msg tea.KeyMsg) (blockEditState, bool) {
	if key.Matches(msg, keys.CtrlHHint) && be.cfg.EnableHints && be.showHint {
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
	case key.Matches(msg, keys.Up):
		if be.hintScroll > 0 {
			be.hintScroll--
		}
	case key.Matches(msg, keys.Down):
		// Bound by content height, not panel height: otherwise the tail of a hint
		// longer than two panel-fulls stays unreachable.
		lines := strings.Count(strings.TrimSuffix(be.hintContent(), "\n"), "\n") + 1
		maxScroll := lines - be.hintH()
		if maxScroll < 0 {
			maxScroll = 0
		}
		if be.hintScroll < maxScroll {
			be.hintScroll++
		}
	case key.Matches(msg, keys.Tab, keys.CtrlHHint):
		be.active = be.prevActive
	}
	return be, true
}

func (be blockEditState) updateKey(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	if key.Matches(msg, keys.Esc) {
		// Nested editor: Esc goes up one level and the model flushes the edits into
		// the canonical tree. Nothing is lost, so no discard prompt - that only
		// guards leaving the block edit entirely.
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
			return be.enterConfirmAlert(al), nil
		}
		return be, func() tea.Msg { return blockEditDiscardedMsg{discarded: false} }
	}

	// Ctrl+S commits the editor stack into the document. That needs model access,
	// so the block layer requests it as a message the root Update handles.
	if key.Matches(msg, keys.CtrlSSaveCh) {
		return be, func() tea.Msg { return commitRequestedMsg{} }
	}
	// Ctrl+L triggers doc-level validation (available in every mode).
	if key.Matches(msg, keys.CtrlLValid) {
		return be, func() tea.Msg { return validateRequestedMsg{} }
	}

	// Ctrl+U / Ctrl+Y: block-level undo/redo. Empty stacks only report status.
	if key.Matches(msg, keys.CtrlUUndo) {
		if len(be.undoStack) == 0 {
			be.statusMsg = "Nothing to undo."
			return be, nil
		}
		return be.dispatch(Undo{}), nil
	}
	if key.Matches(msg, keys.CtrlYRedo) {
		if len(be.redoStack) == 0 {
			be.statusMsg = "Nothing to redo."
			return be, nil
		}
		return be.dispatch(Redo{}), nil
	}

	// H toggles the hint panel, mirroring the root list view. Checked before
	// handleHintKey, which otherwise captures every key while the hint panel has
	// focus, and skipped on the YAML panel so typing "h" still inserts it.
	if key.Matches(msg, keys.Hint) && be.cfg.EnableHints && be.active != blockEditPanelYAML {
		// Sample the on-screen height before flipping the flag: mid-flight it is
		// neither 0 nor the settled target.
		from := be.hintH()
		be.showHint = !be.showHint
		if !be.showHint && be.active == blockEditPanelHint {
			be.active = be.prevActive
		}
		be, tick := be.startHintAnim(from)
		// editorH() changed with showHint, and the textarea's own height is only
		// set at creation/resize.
		be.yamlEditor.SetHeight(be.editorH() - 1)
		if tick {
			return be, hintAnimTick(true)
		}
		return be, nil
	}

	// Ctrl+H toggles hint focus; when focused it also captures navigation.
	if be2, handled := be.handleHintKey(msg); handled {
		return be2, nil
	}

	if key.Matches(msg, keys.Tab) {
		return be.switchPanel(), nil
	}

	if be.active == blockEditPanelTree {
		if key.Matches(msg, keys.Preset) {
			return be.openPresetPicker(), nil
		}
		return be.updateTreePanel(msg)
	}

	// YAML panel active. The buffer may be transiently invalid while typing, and
	// keystrokes are never blocked. The canonical node is parse-gated below, so
	// the tree freezes at the last good state instead of disagreeing with it.
	// editRoot is touched only at flush (navigation/commit).
	prevValue := be.yamlEditor.Value()
	// Tree-less blocks open with the YAML panel focused, so the switchPanel
	// checkpoint never fires. Capture the pre-edit state whenever there is nothing
	// to fall back to, so ctrl+u can always return to the content before this
	// keystroke. It must be taken before Update: the textarea shares its buffer
	// internals, so a struct copy would alias the post-keystroke content.
	var preSnap *blockEditUndoSnap
	if len(be.undoStack) == 0 {
		snap := be.captureSnap()
		preSnap = &snap
	}
	var cmd tea.Cmd
	be.yamlEditor, cmd = be.yamlEditor.Update(msg)
	// Only re-project on a real content change: cursor moves and selection leave
	// the tree unchanged, so there is no reason to re-parse the buffer.
	if be.yamlEditor.Value() != prevValue {
		if preSnap != nil {
			be.undoStack = appendSnapCapped(nil, *preSnap)
		}
		// A real edit forks away from the undone states.
		be.redoStack = nil
		be = be.dispatch(SyncYAML{Content: be.yamlEditor.Value(), Checkpoint: false})
	}
	return be, cmd
}

// syncParsedNode is the parse gate run after every YAML keystroke: it advances
// the canonical node only when content parses, leaving the last good state in
// place otherwise. Returns false when nothing changed.
func (be blockEditState) syncParsedNode(content string) (blockEditState, bool) {
	if be.isCollectionNav() {
		kn, vn, ok := yamledit.ParseEntryFromView(content, be.coll.isMap)
		if !ok {
			return be, false
		}
		return be.applyParsedEntry(kn, vn), true
	}
	if v := yamledit.ValueNodeOfSnippet(content); v != nil {
		be.node = *v
		return be, true
	}
	return be, false
}

// applyParsedEntry writes kn/vn into be.node at the cursor, appending the first
// entry when the collection is empty so a direct YAML edit is not discarded.
func (be blockEditState) applyParsedEntry(kn, vn *yaml.Node) blockEditState {
	cur := be.coll.current
	count := yamledit.EntryCount(&be.node, be.coll.isMap)
	// A map key renamed onto an existing one would splice a duplicate into the
	// canonical mapping. flushCurrentEntry guards navigation/commit; this
	// per-keystroke path writes into the node too, so it needs the same gate.
	if be.coll.isMap && kn != nil && duplicateMapKey(&be.node, cur, kn.Value) {
		be.editorErr = editorError{kind: errParse, message: fmt.Sprintf("Duplicate map key %q - rename it to a unique key first.", kn.Value)}
		return be
	}
	switch {
	case cur >= 0 && cur < count:
		yamledit.SetEntry(&be.node, be.coll.isMap, cur, kn, vn)
	case count == 0:
		if be.coll.isMap {
			be.node.Content = append(be.node.Content, kn, vn)
		} else {
			be.node.Content = append(be.node.Content, vn)
		}
		be.coll.current = 0
		be.tree.Nodes = be.collectionTreeNodes()
	default:
		return be
	}
	// The write succeeded, so a stale duplicate-key error no longer applies.
	if be.editorErr.kind == errParse {
		be.editorErr = editorError{}
	}
	return be
}

// resyncTreeFromYAML re-derives the tree's checked states from the canonical
// node, so the tree can never disagree with it even mid-edit.
func (be blockEditState) resyncTreeFromYAML() fieldtree.Model {
	if be.isCollectionNav() {
		return be.collectionDeriveTree()
	}
	return fieldtree.SyncCheckedFromNode(be.tree, &be.node)
}

// snippetsFn looks up FieldMeta.Snippet scoped to be.key, or nil when no
// MetadataSource is configured.
func (be blockEditState) snippetsFn() func(string) string {
	if be.cfg.Metadata == nil {
		return nil
	}
	return func(fieldName string) string {
		return be.cfg.Metadata.FieldMeta(be.key, fieldName).Snippet
	}
}

// withPreCheckedFields toggles on the fields marked FieldMeta.PreChecked and
// inserts their snippets. Only for new struct blocks, so opening an existing
// block never modifies content.
func (be blockEditState) withPreCheckedFields() blockEditState {
	if be.cfg.Metadata == nil {
		return be
	}
	ctx := yamledit.ToggleCtx{Snippets: be.snippetsFn(), ChildDefs: be.childDefs}
	changed := false
	for _, n := range be.tree.Nodes {
		if n.Kind != fieldtree.KindField || n.Depth != 0 || n.Checked {
			continue
		}
		meta := be.cfg.Metadata.FieldMeta(be.key, n.Label)
		if meta.PreChecked {
			be.node = *fieldtree.ToggleNodeField(&be.node, ctx, n, true)
			changed = true
		}
	}
	if !changed {
		return be
	}
	be.yamlEditor.SetValue(yamledit.NodeToContent(be.key, &be.node))
	be.tree = fieldtree.SyncCheckedFromNode(be.tree, &be.node)
	return be
}

// resyncAfterCommit reloads the editor from the freshly committed block so a
// repeated Ctrl+S is idempotent; the committed baseline is reset, so dirty reads
// clean.
//
// Unused at runtime: commitAll returns to the list and discards the editor
// stack. Kept with its test for a future commit-in-place flow.
func (be blockEditState) resyncAfterCommit(fresh string) blockEditState {
	if !be.isCollectionNav() {
		if v := yamledit.BlockValueNodeOrNil(fresh); v != nil {
			be.node = *v
		} else {
			be.node = yaml.Node{Kind: yaml.MappingNode}
		}
		be.yamlEditor.SetValue(fresh)
		be.committedYAML = yamledit.NodeToContent(be.key, &be.node)
		be.dirty = be.computeDirty()
		return be
	}
	isMap := be.isMapNav()
	oldCount := yamledit.EntryCount(&be.node, isMap)
	be.node = *yamledit.CollValueNode(fresh, isMap)
	if yamledit.EntryCount(&be.node, isMap) != oldCount {
		// Entry count changed: rebuild the tree, losing expansion state, since the
		// structure must match the new node.
		be.tree.Nodes = be.collectionTreeNodes()
		if be.coll.current >= yamledit.EntryCount(&be.node, isMap) {
			be.coll.current = yamledit.EntryCount(&be.node, isMap) - 1
		}
	}
	be.tree = be.collectionDeriveTree()
	be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
	be.committedYAML = yamledit.NodeToContent(be.key, &be.node)
	be.dirty = be.computeDirty()
	return be
}

func (be blockEditState) switchPanel() blockEditState {
	if be.active == blockEditPanelTree {
		// Checkpoint before YAML editing so manual changes are undoable.
		be = be.saveUndo()
		be.active = blockEditPanelYAML
		be.yamlEditor.Focus()
	} else {
		be.active = blockEditPanelTree
		be.yamlEditor.Blur()
	}
	return be
}

// commit validates the editor's content and returns its canonical value node,
// or (nil, false) with the detail in be.editorErr. The node is detached data and
// commit performs no effect itself, leaving the caller to write it into the
// canonical tree or serialize it. Returning the node rather than a snippet
// spares the caller a lossy parse-back: yamledit.ParseBlockText already rejected stray or
// renamed top-level keys with a user-facing message.
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
		v, errMsg := yamledit.ParseBlockText(be.key, be.yamlEditor.Value())
		if errMsg != "" {
			be.editorErr = editorError{kind: errCommit, message: errMsg}
			return be, nil, false
		}
		val = v
	}

	// Final gate against duplicate mapping keys: schema.UnknownKeys cannot see
	// them (yaml.v3 keeps the last value), so one that slipped past the flush
	// guards would be persisted verbatim.
	if path, dup := yamledit.FindDuplicateMappingKey(val); dup {
		be.editorErr = editorError{kind: errCommit, message: fmt.Sprintf("Duplicate key %q - remove or rename it first.", path)}
		return be, nil, false
	}

	if be.knownByPath != nil {
		unknown, err := schema.UnknownKeys([]byte(yamledit.NodeToContent(be.key, val)), be.knownByPath)
		if err != nil {
			be.editorErr = editorError{kind: errCommit, message: fmt.Sprintf("Unknown keys check failed: %v", err)}
			return be, nil, false
		}
		if len(unknown) > 0 {
			be.editorErr = editorError{kind: errCommit, message: fmt.Sprintf("Unknown keys: %s", strings.Join(unknown, ", "))}
			return be, nil, false
		}
	}
	// dirty is deliberately not cleared: flushTopToRoot calls commit() during
	// drill-in/out, where edits reached editRoot but not the document, and
	// clearing it would bypass the "Discard changes?" guard on a later Esc.
	// commitAll discards the whole editor stack, so the flag dies with it.

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
	if be.tree.IsEmpty() {
		leftTitle, leftContent = "Field", be.fieldItemView()
	}
	leftPanel := theme.RenderTitledPanelWith(leftTitle, theme.Size{W: be.listW, H: be.innerH() + 2}, treeActive, leftContent, be.theme.Colors)

	yamlActive := be.active == blockEditPanelYAML
	var topTitle, topContent string
	if !yamlActive {
		topTitle = "Preview"
		// The preview follows the tree selection; rendered preview lines map ~1:1
		// to YAML lines.
		preview := numberPreviewLines(render.PreviewYAML(be.yamlEditor.Value(), be.previewRenderer), be.theme)
		topContent = scrollLinesTo(preview, be.editorH(), be.previewScroll)
	} else {
		topTitle = "Editing YAML"
		topContent = render.ClampLines(be.yamlEditor.View(), be.editorH())
	}
	topPanel := theme.RenderTitledPanelWith(topTitle, theme.Size{W: be.rightW, H: be.editorH() + 2}, yamlActive, topContent, be.theme.Colors)

	rightPanel := topPanel
	if be.hintVisible() {
		hintActive := be.active == blockEditPanelHint
		hintPanel := theme.RenderTitledPanelWith("Hint/Example", theme.Size{W: be.rightW, H: be.hintH() + 2}, hintActive, be.scrolledHintContent(), be.theme.Colors)
		rightPanel = lipgloss.JoinVertical(lipgloss.Left, topPanel, hintPanel)
	}

	feedback := be.feedbackLine()
	legendBar := legend.HelpLine(be.width, be.help, be.currentKeyMap())

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Legend: legendBar})
	if be.height > 0 {
		out = render.ClampLines(out, be.height)
	}
	if be.confirmAlertVisible {
		out = theme.CompositeCenter(be.confirmAlert.Box(), out)
	}
	return out
}

func (be blockEditState) presetView(parentSegs []string) string {
	header := be.breadcrumbHeader(parentSegs)

	leftPanel := theme.RenderTitledPanelWith("Available Presets", theme.Size{W: be.listW, H: be.innerH() + 2}, !be.preset.PreviewFocus, be.preset.ListView(be.theme), be.theme.Colors)
	rightPanel := theme.RenderTitledPanelWith("Preset Preview", theme.Size{W: be.rightW, H: be.innerH() + 2}, be.preset.PreviewFocus, be.preset.PreviewView(be.innerH()), be.theme.Colors)

	var presetKM help.KeyMap
	switch {
	case be.preset.PreviewFocus:
		presetKM = legend.PresetPreview{}
	case be.isCollectionNav():
		presetKM = legend.PresetListCollection{}
	default:
		presetKM = legend.PresetListScalar{}
	}
	legendBar := legend.HelpLine(be.width, be.help, presetKM)

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Legend: legendBar})
	if be.height > 0 {
		out = render.ClampLines(out, be.height)
	}
	return out
}

// validateSnippetText checks that text is valid YAML.
func validateSnippetText(text string) error {
	var check any
	return yaml.Unmarshal([]byte(text), &check)
}
