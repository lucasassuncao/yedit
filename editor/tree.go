package editor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
	"github.com/lucasassuncao/yedit/theme"
)

// treeNodeKind classifies each row in the tree-view left panel.
type treeNodeKind int

const (
	treeNodeField     treeNodeKind = iota // a struct field (leaf or expandable struct)
	treeNodeSeqItem                       // an existing sequence item ([N] label)
	treeNodeAddNew                        // the virtual "+ add new" row
	treeNodeSeparator                     // ADDED / AVAILABLE / UNKNOWN section header (not selectable)
	treeNodeUnknown                       // a field that is present in the YAML but not in the schema (not togglable)
)

// treeNode is one entry in the flat DFS list stored by treeModel.
type treeNode struct {
	kind       treeNodeKind
	yamlPath   []string // path from block root, e.g. ["source", "filter"]
	label      string   // display label
	depth      int
	isLeaf     bool // scalar/slice/map field - no children to expand
	openable   bool // map-of-struct field - Enter/→ drills into a child editor
	checked    bool // field is present in the YAML
	emptyValue bool // checked leaf whose value is empty (null/""/[]/{}) - pruned at save
	expanded   bool
	seqIdx     int             // for treeNodeSeqItem: index in the sequence
	def        schema.FieldDef // for treeNodeField: the backing field definition
}

// treeAction is returned by treeModel.Update to describe what happened.
type treeAction int

const (
	treeNoAction  treeAction = iota
	treeToggled              // Enter on an unchecked leaf / ctrl+d on a checked field - checked state changed
	treeExpanded             // → (or Enter) on a collapsed inline parent
	treeCollapsed            // ← on an expanded node
	treeAddNew               // Enter on the treeNodeAddNew row
	treeDeleted              // ctrl+d on a treeNodeSeqItem row
	treeOpenChild            // Enter/→ on an openable map-of-struct field - drill in
)

// treeModel is the unified left-panel component that replaces fieldListModel,
// seqItemListModel, and composeListModel.
type treeModel struct {
	nodes    []treeNode // nodes in display order (existing chunks first, then available)
	cursor   int        // position within the visible list
	offset   int        // scroll offset within the visible list
	height   int
	isSeq    bool              // true when the block root is KindSlice
	emptyMsg string            // shown when nodes is empty; defaults to "(no fields)"
	defs     []schema.FieldDef // schema defs for KindObject blocks; used by syncTreeCheckedFromNode to recompute the UNKNOWN section
}

// newTreeModel builds a treeModel from a blockSpec and a panel height.
func newTreeModel(spec blockSpec, h int) treeModel {
	tm := treeModel{height: h}

	switch spec.kind {
	case schema.KindList:
		if len(spec.defs) == 0 {
			// Scalar sequence - no tree; YAML editor gets focus directly.
			break
		}
		tm.isSeq = true
		tm.nodes = buildCollectionNodesFromNode(spec.defs, collValueNode(spec.content, false), false)

	case schema.KindDictionary:
		if len(spec.defs) == 0 {
			break // free-form map (e.g. map[string]string) - no tree; raw YAML
		}
		tm.isSeq = true // collection navigator, keyed by the map key
		tm.nodes = buildCollectionNodesFromNode(spec.defs, collValueNode(spec.content, true), true)

	case schema.KindObject:
		tm.defs = spec.defs
		tm.nodes = flattenDefsAsTree(spec.defs, nil, 0)
		valueNode := blockValueNode(spec.content)
		tm.nodes = deriveChecked(valueNode, tm.nodes, false)
		tm.nodes = applySections(tm.nodes, collectUnknownNodes(valueNode, spec.defs))
		tm.nodes = injectNestedUnknowns(tm.nodes, valueNode, spec.defs)
		// Start cursor on the first selectable node (skip opening separator).
		vis := tm.visibleNodes()
		for tm.cursor < len(vis) && tm.nodes[vis[tm.cursor]].kind == treeNodeSeparator {
			tm.cursor++
		}

	default:
		// KindPrimitive, KindDictionary, KindVariant - no tree nodes; YAML editor gets focus.
	}
	return tm
}

// isEmpty reports whether the tree has no nodes - true for primitive and
// free-form collection blocks, which have no sub-fields to navigate.
func (tm treeModel) isEmpty() bool {
	return len(tm.nodes) == 0
}

