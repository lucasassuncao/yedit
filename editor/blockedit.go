package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/components/alert"
	"github.com/lucasassuncao/yedit/components/picker"
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

// blockEditPanel identifies which panel has focus in block-edit mode.
type blockEditPanel int

const (
	blockEditPanelTree blockEditPanel = iota
	blockEditPanelYAML
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
	presetPicker  *picker.Model
	currentPreset string

	confirmAlert *alert.Model // non-nil when showing "Discard changes?" prompt
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
	// Handle confirm-alert (Discard changes?) first.
	if be.confirmAlert != nil {
		if _, ok := msg.(alert.DismissedMsg); ok {
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

	// Preset picker.
	if be.presetPicker != nil {
		switch m := msg.(type) {
		case picker.SelectedMsg:
			return be.applyPreset(m.Name), nil
		case picker.CancelledMsg:
			be.presetPicker = nil
			return be, nil
		}
		if key, ok := msg.(tea.KeyMsg); ok {
			updated, cmd := be.presetPicker.Update(key)
			be.presetPicker = &updated
			return be, cmd
		}
		return be, nil
	}

	switch m := msg.(type) {
	case alert.DismissedMsg:
		be.confirmAlert = nil
		return be, nil
	case tea.WindowSizeMsg:
		be.width = m.Width
		be.height = m.Height
		be.relayout()
		be.yamlEditor.SetWidth(be.rightW - 2)
		be.yamlEditor.SetHeight(be.innerH() - 1)
		be.tree.height = be.innerH()
		return be, nil
	}

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
				func() tea.Msg { return overlayCancelledMsg{} },
				theme.Size{W: be.width, H: be.height},
			)
			be.confirmAlert = &al
			return be, nil
		}
		return be, func() tea.Msg { return overlayCancelledMsg{} }

	case tea.KeyCtrlS:
		return be.commit()

	case tea.KeyTab:
		return be.switchPanel(), nil
	}

	if be.active == blockEditPanelTree {
		if msg.String() == "p" && be.cfg.Presets != nil {
			names := be.cfg.Presets.ListPresets(be.key)
			if len(names) > 0 {
				p := picker.New("Preset", names, be.currentPreset, theme.Size{W: be.width, H: be.height})
				be.presetPicker = &p
			}
			return be, nil
		}
		return be.updateTreePanel(msg)
	}

	// YAML panel active.
	var cmd tea.Cmd
	be.yamlEditor, cmd = be.yamlEditor.Update(msg)
	be.dirty = true
	// Sync tree checked states from YAML.
	if be.kind == schema.KindSlice && len(be.childDefs) > 0 {
		// Structured sequence: update seqBase with the edited item content.
		be.seqBase = be.rebuildSeqBase()
		be.tree = syncTreeCheckedFromYAML(be.tree, "", "x:\n"+be.currentItemContent())
	} else {
		be.tree = syncTreeCheckedFromYAML(be.tree, be.key, be.yamlEditor.Value())
	}
	return be, cmd
}

func (be blockEditState) updateTreePanel(msg tea.KeyMsg) (blockEditState, tea.Cmd) {
	prevSeqIdx := be.tree.NearestSeqItem()

	tree, action := be.tree.Update(msg)
	be.tree = tree

	switch action {
	case treeToggled:
		be.dirty = true
		idx := be.tree.currentNodeIdx()
		if idx >= 0 {
			node := be.tree.nodes[idx]
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

func (be blockEditState) applyPreset(name string) blockEditState {
	if be.cfg.Presets == nil {
		return be
	}
	y, err := be.cfg.Presets.PresetYAML(be.key, name)
	if err != nil {
		be.errMsg = fmt.Sprintf("preset error: %v", err)
		be.presetPicker = nil
		return be
	}
	be.yamlEditor.SetValue(y)
	be.currentPreset = name
	be.errMsg = ""
	be.tree = syncTreeCheckedFromYAML(be.tree, be.key, y)
	be.presetPicker = nil
	be.dirty = true
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

	return be, func() tea.Msg { return overlayConfirmedMsg{Snippet: snippet} }
}

func (be blockEditState) View() string {
	if be.confirmAlert != nil {
		return be.confirmAlert.View()
	}
	if be.presetPicker != nil {
		return be.presetPicker.View()
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
	rightPanel := theme.RenderTitledPanel("Preview", theme.Size{W: be.rightW, H: be.innerH() + 2}, yamlActive, be.yamlEditor.View())

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
		return "[Tab] back to tree • [ctrl+s] commit • [Esc] back"
	}
	const nav = "[↑/↓] navigate • [→/←] expand/collapse"
	const tail = "[Tab] edit YAML • [ctrl+s] commit • [Esc] back"
	if be.kind == schema.KindSlice && len(be.childDefs) > 0 {
		return nav + " • " + be.seqSpaceAction() + " • " + tail
	}
	return nav + " • [Space] toggle • [p] preset • " + tail
}

func (be blockEditState) seqSpaceAction() string {
	idx := be.tree.currentNodeIdx()
	if idx >= 0 && be.tree.nodes[idx].kind == treeNodeAddNew {
		return "[Space] add item"
	}
	return "[Space] toggle field"
}

// validateSnippetText checks that text is valid YAML.
func validateSnippetText(text string) error {
	var check any
	return yamlUnmarshal([]byte(text), &check)
}
