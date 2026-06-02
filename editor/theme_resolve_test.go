package editor

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/lucasassuncao/yedit/theme"
)

func TestResolveThemeZeroValue(t *testing.T) {
	rt := resolveTheme(theme.Theme{})
	if rt.colors.Accent == "" {
		t.Fatal("zero-value theme should resolve Accent from ThemeDark")
	}
	if rt.colors.Success == "" {
		t.Fatal("zero-value theme should resolve Success from ThemeDark")
	}
}

func TestResolveThemeBaseOverride(t *testing.T) {
	rt := resolveTheme(theme.Theme{Base: &theme.ThemeDracula})
	if rt.colors.Accent != "#BD93F9" {
		t.Fatalf("expected Dracula accent, got %q", rt.colors.Accent)
	}
}

func TestResolveThemeColorOverride(t *testing.T) {
	rt := resolveTheme(theme.Theme{
		Base:   &theme.ThemeDracula,
		Colors: theme.Colors{Accent: "#FF0000"},
	})
	if rt.colors.Accent != "#FF0000" {
		t.Fatalf("Colors.Accent should override Base, got %q", rt.colors.Accent)
	}
	if rt.colors.Success != "#50FA7B" {
		t.Fatalf("non-overridden Success should inherit from Dracula, got %q", rt.colors.Success)
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
	if rt.colors.Accent == "" || rt.colors.Success == "" || rt.colors.Danger == "" {
		t.Fatal("derived colors should be non-empty after resolving ThemeDark")
	}
}