// flattenDefsAsTree converts a []schema.FieldDef into a flat DFS list of
// treeNodes, mirroring composeListModel.flattenDefs but producing treeNode.
const maxTreeDepth = 20

func flattenDefsAsTree(defs []schema.FieldDef, prefix []string, depth int) []treeNode {
	if depth > maxTreeDepth {
		// Schema depth limit reached — stop recursing to prevent a stack
		// overflow from circular or pathologically deep schema definitions.
		return nil
	}
	var out []treeNode
	for _, d := range defs {
		path := make([]string, len(prefix)+1)
		copy(path, prefix)
		path[len(prefix)] = d.YAMLName

		var openable, isLeaf bool
		switch d.Presentation {
		case schema.PresentationOverlay:
			openable = true
			isLeaf = true
		case schema.PresentationInline:
			openable = false
			isLeaf = false
		case schema.PresentationFlat:
			openable = false
			isLeaf = true
		default: // PresentationDefault: derive from Kind
			openable = (d.Kind == schema.KindDictionary || d.Kind == schema.KindList) && len(d.Children) > 0
			isLeaf = d.Kind != schema.KindObject || len(d.Children) == 0
		}
		// KindPrimitive is always flat regardless of Presentation.
		if d.Kind == schema.KindPrimitive {
			openable = false
			isLeaf = true
		}

		out = append(out, treeNode{
			kind:     treeNodeField,
			yamlPath: path,
			label:    d.YAMLName,
			depth:    depth,
			isLeaf:   isLeaf,
			openable: openable,
			expanded: false,
			def:      d,
		})
		// Only inline struct parents (Inline presentation) get expandable children.
		if !openable && !isLeaf {
			out = append(out, flattenDefsAsTree(d.Children, path, depth+1)...)
		}
	}
	return out
}

// visibleNodes returns the indices into tm.nodes that should be rendered,
// respecting each node's collapsed/expanded state.
func (tm treeModel) visibleNodes() []int {
	var vis []int
	// Stack holds the depths of collapsed ancestors; while non-empty we are
	// inside a collapsed subtree and skip nodes.
	var stack []int

	for i, n := range tm.nodes {
		for len(stack) > 0 && n.depth <= stack[len(stack)-1] {
			stack = stack[:len(stack)-1]
		}
		if len(stack) > 0 {
			continue
		}
		vis = append(vis, i)
		if !n.isLeaf && !n.expanded && n.kind != treeNodeAddNew {
			stack = append(stack, n.depth)
		}
	}
	return vis
}

// currentNodeIdx returns the tm.nodes index under the cursor, or -1.
func (tm treeModel) currentNodeIdx() int {
	vis := tm.visibleNodes()
	if tm.cursor >= 0 && tm.cursor < len(vis) {
		return vis[tm.cursor]
	}
	return -1
}

// withNodeMutated returns tm with a freshly cloned nodes slice in which
// nodes[idx] has been modified by mut. It keeps the tree's copy-on-write
// discipline: callers never mutate the shared backing array in place.
func (tm treeModel) withNodeMutated(idx int, mut func(*treeNode)) treeModel {
	if idx < 0 || idx >= len(tm.nodes) {
		return tm // stale index; no-op instead of panic
	}
	nodes := make([]treeNode, len(tm.nodes))
	copy(nodes, tm.nodes)
	mut(&nodes[idx])
	tm.nodes = nodes
	return tm
}

// cursorToSeqItem moves the cursor to the treeNodeSeqItem row with the given
// sequence index, or leaves it unchanged when no such row is visible. Used to
// reconcile the cursor with the loaded entry after a refused navigation.
func (tm treeModel) cursorToSeqItem(seqIdx int) treeModel {
	for vi, ni := range tm.visibleNodes() {
		if tm.nodes[ni].kind == treeNodeSeqItem && tm.nodes[ni].seqIdx == seqIdx {
			tm.cursor = vi
			return tm.clampOffset()
		}
	}
	return tm
}

// NearestSeqItem returns the seqIdx of the treeNodeSeqItem that is an ancestor
// of the current cursor, or -1 if none.
func (tm treeModel) NearestSeqItem() int {
	if !tm.isSeq {
		return -1
	}
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return -1
	}
	// Walk backwards to find the closest treeNodeSeqItem at depth 0.
	for i := idx; i >= 0; i-- {
		if tm.nodes[i].kind == treeNodeSeqItem {
			return tm.nodes[i].seqIdx
		}
	}
	return -1
}

