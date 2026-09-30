package editor

import (
	"fmt"
	"github.com/lucasassuncao/yedit/fieldtree"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"gopkg.in/yaml.v3"

	"github.com/lucasassuncao/bezel/animation"
	"github.com/lucasassuncao/bezel/browser"
	"github.com/lucasassuncao/bezel/draw"
	"github.com/lucasassuncao/bezel/layout"
	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/shell"
	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/render"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/validate"

	"github.com/lucasassuncao/yedit/keys"

	"github.com/lucasassuncao/yedit/blocklist"
)

type pane int

const (
	paneList pane = iota
	panePreview
	paneBlockEdit
	paneAlert
	paneDocPreset
	paneHint // the Hint/Example panel has the keys, to scroll it
)

// model is the root bubbletea model. The mode field names the active pane, and
// the shell's overlay/blockEdits hold that pane's data: an overlay is open iff
// mode == paneAlert, blockEdits non-empty iff mode == paneBlockEdit.
type model struct {
	cfg             Config
	doc             document.Document
	schemaTree      []schema.FieldDef
	knownByPath     map[string]map[string]bool
	childrenOf      map[string][]schema.FieldDef
	wiredValidators WiredValidators // built once in newModel, reused on every save/validate

	list            blocklist.Model
	preview         viewport.Model
	previewRenderer *glamour.TermRenderer
	// blockEdits stacks the block editors: index 0 is the block opened from the
	// list, deeper entries are drill-ins, and the last is the active editor. It
	// carries only UI state and each editor's focus path; the data lives in
	// editRoot.
	blockEdits []blockEditState
	// editRoot is the canonical *yaml.Node for the block being edited. Drilling in
	// moves a focus path within this one tree rather than copying substrings
	// between stacked editors, and committing serializes it once. Non-focused
	// parts stay live nodes, so nested edits cannot corrupt them by splicing.
	editRoot     *yaml.Node
	editBlockKey string // top-level YAML key of editRoot
	docPreset    browser.Model
	theme        theme.Resolved
	// sh is the chrome: size, layout, status row, legend and the alert overlay.
	sh shell.Shell

	mode          pane
	showHint      bool            // split the right column to show the Hint/Example panel
	hintAnim      animation.Tween // in-flight hint transition; inactive unless Config.AnimationDuration is set
	hintScroll    int             // first hint line shown while mode == paneHint
	hintFrom      pane            // the pane paneHint gives the keys back to
	saved         bool            // at least one save succeeded this session; reported via Result
	width, height int
}

// newModel constructs the root model from a Config. A path that does not exist
// yet starts the editor with an empty doc.
func newModel(cfg Config) (model, error) {
	if cfg.Schema == nil {
		return model{}, fmt.Errorf("editor: Config.Schema is required")
	}

	tree := discoverSchema(cfg)
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

	rt := theme.Resolve(cfg.Theme, true) // re-resolved when the terminal answers Init
	list := blocklist.New(knownOrder, doc.Blocks(), passthrough, 0, rt)

	preview := viewport.New(viewport.WithWidth(0), viewport.WithHeight(0))
	preview.SetContent(render.PreviewYAML(string(doc.Raw()), nil))
	preview.LeftGutterFunc = draw.ViewportGutter(rt.Muted)
	return model{
		cfg:             cfg,
		doc:             doc,
		schemaTree:      tree,
		knownByPath:     known,
		childrenOf:      childrenOf,
		wiredValidators: validate.WireWithSchema(cfg.Validators, tree, cfg.Metadata),

		list:     list,
		preview:  preview,
		showHint: cfg.EnableHints,
		theme:    rt,
		sh:       shell.New(shell.Config{Layout: rootLayout(0), Theme: rt, Title: cfg.Title, LegendLines: cfg.LegendLines}),
	}.withActions(), nil
}

// withActions stores the current screen's actions on the shell: the legend
// rows count against the body, so the layout needs them too.
func (m model) withActions() model {
	m.sh = m.sh.SetActions(m.actions()...)
	return m
}

