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
| `LegendLines` | `int` | Rows the key legend at the bottom keeps, blank when it has fewer keys, so the panes never change size with focus. `1` gives a compact footer; `2` (the default when `0`) shows more keys before the rest is folded into `+N in [?]`. |

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
| `ShowDiffOnSave` | `bool` | The "Save changes?" dialog shows the diff of what the save writes, scrollable, instead of only the file it saves to. No effect under `NoSaveConfirm`. |

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

## Application tabs

| Field | Type | Description |
|---|---|---|
| `Tabs` | `[]Tab` | Application screens beside the editor's. A strip under the top row names them, the editor first by its file's name (`FrontMsg` still calls it `editor`), the one in front filled, and **ctrl+t** steps through them: the editor's legend names it, and the strip does while a tab is in front. The strip takes a row from every screen. Without tabs, or files opened through `PathField.Open`, there is no strip and nothing changes. |
| `FrontTab` | `string` | The tab, by `Name()`, in front when the editor opens. Empty, or a name no tab has, opens on the editor. |

A `Tab` is a small Bubble Tea model:

| Method | Description |
|---|---|
| `Name() string` | Its name in the strip. |
| `Init() tea.Cmd` | Run once, with the editor's. |
| `Update(tea.Msg) (Tab, tea.Cmd)` | Every message reaches every tab, shown or not, so work a tab started goes on while the document is edited. Keys, mouse and pastes reach only the screen in front. The `WindowSizeMsg` it gets is the terminal less the strip's row, and `editor.FrontMsg{Name}` says which screen is in front, at the start and on every change. |
| `View() string` | Its whole screen, drawn only while it is in front. The strip is drawn under its first row, which is best kept for a header. Its legend is its own: name **ctrl+t** there if it should show. |
| `Busy() string` | What quitting now would cut short, or `""`. While any tab says something, quitting asks first and shows it. |

A tab talks to the editor with messages its commands return:

| Message | What the editor does |
|---|---|
| `editor.SaveMsg{}` | Comes in front and saves as **ctrl+s** does: the same validation and "Save changes?". Once the file is written, every tab gets `editor.SavedMsg{Path}` and the tab that asked comes back in front. A clean document already on disk answers at once. A refused, cancelled or failed save answers nothing; an open block editor holds edits the file does not have yet, so the editor asks to close it first. |
| `editor.QuitMsg{}` | Comes in front and quits as **q** does, asking first when the document has unsaved changes or a tab is `Busy`. |

## Path fields

| Field | Type | Description |
|---|---|---|
| `PathFields` | `map[string]PathField` | Top-level keys that hold a path, or a list of paths. In their block editor, `ctrl+o` opens a file picker in the document's folder; the pick is set as the value of a scalar key, and in a list it replaces the entry under the YAML cursor or is appended. Each pick is one undo step. |

A `PathField` has:

| Field | Type | Description |
|---|---|---|
| `Types` | `[]string` | Extensions the pick may have, e.g. `".yaml"`. Other files stay listed, and the picker says why it refuses them. Empty allows any file. |
| `Dirs` | `bool` | A directory may be picked as well as a file. |
| `Format` | `func(abs, docPath string) (string, error)` | Shapes the pick before it is written. `abs` is the absolute pick; `docPath` is the absolute path the document is saved to (`SavePath` when set), so a relative path holds where the file lands. An error is shown and nothing is written. `nil` writes `abs`. |
| `Open` | `func(value, docPath string) (Config, error)` | Makes the entries of a top-level list open: **Enter** on an entry edits the file it names in a tab of its own, with the `Config` returned. `value` is the entry as written, `docPath` as for `Format`. Each file tab has its own undo, validation and save; **ctrl+t** steps through the open files, **esc** on a file's block list closes it and returns to the main screen (asking first when it has unsaved changes), and a file stays open until then. **q** quits only from the main screen; **ctrl+c** quits from anywhere. Both ask once about every edited file. Opening a file that is already open brings it to front. A file tab ignores the returned `Tabs` and `Actions`. An error is shown and nothing opens. |

```go
editor.Config{
    // ...
    PathFields: map[string]editor.PathField{
        "import": {
            Types: []string{".yaml", ".yml"},
            Format: func(abs, docPath string) (string, error) {
                rel, err := filepath.Rel(filepath.Dir(docPath), abs)
                if err != nil {
                    return abs, nil // another drive: no relative form
                }
                return "./" + filepath.ToSlash(rel), nil
            },
        },
    },
}
```

`ctrl+o` belongs to the block editor here, so it does not clash with an `Action` bound to `ctrl+o` on the root list.

## Appearance

| Field | Type | Description |
|---|---|---|
| `Theme` | `theme.Theme` | Zero-value resolves to `Plain`. See [Themes](THEMES.md). |

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
