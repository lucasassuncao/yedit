package docgenerator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

type docPane int

const (
	docPaneList docPane = iota
	docPaneView
)

const (
	docHeaderLines = 1
	docStatusLines = 2
)

type docTUIModel struct {
	appName  string
	names    []string          // ordered display list (parents then their children)
	indent   map[string]int    // indentation level: 0 = root, 1 = child
	raw      map[string]string // name → raw markdown
	rendered map[string]string
	colors   theme.Colors

	cursor     int
	listOffset int
	listH      int
	listColW   int
	topicLinks []string // topic names linked from the current page, in order

	vp     viewport.Model
	vpColW int
	vpH    int

	active   docPane
	width    int
	height   int
	renderer *glamour.TermRenderer
}

// buildOrderedNames returns names in display order: root topics (sorted) each
// followed by their children (in schema order from ds.Children).
func buildOrderedNames(ds DocSet) (names []string, indent map[string]int) {
	indent = map[string]int{}
	childSet := map[string]bool{}
	for _, children := range ds.Children {
		for _, c := range children {
			childSet[c] = true
			indent[c] = 1
		}
	}

	roots := make([]string, 0, len(ds.Pages))
	for name := range ds.Pages {
		if !childSet[name] {
			roots = append(roots, name)
		}
	}
	sort.Strings(roots)

	for _, root := range roots {
		names = append(names, root)
		names = append(names, ds.Children[root]...)
	}
	return names, indent
}

func newDocTUIModel(ds DocSet, appName string, colors theme.Colors) docTUIModel {
	names, indent := buildOrderedNames(ds)
	return docTUIModel{
		appName:  appName,
		names:    names,
		indent:   indent,
		raw:      ds.Pages,
		rendered: make(map[string]string, len(ds.Pages)),
		colors:   colors,
		active:   docPaneList,
	}
}

func (m *docTUIModel) Init() tea.Cmd { return nil }

func (m *docTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		m.invalidateRendered()
		m.loadCurrent()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			if m.active == docPaneList {
				m.active = docPaneView
			} else {
				m.active = docPaneList
			}
			return m, nil
		}
		if m.handleLinkJump(msg.String()) {
			return m, nil
		}
		if m.active == docPaneList {
			m.handleListKey(msg.String())
		} else {
			m.handleViewportKey(msg.String())
		}
		return m, nil
	}
	return m, nil
}

// handleLinkJump navigates the left panel to a linked topic when a digit key
// matching a footnote index is pressed. Returns true if the key was consumed.
func (m *docTUIModel) handleLinkJump(key string) bool {
	if len(key) != 1 || key < "1" || key > "9" {
		return false
	}
	idx := int(key[0] - '1')
	if idx >= len(m.topicLinks) {
		return false
	}
	m.navigateTo(m.topicLinks[idx])
	m.active = docPaneList
	return true
}

func (m *docTUIModel) handleListKey(key string) {
	n := len(m.names)
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.listOffset = theme.ClampScroll(m.cursor, m.listOffset, m.listH)
			m.loadCurrent()
		}
	case "down", "j":
		if m.cursor < n-1 {
			m.cursor++
			m.listOffset = theme.ClampScroll(m.cursor, m.listOffset, m.listH)
			m.loadCurrent()
		}
	}
}

func (m *docTUIModel) handleViewportKey(key string) {
	switch key {
	case "up", "k":
		m.vp.ScrollUp(1)
	case "down", "j":
		m.vp.ScrollDown(1)
	case "pgup":
		m.vp.ScrollUp(m.vpH / 2)
	case "pgdown":
		m.vp.ScrollDown(m.vpH / 2)
	}
}

func (m *docTUIModel) relayout() {
	m.listColW, m.vpColW = theme.TwoColumnWidths(m.width)

	innerH := m.height - docHeaderLines - docStatusLines - 2
	if innerH < 1 {
		innerH = 1
	}
	m.listH = innerH
	m.vpH = innerH

	m.vp.Width = m.vpColW - 2
	m.vp.Height = m.vpH

	m.listOffset = theme.ClampScroll(m.cursor, m.listOffset, m.listH)

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(m.vp.Width),
	)
	if err == nil {
		m.renderer = r
	}
}

func (m *docTUIModel) invalidateRendered() {
	m.rendered = make(map[string]string, len(m.names))
}

