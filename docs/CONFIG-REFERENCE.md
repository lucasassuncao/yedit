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
| `DocPresets` | `presets.Source` | Optional; when set, `p` on the root list opens a whole-document preset picker. See [Presets](PRESETS.md). |

## Metadata and hints

| Field | Type | Description |
|---|---|---|
| `EnableHints` | `bool` | Show the Hint/Example panel; requires `Metadata` to be set (a warning is shown if it is not). |
| `Metadata` | `MetadataSource` | Field metadata displayed in the hint panel and enforced by the `FromMetadata` validators. See [Metadata and Hints](METADATA-AND-HINTS.md). |
| `AnimationDuration` | `time.Duration` | When `> 0`, the Hint/Example panel eases open and closed over this duration instead of snapping. `0` (the default) keeps the toggle instant and leaves the editor emitting no timer messages. See [Metadata and Hints](METADATA-AND-HINTS.md#animating-the-panel). |
| `LegendLines` | `int` | Rows the key legend at the bottom may take. `1` gives a compact footer; `2` (the default when `0`) shows more keys before the rest is folded into `+N in [?]`. |

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

## Application actions

| Field | Type | Description |
|---|---|---|
| `Actions` | `[]Action` | Application keys on the root list, shown in the legend next to `ctrl+s`. The application decides what each one does; the editor shows it, triggers it, saves first when asked, and reports the outcome. |

An `Action` has:

| Field | Type | Description |
|---|---|---|
| `Key` | `string` | The key that triggers it, e.g. `"ctrl+e"`. It must not shadow a built-in key. |
| `Help` | `string` | The legend label, e.g. `"save & convert"`. |
| `SaveFirst` | `bool` | Save through the same validation and confirmations as `ctrl+s` first. Nothing runs if the save does not happen: a cancelled confirmation or a validation error stops it. A clean document already on disk is not written again. |
| `Run` | `func(ActionContext) (ActionResult, error)` | Runs in the background, with a spinner on the status row. A second action waits until it returns. |
| `Exec` | `func(ActionContext) *exec.Cmd` | Hands the terminal to a program (an editor, a pager, a shell) until it exits. A clean document the program changed on disk is reloaded; with unsaved edits, the editor only says the file changed. `Exec` wins when both it and `Run` are set. |

`ActionContext` carries `Path`, the file on disk (the one just written, with `SaveFirst`), and `Raw`, the document as the editor holds it, saved or not.

`ActionResult` is what `Run` reports. `Message` alone is shown in an alert. With `Output` set, `Message` and `Output` open together in a scrollable pager instead, for a dry run, a diff or a log. An error is shown the same way, titled "<Help> failed", and keeps any `Output` the run produced.

```go
editor.Config{
    // ...
    Actions: []editor.Action{
        {
            Key: "ctrl+e", Help: "save & convert", SaveFirst: true,
            Run: func(ctx editor.ActionContext) (editor.ActionResult, error) {
                out, err := convert(ctx.Path)
                return editor.ActionResult{Message: "Wrote " + out}, err
            },
        },
        {
            Key: "ctrl+o", Help: "open in $EDITOR",
            Exec: func(ctx editor.ActionContext) *exec.Cmd {
                return exec.Command(os.Getenv("EDITOR"), ctx.Path)
            },
        },
    },
}
```

## Appearance

| Field | Type | Description |
|---|---|---|
| `Theme` | `theme.Theme` | Zero-value resolves to `ThemePlain`. See [Themes](THEMES.md). |

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
