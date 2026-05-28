package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
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
	seqBase     string                     // for KindSlice: accumulates full sequence YAML
	knownByPath map[string]map[string]bool // for schema validation at commit

	yamlEditor textarea.Model
	active     blockEditPanel

	isEdit bool // false = add new block, true = edit existing
	dirty  bool // uncommitted changes since last ctrl+s

	width, height int
	listW, rightW int

	errMsg        string
	currentPreset string

	mode         blockEditMode
	presetCursor int
	presetNames  []string
	confirmAlert *alert.Model // alert data when mode == modeConfirming
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
	}
	be.relayout()

	be.tree = newTreeModel(spec, be.innerH())

	// For KindSlice with child defs, seqBase holds the full sequence YAML so we
	// can extract individual item previews and rebuild the snippet on commit.
	// Scalar sequences (no defs) are edited directly as raw YAML.
	structuredSeq := spec.kind == schema.KindSlice && len(spec.defs) > 0
	if structuredSeq {
		if spec.content == "" {
			be.seqBase = spec.key + ":\n"
		} else {
			be.seqBase = spec.content
		}
	}

	// If presets are available and this is a new block, try the "base" preset.
	content := spec.content
	trivial := spec.key + ":\n"
	if (content == "" || content == trivial) && !structuredSeq {
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
	if newBlock && !structuredSeq {
		be = be.withPreCheckedFields()
	}

	// For structured sequences: show the first item (or empty placeholder).
	if structuredSeq {
		firstItem := yamlForSeqItem(spec.key, be.seqBase, 0)
		if firstItem == spec.key+":\n" {
			firstItem = spec.key + ":\n"
		}
		be.yamlEditor.SetValue(firstItem)
	}

	// If no child fields exist, focus the YAML editor immediately.
	if len(spec.defs) == 0 || spec.kind == schema.KindScalar || spec.kind == schema.KindMap {
		be.active = blockEditPanelYAML
		be.yamlEditor.Focus()
	}

	return be
}

func (be blockEditState) newYAMLEditor(content string) textarea.Model {
	ta := textarea.New()
	ta.SetWidth(be.rightW - 2)
	ta.SetHeight(be.innerH() - 1)
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
}

