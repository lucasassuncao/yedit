package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/components/picker"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

// overlayConfirmedMsg is sent when the user confirms with Ctrl+S.
type overlayConfirmedMsg struct{ Snippet string }

// overlayCancelledMsg is sent when the user presses Esc.
type overlayCancelledMsg struct{}

type overlayPanel int

const (
	overlayPanelFields overlayPanel = iota
	overlayPanelYAML
)

// overlayModel is the floating overlay for adding or editing a YAML block.
// Always uses two-panel layout: left field-toggle list + right YAML editor.
type overlayModel struct {
	cfg Config
	key string

	fieldList   fieldListModel
	fieldPanelW int
	fieldPanelH int

	yamlEditor textarea.Model
	yamlPanelW int
	yamlPanelH int

	active overlayPanel
	errMsg string

	isEdit  bool
	editKey string

	totalW int
	totalH int

	currentPreset string
	presetPicker  *picker.Model
}

// newOverlay builds an overlay for the given key.
func newOverlay(cfg Config, key string, childDefs []schema.FieldDef, initialContent string, totalW, totalH int) overlayModel {
	boxW := totalW - 4
	boxH := totalH - 4
	if boxW > 120 {
		boxW = 120
	}
	if boxH > 36 {
		boxH = 36
	}
	if boxW < 60 {
		boxW = 60
	}
	if boxH < 16 {
		boxH = 16
	}

	contentW := boxW - 4
	panelH := boxH - 8
	if panelH < 4 {
		panelH = 4
	}

	om := overlayModel{
		cfg:           cfg,
		key:           key,
		totalW:        totalW,
		totalH:        totalH,
		currentPreset: "custom",
	}

	// Prefer the "base" preset when opening a new (empty) block.
	trivial := key + ":\n"
	if initialContent == "" || initialContent == trivial {
		if cfg.Presets != nil {
			if y, err := cfg.Presets.PresetYAML(key, "base"); err == nil {
				initialContent = y
				om.currentPreset = "base"
			}
		}
		if initialContent == "" {
			initialContent = key + ":\n"
		}
	}

	om.initTwoPanel(childDefs, contentW, panelH, initialContent)
	return om
}

func (om *overlayModel) initTwoPanel(defs []schema.FieldDef, contentW, panelH int, initialContent string) {
	panelSpace := contentW - 4
	if panelSpace < 24 {
		panelSpace = 24
	}
	fieldPanelW := panelSpace / 3
	if fieldPanelW < 18 {
		fieldPanelW = 18
	}
	yamlPanelW := panelSpace - fieldPanelW

	preChecked := om.cfg.preCheckedSet(om.key)
	om.fieldList = newFieldList(defs, preChecked, panelH)
	om.fieldPanelW = fieldPanelW
	om.fieldPanelH = panelH

	ta := textarea.New()
	ta.SetWidth(yamlPanelW - 2)
	ta.SetHeight(panelH - 1)
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.Blur()

	om.yamlPanelW = yamlPanelW
	om.yamlPanelH = panelH
	om.yamlEditor = ta

	if len(defs) == 0 {
		om.active = overlayPanelYAML
		om.yamlEditor.Focus()
	} else {
		om.active = overlayPanelFields
	}

	trivial := om.key + ":\n"
	if initialContent != "" && initialContent != trivial {
		om.yamlEditor.SetValue(strings.ReplaceAll(initialContent, "\r\n", "\n"))
		om.fieldList.SetFields(syncFieldsFromYAML(om.key, om.fieldList.Fields(), om.yamlEditor.Value()))
	} else {
		snippets := om.cfg.fieldSnippetsFor(om.key)
		om.yamlEditor.SetValue(rebuildYAML(om.key, om.fieldList.Fields(), snippets))
		om.errMsg = ""
	}
}

func (om overlayModel) Init() tea.Cmd { return textarea.Blink }

func (om overlayModel) Update(msg tea.Msg) (overlayModel, tea.Cmd) {
	switch m := msg.(type) {
	case picker.SelectedMsg:
		return om.applyPreset(m.Name), nil
	case picker.CancelledMsg:
		om.presetPicker = nil
		return om, nil
	}

	if om.presetPicker != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			updated, cmd := om.presetPicker.Update(key)
			om.presetPicker = &updated
			return om, cmd
		}
		return om, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return om.updateYAMLEditor(msg)
	}
	return om.updateKey(key)
}

func (om overlayModel) updateKey(msg tea.KeyMsg) (overlayModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return om, func() tea.Msg { return overlayCancelledMsg{} }
	case tea.KeyCtrlS:
		return om.confirm()
	case tea.KeyTab:
		return om.switchPanel(), nil
	}

	if om.active == overlayPanelFields {
		if msg.String() == "p" && om.cfg.Presets != nil {
			names := om.cfg.Presets.ListPresets(om.key)
			if len(names) > 0 {
				p := picker.New("Preset", names, om.currentPreset, om.totalW, om.totalH)
				om.presetPicker = &p
			}
			return om, nil
		}
		return om.updateFieldPanel(msg), nil
	}

	return om.updateYAMLEditor(msg)
}

