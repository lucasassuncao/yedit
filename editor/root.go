package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/alert"
	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

type pane int

const (
	paneList pane = iota
	panePreview
	paneBlockEdit
	paneAlert
	paneDocPreset
)

// model is the root bubbletea model.
//
// The active pane is explicit via the mode field. The alert/blockEdits fields
// hold per-mode data: alert is non-nil iff mode == paneAlert, blockEdits is
// non-empty iff mode == paneBlockEdit (its last element is the visible editor).
type model struct {
	cfg             Config
	doc             document.Document
	schemaTree      []schema.FieldDef
	knownByPath     map[string]map[string]bool
	childrenOf      map[string][]schema.FieldDef
	wiredValidators WiredValidators // produced once in newModel; reused on every save/validate

	list            listModel
	preview         viewport.Model
	previewRenderer *glamour.TermRenderer
	// blockEdits is a stack of block editors. Index 0 is the top-level block
	// opened from the list; deeper entries are nested drill-ins. The last element
	// is the visible/active editor. The stack carries only UI state (cursor,
	// expansion) and each editor's focus path - the canonical data lives in
	// editRoot.
	blockEdits []blockEditState
	// editRoot is the single canonical *yaml.Node for the block currently being
	// edited: the parsed value of the top-level block. Drilling in moves a focus
	// path within this one tree (be.focus) rather than copying substrings between
	// stacked editors, and committing serializes it once. Non-focused parts stay
	// as live nodes, so nested edits can never corrupt them via string splicing.
	editRoot     *yaml.Node
	editBlockKey string // top-level YAML key of editRoot
	alert        alert.Model
	alertVisible bool
	docPreset    presetBrowser
	theme        resolvedTheme
	help         help.Model

	mode                         pane
	showHint                     bool // root view: split the right column to show the Hint/Example panel
	saved                        bool // at least one save succeeded this session; reported via Result
	statusMsg                    string
	statusSeq                    uint // incremented with each new status; used to cancel stale clear ticks
	width, height, listW, innerH int
}

// newModel constructs the root model from a Config. The path may be a file
// that does not yet exist; in that case the editor starts with an empty doc.
func newModel(cfg Config) (model, error) {
	if cfg.Schema == nil {
		return model{}, fmt.Errorf("editor: Config.Schema is required")
	}

	var tree []schema.FieldDef
	if cfg.SchemaRecursionDepth > 0 {
		tree = schema.Discover(cfg.Schema, cfg.SchemaRecursionDepth)
	} else {
		tree = schema.Discover(cfg.Schema) // use schema default (1 extra recursive level)
	}
	tree = applyHidden(tree, cfg.Hidden)
	known := schema.KnownChildren(tree)
	childrenOf := buildChildrenMap(tree)
	knownOrder := schema.TopLevelOrder(tree)

	doc, err := document.Load(cfg.Path, knownOrder)
	if err != nil {
		return model{}, fmt.Errorf("loading %s: %w", cfg.Path, err)
	}
	if cfg.SavePath != "" {
		doc = doc.SetPath(cfg.SavePath)
	}

	passthrough := make(map[string]bool, len(cfg.PassthroughKeys))
	for _, k := range cfg.PassthroughKeys {
		passthrough[k] = true
	}

	list := newListModel(knownOrder, doc.Blocks(), passthrough, 0)

	preview := viewport.New(0, 0)
	preview.SetContent(renderPreviewYAML(string(doc.Raw()), nil))

	return model{
		cfg:             cfg,
		doc:             doc,
		schemaTree:      tree,
		knownByPath:     known,
		childrenOf:      childrenOf,
		wiredValidators: Wire(cfg.Validators, cfg),

		list:     list,
		preview:  preview,
		showHint: cfg.EnableHints,
		theme:    resolveTheme(cfg.Theme),
		help:     newHelpModel(resolveTheme(cfg.Theme)),
	}, nil
}

// model-level alert (validation, save confirm, etc.) can be shown over the list
// OR over an active block editor. enterAlert preserves blockEdits so dismissal
// can return to the block editor via enterBlockEdit instead of discarding the
// stack via enterList. The block editor's own confirmAlert uses mode=paneBlockEdit
// and is handled separately in handleDismissedAlert.
func (m model) enterList() model {
	m.mode = paneList
	m.alertVisible = false
	m.blockEdits = nil
	m.editRoot = nil
	m.editBlockKey = ""
	return m
}

