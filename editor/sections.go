package editor

// sectionChunk groups a depth-0 node with all of its depth>0 descendants.
type sectionChunk struct {
	nodes      []treeNode
	hasContent bool
}

// applySections splits depth-0 field chunks into an ADDED group (has content)
// and an AVAILABLE group (all unchecked), injecting treeNodeSeparator headers.
// unknownNodes (treeNodeUnknown rows produced by collectUnknownNodes) are
// appended after AVAILABLE in a dedicatted UNKNOWN section.
// It is idempotent: existing separators are stripped before re-applying.
// Only used for KindObject trees (not KindList).
func applySections(nodes []treeNode, unknownNodes []treeNode) []treeNode {
	clean := stripSeparators(nodes) // also removes stale treeNodeUnknown rows
	chunks := buildChunks(clean)
	classifyChunks(chunks)

	existing, available, addNew := partitionChunks(chunks)

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
	if len(unknownNodes) > 0 {
		result = append(result, sep(""))
		result = append(result, sep("UNKNOWN"))
		result = append(result, unknownNodes...)
	}
	return result
}

// stripSeparators returns a copy of nodes with all treeNodeSeparator and
// treeNodeUnknown entries removed. This makes applySections idempotent: stale
// UNKNOWN rows from the previous sync are discarded before the section is
// rebuilt from the current YAML state.
func stripSeparators(nodes []treeNode) []treeNode {
	out := make([]treeNode, 0, len(nodes))
	for _, n := range nodes {
		if n.kind != treeNodeSeparator && n.kind != treeNodeUnknown {
			out = append(out, n)
		}
	}
	return out
}

// buildChunks groups consecutive nodes into depth-0-rooted chunks.
// Each chunk is one depth-0 node followed by all depth>0 descendants.
func buildChunks(nodes []treeNode) []sectionChunk {
	var chunks []sectionChunk
	var cur *sectionChunk
	for _, n := range nodes {
		if n.depth == 0 {
			if cur != nil {
				chunks = append(chunks, *cur)
			}
			cur = &sectionChunk{nodes: []treeNode{n}}
		} else if cur != nil {
			cur.nodes = append(cur.nodes, n)
		}
	}
	if cur != nil {
		chunks = append(chunks, *cur)
	}
	return chunks
}

// classifyChunks sets hasContent on each chunk in-place.
func classifyChunks(chunks []sectionChunk) {
	for i, c := range chunks {
		root := c.nodes[0]
		if root.kind == treeNodeAddNew {
			continue
		}
		chunks[i].hasContent = chunkHasContent(root, c.nodes[1:])
	}
}

// chunkHasContent reports whether a chunk carries any user-supplied data.
func chunkHasContent(root treeNode, children []treeNode) bool {
	if root.isLeaf || root.openable {
		return root.checked
	}
	// Inline struct parent: the key's presence in YAML (root.checked) is enough
	// signal, and we also check leaf/openable descendants for robustness.
	if root.checked {
		return true
	}
	for _, n := range children {
		if (n.isLeaf || n.openable) && n.checked {
			return true
		}
	}
	return false
}

// partitionChunks splits chunks into three buckets: existing (has content),
// available (no content), and addNew (treeNodeAddNew sentinel).
func partitionChunks(chunks []sectionChunk) (existing, available, addNew []sectionChunk) {
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
	return existing, available, addNew
}
