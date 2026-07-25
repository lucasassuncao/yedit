// Package theme provides the palette, base lipgloss styles, and shared
// layout primitives used across yedit-built TUIs.
package theme

import (
	"charm.land/lipgloss/v2"
)

// Palette - narrow on purpose. Clients can extend it with their own colours;
// add to this list only when at least two yedit components need it.
var (
	Accent       = lipgloss.Color("63")  // blue - active borders, primary highlight
	AccentBright = lipgloss.Color("212") // pink - titles, selection
	Muted        = lipgloss.Color("240") // grey - inactive borders, status hints
	Dim          = lipgloss.Color("245") // light grey - secondary text
	Success      = lipgloss.Color("82")  // green - existing/added items, success alerts
	Warning      = lipgloss.Color("220") // yellow - save-with-warnings alerts
	Danger       = lipgloss.Color("196") // red - error alerts
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
//   - Base: an optional preset to inherit from (nil → ThemePlain)
//   - Colors: per-field overrides applied on top of Base.Colors
//   - Styles: lipgloss overrides applied on top of derived defaults
type Theme struct {
	Base   *Theme
	Colors Colors
	Styles Styles
}

// ResolveColors merges t into a concrete Colors value, starting from ThemePlain
// as the default base. Use this when building a TUI that needs concrete color
// values without importing the editor package.
func ResolveColors(t Theme) Colors {
	c := ThemePlain.Colors
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

// namedTheme pairs a theme's CLI name with its value - the single entry unit
// shared by both All() and Categories(), so the two can never drift apart.
type namedTheme struct {
	name  string
	theme Theme
}

// themeRegistry is the one place a built-in theme is listed: both its CLI
// name and its category. All() and Categories() are just two different
// projections of this slice, built with a for loop - add a theme here once
// and it shows up correctly in both.
var themeRegistry = []struct {
	category string
	themes   []namedTheme
}{
	{"Miscellaneous", []namedTheme{
		{"plain", ThemePlain},
	}},
	{"Fruit", []namedTheme{
		{"banana", ThemeBanana}, {"mint", ThemeMint}, {"strawberry", ThemeStrawberry},
		{"blueberry", ThemeBlueberry}, {"mango", ThemeMango}, {"watermelon", ThemeWatermelon},
		{"peach", ThemePeach}, {"kiwi", ThemeKiwi}, {"lemon", ThemeLemon},
		{"orange", ThemeOrange}, {"grape", ThemeGrape}, {"cherry", ThemeCherry},
		{"pineapple", ThemePineapple}, {"raspberry", ThemeRaspberry}, {"lime", ThemeLime},
		{"pomegranate", ThemePomegranate}, {"apple", ThemeApple}, {"plum", ThemePlum},
		{"apricot", ThemeApricot}, {"dragonfruit", ThemeDragonfruit}, {"blackberry", ThemeBlackberry},
		{"tangerine", ThemeTangerine}, {"fig", ThemeFig}, {"guava", ThemeGuava},
		{"acai", ThemeAcai}, {"coconut", ThemeCoconut}, {"guarana", ThemeGuarana},
		{"melon", ThemeMelon},
	}},
	{"Horizon", []namedTheme{
		{"farzenith", ThemeFarZenith}, {"banuk", ThemeBanuk}, {"nora", ThemeNora},
		{"carja", ThemeCarja}, {"oseram", ThemeOseram}, {"utaru", ThemeUtaru},
		{"tenakth", ThemeTenakth}, {"quen", ThemeQuen},
	}},
	{"Super Mario", []namedTheme{
		{"mario", ThemeMario}, {"luigi", ThemeLuigi}, {"princesspeach", ThemePrincessPeach},
		{"daisy", ThemeDaisy}, {"yoshi", ThemeYoshi}, {"toad", ThemeToad},
		{"rosalina", ThemeRosalina}, {"toadette", ThemeToadette}, {"wario", ThemeWario},
		{"waluigi", ThemeWaluigi}, {"bowser", ThemeBowser},
	}},
	{"Sonic", []namedTheme{
		{"sonic", ThemeSonic}, {"tails", ThemeTails}, {"knuckles", ThemeKnuckles},
		{"shadow", ThemeShadow}, {"amyrose", ThemeAmyRose}, {"cream", ThemeCream},
		{"rouge", ThemeRouge}, {"eggman", ThemeEggman},
	}},
}

// allNamedThemes flattens themeRegistry into a single slice, dropping the
// category grouping - the projection All() needs.
func allNamedThemes() []namedTheme {
	var out []namedTheme
	for _, cat := range themeRegistry {
		out = append(out, cat.themes...)
	}
	return out
}

// namesOf extracts just the names from a slice of namedTheme, in order - the
// projection Categories() needs for each group.
func namesOf(nts []namedTheme) []string {
	names := make([]string, len(nts))
	for i, nt := range nts {
		names[i] = nt.name
	}
	return names
}

// All returns all built-in theme presets keyed by their CLI name.
// Useful for --theme flag validation and --list-themes output in host CLIs.
func All() map[string]Theme {
	m := make(map[string]Theme)
	for _, nt := range allNamedThemes() {
		m[nt.name] = nt.theme
	}
	return m
}

// Category groups a set of related built-in theme names for display purposes -
// e.g. a --list-themes command that wants headings instead of one flat list.
type Category struct {
	Name   string
	Themes []string // names, in display order; each is also a key in All()
}

// Categories returns the built-in themes grouped for display. Every theme
// in All() belongs to exactly one category - "plain" has no siblings of its
// own, so it lives under "Miscellaneous" rather than being left ungrouped.
func Categories() []Category {
	cats := make([]Category, len(themeRegistry))
	for i, cat := range themeRegistry {
		cats[i] = Category{Name: cat.category, Themes: namesOf(cat.themes)}
	}
	return cats
}

// Built-in theme presets. Use directly or as a Base for partial overrides.
var (
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
	// ThemeMelon: cantaloupe - salmon-orange flesh with the netted rind's sage
	// green on the borders. The green is deliberate: peach, apricot, and mango
	// all pair an orange accent with brown borders, so a fourth orange fruit
	// needs the rind to tell itself apart from them at a glance.
	ThemeMelon = Theme{Colors: Colors{
		ActiveBorderColor: "#E8845A", SelectionColor: "#F7D9A8", InactiveBorderColor: "#8A9A6B", AvailableItemColor: "#5A6B45", ExistingItemColor: "#7BB661", ErrorColor: "#C0392B",
	}}
	// ThemeFarZenith: white-and-gold, after the Far Zenith enclave's polished
	// ivory architecture and bronze trim in Horizon Forbidden West. Unfocused
	// panels border in white (the dominant hull color); focus and error trim
	// stay gold/rust, so gold reads as an accent, not the base.
	ThemeFarZenith = Theme{Colors: Colors{
		ActiveBorderColor: "#D4AF37", SelectionColor: "#FFFFFF", InactiveBorderColor: "#FFFFFF", AvailableItemColor: "#8A8368", ExistingItemColor: "#8A9A5B", ErrorColor: "#B7472A",
	}}
	// ThemeBanuk: the Banuk's "Blue Light" - a neon cyan glow (the cables
	// shamans thread through their skin to channel it) against dark
	// weathered hide and machine-metal tones.
	ThemeBanuk = Theme{Colors: Colors{
		ActiveBorderColor: "#00D9FF", SelectionColor: "#7DF9FF", InactiveBorderColor: "#3E4A52", AvailableItemColor: "#5A6670", ExistingItemColor: "#3ED9B0", ErrorColor: "#FF4655",
	}}
	// ThemeNora: earthy hide-and-forest tones, with the Nora's blue woad
	// face paint as the one cool accent against greens and browns.
	ThemeNora = Theme{Colors: Colors{
		ActiveBorderColor: "#3F6B3F", SelectionColor: "#4FB8D0", InactiveBorderColor: "#6B5A45", AvailableItemColor: "#4A3C2E", ExistingItemColor: "#6B8E4E", ErrorColor: "#B33A3A",
	}}
	// ThemeCarja: the sun-worshipping Carja's royal crimson and gold, fire
	// and light against a dark ember base.
	ThemeCarja = Theme{Colors: Colors{
		ActiveBorderColor: "#C81E3A", SelectionColor: "#F4A825", InactiveBorderColor: "#7A3B3B", AvailableItemColor: "#4A2020", ExistingItemColor: "#5DBB63", ErrorColor: "#8B0000",
	}}
	// ThemeOseram: forged metal and rust - no face paint, no ornamentation,
	// just iron, ember-orange heat, and industrial grey.
	ThemeOseram = Theme{Colors: Colors{
		ActiveBorderColor: "#B35A2A", SelectionColor: "#FF8C42", InactiveBorderColor: "#4A4A48", AvailableItemColor: "#3A3A38", ExistingItemColor: "#5DBB63", ErrorColor: "#C0392B",
	}}
	// ThemeUtaru: woven-leaf green (their armor's dominant color) over
	// mustard sashes and tan straps, with the Utaru's white face paint as
	// the bright accent.
	ThemeUtaru = Theme{Colors: Colors{
		ActiveBorderColor: "#5C8A3A", SelectionColor: "#F5F0E0", InactiveBorderColor: "#B8860B", AvailableItemColor: "#6B5A35", ExistingItemColor: "#D4A017", ErrorColor: "#A63D2A",
	}}
	// ThemeTenakth: warrior red armor against an ember base, with the cool
	// blue body paint of some clans as the one calm accent. InactiveBorderColor
	// and AvailableItemColor double as real foreground text elsewhere (status
	// bar, unchecked/passthrough items, the preview gutter) - not just border
	// decoration - so they stay muted rather than near-black to keep that
	// text legible on a black terminal background.
	ThemeTenakth = Theme{Colors: Colors{
		ActiveBorderColor: "#A6231F", SelectionColor: "#3E9BC7", InactiveBorderColor: "#8A5A4A", AvailableItemColor: "#6B4A3A", ExistingItemColor: "#5C7A3A", ErrorColor: "#C0392B",
	}}
	// ThemeQuen: the coastal Quen's turquoise and coral face paint against
	// blue-grey sea mist.
	ThemeQuen = Theme{Colors: Colors{
		ActiveBorderColor: "#1FA8A0", SelectionColor: "#FF7F66", InactiveBorderColor: "#5C7A82", AvailableItemColor: "#3E525A", ExistingItemColor: "#3EBD93", ErrorColor: "#C0392B",
	}}
	// ThemeMario: cap-and-shirt red, white gloves as the bright accent, his
	// overalls blue demoted to unfocused borders.
	ThemeMario = Theme{Colors: Colors{
		ActiveBorderColor: "#E52521", SelectionColor: "#F0F0E8", InactiveBorderColor: "#049CD8", AvailableItemColor: "#7A6552", ExistingItemColor: "#43B047", ErrorColor: "#A61B1B",
	}}
	// ThemeLuigi: his green over denim-overalls blue (the same vivid blue as
	// Mario's - previously too desaturated here and just read as grey),
	// white gloves as the bright accent.
	ThemeLuigi = Theme{Colors: Colors{
		ActiveBorderColor: "#43B047", SelectionColor: "#F0F0E8", InactiveBorderColor: "#049CD8", AvailableItemColor: "#4A6B85", ExistingItemColor: "#8BC34A", ErrorColor: "#C0392B",
	}}
	// ThemePrincessPeach: dress pink, white gloves as the bright accent, her
	// golden hair as the second vivid color (mirrors the Mario/Luigi pattern:
	// main color + white gloves + a secondary vivid color, not a muted one).
	// Named PrincessPeach, not Peach, to avoid colliding with the fruit
	// preset of the same name.
	ThemePrincessPeach = Theme{Colors: Colors{
		ActiveBorderColor: "#F06CA0", SelectionColor: "#F5F5F0", InactiveBorderColor: "#F0C419", AvailableItemColor: "#B5527A", ExistingItemColor: "#5DBB63", ErrorColor: "#C0392B",
	}}
	// ThemeDaisy: her dress orange (predominant) over her brown hair
	// (secondary); white sleeve, teal brooch gem, and a deeper orange trim
	// fill the rest, all pulled from the reference art rather than invented.
	ThemeDaisy = Theme{Colors: Colors{
		ActiveBorderColor: "#F39C12", SelectionColor: "#F5F0E8", InactiveBorderColor: "#8A5A3A", AvailableItemColor: "#B8791E", ExistingItemColor: "#2E9C9C", ErrorColor: "#C0392B",
	}}
	// ThemeYoshi: his green body (predominant) over his white belly
	// (secondary); orange boots, red mouth, and cream spikes fill the rest.
	ThemeYoshi = Theme{Colors: Colors{
		ActiveBorderColor: "#3CB043", SelectionColor: "#FF8C1A", InactiveBorderColor: "#F5F0E8", AvailableItemColor: "#B8A888", ExistingItemColor: "#D64545", ErrorColor: "#8B0000",
	}}
	// ThemeToad: white cap/body (predominant) over his cap's red spot
	// (secondary); blue vest, gold trim, and brown shoes fill the rest.
	ThemeToad = Theme{Colors: Colors{
		ActiveBorderColor: "#F5F0E8", SelectionColor: "#2E6DA4", InactiveBorderColor: "#E52521", AvailableItemColor: "#8A6248", ExistingItemColor: "#F0C419", ErrorColor: "#8B0000",
	}}
	// ThemeRosalina: her cosmic teal gown (predominant) over her golden hair
	// (secondary); a silver crown, blue eyes, and a pink gem fill the rest.
	ThemeRosalina = Theme{Colors: Colors{
		ActiveBorderColor: "#4FB8C0", SelectionColor: "#E8E8EC", InactiveBorderColor: "#D4AF37", AvailableItemColor: "#7A9BA0", ExistingItemColor: "#4A9FD8", ErrorColor: "#D6396B",
	}}
	// ThemeToadette: her pink cap/dress (predominant) over her cap's white
	// spot (secondary); red vest trim, gold trim, and brown shoes fill the
	// rest.
	ThemeToadette = Theme{Colors: Colors{
		ActiveBorderColor: "#E85AA0", SelectionColor: "#D62839", InactiveBorderColor: "#F5F0EC", AvailableItemColor: "#8A6248", ExistingItemColor: "#F0C419", ErrorColor: "#B33A5C",
	}}
	// ThemeWario: his yellow shirt/cap (predominant) over his purple
	// overalls (secondary); white gloves, green shoes, and his pink nose
	// fill the rest.
	ThemeWario = Theme{Colors: Colors{
		ActiveBorderColor: "#F0C419", SelectionColor: "#F5F0E8", InactiveBorderColor: "#7B2D8E", AvailableItemColor: "#B87A6E", ExistingItemColor: "#3E8E41", ErrorColor: "#B33A3A",
	}}
	// ThemeWaluigi: his purple shirt/cap (predominant) over his dark navy
	// overalls (secondary); white gloves, a gold "L", orange shoes, and his
	// pink nose fill the rest.
	ThemeWaluigi = Theme{Colors: Colors{
		ActiveBorderColor: "#7B2D8E", SelectionColor: "#F5F0E8", InactiveBorderColor: "#42425E", AvailableItemColor: "#8A5A2E", ExistingItemColor: "#F0C419", ErrorColor: "#B33A5C",
	}}
	// ThemeBowser: his orange-tan hide (predominant) over his green shell
	// (secondary); red-orange spikes, a muted hide tone, cream claws, and
	// dark red danger fill the rest.
	ThemeBowser = Theme{Colors: Colors{
		ActiveBorderColor: "#E8A33D", SelectionColor: "#D2691E", InactiveBorderColor: "#4A7A3A", AvailableItemColor: "#B98A55", ExistingItemColor: "#F0E6D2", ErrorColor: "#8B0000",
	}}
	// ThemeSonic: his blue fur (predominant) over his tan muzzle/belly
	// (secondary); red shoes, a gold shoe buckle, and green eyes fill the rest.
	ThemeSonic = Theme{Colors: Colors{
		ActiveBorderColor: "#1A50BC", SelectionColor: "#E8291F", InactiveBorderColor: "#FFD78F", AvailableItemColor: "#C9A227", ExistingItemColor: "#00A845", ErrorColor: "#8B0000",
	}}
	// ThemeTails: his orange fur (predominant) over his white belly/tail-tips
	// (secondary); red shoes and blue eyes fill the rest.
	ThemeTails = Theme{Colors: Colors{
		ActiveBorderColor: "#F1B000", SelectionColor: "#E8291F", InactiveBorderColor: "#F5F0E8", AvailableItemColor: "#B8830A", ExistingItemColor: "#0FB3F0", ErrorColor: "#8B0000",
	}}
	// ThemeKnuckles: his red fur (predominant) over his peach muzzle/chest
	// (secondary); his purple eyes and his shoe's grey, green, and yellow
	// parts fill the rest.
	ThemeKnuckles = Theme{Colors: Colors{
		ActiveBorderColor: "#FF1400", SelectionColor: "#5F3FAA", InactiveBorderColor: "#FFDDA0", AvailableItemColor: "#8A8A8A", ExistingItemColor: "#01AA33", ErrorColor: "#8B0000",
	}}
	// ThemeShadow: his signature red quill stripes (predominant for UI
	// purposes) over his black fur (secondary, rendered as dark charcoal -
	// true black would vanish against the terminal's black background);
	// his white chest tuft and a gold rocket-shoe accent fill the rest.
	ThemeShadow = Theme{Colors: Colors{
		ActiveBorderColor: "#DC0000", SelectionColor: "#F5F0E8", InactiveBorderColor: "#48484C", AvailableItemColor: "#5A5A5C", ExistingItemColor: "#FFB528", ErrorColor: "#8B0000",
	}}
	// ThemeAmyRose: her pink fur (predominant) over her red dress/shoes
	// (secondary); white gloves, her gold bracelets, and green eyes fill
	// the rest.
	ThemeAmyRose = Theme{Colors: Colors{
		ActiveBorderColor: "#FD95C6", SelectionColor: "#F5F0E8", InactiveBorderColor: "#D10000", AvailableItemColor: "#C9A227", ExistingItemColor: "#01A900", ErrorColor: "#8B0000",
	}}
	// ThemeCream: her buff fur (predominant) over her orange dress
	// (secondary); her pink inner ears, brown eyes, and white muzzle fill
	// the rest.
	ThemeCream = Theme{Colors: Colors{
		ActiveBorderColor: "#F5DFA0", SelectionColor: "#F0A8C0", InactiveBorderColor: "#E8821A", AvailableItemColor: "#6B4A2E", ExistingItemColor: "#F5F0E8", ErrorColor: "#C0392B",
	}}
	// ThemeRouge: her white fur (predominant) over her black catsuit
	// (secondary, rendered as dark charcoal for the same reason as
	// Shadow's fur); her pink heart chest plate, tan skin, and teal-green
	// eyes fill the rest.
	ThemeRouge = Theme{Colors: Colors{
		ActiveBorderColor: "#F5F0E8", SelectionColor: "#E85AA0", InactiveBorderColor: "#48484C", AvailableItemColor: "#B89868", ExistingItemColor: "#2E9C7A", ErrorColor: "#B33A3A",
	}}
	// ThemeEggman: his red coat (predominant) over his black pants/boots
	// (secondary, rendered as dark charcoal for the same reason as
	// Shadow's fur); his orange mustache, gold coat buttons, and white
	// gloves fill the rest.
	ThemeEggman = Theme{Colors: Colors{
		ActiveBorderColor: "#CC2936", SelectionColor: "#D2691E", InactiveBorderColor: "#48484C", AvailableItemColor: "#C9A227", ExistingItemColor: "#F0E6D2", ErrorColor: "#8B0000",
	}}
)
