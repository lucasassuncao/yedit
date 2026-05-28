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
	isLeaf   bool // scalar/slice/map field — no children to expand
	checked  bool // field is present in the YAML
	expanded bool
	seqIdx   int             // for treeNodeSeqItem: index in the sequence
	def      schema.FieldDef // for treeNodeField: the backing field definition
}

// treeAction is returned by treeModel.Update to describe what happened.
type treeAction int

const (
	treeNoAction  treeAction = iota
	treeToggled              // Space on a leaf — checked state changed
	treeExpanded             // → on a collapsible node
	treeCollapsed            // ← on an expanded node
	treeAddNew               // Space/Enter on the treeNodeAddNew row
	treeDeleted              // ctrl+d on a treeNodeSeqItem row
)

// treeModel is the unified left-panel component that replaces fieldListModel,
// seqItemListModel, and composeListModel.
type treeModel struct {
	nodes  []treeNode // nodes in display order (existing chunks first, then available)
	cursor int        // position within the visible list
	offset int        // scroll offset within the visible list
	height int
	isSeq  bool // true when the block root is KindSlice
}

// newTreeModel builds a treeModel from a blockSpec and a panel height.
func newTreeModel(spec blockSpec, h int) treeModel {
	tm := treeModel{height: h}

	switch spec.kind {
	case schema.KindSlice:
		if len(spec.defs) == 0 {
			// Scalar sequence — no tree; YAML editor gets focus directly.
			break
		}
		tm.isSeq = true
		entries := parseSeqEntries(spec.key, spec.content)
		tm.nodes = buildSeqNodes(spec.defs, entries)

	case schema.KindStruct:
		tm.nodes = flattenDefsAsTree(spec.defs, nil, 0)
		tm.nodes = syncTreeCheckedStates(tm.nodes, spec.key, spec.content)
		tm.nodes = applySections(tm.nodes)
		// Start cursor on the first selectable node (skip opening separator).
		vis := tm.visibleNodes()
		for tm.cursor < len(vis) && tm.nodes[vis[tm.cursor]].kind == treeNodeSeparator {
			tm.cursor++
		}

	default:
		// KindScalar, KindMap, KindUnion — no tree nodes; YAML editor gets focus.
	}
	return tm
}

// buildSeqNodes creates the node list for a sequence block:
// one treeNodeSeqItem per existing entry (collapsed), then treeNodeAddNew.
func buildSeqNodes(childDefs []schema.FieldDef, entries []seqEntry) []treeNode {
	var nodes []treeNode
	for i, e := range entries {
		seqNode := treeNode{
			kind:     treeNodeSeqItem,
			yamlPath: []string{e.Label},
			label:    e.Label,
			depth:    0,
			isLeaf:   false,
			checked:  true,
			expanded: false,
			seqIdx:   i,
		}
		nodes = append(nodes, seqNode)
		// Append child field nodes for this item (hidden until expanded).
		children := flattenDefsAsTree(childDefs, []string{e.Label}, 1)
		children = syncTreeCheckedStates(children, "", "x:\n"+e.Content)
		nodes = append(nodes, children...)
	}
	nodes = append(nodes, treeNode{
		kind:   treeNodeAddNew,
		label:  "+ add new",
		depth:  0,
		isLeaf: true,
	})
	return nodes
}

// flattenDefsAsTree converts a []schema.FieldDef into a flat DFS list of
// treeNodes, mirroring composeListModel.flattenDefs but producing treeNode.
func flattenDefsAsTree(defs []schema.FieldDef, prefix []string, depth int) []treeNode {
	var out []treeNode
	for _, d := range defs {
		path := make([]string, len(prefix)+1)
		copy(path, prefix)
		path[len(prefix)] = d.YAMLName

		isLeaf := d.Kind != schema.KindStruct || len(d.Children) == 0
		out = append(out, treeNode{
			kind:     treeNodeField,
			yamlPath: path,
			label:    d.YAMLName,
			depth:    depth,
			isLeaf:   isLeaf,
			expanded: false,
			def:      d,
		})
		if !isLeaf && len(d.Children) > 0 {
			out = append(out, flattenDefsAsTree(d.Children, path, depth+1)...)
		}
	}
	return out
}

