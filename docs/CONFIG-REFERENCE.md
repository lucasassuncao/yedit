# Config Reference

Every field of `editor.Config`, in one table. See the linked guide for each concern's full explanation and examples.

---

## Core

| Field | Type | Description |
|---|---|---|
| `Path` | `string` | YAML file to load; also the default save target when `SavePath` is empty. |
| `Schema` | `any` | Non-nil struct pointer describing the document (e.g. `&MyConfig{}`). The editor introspects it via `schema.Discover`. See [Schema Kinds Reference](SCHEMA-KINDS.md). |
| `Title` | `string` | Label shown in the TUI header. |
| `SavePath` | `string` | Write to this path instead of `Path`; `Path` is still used for loading. |
| `SchemaRecursionDepth` | `int` | Extra levels a self-referential type expands (e.g. `CategoryFilter.Any []CategoryFilter`); `0` uses the default (1). |
| `Hidden` | `[]string` | Top-level keys to omit from the UI entirely. |
| `PassthroughKeys` | `[]string` | Top-level keys preserved as-is; hidden from all sections and excluded from unknown-key validation. |

## Presets

| Field | Type | Description |
|---|---|---|
| `BlockPresets` | `presets.Source` | Optional; `nil` disables the preset picker inside block editors. See [Presets](PRESETS.md). |
| `DocPresets` | `presets.Source` | Optional; when set, `p` on the root list opens a whole-document template picker. See [Presets](PRESETS.md). |

## Metadata and hints

| Field | Type | Description |
|---|---|---|
| `EnableHints` | `bool` | Show the Hint/Example panel; requires `Metadata` to be set (a warning is shown if it is not). |
| `Metadata` | `MetadataSource` | Field metadata displayed in the hint panel and enforced by the `FromMetadata` validators. See [Metadata and Hints](METADATA-AND-HINTS.md). |

## Validation

| Field | Type | Description |
|---|---|---|
| `Validators` | `[]Validator` | Rules evaluated before every save and on the validate shortcut. See [Validators Reference](VALIDATORS.md). |
| `NoValidateOnSave` | `bool` | Allow saving even when validators report errors; a warning alert is shown but does not block. |

## Confirmations

| Field | Type | Description |
|---|---|---|
| `NoDeleteConfirm` | `bool` | Skip the "Remove block?" confirmation dialog; deletion is still undoable via Ctrl+U. See [Undo & Redo](UNDO.md). |
| `NoSaveConfirm` | `bool` | Skip the "Save changes?" confirmation dialog; warning confirms (`NoValidateOnSave`) are still shown. |

## Appearance

| Field | Type | Description |
|---|---|---|
| `Theme` | `theme.Theme` | Zero-value resolves to `ThemeDark`. See [Themes](THEMES.md). |

## Session tracing

All session-observability options live under `Config.Trace` (type `Trace`):

| Field | Type | Description |
|---|---|---|
| `Dump` | `bool` | When true, records every action and keystroke to a JSONL file; the path is reported in `Result.DumpPath`. |
| `DumpPath` | `string` | Optional explicit path for the `Dump` trace file; ignored when `Dump` is false. Empty falls back to a timestamped file in the OS temp dir. |
| `OnAction` | `func(blockKey string, a BlockAction)` | Optional; called synchronously after every `BlockAction` is dispatched. |
| `OnModelAction` | `func(a ModelAction)` | Optional; called synchronously after every `ModelAction` is dispatched. |
| `OnMsg` | `func(where string, msg tea.Msg)` | Optional; called synchronously for every raw `tea.Msg` the program receives. |

See [Session Tracing](SESSION-TRACING.md) for the full event schema and coverage details.

## Result

`editor.Run` / `editor.RunContext` return a `Result`:

| Field | Type | Description |
|---|---|---|
| `Saved` | `bool` | True when at least one save to disk succeeded during the session. |
| `DumpPath` | `string` | Path of the session trace file, set when `Config.Trace.Dump` is true. |
