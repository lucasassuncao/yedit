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
	"gopkg.in/yaml.v3"

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

// blockBreadcrumbPrefix returns the breadcrumb segments for all editors in the
// stack except the top one. The top editor appends its own key and tree segments.
func (m model) blockBreadcrumbPrefix() []string {
	n := len(m.blockEdits)
	if n <= 1 {
		return nil
	}
	var segs []string
	for _, be := range m.blockEdits[:n-1] {
		segs = append(segs, be.key)
		segs = append(segs, be.tree.BreadcrumbSegments()...)
	}
	return segs
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

// --- Screen transitions ---
//
// These are the ONLY functions that assign m.mode. Each one sets the active
// pane together with the data that pane owns, so the two invariants
//
//	m.alert != nil        ⟺  m.mode == paneAlert
//	len(m.blockEdits) > 0  ⟺  m.mode == paneBlockEdit
//
// cannot be violated by a caller that forgets to clear a sibling field. The
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

// openChildMsg requests drilling into a nested field, pushing a new block editor
// scoped to that field onto the stack. relSegs is the focus-path suffix from the
// parent editor's focus to the drilled-into node (e.g. [segIdx(2), segKey("any")]
// for the "any" field of the parent collection's current item). The model
// resolves the actual content from editRoot at the resulting focus path.
type openChildMsg struct {
	key     string
	defs    []schema.FieldDef
	kind    schema.Kind
	relSegs []pathSeg
}

// blockEditCommittedMsg is sent when the user commits a block edit (Ctrl+S).
type blockEditCommittedMsg struct{ Snippet string }

// drillOutMsg is sent when the user presses Esc inside a nested editor. Unlike
// blockEditDiscardedMsg (which abandons the whole block edit), it navigates up
// one level while KEEPING edits: the current level is flushed into the canonical
// editRoot, popped, and the parent editor is refreshed to reflect the change.
type drillOutMsg struct{}

// blockEditDiscardedMsg is sent when the user closes a block edit (Esc).
// discarded is true only when uncommitted changes were intentionally thrown away
// (user confirmed the "Discard changes?" dialog). It is false when Esc is pressed
// on a clean editor (no uncommitted changes) — in that case the status message
// from the last commit should be preserved.
type blockEditDiscardedMsg struct{ discarded bool }

// pendingRemoveMsg is dispatched by the "Remove field?" confirm alert when the
// user chooses Yes. nodeIdx is the index into blockEditState.tree.nodes.
type pendingRemoveMsg struct{ nodeIdx int }

// pendingEntryDeleteMsg is dispatched by the "Remove entry?" confirm alert when
// the user confirms deleting a whole collection entry. seqIdx indexes the entry.
type pendingEntryDeleteMsg struct{ seqIdx int }

// confirmedDeleteMsg is dispatched by the "Remove block?" confirm alert when
// the user confirms deleting a top-level block from the main list.
type confirmedDeleteMsg struct{ Key string }

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSizeMsg(msg)
	case openItemMsg:
		if m.cfg.ReadOnly {
			m.statusMsg = "Read-only mode — editing is disabled."
			return m, nil
		}
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

func (m model) handleBlockEditDiscarded(msg blockEditDiscardedMsg) (tea.Model, tea.Cmd) {
	if len(m.blockEdits) > 0 {
		m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
	}
	if len(m.blockEdits) == 0 {
		m.enterList()
		if msg.discarded {
			// User threw away uncommitted changes — show explicit feedback.
			m.statusMsg = "Cancelled."
		}
		// else: clean Esc after a commit — preserve the existing status message
		// (e.g. "Block updated (not saved yet)").
	} else {
		m.statusMsg = ""
	}
	return m, nil
}

// handleDrillOut navigates up one level while keeping edits. The current (child)
// editor is flushed into editRoot, popped, and the parent editor is refreshed
// from editRoot so it reflects what the child changed. Editing a child and
// returning to fix a parent field is therefore non-destructive — nothing is
// committed to the document until Ctrl+S. Only fired for nested editors.
func (m model) handleDrillOut() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) <= 1 {
		return m, nil
	}
	childWasDirty := m.topBE().dirty

	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		// Invalid YAML in the child — cannot write it into the canonical tree.
		// The error is already shown; stay so the user can fix it.
		return m, nil
	}
	m.blockEdits = m.blockEdits[:len(m.blockEdits)-1]
	m = m.refreshTopFromRoot(childWasDirty)
	m.statusMsg = ""
	return m, nil
}