// syncTreeCheckedStates updates the checked field on leaf nodes by parsing
// yamlContent. For sequence items the key is "" and content is "x:\n<item>".
func syncTreeCheckedStates(nodes []treeNode, key, yamlContent string) []treeNode {
	if yamlContent == "" {
		return nodes
	}
	var doc map[string]any
	if err := yamlUnmarshal([]byte(yamlContent), &doc); err != nil {
		return nodes
	}

	// Navigate to the sub-map under key (or "x" for seq item content).
	var sub map[string]any
	if key == "" {
		sub, _ = doc["x"].(map[string]any)
		if sub == nil {
			// doc["x"] might be a slice element — handle []any
			if items, ok := doc["x"].([]any); ok && len(items) > 0 {
				sub, _ = items[0].(map[string]any)
			}
		}
	} else {
		sub, _ = doc[key].(map[string]any)
	}

	result := make([]treeNode, len(nodes))
	copy(result, nodes)
	for i, n := range result {
		if n.kind != treeNodeField || !n.isLeaf {
			continue
		}
		// Walk the path through sub to see if the leaf key exists.
		cur := sub
		path := n.yamlPath
		// For seq item children only (key==""), path[0] is the item label, not a
		// YAML key — skip it. Regular nested struct fields must NOT skip path[0].
		startIdx := 0
		if key == "" && n.depth > 0 {
			startIdx = 1
		}
		for j := startIdx; j < len(path)-1 && cur != nil; j++ {
			cur, _ = cur[path[j]].(map[string]any)
		}
		if cur != nil && len(path) > startIdx {
			_, result[i].checked = cur[path[len(path)-1]]
		}
	}
	return result
}

// visibleNodes returns the indices into tm.nodes that should be rendered,
// respecting each node's collapsed/expanded state.
func (tm treeModel) visibleNodes() []int {
	var vis []int
	// collapsedAt tracks when we are inside a collapsed subtree.
	// We use depth to detect exit from the collapsed region.
	type collapseFrame struct {
		depth int
		kind  treeNodeKind
	}
	var stack []collapseFrame

	for i, n := range tm.nodes {
		// Pop stack entries whose depth is >= current node's depth.
		for len(stack) > 0 && n.depth <= stack[len(stack)-1].depth {
			stack = stack[:len(stack)-1]
		}
		// If we are inside a collapsed node, skip.
		if len(stack) > 0 {
			continue
		}
		vis = append(vis, i)
		// If this node is collapsible and not expanded, push to stack.
		if !n.isLeaf && !n.expanded && n.kind != treeNodeAddNew {
			stack = append(stack, collapseFrame{depth: n.depth, kind: n.kind})
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

// Update handles all keyboard input for the tree panel.
func (tm treeModel) Update(msg tea.KeyMsg) (treeModel, treeAction) {
	vis := tm.visibleNodes()
	n := len(vis)
	if n == 0 {
		return tm, treeNoAction
	}

	switch msg.String() {
	case "up", "k":
		return tm.moveUp(), treeNoAction
	case "down", "j":
		return tm.moveDown(n), treeNoAction
	case "right", "l":
		return tm.handleRight()
	case "left", "h":
		return tm.handleLeft(vis)
	case " ":
		return tm.handleSpace()
	case "enter":
		return tm.handleEnter()
	case "ctrl+d":
		idx := tm.currentNodeIdx()
		if idx >= 0 && tm.nodes[idx].kind == treeNodeSeqItem {
			tm = tm.WithDeletedSeqItem(tm.nodes[idx].seqIdx)
			return tm, treeDeleted
		}
	case "g":
		tm.cursor = 0
		tm.offset = 0
		for tm.cursor < n-1 && tm.nodes[vis[tm.cursor]].kind == treeNodeSeparator {
			tm.cursor++
		}
		return tm, treeNoAction
	case "G":
		if n > 0 {
			tm.cursor = n - 1
			for tm.cursor > 0 && tm.nodes[vis[tm.cursor]].kind == treeNodeSeparator {
				tm.cursor--
			}
			if tm.cursor >= tm.offset+tm.height {
				tm.offset = tm.cursor - tm.height + 1
			}
		}
		return tm, treeNoAction
	}

	return tm, treeNoAction
}

func (tm treeModel) moveUp() treeModel {
	vis := tm.visibleNodes()
	for tm.cursor > 0 {
		tm.cursor--
		if tm.nodes[vis[tm.cursor]].kind != treeNodeSeparator {
			break
		}
	}
	if tm.cursor < tm.offset {
		tm.offset = tm.cursor
	}
	return tm
}

func (tm treeModel) moveDown(n int) treeModel {
	vis := tm.visibleNodes()
	for tm.cursor < n-1 {
		tm.cursor++
		if tm.nodes[vis[tm.cursor]].kind != treeNodeSeparator {
			break
		}
	}
	if tm.cursor >= tm.offset+tm.height {
		tm.offset = tm.cursor - tm.height + 1
	}
	return tm
}

func (tm treeModel) handleRight() (treeModel, treeAction) {
	idx := tm.currentNodeIdx()
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
				if tm.cursor < tm.offset {
					tm.offset = tm.cursor
				}
				break
			}
		}
	}
	return tm, treeNoAction
}

