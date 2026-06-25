package editor

import (
	"strings"

	"github.com/lucasassuncao/yedit/theme"
)

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

// blockBreadcrumbPrefix returns the breadcrumb segments for all editors in the
// stack except the top one. The top editor appends its own key and tree segments.
func (m model) blockBreadcrumbPrefix() []string {
	n := len(m.blockEdits)
	if n <= 1 {
		return nil
	}
	var segs []string
	for _, be := range m.blockEdits[:n-1] {
		segs = append(segs, be.key)
		// BreadcrumbSegments returns the path to the field the user drilled into.
		// Its last element equals the child editor's be.key and would duplicate it,
		// so only the leading segments (e.g. "[0]" for collection entries) are kept.
		sub := be.tree.BreadcrumbSegments()
		if len(sub) > 1 {
			segs = append(segs, sub[:len(sub)-1]...)
		}
	}
	return segs
}

// renderHeader builds the root screen's header line from the config title and
// the document's path/dirty state.
func renderHeader(title, file string, dirty bool, width int, th resolvedTheme) string {
	info := file
	if dirty {
		info = file + " ● modified"
	}
	return theme.RenderHeaderWith(title, info, "", width, th.colors)
}

// breadcrumbHeader builds a block editor's header line: parentSegs (from
// model.blockBreadcrumbPrefix) plus this editor's own key and tree position.
func (be blockEditState) breadcrumbHeader(parentSegs []string) string {
	segs := append(append(parentSegs, be.key), be.tree.BreadcrumbSegments()...)
	return theme.RenderHeaderWith(be.cfg.Title, strings.Join(segs, " › "), "", be.width, be.theme.colors)
}
