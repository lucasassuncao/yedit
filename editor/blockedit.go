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

// blockEditState is the full-screen block editing mode that replaces the old
// floating overlayModel. It reuses the same two-panel layout as the root view.
type blockEditState struct {
	cfg Config
	key string // top-level YAML key being edited

	tree        treeModel
	childDefs   []schema.FieldDef          // schema children for this block
	kind        schema.Kind                // block kind (Struct, Slice, …)
	seqBase     string                     // for KindList: accumulates full sequence YAML
	knownByPath map[string]map[string]bool // for schema validation at commit

	yamlEditor      textarea.Model
	previewRenderer *glamour.TermRenderer // non-nil only for KindObject blocks
	active          blockEditPanel

	isEdit bool // false = add new block, true = edit existing
	dirty  bool // uncommitted changes since last ctrl+s

	// childPath is set when this editor was opened by drilling into a nested
	// map-of-struct field of a parent editor. On commit the snippet is spliced
	// back into the parent at this path (relative to the parent block's value).
	childPath []string

	width, height int
	listW, rightW int

	errMsg        string
	currentPreset string

	mode         blockEditMode
	presetCursor int
	presetNames  []string
	confirmAlert *alert.Model // alert data when mode == modeConfirming

	previewFocus  bool // preset browser: right panel has keyboard focus
	previewScroll int  // preset browser: line scroll offset in preview panel

	undoSnap *blockEditUndoSnap // one-level undo for preset apply/append
	theme    resolvedTheme
}

// blockEditUndoSnap captures the state of a blockEditState before a preset
// operation so it can be restored by a single ctrl+u.
type blockEditUndoSnap struct {
	seqBase   string
	yamlValue string
	dirty     bool
	preset    string
}