// WithDeletedSeqItem removes the seqItem at seqIdx and all its child nodes.
func (tm treeModel) WithDeletedSeqItem(seqIdx int) treeModel {
	// Find the range of nodes belonging to this seqItem.
	start := -1
	end := len(tm.nodes)
	for i, n := range tm.nodes {
		if n.kind == treeNodeSeqItem && n.seqIdx == seqIdx {
			start = i
		} else if start >= 0 && i > start && n.depth == 0 {
			end = i
			break
		}
	}
	if start < 0 {
		return tm
	}
	nodes := make([]treeNode, 0, len(tm.nodes)-(end-start))
	nodes = append(nodes, tm.nodes[:start]...)
	nodes = append(nodes, tm.nodes[end:]...)

	// Renumber seqIdx for remaining seqItem nodes.
	counter := 0
	for i, n := range nodes {
		if n.kind == treeNodeSeqItem {
			nodes[i].seqIdx = counter
			counter++
		}
	}

	tm.nodes = nodes
	// Adjust cursor.
	vis := tm.visibleNodes()
	if tm.cursor >= len(vis) {
		tm.cursor = len(vis) - 1
		if tm.cursor < 0 {
			tm.cursor = 0
		}
	}
	return tm
}

// WithNewSeqItem appends a new seqItem node (collapsed) with child field nodes
// for defs. The caller supplies the item's display label.
func (tm treeModel) WithNewSeqItem(defs []schema.FieldDef, label string) treeModel {
	// Insert before treeNodeAddNew (last node).
	newSeqIdx := 0
	for _, n := range tm.nodes {
		if n.kind == treeNodeSeqItem {
			newSeqIdx++
		}
	}
	displayLabel := label
	if displayLabel == "" {
		displayLabel = fmt.Sprintf("item %d", newSeqIdx+1)
	}
	seqNode := treeNode{
		kind:     treeNodeSeqItem,
		yamlPath: []string{displayLabel},
		label:    displayLabel,
		depth:    0,
		isLeaf:   false,
		checked:  true,
		expanded: true, // expand new items immediately so user sees the fields
		seqIdx:   newSeqIdx,
	}
	children := flattenDefsAsTree(defs, []string{displayLabel}, 1)

	insertAt := len(tm.nodes) - 1 // before treeNodeAddNew
	if insertAt < 0 {
		insertAt = 0
	}
	nodes := make([]treeNode, 0, len(tm.nodes)+1+len(children))
	for _, n := range tm.nodes[:insertAt] {
		if n.kind != treeNodeSeparator {
			nodes = append(nodes, n)
		}
	}
	nodes = append(nodes, seqNode)
	nodes = append(nodes, children...)
	nodes = append(nodes, tm.nodes[insertAt:]...)
	tm.nodes = nodes

	// Move cursor to the new seqItem.
	vis := tm.visibleNodes()
	for vi, ni := range vis {
		if tm.nodes[ni].kind == treeNodeSeqItem && tm.nodes[ni].seqIdx == newSeqIdx {
			tm.cursor = vi
			break
		}
	}
	return tm
}

// Update handles all keyboard input for the tree panel. The full (cursor target
// × key) → treeAction matrix is documented in INTERACTION.md and enforced by
// TestMatrix_TreeActions.
func (tm treeModel) Update(msg tea.KeyMsg) (treeModel, treeAction) {
	vis := tm.visibleNodes()
	n := len(vis)
	if n == 0 {
		return tm, treeNoAction
	}

	switch {
	case key.Matches(msg, kbUp):
		return tm.moveUp(), treeNoAction
	case key.Matches(msg, kbDown):
		return tm.moveDown(), treeNoAction
	case key.Matches(msg, kbRight):
		return tm.handleRight()
	case key.Matches(msg, kbLeft):
		return tm.handleLeft(vis)
	case key.Matches(msg, kbEnter):
		return tm.handleEnter()
	case key.Matches(msg, kbCtrlDRemove):
		return tm.handleRemove()
	}

	return tm, treeNoAction
}

