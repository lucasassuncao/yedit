# Themes

This document explains how to configure `Config.Theme` in `editor.Config`.

---

## Built-in themes

`theme.All()` returns every built-in preset keyed by name - useful for a `--theme` CLI flag or a `--list-themes` command:

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

`plain` (`theme.ThemePlain`) is the default when `Config.Theme` is left at its zero value. It uses only ANSI 16-color codes (`"4"`, `"6"`, `"8"`, `"2"`, `"1"`) instead of hex/256-color values, for terminals with limited color support.

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
import "github.com/lucasassuncao/yedit/themebrowser"

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
    Base   *Theme // optional preset to inherit from (nil → ThemePlain)
    Colors Colors // per-field overrides applied on top of Base.Colors
    Styles Styles // lipgloss overrides applied on top of derived defaults
}

type Colors struct {
    ActiveBorderColor   string // focused panel borders, section labels, hint key text
    SelectionColor      string // selected cursor item, active panel title
    InactiveBorderColor string // unfocused panel borders, status bar text
    AvailableItemColor  string // items not yet added to the document, secondary text
    ExistingItemColor   string // items already present in the YAML document
    ErrorColor          string // validation errors, unknown keys
}

type Styles struct {
    CursorLine *lipgloss.Style
    HintText   *lipgloss.Style
    ErrorText  *lipgloss.Style
}
```

Each `Colors` field accepts a hex value (`"#7C3AED"`), an ANSI 256-color code (`"63"`), or a named terminal color. An empty string means "inherit from `Base`" during resolution.

## Custom theme via partial override

Start from a built-in preset and override only what you need:

```go
myTheme := theme.Theme{
    Base: &theme.ThemeGrape,
    Colors: theme.Colors{
        SelectionColor: "#FFB86C", // orange instead of Grape's default
    },
}

editor.Run(editor.Config{
    Schema: &MyConfig{},
    Theme:  myTheme,
})
```

## Custom theme from scratch

Set every `Colors` field directly, with no `Base` (falls back to `ThemePlain` for any field left empty):

```go
myTheme := theme.Theme{
    Colors: theme.Colors{
        ActiveBorderColor:   "#00FF00",
        SelectionColor:      "#FFFF00",
        InactiveBorderColor: "#888888",
        AvailableItemColor:  "#666666",
        ExistingItemColor:   "#00FFFF",
        ErrorColor:          "#FF0000",
    },
}
```

## Resolving colors outside the editor

`theme.ResolveColors(t)` merges a `Theme` down to a concrete `Colors` value without importing `editor` - useful when building a companion TUI (e.g. `docgenerator`'s doc browser) that should match the host app's theme:

```go
colors := theme.ResolveColors(myTheme)
```

## Low-color terminals

There is no `NO_COLOR` switch. Use `ThemePlain` (the default) for terminals with limited color support: it is built entirely from ANSI 16-color codes rather than hex or 256-color values, so the terminal's own palette controls how it renders.
