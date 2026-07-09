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
| `plain` | `dark` | `light` | `dracula` |
| `monokai` | `solarized` | `banana` | `mint` |
| `strawberry` | `blueberry` | `mango` | `watermelon` |
| `peach` | `kiwi` | `lemon` | `orange` |
| `grape` | `cherry` | `pineapple` | `raspberry` |
| `lime` | `pomegranate` | `apple` | `plum` |
| `apricot` | `dragonfruit` | `blackberry` | `tangerine` |
| `fig` | `guava` | `acai` | `coconut` |
| `guarana` | | | |

`dark` (`theme.ThemeDark`) is the default when `Config.Theme` is left at its zero value. `plain` uses only ANSI 16-color codes (`"4"`, `"6"`, `"8"`, `"2"`, `"1"`) instead of hex/256-color values, for terminals with limited color support.

```go
editor.Run(editor.Config{
    Schema: &MyConfig{},
    Theme:  theme.ThemeDracula,
})
```

## Structure

A `Theme` is a three-layer appearance configuration:

```go
type Theme struct {
    Base   *Theme // optional preset to inherit from (nil → ThemeDark)
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
    Base: &theme.ThemeDracula,
    Colors: theme.Colors{
        SelectionColor: "#FFB86C", // orange instead of Dracula's pink
    },
}

editor.Run(editor.Config{
    Schema: &MyConfig{},
    Theme:  myTheme,
})
```

## Custom theme from scratch

Set every `Colors` field directly, with no `Base` (falls back to `ThemeDark` for any field left empty):

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

## NO_COLOR

`theme.NoColor()` reports whether the `NO_COLOR` environment variable is set. When it is, `theme.Color(...)` returns an empty `lipgloss.Color` (terminal default) instead of the requested value, so the whole UI renders monochrome regardless of the configured theme. This is a single global switch, not per-`Theme` configurable.
