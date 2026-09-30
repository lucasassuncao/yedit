// Package blocklist is the root editor's left panel: the schema's top-level
// keys and the document's blocks, projected onto a bezel list.
package blocklist

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/lucasassuncao/bezel/list"
	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/keys"
)

// Item represents one row in the left panel of the root editor view.
type Item struct {
	Key      string
	Existing bool
	Unknown  bool // present in YAML but not in schema; not openable, excluded from AddedCount
	// Passthrough marks a key from Config.PassthroughKeys. It is deliberately
	// preserved rather than a problem, so it is styled apart from Unknown even
	// though it also sets Unknown: there is no schema to open either way.
	Passthrough bool
	Separator   bool // visual divider row, not selectable
}

// OpenItemMsg is sent when the user presses Enter on a list item.
type OpenItemMsg struct{ Item Item }

// DeleteItemMsg is sent when the user presses d on an existing item.
type DeleteItemMsg struct{ Key string }

// Model is the scrollable left-panel list of known + existing top-level keys.
type Model struct {
	knownKeys   []string // canonical order from the schema
	passthrough map[string]bool
	items       []Item
	list        list.Model
}

// IsFiltering reports whether the list is in text-filter mode (/ was pressed).
func (lm Model) IsFiltering() bool { return lm.list.IsFiltering() }

// BuildItems merges the canonical key order with the document's blocks,
// keeping existing keys in file order above the available ones. The caller
// strips hidden keys beforehand.
func BuildItems(knownKeys []string, existing []document.Block, passthrough map[string]bool) []Item {
	knownSet := make(map[string]bool, len(knownKeys))
	for _, k := range knownKeys {
		knownSet[k] = true
	}

	existingSet := make(map[string]bool, len(existing))
	for _, b := range existing {
		if knownSet[b.Key] {
			existingSet[b.Key] = true
		}
	}

	items := make([]Item, 0, len(knownKeys)+4)

	existingItems := make([]Item, 0)
	for _, b := range existing {
		if knownSet[b.Key] {
			existingItems = append(existingItems, Item{Key: b.Key, Existing: true})
		}
	}
	if len(existingItems) > 0 {
		items = append(items, Item{Separator: true, Key: "ADDED"})
		items = append(items, existingItems...)
	}

	// AVAILABLE keeps the schema's canonical order, matching where Insert places
	// new blocks.
	available := make([]string, 0)
	for _, k := range knownKeys {
		if !existingSet[k] {
			available = append(available, k)
		}
	}

	if len(available) > 0 {
		items = append(items, Item{Separator: true, Key: ""})
		items = append(items, Item{Separator: true, Key: "AVAILABLE"})
		for _, k := range available {
			items = append(items, Item{Key: k, Existing: false})
		}
	}

	// UNKNOWN: in the file, absent from the schema, and not declared passthrough.
	var unknownItems []Item
	for _, b := range existing {
		if !knownSet[b.Key] && !passthrough[b.Key] {
			unknownItems = append(unknownItems, Item{Key: b.Key, Existing: true, Unknown: true})
		}
	}
	if len(unknownItems) > 0 {
		items = append(items, Item{Separator: true, Key: ""})
		items = append(items, Item{Separator: true, Key: "UNKNOWN"})
		items = append(items, unknownItems...)
	}

	// PASSTHROUGH: declared passthrough, in the file, and not in the schema. Keys
	// in both the schema and the passthrough list belong to ADDED/AVAILABLE and
	// must not be duplicated here.
	var passthroughItems []Item
	for _, b := range existing {
		if passthrough[b.Key] && !knownSet[b.Key] {
			passthroughItems = append(passthroughItems, Item{Key: b.Key, Existing: true, Unknown: true, Passthrough: true})
		}
	}
	if len(passthroughItems) > 0 {
		items = append(items, Item{Separator: true, Key: ""})
		items = append(items, Item{Separator: true, Key: "PASSTHROUGH"})
		items = append(items, passthroughItems...)
	}

	return items
}

