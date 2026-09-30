package editor

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/bezel/animation"
	"github.com/lucasassuncao/bezel/browser"
	"github.com/lucasassuncao/bezel/draw"
	"github.com/lucasassuncao/bezel/layout"
	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/shell"
	"github.com/lucasassuncao/bezel/textbox"
	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/render"
	"github.com/lucasassuncao/yedit/schema"
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
	// metaBlock and metaPrefix address a drilled-in editor in the metadata tree,
	// which is keyed by the root block; empty for the top-level editor.
	metaBlock, metaPrefix string

	width, height int
	// sh is the chrome: size, layout, status row, legend and the confirm overlay.
	sh shell.Shell

	editorErr     editorError
	statusMsg     string // neutral feedback (e.g. "Undone."); cleared on next edit action
	currentPreset string

	mode   blockEditMode
	preset browser.Model

	undoStack []blockEditUndoSnap // undo history; each mutating op pushes a snapshot
	redoStack []blockEditUndoSnap // redo history; populated by restoreUndo, discarded on new mutations
	actionLog []BlockAction       // in-memory log for debug and replay
	theme     theme.Resolved
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
		theme:         theme.Resolve(cfg.Theme, true),
		showHint:      cfg.EnableHints,
	}
	be.sh = shell.New(shell.Config{Layout: blockLayout(0), Theme: be.theme, Title: cfg.Title, LegendLines: cfg.LegendLines})
	be.sh, _, _ = be.sh.Update(tea.WindowSizeMsg{Width: w, Height: h})
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

// withTheme swaps the styles once the terminal has said whether it is dark.
func (be blockEditState) withTheme(rt theme.Resolved) blockEditState {
	be.theme = rt
	be.sh = be.sh.SetTheme(rt)
	be.yamlEditor = textbox.Restyle(be.yamlEditor, rt)
	return be
}

func (be blockEditState) newYAMLEditor(content string) textarea.Model {
	// The gutter matches the Preview panel's, and the height is the layout's.
	ta := textbox.New(be.theme)
	ta.SetWidth(be.rightW())
	ta.SetHeight(be.editorH() - 1)
	ta.Blur()
	if content != "" {
		textbox.SetText(&ta, content)
	}
	return ta
}

// blockLayout is the editor screen: the field tree on the left, the YAML
// editor or preview on the right with the hint panel under it when open.
func blockLayout(hintH int) layout.Node {
	right := layout.Node(layout.Fill("editor"))
	if hintH > 0 {
		right = layout.Rows(layout.Fill("editor"), layout.Fixed("hint", layout.Lines(hintH+2)).Info())
	}
	return layout.Columns(listColumn("fields"), right)
}

func presetLayout() layout.Node {
	return layout.Columns(listColumn("presets"), layout.Fill("preview"))
}

func (be blockEditState) currentLayout() layout.Node {
	if be.mode == modePresetBrowser {
		return presetLayout()
	}
	return blockLayout(be.hintH())
}

// relayout re-resolves the shell for the current mode and hint height and
// rebuilds the preview renderer for the width it placed the editor at.
func (be blockEditState) relayout() blockEditState {
	be.sh = be.sh.SetLayout(be.currentLayout()).SetActions(be.actions()...)
	be.previewRenderer = render.NewPreviewRenderer(max(1, be.rightW()-draw.GutterWidth))
	return be
}

// innerH is the content height of the left panel, shared by every pane on
// the screen. rightW is the content width of the editor column.
func (be blockEditState) innerH() int { return draw.InnerRect(be.sh.Rect("fields")).H }

