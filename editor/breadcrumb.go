package editor

import (
	"strings"

	"github.com/lucasassuncao/yedit/theme"
)

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
		// The last segment equals the child editor's be.key and would duplicate it,
		// so keep only the leading ones (e.g. "[0]" for collection entries).
		sub := be.tree.BreadcrumbSegments()
		if len(sub) > 1 {
			segs = append(segs, sub[:len(sub)-1]...)
		}
	}
	return segs
}

// renderHeader builds the root screen's header line from the config title and
// the document's path/dirty state.
func renderHeader(title, file string, dirty bool, width int, th theme.Resolved) string {
	info := file
	if dirty {
		info = file + " ● modified"
	}
	return theme.RenderHeaderWith(title, info, "", width, th.Colors)
}

// breadcrumbHeader builds a block editor's header line from parentSegs plus this
// editor's own key and tree position.
func (be blockEditState) breadcrumbHeader(parentSegs []string) string {
	segs := append(append([]string(nil), parentSegs...), be.key)
	segs = append(segs, be.tree.BreadcrumbSegments()...)
	return theme.RenderHeaderWith(be.cfg.Title, strings.Join(segs, " › "), "", be.width, be.theme.Colors)
}
