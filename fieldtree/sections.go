package fieldtree

// sectionChunk groups a depth-0 node with all of its depth>0 descendants.
type sectionChunk struct {
	nodes      []Node
	hasContent bool
}

// applySections splits depth-0 field chunks into an ADDED group (has content)
// and an AVAILABLE group (all unchecked), injecting KindSeparator headers.
// unknownNodes (KindUnknown rows produced by collectUnknownNodes) are
// appended after AVAILABLE in a dedicated UNKNOWN section.
// It is idempotent: existing separators are stripped before re-applying.
// Only used for KindObject trees (not KindList).
func applySections(nodes []Node, unknownNodes []Node) []Node {
	clean := stripSeparators(nodes) // also removes stale KindUnknown rows
	chunks := buildChunks(clean)
	classifyChunks(chunks)

	existing, available, addNew := partitionChunks(chunks)

	sep := func(label string) Node {
		return Node{Kind: KindSeparator, Label: label, Depth: 0, IsLeaf: true}
	}
	var result []Node
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

// stripSeparators returns a copy of nodes with all KindSeparator and
// KindUnknown entries removed. This makes applySections idempotent: stale
// UNKNOWN rows from the previous sync are discarded before the section is
// rebuilt from the current YAML state.
func stripSeparators(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Kind != KindSeparator && n.Kind != KindUnknown {
			out = append(out, n)
		}
	}
	return out
}

// buildChunks groups consecutive nodes into depth-0-rooted chunks.
// Each chunk is one depth-0 node followed by all depth>0 descendants.
func buildChunks(nodes []Node) []sectionChunk {
	var chunks []sectionChunk
	var cur *sectionChunk
	for _, n := range nodes {
		if n.Depth == 0 {
			if cur != nil {
				chunks = append(chunks, *cur)
			}
			cur = &sectionChunk{nodes: []Node{n}}
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
		if root.Kind == KindAddNew {
			continue
		}
		chunks[i].hasContent = chunkHasContent(root, c.nodes[1:])
	}
}

// chunkHasContent reports whether a chunk carries any user-supplied data.
func chunkHasContent(root Node, children []Node) bool {
	if root.IsLeaf || root.Openable {
		return root.Checked
	}
	// Inline struct parent: the key's presence in YAML (root.Checked) is enough
	// signal, and we also check leaf/openable descendants for robustness.
	if root.Checked {
		return true
	}
	for _, n := range children {
		if (n.IsLeaf || n.Openable) && n.Checked {
			return true
		}
	}
	return false
}

// partitionChunks splits chunks into three buckets: existing (has content),
// available (no content), and addNew (KindAddNew sentinel).
func partitionChunks(chunks []sectionChunk) (existing, available, addNew []sectionChunk) {
	for _, c := range chunks {
		switch {
		case c.nodes[0].Kind == KindAddNew:
			addNew = append(addNew, c)
		case c.hasContent:
			existing = append(existing, c)
		default:
			available = append(available, c)
		}
	}
	return existing, available, addNew
}
