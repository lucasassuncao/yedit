package editor

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
