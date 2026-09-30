// Package fieldtree is the block editor's left panel: a flat, DFS-ordered
// tree of schema fields and collection entries, projected from a YAML value
// node and rendered as a scrollable, checkable list.
package fieldtree

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/bezel/tree"
	"github.com/lucasassuncao/yedit/keys"
	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/yamledit"
)

// NodeKind classifies each row in the tree-view left panel.
type NodeKind int

const (
	KindField     NodeKind = iota // a struct field (leaf or expandable struct)
	KindSeqItem                   // an existing sequence item ([N] label)
	KindAddNew                    // the virtual "+ add new" row
	KindSeparator                 // ADDED / AVAILABLE / UNKNOWN section header (not selectable)
	KindUnknown                   // a field that is present in the YAML but not in the schema (not togglable)
)

// Node is one entry in the flat DFS list stored by Model.
type Node struct {
	Kind       NodeKind
	YAMLPath   []string // path from block root, e.g. ["source", "filter"]
	Label      string   // display label
	Depth      int
	IsLeaf     bool // scalar/slice/map field - no children to expand
	Openable   bool // map-of-struct field - Enter/→ drills into a child editor
	Checked    bool // field is present in the YAML
	EmptyValue bool // checked leaf whose value is empty (null/""/[]/{}) - pruned at save
	Expanded   bool
	SeqIdx     int             // for KindSeqItem: index in the sequence
	Def        schema.FieldDef // for KindField: the backing field definition
}

// Action is returned by Model.Update to describe what happened.
type Action int

const (
	ActionNone      Action = iota
	ActionToggled          // Enter on an unchecked leaf / ctrl+d on a checked field - checked state changed
	ActionExpanded         // → (or Enter) on a collapsed inline parent
	ActionCollapsed        // ← on an expanded node
	ActionAddNew           // Enter on the KindAddNew row
	ActionDeleted          // ctrl+d on a KindSeqItem row
	ActionOpenChild        // Enter/→ on an openable map-of-struct field - drill in
)

// Model is the tree panel: the bezel tree over yedit's nodes (Nodes, Cursor,
// Offset and Height are its fields), plus what the schema projection needs.
type Model struct {
	tree.Model[Node]

	isSeq bool              // true when the block root is a sequence or map navigator
	defs  []schema.FieldDef // schema defs for object blocks; used by SyncCheckedFromNode to recompute the UNKNOWN section
}

// TreeInfo is what the bezel tree reads from a node: the add-new row never
// expands, separators are skipped by the cursor.
func (n Node) TreeInfo() tree.Info {
	return tree.Info{Depth: n.Depth, Leaf: n.IsLeaf || n.Kind == KindAddNew, Expanded: n.Expanded, Section: n.Kind == KindSeparator}
}

// Expand is the copy the bezel tree stores when a row opens or closes.
func (n Node) Expand(open bool) Node {
	n.Expanded = open
	return n
}

// newModel wraps nodes in the bezel tree with yedit's keys and message.
func newModel(nodes []Node, h int) Model {
	t := tree.New(nodes, h).WithKeys(tree.Keys{Up: keys.Up, Down: keys.Down, Left: keys.Left, Right: keys.Right})
	t.EmptyMessage = "  (no fields)"
	return Model{Model: t}
}

// New builds a Model for a block of the given kind from its schema defs, its
// current YAML content, and the panel height. A kind with no defs (a scalar
// sequence, a free-form map, a primitive) yields an empty tree, which is the
// signal that the YAML editor takes focus directly.
func New(kind schema.Kind, defs []schema.FieldDef, content string, h int) Model {
	tm := newModel(nil, h)

	switch kind {
	case schema.KindList:
		if len(defs) == 0 {
			// Scalar sequence - no tree; YAML editor gets focus directly.
			break
		}
		tm.isSeq = true
		tm.Nodes = CollectionNodes(defs, yamledit.CollValueNode(content, false), false)

	case schema.KindDictionary:
		if len(defs) == 0 {
			break // free-form map (e.g. map[string]string) - no tree; raw YAML
		}
		tm.isSeq = true // collection navigator, keyed by the map key
		tm.Nodes = CollectionNodes(defs, yamledit.CollValueNode(content, true), true)

	case schema.KindObject:
		tm.defs = defs
		tm.Nodes = flattenDefsAsTree(defs, nil, 0)
		valueNode := yamledit.BlockValueNode(content)
		tm.Nodes = DeriveChecked(valueNode, tm.Nodes, false)
		tm.Nodes = applySections(tm.Nodes, collectUnknownNodes(valueNode, defs))
		tm.Nodes = injectNestedUnknowns(tm.Nodes, valueNode, defs)
		// Start cursor on the first selectable node (skip opening separator).
		vis := tm.VisibleNodes()
		for tm.Cursor < len(vis) && tm.Nodes[vis[tm.Cursor]].Kind == KindSeparator {
			tm.Cursor++
		}

	default:
		// KindPrimitive, KindDictionary, KindVariant - no tree nodes; YAML editor gets focus.
	}
	return tm
}

