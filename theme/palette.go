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

// Palette - narrow on purpose. Clients can extend it with their own colours;
// add to this list only when at least two yedit components need it.
var (
	Accent       = colorVal("63")  // blue - active borders, primary highlight
	AccentBright = colorVal("212") // pink - titles, selection
	Muted        = colorVal("240") // grey - inactive borders, status hints
	Dim          = colorVal("245") // light grey - secondary text
	Success      = colorVal("82")  // green - existing/added items, success alerts
	Warning      = colorVal("220") // yellow - save-with-warnings alerts
	Danger       = colorVal("196") // red - error alerts
)

// Common item styles. Each TUI is free to compose its own variants on top.
var (
	StatusBar = lipgloss.NewStyle().Foreground(Muted).PaddingLeft(1)
)

// Colors holds the six palette values that drive all editor styling.
// Each field is a lipgloss-compatible color string: a hex value ("#7C3AED"),
// an ANSI 256-color code ("63"), or a named terminal color.
// Empty string means "inherit from Base" during theme resolution.
type Colors struct {
	ActiveBorderColor   string // focused panel borders, section labels, hint key text
	SelectionColor      string // selected cursor item, active panel title
	InactiveBorderColor string // unfocused panel borders, status bar text
	AvailableItemColor  string // items not yet added to the document, secondary text
	ExistingItemColor   string // items already present in the YAML document
	ErrorColor          string // validation errors, unknown keys
}