// enterPreview focuses the read-only preview pane.
func (m model) enterPreview() model {
	m.mode = panePreview
	m.alertVisible = false
	return m
}

// enterBlockEdit makes the block-editor stack the active screen. The caller is
// responsible for having pushed onto m.blockEdits first.
func (m model) enterBlockEdit() model {
	m.mode = paneBlockEdit
	m.alertVisible = false
	return m
}

// enterAlert shows a modal alert over the current (list) screen.
func (m model) enterAlert(al alert.Model) model {
	m.mode = paneAlert
	m.alert = al
	m.alertVisible = true
	return m
}

// enterDocPreset switches to the document-level template picker.
func (m model) enterDocPreset(pb presetBrowser) model {
	m.mode = paneDocPreset
	m.docPreset = pb
	m.alertVisible = false
	return m
}

func (m model) viewDocPreset() string {
	header := renderHeader(m.cfg.Title, m.doc.Path(), m.doc.Dirty(), m.width, m.theme)

	leftPanel := theme.RenderTitledPanelWith("Templates", theme.Size{W: m.listW, H: m.innerH + 2}, !m.docPreset.previewFocus, m.docPreset.listView(m.theme), m.theme.colors)

	_, rightW := theme.TwoColumnWidths(m.width)
	rightPanel := theme.RenderTitledPanelWith("Preview", theme.Size{W: rightW, H: m.innerH + 2}, m.docPreset.previewFocus, m.docPreset.previewView(m.innerH), m.theme.colors)

	feedback := renderStatusLine(m.width, m.theme.status, m.statusMsg)
	var km help.KeyMap
	if m.docPreset.previewFocus {
		km = docPresetPreviewKeyMap{}
	} else {
		km = docPresetListKeyMap{}
	}
	legend := renderHelpLine(m.width, m.help, km)

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Legend: legend})
	if m.height > 0 {
		if lines := strings.Split(out, "\n"); len(lines) > m.height {
			out = strings.Join(lines[:m.height], "\n")
		}
	}
	return out
}

func applyHidden(fields []schema.FieldDef, hidden []string) []schema.FieldDef {
	if len(hidden) == 0 {
		return fields
	}
	topHide := make(map[string]bool, len(hidden))
	nestedHide := make(map[string][]string, len(hidden))
	for _, h := range hidden {
		if i := strings.IndexByte(h, '.'); i >= 0 {
			parent := h[:i]
			nestedHide[parent] = append(nestedHide[parent], h[i+1:])
		} else {
			topHide[h] = true
		}
	}
	out := make([]schema.FieldDef, 0, len(fields))
	for _, f := range fields {
		if topHide[f.YAMLName] {
			continue
		}
		if nested, ok := nestedHide[f.YAMLName]; ok {
			f.Children = applyHidden(f.Children, nested)
		}
		out = append(out, f)
	}
	return out
}

// applyPresentation stamps Presentation on FieldDefs from the MetadataSource so
// that presentation intent travels with the field into collection navigators.
// prefixSegs is the dot-path from the block root to the current defs level (nil at top level).
func applyPresentation(fields []schema.FieldDef, meta MetadataSource, blockKey string, prefixSegs []string) []schema.FieldDef {
	if meta == nil {
		return fields
	}
	out := make([]schema.FieldDef, len(fields))
	for i, f := range fields {
		childSegs := make([]string, len(prefixSegs)+1)
		copy(childSegs, prefixSegs)
		childSegs[len(prefixSegs)] = f.YAMLName
		if p := meta.FieldMeta(blockKey, strings.Join(childSegs, ".")).Presentation; p != schema.PresentationDefault {
			f.Presentation = p
		}
		if len(f.Children) > 0 {
			f.Children = applyPresentation(f.Children, meta, blockKey, childSegs)
		}
		out[i] = f
	}
	return out
}

func buildChildrenMap(fields []schema.FieldDef) map[string][]schema.FieldDef {
	m := make(map[string][]schema.FieldDef, len(fields))
	for _, f := range fields {
		m[f.YAMLName] = f.Children
	}
	return m
}

// fieldKind returns the Kind of the named top-level field, or KindPrimitive if not found.
func fieldKind(fields []schema.FieldDef, name string) schema.Kind {
	for _, f := range fields {
		if f.YAMLName == name {
			return f.Kind
		}
	}
	return schema.KindPrimitive
}

