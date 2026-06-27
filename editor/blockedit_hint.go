package editor

import "strings"

// fieldItemView renders the left panel for a tree-less block (primitive, enum,
// or free-form collection): a single non-toggleable row naming the field being
// edited. There are no sub-fields to navigate, so the row is just an anchor -
// the field's metadata lives in the Hint/Example panel.
func (be blockEditState) fieldItemView() string {
	return be.theme.existingItem.Render(" ▸ " + be.key)
}

// hintContent returns the rendered string for the bottom-right hint panel.
// scrolledHintContent returns the hint content clipped to hintH() lines,
// starting at hintScroll. Used when hint panel has focus for scrolling.
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
	// Tree-less blocks (primitive/enum/free-form collection) have no field nodes;
	// show the block's own metadata instead of the "select a field" placeholder.
	if be.tree.isEmpty() {
		return be.fieldHintFor(be.def.YAMLName)
	}
	idx := be.tree.currentNodeIdx()
	if idx < 0 {
		return be.theme.hintDim.Render("  select a field to see hints")
	}
	node := be.tree.nodes[idx]

	switch node.kind {
	case treeNodeUnknown:
		return be.theme.unknownItem.Render("⚠ unknown key - not declare in the schema\n remove it before saving")
	case treeNodeField:
		// handled below
	default:
		return be.theme.hintDim.Render("  select a field to see hints")
	}

	fieldPath := strings.Join(node.yamlPath, ".")
	if be.isCollectionNav() && len(node.yamlPath) > 0 {
		fieldPath = strings.Join(node.yamlPath[1:], ".")
	}
	return be.fieldHintFor(fieldPath)
}

// fieldHintFor builds the hint text for a single field definition.
// fieldPath is the dot-joined path from the block root (e.g. "source.path").
func (be blockEditState) fieldHintFor(fieldPath string) string {
	if be.cfg.Metadata == nil {
		return be.theme.hintDim.Render("  Config.Metadata is not set - no metadata source configured")
	}
	meta := be.cfg.Metadata.FieldMeta(be.key, fieldPath)
	ex := meta.Example
	if ex == "" && meta.Multiline {
		fieldName := fieldPath
		if i := strings.LastIndex(fieldPath, "."); i >= 0 {
			fieldName = fieldPath[i+1:]
		}
		ex = fieldName + ": |\n  line 1\n  line 2\n"
	}
	if out := renderFieldHint(be.theme, meta, ex); out != "" {
		return out
	}
	return be.theme.hintDim.Render("  no metadata declared for this field")
}