func (tm treeModel) handleSpace() (treeModel, treeAction) {
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return tm, treeNoAction
	}
	nd := tm.nodes[idx]
	// Space = toggle only. [+ add new] and navigation are Enter's job.
	if nd.kind == treeNodeField && nd.isLeaf {
		nodes := make([]treeNode, len(tm.nodes))
		copy(nodes, tm.nodes)
		nodes[idx].checked = !nodes[idx].checked
		tm.nodes = nodes
		return tm, treeToggled
	}
	return tm, treeNoAction
}

func (tm treeModel) handleEnter() (treeModel, treeAction) {
	idx := tm.currentNodeIdx()
	if idx < 0 {
		return tm, treeNoAction
	}
	nd := tm.nodes[idx]
	switch nd.kind {
	case treeNodeAddNew:
		return tm, treeAddNew

	case treeNodeSeqItem:
		nodes := make([]treeNode, len(tm.nodes))
		copy(nodes, tm.nodes)
		nodes[idx].expanded = !nodes[idx].expanded
		tm.nodes = nodes
		if nodes[idx].expanded {
			return tm, treeExpanded
		}
		return tm, treeCollapsed

	case treeNodeField:
		if nd.isLeaf {
			return tm.handleSpace() // leaf: Enter = toggle (same as Space)
		}
		nodes := make([]treeNode, len(tm.nodes))
		copy(nodes, tm.nodes)
		nodes[idx].expanded = !nodes[idx].expanded
		tm.nodes = nodes
		if nodes[idx].expanded {
			return tm, treeExpanded
		}
		return tm, treeCollapsed
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

// applySections splits depth-0 field chunks into an ADDED group (has content)
// and an AVAILABLE group (all unchecked), injecting treeNodeSeparator headers.
// It is idempotent: existing separators are stripped before re-applying.
// Only used for KindStruct trees (not KindSlice).
func applySections(nodes []treeNode) []treeNode {
	// Strip existing separators.
	clean := make([]treeNode, 0, len(nodes))
	for _, n := range nodes {
		if n.kind != treeNodeSeparator {
			clean = append(clean, n)
		}
	}

	// Extract depth-0 chunks: each chunk is [depth-0 node + its depth>0 children].
	type chunk struct {
		nodes      []treeNode
		hasContent bool
	}
	var chunks []chunk
	var cur *chunk
	for _, n := range clean {
		if n.depth == 0 {
			if cur != nil {
				chunks = append(chunks, *cur)
			}
			cur = &chunk{nodes: []treeNode{n}}
		} else if cur != nil {
			cur.nodes = append(cur.nodes, n)
		}
	}
	if cur != nil {
		chunks = append(chunks, *cur)
	}

	// Classify each chunk.
	for i, c := range chunks {
		root := c.nodes[0]
		if root.kind == treeNodeAddNew {
			continue
		}
		if root.isLeaf {
			chunks[i].hasContent = root.checked
		} else {
			for _, n := range c.nodes[1:] {
				if n.isLeaf && n.checked {
					chunks[i].hasContent = true
					break
				}
			}
		}
	}

	// Partition into existing / available / addNew.
	var existing, available, addNew []chunk
	for _, c := range chunks {
		switch {
		case c.nodes[0].kind == treeNodeAddNew:
			addNew = append(addNew, c)
		case c.hasContent:
			existing = append(existing, c)
		default:
			available = append(available, c)
		}
	}

	sep := func(label string) treeNode {
		return treeNode{kind: treeNodeSeparator, label: label, depth: 0, isLeaf: true}
	}
	var result []treeNode
	if len(existing) > 0 {
		result = append(result, sep("ADDED"))
		for _, c := range existing {
			result = append(result, c.nodes...)
		}
	}
	if len(available) > 0 {
		if len(existing) > 0 {
			result = append(result, sep("")) // blank line between sections
		}
		result = append(result, sep("AVAILABLE"))
		for _, c := range available {
			result = append(result, c.nodes...)
		}
	}
	for _, c := range addNew {
		result = append(result, c.nodes...)
	}
	return result
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
		if nodes[i].isLeaf && nodes[i].checked {
			return true
		}
	}
	return false
}

// View renders the tree panel content.
func (tm treeModel) View() string {
	vis := tm.visibleNodes()
	if len(vis) == 0 {
		return availableItemStyle.Render("  (no fields)")
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
		nd := tm.nodes[ni]

		var line string
		switch nd.kind {
		case treeNodeSeparator:
			if nd.label == "" {
				line = ""
			} else {
				line = sectionLabelStyle.Render(" " + nd.label)
			}

		case treeNodeAddNew:
			label := "  [+ add new]"
			if vi == tm.cursor {
				line = selectedItemStyle.Render("▶" + label)
			} else {
				line = availableItemStyle.Render(" " + label)
			}

		case treeNodeSeqItem:
			var arrow string
			if nd.expanded {
				arrow = "▼"
			} else {
				arrow = "▶"
			}
			label := fmt.Sprintf("%s [%d] %s", arrow, nd.seqIdx, nd.label)
			if vi == tm.cursor {
				line = selectedItemStyle.Render("▶ " + label)
			} else {
				line = existingItemStyle.Render("  " + label)
			}

		default: // treeNodeField
			indent := strings.Repeat("  ", nd.depth)
			var mark string
			switch {
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
				line = selectedItemStyle.Render("▶ " + label)
			case nd.checked:
				line = existingItemStyle.Render("  " + label)
			case !nd.isLeaf && hasCheckedDescendant(tm.nodes, ni):
				line = existingItemStyle.Render("  " + label)
			case !nd.isLeaf:
				line = sectionLabelStyle.Render(" " + label) // PaddingLeft(1) + 1 sp = 2 cells, matches cursor prefix
			default:
				line = availableItemStyle.Render("  " + label)
			}
		}

		sb.WriteString(line + "\n")
	}

	if hasMore {
		remaining := len(vis) - end
		sb.WriteString(availableItemStyle.Render(fmt.Sprintf("  ↓ %d more", remaining)))
	} else {
		rendered := end - tm.offset
		for i := rendered; i < tm.height; i++ {
			sb.WriteByte('\n')
		}
	}

	return strings.TrimSuffix(sb.String(), "\n")
}