func (be blockEditState) innerH() int {
	h := be.height - headerLines - statusBarLines - 2
	if h < 1 {
		h = 1
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

	if m, ok := msg.(tea.WindowSizeMsg); ok {
		be.width = m.Width
		be.height = m.Height
		be.relayout()
		be.yamlEditor.SetWidth(be.rightW - 2)
		be.yamlEditor.SetHeight(be.innerH() - 1)
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
		be.mode = modeEditing
	case "enter":
		if be.presetCursor >= 0 && be.presetCursor < len(be.presetNames) {
			be = be.applyPreset(be.presetNames[be.presetCursor])
		}
		be.mode = modeEditing
	case "up", "k":
		if be.presetCursor > 0 {
			be.presetCursor--
		}
	case "down", "j":
		if be.presetCursor < len(be.presetNames)-1 {
			be.presetCursor++
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
	if be.kind == schema.KindSlice && len(be.childDefs) > 0 {
		// Structured sequence: update seqBase with the edited item content.
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
	if be.kind == schema.KindSlice && len(be.childDefs) > 0 {
		return syncTreeCheckedFromYAML(be.tree, "", "x:\n"+be.currentItemContent())
	}
	return syncTreeCheckedFromYAML(be.tree, be.key, be.yamlEditor.Value())
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
	yaml := be.yamlEditor.Value()
	changed := false
	for _, fieldName := range fields {
		if n, ok := nodeByLabel[fieldName]; ok && !n.checked {
			yaml = applyTreeToggle(ctx, n, true, yaml)
			changed = true
		}
	}
	if !changed {
		return be
	}
	be.yamlEditor.SetValue(yaml)
	be.tree = syncTreeCheckedFromYAML(be.tree, be.key, yaml)
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
	if be.kind == schema.KindSlice {
		newYAML = applyToggleToSeqItem(ctx, node, false, be.yamlEditor.Value())
	} else {
		newYAML = applyTreeToggle(ctx, node, false, be.yamlEditor.Value())
	}
	be.yamlEditor.SetValue(newYAML)
	be.dirty = true
	if be.kind == schema.KindSlice {
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
	case treeToggled:
		idx := be.tree.currentNodeIdx()
		if idx >= 0 {
			node := be.tree.nodes[idx]
			// If toggling OFF a field that has content, ask before destroying it.
			if !node.checked && be.fieldHasContent(node) {
				// Revert the tree toggle while waiting for confirmation.
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
			if be.kind == schema.KindSlice {
				newYAML = applyToggleToSeqItem(ctx, node, node.checked, be.yamlEditor.Value())
			} else {
				newYAML = applyTreeToggle(ctx, node, node.checked, be.yamlEditor.Value())
			}
			be.yamlEditor.SetValue(newYAML)
			if be.kind == schema.KindSlice {
				be.seqBase = be.rebuildSeqBase()
			}
			be.tree = be.resyncTreeFromYAML()
		}

	case treeAddNew:
		be.dirty = true
		be.tree = be.tree.WithNewSeqItem(be.childDefs, "")
		be.seqBase = be.rebuildSeqBase()
		// Update YAML editor to show the new (empty) item.
		newSeqIdx := be.tree.NearestSeqItem()
		be.yamlEditor.SetValue(yamlForSeqItem(be.key, be.seqBase, newSeqIdx))

	case treeDeleted:
		be.dirty = true
		be.seqBase = be.rebuildSeqBase()
		newSeqIdx := be.tree.NearestSeqItem()
		if newSeqIdx >= 0 {
			be.yamlEditor.SetValue(yamlForSeqItem(be.key, be.seqBase, newSeqIdx))
		} else {
			be.yamlEditor.SetValue(be.key + ":\n")
		}
	}

	// Update preview when cursor moved to a different seq item.
	if action == treeNoAction || action == treeExpanded || action == treeCollapsed {
		newSeqIdx := be.tree.NearestSeqItem()
		if be.kind == schema.KindSlice && newSeqIdx != prevSeqIdx {
			if newSeqIdx >= 0 {
				be.yamlEditor.SetValue(yamlForSeqItem(be.key, be.seqBase, newSeqIdx))
			} else {
				be.yamlEditor.SetValue(be.key + ":\n")
			}
		}
	}

	return be, nil
}

// rebuildSeqBase reconstructs the full sequence YAML from tree node data.
// For seq items that exist in seqBase, preserves their original content;
// for new items, uses the current YAML editor value.
func (be blockEditState) rebuildSeqBase() string {
	entries := parseSeqEntries(be.key, be.seqBase)

	// Update the entry corresponding to the currently displayed item.
	currentIdx := be.tree.NearestSeqItem()
	if currentIdx >= 0 && currentIdx < len(entries) {
		content := itemContentFrom(be.key, be.yamlEditor.Value())
		if content != "" {
			label := labelFromContent(content)
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
			Content: be.initialSeqItemContent(label),
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
	for i, n := range names {
		if n == be.currentPreset {
			be.presetCursor = i
			break
		}
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
	be.currentPreset = name
	be.errMsg = ""
	be.dirty = true

	if be.kind == schema.KindSlice && len(be.childDefs) > 0 {
		// Structured sequence: rebuild seqBase and tree from the preset, then
		// show the first item in the YAML editor.
		be.seqBase = y
		entries := parseSeqEntries(be.key, y)
		be.tree.nodes = buildSeqNodes(be.childDefs, entries)
		be.tree.cursor = 0
		be.tree.offset = 0
		if len(entries) > 0 {
			be.yamlEditor.SetValue(yamlForSeqItem(be.key, y, 0))
		} else {
			be.yamlEditor.SetValue(be.key + ":\n")
		}
		return be
	}

	be.yamlEditor.SetValue(y)
	be.tree = syncTreeCheckedFromYAML(be.tree, be.key, y)
	return be
}

func (be blockEditState) commit() (blockEditState, tea.Cmd) {
	be.errMsg = ""

	var snippet string
	if be.kind == schema.KindSlice && len(be.childDefs) > 0 {
		// Structured sequence: save current item back into seqBase before assembling.
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
	header := theme.RenderHeader(be.cfg.Title, breadcrumb, "", be.width)

	treeActive := be.active == blockEditPanelTree
	leftPanel := theme.RenderTitledPanel("Fields", theme.Size{W: be.listW, H: be.innerH() + 2}, treeActive, be.tree.View())

	yamlActive := be.active == blockEditPanelYAML
	yamlTitle := "Preview"
	if yamlActive {
		yamlTitle = "Editing YAML"
	}
	rightPanel := theme.RenderTitledPanel(yamlTitle, theme.Size{W: be.rightW, H: be.innerH() + 2}, yamlActive, be.yamlEditor.View())

	hintText := be.currentHint()

	var feedback string
	if be.errMsg != "" {
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(lipgloss.NewStyle().Foreground(theme.Danger).Render(be.errMsg))
	} else if be.dirty {
		feedback = lipgloss.NewStyle().Width(be.width).
			Render(statusStyle.Render("Uncommitted changes — ctrl+s to commit"))
	}
	hint := lipgloss.NewStyle().Width(be.width).Render(statusStyle.Render(hintText))

	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Hint: hint})
}

// currentHint returns the hint bar text for the current panel and cursor state.
func (be blockEditState) currentHint() string {
	if be.active != blockEditPanelTree {
		return "[Tab] change pane • [ctrl+s] save changes • [Esc] back"
	}
	const nav = "[↑/↓] nav • [→/←] expand"
	const tail = "[Tab] change pane • [ctrl+s] save changes • [Esc] back"
	if be.kind == schema.KindSlice && len(be.childDefs) > 0 {
		return nav + " • [Enter] add • [ctrl+d] delete • " + tail
	}
	presetHint := ""
	if be.cfg.Presets != nil {
		presetHint = " • [p] preset"
	}
	return nav + presetHint + " • [Enter] add • [ctrl+d] remove • " + tail
}

func (be blockEditState) presetView() string {
	segs := be.tree.BreadcrumbSegments()
	breadcrumb := be.key
	if len(segs) > 0 {
		breadcrumb = be.key + " › " + strings.Join(segs, " › ")
	}
	header := theme.RenderHeader(be.cfg.Title, breadcrumb, "", be.width)

	leftPanel := theme.RenderTitledPanel("Available Presets", theme.Size{W: be.listW, H: be.innerH() + 2}, true, be.renderPresetList())
	rightPanel := theme.RenderTitledPanel("Preset Preview", theme.Size{W: be.rightW, H: be.innerH() + 2}, false, be.presetPreviewYAML())

	hint := lipgloss.NewStyle().Width(be.width).Render(statusStyle.Render("[↑/↓] navigate • [Enter] apply • [Esc] cancel"))

	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Hint: hint})
}

func (be blockEditState) renderPresetList() string {
	var sb strings.Builder
	for i, name := range be.presetNames {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i == be.presetCursor {
			sb.WriteString(selectedItemStyle.Render("▶  " + name))
		} else {
			sb.WriteString(availableItemStyle.Render("   " + name))
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
