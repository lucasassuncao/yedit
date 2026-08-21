package editor

import (
	"github.com/lucasassuncao/yedit/hint"
)

// selectedHint renders the Hint/Example panel body for the selected list item.
// All display data comes from MetadataSource.
func (m model) selectedHint() string {
	if m.cfg.Metadata == nil {
		return m.theme.HintDim.Render("  Config.Metadata is not set - no metadata source configured")
	}
	it := m.list.SelectedItem()
	if it == nil || it.Separator {
		return m.theme.HintDim.Render("  select a field to see hints")
	}
	if it.Unknown {
		return m.theme.HintDim.Render("  unknown key - not in the schema")
	}
	def := fieldDefByName(m.schemaTree, it.Key)
	if def.YAMLName == "" {
		def.YAMLName = it.Key
	}
	meta := m.cfg.Metadata.FieldMeta(it.Key, "")
	ex := meta.Example
	if ex == "" && meta.Multiline {
		ex = it.Key + ": |\n  line 1\n  line 2\n"
	}
	if out := hint.Render(m.theme, meta, ex); out != "" {
		return out
	}
	return m.theme.HintDim.Render("  no metadata declared for this field")
}