// newBlockEdit creates the full-screen block editing state.
func newBlockEdit(cfg Config, spec blockSpec, w, h int) blockEditState {
	be := blockEditState{
		cfg:           cfg,
		key:           spec.key,
		childDefs:     spec.defs,
		kind:          spec.kind,
		knownByPath:   spec.knownByPath,
		currentPreset: "custom",
		width:         w,
		height:        h,
		theme:         resolveTheme(cfg.Theme),
	}
	be.relayout()

	be.tree = newTreeModel(spec, be.innerH())

	// For KindList with child defs, seqBase holds the full sequence YAML so we
	// can extract individual item previews and rebuild the snippet on commit.
	// Scalar sequences (no defs) are edited directly as raw YAML.
	// Structured collections (KindList or KindDictionary with child defs) hold the full
	// block YAML in seqBase so we can extract per-entry previews and rebuild on
	// commit. Scalar/free-form blocks are edited directly as raw YAML.
	structured := (spec.kind == schema.KindList || spec.kind == schema.KindDictionary) && len(spec.defs) > 0
	if structured {
		if spec.content == "" {
			be.seqBase = spec.key + ":\n"
		} else {
			be.seqBase = spec.content
		}
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

	// For new struct blocks, pre-check fields listed in cfg.PreCheckedFields.
	newBlock := spec.content == "" || spec.content == spec.key+":\n"
	if newBlock && !structured {
		be = be.withPreCheckedFields()
	}

	// For structured collections: show the first entry (or empty placeholder).
	if structured {
		be.yamlEditor.SetValue(be.entryYAML(0))
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
	return be.innerH() - 2 - be.hintH()
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
		if be.dirty {
			al := alert.NewConfirm(
				"Discard changes?",
				"Uncommitted changes will be lost.",
				func() tea.Msg { return blockEditDiscardedMsg{} },
				theme.Size{W: be.width, H: be.height},
			)
			be.confirmAlert = &al
			be.mode = modeConfirming
			return be, nil
		}
		return be, func() tea.Msg { return blockEditDiscardedMsg{} }

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

	// YAML panel active.
	var cmd tea.Cmd
	be.yamlEditor, cmd = be.yamlEditor.Update(msg)
	be.dirty = true
	if be.isCollectionNav() {
		// Structured collection: update seqBase with the edited entry content.
		be.seqBase = be.rebuildSeqBase()
	}
	be.tree = be.resyncTreeFromYAML()
	return be, cmd
}

// resyncTreeFromYAML re-derives the tree's checked states (and re-applies the
// ADDED/AVAILABLE sectioning for struct blocks) from the current yamlEditor.
// For structured sequences the editor shows a single item, so the lookup uses
// the synthetic "x:\n<item>" form expected by syncTreeCheckedStates.
func (be blockEditState) resyncTreeFromYAML() treeModel {
	if be.isCollectionNav() {
		return be.syncCurrentEntry()
	}
	return syncTreeCheckedFromYAML(be.tree, be.key, be.yamlEditor.Value())
}

// syncCurrentEntry updates the displayed entry's label and the checked state of
// its own field nodes from its YAML content, leaving sibling entries untouched —
// so editing one entry never overwrites another's label or checkmarks.
func (be blockEditState) syncCurrentEntry() treeModel {
	tm := be.tree
	seqIdx := tm.NearestSeqItem()
	if seqIdx < 0 {
		return tm
	}
	content := be.currentItemContent()
	newLabel := be.entryLabel(content)
	fields := entryFieldValues(be.isMapNav(), content)

	nodes := make([]treeNode, len(tm.nodes))
	copy(nodes, tm.nodes)
	for i := 0; i < len(nodes); i++ {
		if nodes[i].kind != treeNodeSeqItem || nodes[i].seqIdx != seqIdx {
			continue
		}
		if newLabel != "" {
			nodes[i].label = newLabel
			nodes[i].yamlPath = []string{newLabel}
		}
		for j := i + 1; j < len(nodes) && nodes[j].depth > 0; j++ {
			if nodes[j].kind != treeNodeField {
				continue
			}
			if newLabel != "" && len(nodes[j].yamlPath) > 0 {
				p := append([]string(nil), nodes[j].yamlPath...)
				p[0] = newLabel
				nodes[j].yamlPath = p
			}
			if !nodes[j].isLeaf {
				continue
			}
			cur := fields
			path := nodes[j].yamlPath
			for k := 1; k < len(path)-1 && cur != nil; k++ {
				cur, _ = cur[path[k]].(map[string]any)
			}
			if cur != nil && len(path) > 1 {
				_, nodes[j].checked = cur[path[len(path)-1]]
			}
		}
		break
	}
	tm.nodes = nodes
	return tm
}

// entryFieldValues returns the field map of a single collection entry's content
// ("  - name: x" for sequences, "  key:\n    field: x" for maps).
func entryFieldValues(isMap bool, content string) map[string]any {
	if content == "" {
		return nil
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte("x:\n"+content), &doc); err != nil {
		return nil
	}
	if isMap {
		outer, _ := doc["x"].(map[string]any)
		for _, v := range outer { // single entry: take its value
			m, _ := v.(map[string]any)
			return m
		}
		return nil
	}
	items, _ := doc["x"].([]any)
	if len(items) > 0 {
		m, _ := items[0].(map[string]any)
		return m
	}
	return nil
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
	content := be.yamlEditor.Value()
	changed := false
	for _, fieldName := range fields {
		if n, ok := nodeByLabel[fieldName]; ok && !n.checked {
			content = applyTreeToggle(ctx, n, true, content)
			changed = true
		}
	}
	if !changed {
		return be
	}
	be.yamlEditor.SetValue(content)
	be.tree = syncTreeCheckedFromYAML(be.tree, be.key, content)
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
	var newYAML string
	if be.isCollectionNav() {
		newYAML = be.applyEntryToggle(ctx, node, false, be.yamlEditor.Value())
	} else {
		newYAML = applyTreeToggle(ctx, node, false, be.yamlEditor.Value())
	}
	be.yamlEditor.SetValue(newYAML)
	be.dirty = true
	if be.isCollectionNav() {
		be.seqBase = be.rebuildSeqBase()
	}
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
	// requires reloading its content into the YAML editor.
	if be.isCollectionNav() && (action == treeNoAction || action == treeExpanded || action == treeCollapsed) {
		newSeqIdx := be.tree.NearestSeqItem()
		if newSeqIdx != prevSeqIdx {
			if newSeqIdx >= 0 {
				be.yamlEditor.SetValue(be.entryYAML(newSeqIdx))
			} else {
				be.yamlEditor.SetValue(be.key + ":\n")
			}
		}
	}

	return be, nil
}

// handleTreeOpenChild drills into the openable map-of-struct field under the
// cursor by emitting an openChildMsg. Collection navigators are excluded because
// their entry paths are position-keyed, not field-path-keyed.
func (be blockEditState) handleTreeOpenChild() (blockEditState, tea.Cmd) {
	if be.isCollectionNav() {
		return be, nil
	}
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be, nil
	}
	node := be.tree.nodes[idx]
	childContent := extractSubBlock(be.yamlEditor.Value(), node.yamlPath)
	childPath := append([]string(nil), node.yamlPath...)
	return be, func() tea.Msg {
		return openChildMsg{
			key:     node.def.YAMLName,
			defs:    node.def.Children,
			kind:    node.def.Kind,
			content: childContent,
			path:    childPath,
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
	var newYAML string
	if be.isCollectionNav() {
		newYAML = be.applyEntryToggle(ctx, node, node.checked, be.yamlEditor.Value())
	} else {
		newYAML = applyTreeToggle(ctx, node, node.checked, be.yamlEditor.Value())
	}
	be.yamlEditor.SetValue(newYAML)
	if be.isCollectionNav() {
		be.seqBase = be.rebuildSeqBase()
	}
	be.tree = be.resyncTreeFromYAML()
	return be, nil
}

// handleTreeAddNew appends a fresh entry to the collection and moves the cursor
// to it so the user can start filling in its fields immediately.
func (be blockEditState) handleTreeAddNew() blockEditState {
	be = be.saveUndo()
	be.dirty = true
	be.tree = be.tree.WithNewSeqItem(be.childDefs, be.newEntryLabel())
	be.seqBase = be.rebuildSeqBase()
	newSeqIdx := be.tree.NearestSeqItem()
	be.yamlEditor.SetValue(be.entryYAML(newSeqIdx))
	be.tree = be.resyncTreeFromYAML()
	return be
}

// handleTreeDeleted rebuilds seqBase after the tree has already removed the
// entry node, then shows the entry now under the cursor (or a blank placeholder
// when the collection becomes empty).
func (be blockEditState) handleTreeDeleted() blockEditState {
	be = be.saveUndo()
	be.dirty = true
	be.seqBase = be.rebuildSeqBase()
	newSeqIdx := be.tree.NearestSeqItem()
	if newSeqIdx >= 0 {
		be.yamlEditor.SetValue(be.entryYAML(newSeqIdx))
	} else {
		be.yamlEditor.SetValue(be.key + ":\n")
	}
	return be
}

// rebuildSeqBase reconstructs the full sequence YAML from tree node data.
// For seq items that exist in seqBase, preserves their original content;
// for new items, uses the current YAML editor value.
func (be blockEditState) rebuildSeqBase() string {
	entries := be.parseEntries()

	// Update the entry corresponding to the currently displayed item.
	currentIdx := be.tree.NearestSeqItem()
	if currentIdx >= 0 && currentIdx < len(entries) {
		content := itemContentFrom(be.key, be.yamlEditor.Value())
		if content != "" {
			label := be.entryLabel(content)
			if label == "" {
				label = entries[currentIdx].Label
			}
			entries[currentIdx] = seqEntry{Label: label, Content: content}
		}
	}

	// Reconcile entries count with tree seq items.
	var seqItems []treeNode
	for _, n := range be.tree.nodes {
		if n.kind == treeNodeSeqItem {
			seqItems = append(seqItems, n)
		}
	}

	// If we have more tree items than entries (new item was added), append empty.
	for len(entries) < len(seqItems) {
		idx := len(entries)
		label := seqItems[idx].label
		entries = append(entries, seqEntry{
			Label:   label,
			Content: be.initialEntryContent(label),
		})
	}
	// If entries were deleted, trim.
	if len(entries) > len(seqItems) {
		entries = entries[:len(seqItems)]
	}

	return seqEntriesToBase(be.key, entries)
}

// currentItemContent returns the indented YAML lines for the currently displayed item.
func (be blockEditState) currentItemContent() string {
	return itemContentFrom(be.key, be.yamlEditor.Value())
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

// parseEntries splits be.seqBase into entries using the kind-appropriate parser.
func (be blockEditState) parseEntries() []seqEntry {
	if be.isMapNav() {
		return parseMapEntries(be.key, be.seqBase)
	}
	return parseSeqEntries(be.key, be.seqBase)
}

// entryYAML is the single-entry editor view ("block:\n  <entry>") for index idx.
func (be blockEditState) entryYAML(idx int) string {
	if !be.isMapNav() {
		return yamlForSeqItem(be.key, be.seqBase, idx)
	}
	entries := parseMapEntries(be.key, be.seqBase)
	if idx < 0 || idx >= len(entries) {
		return be.key + ":\n"
	}
	// Normalize to block style so a flow entry ("key: {a: b}") shown in the YAML
	// pane renders one field per line, like every other block.
	return withYAMLRoot(be.key+":\n"+entries[idx].Content, func(*yaml.Node) bool { return true })
}

// applyEntryToggle adds/removes a field within the current entry view.
func (be blockEditState) applyEntryToggle(ctx toggleCtx, node treeNode, checked bool, content string) string {
	if be.isMapNav() {
		return applyToggleToMapEntry(ctx, node, checked, content)
	}
	return applyToggleToSeqItem(ctx, node, checked, content)
}

// entryLabel derives an entry's label from its content: the map key for maps,
// the "name" field for sequences.
func (be blockEditState) entryLabel(content string) string {
	if be.isMapNav() {
		if i := strings.IndexByte(content, '\n'); i >= 0 {
			return mapEntryKey(content[:i])
		}
		return mapEntryKey(content)
	}
	return labelFromContent(content)
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
	n := 1
	for _, node := range be.tree.nodes {
		if node.kind == treeNodeSeqItem {
			n++
		}
	}
	return fmt.Sprintf("key%d", n)
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
	be.undoSnap = &blockEditUndoSnap{
		seqBase:   be.seqBase,
		yamlValue: be.yamlEditor.Value(),
		dirty:     be.dirty,
		preset:    be.currentPreset,
	}
	return be
}

func (be blockEditState) restoreUndo() blockEditState {
	snap := be.undoSnap
	be.undoSnap = nil
	be.currentPreset = snap.preset
	be.dirty = snap.dirty
	be.errMsg = ""

	if be.isCollectionNav() {
		return be.restoreUndoCollection(snap)
	}
	be.yamlEditor.SetValue(snap.yamlValue)
	be.tree = syncTreeCheckedFromYAML(be.tree, be.key, snap.yamlValue)
	return be
}

func (be blockEditState) restoreUndoCollection(snap *blockEditUndoSnap) blockEditState {
	be.seqBase = snap.seqBase
	entries := be.parseEntries()
	if be.isMapNav() {
		be.tree.nodes = buildMapNodes(be.childDefs, entries)
	} else {
		be.tree.nodes = buildSeqNodes(be.childDefs, entries)
	}
	be.tree.cursor = 0
	be.tree.offset = 0
	if len(entries) > 0 {
		be.yamlEditor.SetValue(be.entryYAML(0))
	} else {
		be.yamlEditor.SetValue(be.key + ":\n")
	}
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
		// Structured collection: rebuild seqBase and tree from the preset, then
		// show the first entry in the YAML editor.
		be.seqBase = y
		entries := be.parseEntries()
		if be.isMapNav() {
			be.tree.nodes = buildMapNodes(be.childDefs, entries)
		} else {
			be.tree.nodes = buildSeqNodes(be.childDefs, entries)
		}
		be.tree.cursor = 0
		be.tree.offset = 0
		if len(entries) > 0 {
			be.yamlEditor.SetValue(be.entryYAML(0))
		} else {
			be.yamlEditor.SetValue(be.key + ":\n")
		}
		return be
	}

	be.yamlEditor.SetValue(y)
	be.tree = syncTreeCheckedFromYAML(be.tree, be.key, y)
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

	var newEntries []seqEntry
	if be.isMapNav() {
		newEntries = parseMapEntries(be.key, y)
	} else {
		newEntries = parseSeqEntries(be.key, y)
		// Normalize indentation of preset entries to match the existing seqBase so
		// the combined seqBase can be re-parsed consistently.
		dst := seqItemIndent(be.seqBase, be.key)
		src := seqItemIndent(y, be.key)
		if src != dst && src > 0 && dst > 0 {
			for i, e := range newEntries {
				newEntries[i].Content = reindentSeqContent(e.Content, src, dst)
			}
		}
	}
	if len(newEntries) == 0 {
		return be
	}

	combined := append(be.parseEntries(), newEntries...)
	be.seqBase = seqEntriesToBase(be.key, combined)

	if be.isMapNav() {
		be.tree.nodes = buildMapNodes(be.childDefs, combined)
	} else {
		be.tree.nodes = buildSeqNodes(be.childDefs, combined)
	}
	be.tree.offset = 0
	// All entries start collapsed, so visible index equals entry index.
	be.tree.cursor = len(combined) - 1

	be.yamlEditor.SetValue(be.entryYAML(len(combined) - 1))
	be.currentPreset = name
	be.errMsg = ""
	be.dirty = true
	return be
}

func (be blockEditState) commit() (blockEditState, tea.Cmd) {
	be.errMsg = ""

	var snippet string
	if be.isCollectionNav() {
		// Structured collection: save current entry back into seqBase before assembling.
		be.seqBase = be.rebuildSeqBase()
		snippet = be.seqBase
	} else {
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

func (be blockEditState) View() string {
	switch be.mode {
	case modeConfirming:
		return be.confirmAlert.View()
	case modePresetBrowser:
		return be.presetView()
	}

	// Build breadcrumb.
	segs := be.tree.BreadcrumbSegments()
	breadcrumb := be.key
	if len(segs) > 0 {
		breadcrumb = be.key + " › " + strings.Join(segs, " › ")
	}
	header := theme.RenderHeaderWith(be.cfg.Title, breadcrumb, "", be.width, be.theme.colors)

	treeActive := be.active == blockEditPanelTree
	leftPanel := theme.RenderTitledPanelWith("Fields", theme.Size{W: be.listW, H: be.innerH() + 2}, treeActive, be.tree.View(be.theme), be.theme.colors)

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
	if be.errMsg != "" {
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.errorText.Render(be.errMsg))
	} else if be.dirty {
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(be.theme.status.Render(msgUncommittedChanges))
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

func (be blockEditState) presetView() string {
	segs := be.tree.BreadcrumbSegments()
	breadcrumb := be.key
	if len(segs) > 0 {
		breadcrumb = be.key + " › " + strings.Join(segs, " › ")
	}
	header := theme.RenderHeaderWith(be.cfg.Title, breadcrumb, "", be.width, be.theme.colors)

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

// hintContent returns the rendered string for the bottom-right hint panel.
func (be blockEditState) hintContent() string {
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be.theme.hintDim.Render("  select a field to see hints")
	}
	node := be.tree.nodes[idx]
	if node.kind != treeNodeField {
		return be.theme.hintDim.Render("  select a field to see hints")
	}
	return be.fieldHint(node)
}

// fieldHint builds the hint text for a single treeNodeField.
func (be blockEditState) fieldHint(node treeNode) string {
	def := node.def
	var sb strings.Builder

	sb.WriteString(be.theme.hintKey.Render("type") + "  " + kindHumanLabel(def.Kind) + "\n")

	if def.Required {
		sb.WriteString(be.theme.hintKey.Render("required") + "\n")
	}
	if def.Default != "" {
		sb.WriteString(be.theme.hintKey.Render("default") + "  " + def.Default + "\n")
	}
	if len(def.OneOf) > 0 {
		sb.WriteString("\n" + be.theme.hintKey.Render("values") + "\n")
		for _, v := range def.OneOf {
			sb.WriteString("  • " + v + "\n")
		}
	}
	if def.Description != "" {
		sb.WriteString("\n" + def.Description + "\n")
	}

	example := be.fieldExample(def)
	if example != "" {
		sb.WriteString("\n" + be.theme.hintKey.Render("Example") + "\n")
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

// extractFromBasePreset parses the "base" preset for be.key and returns the
// YAML representation of the top-level child fieldName, or "" if unavailable.
func (be blockEditState) extractFromBasePreset(fieldName string) string {
	if be.cfg.Presets == nil {
		return ""
	}
	presetYAML, err := be.cfg.Presets.PresetYAML(be.key, "base")
	if err != nil {
		return ""
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(presetYAML), &doc); err != nil {
		return ""
	}
	root, _ := doc[be.key].(map[string]any)
	val, ok := root[fieldName]
	if !ok {
		return ""
	}
	out, err := yaml.Marshal(map[string]any{fieldName: val})
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}