// Styles holds optional per-element lipgloss overrides. Nil fields are ignored
// during theme resolution and the default derived from Colors is used instead.
type Styles struct {
	CursorLine *lipgloss.Style
	HintText   *lipgloss.Style
	ErrorText  *lipgloss.Style
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

// ResolveColors merges t into a concrete Colors value, starting from ThemeDark
// as the default base. Use this when building a TUI that needs concrete color
// values without importing the editor package.
func ResolveColors(t Theme) Colors {
	c := ThemeDark.Colors
	if t.Base != nil {
		c = mergeColors(c, t.Base.Colors)
	}
	return mergeColors(c, t.Colors)
}

func mergeColors(base, over Colors) Colors {
	if over.ActiveBorderColor != "" {
		base.ActiveBorderColor = over.ActiveBorderColor
	}
	if over.SelectionColor != "" {
		base.SelectionColor = over.SelectionColor
	}
	if over.InactiveBorderColor != "" {
		base.InactiveBorderColor = over.InactiveBorderColor
	}
	if over.AvailableItemColor != "" {
		base.AvailableItemColor = over.AvailableItemColor
	}
	if over.ExistingItemColor != "" {
		base.ExistingItemColor = over.ExistingItemColor
	}
	if over.ErrorColor != "" {
		base.ErrorColor = over.ErrorColor
	}
	return base
}

// All returns all built-in theme presets keyed by their CLI name.
// Useful for --theme flag validation and --list-themes output in host CLIs.
func All() map[string]Theme {
	return map[string]Theme{
		"plain":       ThemePlain,
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
		ActiveBorderColor: "63", SelectionColor: "212", InactiveBorderColor: "240", AvailableItemColor: "245", ExistingItemColor: "82", ErrorColor: "196",
	}}
	ThemeLight = Theme{Colors: Colors{
		ActiveBorderColor: "#6D28D9", SelectionColor: "#7C3AED", InactiveBorderColor: "#9CA3AF", AvailableItemColor: "#D1D5DB", ExistingItemColor: "#059669", ErrorColor: "#DC2626",
	}}
	ThemeDracula = Theme{Colors: Colors{
		ActiveBorderColor: "#BD93F9", SelectionColor: "#FF79C6", InactiveBorderColor: "#6272A4", AvailableItemColor: "#44475A", ExistingItemColor: "#50FA7B", ErrorColor: "#FF5555",
	}}
	ThemeMonokai = Theme{Colors: Colors{
		ActiveBorderColor: "#AE81FF", SelectionColor: "#E6DB74", InactiveBorderColor: "#75715E", AvailableItemColor: "#3E3D32", ExistingItemColor: "#A6E22E", ErrorColor: "#F92672",
	}}
	ThemeSolarized = Theme{Colors: Colors{
		ActiveBorderColor: "#268BD2", SelectionColor: "#2AA198", InactiveBorderColor: "#586E75", AvailableItemColor: "#657B83", ExistingItemColor: "#859900", ErrorColor: "#DC322F",
	}}
	ThemeBanana = Theme{Colors: Colors{
		ActiveBorderColor: "#F4D03F", SelectionColor: "#E6FF79", InactiveBorderColor: "#8D7B3A", AvailableItemColor: "#5C4F20", ExistingItemColor: "#E6FF79", ErrorColor: "#E74C3C",
	}}
	ThemeMint = Theme{Colors: Colors{
		ActiveBorderColor: "#3EB489", SelectionColor: "#98DFAF", InactiveBorderColor: "#4A7B6F", AvailableItemColor: "#2E4F46", ExistingItemColor: "#2ECC71", ErrorColor: "#E74C3C",
	}}
	ThemeStrawberry = Theme{Colors: Colors{
		ActiveBorderColor: "#E83A59", SelectionColor: "#FF7096", InactiveBorderColor: "#8B3A52", AvailableItemColor: "#5C2035", ExistingItemColor: "#4CAF50", ErrorColor: "#C0392B",
	}}
	ThemeBlueberry = Theme{Colors: Colors{
		ActiveBorderColor: "#6C63FF", SelectionColor: "#A89CFF", InactiveBorderColor: "#4A4580", AvailableItemColor: "#2E2A55", ExistingItemColor: "#4CAF50", ErrorColor: "#E74C3C",
	}}
	ThemeMango = Theme{Colors: Colors{
		ActiveBorderColor: "#FF9F1C", SelectionColor: "#FFCF77", InactiveBorderColor: "#9A6020", AvailableItemColor: "#5C3A10", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeWatermelon = Theme{Colors: Colors{
		ActiveBorderColor: "#FF4D6D", SelectionColor: "#FF8FA3", InactiveBorderColor: "#4A7C59", AvailableItemColor: "#2D5240", ExistingItemColor: "#52B788", ErrorColor: "#C9184A",
	}}
	ThemePeach = Theme{Colors: Colors{
		ActiveBorderColor: "#FF8B64", SelectionColor: "#FFCBA4", InactiveBorderColor: "#9A6448", AvailableItemColor: "#5C3A28", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeKiwi = Theme{Colors: Colors{
		ActiveBorderColor: "#8DB600", SelectionColor: "#C5E84A", InactiveBorderColor: "#5A6E2A", AvailableItemColor: "#384418", ExistingItemColor: "#C5E84A", ErrorColor: "#E74C3C",
	}}
	ThemeLemon = Theme{Colors: Colors{
		ActiveBorderColor: "#FFE600", SelectionColor: "#FFF176", InactiveBorderColor: "#9A8A20", AvailableItemColor: "#5C5010", ExistingItemColor: "#8BC34A", ErrorColor: "#E74C3C",
	}}
	ThemeOrange = Theme{Colors: Colors{
		ActiveBorderColor: "#FF6B00", SelectionColor: "#FFA040", InactiveBorderColor: "#9A4A10", AvailableItemColor: "#5C2C08", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeGrape = Theme{Colors: Colors{
		ActiveBorderColor: "#9B59B6", SelectionColor: "#C39BD3", InactiveBorderColor: "#5C3A7A", AvailableItemColor: "#3A2050", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeCherry = Theme{Colors: Colors{
		ActiveBorderColor: "#CC0000", SelectionColor: "#FF6B9D", InactiveBorderColor: "#7A1A30", AvailableItemColor: "#4A0A1A", ExistingItemColor: "#4CAF50", ErrorColor: "#8B0000",
	}}
	ThemePineapple = Theme{Colors: Colors{
		ActiveBorderColor: "#FFD700", SelectionColor: "#FFF44F", InactiveBorderColor: "#7A6A10", AvailableItemColor: "#4A4010", ExistingItemColor: "#2E8B57", ErrorColor: "#E74C3C",
	}}
	ThemeRaspberry = Theme{Colors: Colors{
		ActiveBorderColor: "#E91E8C", SelectionColor: "#FF6EC7", InactiveBorderColor: "#8B1A5A", AvailableItemColor: "#5C1038", ExistingItemColor: "#4CAF50", ErrorColor: "#C2185B",
	}}
	ThemeLime = Theme{Colors: Colors{
		ActiveBorderColor: "#00C853", SelectionColor: "#69FF47", InactiveBorderColor: "#2E6B30", AvailableItemColor: "#1A4020", ExistingItemColor: "#69FF47", ErrorColor: "#E74C3C",
	}}
	ThemePomegranate = Theme{Colors: Colors{
		ActiveBorderColor: "#96002D", SelectionColor: "#FF1654", InactiveBorderColor: "#6B1020", AvailableItemColor: "#3A0810", ExistingItemColor: "#C5E84A", ErrorColor: "#FF1654",
	}}
	ThemeApple = Theme{Colors: Colors{
		ActiveBorderColor: "#FF3B30", SelectionColor: "#FF9F0A", InactiveBorderColor: "#8B2020", AvailableItemColor: "#4A1010", ExistingItemColor: "#34C759", ErrorColor: "#FF3B30",
	}}
	ThemePlum = Theme{Colors: Colors{
		ActiveBorderColor: "#8E4585", SelectionColor: "#C490BD", InactiveBorderColor: "#5A2A5A", AvailableItemColor: "#361836", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeApricot = Theme{Colors: Colors{
		ActiveBorderColor: "#FBAE52", SelectionColor: "#FDD5A0", InactiveBorderColor: "#9A6A30", AvailableItemColor: "#5C3A18", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeDragonfruit = Theme{Colors: Colors{
		ActiveBorderColor: "#FF2D78", SelectionColor: "#FF6EAE", InactiveBorderColor: "#8B1A5A", AvailableItemColor: "#5C0A38", ExistingItemColor: "#4CAF50", ErrorColor: "#E74C3C",
	}}
	ThemeBlackberry = Theme{Colors: Colors{
		ActiveBorderColor: "#5C3A6B", SelectionColor: "#9B6FAE", InactiveBorderColor: "#3A1E4A", AvailableItemColor: "#200A30", ExistingItemColor: "#4CAF50", ErrorColor: "#E74C3C",
	}}
	ThemeTangerine = Theme{Colors: Colors{
		ActiveBorderColor: "#FF8C00", SelectionColor: "#FFB347", InactiveBorderColor: "#9A5A10", AvailableItemColor: "#5C3008", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeFig = Theme{Colors: Colors{
		ActiveBorderColor: "#7B3F6E", SelectionColor: "#B07AAA", InactiveBorderColor: "#4A2048", AvailableItemColor: "#2A0E30", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeGuava = Theme{Colors: Colors{
		ActiveBorderColor: "#FF6B8A", SelectionColor: "#FFB3C1", InactiveBorderColor: "#8B3A50", AvailableItemColor: "#5C1A30", ExistingItemColor: "#4CAF50", ErrorColor: "#C0392B",
	}}
	ThemeAcai = Theme{Colors: Colors{
		ActiveBorderColor: "#4A1A6B", SelectionColor: "#9B4FCC", InactiveBorderColor: "#3A1050", AvailableItemColor: "#200830", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemeCoconut = Theme{Colors: Colors{
		ActiveBorderColor: "#C4A882", SelectionColor: "#EDD9B8", InactiveBorderColor: "#7A6048", AvailableItemColor: "#4A3828", ExistingItemColor: "#5DBB63", ErrorColor: "#E74C3C",
	}}
	ThemePlain = Theme{Colors: Colors{
		ActiveBorderColor:   "4", // ANSI blue
		SelectionColor:      "6", // ANSI cyan
		InactiveBorderColor: "8", // ANSI dark grey
		AvailableItemColor:  "8", // ANSI dark grey
		ExistingItemColor:   "2", // ANSI green
		ErrorColor:          "1", // ANSI red
	}}
	ThemeGuarana = Theme{Colors: Colors{
		ActiveBorderColor: "#A83220", SelectionColor: "#D4503C", InactiveBorderColor: "#5C2A1A", AvailableItemColor: "#3A1408", ExistingItemColor: "#4A7C2F", ErrorColor: "#C0392B",
	}}
)
