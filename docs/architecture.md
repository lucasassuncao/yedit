# yedit — Internal Architecture

This document describes how the editor works end-to-end: how data flows from disk through the node tree and back, how the editor stack enables nested editing, and how the undo system operates at two independent levels.

---

## Layers at a glance

```
┌─────────────────────────────────────────────────────┐
│  editor.Run (bubbletea program)                     │
│  ┌───────────────┐  ┌──────────────────────────┐    │
│  │  model (root) │  │  blockEditState (stack)  │    │
│  │  list / alert │  │  tree · YAML pane · node │    │
│  └───────────────┘  └──────────────────────────┘    │
│           │                      │                  │
│      document.Document     *yaml.Node (editRoot)    │
│      (raw bytes + history)  single canonical tree   │
└─────────────────────────────────────────────────────┘
           │
      file on disk
```

There are two separate "sources of truth" that never overlap:

| Layer | What it owns | Undo mechanism |
|---|---|---|
| `document.Document` | Raw YAML bytes as they would appear on disk | `doc.Undo()` / `doc.Redo()` — byte-level snapshots |
| `blockEditState.node` | Parsed `*yaml.Node` while a block is open for editing | `be.undoStack` / `be.redoStack` — node snapshots |

The document is only mutated when the user commits (Ctrl+S inside an editor) or explicitly removes a block. Between those moments, the only thing that changes is the in-memory node tree.

---

## document.Document

`document/document.go` owns the raw bytes of the YAML file.

```
Load(path, knownOrder) → *Document
    raw []byte              ← file content (CRLF normalised)
    blocks []Block          ← parsed block list (key + line range)
    history [][]byte        ← undo stack (raw snapshots)
    future  [][]byte        ← redo stack
    knownOrder []string     ← canonical key order for Insert/Replace
```

**Block model.** The document is divided at the top level into named blocks — one per top-level YAML key. `ParseBlocks` splits the raw bytes into `Block{Key, StartLine, EndLine}` entries without full YAML parsing. This keeps all mutations fast: `Insert`, `Replace`, and `Remove` splice raw lines, then re-parse the block index.

**Mutation methods:**

| Method | What it does |
|---|---|
| `Insert(snippet)` | Appends a new block, positioned by `knownOrder` |
| `Replace(key, snippet)` | Removes the block then inserts the new version |
| `Remove(key)` | Removes the block entirely |

Every mutation calls `snapshot()` first (saves the current raw bytes to `history`) and sets `dirty = true`. `Undo()` / `Redo()` swap the raw bytes back from those snapshots and re-parse the block index.

**Round-trip guard.** After `Insert` and `Replace`, the document re-reads the stored block with `BlockContent` and compares it against the submitted snippet using `blockSemanticEqual` (YAML-parse both and compare node structure). If they diverge the mutation is rolled back. This catches any serialisation quirk before it reaches disk.

---

## Schema layer

`schema.Discover(ptr, depth)` walks the Go struct passed as `Config.Schema` via reflection and builds a `[]FieldDef` tree:

```go
type FieldDef struct {
    YAMLName string
    Kind      Kind       // KindPrimitive | KindObject | KindList | KindDictionary
    Children  []FieldDef // nested struct fields
    // ...
}
```

`Kind` drives how a block editor behaves:

| Kind | What it maps to | Editor mode |
|---|---|---|
| `KindObject` | struct | tree + YAML pane (field toggles) |
| `KindList` | `[]Struct` | collection navigator (sequence) |
| `KindDictionary` | `map[string]Struct` | collection navigator (map) |
| `KindPrimitive` | scalar / `[]string` / free map | YAML pane only |

`schema.KnownChildren(tree)` produces a `map[path]map[key]bool` used at commit time to detect unknown YAML keys.

---

## Root model

`editor/root.go` holds the bubbletea `model`:

```go
type model struct {
    cfg         Config
    doc         *document.Document
    schemaTree  []schema.FieldDef

    list        listModel        // left-panel block list
    blockEdits  []*blockEditState // editor stack (non-empty when a block is open)
    editRoot    *yaml.Node       // canonical node for the block being edited
    editBlockKey string          // top-level key that editRoot belongs to
    alert       *alert.Model
    // ...
}
```

**Exactly one pane is active at a time:** `paneList`, `panePreview`, `paneBlockEdit`, or `paneAlert`. The four `enter*` methods are the only places that change `m.mode`, so invariants are maintained by construction: `alert != nil ⟺ paneAlert`, `len(blockEdits) > 0 ⟺ paneBlockEdit`.

---

## Opening a block

When the user presses Enter on a list item, `handleOpenItem` runs:

