package theme

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveThemeZeroValue(t *testing.T) {
	is := assert.New(t)
	rt := Resolve(Theme{})
	is.NotEmpty(rt.Colors.ActiveBorderColor, "zero-value theme should resolve ActiveBorderColor from ThemePlain")
	is.NotEmpty(rt.Colors.ExistingItemColor, "zero-value theme should resolve ExistingItemColor from ThemePlain")
}

func TestResolveThemeBaseOverride(t *testing.T) {
	must := require.New(t)
	rt := Resolve(Theme{Base: &ThemeGrape})
	must.Equal("#9B59B6", rt.Colors.ActiveBorderColor, "expected Grape accent")
}

func TestResolveThemeColorOverride(t *testing.T) {
	is := assert.New(t)
	rt := Resolve(Theme{
		Base:   &ThemeGrape,
		Colors: Colors{ActiveBorderColor: "#FF0000"},
	})
	is.Equal("#FF0000", rt.Colors.ActiveBorderColor, "Colors.ActiveBorderColor should override Base")
	is.Equal("#5DBB63", rt.Colors.ExistingItemColor, "non-overridden ExistingItemColor should inherit from Grape")
}

func TestResolveThemeStyleOverride(t *testing.T) {
	must := require.New(t)
	custom := lipgloss.NewStyle().Bold(true)
	rt := Resolve(Theme{
		Styles: Styles{ErrorText: &custom},
	})
	must.Equal(custom, rt.ErrorText, "Styles.ErrorText should be applied to rt.ErrorText")
}

func TestResolveThemeDerivedColorsSet(t *testing.T) {
	is := assert.New(t)
	rt := Resolve(Theme{})
	// Colors resolved from ThemePlain must all be non-empty.
	is.NotEmpty(rt.Colors.ActiveBorderColor, "derived Colors should be non-empty after resolving ThemePlain")
	is.NotEmpty(rt.Colors.ExistingItemColor, "derived Colors should be non-empty after resolving ThemePlain")
	is.NotEmpty(rt.Colors.ErrorColor, "derived Colors should be non-empty after resolving ThemePlain")
}