// refreshTopFromRoot rebuilds the active editor's content from the node at its
// focus path in editRoot, preserving tree cursor/expansion and the current
// collection entry. markDirty propagates uncommitted-changes state up from a
// child so the top-level "Discard changes?" guard still fires.
func (m model) refreshTopFromRoot(markDirty bool) model {
	top := m.topBE()
	if top == nil {
		return m
	}
	node := nodeAt(m.editRoot, top.focus)
	if node == nil {
		return m
	}
	be := *top
	if be.isCollectionNav() {
		isMap := be.isMapNav()
		oldCount := entryCount(be.node, isMap)
		be.node = node
		// Rebuild the tree only when the entry count changed; otherwise keep it so
		// the expanded/collapsed view and cursor survive the round-trip.
		if entryCount(node, isMap) != oldCount {
			be.tree.nodes = be.collectionTreeNodes()
			if be.coll.current >= entryCount(node, isMap) {
				be.coll.current = entryCount(node, isMap) - 1
			}
		}
		be.yamlEditor.SetValue(be.entryYAML(be.coll.current))
	} else {
		be.node = node
		be.yamlEditor.SetValue(nodeToContent(be.key, node))
	}
	be.tree = be.resyncTreeFromYAML()
	if markDirty {
		be.dirty = true
	}
	m.setTopBE(&be)
	return m
}

func (m model) handleDeleteItemMsg(msg deleteItemMsg) (tea.Model, tea.Cmd) {
	if m.cfg.ReadOnly {
		m.statusMsg = "Read-only mode — deletion is disabled."
		return m, nil
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

func (m model) handlePaneBlockEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	top := m.topBE()
	if top == nil {
		m.enterList()
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+s":
			if m.cfg.ReadOnly {
				top.statusMsg = "Read-only mode — saving is disabled."
				m.setTopBE(top)
				return m, nil
			}
			// Commit all stacked editors at once, then save the file.
			return m.saveAll()
		case "ctrl+u":
			if top.undoSnap != nil {
				be := top.restoreUndo()
				be.statusMsg = "Undone."
				m.setTopBE(&be)
				return m, nil
			}
			// Never fall through to m.doc.Undo() while a block editor is open.
			top.statusMsg = "Nothing to undo."
			m.setTopBE(top)
			return m, nil
		}
	}
	be, cmd := top.Update(msg)
	m.setTopBE(&be)
	return m, cmd
}

func (m model) handleGlobalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+s":
		if m.cfg.ReadOnly {
			m.statusMsg = "Read-only mode — saving is disabled."
			return m, nil, true
		}
		mo, cmd := m.saveAll()
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
		case "h":
			m.showHint = !m.showHint
			m.relayout()
			m.scrollPreviewToSelected()
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
		m.enterList()
		m.statusMsg = ""
		return m, nil
	}
	m.enterPreview()
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

// selectedHint renders the Hint/Example panel body for the currently selected
// list item, using its schema metadata.
func (m model) selectedHint() string {
	it := m.list.SelectedItem()
	if it == nil || it.Separator {
		return m.theme.hintDim.Render("  select a field to see hints")
	}
	if it.Unknown {
		return m.theme.hintDim.Render("  unknown key — not in the schema")
	}
	def := fieldDefByName(m.schemaTree, it.Key)
	if def.YAMLName == "" {
		def.YAMLName = it.Key
	}
	return renderFieldHint(m.theme, def, m.hintExample(it.Key, def))
}

// hintExample resolves the Example snippet for a top-level field: its "base"
// preset when one exists, otherwise a structural fallback from the schema.
func (m model) hintExample(key string, def schema.FieldDef) string {
	if m.cfg.Presets != nil {
		if y, err := m.cfg.Presets.PresetYAML(key, "base"); err == nil {
			return y
		}
	}
	return generateFallbackExample(def)
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
	// Unknown items have no schema, so skip unknown-key validation inside the overlay.
	knownByPath := m.knownByPath
	if it.Unknown {
		knownByPath = nil
	}
	be := newBlockEdit(m.cfg, blockSpec{key: it.Key, defs: children, kind: kind, def: fieldDefByName(m.schemaTree, it.Key), content: initial, knownByPath: knownByPath}, m.width, m.height)
	be.isEdit = it.Existing
	be.focus = nil // top-level editor edits the whole block
	m.blockEdits = []*blockEditState{&be}
	m.editBlockKey = it.Key
	// Canonical tree, refreshed from the top editor on every flush (drill-in /
	// commit). A non-nil placeholder is enough; the first flush populates it.
	m.editRoot = &yaml.Node{Kind: yaml.MappingNode}
	m.enterBlockEdit()
	return m, be.Init()
}