// IsEmpty reports whether the tree has no nodes - true for primitive and
// free-form collection blocks, which have no sub-fields to navigate.
func (tm Model) IsEmpty() bool {
	return len(tm.Nodes) == 0
}

// flattenDefsAsTree converts a []schema.FieldDef into a flat DFS list of
// Nodes, one per field, recursing into inline struct parents only.
const maxTreeDepth = 20

func flattenDefsAsTree(defs []schema.FieldDef, prefix []string, depth int) []Node {
	if depth > maxTreeDepth {
		// Schema depth limit reached — stop recursing to prevent a stack
		// overflow from circular or pathologically deep schema definitions.
		return nil
	}
	var out []Node
	for _, d := range defs {
		path := make([]string, len(prefix)+1)
		copy(path, prefix)
		path[len(prefix)] = d.YAMLName

		openable, isLeaf := shape(d)
		out = append(out, Node{
			Kind:     KindField,
			YAMLPath: path,
			Label:    d.YAMLName,
			Depth:    depth,
			IsLeaf:   isLeaf,
			Openable: openable,
			Expanded: false,
			Def:      d,
		})
		// Only inline struct parents (Inline presentation) get expandable children.
		if !openable && !isLeaf {
			out = append(out, flattenDefsAsTree(d.Children, path, depth+1)...)
		}
	}
	return out
}

// VisibleNodes is the bezel tree's Visible under yedit's older name.
func (tm Model) VisibleNodes() []int { return tm.Visible() }

// CurrentNodeIdx returns the tm.Nodes index under the cursor, or -1.
func (tm Model) CurrentNodeIdx() int { return tm.CurrentIdx() }

// WithNodeMutated keeps the copy-on-write discipline of the bezel tree and
// returns yedit's Model rather than the embedded one.
func (tm Model) WithNodeMutated(idx int, mut func(*Node)) Model {
	tm.Model = tm.Model.WithNodeMutated(idx, mut)
	return tm
}

// ClampCursor returns yedit's Model rather than the embedded one.
func (tm Model) ClampCursor() Model {
	tm.Model = tm.Model.ClampCursor()
	return tm
}

// CursorToSeqItem moves the cursor to the KindSeqItem row with the given
// sequence index, or leaves it unchanged when no such row is visible. Used to
// reconcile the cursor with the loaded entry after a refused navigation.
func (tm Model) CursorToSeqItem(seqIdx int) Model {
	tm.Model, _ = tm.CursorTo(func(n Node) bool { return n.Kind == KindSeqItem && n.SeqIdx == seqIdx })
	return tm
}

// NearestSeqItem returns the seqIdx of the KindSeqItem that is an ancestor
// of the current cursor, or -1 if none.
func (tm Model) NearestSeqItem() int {
	if !tm.isSeq {
		return -1
	}
	idx := tm.CurrentNodeIdx()
	if idx < 0 {
		return -1
	}
	// Walk backwards to find the closest KindSeqItem at depth 0.
	for i := idx; i >= 0; i-- {
		if tm.Nodes[i].Kind == KindSeqItem {
			return tm.Nodes[i].SeqIdx
		}
	}
	return -1
}

