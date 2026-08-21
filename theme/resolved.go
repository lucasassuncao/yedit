package theme

import (
	"charm.land/lipgloss/v2"
)

// Resolved is a Theme after the whole cascade has been applied: ready-to-use
// lipgloss styles plus the colors they were derived from. A consumer builds one
// per instance and only reads it afterwards.
type Resolved struct {
	Colors Colors

	// Derived styles, computed from Colors rather than configured directly.
	// Theme.Styles can still override some of them; see Resolve.
	ExistingItem    lipgloss.Style
	AvailableItem   lipgloss.Style
	UnknownItem     lipgloss.Style
	PassthroughItem lipgloss.Style
	DraftItem       lipgloss.Style // checked-but-empty tree fields: will be pruned at save unless filled
	SelectedItem    lipgloss.Style
	SectionLabel    lipgloss.Style
	Status          lipgloss.Style
	FilterPrompt    lipgloss.Style
	HintKey         lipgloss.Style
	HintDim         lipgloss.Style
	ErrorText       lipgloss.Style
}

// Resolve merges t into a concrete Resolved. Merge order:
//  1. ThemePlain defaults
//  2. t.Base.Colors (if non-nil)
//  3. t.Colors (non-"" fields win)
//  4. Build derived styles from resolved Colors
//  5. t.Styles overrides (non-zero lipgloss.Style wins)
//
// Steps 1-3 are ResolveColors - the single owner of the color cascade.
func Resolve(t Theme) Resolved {
	c := ResolveColors(t)

	rt := buildDerivedStyles(c)
	rt.Colors = c

	if t.Styles.ErrorText != nil {
		rt.ErrorText = *t.Styles.ErrorText
	}
	if t.Styles.HintText != nil {
		rt.HintDim = *t.Styles.HintText
	}
	if t.Styles.CursorLine != nil {
		rt.SelectedItem = *t.Styles.CursorLine
	}
	return rt
}

// buildDerivedStyles creates the internal lipgloss styles from the resolved
// color palette.
func buildDerivedStyles(c Colors) Resolved {
	accent := lipgloss.Color(c.ActiveBorderColor)
	accentBright := lipgloss.Color(c.SelectionColor)
	muted := lipgloss.Color(c.InactiveBorderColor)
	dim := lipgloss.Color(c.AvailableItemColor)
	success := lipgloss.Color(c.ExistingItemColor)
	danger := lipgloss.Color(c.ErrorColor)

	return Resolved{
		ExistingItem:    lipgloss.NewStyle().Foreground(success),
		AvailableItem:   lipgloss.NewStyle().Foreground(dim),
		UnknownItem:     lipgloss.NewStyle().Foreground(danger),
		PassthroughItem: lipgloss.NewStyle().Foreground(dim),
		DraftItem:       lipgloss.NewStyle().Foreground(Warning),
		SelectedItem:    lipgloss.NewStyle().Bold(true).Foreground(accentBright),
		SectionLabel:    lipgloss.NewStyle().Bold(true).Foreground(accent).PaddingLeft(1),
		Status:          lipgloss.NewStyle().Foreground(muted).PaddingLeft(1),
		FilterPrompt:    lipgloss.NewStyle().Bold(true).Foreground(accentBright),
		HintKey:         lipgloss.NewStyle().Bold(true).Foreground(accent),
		HintDim:         lipgloss.NewStyle().Foreground(muted),
		ErrorText:       lipgloss.NewStyle().Foreground(danger),
	}
}
