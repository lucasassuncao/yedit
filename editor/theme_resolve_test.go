package editor

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

func TestResolveThemeZeroValue(t *testing.T) {
	rt := resolveTheme(theme.Theme{})
	if rt.colors.ActiveBorderColor == "" {
		t.Fatal("zero-value theme should resolve ActiveBorderColor from ThemeDark")
	}
	if rt.colors.ExistingItemColor == "" {
		t.Fatal("zero-value theme should resolve ExistingItemColor from ThemeDark")
	}
}

func TestResolveThemeBaseOverride(t *testing.T) {
	rt := resolveTheme(theme.Theme{Base: &theme.ThemeDracula})
	if rt.colors.ActiveBorderColor != "#BD93F9" {
		t.Fatalf("expected Dracula accent, got %q", rt.colors.ActiveBorderColor)
	}
}

func TestResolveThemeColorOverride(t *testing.T) {
	rt := resolveTheme(theme.Theme{
		Base:   &theme.ThemeDracula,
		Colors: theme.Colors{ActiveBorderColor: "#FF0000"},
	})
	if rt.colors.ActiveBorderColor != "#FF0000" {
		t.Fatalf("Colors.ActiveBorderColor should override Base, got %q", rt.colors.ActiveBorderColor)
	}
	if rt.colors.ExistingItemColor != "#50FA7B" {
		t.Fatalf("non-overridden ExistingItemColor should inherit from Dracula, got %q", rt.colors.ExistingItemColor)
	}
}

func TestResolveThemeStyleOverride(t *testing.T) {
	custom := lipgloss.NewStyle().Bold(true)
	rt := resolveTheme(theme.Theme{
		Styles: theme.Styles{ErrorText: &custom},
	})
	if rt.styles.ErrorText != &custom {
		t.Fatal("Styles.ErrorText pointer should be stored in resolved theme")
	}
}

func TestResolveThemeDerivedColorsSet(t *testing.T) {
	rt := resolveTheme(theme.Theme{})
	// Colors resolved from ThemeDark must all be non-empty.
	if rt.colors.ActiveBorderColor == "" || rt.colors.ExistingItemColor == "" || rt.colors.ErrorColor == "" {
		t.Fatal("derived colors should be non-empty after resolving ThemeDark")
	}
}
