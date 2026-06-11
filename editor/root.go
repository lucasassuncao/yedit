package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/internal/alert"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

type pane int

const (
	paneList pane = iota
	panePreview
	paneBlockEdit
	paneAlert
)

// model is the root bubbletea model.
//
// The active pane is explicit via the mode field. The alert/blockEdits fields
// hold per-mode data: alert is non-nil iff mode == paneAlert, blockEdits is
// non-empty iff mode == paneBlockEdit (its last element is the visible editor).
type model struct {
	cfg         Config
	doc         *document.Document
	schemaTree  []schema.FieldDef
	knownByPath map[string]map[string]bool
	childrenOf  map[string][]schema.FieldDef

	list            listModel
	preview         viewport.Model
	previewRenderer *glamour.TermRenderer
	// blockEdits is a stack of block editors. Index 0 is the top-level block
	// opened from the list; deeper entries are nested drill-ins. The last element
	// is the visible/active editor. The stack carries only UI state (cursor,
	// expansion) and each editor's focus path — the canonical data lives in
	// editRoot.
	blockEdits []*blockEditState
	// editRoot is the single canonical *yaml.Node for the block currently being
	// edited: the parsed value of the top-level block. Drilling in moves a focus
	// path within this one tree (be.focus) rather than copying substrings between
	// stacked editors, and committing serializes it once. Non-focused parts stay
	// as live nodes, so nested edits can never corrupt them via string splicing.
	editRoot     *yaml.Node
	editBlockKey string // top-level YAML key of editRoot
	alert        *alert.Model
	theme        resolvedTheme

	mode                         pane
	showHint                     bool // root view: split the right column to show the Hint/Example panel
	saved                        bool // at least one save succeeded this session; reported via Result
	statusMsg                    string
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
	// RequiredFromSchema validators cannot see the schema at construction time;
	// wire the discovered (and Hidden-filtered) tree into them here.
	for _, v := range cfg.Validators {
		if rfs, ok := v.(*requiredFromSchemaValidator); ok {
			rfs.defs = tree
		}
	}
	known := schema.KnownChildren(tree)
	childrenOf := buildChildrenMap(tree)
	knownOrder := schema.TopLevelOrder(tree)

	doc, err := document.Load(cfg.Path, knownOrder)
	if err != nil {
		return model{}, fmt.Errorf("loading %s: %w", cfg.Path, err)
	}
	if cfg.SavePath != "" {
		doc.SetPath(cfg.SavePath)
	}

	passthrough := make(map[string]bool, len(cfg.PassthroughKeys))
	for _, k := range cfg.PassthroughKeys {
		passthrough[k] = true
	}

	list := newListModel(knownOrder, doc.Blocks(), passthrough, 0)

	preview := viewport.New(0, 0)
	preview.SetContent(renderPreviewYAML(string(doc.Raw()), nil))

	return model{
		cfg:         cfg,
		doc:         doc,
		schemaTree:  tree,
		knownByPath: known,
		childrenOf:  childrenOf,

		list:    list,
		preview: preview,
		theme:   resolveTheme(cfg.Theme),
	}, nil
}

// model-level alert is always a modal over the list (the block editor uses its
// own confirmAlert), so enterAlert preserves blockEdits and dismissal returns to
// the list via enterList.

// enterList makes the block list the active screen, discarding any open editor
// stack and alert.
func (m *model) enterList() {
	m.mode = paneList
	m.alert = nil
	m.blockEdits = nil
}

// enterPreview focuses the read-only preview pane.
func (m *model) enterPreview() {
	m.mode = panePreview
	m.alert = nil
}

// enterBlockEdit makes the block-editor stack the active screen. The caller is
// responsible for having pushed onto m.blockEdits first.
func (m *model) enterBlockEdit() {
	m.mode = paneBlockEdit
	m.alert = nil
}