// fieldDefByName returns the FieldDef of the named top-level field, or a zero
// FieldDef when it has no schema entry (e.g. an unknown key).
func fieldDefByName(fields []schema.FieldDef, name string) schema.FieldDef {
	for _, f := range fields {
		if f.YAMLName == name {
			return f
		}
	}
	return schema.FieldDef{}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSizeMsg(msg)
	case openItemMsg:
		return m.handleOpenItem(msg.Item)
	case openChildMsg:
		return m.dispatch(DrillIn{Key: msg.key, Defs: msg.defs, Kind: msg.kind, RelSegs: msg.relSegs})
	case blockEditDiscardedMsg:
		return m.handleBlockEditDiscarded(msg)
	case drillOutMsg:
		return m.dispatch(DrillOut{})
	case commitRequestedMsg:
		return m.saveAll()
	case deleteItemMsg:
		return m.handleDeleteItemMsg(msg)
	case confirmedDeleteMsg:
		m = m.enterList()
		return m.dispatch(DeleteBlock(msg))
	case confirmedReloadMsg:
		m = m.enterList()
		return m.dispatch(Reload{})
	case confirmedDocPresetMsg:
		return m.handleConfirmedDocPreset(msg)
	case validateRequestedMsg:
		return m.validateKeys()
	case alert.DismissedMsg:
		return m.handleDismissedAlert(msg)
	case doSaveMsg:
		return m.dispatch(Save{})
	case saveResultMsg:
		if msg.err != nil {
			return m.showAlert("Save failed", msg.err.Error(), alert.KindError)
		}
		m.doc = msg.doc
		m.saved = true
		// syncView refreshes the list's dirty decorations (e.g. unsaved-changes
		// indicator) immediately so they reflect the now-saved state.
		m = m.syncView()
		return m.showAlert("Saved", fmt.Sprintf("Saved to %s.", m.doc.Path()), alert.KindSuccess)
	case reloadResultMsg:
		if msg.err != nil {
			return m.showAlert("Reload failed", msg.err.Error(), alert.KindError)
		}
		m.doc = msg.doc
		m = m.syncView()
		return m.withStatus(fmt.Sprintf("Reloaded %s from disk.", m.doc.Path()))
	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.statusMsg = ""
		}
		return m, nil
	}

	return m.handleModeUpdate(msg)
}

// handleConfirmedDocPreset replaces the document with the preset content after
// the user confirms the action.
func (m model) handleConfirmedDocPreset(msg confirmedDocPresetMsg) (tea.Model, tea.Cmd) {
	newDoc, err := m.doc.ReplaceRaw([]byte(msg.Content))
	if err != nil {
		return m.withStatus(fmt.Sprintf("Failed to apply preset %q: %v", msg.Name, err))
	}
	m.doc = newDoc
	m = m.syncView()
	m = m.enterList()
	return m.withStatus(fmt.Sprintf("Applied preset %q — ctrl+s to save.", msg.Name))
}

// handleDismissedAlert clears the active alert and returns to the appropriate screen. Routing depends on the current mode,
// not on whether a block editor stack exists, because entering paneAlert preserves blockEdits for return?
//
//   - paneBlockEdit: the block editor's own confirm overlay (save/delete) is active; forward DismissedMsg to the block editor to clear it.
//   - any other mode: a root-level alert (validation, etc.) was shown. If a block editor stack was preserved, restore it, otherwise return to the list.
func (m model) handleDismissedAlert(msg alert.DismissedMsg) (tea.Model, tea.Cmd) {
	if m.mode == paneBlockEdit {
		if top := m.topBE(); top != nil {
			be, cmd := top.Update(msg)
			return m.withTopBE(be), cmd
		}
	}
	if len(m.blockEdits) > 0 {
		m = m.enterBlockEdit()
	} else {
		m = m.enterList()
	}
	return m, nil
}

// handleModeUpdate dispatches msg to the active pane when no root-level
// message handler matched first.
func (m model) handleModeUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case paneAlert:
		if key, ok := msg.(tea.KeyMsg); ok {
			// Global shortcuts (save, validate) work even while an alert is shown.
			if mo, cmd, handled := m.handleGlobalKey(key); handled {
				return mo, cmd
			}
			al, cmd := m.alert.Update(key)
			m.alert = al
			return m, cmd
		}
	case paneBlockEdit:
		return m.handlePaneBlockEdit(msg)
	case panePreview:
		return m.handlePreviewUpdate(msg)
	case paneList:
		if key, ok := msg.(tea.KeyMsg); ok {
			return m.handleListKey(key)
		}
	case paneDocPreset:
		if key, ok := msg.(tea.KeyMsg); ok {
			return m.handleDocPresetKey(key)
		}
	}
	return m, nil
}