// rows projects items onto list rows: the mark says what the key is, the
// style comes from the theme at render time.
func rows(items []Item, th *theme.Resolved) []list.Row {
	out := make([]list.Row, len(items))
	for i, it := range items {
		r := list.Row{Label: it.Key, Section: it.Separator, Value: it}
		switch {
		case it.Separator:
		case it.Passthrough:
			r.Mark, r.Style = "○", &th.Dim
		case it.Unknown:
			r.Mark, r.Style = "⚠", &th.Danger
		case it.Existing:
			r.Mark, r.Style = "●", &th.Success
		default:
			r.Mark = "+"
		}
		out[i] = r
	}
	return out
}

// New builds the list for the schema's keys and the document's blocks. th
// colours the rows; it is fixed for the editor's life.
func New(knownKeys []string, existing []document.Block, passthrough map[string]bool, height int, th theme.Resolved) Model {
	items := BuildItems(knownKeys, existing, passthrough)
	return Model{
		knownKeys:   knownKeys,
		passthrough: passthrough,
		items:       items,
		list:        list.New(rows(items, &th), height).WithKeys(listKeys()),
	}
}

// listKeys are yedit's bindings for what the list does itself. → opens a
// block too, as it opens a node in the block editor's tree.
func listKeys() list.Keys {
	open := key.NewBinding(key.WithKeys(append(keys.Enter.Keys(), "right")...))
	return list.Keys{Up: keys.Up, Down: keys.Down, Enter: open, Filter: keys.Filter, Esc: keys.Esc}
}

// SetHeight updates the visible row count and re-clamps the scroll offset.
func (lm Model) SetHeight(h int) Model {
	lm.list = lm.list.SetHeight(h)
	return lm
}

// Rebuild refreshes the list after blocks change without losing cursor position.
func (lm Model) Rebuild(existing []document.Block, th theme.Resolved) Model {
	lm.items = BuildItems(lm.knownKeys, existing, lm.passthrough)
	lm.list = lm.list.SetRows(rows(lm.items, &th))
	return lm
}

// AddedCount returns how many recognised top-level keys are present in the doc.
func (lm Model) AddedCount() int {
	n := 0
	for _, it := range lm.items {
		if it.Existing && !it.Unknown {
			n++
		}
	}
	return n
}

// SelectedItem returns the item under the cursor, or nil on a separator or an
// empty list. In filter mode it follows the filter cursor instead.
func (lm Model) SelectedItem() *Item {
	r := lm.list.Selected()
	if r == nil {
		return nil
	}
	it := r.Value.(Item)
	return &it
}

// ItemByKey returns the Item for key, or a zero Item when absent.
// Separators are skipped so a block named like a section label ("ADDED") cannot
// match one.
func (lm Model) ItemByKey(key string) Item {
	for _, it := range lm.items {
		if !it.Separator && it.Key == key {
			return it
		}
	}
	return Item{Key: key}
}

// Update handles keyboard input. Delete is yedit's own key; enter opens what
// the list chose, unless it is an unknown key with no schema to open.
func (lm Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return lm, nil
	}
	if !lm.IsFiltering() && key.Matches(km, keys.CtrlDDelete) {
		if it := lm.SelectedItem(); it != nil && it.Existing {
			k := it.Key
			return lm, func() tea.Msg { return DeleteItemMsg{Key: k} }
		}
		return lm, nil
	}
	var action list.Action
	lm.list, action = lm.list.Update(km)
	if action == list.Chosen {
		if it := lm.SelectedItem(); it != nil && !it.Unknown {
			item := *it
			return lm, func() tea.Msg { return OpenItemMsg{Item: item} }
		}
	}
	return lm, nil
}

// View renders the scrollable list or the filter prompt, depending on mode.
func (lm Model) View(th theme.Resolved) string { return lm.list.View(th) }

// KnownCount is how many keys the schema declares, filtered or not. Pair it
// with AddedCount for a "3/12" style counter.
func (lm Model) KnownCount() int { return len(lm.knownKeys) }

// IsPassthrough reports whether key is carried through untouched rather than
// being part of the editable schema.
func (lm Model) IsPassthrough(key string) bool { return lm.passthrough[key] }

// Filter is the text typed in filtering mode, empty when not filtering.
func (lm Model) Filter() string { return lm.list.Filter() }