// enterAlert shows a modal alert over the current (list) screen.
func (m *model) enterAlert(al alert.Model) {
	m.mode = paneAlert
	m.alert = &al
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
		return m.handleOpenChild(msg)
	case blockEditCommittedMsg:
		return m.handleOverlayConfirmed(msg.Snippet)
	case blockEditDiscardedMsg:
		return m.handleBlockEditDiscarded(msg)
	case drillOutMsg:
		return m.handleDrillOut()
	case deleteItemMsg:
		return m.handleDeleteItemMsg(msg)
	case confirmedDeleteMsg:
		m.enterList()
		return m.handleDelete(msg.Key)
	case confirmedReloadMsg:
		m.enterList()
		return m.execReload()
	case alert.DismissedMsg:
		// Forward to the active blockEdit first so its confirmAlert is cleared.
		if top := m.topBE(); top != nil {
			be, cmd := top.Update(msg)
			m.setTopBE(&be)
			return m, cmd
		}
		m.enterList()
		return m, nil
	case doSaveMsg:
		return m.execSave()
	}

	switch m.mode {
	case paneAlert:
		if key, ok := msg.(tea.KeyMsg); ok {
			al, cmd := m.alert.Update(key)
			m.alert = &al
			return m, cmd
		}
	case paneBlockEdit:
		return m.handlePaneBlockEdit(msg)
	case panePreview:
		if key, ok := msg.(tea.KeyMsg); ok {
			return m.handlePreviewKey(key)
		}
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	case paneList:
		if key, ok := msg.(tea.KeyMsg); ok {
			return m.handleListKey(key)
		}
	}
	return m, nil
}

func (m model) handleWindowSizeMsg(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.relayout()
	// relayout only sizes the root list/preview; forward the resize to every
	// stacked sub-model so each editor's panels resize too.
	if len(m.blockEdits) > 0 {
		var cmd tea.Cmd
		for i := range m.blockEdits {
			be, c := m.blockEdits[i].Update(msg)
			m.blockEdits[i] = &be
			if i == len(m.blockEdits)-1 {
				cmd = c
			}
		}
		return m, cmd
	}
	if m.alert != nil {
		m.alert.Resize(theme.Size{W: m.width, H: m.height})
	}
	return m, nil
}

func (m model) handleDeleteItemMsg(msg deleteItemMsg) (tea.Model, tea.Cmd) {
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
	if err := m.doc.Remove(key); err != nil {
		m.statusMsg = fmt.Sprintf("Error removing %s: %v", key, err)
		return m, nil
	}
	m.syncView()
	m.statusMsg = fmt.Sprintf("Removed %q (not saved yet).", key)
	return m, nil
}

func (m model) undo() tea.Model {
	if !m.doc.Undo() {
		m.statusMsg = "Nothing to undo."
		return m
	}
	m.syncView()
	m.statusMsg = "Undone."
	return m
}

func (m model) redo() tea.Model {
	if !m.doc.Redo() {
		m.statusMsg = "Nothing to redo."
		return m
	}
	m.syncView()
	m.statusMsg = "Redone."
	return m
}

func (m model) collectErrors() []Violation {
	var errs []Violation
	if u := schema.UnknownKeys(m.doc.Raw(), m.knownByPath); len(u) > 0 {
		var filtered []string
		for _, k := range u {
			if !m.list.passthrough[k] {
				filtered = append(filtered, k)
			}
		}
		if len(filtered) > 0 {
			errs = append(errs, Violation{Message: "Unknown key(s): " + strings.Join(filtered, ", ")})
		}
	}
	errs = append(errs, RunAll(m.cfg.Validators, m.doc.Raw(), m.doc.Blocks())...)
	return errs
}

func formatErrors(errs []Violation) string {
	var sb strings.Builder
	for i, e := range errs {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("• ")
		sb.WriteString(e.String())
	}
	return sb.String()
}

func (m model) save() (tea.Model, tea.Cmd) {
	errs := m.collectErrors()
	if len(errs) > 0 && !m.cfg.NoValidateOnSave {
		return m.showAlert("Cannot save — fix errors first", formatErrors(errs), alert.KindError)
	}
	doSave := func() tea.Msg { return doSaveMsg{} }
	// An external edit since open is a substantive data-loss risk — always confirm
	// before clobbering it, even under NoSaveConfirm.
	if m.doc.ExternallyChanged() {
		msg := fmt.Sprintf("%s changed on disk since you opened it.\nSaving overwrites those external changes.", m.doc.Path())
		return m.showConfirmAlert("File changed on disk — overwrite?", msg, doSave)
	}
	if len(errs) > 0 {
		// NoValidateOnSave: always confirm — warning is substantive, not routine.
		msg := fmt.Sprintf("Save to %s?\n\nWarnings:\n%s", m.doc.Path(), formatErrors(errs))
		return m.showConfirmAlert("Save with warnings?", msg, doSave)
	}
	if m.cfg.NoSaveConfirm {
		return m, doSave
	}
	return m.showConfirmAlert("Save changes?", fmt.Sprintf("Save to %s?", m.doc.Path()), doSave)
}

