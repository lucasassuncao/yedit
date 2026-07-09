# Presets

This document explains how to configure `Config.BlockPresets` / `Config.DocPresets` in `editor.Config`.

---

Presets populate the preset picker with ready-made YAML snippets for a block. When a user selects a preset, the editor seeds the YAML text area with the chosen snippet.

Presets are defined as Go structs and marshaled to YAML automatically - no hand-written YAML files or boilerplate interface implementations required.

## The three-function pattern

For each field that has presets, define three functions:

```go
type DatabaseConfig struct {
    Driver   string `yaml:"driver"`
    DSN      string `yaml:"dsn"`
    MaxConns int    `yaml:"max-conns"`
}

// private: holds all preset values
func databasePresetsMap() map[string]DatabaseConfig {
    return map[string]DatabaseConfig{
        "postgres-local": {Driver: "postgres", DSN: "postgres://localhost/mydb", MaxConns: 10},
        "mysql-local":    {Driver: "mysql",    DSN: "mysql://localhost/mydb",    MaxConns: 5},
        "sqlite":         {Driver: "sqlite",   DSN: "file:app.db"},
    }
}

// public: returns the content of a specific preset by name
func DatabasePreset(name string) DatabaseConfig {
    return databasePresetsMap()[name]
}

// public: lists all available preset names (sorted)
func ListOfDatabasePresets() []string {
    field := "database"
    return presets.ForField(field, databasePresetsMap()).ListPresets(field)
}
```

## Single field

When only one field has presets, pass `presets.ForField` directly:

```go
editor.Run(editor.Config{
    Schema:  &MyConfig{},
    Presets: presets.ForField("database", databasePresetsMap()),
    // ...
})
```

## Multiple fields

When more than one field has presets, wrap them with `presets.Combine`:

```go
editor.Run(editor.Config{
    Schema: &MyConfig{},
    Presets: presets.Combine(
        presets.ForField("database", databasePresetsMap()),
        presets.ForField("server",   serverPresetsMap()),
        presets.ForField("logging",  loggingPresetsMap()),
    ),
    // ...
})
```

`Combine` merges the sources into one: when the editor asks for `"database"` presets, it dispatches to the `ForField("database", ...)` source; `"server"` goes to the next, and so on.

## Dynamic lookup (no picker)

When presets come from an external source (database, API) and cannot be enumerated upfront, use `presets.Func`. The picker will not appear, but direct `(field, name)` lookups still work:

```go
editor.Run(editor.Config{
    Presets: presets.Func(func(field, name string) (string, error) {
        snippet, err := myRegistry.FetchPreset(field, name)
        return snippet, err
    }),
})
```

## Full example

See `examples/test/main.go` for a complete, runnable example that exercises presets, metadata, and validators together.