// A model-level alert can appear over the list or over an active block editor,
// so enterAlert preserves blockEdits and dismissal returns through
// enterBlockEdit instead of discarding the stack via enterList. The block
// editor's own confirmAlert stays in paneBlockEdit and is handled separately in
// handleDismissedAlert.
func (m model) enterList() model {
	m.mode = paneList
	m.sh = m.sh.Pop()
	m.blockEdits = nil
	m.editRoot = nil
	m.editBlockKey = ""
	return m
}

// enterPreview focuses the read-only preview pane.
func (m model) enterPreview() model {
	m.mode = panePreview
	m.sh = m.sh.Pop()
	return m
}

// enterBlockEdit makes the block-editor stack the active screen. The caller must
// have pushed onto m.blockEdits first.
func (m model) enterBlockEdit() model {
	m.mode = paneBlockEdit
	m.sh = m.sh.Pop()
	return m
}

// enterAlert shows a modal over the current (list) screen.
func (m model) enterAlert(o overlay.Overlay) model {
	m.mode = paneAlert
	m.sh = m.sh.Push(o)
	return m
}

// enterDocPreset switches to the document-level preset picker.
func (m model) enterDocPreset(pb browser.Model) model {
	m.mode = paneDocPreset
	m.docPreset = pb
	m.sh = m.sh.Pop()
	return m.relayout()
}

// docPresetPanes is the preset picker screen: presets on the left, the
// chosen one on the right.
func (m model) docPresetPanes() map[string]shell.Pane {
	return map[string]shell.Pane{
		"presets": {Title: "Presets", Body: func(r layout.Rect) string { return m.docPreset.ListView(m.theme, r.H) }},
		"preview": {Title: "Preview", Body: func(r layout.Rect) string { return m.docPreset.PreviewView(r.H) }},
	}
}