func (m *docTUIModel) renderDoc(name string) string {
	if r, ok := m.rendered[name]; ok {
		return r
	}
	raw := m.raw[name]
	if m.renderer == nil {
		m.rendered[name] = raw
		return raw
	}
	out, err := m.renderer.Render(raw)
	if err != nil {
		m.rendered[name] = raw
		return raw
	}
	m.rendered[name] = out
	return out
}

func (m *docTUIModel) loadCurrent() {
	if len(m.names) == 0 || m.cursor >= len(m.names) || m.vp.Width == 0 {
		return
	}
	name := m.names[m.cursor]
	m.vp.SetContent(m.renderDoc(name))
	m.vp.GotoTop()
	m.topicLinks = extractTopicLinks(m.raw[name], m.raw)
}

// extractTopicLinks scans raw markdown for (./name.md) links and returns
// the topic names in appearance order, limited to known pages. Link targets
// are lowercased on generation, so they are resolved back to page keys
// case-insensitively.
func extractTopicLinks(raw string, known map[string]string) []string {
	var topics []string
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		for {
			i := strings.Index(line, "](./")
			if i < 0 {
				break
			}
			rest := line[i+4:]
			j := strings.Index(rest, ".md)")
			if j < 0 {
				break
			}
			if name, ok := resolvePageKey(rest[:j], known); ok && !seen[name] {
				topics = append(topics, name)
				seen[name] = true
			}
			line = rest[j+4:]
		}
	}
	return topics
}

// resolvePageKey maps a link target to a page key: exact match first, then a
// case-insensitive scan (link targets are lowercased filenames while page keys
// keep their original casing).
func resolvePageKey(target string, known map[string]string) (string, bool) {
	if _, ok := known[target]; ok {
		return target, true
	}
	for key := range known {
		if strings.EqualFold(key, target) {
			return key, true
		}
	}
	return "", false
}

func (m *docTUIModel) navigateTo(name string) {
	for i, n := range m.names {
		if n == name {
			m.cursor = i
			m.listOffset = theme.ClampScroll(m.cursor, m.listOffset, m.listH)
			m.loadCurrent()
			return
		}
	}
}

func (m *docTUIModel) View() string {
	if m.width == 0 {
		return "Loading…"
	}

	var listSB strings.Builder
	end := m.listOffset + m.listH
	if end > len(m.names) {
		end = len(m.names)
	}
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.colors.SelectionColor))
	availableStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.AvailableItemColor))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.colors.InactiveBorderColor)).PaddingLeft(1)

	for i := m.listOffset; i < end; i++ {
		label := m.names[i]
		pad := strings.Repeat("  ", m.indent[label])
		if i == m.cursor {
			listSB.WriteString(selectedStyle.Render("▶ "+pad+label) + "\n")
		} else {
			listSB.WriteString(availableStyle.Render("  "+pad+label) + "\n")
		}
	}

	leftPanel := theme.RenderTitledPanelWith("Topics", theme.Size{W: m.listColW, H: m.listH + 2}, m.active == docPaneList, listSB.String(), m.colors)

	rightTitle := "Documentation"
	if m.cursor >= 0 && m.cursor < len(m.names) {
		rightTitle = m.names[m.cursor]
	}
	rightPanel := theme.RenderTitledPanelWith(rightTitle, theme.Size{W: m.vpColW, H: m.vpH + 2}, m.active == docPaneView, m.vp.View(), m.colors)

	legend := statusStyle.Render("[Tab] switch panel  [↑/↓ j/k] navigate / scroll  [PgUp/PgDn] half-page  [1-9] jump to linked topic  [q] quit")
	header := theme.RenderHeaderWith(m.appName, "docs", "", m.width, m.colors)
	return theme.RenderTwoColumnView(theme.TwoColumnLayout{Header: header, Left: leftPanel, Right: rightPanel, Feedback: "", Legend: legend})
}

// RenderMarkdownDocsInTerminal launches the two-panel documentation TUI.
// appName is displayed in the header bar. An optional theme.Theme controls
// colors; zero value resolves to ThemeDark.
func RenderMarkdownDocsInTerminal(docs DocSet, appName string, t ...theme.Theme) error {
	if len(docs.Pages) == 0 {
		return fmt.Errorf("no documentation to display")
	}
	th := theme.Theme{}
	if len(t) > 0 {
		th = t[0]
	}
	m := newDocTUIModel(docs, appName, theme.ResolveColors(th))
	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run docs TUI: %w", err)
	}
	return nil
}
