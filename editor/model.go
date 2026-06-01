package editor

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
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
	// opened from the list; deeper entries are nested map-of-struct drill-ins.
	// The last element is the visible/active editor.
	blockEdits []*blockEditState
	alert      *alert.Model

	mode                         pane
	statusMsg                    string
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
	}, nil
}

// topBE returns the active (deepest) block editor, or nil when none is open.
func (m *model) topBE() *blockEditState {
	if len(m.blockEdits) == 0 {
		return nil
	}
	return m.blockEdits[len(m.blockEdits)-1]
}

// setTopBE replaces the active block editor in place.
func (m *model) setTopBE(be *blockEditState) {
	if len(m.blockEdits) > 0 {
		m.blockEdits[len(m.blockEdits)-1] = be
	}
}

func applyHidden(fields []schema.FieldDef, hidden []string) []schema.FieldDef {
	if len(hidden) == 0 {
		return fields
	}
	topHide := make(map[string]bool)
	nestedHide := make(map[string][]string)
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

// openChildMsg requests drilling into a nested map-of-struct field, pushing a
// new block editor scoped to that field onto the stack.
type openChildMsg struct {
	key     string
	defs    []schema.FieldDef
	kind    schema.Kind
	content string
	path    []string // path in the parent block's value to splice back into
}

// blockEditCommittedMsg is sent when the user commits a block edit (Ctrl+S).
type blockEditCommittedMsg struct{ Snippet string }

// blockEditDiscardedMsg is sent when the user cancels a block edit (Esc).
type blockEditDiscardedMsg struct{}

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
	case openItemMsg:
		return m.handleOpenItem(msg.Item)
	case openChildMsg:
		return m.handleOpenChild(msg)
	case blockEditCommittedMsg:
		return m.handleOverlayConfirmed(msg.Snippet)
	case blockEditDiscardedMsg:
		if len(m.blockEdits) > 0 {
			m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
		}
		if len(m.blockEdits) == 0 {
			m.mode = paneList
			m.statusMsg = "Cancelled."
		} else {
			m.statusMsg = ""
		}
		return m, nil
	case deleteItemMsg:
		return m.showConfirmAlert(
			"Remove block?",
			fmt.Sprintf("Remove %q? Its content will be lost.", msg.Key),
			func() tea.Msg { return confirmedDeleteMsg(msg) },
		)
	case confirmedDeleteMsg:
		m.alert = nil
		m.mode = paneList
		return m.handleDelete(msg.Key)
	case alert.DismissedMsg:
		// Forward to the active blockEdit first so its confirmAlert is cleared.
		if top := m.topBE(); top != nil {
			be, cmd := top.Update(msg)
			m.setTopBE(&be)
			return m, cmd
		}
		m.alert = nil
		m.mode = paneList
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
		top := m.topBE()
		if top == nil {
			m.mode = paneList
			return m, nil
		}
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+u" {
			if top.undoSnap != nil {
				be := top.restoreUndo()
				m.setTopBE(&be)
				return m, nil
			}
			return m.undo(), nil
		}
		be, cmd := top.Update(msg)
		m.setTopBE(&be)
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
	m.scrollPreviewToSelected()
	return m, cmd
}

func (m model) handlePreviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "esc":
		return m.togglePreviewPane()
	}
	// The preview is read-only; remaining keys only scroll the viewport.
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}

func (m model) togglePreviewPane() (tea.Model, tea.Cmd) {
	if m.mode == panePreview {
		m.mode = paneList
		m.statusMsg = ""
		return m, nil
	}
	m.mode = panePreview
	m.statusMsg = "Viewing YAML — ↑/↓ scroll, Tab/Esc back to list."
	return m, nil
}

func (m *model) syncView() {
	m.refreshPreview()
	m.list.Rebuild(m.doc.Blocks())
	m.scrollPreviewToSelected()
}

// scrollPreviewToSelected scrolls the read-only preview so the YAML for the
// selected top-level block sits near the top, letting list navigation track the
// document. Applies only in the list pane and only for keys present in the file.
// The scroll is line-based, so it can drift slightly when long lines above the
// block wrap.
func (m *model) scrollPreviewToSelected() {
	if m.mode != paneList {
		return
	}
	it := m.list.SelectedItem()
	if it == nil || !it.Existing {
		return
	}
	for _, b := range m.doc.Blocks() {
		if b.Key == it.Key {
			m.preview.SetYOffset(b.Line - 1)
			return
		}
	}
}

// newPreviewRenderer builds a glamour renderer that word-wraps to wrap columns.
// It starts from the dark style (or the colorless ASCII style under NO_COLOR)
// and trims glamour's default chrome: the document and code-block left margins
// stack to ~4 columns and the block prefix/suffix add blank lines, all wasteful
// inside a panel that already has its own border. A single-column margin is
// kept. Returns nil on error, in which case renderPreviewYAML falls back to
// plain text.
func newPreviewRenderer(wrap int) *glamour.TermRenderer {
	cfg := styles.DarkStyleConfig
	if os.Getenv("NO_COLOR") != "" {
		cfg = styles.NoTTYStyleConfig
	}
	one, zero := uint(1), uint(0)
	cfg.Document.Margin = &one
	cfg.Document.BlockPrefix = ""
	cfg.Document.BlockSuffix = ""
	cfg.CodeBlock.Margin = &zero

	r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(wrap))
	if err != nil {
		return nil
	}
	return r
}

