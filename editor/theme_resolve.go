package editor

import (
	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/yedit/theme"
)

// resolvedTheme is the fully merged, ready-to-use styling for one editor
// instance. Built once in newModel; never mutated after that.
type resolvedTheme struct {
	colors theme.Colors

	// internal derived styles - computed from colors, not user-configurable
	existingItem    lipgloss.Style
	availableItem   lipgloss.Style
	unknownItem     lipgloss.Style
	passthroughItem lipgloss.Style
	draftItem       lipgloss.Style // checked-but-empty tree fields: will be pruned at save unless filled
	selectedItem    lipgloss.Style
	sectionLabel    lipgloss.Style
	status          lipgloss.Style
	filterPrompt    lipgloss.Style
	hintKey         lipgloss.Style
	hintDim         lipgloss.Style
	errorText       lipgloss.Style
}

// resolveTheme merges t into a concrete resolvedTheme. Merge order:
//  1. ThemeDark defaults
//  2. t.Base.Colors (if non-nil)
//  3. t.Colors (non-"" fields win)
//  4. Build derived styles from resolved colors
//  5. t.Styles overrides (non-zero lipgloss.Style wins)
//
// Steps 1-3 are theme.ResolveColors - the single owner of the color cascade.
func resolveTheme(t theme.Theme) resolvedTheme {
	c := theme.ResolveColors(t)

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
	return rt
}

// buildDerivedStyles creates the internal lipgloss styles from the resolved
// color palette. Respects NO_COLOR (via theme.Color) by producing empty colors.
func buildDerivedStyles(c theme.Colors) resolvedTheme {
	accent := theme.Color(c.ActiveBorderColor)
	accentBright := theme.Color(c.SelectionColor)
	muted := theme.Color(c.InactiveBorderColor)
	dim := theme.Color(c.AvailableItemColor)
	success := theme.Color(c.ExistingItemColor)
	danger := theme.Color(c.ErrorColor)

	return resolvedTheme{
		existingItem:    lipgloss.NewStyle().Foreground(success),
		availableItem:   lipgloss.NewStyle().Foreground(dim),
		unknownItem:     lipgloss.NewStyle().Foreground(danger),
		passthroughItem: lipgloss.NewStyle().Foreground(dim),
		draftItem:       lipgloss.NewStyle().Foreground(theme.Warning),
		selectedItem:    lipgloss.NewStyle().Bold(true).Foreground(accentBright),
		sectionLabel:    lipgloss.NewStyle().Bold(true).Foreground(accent).PaddingLeft(1),
		status:          lipgloss.NewStyle().Foreground(muted).PaddingLeft(1),
		filterPrompt:    lipgloss.NewStyle().Bold(true).Foreground(accentBright),
		hintKey:         lipgloss.NewStyle().Bold(true).Foreground(accent),
		hintDim:         lipgloss.NewStyle().Foreground(muted),
		errorText:       lipgloss.NewStyle().Foreground(danger),
	}
}
