# Themes

This document explains how to configure `Config.Theme` in `editor.Config`.

---

## Built-in themes

Themes live in `github.com/lucasassuncao/bezel/theme`. `theme.All()` returns every built-in preset keyed by name - useful for a `--theme` CLI flag or a `--list-themes` command:

```go
for name, t := range theme.All() {
    fmt.Println(name)
}
```

| Name | Name | Name | Name |
|---|---|---|---|
| `plain` | `banana` | `mint` | `strawberry` |
| `blueberry` | `mango` | `watermelon` | `peach` |
| `kiwi` | `lemon` | `orange` | `grape` |
| `cherry` | `pineapple` | `raspberry` | `lime` |
| `pomegranate` | `apple` | `plum` | `apricot` |
| `dragonfruit` | `blackberry` | `tangerine` | `fig` |
| `guava` | `acai` | `coconut` | `guarana` |
| `melon` | `farzenith` | `banuk` | `nora` |
| `carja` | `oseram` | `utaru` | `tenakth` |
| `quen` | `mario` | `luigi` | `princesspeach` |
| `daisy` | `yoshi` | `toad` | `rosalina` |
| `toadette` | `wario` | `waluigi` | `bowser` |
| `sonic` | `tails` | `knuckles` | `shadow` |
| `amyrose` | `cream` | `rouge` | `eggman` |

`default` (`theme.ThemeDefault`) is what a zero `Config.Theme` resolves to: an adaptive palette with one value per role for light terminals and one for dark ones, picked from the terminal's answer when the editor starts. `plain` (`theme.ThemePlain`) uses only ANSI 16-color codes (`"4"`, `"6"`, `"8"`, `"2"`, `"1"`) instead of hex/256-color values, for terminals with limited color support.

Note: the Super Mario character is `theme.ThemePrincessPeach` / `"princesspeach"`, not `"peach"` - that name is already taken by the fruit preset.

### Grouped listing

`theme.Categories()` returns the same built-in themes organized into named groups, for a `--list-themes` that wants headings instead of one flat list:

```go
for _, cat := range theme.Categories() {
    fmt.Println(cat.Name + ":")
    for _, name := range cat.Themes {
        fmt.Println("  " + name)
    }
}
```

`plain` has no siblings of its own, so it lives under a "Miscellaneous" category rather than being left ungrouped. A test (`TestThemeRegistryHasNoDuplicateNames`) guards against a theme name being listed in two categories at once.

### Interactive browser

`themebrowser.BrowseInTerminal(t ...theme.Theme)` renders an inline (not full-screen) scrollable table (`↑`/`↓` to navigate, `q`/`esc`/`ctrl+c` to quit) listing every built-in theme name next to its `theme.Categories()` category. Wire it directly to a host CLI's `--list-themes` flag instead of printing plain text:

```go
import "github.com/lucasassuncao/bezel/themebrowser"

themebrowser.BrowseInTerminal()
```

```go
editor.Run(editor.Config{
    Schema: &MyConfig{},
    Theme:  theme.ThemeGrape,
})
```

## Structure

A `Theme` is a three-layer appearance configuration:

```go
type Theme struct {
    Base   *Theme // optional preset to inherit from (nil → the adaptive default)
    Colors Colors // per-field overrides applied on top of Base.Colors
    Styles Styles // lipgloss overrides applied on top of derived defaults
}

type Colors struct {
    Accent    string // focused borders, section headings, legend keys
    Selection string // the cursor row, the focused panel's title
    Border    string // unfocused borders, status and hint text
    Dim       string // secondary text, items not yet in the document
    Text      string // body text
    Success   string // items present in the document
    Warning   string // drafts, soft failures
    Danger    string // validation errors, unknown keys
    Info      string // counts, badges
    OnAccent  string // text drawn on a filled background
}

type Styles struct {
    Cursor *lipgloss.Style
    Muted  *lipgloss.Style
    Danger *lipgloss.Style
}
```

Each `Colors` field accepts a hex value (`"#7C3AED"`) or an ANSI 256-color code (`"63"`). An empty string means "inherit from `Base`", then the adaptive default, which has one value for a light terminal and one for a dark one. The editor asks the terminal which it is when it starts.

## Custom theme via partial override

Start from a built-in preset and override only what you need:

```go
myTheme := theme.Theme{
    Base: &theme.ThemeGrape,
    Colors: theme.Colors{
        Selection: "#FFB86C", // orange instead of Grape's default
    },
}

editor.Run(editor.Config{
    Schema: &MyConfig{},
    Theme:  myTheme,
})
```

## Custom theme from scratch

Set every `Colors` field directly, with no `Base` (any field left empty falls back to the adaptive default):

```go
myTheme := theme.Theme{
    Colors: theme.Colors{
        Accent:    "#00FF00",
        Selection: "#FFFF00",
        Border:    "#888888",
        Dim:       "#666666",
        Success:   "#00FFFF",
        Danger:    "#FF0000",
    },
}
```

## Resolving colors outside the editor

`theme.ResolveColors(t, dark)` merges a `Theme` down to a concrete `Colors` value without importing `editor` - useful when building a companion TUI that should match the host app's theme:

```go
colors := theme.ResolveColors(myTheme, theme.DarkTerminal())
```

## Low-color terminals

There is no `NO_COLOR` switch. Use `ThemePlain` for terminals with limited color support: it is built entirely from ANSI 16-color codes rather than hex or 256-color values, so the terminal's own palette controls how it renders.