func (om overlayModel) updateYAMLEditor(msg tea.Msg) (overlayModel, tea.Cmd) {
	if om.active == overlayPanelYAML {
		var cmd tea.Cmd
		om.yamlEditor, cmd = om.yamlEditor.Update(msg)
		om.fieldList.SetFields(syncFieldsFromYAML(om.key, om.fieldList.Fields(), om.yamlEditor.Value()))
		return om, cmd
	}
	return om, nil
}

func (om overlayModel) applyPreset(name string) overlayModel {
	if om.cfg.Presets == nil {
		return om
	}
	y, err := om.cfg.Presets.PresetYAML(om.key, name)
	if err != nil {
		om.errMsg = fmt.Sprintf("preset error: %v", err)
		om.presetPicker = nil
		return om
	}
	om.yamlEditor.SetValue(y)
	om.currentPreset = name
	om.errMsg = ""
	om.fieldList.SetFields(syncFieldsFromYAML(om.key, om.fieldList.Fields(), y))
	om.presetPicker = nil
	return om
}

func (om overlayModel) confirm() (overlayModel, tea.Cmd) {
	om.errMsg = ""
	snippet := om.yamlEditor.Value()
	if err := validateSnippetText(snippet); err != nil {
		om.errMsg = fmt.Sprintf("Invalid YAML: %v", err)
		return om, nil
	}
	if !strings.HasSuffix(snippet, "\n") {
		snippet += "\n"
	}
	return om, func() tea.Msg { return overlayConfirmedMsg{Snippet: snippet} }
}

func (om overlayModel) switchPanel() overlayModel {
	if om.active == overlayPanelFields {
		om.active = overlayPanelYAML
		om.yamlEditor.Focus()
	} else {
		om.active = overlayPanelFields
		om.yamlEditor.Blur()
	}
	return om
}

func (om overlayModel) updateFieldPanel(msg tea.KeyMsg) overlayModel {
	updated, toggled := om.fieldList.Update(msg)
	om.fieldList = updated
	if toggled {
		fs := om.fieldList.ToggledField()
		snippets := om.cfg.fieldSnippetsFor(om.key)
		snippet := ""
		if snippets != nil {
			snippet = snippets[fs.Def.YAMLName]
		}
		if om.isEdit {
			val := applyFieldToggle(om.key, om.fieldList.Fields(), fs.Def.YAMLName, snippet, fs.Checked, om.yamlEditor.Value(), snippets)
			om.yamlEditor.SetValue(val)
			om.errMsg = ""
		} else {
			om.yamlEditor.SetValue(rebuildYAML(om.key, om.fieldList.Fields(), snippets))
			om.errMsg = ""
		}
	}
	return om
}

func (om overlayModel) View() string {
	action := "add block"
	if om.isEdit {
		action = "edit block"
	}
	titleText := fmt.Sprintf(" %s [%s · preset: %s] ", om.key, action, om.currentPreset)
	title := overlayTitleStyle.Render(titleText)

	content := om.viewTwoPanel()

	var hintText string
	if om.active == overlayPanelFields {
		if len(om.fieldList.Fields()) > 0 {
			hintText = "[Tab] switch panel • [Space] toggle • [p] preset • [ctrl+s] apply • [Esc] cancel"
		} else {
			hintText = "[Tab] switch panel • [p] preset • [ctrl+s] apply • [Esc] cancel"
		}
	} else {
		hintText = "[Tab] switch panel • [ctrl+s] apply • [Esc] cancel"
	}
	hint := statusStyle.Render(hintText)

	parts := []string{title, content}
	if om.errMsg != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(theme.Danger).Render(om.errMsg))
	}
	parts = append(parts, hint)

	box := overlayBorderStyle.Render(strings.Join(parts, "\n"))

	if om.presetPicker != nil {
		return om.presetPicker.View()
	}
	return theme.CenterBox(box, om.totalW, om.totalH)
}

func (om overlayModel) viewTwoPanel() string {
	leftBorder := panelStyle
	rightBorder := panelStyle
	if om.active == overlayPanelFields {
		leftBorder = activePanelStyle
	} else {
		rightBorder = activePanelStyle
	}

	leftPanel := leftBorder.
		Width(om.fieldPanelW).
		Height(om.fieldPanelH).
		Render(om.fieldList.View())

	rightPanel := rightBorder.
		Width(om.yamlPanelW).
		Height(om.yamlPanelH).
		Render(om.yamlEditor.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// validateSnippetText is a thin wrapper around document.ValidateSnippet to
// avoid an import cycle hint at the call site.
func validateSnippetText(text string) error {
	var check any
	return yamlUnmarshal([]byte(text), &check)
}