// WithDeletedSeqItem removes the seqItem at seqIdx and all its child nodes.
func (tm Model) WithDeletedSeqItem(seqIdx int) Model {
	// Find the range of nodes belonging to this seqItem.
	start := -1
	end := len(tm.Nodes)
	for i, n := range tm.Nodes {
		if n.Kind == KindSeqItem && n.SeqIdx == seqIdx {
			start = i
		} else if start >= 0 && i > start && n.Depth == 0 {
			end = i
			break
		}
	}
	if start < 0 {
		return tm
	}
	nodes := make([]Node, 0, len(tm.Nodes)-(end-start))
	nodes = append(nodes, tm.Nodes[:start]...)
	nodes = append(nodes, tm.Nodes[end:]...)

	// Renumber seqIdx for remaining seqItem nodes.
	counter := 0
	for i, n := range nodes {
		if n.Kind == KindSeqItem {
			nodes[i].SeqIdx = counter
			counter++
		}
	}

	tm.Nodes = nodes
	return tm.ClampCursor()
}

// WithNewSeqItem appends a new seqItem node (collapsed) with child field nodes
// for defs. The caller supplies the item's display label.
func (tm Model) WithNewSeqItem(defs []schema.FieldDef, label string) Model {
	// Insert before KindAddNew (last node).
	newSeqIdx := 0
	for _, n := range tm.Nodes {
		if n.Kind == KindSeqItem {
			newSeqIdx++
		}
	}
	displayLabel := label
	if displayLabel == "" {
		displayLabel = fmt.Sprintf("item %d", newSeqIdx+1)
	}
	seqNode := Node{
		Kind:     KindSeqItem,
		YAMLPath: []string{displayLabel},
		Label:    displayLabel,
		Depth:    0,
		IsLeaf:   false,
		Checked:  true,
		Expanded: true, // expand new items immediately so user sees the fields
		SeqIdx:   newSeqIdx,
	}
	children := flattenDefsAsTree(defs, []string{displayLabel}, 1)

	insertAt := len(tm.Nodes) - 1 // before KindAddNew
	if insertAt < 0 {
		insertAt = 0
	}
	nodes := make([]Node, 0, len(tm.Nodes)+1+len(children))
	for _, n := range tm.Nodes[:insertAt] {
		if n.Kind != KindSeparator {
			nodes = append(nodes, n)
		}
	}
	nodes = append(nodes, seqNode)
	nodes = append(nodes, children...)
	nodes = append(nodes, tm.Nodes[insertAt:]...)
	tm.Nodes = nodes
	return tm.CursorToSeqItem(newSeqIdx)
}

// Update handles all keyboard input for the tree panel. The full (cursor target
// × key) → Action matrix is documented in INTERACTION.md and enforced by
// TestMatrix_TreeActions.
func (tm Model) Update(msg tea.KeyPressMsg) (Model, Action) {
	vis := tm.VisibleNodes()
	if len(vis) == 0 {
		return tm, ActionNone
	}

	switch {
	case key.Matches(msg, keys.Right):
		// → on an openable field drills in; the bezel tree only knows expand.
		if idx := tree.At(tm.Cursor, vis); idx >= 0 && tm.Nodes[idx].Openable {
			return tm, ActionOpenChild
		}
	case key.Matches(msg, keys.Enter):
		return tm.handleEnter(vis)
	case key.Matches(msg, keys.CtrlDRemove):
		return tm.handleRemove(vis)
	}

	var act tree.Action
	tm.Model, act = tm.Model.Update(msg)
	switch act {
	case tree.Expanded:
		return tm, ActionExpanded
	case tree.Collapsed:
		return tm, ActionCollapsed
	}
	return tm, ActionNone
}

// handleEnter adds the field under the cursor (Enter = universal add).
// For KindAddNew it fires ActionAddNew; for unchecked leaf fields it checks
// them; for everything else it does nothing (expand/collapse is arrows-only).
func (tm Model) handleEnter(vis []int) (Model, Action) {
	idx := tree.At(tm.Cursor, vis)
	if idx < 0 {
		return tm, ActionNone
	}
	nd := tm.Nodes[idx]
	switch nd.Kind {
	case KindAddNew:
		return tm, ActionAddNew
	case KindField:
		if nd.Openable {
			return tm, ActionOpenChild
		}
		if !nd.IsLeaf {
			// Inline struct parent: its presence in the YAML is derived from its
			// children (toggling a child auto-creates the parent). Enter expands it
			// like → rather than inserting a stray empty key with a phantom checked
			// state that sync never clears.
			if !nd.Expanded {
				tm = tm.WithNodeMutated(idx, func(n *Node) { n.Expanded = true })
				return tm, ActionExpanded
			}
			return tm, ActionNone
		}
		if !nd.Checked {
			tm = tm.WithNodeMutated(idx, func(n *Node) { n.Checked = true })
			return tm, ActionToggled
		}
	}
	return tm, ActionNone
}