// renderPreviewYAML renders raw YAML through r (wrapped in a markdown code fence)
// for syntax-highlighted display. Falls back to the plain text when r is nil or
// rendering fails.
func renderPreviewYAML(raw string, r *glamour.TermRenderer) string {
	raw = strings.TrimRight(raw, "\n")
	if r == nil || raw == "" {
		return raw
	}
	out, err := r.Render("```yaml\n" + raw + "\n```")
	if err != nil {
		return raw
	}
	return trimBlankLines(out)
}

var ansiEscapeRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// trimBlankLines drops leading and trailing whitespace-only lines — glamour
// emits a padded blank line around the code block — while leaving any interior
// blank lines intact. It is ANSI-aware so colored padding still reads as blank.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	blank := func(l string) bool {
		return strings.TrimSpace(ansiEscapeRE.ReplaceAllString(l, "")) == ""
	}
	start, end := 0, len(lines)
	for start < end && blank(lines[start]) {
		start++
	}
	for end > start && blank(lines[end-1]) {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

// clampLines truncates s to at most maxLines newline-separated lines so that
// content passed to RenderTitledPanel never overflows its allocated height.
// lipgloss.Height() is a minimum, not a cap — without this, a tall hint or
// preview would push the right column taller than the left.
func clampLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}

// refreshPreview re-renders the document into the read-only preview viewport,
// syntax-highlighted and wrapped to the current panel width.
func (m *model) refreshPreview() {
	m.preview.SetContent(renderPreviewYAML(string(m.doc.Raw()), m.previewRenderer))
}

func (m model) handleOpenItem(it listItem) (tea.Model, tea.Cmd) {
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
	m.blockEdits = []*blockEditState{&be}
	m.mode = paneBlockEdit
	return m, be.Init()
}

// handleOpenChild pushes a nested block editor for a map-of-struct field that
// the user drilled into from the parent editor. Unknown-key validation is left
// to the parent commit, so the child editor uses a nil knownByPath (its root key
// is the field name, which would otherwise read as an unknown top-level key).
func (m model) handleOpenChild(msg openChildMsg) (tea.Model, tea.Cmd) {
	content := msg.content
	if content == "" {
		content = msg.key + ":\n"
	}
	be := newBlockEdit(m.cfg, blockSpec{key: msg.key, defs: msg.defs, kind: msg.kind, content: content, knownByPath: nil}, m.width, m.height)
	be.isEdit = true
	be.childPath = msg.path
	m.blockEdits = append(m.blockEdits, &be)
	m.mode = paneBlockEdit
	return m, be.Init()
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
	if len(m.blockEdits) == 0 {
		return m, nil
	}

	// Nested child commit: splice the snippet back into the parent editor's YAML
	// at the drill-in path, resync the parent tree, then pop back to the parent.
	if len(m.blockEdits) > 1 {
		child := m.blockEdits[len(m.blockEdits)-1]
		parent := m.blockEdits[len(m.blockEdits)-2]
		newYAML := replaceSubBlock(parent.yamlEditor.Value(), child.childPath, snippet)
		parent.yamlEditor.SetValue(newYAML)
		parent.dirty = true
		parent.tree = parent.resyncTreeFromYAML()
		m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
		m.statusMsg = fmt.Sprintf("Updated %q (not saved yet) — Esc to return.", child.key)
		return m, nil
	}

	// Top-level block commit → write to the document.
	be := m.blockEdits[0]
	isEdit := be.isEdit
	var err error
	if isEdit {
		err = m.doc.Replace(be.key, snippet)
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
		be.isEdit = true
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
	m.mode = paneAlert
	return m, nil
}

func (m model) showConfirmAlert(title, message string, confirmCmd tea.Cmd) (tea.Model, tea.Cmd) {
	al := alert.NewConfirm(title, message, confirmCmd, theme.Size{W: m.width, H: m.height})
	m.alert = &al
	m.mode = paneAlert
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
	m.preview.Height = m.innerH
	m.previewRenderer = newPreviewRenderer(m.preview.Width)
	m.refreshPreview()
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
			return top.View()
		}
	}

	previewFocused := m.mode == panePreview

	header := renderHeader(m.cfg.Title, m.doc.Path(), m.doc.Dirty(), m.width)

	leftTitle := fmt.Sprintf("Blocks (%d/%d)", m.list.AddedCount(), len(m.list.knownKeys))
	leftPanel := theme.RenderTitledPanel(leftTitle, theme.Size{W: m.listW, H: m.innerH + 2}, !previewFocused, m.list.View())

	_, rightW := theme.TwoColumnWidths(m.width)
	rightPanel := theme.RenderTitledPanel("Preview", theme.Size{W: rightW, H: m.innerH + 2}, previewFocused, m.preview.View())

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

	feedback := lipgloss.NewStyle().Width(m.width).Render(statusStyle.Render(m.statusMsg))
	hint := lipgloss.NewStyle().Width(m.width).Render(statusStyle.Render(hintText))

	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: feedback, Hint: hint})
}