func (tm treeModel) moveUp() treeModel {
	vis := tm.visibleNodes()
	if len(vis) == 0 {
		return tm
	}
	if tm.cursor >= len(vis) {
		tm.cursor = len(vis) - 1
	}
	start := tm.cursor
	for tm.cursor > 0 {
		tm.cursor--
		if tm.nodes[vis[tm.cursor]].kind != treeNodeSeparator {
			break
		}
	}
	// If no non-separator was found above, stay put.
	if tm.nodes[vis[tm.cursor]].kind == treeNodeSeparator {
		tm.cursor = start
	}
	tm.offset = theme.ClampScroll(tm.cursor, tm.offset, tm.height)
	return tm
}

func (tm treeModel) moveDown() treeModel {
	vis := tm.visibleNodes()
	if len(vis) == 0 {
		return tm
	}
	if tm.cursor >= len(vis) {
		tm.cursor = len(vis) - 1
	}
	start := tm.cursor
	// Move down, skipping separators, while staying within bounds
	for tm.cursor+1 < len(vis) {
		tm.cursor++
		if tm.nodes[vis[tm.cursor]].kind != treeNodeSeparator {
			break
		}
	}
	// If we're now on a separator (or couldn't move), stay at the original position
	if tm.cursor < len(vis) && tm.nodes[vis[tm.cursor]].kind == treeNodeSeparator {
		tm.cursor = start
	}
	tm.offset = theme.ClampScroll(tm.cursor, tm.offset, tm.height)
	return tm
}

// clampOffset scrolls the viewport so the current cursor row stays visible.
func (tm treeModel) clampOffset() treeModel {
	tm.offset = theme.ClampScroll(tm.cursor, tm.offset, tm.height)
	return tm
}

// clampCursor forces the cursor back into the visible range. An empty tree
// leaves the cursor at 0 (harmless: every consumer guards len(vis)==0). Used
// after state restores (undo/redo) where a snapshot's cursor may no longer be
// valid against the restored node set.
func (tm treeModel) clampCursor() treeModel {
	vis := tm.visibleNodes()
	switch {
	case len(vis) == 0:
		tm.cursor = 0
	case tm.cursor < 0:
		tm.cursor = 0
	case tm.cursor >= len(vis):
		tm.cursor = len(vis) - 1
	}
	return tm
}

func (tm treeModel) handleRight() (treeModel, treeAction) {
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return tm, treeNoAction
	}
	if tm.nodes[idx].openable {
		return tm, treeOpenChild
	}
	if idx >= 0 && !tm.nodes[idx].isLeaf && !tm.nodes[idx].expanded &&
		tm.nodes[idx].kind != treeNodeAddNew {
		tm = tm.withNodeMutated(idx, func(n *treeNode) { n.expanded = true })
		return tm, treeExpanded
	}
	return tm, treeNoAction
}

func (tm treeModel) handleLeft(vis []int) (treeModel, treeAction) {
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return tm, treeNoAction
	}
	nd := tm.nodes[idx]
	if !nd.isLeaf && nd.expanded {
		tm = tm.withNodeMutated(idx, func(n *treeNode) { n.expanded = false })
		return tm, treeCollapsed
	}
	if nd.depth > 0 {
		for vi := tm.cursor - 1; vi >= 0; vi-- {
			if tm.nodes[vis[vi]].depth == nd.depth-1 {
				tm.cursor = vi
				tm = tm.clampOffset()
				break
			}
		}
	}
	return tm, treeNoAction
}

// handleEnter adds the field under the cursor (Enter = universal add).
// For treeNodeAddNew it fires treeAddNew; for unchecked leaf fields it checks
// them; for everything else it does nothing (expand/collapse is arrows-only).
func (tm treeModel) handleEnter() (treeModel, treeAction) {
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return tm, treeNoAction
	}
	nd := tm.nodes[idx]
	switch nd.kind {
	case treeNodeAddNew:
		return tm, treeAddNew
	case treeNodeField:
		if nd.openable {
			return tm, treeOpenChild
		}
		if !nd.isLeaf {
			// Inline struct parent: its presence in the YAML is derived from its
			// children (toggling a child auto-creates the parent). Enter expands it
			// like → rather than inserting a stray empty key with a phantom checked
			// state that sync never clears.
			if !nd.expanded {
				tm = tm.withNodeMutated(idx, func(n *treeNode) { n.expanded = true })
				return tm, treeExpanded
			}
			return tm, treeNoAction
		}
		if !nd.checked {
			tm = tm.withNodeMutated(idx, func(n *treeNode) { n.checked = true })
			return tm, treeToggled
		}
	}
	return tm, treeNoAction
}