// handleRemove removes the item under the cursor (ctrl+d = universal remove).
// For seq items it fires ActionDeleted; for checked fields it unchecks them.
func (tm Model) handleRemove(vis []int) (Model, Action) {
	idx := tree.At(tm.Cursor, vis)
	if idx < 0 {
		return tm, ActionNone
	}
	nd := tm.Nodes[idx]
	switch nd.Kind {
	case KindSeqItem:
		// Deletion is deferred to the block editor so it can confirm first; the
		// tree is mutated only when the removal is actually performed.
		return tm, ActionDeleted
	case KindField:
		if nd.Checked {
			tm = tm.WithNodeMutated(idx, func(n *Node) { n.Checked = false })
			return tm, ActionToggled
		}
		// A non-leaf parent struct (e.g. hooks.before) carries no checkbox of its
		// own, but ctrl+d should still remove the whole subtree when it holds
		// content. Route through ActionToggled; the block editor confirms removal
		// and deletes the parent mapping by its path.
		if !nd.IsLeaf && hasCheckedDescendant(tm.Nodes, idx) {
			return tm, ActionToggled
		}
	case KindUnknown:
		return tm, ActionToggled
	}
	return tm, ActionNone
}

// pathEqual reports whether two YAMLPath slices are identical.
func pathEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// restoreCursorToPath moves the cursor to the first node whose YAMLPath
// matches path, expanding collapsed ancestors on the way. Copy-on-write: the
// incoming slice may be shared with an undo snapshot.
func (tm Model) restoreCursorToPath(path []string) Model {
	if len(path) == 0 {
		return tm
	}
	tm.Model, _ = tm.Reveal(func(n Node) bool { return pathEqual(n.YAMLPath, path) })
	return tm
}

// hasCheckedDescendant reports whether any leaf descendant of nodes[parentIdx]
// has checked=true. Used to give parent nodes an "existing" colour when they
// contain at least one active field.
func hasCheckedDescendant(nodes []Node, parentIdx int) bool {
	parentDepth := nodes[parentIdx].Depth
	for i := parentIdx + 1; i < len(nodes); i++ {
		if nodes[i].Depth <= parentDepth {
			break
		}
		// Leaves and openable fields (nested collections) both carry a checked
		// state; an openable child holding content counts just like a leaf.
		if (nodes[i].IsLeaf || nodes[i].Openable) && nodes[i].Checked {
			return true
		}
	}
	return false
}

// checkedDescendants computes, for every node, whether hasCheckedDescendant
// would report true - in one pass instead of one subtree walk per parent.
// View calls this once and looks up the result per visible row; calling
// hasCheckedDescendant directly per row would re-walk each parent's subtree
// on every render, which is quadratic for deeply-nested schemas. Ancestor
// bookkeeping is a stack bounded by maxTreeDepth, so this is O(n) in
// practice regardless of nesting shape.
func checkedDescendants(nodes []Node) []bool {
	has := make([]bool, len(nodes))
	var ancestors []int // indices of currently-open ancestors, shallowest first
	for i, n := range nodes {
		for len(ancestors) > 0 && nodes[ancestors[len(ancestors)-1]].Depth >= n.Depth {
			ancestors = ancestors[:len(ancestors)-1]
		}
		if (n.IsLeaf || n.Openable) && n.Checked {
			for _, anc := range ancestors {
				has[anc] = true
			}
		}
		ancestors = append(ancestors, i)
	}
	return has
}

// View renders the tree panel content through the bezel tree.
func (tm Model) View(th theme.Resolved) string {
	checkedDesc := checkedDescendants(tm.Nodes)
	return tm.Model.View(th, func(nd Node, ni int, selected bool) string {
		return nodeLine(nd, ni, selected, th, checkedDesc)
	})
}