// flushTopToRoot commits the active editor and writes its value node into
// editRoot at the editor's focus path. Returns (updatedModel, true) on success;
// on a validation error it sets the editor's error and returns false so the
// caller aborts the navigation/commit.
func (m model) flushTopToRoot() (model, bool) {
	top := m.topBE()
	committed, cmd := top.commit()
	m.setTopBE(&committed)
	if cmd == nil {
		m.statusMsg = committed.errMsg
		return m, false
	}
	snippet := cmd().(blockEditCommittedMsg).Snippet
	val := valueNodeOfSnippet(snippet)
	if val == nil || !setNodeAt(m.editRoot, committed.focus, val) {
		m.statusMsg = "internal error: could not write editor into canonical tree"
		return m, false
	}
	return m, true
}

// handleOpenChild drills into a nested field. It flushes the parent editor into
// the canonical editRoot, then builds the child editor from the node living at
// the child's focus path within that same tree — no substring copy. Unknown-key
// validation is left to the parent, so the child uses a nil knownByPath (its
// root key is the field name, which would otherwise read as an unknown key).
func (m model) handleOpenChild(msg openChildMsg) (tea.Model, tea.Cmd) {
	const maxNestingDepth = 10
	if len(m.blockEdits) >= maxNestingDepth {
		m.statusMsg = fmt.Sprintf("Maximum nesting depth (%d) reached.", maxNestingDepth)
		return m, nil
	}

	// Flush the parent into editRoot so the child reads the parent's live state.
	parentFocus := append([]pathSeg(nil), m.topBE().focus...)
	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		return m, nil
	}

	childFocus := append([]pathSeg(nil), parentFocus...)
	childFocus = append(childFocus, msg.relSegs...)
	content := msg.key + ":\n"
	if node := nodeAt(m.editRoot, childFocus); node != nil {
		content = nodeToContent(msg.key, node)
	}
	be := newBlockEdit(m.cfg, blockSpec{key: msg.key, defs: msg.defs, kind: msg.kind, content: content, knownByPath: nil}, m.width, m.height)
	be.isEdit = true
	be.focus = childFocus

	m.blockEdits = append(m.blockEdits, &be)
	m.enterBlockEdit()
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

// handleOverlayConfirmed handles a blockEditCommittedMsg: the editor committing
// itself to the document while staying open. The live Ctrl+S flow uses commitAll
// (canonical-tree flush) instead; this path remains for direct top-level commits
// such as tests seeding content. Nested commits are never produced this way.
func (m model) handleOverlayConfirmed(snippet string) (tea.Model, tea.Cmd) {
	if len(m.blockEdits) != 1 {
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
	// Re-sync after commit so repeated Ctrl+S is idempotent.
	if fresh, err := m.doc.BlockContent(be.key); err == nil {
		*be = be.resyncAfterCommit(fresh)
	}
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
		var filtered []string
		for _, k := range u {
			if !m.list.passthrough[k] {
				filtered = append(filtered, k)
			}
		}
		if len(filtered) > 0 {
			errs = append(errs, "Unknown key(s): "+strings.Join(filtered, ", "))
		}
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

// saveAll is the Ctrl+S handler. When block editors are open it commits all
// stacked editors into m.doc and returns to the list — file save is a separate
// action triggered by Ctrl+S from the list view. When no editors are open it
// saves the file directly.
func (m model) saveAll() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) > 0 {
		return m.commitAll()
	}
	return m.save()
}

// commitAll commits the open editor stack into m.doc and returns to the list
// without writing the file. Because every drill-in already flushed its parent
// into editRoot, only the active (top) editor is still live: flush it, then
// serialize the whole canonical tree once. No per-level string splicing.
func (m model) commitAll() (tea.Model, tea.Cmd) {
	if len(m.blockEdits) == 0 {
		return m, nil
	}
	isEdit := m.blockEdits[0].isEdit

	var ok bool
	if m, ok = m.flushTopToRoot(); !ok {
		return m, nil
	}

	final := nodeToContent(m.editBlockKey, m.editRoot)
	var err error
	if isEdit {
		err = m.doc.Replace(m.editBlockKey, final)
	} else {
		err = m.doc.Insert(final)
	}
	if err != nil {
		m.statusMsg = fmt.Sprintf("Apply error: %v", err)
		return m, nil
	}
	m.syncView()
	m.enterList()
	m.statusMsg = "Changes committed (not saved yet) — ctrl+s to save."
	return m, nil
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
	return m.showAlert("Saved", fmt.Sprintf("Saved to %s.", m.doc.Path()), alert.KindSuccess)
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

	header := renderHeader(m.cfg.Title, m.doc.Path(), m.doc.Dirty(), m.cfg.ReadOnly, m.width, m.theme)

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
	if !previewFocused && !m.list.IsFiltering() {
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