type doSaveMsg struct{}

func (m model) execSave() (tea.Model, tea.Cmd) {
	if err := m.doc.Save(); err != nil {
		return m.showAlert("Save failed", err.Error(), alert.KindError)
	}
	m.saved = true
	return m.showAlert("Saved", fmt.Sprintf("Saved to %s.", m.doc.Path()), alert.KindSuccess)
}

// reload re-reads the file from disk, discarding local edits. Unsaved changes
// are a substantive loss, so they require confirmation; a clean document
// reloads immediately.
func (m model) reload() (tea.Model, tea.Cmd) {
	if m.doc.Dirty() {
		msg := fmt.Sprintf("Re-read %s from disk?\nUnsaved changes will be lost.", m.doc.Path())
		return m.showConfirmAlert("Reload from disk?", msg, func() tea.Msg { return confirmedReloadMsg{} })
	}
	return m.execReload()
}

func (m model) execReload() (tea.Model, tea.Cmd) {
	if err := m.doc.Reload(); err != nil {
		return m.showAlert("Reload failed", err.Error(), alert.KindError)
	}
	m.syncView()
	m.statusMsg = fmt.Sprintf("Reloaded %s from disk.", m.doc.Path())
	return m, nil
}

func (m model) validateKeys() (tea.Model, tea.Cmd) {
	if errs := m.collectErrors(); len(errs) > 0 {
		return m.showAlert("Validation errors", formatErrors(errs), alert.KindError)
	}
	return m.showAlert("Validation passed", "All keys are valid and no conflicts were found.", alert.KindSuccess)
}

func (m model) showAlert(title, message string, kind alert.Kind) (tea.Model, tea.Cmd) {
	m.enterAlert(alert.New(title, message, kind, theme.Size{W: m.width, H: m.height}))
	return m, nil
}

func (m model) showConfirmAlert(title, message string, confirmCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.enterAlert(alert.NewConfirm(title, message, confirmCmd, theme.Size{W: m.width, H: m.height}))
	return m, nil
}

const (
	headerLines    = 1
	statusBarLines = 2
)

func (m *model) relayout() {
	var previewW int
	m.listW, previewW = theme.TwoColumnWidths(m.width)
	m.innerH = m.height - headerLines - statusBarLines - 2
	if m.innerH < 1 {
		m.innerH = 1
	}
	m.list.SetHeight(m.innerH)
	m.preview.Width = previewW - 2
	ph := m.innerH
	if m.showHint {
		ph = m.previewPanelH()
	}
	if ph < 1 {
		ph = 1
	}
	m.preview.Height = ph
	m.previewRenderer = newPreviewRenderer(m.preview.Width)
	m.refreshPreview()
}

// hintPanelH is the content height of the Hint/Example panel when it shares the
// right column with the preview. Mirrors blockEditState.hintH: ~1/3, floored.
func (m model) hintPanelH() int {
	total := m.innerH - 2 // extra border row from stacking two panels
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

// previewPanelH is the content height of the preview when the hint panel shares
// the right column.
func (m model) previewPanelH() int {
	h := m.innerH - 2 - m.hintPanelH()
	if h < 0 {
		h = 0
	}
	return h
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	if m.width < 80 || m.height < 20 {
		return "Terminal too small — resize to at least 80×20."
	}

	switch m.mode {
	case paneAlert:
		return m.alert.View()
	case paneBlockEdit:
		if top := m.topBE(); top != nil {
			return top.View(m.blockBreadcrumbPrefix())
		}
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

	var hintText string
	if previewFocused {
		hintText = hintModelPreviewFocused
	} else if m.list.IsFiltering() {
		hintText = hintModelFiltering
	} else if it := m.list.SelectedItem(); it != nil && it.Existing {
		hintText = hintModelExisting
	} else {
		hintText = hintModelNew
	}
	if !previewFocused && !m.list.IsFiltering() && m.cfg.Hints != nil {
		if m.showHint {
			hintText += hintSep + keyHintHide
		} else {
			hintText += hintSep + keyHint
		}
	}

	feedback := lipgloss.NewStyle().Width(m.width).Render(m.theme.status.Render(m.statusMsg))
	hint := lipgloss.NewStyle().Width(m.width).Render(m.theme.status.Render(hintText))

	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Hint: hint})
}
