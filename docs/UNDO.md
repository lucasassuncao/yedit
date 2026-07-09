# Undo & Redo

yedit maintains two independent undo stacks. Both use the same keys - **Ctrl+U** (undo) and **Ctrl+Y** (redo) - but which stack responds depends on where the cursor is.

---

## Two levels

| Level | Scope | Where Ctrl+U/Ctrl+Y apply |
|---|---|---|
| Block editor | In-memory `yaml.Node` changes while a block is open | Inside a block editor (tree or YAML panel) |
| Document | Raw byte snapshots of committed saves and removals | On the root list |

Ctrl+U while a block editor is open never falls through to the document level - it is fully consumed by the block editor's own stack. Closing a block without committing (Esc, no pending changes) discards the in-editor undo history without touching the document history at all.

## Block-level undo

Every mutating `BlockAction` (`ToggleField`, `SyncYAML` with `Checkpoint: true`, `AddEntry`, `DeleteEntry`, `ApplyPreset`, `AppendPreset`) pushes a snapshot onto the block's undo stack before applying the change. A snapshot captures:

- the canonical `yaml.Node` (deep copy)
- the current collection entry index (for collection-nav blocks)
- the YAML text area's live content
- the current preset name
- the tree's cursor/scroll/expansion state

Pressing Ctrl+U restores the most recent snapshot that actually differs from the live state and pushes the current state onto the redo stack. Ctrl+Y does the reverse. The stack is capped at 50 entries; older snapshots are dropped.

**Speculative checkpoints are deduplicated.** Some UI actions (like Tab-ing into the YAML panel) save a checkpoint defensively even if nothing changes afterward. An exact duplicate of the stack top is skipped when pushing, and any duplicate left on top is dropped before restoring - so Ctrl+U never appears to "do nothing" because of a no-op checkpoint sitting on the stack.

Deleting a field or a collection entry is undoable even when `Config.NoDeleteConfirm` skips the confirmation dialog - the delete still saves a checkpoint first.

## Document-level undo

Pressing Ctrl+U on the root list restores the document to its state before the most recent committed operation (`Insert`, `Replace`, `Remove` - i.e. a block was saved, edited and committed, or deleted). Each undo/redo step is a full raw-byte snapshot of the document, capped the same way as the block-level stack.

**What is and isn't tracked:**

- Committing a block (Ctrl+S inside the editor) *is* tracked - undoable from the list.
- Deleting a block from the list *is* tracked.
- Applying a whole-document preset (`Config.DocPresets`) replaces the document via `Document.ReplaceRaw`, which **does not snapshot**. A document preset application cannot be undone with Ctrl+U - this is why applying one shows a confirmation dialog first ("this will replace the entire document - all unsaved changes will be lost").
- In-progress edits inside an open block editor are not part of document history at all until they are committed - closing a block editor without committing (Esc) simply discards them; there is nothing to undo at the document level.

## Practical implications

- If you need to back out of a bad document-preset application, there is no undo - reload from disk instead (if the file hasn't been saved yet) or manually fix the document.
- Undo inside a block editor is "free" - experiment with toggles and edits, Ctrl+U as needed, and nothing touches the document until Ctrl+S.
- The two stacks never interact: undoing at the document level after committing several blocks steps back through *committed* states one at a time, not through the individual field-level edits that built up each commit.
