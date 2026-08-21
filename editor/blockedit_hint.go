package editor

import (
	"strings"

	"github.com/lucasassuncao/yedit/fieldtree"
	"github.com/lucasassuncao/yedit/hint"
)

// fieldItemView renders the left panel of a tree-less block as a single
// non-toggleable row naming the field. There is nothing to navigate, so the row
// is just an anchor; the metadata lives in the Hint/Example panel.
func (be blockEditState) fieldItemView() string {
	return be.theme.ExistingItem.Render(" ▸ " + be.key)
}

// scrolledHintContent clips the hint content to hintH() lines starting at
// hintScroll, for when the hint panel has focus.
func (be blockEditState) scrolledHintContent() string {
	content := be.hintContent()
	if content == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	h := be.hintH()
	maxScroll := len(lines) - h
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := be.hintScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + h
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[scroll:end], "\n")
}

func (be blockEditState) hintContent() string {
	// Tree-less blocks have no field nodes, so show the block's own metadata. The
	// empty fieldPath makes this a block-level lookup, like the root list's hint
	// panel; be.def.YAMLName would be misread as a child path and never resolve.
	if be.tree.IsEmpty() {
		return be.fieldHintFor("")
	}
	idx := be.tree.CurrentNodeIdx()
	if idx < 0 {
		return be.theme.HintDim.Render("  select a field to see hints")
	}
	node := be.tree.Nodes[idx]

	switch node.Kind {
	case fieldtree.KindUnknown:
		return be.theme.UnknownItem.Render("⚠ unknown key - not declared in the schema\n remove it before saving")
	case fieldtree.KindField:
		// handled below
	default:
		return be.theme.HintDim.Render("  select a field to see hints")
	}

	fieldPath := strings.Join(node.YAMLPath, ".")
	if be.isCollectionNav() && len(node.YAMLPath) > 0 {
		fieldPath = strings.Join(node.YAMLPath[1:], ".")
	}
	return be.fieldHintFor(fieldPath)
}

// fieldHintFor builds the hint text for the field at fieldPath, a dot-joined
// path from the block root (e.g. "source.path").
func (be blockEditState) fieldHintFor(fieldPath string) string {
	if be.cfg.Metadata == nil {
		return be.theme.HintDim.Render("  Config.Metadata is not set - no metadata source configured")
	}
	meta := be.cfg.Metadata.FieldMeta(be.key, fieldPath)
	ex := meta.Example
	if ex == "" && meta.Multiline {
		// An empty fieldPath means the block's own metadata, so fall back to the
		// block key to keep the generated example named.
		fieldName := fieldPath
		switch {
		case fieldName == "":
			fieldName = be.key
		case strings.LastIndex(fieldPath, ".") >= 0:
			fieldName = fieldPath[strings.LastIndex(fieldPath, ".")+1:]
		}
		ex = fieldName + ": |\n  line 1\n  line 2\n"
	}
	if out := hint.Render(be.theme, meta, ex); out != "" {
		return out
	}
	return be.theme.HintDim.Render("  no metadata declared for this field")
}