// handleRemove removes the item under the cursor (ctrl+d = universal remove).
// For seq items it fires treeDeleted; for checked fields it unchecks them.
func (tm treeModel) handleRemove() (treeModel, treeAction) {
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return tm, treeNoAction
	}
	nd := tm.nodes[idx]
	switch nd.kind {
	case treeNodeSeqItem:
		// Deletion is deferred to the block editor so it can confirm first; the
		// tree is mutated only when the removal is actually performed.
		return tm, treeDeleted
	case treeNodeField:
		if nd.checked {
			tm = tm.withNodeMutated(idx, func(n *treeNode) { n.checked = false })
			return tm, treeToggled
		}
		// A non-leaf parent struct (e.g. hooks.before) carries no checkbox of its
		// own, but ctrl+d should still remove the whole subtree when it holds
		// content. Route through treeToggled; the block editor confirms removal
		// and deletes the parent mapping by its path.
		if !nd.isLeaf && hasCheckedDescendant(tm.nodes, idx) {
			return tm, treeToggled
		}
	case treeNodeUnknown:
		return tm, treeToggled
	}
	return tm, treeNoAction
}

// pathEqual reports whether two yamlPath slices are identical.
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

// restoreCursorToPath moves the cursor to the first visible node whose
// yamlPath matches path. Used after node reordering to keep the selection stable.
func (tm treeModel) restoreCursorToPath(path []string) treeModel {
	if len(path) == 0 {
		return tm
	}
	// First pass: try the visible nodes directly (common case — already expanded).
	for vi, ni := range tm.visibleNodes() {
		if pathEqual(tm.nodes[ni].yamlPath, path) {
			tm.cursor = vi
			tm.offset = theme.ClampScroll(tm.cursor, tm.offset, tm.height)
			return tm
		}
	}
	// Second pass: the target node may be hidden under a collapsed ancestor.
	// Expand any ancestor whose path is a prefix of the target path so the
	// node becomes visible, then retry. Clone before the first mutation: the
	// incoming slice may be shared (e.g. with an undo snapshot), and the tree's
	// copy-on-write discipline forbids writing through a shared backing array.
	var nodes []treeNode
	for i, n := range tm.nodes {
		if n.kind != treeNodeField || n.isLeaf || n.expanded {
			continue
		}
		if isPathPrefix(n.yamlPath, path) {
			if nodes == nil {
				nodes = make([]treeNode, len(tm.nodes))
				copy(nodes, tm.nodes)
			}
			nodes[i].expanded = true
		}
	}
	if nodes == nil {
		return tm // node is not in the tree at all; leave cursor unchanged
	}
	tm.nodes = nodes
	for vi, ni := range tm.visibleNodes() {
		if pathEqual(tm.nodes[ni].yamlPath, path) {
			tm.cursor = vi
			tm.offset = theme.ClampScroll(tm.cursor, tm.offset, tm.height)
			return tm
		}
	}
	return tm
}

// isPathPrefix reports whether prefix is a strict prefix of path (i.e. prefix
// has fewer elements and every element matches).
func isPathPrefix(prefix, path []string) bool {
	if len(prefix) == 0 || len(prefix) >= len(path) {
		return false
	}
	for i, p := range prefix {
		if path[i] != p {
			return false
		}
	}
	return true
}

// hasCheckedDescendant reports whether any leaf descendant of nodes[parentIdx]
// has checked=true. Used to give parent nodes an "existing" colour when they
// contain at least one active field.
func hasCheckedDescendant(nodes []treeNode, parentIdx int) bool {
	parentDepth := nodes[parentIdx].depth
	for i := parentIdx + 1; i < len(nodes); i++ {
		if nodes[i].depth <= parentDepth {
			break
		}
		// Leaves and openable fields (nested collections) both carry a checked
		// state; an openable child holding content counts just like a leaf.
		if (nodes[i].isLeaf || nodes[i].openable) && nodes[i].checked {
			return true
		}
	}
	return false
}

