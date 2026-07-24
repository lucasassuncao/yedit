package editor

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasassuncao/yedit/theme"
)

func TestResolveThemeZeroValue(t *testing.T) {
	is := assert.New(t)
	rt := resolveTheme(theme.Theme{})
	is.NotEmpty(rt.colors.ActiveBorderColor, "zero-value theme should resolve ActiveBorderColor from ThemePlain")
	is.NotEmpty(rt.colors.ExistingItemColor, "zero-value theme should resolve ExistingItemColor from ThemePlain")
}

func TestResolveThemeBaseOverride(t *testing.T) {
	must := require.New(t)
	rt := resolveTheme(theme.Theme{Base: &theme.ThemeGrape})
	must.Equal("#9B59B6", rt.colors.ActiveBorderColor, "expected Grape accent")
}

func TestResolveThemeColorOverride(t *testing.T) {
	is := assert.New(t)
	rt := resolveTheme(theme.Theme{
		Base:   &theme.ThemeGrape,
		Colors: theme.Colors{ActiveBorderColor: "#FF0000"},
	})
	is.Equal("#FF0000", rt.colors.ActiveBorderColor, "Colors.ActiveBorderColor should override Base")
	is.Equal("#5DBB63", rt.colors.ExistingItemColor, "non-overridden ExistingItemColor should inherit from Grape")
}

func TestResolveThemeStyleOverride(t *testing.T) {
	must := require.New(t)
	custom := lipgloss.NewStyle().Bold(true)
	rt := resolveTheme(theme.Theme{
		Styles: theme.Styles{ErrorText: &custom},
	})
	must.Equal(custom, rt.errorText, "Styles.ErrorText should be applied to rt.errorText")
}

func TestResolveThemeDerivedColorsSet(t *testing.T) {
	is := assert.New(t)
	rt := resolveTheme(theme.Theme{})
	// Colors resolved from ThemePlain must all be non-empty.
	is.NotEmpty(rt.colors.ActiveBorderColor, "derived colors should be non-empty after resolving ThemePlain")
	is.NotEmpty(rt.colors.ExistingItemColor, "derived colors should be non-empty after resolving ThemePlain")
	is.NotEmpty(rt.colors.ErrorColor, "derived colors should be non-empty after resolving ThemePlain")
}