// nodeLine renders a single tree row; ni indexes tm.Nodes for descendant
// lookups.
func nodeLine(nd Node, ni int, selected bool, th theme.Resolved, checkedDesc []bool) string {
	switch nd.Kind {
	case KindSeparator:
		if nd.Label == "" {
			return ""
		}
		return th.Section.Render(" " + nd.Label)

	case KindAddNew:
		label := "  [+ add new]"
		if selected {
			return th.Cursor.Render("▶" + label)
		}
		return th.Dim.Render(" " + label)

	case KindSeqItem:
		arrow := "▶"
		if nd.Expanded {
			arrow = "▼"
		}
		label := fmt.Sprintf("%s [%d] %s", arrow, nd.SeqIdx, nd.Label)
		if selected {
			return th.Cursor.Render("▶ " + label)
		}
		return th.Success.Render("  " + label)

	case KindUnknown:
		indent := strings.Repeat("  ", nd.Depth)
		if selected {
			return th.Danger.Render(indent + "▶⚠ " + nd.Label)
		}
		return th.Danger.Render(indent + "⚠ " + nd.Label)
	default: // KindField
		return fieldLine(nd, ni, selected, th, checkedDesc)
	}
}

// fieldLine renders a KindField row, choosing its mark and colour from the
// node's leaf/openable/checked/expanded state.
func fieldLine(nd Node, ni int, selected bool, th theme.Resolved, checkedDesc []bool) string {
	indent := strings.Repeat("  ", nd.Depth)
	var mark string
	switch {
	case nd.Openable:
		mark = "→" // drill-in: opens a nested editor (distinct from inline expand)
	case !nd.IsLeaf && nd.Expanded:
		mark = "▾"
	case !nd.IsLeaf:
		mark = "▸"
	case nd.Checked && nd.EmptyValue:
		mark = "◐" // present but empty: a draft that is pruned at save unless filled
	case nd.Checked:
		mark = "●"
	default:
		mark = "○"
	}
	label := fmt.Sprintf("%s%s %s", indent, mark, nd.Label)
	switch {
	case selected:
		return th.Cursor.Render("▶ " + label)
	case nd.Openable:
		// Openable fields are leaf-like for styling: active when they hold
		// content, muted when empty - never the inline-struct header style.
		if nd.Checked {
			return th.Success.Render("  " + label)
		}
		return th.Dim.Render("  " + label)
	case nd.Checked && nd.EmptyValue:
		// Warning-colored, distinct from both available (unchecked) and existing
		// (committed): it lives under ADDED but will not persist while empty, so
		// it must not read as either "not toggled" or "committed value".
		return th.Warning.Render("  " + label)
	case nd.Checked:
		return th.Success.Render("  " + label)
	case !nd.IsLeaf && checkedDesc[ni]:
		return th.Success.Render("  " + label)
	case !nd.IsLeaf:
		return th.Dim.Render("  " + label)
	default:
		return th.Dim.Render("  " + label)
	}
}

// ExpandsInline reports whether d's children are drawn in the same tree, below
// d, rather than in an editor of their own or not at all.
func ExpandsInline(d schema.FieldDef) bool {
	openable, isLeaf := shape(d)
	return !openable && !isLeaf
}

// shape is how a field sits in the tree: openable into its own editor, and
// whether it is a leaf. Presentation decides; the default derives from Kind.
func shape(d schema.FieldDef) (openable, isLeaf bool) {
	switch d.Presentation {
	case schema.PresentationOverlay:
		openable, isLeaf = true, true
	case schema.PresentationInline:
		openable, isLeaf = false, false
	case schema.PresentationFlat:
		openable, isLeaf = false, true
	default: // PresentationDefault: derive from Kind
		openable = (d.Kind == schema.KindDictionary || d.Kind == schema.KindList) && len(d.Children) > 0
		isLeaf = d.Kind != schema.KindObject || len(d.Children) == 0
	}
	// KindPrimitive is always flat regardless of Presentation.
	if d.Kind == schema.KindPrimitive {
		openable, isLeaf = false, true
	}
	return openable, isLeaf
}