// View renders the tree panel content.
func (tm treeModel) View(th resolvedTheme) string {
	vis := tm.visibleNodes()
	if len(vis) == 0 {
		msg := "  (no fields)"
		if tm.emptyMsg != "" {
			msg = tm.emptyMsg
		}
		return th.availableItem.Render(msg)
	}

	// Reserve last row for a scroll indicator when items overflow below.
	maxVisible := tm.height
	hasMore := tm.offset+tm.height < len(vis)
	if hasMore {
		maxVisible = tm.height - 1
	}

	end := tm.offset + maxVisible
	if end > len(vis) {
		end = len(vis)
	}

	var sb strings.Builder
	for vi := tm.offset; vi < end; vi++ {
		ni := vis[vi]
		sb.WriteString(tm.nodeLine(tm.nodes[ni], ni, vi, th) + "\n")
	}

	if hasMore {
		remaining := len(vis) - end
		sb.WriteString(th.availableItem.Render(fmt.Sprintf("  ↓ %d more", remaining)))
	} else {
		rendered := end - tm.offset
		for i := rendered; i < tm.height; i++ {
			sb.WriteByte('\n')
		}
	}

	return strings.TrimSuffix(sb.String(), "\n")
}

// nodeLine renders a single tree row. vi is the visible index (compared against
// the cursor); ni indexes tm.nodes (for descendant lookups).
func (tm treeModel) nodeLine(nd treeNode, ni, vi int, th resolvedTheme) string {
	switch nd.kind {
	case treeNodeSeparator:
		if nd.label == "" {
			return ""
		}
		return th.sectionLabel.Render(" " + nd.label)

	case treeNodeAddNew:
		label := "  [+ add new]"
		if vi == tm.cursor {
			return th.selectedItem.Render("▶" + label)
		}
		return th.availableItem.Render(" " + label)

	case treeNodeSeqItem:
		arrow := "▶"
		if nd.expanded {
			arrow = "▼"
		}
		label := fmt.Sprintf("%s [%d] %s", arrow, nd.seqIdx, nd.label)
		if vi == tm.cursor {
			return th.selectedItem.Render("▶ " + label)
		}
		return th.existingItem.Render("  " + label)

	case treeNodeUnknown:
		indent := strings.Repeat("  ", nd.depth)
		if vi == tm.cursor {
			return th.unknownItem.Render(indent + "▶⚠ " + nd.label)
		}
		return th.unknownItem.Render(indent + "⚠ " + nd.label)
	default: // treeNodeField
		return tm.fieldLine(nd, ni, vi, th)
	}
}

// fieldLine renders a treeNodeField row, choosing its mark and colour from the
// node's leaf/openable/checked/expanded state.
func (tm treeModel) fieldLine(nd treeNode, ni, vi int, th resolvedTheme) string {
	indent := strings.Repeat("  ", nd.depth)
	var mark string
	switch {
	case nd.openable:
		mark = "→" // drill-in: opens a nested editor (distinct from inline expand)
	case !nd.isLeaf && nd.expanded:
		mark = "▾"
	case !nd.isLeaf:
		mark = "▸"
	case nd.checked && nd.emptyValue:
		mark = "◌" // present but empty: a draft that is pruned at save unless filled
	case nd.checked:
		mark = "●"
	default:
		mark = "○"
	}
	label := fmt.Sprintf("%s%s %s", indent, mark, nd.label)
	switch {
	case vi == tm.cursor:
		return th.selectedItem.Render("▶ " + label)
	case nd.openable:
		// Openable fields are leaf-like for styling: active when they hold
		// content, muted when empty - never the inline-struct header style.
		if nd.checked {
			return th.existingItem.Render("  " + label)
		}
		return th.availableItem.Render("  " + label)
	case nd.checked && nd.emptyValue:
		// Muted like an available field: it lives under ADDED but will not persist
		// while empty, so it must not read as a committed value.
		return th.availableItem.Render("  " + label)
	case nd.checked:
		return th.existingItem.Render("  " + label)
	case !nd.isLeaf && hasCheckedDescendant(tm.nodes, ni):
		return th.existingItem.Render("  " + label)
	case !nd.isLeaf:
		return th.availableItem.Render("  " + label)
	default:
		return th.availableItem.Render("  " + label)
	}
}
