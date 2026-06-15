package editor

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/theme"
)

func TestResolveThemeZeroValue(t *testing.T) {
	is := assert.New(t)
	rt := resolveTheme(theme.Theme{})
	is.NotEmpty(rt.colors.ActiveBorderColor, "zero-value theme should resolve ActiveBorderColor from ThemeDark")
	is.NotEmpty(rt.colors.ExistingItemColor, "zero-value theme should resolve ExistingItemColor from ThemeDark")
}

func TestResolveThemeBaseOverride(t *testing.T) {
	must := require.New(t)
	rt := resolveTheme(theme.Theme{Base: &theme.ThemeDracula})
	must.Equal("#BD93F9", rt.colors.ActiveBorderColor, "expected Dracula accent")
}

func TestResolveThemeColorOverride(t *testing.T) {
	is := assert.New(t)
	rt := resolveTheme(theme.Theme{
		Base:   &theme.ThemeDracula,
		Colors: theme.Colors{ActiveBorderColor: "#FF0000"},
	})
	is.Equal("#FF0000", rt.colors.ActiveBorderColor, "Colors.ActiveBorderColor should override Base")
	is.Equal("#50FA7B", rt.colors.ExistingItemColor, "non-overridden ExistingItemColor should inherit from Dracula")
}

func TestResolveThemeStyleOverride(t *testing.T) {
	must := require.New(t)
	custom := lipgloss.NewStyle().Bold(true)
	rt := resolveTheme(theme.Theme{
		Styles: theme.Styles{ErrorText: &custom},
	})
	must.Equal(&custom, rt.styles.ErrorText, "Styles.ErrorText pointer should be stored in resolved theme")
}

func TestResolveThemeDerivedColorsSet(t *testing.T) {
	is := assert.New(t)
	rt := resolveTheme(theme.Theme{})
	// Colors resolved from ThemeDark must all be non-empty.
	is.NotEmpty(rt.colors.ActiveBorderColor, "derived colors should be non-empty after resolving ThemeDark")
	is.NotEmpty(rt.colors.ExistingItemColor, "derived colors should be non-empty after resolving ThemeDark")
	is.NotEmpty(rt.colors.ErrorColor, "derived colors should be non-empty after resolving ThemeDark")
}
