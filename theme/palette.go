// Package theme provides the palette, base lipgloss styles, and shared
// layout primitives used across yedit-built TUIs.
package theme

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// colorVal returns a lipgloss.Color unless NO_COLOR is set, in which case it
// returns an empty color (terminal default) so all rendering is monochrome.
func colorVal(c string) lipgloss.Color {
	if os.Getenv("NO_COLOR") != "" {
		return lipgloss.Color("")
	}
	return lipgloss.Color(c)
}

// Palette — narrow on purpose. Clients can extend it with their own colours;
// add to this list only when at least two yedit components need it.
var (
	Accent       = colorVal("63")  // blue — active borders, primary highlight
	AccentBright = colorVal("212") // pink — titles, selection
	Muted        = colorVal("240") // grey — inactive borders, status hints
	Dim          = colorVal("245") // light grey — secondary text
	Success      = colorVal("82")  // green — existing/added items, success alerts
	Danger       = colorVal("196") // red — error alerts
)

// Common item styles. Each TUI is free to compose its own variants on top.
var (
	SelectedItem  = lipgloss.NewStyle().Bold(true).Foreground(AccentBright)
	ExistingItem  = lipgloss.NewStyle().Foreground(Success)
	AvailableItem = lipgloss.NewStyle().Foreground(Dim)
	StatusBar     = lipgloss.NewStyle().Foreground(Muted).PaddingLeft(1)
)

// Colors holds the six palette values that drive all editor styling.
// Each field is a lipgloss-compatible color string: a hex value ("#7C3AED"),
// an ANSI 256-color code ("63"), or a named terminal color.
// Empty string means "inherit from Base" during theme resolution.
type Colors struct {
	Accent       string
	AccentBright string
	Muted        string
	Dim          string
	Success      string
	Danger       string
}

// Styles holds optional per-element lipgloss overrides. Nil fields are ignored
// during theme resolution and the default derived from Colors is used instead.
type Styles struct {
	ActiveBorder   *lipgloss.Style
	InactiveBorder *lipgloss.Style
	CursorLine     *lipgloss.Style
	Header         *lipgloss.Style
	HintText       *lipgloss.Style
	ErrorText      *lipgloss.Style
}

// Theme is a three-layer appearance configuration:
//   - Base: an optional preset to inherit from (nil → ThemeDark)
//   - Colors: per-field overrides applied on top of Base.Colors
//   - Styles: lipgloss overrides applied on top of derived defaults
type Theme struct {
	Base   *Theme
	Colors Colors
	Styles Styles
}

// All returns all built-in theme presets keyed by their CLI name.
// Useful for --theme flag validation and --list-themes output in host CLIs.
func All() map[string]Theme {
	return map[string]Theme{
		"dark":        ThemeDark,
		"light":       ThemeLight,
		"dracula":     ThemeDracula,
		"monokai":     ThemeMonokai,
		"solarized":   ThemeSolarized,
		"banana":      ThemeBanana,
		"mint":        ThemeMint,
		"strawberry":  ThemeStrawberry,
		"blueberry":   ThemeBlueberry,
		"mango":       ThemeMango,
		"watermelon":  ThemeWatermelon,
		"peach":       ThemePeach,
		"kiwi":        ThemeKiwi,
		"lemon":       ThemeLemon,
		"orange":      ThemeOrange,
		"grape":       ThemeGrape,
		"cherry":      ThemeCherry,
		"pineapple":   ThemePineapple,
		"raspberry":   ThemeRaspberry,
		"lime":        ThemeLime,
		"pomegranate": ThemePomegranate,
		"apple":       ThemeApple,
		"plum":        ThemePlum,
		"apricot":     ThemeApricot,
		"dragonfruit": ThemeDragonfruit,
		"blackberry":  ThemeBlackberry,
		"tangerine":   ThemeTangerine,
		"fig":         ThemeFig,
		"guava":       ThemeGuava,
		"acai":        ThemeAcai,
		"coconut":     ThemeCoconut,
		"guarana":     ThemeGuarana,
	}
}

