# Session tracing (Dump)

This document explains `editor.Config.Trace.Dump` and the lower-level `OnAction` / `OnModelAction` / `OnMsg` hooks it is built on. Use these to record what a user did in a session - keystrokes and the semantic actions they produced - for bug reports and reproduction.

---

## Quick start

Set `Config.Trace.Dump: true`. When the session ends, `Result.DumpPath` holds the path of a JSONL trace file written to the OS temp dir (`yedit-dump-<timestamp>.jsonl`):

```go
res, err := editor.Run(editor.Config{
    Path:   "config.yaml",
    Schema: &MyConfig{},
    Trace:  editor.Trace{Dump: true},
})
if err != nil {
    log.Fatal(err)
}
if res.DumpPath != "" {
    fmt.Println("session trace written to", res.DumpPath)
}
```

No other wiring is required - `Dump` composes the trace writer internally on top of the same hooks described below.

Set `Config.Trace.DumpPath` to write to a specific file instead of the OS temp dir default:

```go
editor.Config{
    // ...
    Trace: editor.Trace{
        Dump:     true,
        DumpPath: "./trace.jsonl", // optional; empty falls back to the temp dir default
    },
}
```

## Event schema

Each line of the file is one JSON object. Field order in the file is `ts, seq, scope, where, key, type, action` (a struct is used internally instead of a map specifically so the order stays stable rather than alphabetical).

