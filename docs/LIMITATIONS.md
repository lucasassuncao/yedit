# Known limitations

Behaviors that come from a dependency rather than yedit's own design, and that
could surprise you or, in one case, silently alter your data.

---

## Tabs and control characters are silently rewritten in the YAML editor

The block editor's "Editing YAML" panel is a [`charm.land/bubbles/v2` `textarea`](https://pkg.go.dev/charm.land/bubbles/v2/textarea). Every rune it receives - typed or pasted - passes through the library's internal `runeutil.Sanitizer`, which is not configurable through the public API:

- a tab (`\t`) is replaced with 4 spaces
- any other control character is dropped
- invalid UTF-8 is dropped

This runs on load too, not just on edits: a block's whole YAML is fed into the textarea via `SetValue` when you open it, and the textarea's buffer - not the original bytes - is what gets parsed back on commit (Ctrl+S). So if a value anywhere in the block contains a raw tab (a pasted Windows path, a description copied from a spreadsheet, a code snippet), opening that block and committing *any* change - even to an unrelated field - silently rewrites that tab to four spaces on disk.

```
input:  description: "col1\tcol2"
stored: description: "col1    col2"   ← rewritten, no warning
```

**This does not affect indentation.** Committing a block never writes the textarea's literal text to disk: `commit()` parses the buffer into a `yaml.Node` and the block is re-serialized from that node with a fixed 2-space indent (`enc.SetIndent(2)`, `editor/yaml.go`), regardless of how it was typed - 2 spaces, 4 spaces, or tabs (already turned into spaces by the sanitizer above). So a tab that leaks in never survives *as* misaligned indentation; the risk below is scoped to the content of scalar values, not document structure.

**Practical implications:**

- This is unlikely to matter for typical config values (paths, flags, short strings), which rarely contain raw tabs.
- It is a real risk for the *content of a value* pasted from a spreadsheet, terminal, or code snippet, where embedded tabs are common - e.g. `"col1\tcol2"` silently becomes `"col1    col2"`.
- There is no warning or confirmation before this happens - the rewrite is invisible until you diff the file.
- Nothing in yedit can prevent it without vendoring or forking `bubbles/v2`'s textarea, which isn't planned. If you need a value with a literal tab preserved, edit that field outside yedit (a text editor, then paste the finished YAML block via the preset/apply flow) rather than typing or pasting it directly into the "Editing YAML" panel.
