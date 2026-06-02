package editor

import (
	"os"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

// resolvedTheme is the fully merged, ready-to-use styling for one editor
// instance. Built once in newModel; never mutated after that.
type resolvedTheme struct {
	colors theme.Colors
	styles theme.Styles

	// internal derived styles — computed from colors, not user-configurable
	existingItem  lipgloss.Style
	availableItem lipgloss.Style
	unknownItem   lipgloss.Style
	selectedItem  lipgloss.Style
	sectionLabel  lipgloss.Style
	status        lipgloss.Style
	filterPrompt  lipgloss.Style
	hintKey       lipgloss.Style
	hintDim       lipgloss.Style
	errorText     lipgloss.Style
}

// resolveTheme merges t into a concrete resolvedTheme. Merge order:
//  1. ThemeDark defaults
//  2. t.Base.Colors (if non-nil)
//  3. t.Colors (non-"" fields win)
//  4. Build derived styles from resolved colors
//  5. t.Styles overrides (non-zero lipgloss.Style wins)
func resolveTheme(t theme.Theme) resolvedTheme {
	c := theme.ThemeDark.Colors
	if t.Base != nil {
		c = mergeColors(c, t.Base.Colors)
	}
	c = mergeColors(c, t.Colors)

	rt := buildDerivedStyles(c)
	rt.colors = c

	if t.Styles.ErrorText != nil {
		rt.errorText = *t.Styles.ErrorText
	}
	if t.Styles.HintText != nil {
		rt.hintDim = *t.Styles.HintText
	}
	if t.Styles.CursorLine != nil {
		rt.selectedItem = *t.Styles.CursorLine
	}
	rt.styles = t.Styles
	return rt
}

func mergeColors(base, over theme.Colors) theme.Colors {
	if over.Accent != "" {
		base.Accent = over.Accent
	}
	if over.AccentBright != "" {
		base.AccentBright = over.AccentBright
	}
	if over.Muted != "" {
		base.Muted = over.Muted
	}
	if over.Dim != "" {
		base.Dim = over.Dim
	}
	if over.Success != "" {
		base.Success = over.Success
	}
	if over.Danger != "" {
		base.Danger = over.Danger
	}
	return base
}

// buildDerivedStyles creates the internal lipgloss styles from the resolved
// color palette. Respects NO_COLOR by producing empty colors when set.
func buildDerivedStyles(c theme.Colors) resolvedTheme {
	accent := toColor(c.Accent)
	accentBright := toColor(c.AccentBright)
	muted := toColor(c.Muted)
	dim := toColor(c.Dim)
	success := toColor(c.Success)
	danger := toColor(c.Danger)

	return resolvedTheme{
		existingItem:  lipgloss.NewStyle().Foreground(success),
		availableItem: lipgloss.NewStyle().Foreground(dim),
		unknownItem:   lipgloss.NewStyle().Foreground(danger),
		selectedItem:  lipgloss.NewStyle().Bold(true).Foreground(accentBright),
		sectionLabel:  lipgloss.NewStyle().Bold(true).Foreground(accent).PaddingLeft(1),
		status:        lipgloss.NewStyle().Foreground(muted).PaddingLeft(1),
		filterPrompt:  lipgloss.NewStyle().Bold(true).Foreground(accentBright),
		hintKey:       lipgloss.NewStyle().Bold(true).Foreground(accent),
		hintDim:       lipgloss.NewStyle().Foreground(muted),
		errorText:     lipgloss.NewStyle().Foreground(danger),
	}
}

// toColor converts a color string to lipgloss.Color, returning an empty color
// when the NO_COLOR environment variable is set (monochrome mode).
func toColor(s string) lipgloss.Color {
	if os.Getenv("NO_COLOR") != "" {
		return lipgloss.Color("")
	}
	return lipgloss.Color(s)
}
