package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/components/alert"
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
)

// model is the root bubbletea model.
//
// The active pane is derived from state, not tracked explicitly:
//   - alert != nil       → paneAlert
//   - blockEdit != nil   → paneBlockEdit
//   - previewFocused     → panePreview
//   - otherwise          → paneList
type model struct {
	cfg         Config
	doc         *document.Document
	schemaTree  []schema.FieldDef
	knownByPath map[string]map[string]bool
	childrenOf  map[string][]schema.FieldDef

	list      listModel
	preview   textarea.Model
	blockEdit *blockEditState
	alert     *alert.Model

	previewFocused bool
	statusMsg      string
	helpVisible    bool

	width, height, listW, innerH int
}

// newModel constructs the root model from a Config. The path may be a file
// that does not yet exist; in that case the editor starts with an empty doc.
func newModel(cfg Config) (model, error) {
	if cfg.Schema == nil {
		return model{}, fmt.Errorf("editor: Config.Schema is required")
	}

	tree := schema.Discover(cfg.Schema)
	tree = applyHidden(tree, cfg.Hidden)
	known := schema.KnownChildren(tree)
	childrenOf := buildChildrenMap(tree)
	knownOrder := schema.TopLevelOrder(tree)

	doc, err := document.Load(cfg.Path, knownOrder)
	if err != nil {
		return model{}, fmt.Errorf("loading %s: %w", cfg.Path, err)
	}

	list := newListModel(knownOrder, doc.Blocks(), 0)

	preview := textarea.New()
	preview.CharLimit = 0
	preview.ShowLineNumbers = false
	preview.Blur()
	preview.SetValue(string(doc.Raw()))

	return model{
		cfg:         cfg,
		doc:         doc,
		schemaTree:  tree,
		knownByPath: known,
		childrenOf:  childrenOf,

		list:    list,
		preview: preview,
	}, nil
}