1. Read the current block content from the document with `doc.BlockContent(key)` (or use an empty template for a new block).
2. Create a `blockEditState` with `newBlockEdit(cfg, spec, w, h)`.
3. Set `be.focus = nil` — the root editor addresses the whole block.
4. Push it onto `m.blockEdits`.
5. Initialise `m.editRoot` as a fresh empty `MappingNode` (a non-nil placeholder; the first flush populates it).
6. Call `enterBlockEdit()`.

---

## blockEditState — the block editor

Each `blockEditState` owns:

```
node        *yaml.Node       ← canonical value node (single source of truth for this editor)
tree        treeModel        ← checkmark tree projected from node
yamlEditor  textarea.Model   ← text buffer (tolerant; may be mid-edit)
coll        collectionBuffer ← current entry index (collections only)
focus       []pathSeg        ← address of this editor inside editRoot
undoStack   []*blockEditUndoSnap
redoStack   []*blockEditUndoSnap
```

### be.node: the single source of truth

`be.node` is the `*yaml.Node` for the block's value mapping (or sequence/map root for collections). It is the only authoritative representation of the block's current data. Everything else is derived from it:

- **Tree panel** — `syncTreeCheckedFromNode(tree, node)` walks the node to compute which fields are present (checked), then applies ADDED/AVAILABLE sectioning.
- **YAML panel** — `nodeToContent(key, node)` serialises the node to the text displayed (and editable) on the right.

### Tolerant parse gate

Typing in the YAML panel can leave the buffer temporarily invalid. After each content-changing keystroke, `syncParsedNode` tries to parse the buffer with `valueNodeOfSnippet`:

- **Parse succeeds** → `be.node` advances to the new node; the tree is re-derived from it.
- **Parse fails** → `be.node` keeps its last valid state; the tree is unchanged.

Navigation keys (arrows, selection) do not trigger the gate — there is nothing to re-project.

### Tree toggles

When the user checks or unchecks a field in the tree panel, `handleTreeToggle` runs:

1. `be.saveUndo()` — snapshot current state before mutation.
2. `toggleNodeField(be.node, ctx, node, checked)` — structurally adds or removes the field from `be.node` using `applyToggleAt`, then calls `pruneEmptyMappings(be.node)` to remove any now-empty parent mappings or sequences.
3. `reorderNestedMappingKeys` — sorts keys back to schema order.
4. `syncTreeCheckedFromNode` — re-derives the tree from the updated `be.node`.
5. `nodeToContent(key, be.node)` → `yamlEditor.SetValue(...)` — re-renders the YAML panel.

The tree and YAML panel are always consistent because both are derived from `be.node` after every mutation.

---

## Collection navigation