// handlePreviewUpdate routes a message to the preview pane, preferring key
// bindings over generic viewport updates.
func (m model) handlePreviewUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		return m.handlePreviewKey(key)
	}
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}

func (m model) handleWindowSizeMsg(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.help.Width = m.width - 1
	m = m.relayout()
	// relayout only sizes the root list/preview; forward the resize to every
	// stacked sub-model so each editor's panels resize too.
	if len(m.blockEdits) > 0 {
		var cmd tea.Cmd
		for i := range m.blockEdits {
			be, c := m.blockEdits[i].Update(msg)
			m.blockEdits[i] = be
			if i == len(m.blockEdits)-1 {
				cmd = c
			}
		}
		return m, cmd
	}
	return m, nil
}

func (m model) handleDeleteItemMsg(msg deleteItemMsg) (tea.Model, tea.Cmd) {
	if m.mode == paneBlockEdit {
		return m, nil // stale Cmd: editor is already open, discard
	}
	if m.cfg.NoDeleteConfirm {
		return m.handleDelete(msg.Key)
	}
	return m.showConfirmAlert(
		"Remove block?",
		fmt.Sprintf("Remove %q? Its content will be lost.", msg.Key),
		func() tea.Msg { return confirmedDeleteMsg(msg) },
	)
}

func (m model) handleDelete(key string) (tea.Model, tea.Cmd) {
	var err error
	m.doc, err = m.doc.Remove(key)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Error removing %s: %v", key, err)
		return m, nil
	}
	m = m.syncView()
	return m.withStatus(fmt.Sprintf("Removed %q (not saved yet).", key))
}

func (m model) showAlert(title, message string, kind alert.Kind) (tea.Model, tea.Cmd) {
	m = m.enterAlert(alert.New(title, message, kind))
	return m, nil
}

func (m model) showConfirmAlert(title, message string, confirmCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m = m.enterAlert(alert.NewConfirm(title, message, confirmCmd))
	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	if m.width < 80 || m.height < 20 {
		return "Terminal too small - resize to at least 80×20."
	}

	if m.mode == paneBlockEdit {
		if top := m.topBE(); top != nil {
			return top.View(m.blockBreadcrumbPrefix())
		}
	}

	if m.mode == paneDocPreset {
		return m.viewDocPreset()
	}

	previewFocused := m.mode == panePreview

	header := renderHeader(m.cfg.Title, m.doc.Path(), m.doc.Dirty(), m.width, m.theme)

	leftTitle := fmt.Sprintf("Blocks (%d/%d)", m.list.AddedCount(), len(m.list.knownKeys))
	leftPanel := theme.RenderTitledPanelWith(leftTitle, theme.Size{W: m.listW, H: m.innerH + 2}, !previewFocused, m.list.View(m.theme), m.theme.colors)

	_, rightW := theme.TwoColumnWidths(m.width)
	var rightPanel string
	if m.showHint {
		previewPanel := theme.RenderTitledPanelWith("Preview", theme.Size{W: rightW, H: m.previewPanelH() + 2}, previewFocused, m.preview.View(), m.theme.colors)
		hintPanel := theme.RenderTitledPanelWith("Hint/Example", theme.Size{W: rightW, H: m.hintPanelH() + 2}, false, clampLines(m.selectedHint(), m.hintPanelH()), m.theme.colors)
		rightPanel = lipgloss.JoinVertical(lipgloss.Left, previewPanel, hintPanel)
	} else {
		rightPanel = theme.RenderTitledPanelWith("Preview", theme.Size{W: rightW, H: m.innerH + 2}, previewFocused, m.preview.View(), m.theme.colors)
	}

	feedback := renderStatusLine(m.width, m.theme.status, m.statusMsg)
	legend := renderHelpLine(m.width, m.help, listKeyMapFor(m, previewFocused))

	out := theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Legend: legend})
	if m.height > 0 {
		if lines := strings.Split(out, "\n"); len(lines) > m.height {
			out = strings.Join(lines[:m.height], "\n")
		}
	}
	if m.alertVisible {
		out = theme.CompositeCenter(m.alert.Box(), out)
	}
	return out
}
