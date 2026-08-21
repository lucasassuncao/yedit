package fieldtree

// BreadcrumbSegments returns the path components from the block root to the
// current cursor position, suitable for joining with " › ".
func (tm Model) BreadcrumbSegments() []string {
	idx := tm.CurrentNodeIdx()
	if idx < 0 {
		return nil
	}
	n := tm.Nodes[idx]
	switch n.Kind {
	case KindAddNew:
		return []string{"+ add new"}
	case KindSeqItem:
		return []string{n.Label}
	default:
		// YAMLPath is already the full path, and for seq-item children path[0] is
		// the item label, which reads as a breadcrumb segment too.
		return n.YAMLPath
	}
}