func applyHidden(fields []schema.FieldDef, hidden []string) []schema.FieldDef {
	if len(hidden) == 0 {
		return fields
	}
	skip := make(map[string]bool, len(hidden))
	for _, h := range hidden {
		skip[h] = true
	}
	out := fields[:0]
	for _, f := range fields {
		if !skip[f.YAMLName] {
			out = append(out, f)
		}
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

// fieldKind returns the Kind of the named top-level field, or KindScalar if not found.
func fieldKind(fields []schema.FieldDef, name string) schema.Kind {
	for _, f := range fields {
		if f.YAMLName == name {
			return f.Kind
		}
	}
	return schema.KindScalar
}

func (m model) activePane() pane {
	switch {
	case m.alert != nil:
		return paneAlert
	case m.blockEdit != nil:
		return paneBlockEdit
	case m.previewFocused:
		return panePreview
	default:
		return paneList
	}
}

func (m *model) scrollPreviewToKey(key string) {
	if key == "" {
		return
	}
	target := key + ":"
	for i, l := range strings.Split(string(m.doc.Raw()), "\n") {
		if strings.HasPrefix(l, target) {
			m.preview.SetCursor(i)
			return
		}
	}
}

// overlayConfirmedMsg is sent when the user commits a block edit (Ctrl+S).
type overlayConfirmedMsg struct{ Snippet string }

// overlayCancelledMsg is sent when the user cancels a block edit (Esc).
type overlayCancelledMsg struct{}

// pendingRemoveMsg is dispatched by the "Remove field?" confirm alert when the
// user chooses Yes. nodeIdx is the index into blockEditState.tree.nodes.
type pendingRemoveMsg struct{ nodeIdx int }

// confirmedDeleteMsg is dispatched by the "Remove block?" confirm alert when
// the user confirms deleting a top-level block from the main list.
type confirmedDeleteMsg struct{ Key string }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		return m, nil
	case spaceOnItemMsg:
		return m.handleSpace(msg.Item)
	case overlayConfirmedMsg:
		return m.handleOverlayConfirmed(msg.Snippet)
	case overlayCancelledMsg:
		m.blockEdit = nil
		m.statusMsg = "Cancelled."
		return m, nil
	case deleteItemMsg:
		return m.showConfirmAlert(
			"Remove block?",
			fmt.Sprintf("Remove %q? Its content will be lost.", msg.Key),
			func() tea.Msg { return confirmedDeleteMsg(msg) },
		)
	case confirmedDeleteMsg:
		m.alert = nil
		return m.handleDelete(msg.Key)
	case alert.DismissedMsg:
		// Forward to blockEdit first so its confirmAlert is cleared.
		if m.blockEdit != nil {
			be, cmd := m.blockEdit.Update(msg)
			m.blockEdit = &be
			return m, cmd
		}
		m.alert = nil
		return m, nil
	case doSaveMsg:
		return m.execSave()
	}

	switch m.activePane() {
	case paneAlert:
		if key, ok := msg.(tea.KeyMsg); ok {
			al, cmd := m.alert.Update(key)
			m.alert = &al
			return m, cmd
		}
	case paneBlockEdit:
		be, cmd := m.blockEdit.Update(msg)
		m.blockEdit = &be
		return m, cmd
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

func (m model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+s":
		mo, cmd := m.save()
		return mo, cmd, true
	case "ctrl+l":
		mo, cmd := m.validateKeys()
		return mo, cmd, true
	}
	return m, nil, false
}

func (m model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if mo, cmd, handled := m.handleGlobalKey(msg); handled {
		return mo, cmd
	}

	if !m.list.IsFiltering() {
		switch msg.String() {
		case "tab":
			return m.togglePreviewPane()
		case "?":
			m.helpVisible = !m.helpVisible
			return m, nil
		case "ctrl+u":
			return m.undo(), nil
		case "esc", "ctrl+c":
			if m.doc.Dirty() {
				return m.showConfirmAlert("Quit without saving?",
					"Unsaved changes will be lost.", tea.Quit)
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if it := m.list.SelectedItem(); it != nil {
		m.scrollPreviewToKey(it.Key)
	}
	return m, cmd
}

func (m model) handlePreviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "esc":
		return m.togglePreviewPane()
	}
	return m.updatePreviewEditor(msg)
}

func (m model) togglePreviewPane() (tea.Model, tea.Cmd) {
	if m.previewFocused {
		m.previewFocused = false
		m.preview.Blur()
		m.statusMsg = ""
		return m, nil
	}
	m.previewFocused = true
	cmd := m.preview.Focus()
	m.statusMsg = "Editing YAML directly — Tab/Esc back to list."
	return m, cmd
}

func (m model) updatePreviewEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	if err := m.doc.ReplaceRaw([]byte(m.preview.Value())); err == nil {
		m.list.Rebuild(m.doc.Blocks())
	}
	return m, cmd
}

func (m *model) syncView() {
	m.preview.SetValue(string(m.doc.Raw()))
	m.list.Rebuild(m.doc.Blocks())
	if it := m.list.SelectedItem(); it != nil {
		m.scrollPreviewToKey(it.Key)
	}
}

func (m model) handleSpace(it listItem) (tea.Model, tea.Cmd) {
	var initial string
	if it.Existing {
		current, err := m.doc.BlockContent(it.Key)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Error reading %s: %v", it.Key, err)
			return m, nil
		}
		initial = current
	} else {
		initial = it.Key + ":\n"
	}

	children := m.childrenOf[it.Key]
	kind := fieldKind(m.schemaTree, it.Key)
	be := newBlockEdit(m.cfg, blockSpec{key: it.Key, defs: children, kind: kind, content: initial, knownByPath: m.knownByPath}, m.width, m.height)
	be.isEdit = it.Existing
	m.blockEdit = &be
	action := "Add"
	if it.Existing {
		action = "Edit"
	}
	m.statusMsg = fmt.Sprintf("%s block %q — Tab panel, Ctrl+S commit, Esc back.", action, it.Key)
	return m, nil
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

func (m model) handleOverlayConfirmed(snippet string) (tea.Model, tea.Cmd) {
	isEdit := m.blockEdit != nil && m.blockEdit.isEdit

	var err error
	if isEdit {
		err = m.doc.Replace(m.blockEdit.key, snippet)
	} else {
		err = m.doc.Insert(snippet)
	}
	if err != nil {
		m.statusMsg = fmt.Sprintf("Apply error: %v", err)
		return m, nil
	}
	m.syncView()
	// Keep blockEdit open — user stays in editing mode after commit.
	if isEdit {
		m.statusMsg = "Block updated (not saved yet) — Esc to return."
	} else {
		// First commit transitions the block edit to edit mode.
		if m.blockEdit != nil {
			m.blockEdit.isEdit = true
		}
		m.statusMsg = "Block added (not saved yet) — Esc to return."
	}
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

func (m model) collectErrors() []string {
	var errs []string
	if u := schema.UnknownKeys(m.doc.Raw(), m.knownByPath); len(u) > 0 {
		errs = append(errs, "Unknown key(s): "+strings.Join(u, ", "))
	}
	errs = append(errs, RunAll(m.cfg.Validators, m.doc.Raw(), m.doc.Blocks())...)
	return errs
}

func formatErrors(errs []string) string {
	var sb strings.Builder
	for i, e := range errs {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("• ")
		sb.WriteString(e)
	}
	return sb.String()
}

func (m model) save() (tea.Model, tea.Cmd) {
	if errs := m.collectErrors(); len(errs) > 0 {
		return m.showAlert("Cannot save — fix errors first", formatErrors(errs), alert.KindError)
	}
	doSave := func() tea.Msg { return doSaveMsg{} }
	return m.showConfirmAlert("Save changes?", fmt.Sprintf("Save to %s?", m.doc.Path()), doSave)
}

type doSaveMsg struct{}

func (m model) execSave() (tea.Model, tea.Cmd) {
	if err := m.doc.Save(); err != nil {
		return m.showAlert("Save failed", err.Error(), alert.KindError)
	}
	return m.showAlert("Saved", fmt.Sprintf("Saved to %s.", m.doc.Path()), alert.KindSuccess)
}

func (m model) validateKeys() (tea.Model, tea.Cmd) {
	if errs := m.collectErrors(); len(errs) > 0 {
		return m.showAlert("Validation errors", formatErrors(errs), alert.KindError)
	}
	return m.showAlert("Validation passed", "All keys are valid and no conflicts were found.", alert.KindSuccess)
}

func (m model) showAlert(title, message string, kind alert.Kind) (tea.Model, tea.Cmd) {
	al := alert.New(title, message, kind, theme.Size{W: m.width, H: m.height})
	m.alert = &al
	return m, nil
}

func (m model) showConfirmAlert(title, message string, confirmCmd tea.Cmd) (tea.Model, tea.Cmd) {
	al := alert.NewConfirm(title, message, confirmCmd, theme.Size{W: m.width, H: m.height})
	m.alert = &al
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
	m.preview.SetWidth(previewW - 2)
	m.preview.SetHeight(m.innerH)
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	if m.width < 80 || m.height < 20 {
		return "Terminal too small — resize to at least 80×20."
	}

	switch m.activePane() {
	case paneAlert:
		return m.alert.View()
	case paneBlockEdit:
		return m.blockEdit.View()
	}

	if m.helpVisible {
		return renderHelpOverlay(listHelpSections, theme.Size{W: m.width, H: m.height})
	}

	header := renderHeader(m.cfg.Title, m.doc.Path(), m.doc.Dirty(), m.width)

	leftTitle := fmt.Sprintf("Blocks (%d/%d)", m.list.AddedCount(), len(m.list.knownKeys))
	leftPanel := theme.RenderTitledPanel(leftTitle, theme.Size{W: m.listW, H: m.innerH + 2}, !m.previewFocused, m.list.View())

	_, rightW := theme.TwoColumnWidths(m.width)
	previewTitle := "Preview"
	if m.previewFocused {
		previewTitle = "Editing YAML"
	}
	rightPanel := theme.RenderTitledPanel(previewTitle, theme.Size{W: rightW, H: m.innerH + 2}, m.previewFocused, m.preview.View())

	var hintText string
	if m.previewFocused {
		hintText = "[Tab] / [Esc] back to list"
	} else if m.list.IsFiltering() {
		hintText = "[type] filter • [↑/↓] navigate • [Enter] select • [Esc] clear"
	} else if it := m.list.SelectedItem(); it != nil && it.Existing {
		hintText = "[↑/↓] nav • [Enter] open • [ctrl+d] delete • [ctrl+u] undo • [ctrl+s] save • [ctrl+l] validate • [?] help"
	} else {
		hintText = "[↑/↓] nav • [Enter] add • [ctrl+u] undo • [ctrl+s] save • [ctrl+l] validate • [?] help"
	}

	feedback := lipgloss.NewStyle().Width(m.width).Render(statusStyle.Render(m.statusMsg))
	hint := lipgloss.NewStyle().Width(m.width).Render(statusStyle.Render(hintText))

	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Hint: hint})
}