// Built-in theme presets. Use directly or as a Base for partial overrides.
var (
	ThemeDark = Theme{Colors: Colors{
		Accent: "63", AccentBright: "212", Muted: "240", Dim: "245", Success: "82", Danger: "196",
	}}
	ThemeLight = Theme{Colors: Colors{
		Accent: "#6D28D9", AccentBright: "#7C3AED", Muted: "#9CA3AF", Dim: "#D1D5DB", Success: "#059669", Danger: "#DC2626",
	}}
	ThemeDracula = Theme{Colors: Colors{
		Accent: "#BD93F9", AccentBright: "#FF79C6", Muted: "#6272A4", Dim: "#44475A", Success: "#50FA7B", Danger: "#FF5555",
	}}
	ThemeMonokai = Theme{Colors: Colors{
		Accent: "#AE81FF", AccentBright: "#E6DB74", Muted: "#75715E", Dim: "#3E3D32", Success: "#A6E22E", Danger: "#F92672",
	}}
	ThemeSolarized = Theme{Colors: Colors{
		Accent: "#268BD2", AccentBright: "#2AA198", Muted: "#586E75", Dim: "#657B83", Success: "#859900", Danger: "#DC322F",
	}}
	ThemeBanana = Theme{Colors: Colors{
		Accent: "#F4D03F", AccentBright: "#E6FF79", Muted: "#8D7B3A", Dim: "#5C4F20", Success: "#E6FF79", Danger: "#E74C3C",
	}}
	ThemeMint = Theme{Colors: Colors{
		Accent: "#3EB489", AccentBright: "#98DFAF", Muted: "#4A7B6F", Dim: "#2E4F46", Success: "#2ECC71", Danger: "#E74C3C",
	}}
	ThemeStrawberry = Theme{Colors: Colors{
		Accent: "#E83A59", AccentBright: "#FF7096", Muted: "#8B3A52", Dim: "#5C2035", Success: "#4CAF50", Danger: "#C0392B",
	}}
	ThemeBlueberry = Theme{Colors: Colors{
		Accent: "#6C63FF", AccentBright: "#A89CFF", Muted: "#4A4580", Dim: "#2E2A55", Success: "#4CAF50", Danger: "#E74C3C",
	}}
	ThemeMango = Theme{Colors: Colors{
		Accent: "#FF9F1C", AccentBright: "#FFCF77", Muted: "#9A6020", Dim: "#5C3A10", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeWatermelon = Theme{Colors: Colors{
		Accent: "#FF4D6D", AccentBright: "#FF8FA3", Muted: "#4A7C59", Dim: "#2D5240", Success: "#52B788", Danger: "#C9184A",
	}}
	ThemePeach = Theme{Colors: Colors{
		Accent: "#FF8B64", AccentBright: "#FFCBA4", Muted: "#9A6448", Dim: "#5C3A28", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeKiwi = Theme{Colors: Colors{
		Accent: "#8DB600", AccentBright: "#C5E84A", Muted: "#5A6E2A", Dim: "#384418", Success: "#C5E84A", Danger: "#E74C3C",
	}}
	ThemeLemon = Theme{Colors: Colors{
		Accent: "#FFE600", AccentBright: "#FFF176", Muted: "#9A8A20", Dim: "#5C5010", Success: "#8BC34A", Danger: "#E74C3C",
	}}
	ThemeOrange = Theme{Colors: Colors{
		Accent: "#FF6B00", AccentBright: "#FFA040", Muted: "#9A4A10", Dim: "#5C2C08", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeGrape = Theme{Colors: Colors{
		Accent: "#9B59B6", AccentBright: "#C39BD3", Muted: "#5C3A7A", Dim: "#3A2050", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeCherry = Theme{Colors: Colors{
		Accent: "#CC0000", AccentBright: "#FF6B9D", Muted: "#7A1A30", Dim: "#4A0A1A", Success: "#4CAF50", Danger: "#8B0000",
	}}
	ThemePineapple = Theme{Colors: Colors{
		Accent: "#FFD700", AccentBright: "#FFF44F", Muted: "#7A6A10", Dim: "#4A4010", Success: "#2E8B57", Danger: "#E74C3C",
	}}
	ThemeRaspberry = Theme{Colors: Colors{
		Accent: "#E91E8C", AccentBright: "#FF6EC7", Muted: "#8B1A5A", Dim: "#5C1038", Success: "#4CAF50", Danger: "#C2185B",
	}}
	ThemeLime = Theme{Colors: Colors{
		Accent: "#00C853", AccentBright: "#69FF47", Muted: "#2E6B30", Dim: "#1A4020", Success: "#69FF47", Danger: "#E74C3C",
	}}
	ThemePomegranate = Theme{Colors: Colors{
		Accent: "#96002D", AccentBright: "#FF1654", Muted: "#6B1020", Dim: "#3A0810", Success: "#C5E84A", Danger: "#FF1654",
	}}
	ThemeApple = Theme{Colors: Colors{
		Accent: "#FF3B30", AccentBright: "#FF9F0A", Muted: "#8B2020", Dim: "#4A1010", Success: "#34C759", Danger: "#FF3B30",
	}}
	ThemePlum = Theme{Colors: Colors{
		Accent: "#8E4585", AccentBright: "#C490BD", Muted: "#5A2A5A", Dim: "#361836", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeApricot = Theme{Colors: Colors{
		Accent: "#FBAE52", AccentBright: "#FDD5A0", Muted: "#9A6A30", Dim: "#5C3A18", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeDragonfruit = Theme{Colors: Colors{
		Accent: "#FF2D78", AccentBright: "#FF6EAE", Muted: "#8B1A5A", Dim: "#5C0A38", Success: "#4CAF50", Danger: "#E74C3C",
	}}
	ThemeBlackberry = Theme{Colors: Colors{
		Accent: "#5C3A6B", AccentBright: "#9B6FAE", Muted: "#3A1E4A", Dim: "#200A30", Success: "#4CAF50", Danger: "#E74C3C",
	}}
	ThemeTangerine = Theme{Colors: Colors{
		Accent: "#FF8C00", AccentBright: "#FFB347", Muted: "#9A5A10", Dim: "#5C3008", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeFig = Theme{Colors: Colors{
		Accent: "#7B3F6E", AccentBright: "#B07AAA", Muted: "#4A2048", Dim: "#2A0E30", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeGuava = Theme{Colors: Colors{
		Accent: "#FF6B8A", AccentBright: "#FFB3C1", Muted: "#8B3A50", Dim: "#5C1A30", Success: "#4CAF50", Danger: "#C0392B",
	}}
	ThemeAcai = Theme{Colors: Colors{
		Accent: "#4A1A6B", AccentBright: "#9B4FCC", Muted: "#3A1050", Dim: "#200830", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeCoconut = Theme{Colors: Colors{
		Accent: "#C4A882", AccentBright: "#EDD9B8", Muted: "#7A6048", Dim: "#4A3828", Success: "#5DBB63", Danger: "#E74C3C",
	}}
	ThemeGuarana = Theme{Colors: Colors{
		Accent: "#A83220", AccentBright: "#D4503C", Muted: "#5C2A1A", Dim: "#3A1408", Success: "#4A7C2F", Danger: "#C0392B",
	}}
)
