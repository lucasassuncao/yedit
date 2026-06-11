package editor

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/schema"
)

// treeNodeKind classifies each row in the tree-view left panel.
type treeNodeKind int

const (
	treeNodeField     treeNodeKind = iota // a struct field (leaf or expandable struct)
	treeNodeSeqItem                       // an existing sequence item ([N] label)
	treeNodeAddNew                        // the virtual "+ add new" row
	treeNodeSeparator                     // ADDED / AVAILABLE section header (not selectable)
)

// treeNode is one entry in the flat DFS list stored by treeModel.
type treeNode struct {
	kind     treeNodeKind
	yamlPath []string // path from block root, e.g. ["source", "filter"]
	label    string   // display label
	depth    int
	isLeaf   bool // scalar/slice/map field - no children to expand
	openable bool // map-of-struct field - Enter/→ drills into a child editor
	checked  bool // field is present in the YAML
	expanded bool
	seqIdx   int             // for treeNodeSeqItem: index in the sequence
	def      schema.FieldDef // for treeNodeField: the backing field definition
}

// treeAction is returned by treeModel.Update to describe what happened.
type treeAction int

const (
	treeNoAction  treeAction = iota
	treeToggled              // Space on a leaf - checked state changed
	treeExpanded             // → on a collapsible node
	treeCollapsed            // ← on an expanded node
	treeAddNew               // Space/Enter on the treeNodeAddNew row
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
	isSeq    bool   // true when the block root is KindSlice
	emptyMsg string // shown when nodes is empty; defaults to "(no fields)"
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
		tm.nodes = buildSeqNodesFromNode(spec.defs, collValueNode(spec.content, false))

	case schema.KindDictionary:
		if len(spec.defs) == 0 {
			break // free-form map (e.g. map[string]string) - no tree; raw YAML
		}
		tm.isSeq = true // collection navigator, keyed by the map key
		tm.nodes = buildMapNodesFromNode(spec.defs, collValueNode(spec.content, true))

	case schema.KindObject:
		tm.nodes = flattenDefsAsTree(spec.defs, nil, 0)
		tm.nodes = deriveChecked(blockValueNode(spec.content), tm.nodes, false)
		tm.nodes = applySections(tm.nodes)
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
func flattenDefsAsTree(defs []schema.FieldDef, prefix []string, depth int) []treeNode {
	var out []treeNode
	for _, d := range defs {
		path := make([]string, len(prefix)+1)
		copy(path, prefix)
		path[len(prefix)] = d.YAMLName

		isLeaf := d.Kind != schema.KindObject || len(d.Children) == 0
		// A map[string]Struct or []Struct field has no inline children, but can be
		// opened in a dedicated editor: pressing Enter/→ drills into the collection.
		openable := (d.Kind == schema.KindDictionary || d.Kind == schema.KindList) && len(d.Children) > 0
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
		// Only inline struct parents (KindObject) get expandable children in the
		// tree. Openable collections (list/map of struct) drill into a dedicated
		// editor instead, so they carry no inline children.
		if d.Kind == schema.KindObject && len(d.Children) > 0 {
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

// BreadcrumbSegments returns the path components from the block root to the
// current cursor position, suitable for joining with " › ".
func (tm treeModel) BreadcrumbSegments() []string {
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return nil
	}
	n := tm.nodes[idx]
	switch n.kind {
	case treeNodeAddNew:
		return []string{"+ add new"}
	case treeNodeSeqItem:
		return []string{n.label}
	default:
		// yamlPath already has the full path; for seq-item children path[0] is
		// the item label, which serves as a breadcrumb segment too.
		return n.yamlPath
	}
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
	nodes = append(nodes, tm.nodes[:insertAt]...)
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

	switch msg.String() {
	case "up":
		return tm.moveUp(), treeNoAction
	case "down":
		return tm.moveDown(), treeNoAction
	case "right":
		return tm.handleRight()
	case "left":
		return tm.handleLeft(vis)
	case "enter":
		return tm.handleEnter()
	case "ctrl+d":
		return tm.handleRemove()
	}

	return tm, treeNoAction
}

func (tm treeModel) moveUp() treeModel {
	vis := tm.visibleNodes()
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
	if tm.cursor < tm.offset {
		tm.offset = tm.cursor
	}
	return tm
}

func (tm treeModel) moveDown() treeModel {
	vis := tm.visibleNodes()
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
	if tm.cursor >= tm.offset+tm.height {
		tm.offset = tm.cursor - tm.height + 1
	}
	return tm
}

// clampOffset scrolls the viewport so the current cursor row stays visible.
func (tm treeModel) clampOffset() treeModel {
	if tm.cursor < tm.offset {
		tm.offset = tm.cursor
	}
	if tm.height > 0 && tm.cursor >= tm.offset+tm.height {
		tm.offset = tm.cursor - tm.height + 1
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
		nodes := make([]treeNode, len(tm.nodes))
		copy(nodes, tm.nodes)
		nodes[idx].expanded = true
		tm.nodes = nodes
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
		nodes := make([]treeNode, len(tm.nodes))
		copy(nodes, tm.nodes)
		nodes[idx].expanded = false
		tm.nodes = nodes
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
				nodes := make([]treeNode, len(tm.nodes))
				copy(nodes, tm.nodes)
				nodes[idx].expanded = true
				tm.nodes = nodes
				return tm, treeExpanded
			}
			return tm, treeNoAction
		}
		if !nd.checked {
			nodes := make([]treeNode, len(tm.nodes))
			copy(nodes, tm.nodes)
			nodes[idx].checked = true
			tm.nodes = nodes
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
			nodes := make([]treeNode, len(tm.nodes))
			copy(nodes, tm.nodes)
			nodes[idx].checked = false
			tm.nodes = nodes
			return tm, treeToggled
		}
		// A non-leaf parent struct (e.g. hooks.before) carries no checkbox of its
		// own, but ctrl+d should still remove the whole subtree when it holds
		// content. Route through treeToggled; the block editor confirms removal
		// and deletes the parent mapping by its path.
		if !nd.isLeaf && hasCheckedDescendant(tm.nodes, idx) {
			return tm, treeToggled
		}
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
	for vi, ni := range tm.visibleNodes() {
		if pathEqual(tm.nodes[ni].yamlPath, path) {
			tm.cursor = vi
			if tm.cursor < tm.offset {
				tm.offset = tm.cursor
			} else if tm.height > 0 && tm.cursor >= tm.offset+tm.height {
				tm.offset = tm.cursor - tm.height + 1
			}
			return tm
		}
	}
	return tm
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
	case nd.checked:
		return th.existingItem.Render("  " + label)
	case !nd.isLeaf && hasCheckedDescendant(tm.nodes, ni):
		return th.existingItem.Render("  " + label)
	case !nd.isLeaf:
		return th.sectionLabel.Render(" " + label) // PaddingLeft(1) + 1 sp = 2 cells, matches cursor prefix
	default:
		return th.availableItem.Render("  " + label)
	}
}