func (be blockEditState) rightW() int {
	if be.mode == modePresetBrowser {
		return draw.InnerRect(be.sh.Rect("preview")).W
	}
	return draw.InnerRect(be.sh.Rect("editor")).W
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

// editorH returns the content height of the top-right panel (editor/preview):
// what is left after the hint panel took its share.
func (be blockEditState) editorH() int {
	if !be.hintVisible() {
		return be.innerH()
	}
	return max(be.innerH()-2-be.hintH(), 0)
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
func (be blockEditState) enterConfirmAlert(o overlay.Overlay) blockEditState {
	be.sh = be.sh.Push(o)
	be.mode = modeConfirming
	return be
}

// confirm builds the block editor's yes/no modal, running onYes when accepted.
func (be blockEditState) confirm(title, message string, onYes tea.Cmd) overlay.Overlay {
	return overlay.NewConfirm(title, message, onYes, be.theme.Modal, be.theme.Legend)
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
		return be.dispatch(ToggleField{NodeIdx: m.nodeIdx, Checked: false}), nil
	}
	if m, ok := msg.(pendingEntryDeleteMsg); ok {
		be.mode = modeEditing
		return be.dispatch(DeleteEntry{SeqIdx: m.seqIdx}), nil
	}
	// The confirm's own close arrives beside the pending message above, in
	// either order, so the overlay is popped regardless of mode.
	if _, ok := msg.(overlay.CloseMsg); ok {
		be.sh, _, _ = be.sh.Update(msg)
		if be.mode == modeConfirming {
			be.mode = modeEditing
		}
		return be, nil
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
		be.sh, _, _ = be.sh.Update(m)
		be = be.relayout()
		be.yamlEditor.SetWidth(be.rightW())
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
	if km, ok := msg.(tea.KeyMsg); ok {
		// Global shortcuts stay live under the overlay so Ctrl+S / Ctrl+L never
		// appear to be unavailable.
		switch {
		case key.Matches(km, keys.CtrlSSaveCh):
			return be, func() tea.Msg { return commitRequestedMsg{} }
		case key.Matches(km, keys.CtrlLValid):
			return be, func() tea.Msg { return validateRequestedMsg{} }
		}
		var cmd tea.Cmd
		be.sh, _, cmd = be.sh.Update(km)
		return be, cmd
	}
	return be, nil
}

func (be blockEditState) updatePresetBrowser(msg tea.Msg) (blockEditState, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return be, nil
	}
	// Append is yedit's own key, for collection blocks only; the browser
	// answers for the rest.
	if key.Matches(km, keys.AAppend) && !be.preset.PreviewFocus && be.isCollectionNav() {
		return be.presetChosen(AppendPreset{Name: be.preset.Selected().Label}), nil
	}
	var action browser.Action
	be.preset, action = be.preset.Update(km)
	switch action {
	case browser.Chosen:
		return be.presetChosen(ApplyPreset{Name: be.preset.Selected().Label}), nil
	case browser.Dismissed:
		be.mode = modeEditing
		return be.relayout(), nil
	}
	return be, nil
}

// presetChosen resolves the chosen preset's YAML, dispatches act with it and
// closes the browser.
func (be blockEditState) presetChosen(act BlockAction) blockEditState {
	be.mode = modeEditing
	be = be.relayout()
	name := be.preset.Selected().Label
	y, err := be.cfg.BlockPresets.PresetYAML(be.key, name)
	if err != nil {
		be.editorErr = editorError{kind: errPreset, message: fmt.Sprintf("preset error: %v", err)}
		return be
	}
	switch a := act.(type) {
	case ApplyPreset:
		a.Content = y
		return be.dispatch(a)
	case AppendPreset:
		a.Content = y
		return be.dispatch(a)
	}
	return be
}

func (be blockEditState) updateEditing(msg tea.Msg) (blockEditState, tea.Cmd) {
	if next, cmd, ok := be.handleActionMsg(msg); ok {
		return next, cmd
	}
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
	be.yamlEditor, cmd = textbox.Update(be.yamlEditor, msg)
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

// handleActionMsg runs what the block editor's actions sent. Returns false
// for any other message.
func (be blockEditState) handleActionMsg(msg tea.Msg) (blockEditState, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case shell.FocusMsg:
		return be.focusPanel(msg).relayout(), nil, true
	case beBackMsg:
		next, cmd := be.back()
		return next, cmd, true
	case beUndoMsg:
		if len(be.undoStack) == 0 {
			be.statusMsg = "Nothing to undo."
			return be, nil, true
		}
		return be.dispatch(Undo{}), nil, true
	case beRedoMsg:
		if len(be.redoStack) == 0 {
			be.statusMsg = "Nothing to redo."
			return be, nil, true
		}
		return be.dispatch(Redo{}), nil, true
	case beToggleHintMsg:
		next, cmd := be.toggleHint()
		return next, cmd, true
	case beFocusHintMsg:
		if be.active == blockEditPanelHint {
			be.active = be.prevActive
		} else {
			be.prevActive = be.active
			be.active = blockEditPanelHint
		}
		return be.relayout(), nil, true
	case bePresetMsg:
		return be.openPresetPicker(), nil, true
	}
	return be, nil, false
}

// back is esc. A nested editor goes up one level and the model flushes the
// edits into the canonical tree, so nothing is lost; only leaving the block
// entirely asks before discarding.
func (be blockEditState) back() (blockEditState, tea.Cmd) {
	if len(be.focus) > 0 {
		return be, func() tea.Msg { return drillOutMsg{} }
	}
	if be.dirty {
		al := be.confirm(
			"Discard changes?",
			"Uncommitted changes will be lost.",
			func() tea.Msg { return blockEditDiscardedMsg{discarded: true} },
		)
		return be.enterConfirmAlert(al), nil
	}
	return be, func() tea.Msg { return blockEditDiscardedMsg{discarded: false} }
}

// toggleHint shows or hides the hint panel, mirroring the root list view.
func (be blockEditState) toggleHint() (blockEditState, tea.Cmd) {
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

// scrollHint moves the focused hint panel; every other key stops there.
func (be blockEditState) scrollHint(msg tea.KeyMsg) blockEditState {
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
	}
	return be
}

func (be blockEditState) updateKey(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	// The shell runs this screen's actions; a Help that opened its overlay
	// puts the editor in the mode that closes it.
	if km, ok := msg.(tea.KeyPressMsg); ok {
		if sh, handled, cmd := be.sh.SetFocus(be.activePane()).SetActions(be.actions()...).Update(km); handled {
			be.sh = sh
			if sh.HasOverlay() {
				be.mode = modeConfirming
			}
			return be, cmd
		}
	}

	switch be.active {
	case blockEditPanelHint:
		return be.scrollHint(msg), nil
	case blockEditPanelTree:
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

// snippetsFn looks up FieldMeta.Snippet scoped to this editor, or nil when no
// MetadataSource is configured.
func (be blockEditState) snippetsFn() func(string) string {
	if be.cfg.Metadata == nil {
		return nil
	}
	return func(fieldName string) string {
		return be.fieldMeta(fieldName).Snippet
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
		meta := be.fieldMeta(n.Label)
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

// activePane names the shell leaf of the active panel.
func (be blockEditState) activePane() string {
	switch be.active {
	case blockEditPanelYAML:
		return "editor"
	case blockEditPanelHint:
		return "hint"
	}
	return "fields"
}

// focusPanel runs what the shell's focus move means here: the YAML editor
// holds the cursor while focused and checkpoints undo on the way in.
func (be blockEditState) focusPanel(msg shell.FocusMsg) blockEditState {
	if msg.From == "editor" {
		be.yamlEditor.Blur()
	}
	switch msg.To {
	case "editor":
		if msg.From != "hint" { // back from the hint the same YAML session goes on
			be = be.saveUndo()
		}
		be.active = blockEditPanelYAML
		be.yamlEditor.Focus()
	case "fields":
		be.active = blockEditPanelTree
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
// View draws the editor screen through its shell. The shell copy takes the
// frame's subtitle, legend, status and focus; nothing is written back.
func (be blockEditState) View(parentSegs []string) string {
	sh := be.sh.SetSubtitle(be.breadcrumb(parentSegs)).SetLayout(be.currentLayout()).SetActions(be.actions()...)
	if be.mode == modePresetBrowser {
		return be.presetView(sh)
	}
	if text, isErr := be.feedbackLine(); text != "" {
		level := shell.Info
		if isErr {
			level = shell.Error
		}
		sh, _ = sh.SetStatus(text, level, 0)
	}

	fieldsTitle := "Fields"
	if be.tree.IsEmpty() {
		fieldsTitle = "Field"
	}
	editorTitle, focus := "Preview", "fields"
	switch be.active {
	case blockEditPanelYAML:
		editorTitle, focus = "Editing YAML", "editor"
	case blockEditPanelHint:
		focus = "hint"
	}

	panes := map[string]shell.Pane{
		"fields": {Title: fieldsTitle, Body: func(layout.Rect) string {
			if be.tree.IsEmpty() {
				return be.fieldItemView()
			}
			return be.tree.View(be.theme)
		}},
		"editor": {Title: editorTitle, Body: func(r layout.Rect) string {
			if be.active == blockEditPanelYAML {
				return be.yamlEditor.View()
			}
			// The preview follows the tree selection; rendered preview lines
			// map ~1:1 to YAML lines.
			preview := draw.NumberLines(render.PreviewYAML(be.yamlEditor.Value(), be.previewRenderer), be.theme.Muted)
			return draw.ScrollTo(preview, r.H, be.previewScroll)
		}},
	}
	if be.hintVisible() {
		panes["hint"] = shell.Pane{Title: "Hint/Example", Body: func(layout.Rect) string { return be.scrolledHintContent() }}
	}
	return sh.SetFocus(focus).View(panes)
}

func (be blockEditState) presetView(sh shell.Shell) string {
	focus := "presets"
	if be.preset.PreviewFocus {
		focus = "preview"
	}
	panes := map[string]shell.Pane{
		"presets": {Title: "Available Presets", Body: func(r layout.Rect) string { return be.preset.ListView(be.theme, r.H) }},
		"preview": {Title: "Preset Preview", Body: func(r layout.Rect) string { return be.preset.PreviewView(r.H) }},
	}
	return sh.SetFocus(focus).View(panes)
}

// validateSnippetText checks that text is valid YAML.
func validateSnippetText(text string) error {
	var check any
	return yaml.Unmarshal([]byte(text), &check)
}
