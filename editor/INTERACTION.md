# Interaction model

The block editor's left panel is a tree (`tree.go`). Every keypress goes through
`tree.Update`, which returns a `treeAction`; the `blockEditState` then reacts to
that action. The behavior is a function of **what the cursor is on** crossed with
**which key was pressed**.

## Cursor targets

How the four schema `Kind`s (plus structure and state) appear as tree rows:

| Schema shape | Tree row | Distinct states |
| --- | --- | --- |
| `KindPrimitive`, or `KindList`/`KindDictionary` **without** children | **leaf** (scalar) | unchecked, checked |
| `KindObject` **with** children | **inline parent** (expand in place) | collapsed/expanded × empty/has-content |
| `KindList`/`KindDictionary` **with** children | **openable** (drill into a sub-editor) | empty, has-content |
| — | **seqItem** (a collection entry) | collapsed, expanded |
| — | **addNew** (`[+ add new]` row) | — |

`openable` lists and maps behave identically at the tree layer; the difference is
only in the apply layer (`applyToggleToSeqItem` vs `applyToggleToMapEntry`).

## Action matrix (11 targets × 6 keys = 66 cells)

| Target | up | down | left | right | enter | ctrl+d |
| --- | --- | --- | --- | --- | --- | --- |
| addNew | – | – | – | – | add | – |
| seqItem collapsed | – | – | – | expand | – | del* |
| seqItem expanded | – | – | collapse | – | – | del* |
| leaf unchecked | – | – | – | – | toggle-on | – |
| leaf checked | – | – | – | – | – | toggle-off† |
| inline collapsed, empty | – | – | – | expand | expand | – |
| inline collapsed, has-content | – | – | – | expand | expand | toggle-off† |
| inline expanded, empty | – | – | collapse | – | – | – |
| inline expanded, has-content | – | – | collapse | – | – | toggle-off† |
| openable empty | – | – | – | drill-in | drill-in | – |
| openable has-content | – | – | – | drill-in | drill-in | toggle-off† |

Legend: `–` no-op · `*` confirms "Remove entry?" (skipped when `NoDeleteConfirm`)
· `†` when the field has content, confirms "Remove field?".

Notes:

- `left` on a nested row that cannot collapse moves the cursor to its parent row
  (still a no-op action).
- `up`/`down` only move the cursor; in a collection, crossing into a different
  entry flushes the current entry's buffer and loads the new one.
- An inline parent has no checkbox of its own — its presence in the YAML is
  derived from its children. Toggling a child auto-creates the parent; `ctrl+d`
  on the parent removes the whole subtree.
- Global keys (`tab` switch pane, `ctrl+s` commit, `ctrl+u` undo, `esc`
  back/drill-out, `p` preset picker) act on the editor, not the cursor target,
  so they are not part of this grid.

## Enforcement

This grid is a tested contract, not just documentation.

The number **66** here means the cells of *this* matrix (11 cursor targets × 6
keys) — it is not the number of test functions in the package. One test,
`TestMatrix_TreeActions`, drives all 66 cells in a loop.

- `interaction_matrix_test.go` → `TestMatrix_TreeActions` asserts all 66 cells
  against this table, so any change to a cell breaks the build.
- `TestMatrix_StateMutations` / `TestMatrix_LeftMovesToParent` verify the state
  side effects (expand/collapse/toggle and left-to-parent).
- The downstream consequence of the mutating actions is validated across the
  struct / seq / map block contexts by `TestMatrix_ToggleConsequenceAcrossContexts`
  and the suite in `nested_combinations_test.go`.