| Field | Type | Present when | Meaning |
|---|---|---|---|
| `ts` | RFC 3339 timestamp | always | Wall-clock time the event was recorded. |
| `seq` | int | always | Monotonic counter starting at 1, incremented per event. Use this instead of `ts` to establish exact ordering. |
| `scope` | string | always | One of `"key"`, `"block"`, `"model"`, `"msg"` (see below). |
| `where` | string | always | Location in the UI at the time of the event. For `scope:"key"`/`"msg"` this is the full pane/block/panel/mode descriptor (see [Location strings](#location-strings)); for `scope:"block"` it is just the block's key; empty for `scope:"model"`. |
| `key` | string | `scope:"key"` only | Human-readable key name from [`tea.KeyMsg.String()`](https://pkg.go.dev/github.com/charmbracelet/bubbletea#KeyMsg) - e.g. `"enter"`, `"esc"`, `"down"`, `"ctrl+c"`, or the literal rune (`"j"`). Alt-modified keys get an `"alt+"` prefix. |
| `type` | string | `scope:"block"`/`"model"`/`"msg"` only | The concrete Go type of the action/message, via `%T` (e.g. `"editor.ToggleField"`, `"editor.commitRequestedMsg"`). This is the discriminator for decoding `action`. |
| `action` | object | `scope:"block"`/`"model"`/`"msg"` only | The value's exported fields, e.g. `{"NodeIdx":3,"Checked":true}` for a `ToggleField`. Values with no exported fields (`AddEntry{}`, internal messages with only unexported fields) serialize as `{}` - the `type` alone still records that the event happened. |

### `scope` values

- **`"key"`** - a raw keystroke. Only `tea.KeyMsg` messages get this scope (see `"msg"` below for everything else); most cursor movement is a `"key"` event with no corresponding `"block"`/`"model"` line, since it doesn't mutate anything.
- **`"block"`** - a [`BlockAction`](../editor/actions.go) dispatched inside a block editor (`ToggleField`, `SyncYAML`, `AddEntry`, `DeleteEntry`, `NavigateEntry`, `ApplyPreset`, `AppendPreset`, `Undo`, `Redo`). Captured via `OnAction`, which fires from the single block-level dispatch gateway, so every block mutation is guaranteed to appear.
- **`"model"`** - a [`ModelAction`](../editor/actions.go) dispatched at the document level (`DrillIn`, `DrillOut`, `DeleteBlock`, `Save`, `Reload`, `ToggleHints`, `ApplyDocPreset`, …). Captured via `OnModelAction`.
- **`"msg"`** - every other `tea.Msg` the program receives, captured via `OnMsg` at the single top-level `model.Update` entry point. This is the catch-all: several real user-triggered transitions - opening a block from the root list, Ctrl+S commit/save, confirming a delete/reload/doc-preset dialog, dismissing an alert, running validation - are handled directly in `model.Update`'s switch instead of going through `model.dispatch(ModelAction)`, so they have no `"model"`-scope representation. `"msg"` closes that gap: it fires for literally everything except four known noise sources, none of which reflects user input: `cursor.BlinkMsg` (the textarea's continuous blink tick), `cursor.initialBlinkMsg` (fires once whenever a textarea gains focus), `cursor.blinkCanceled` (fires on every keystroke typed into a textarea - typing cancels the pending blink), and yedit's own `clearStatusMsg` (a status-bar decay timer).

### Location strings

`where` for `scope:"key"` events is produced by `model.traceLocation()`:

| Format | Meaning |
|---|---|
| `list` | Root block list. |
| `preview` | Read-only preview pane. |
| `alert` | A confirmation/alert dialog is showing. |
| `docPreset` | Whole-document preset picker is open. |
| `block:<key>:<panel>:<mode>` | Inside a block editor. `<panel>` is `tree`, `yaml`, or `hint`; `<mode>` is `editing`, `presetBrowser`, or `confirming`. |

Example: `block:categories:tree:editing` means the cursor is in the tree panel of the `categories` block, in normal editing mode (not the preset picker or a confirm dialog).

## Coverage: every action is captured

`Config.Trace.Dump` is designed for 100% coverage - every message the editor's `Update` loop ever receives is recorded, because `OnMsg` fires unconditionally on the first line of `model.Update`, before any routing happens. Concretely this means, on top of the `"key"`/`"block"`/`"model"` events:

- Opening a block from the root list (`openItemMsg`) - `ModelAction` has an `OpenBlock{Key}` type for this, but that path is never actually dispatched through `model.dispatch`; it shows up as `scope:"msg"`, `type:"editor.openItemMsg"` instead.
- Ctrl+S commit/save (`commitRequestedMsg`, `doSaveMsg`, `saveResultMsg`).
- Confirming "Remove block?" / "Reload from disk?" / "Apply document preset?" dialogs (the confirmation itself flows through `model.dispatch` and appears as `"model"`; the message that *shows* the dialog does not, and appears as `"msg"`).
- Dismissing any alert (`alert.DismissedMsg`).
- Ctrl+L validate (`validateRequestedMsg`).
- TAB (pane switch) and the preset picker were already fully covered before this: TAB is an ordinary `tea.KeyMsg`, and selecting a preset dispatches `ApplyPreset`/`AppendPreset` as a `"block"` event: `"msg"` scope mainly closes gaps at the document/model level, not inside the block editor.

Some of these messages (e.g. `openChildMsg`, `saveResultMsg`) have only unexported fields, so `action` serializes to `{}` - but `type` still records that the event happened and in what order, which is what matters for reproducing a sequence of interactions.

## Example trace

```json
{"ts":"2026-07-09T02:36:59.7626583-03:00","seq":33,"scope":"key","where":"block:categories:tree:editing","key":"down"}
{"ts":"2026-07-09T02:36:59.7632038-03:00","seq":34,"scope":"block","where":"categories","type":"editor.NavigateEntry","action":{"Idx":1}}
{"ts":"2026-07-09T02:37:10.8589344-03:00","seq":79,"scope":"key","where":"block:categories:tree:editing","key":"enter"}
{"ts":"2026-07-09T02:37:10.8595491-03:00","seq":80,"scope":"block","where":"categories","type":"editor.AddEntry","action":{}}
```

Reading this: pressing `down` (seq 33) while the cursor was on the entries list of `categories` moved it to entry index 1, producing a `NavigateEntry{Idx:1}` (seq 34) one event later. Later, pressing `enter` (seq 79) past the last entry created a new one, producing `AddEntry{}` (seq 80).

## Lower-level hooks

`Dump` is a convenience built on three `Config.Trace` fields you can also use directly (they compose with `Dump` - both fire if both are set):

```go
type Trace struct {
    // ...
    OnAction      func(blockKey string, a BlockAction) // every BlockAction, with the block it applied to
    OnModelAction func(a ModelAction)                   // every ModelAction
    OnMsg         func(where string, msg tea.Msg)        // every raw tea.Msg (all keystrokes, resize, ticks - unfiltered)
}
```

Use these instead of `Dump` when you need custom storage (e.g. writing to a different location than the OS temp dir, streaming to a log aggregator, or filtering to specific action/message types). `OnMsg` fires for **every** `tea.Msg` with no filtering at all - `Dump`'s internal writer filters out only the four noise sources listed in [Coverage](#coverage-every-action-is-captured); build your own filter if you want something narrower, e.g. keystrokes only.

## Replaying a trace

The dump format mirrors `editor.BlockAction` values closely enough to reconstruct them, but there is currently no public API to feed a dump file back through `replayBlock` (the internal replay helper used by `editor`'s own tests) - decoding `action` back into a concrete `BlockAction` requires switching on `type` and unmarshaling into the matching struct yourself.