// discoverSchema runs schema discovery for cfg (honouring SchemaRecursionDepth)
// and applies the Hidden filter. It is the single producer of the schema tree:
// newModel hands the same tree to the UI and to the wired validators, so the
// two can never disagree about the schema.
func discoverSchema(cfg Config) []schema.FieldDef {
	return applyHidden(schema.DiscoverDepth(cfg.Schema, cfg.SchemaRecursionDepth), cfg.Hidden)
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
		// Only what the tree draws inline is stamped now. A field opened in its
		// own editor is stamped when it opens (handleOpenChild), by the same path,
		// so a recursive schema is never walked past what is on screen.
		if len(f.Children) > 0 && fieldtree.ExpandsInline(f) {
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

// traceLocation describes where in the UI a message is about to be handled,
// for OnMsg session tracing. Format: "<pane>" or, inside the block editor,
// "block:<key>:<panel>:<mode>".
func (m model) traceLocation() string {
	switch m.mode {
	case paneList:
		return "list"
	case panePreview:
		return "preview"
	case paneHint:
		return "hint"
	case paneAlert:
		return "alert"
	case paneDocPreset:
		return "docPreset"
	case paneBlockEdit:
		if len(m.blockEdits) == 0 {
			return "blockEdit"
		}
		be := m.blockEdits[len(m.blockEdits)-1]
		panelName := "tree"
		switch be.active {
		case blockEditPanelYAML:
			panelName = "yaml"
		case blockEditPanelHint:
			panelName = "hint"
		}
		modeName := "editing"
		switch be.mode {
		case modePresetBrowser:
			modeName = "presetbrowser"
		case modeConfirming:
			modeName = "confirming"
		}
		return fmt.Sprintf("block:%s:%s:%s", be.key, panelName, modeName)
	default:
		return "unknown"
	}
}

// Init asks the terminal for its background so the theme can match it.
func (m model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// withTheme swaps the styles in the root screen and every stacked block editor.
func (m model) withTheme(rt theme.Resolved) model {
	m.theme = rt
	m.sh = m.sh.SetTheme(rt)
	m.list = m.list.Rebuild(m.doc.Blocks(), rt)
	m.preview.LeftGutterFunc = draw.ViewportGutter(rt.Muted)
	for i := range m.blockEdits {
		m.blockEdits[i] = m.blockEdits[i].withTheme(rt)
	}
	return m
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.cfg.Trace.OnMsg != nil {
		m.cfg.Trace.OnMsg(m.traceLocation(), msg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.handleWindowSizeMsg(msg)
	case tea.BackgroundColorMsg:
		return m.withTheme(theme.Resolve(m.cfg.Theme, msg.IsDark())), nil
	case blocklist.OpenItemMsg:
		return m.handleOpenItem(msg.Item)
	case openChildMsg:
		return m.dispatch(DrillIn{Key: msg.key, Defs: msg.defs, Kind: msg.kind, RelSegs: msg.relSegs})
	case blockEditDiscardedMsg:
		return m.handleBlockEditDiscarded(msg)
	case drillOutMsg:
		return m.dispatch(DrillOut{})
	case commitRequestedMsg:
		return m.saveAll()
	case blocklist.DeleteItemMsg:
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
	case overlay.CloseMsg:
		return m.handleDismissedAlert(msg)
	case doSaveMsg:
		return m.dispatch(Save{})
	case saveResultMsg:
		return m.handleSaveResult(msg)
	case reloadResultMsg:
		return m.handleReloadResult(msg)
	case hintAnimTickMsg:
		return m.handleHintAnimTick(msg)
	case previewBackMsg:
		return m.focusRootPane(shell.FocusMsg{From: "preview", To: "list"})
	case shell.FocusMsg:
		if m.mode == paneBlockEdit {
			return m.handlePaneBlockEdit(msg)
		}
		return m.focusRootPane(msg)
	case quitRequestedMsg:
		return m.quitOrConfirm()
	case openDocPresetsMsg:
		return m.openDocPresets()
	case toggleHintsMsg:
		return m.toggleHints()
	case focusHintMsg:
		return m.toggleHintFocus(), nil
	case saveRequestedMsg:
		return m.dispatch(CommitBlock{})
	case docUndoMsg:
		return m.dispatch(DocUndo{})
	case docRedoMsg:
		return m.dispatch(DocRedo{})
	case reloadRequestedMsg:
		return m.reload()
	}

	return m.handleModeUpdate(msg)
}

func (m model) handleSaveResult(msg saveResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.showAlert("Save failed", msg.err.Error(), overlay.Danger)
	}
	// The save ran on a snapshot; apply only its persistence outcome so any
	// edit made while the save was in flight is not clobbered.
	m.doc = m.doc.MarkSaved(msg.doc)
	m.saved = true
	// syncView refreshes the list's dirty decorations (e.g. unsaved-changes
	// indicator) immediately so they reflect the now-saved state.
	m = m.syncView()
	return m.showAlert("Saved", fmt.Sprintf("Saved to %s.", m.doc.Path()), overlay.Success)
}

func (m model) handleReloadResult(msg reloadResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m.showAlert("Reload failed", msg.err.Error(), overlay.Danger)
	}
	m.doc = msg.doc
	m = m.syncView()
	return m.withStatus(fmt.Sprintf("Reloaded %s from disk.", m.doc.Path()))
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
	return m.withStatus(fmt.Sprintf("Applied preset %q - ctrl+s to save.", msg.Name))
}

// handleDismissedAlert pops the overlay and returns to the screen under it.
// In paneBlockEdit the modal belongs to the block editor's own shell, so the
// message is forwarded there; otherwise a preserved editor stack is restored.
func (m model) handleDismissedAlert(msg overlay.CloseMsg) (tea.Model, tea.Cmd) {
	if m.mode == paneBlockEdit {
		if top := m.topBE(); top != nil {
			be, cmd := top.Update(msg)
			return m.withTopBE(be), cmd
		}
	}
	m.sh, _, _ = m.sh.Update(msg)
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
	// Ctrl+C quits from every mode (the terminal is in raw mode, so it arrives
	// as a plain key). Intercepting it here keeps the policy uniform instead of
	// working only in the list view.
	if km, ok := msg.(tea.KeyMsg); ok && key.Matches(km, keys.CtrlCQuit) {
		return m.quitOrConfirm()
	}
	switch m.mode {
	case paneAlert:
		if key, ok := msg.(tea.KeyMsg); ok {
			// Global shortcuts (save, validate) work even while an alert is shown.
			if mo, cmd, handled := m.handleGlobalKey(key); handled {
				return mo, cmd
			}
			var cmd tea.Cmd
			m.sh, _, cmd = m.sh.Update(key)
			return m, cmd
		}
	case paneBlockEdit:
		return m.handlePaneBlockEdit(msg)
	case panePreview:
		return m.handlePreviewUpdate(msg)
	case paneHint:
		if key, ok := msg.(tea.KeyMsg); ok {
			return m.handleHintKey(key)
		}
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

// quitOrConfirm quits immediately when nothing would be lost, otherwise asks
// first. Uncommitted block-editor changes count as loss even while the
// document itself is still clean.
func (m model) quitOrConfirm() (tea.Model, tea.Cmd) {
	dirty := m.doc.Dirty()
	for _, be := range m.blockEdits {
		if be.dirty {
			dirty = true
			break
		}
	}
	if dirty {
		return m.showConfirmAlert("Quit without saving?",
			"Unsaved changes will be lost.", tea.Quit)
	}
	return m, tea.Quit
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
	m.sh, _, _ = m.sh.Update(msg)
	m = m.relayout()
	// relayout only sizes the root list/preview; forward the resize to every
	// stacked sub-model so each editor's panels resize too.
	if len(m.blockEdits) > 0 {
		// Clone the stack before updating: writing through m.blockEdits would
		// mutate the backing array shared with prior model copies (the same
		// copy-on-write discipline withTopBE enforces).
		updated := make([]blockEditState, len(m.blockEdits))
		var cmd tea.Cmd
		for i := range m.blockEdits {
			be, c := m.blockEdits[i].Update(msg)
			updated[i] = be
			if i == len(m.blockEdits)-1 {
				cmd = c
			}
		}
		m.blockEdits = updated
		return m, cmd
	}
	return m, nil
}

func (m model) handleDeleteItemMsg(msg blocklist.DeleteItemMsg) (tea.Model, tea.Cmd) {
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
		return m.withStickyError(fmt.Sprintf("Error removing %s: %v", key, err)), nil
	}
	m = m.syncView()
	return m.withStatus(fmt.Sprintf("Removed %q (not saved yet).", key))
}

func (m model) showAlert(title, message string, kind overlay.Kind) (tea.Model, tea.Cmd) {
	m = m.enterAlert(overlay.NewAlert(kind, title, message, m.theme.ModalFor(kind), m.theme.Legend))
	return m, nil
}

func (m model) showConfirmAlert(title, message string, confirmCmd tea.Cmd) (tea.Model, tea.Cmd) {
	m = m.enterAlert(overlay.NewConfirm(title, message, confirmCmd, m.theme.Modal, m.theme.Legend))
	return m, nil
}

func (m model) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m model) viewContent() string {
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

	info := m.doc.Path()
	if m.doc.Dirty() {
		info += " ● modified"
	}
	sh := m.sh.SetSubtitle(info).SetActions(m.actions()...)

	if m.mode == paneDocPreset {
		focus := "presets"
		if m.docPreset.PreviewFocus {
			focus = "preview"
		}
		return sh.SetFocus(focus).View(m.docPresetPanes())
	}

	focus := m.rootPane()
	panes := map[string]shell.Pane{
		"list": {
			Title: fmt.Sprintf("Blocks (%d/%d)", m.list.AddedCount(), m.list.KnownCount()),
			Body:  func(layout.Rect) string { return m.list.View(m.theme) },
		},
		"preview": {Title: "Preview", Body: func(layout.Rect) string { return m.preview.View() }},
	}
	if m.hintVisible() {
		panes["hint"] = shell.Pane{Title: "Hint/Example", Body: func(layout.Rect) string { return m.hintView() }}
	}
	return sh.SetFocus(focus).View(panes)
}