For `KindList` and `KindDictionary` blocks with schema-defined children, the editor becomes a **collection navigator**. `be.node` is the entire sequence or map node (not just one entry's value). A `collectionBuffer` tracks which entry is shown:

```
be.node          ← sequence/map node — owns ALL entries
be.coll.current  ← index of the entry shown in yamlEditor
```

Navigation between entries is a two-step flush-load cycle:

1. `flushCurrentEntry()` — parse the YAML editor text and write it back into `be.node` at the current entry position (parse gate: rejects invalid YAML and blocks navigation).
2. `loadEntry(idx)` — read `be.node[idx]` and set `yamlEditor` to its serialised form.

`collectionDeriveTree()` re-projects labels and field checkmarks for **all** entries from `be.node` after any structural change (add, delete, reorder).

---

## editRoot and the editor stack

`model.editRoot` is the single canonical `*yaml.Node` for the top-level block currently being edited. It starts as an empty mapping and is populated by the first `flushTopToRoot` call.

**Why one shared tree instead of one node per editor?**  
String-splicing between stacked editors would corrupt nested data (e.g. if an outer field's text happened to match an inner block boundary). Using one shared node tree means every editor's `focus` path addresses the same live object, so writes at any depth can never corrupt unrelated paths.

### flushTopToRoot

Serialises the active (top) editor and writes its result back into `editRoot` at `be.focus`:

```
top.commit() → snippet (YAML text)
valueNodeOfSnippet(snippet) → val (*yaml.Node)
setNodeAt(editRoot, top.focus, val)
```

`setNodeAt` with `segs = nil` (root editor, `focus = nil`) replaces `editRoot` itself with `val`.

### Drill-in (Enter on an openable field)

1. Flush the current top editor into `editRoot` (`flushTopToRoot`).
2. Compute `childFocus = parentFocus + relSegs`.
3. Read `nodeAt(editRoot, childFocus)` — the child's current content from the live tree.
4. Create a `blockEditState` for the child field with `focus = childFocus`.
5. Push it onto `blockEdits`.

### Drill-out (Esc on a nested editor)

1. Record `childWasDirty = top.dirty`.
2. Flush the child into `editRoot` (`flushTopToRoot`).
3. Pop the child from `blockEdits`.
4. If `childWasDirty`, call `saveUndo()` on the new top (parent) editor — this lets Ctrl+U on the parent undo the entire drill-in in one step.
5. `refreshTopFromRoot(childWasDirty)` — re-read the parent's focus path from `editRoot` and rebuild its YAML panel and tree from the updated node.

No data is written to the document during drill-out. Changes accumulate in `editRoot` until Ctrl+S.

---

## commitAll — writing back to the document

Ctrl+S inside any block editor calls `saveAll`, which runs validators first and then `commitAll`:

```
commitAll():
    isEdit = blockEdits[0].isEdit   ← true = Replace, false = Insert

    flushTopToRoot()                ← write active editor into editRoot

    pruneEmptyMappings(editRoot)    ← remove empty mappings/sequences left by toggles
                                       and empty collection items left by drill-out

    blockIsEmpty = editRoot is an empty MappingNode

    switch:
    case blockIsEmpty && isEdit:
        doc.Remove(editBlockKey)    ← all fields removed → delete the key entirely

    case !blockIsEmpty:
        final = nodeToContent(editBlockKey, editRoot)   ← serialise once
        if isEdit:
            current = doc.BlockContent(editBlockKey)
            if normalizeBlockContent(current) != final: ← skip if content unchanged
                doc.Replace(editBlockKey, final)
        else:
            doc.Insert(final)

    syncView(); enterList()
```

`normalizeBlockContent` parses the document's existing block content and re-serialises it through `nodeToContent` so both sides of the comparison go through the same formatting pipeline (block style, 2-space indent). This prevents a no-op commit (e.g. an empty collection item that was pruned) from taking a history snapshot or marking the document dirty.

---

## Undo/redo — two independent levels

### Inside a block editor (Ctrl+U / Ctrl+Y)

Every mutating operation calls `be.saveUndo()` before changing state. `saveUndo` calls `captureSnap()`:

```go
type blockEditUndoSnap struct {
    node       *yaml.Node  // deep clone of be.node
    yamlValue  string      // text buffer at snapshot time
    dirty      bool
    preset     string
    treeNodes  []treeNode  // tree state (cursor, expansion)
    // ...
}
```

`restoreUndo()` clones the snapshot's node back into `be.node`, restores the YAML buffer and tree, and pushes the undone state onto `redoStack`. `restoreRedo()` is the symmetric operation.

Ctrl+U/Ctrl+Y while a block editor is open **never** falls through to `doc.Undo()`/`doc.Redo()`. The two undo levels are fully separated.

### At the document level (list view, Ctrl+U / Ctrl+Y)

`doc.Undo()` and `doc.Redo()` swap raw byte snapshots. These cover `Insert`, `Replace`, and `Remove` operations — i.e. every Ctrl+S commit, every block deletion, and every preset application.

---

## pruneEmptyMappings

Called after every tree toggle and again in `commitAll`. Removes:

- Mapping keys whose value is an empty mapping or empty sequence (bottom-up, so intermediate nodes are cleaned after their children).
- Empty mapping items (`{}`) from sequences — this handles the case where a drill-in is opened for a new collection item and the user commits without adding any fields.

---

## Validators

Two families, both run at save time via `RunAll(cfg.Validators, raw, blocks)`:

**FromMetadata family** (`RequiredFromMetadata`, `OneOfFromMetadata`, etc.) — wired at startup in `newModel` with the discovered schema tree and `cfg.Metadata`. They read `FieldMeta` from the `MetadataSource` for each field and enforce the declared constraint against the raw YAML.

**Explicit family** (`Required`, `ValueOneOf`, `ValueInRange`, etc.) — self-contained; work with just the raw bytes and block list. Used for one-off or cross-field rules.

---

## MetadataSource and the hint panel

`Config.Metadata` is a `MetadataSource`:

```go
type MetadataSource interface {
    FieldMeta(blockKey, fieldPath string) FieldMeta
}
```

`FieldMeta` carries display data (Description, Type, Default, OneOf, Example, …) and constraint data (Required, Min, Max, Pattern, MinCount, MaxCount, Unique, Deprecated). It is the single source of truth for both the Hint/Example panel and the `FromMetadata` validators — constraints are declared once and reused in both places.

The hint panel is opt-in via `Config.EnableHints`. When enabled, the right column in the list view splits: Preview on top, Hint/Example below. The panel renders the `FieldMeta` for the currently selected block via `selectedHint()`, and `blockEditState.fieldHintFor(path)` does the same for individual fields inside an open editor.
