package editor

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lucasassuncao/yedit/document"
	"github.com/lucasassuncao/yedit/theme"
)

// listItem represents one row in the left panel of the root editor view.
type listItem struct {
	Key       string
	Existing  bool
	Unknown   bool // key present in YAML but not in schema
	Separator bool // visual divider row, not selectable
}

// openItemMsg is sent when the user presses Enter on a list item.
type openItemMsg struct{ Item listItem }

// deleteItemMsg is sent when the user presses d on an existing item.
type deleteItemMsg struct{ Key string }

// listModel is the scrollable left-panel list of known + existing top-level keys.
type listModel struct {
	knownKeys   []string // canonical order from the schema
	passthrough map[string]bool
	items       []listItem
	cursor      int
	height      int
	offset      int

	filter    string
	filtering bool
	fCursor   int
	fOffset   int
}

// IsFiltering reports whether the list is in text-filter mode (/ was pressed).
func (lm listModel) IsFiltering() bool { return lm.filtering }

// buildListItems merges the canonical key order with the blocks present in the
// current document, keeping existing keys (in file order) above available keys
// (alphabetised). Hidden keys are stripped beforehand by the caller.
func buildListItems(knownKeys []string, existing []document.Block, passthrough map[string]bool) []listItem {
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

	items := make([]listItem, 0, len(knownKeys)+4)

	existingItems := make([]listItem, 0)
	for _, b := range existing {
		if knownSet[b.Key] {
			existingItems = append(existingItems, listItem{Key: b.Key, Existing: true})
		}
	}
	if len(existingItems) > 0 {
		items = append(items, listItem{Separator: true, Key: "ADDED"})
		items = append(items, existingItems...)
	}

	// AVAILABLE keys keep the schema's canonical order (knownKeys), matching the
	// order in which Insert places new blocks.
	available := make([]string, 0)
	for _, k := range knownKeys {
		if !existingSet[k] {
			available = append(available, k)
		}
	}

	if len(available) > 0 {
		items = append(items, listItem{Separator: true, Key: ""})
		items = append(items, listItem{Separator: true, Key: "AVAILABLE"})
		for _, k := range available {
			items = append(items, listItem{Key: k, Existing: false})
		}
	}

	// UNKNOWN: blocks present in the file but absent from the schema and not
	// declared as passthrough keys (which are silently preserved).
	var unknownItems []listItem
	for _, b := range existing {
		if !knownSet[b.Key] && !passthrough[b.Key] {
			unknownItems = append(unknownItems, listItem{Key: b.Key, Existing: true, Unknown: true})
		}
	}
	if len(unknownItems) > 0 {
		items = append(items, listItem{Separator: true, Key: ""})
		items = append(items, listItem{Separator: true, Key: "UNKNOWN"})
		items = append(items, unknownItems...)
	}

	// PASSTHROUGH: keys declared as passthrough that exist in the file AND are
	// not already shown under ADDED (i.e. not in the schema). Keys that appear
	// in both the schema and the passthrough list are handled by ADDED/AVAILABLE
	// and must not be duplicated here.
	var passthroughItems []listItem
	for _, b := range existing {
		if passthrough[b.Key] && !knownSet[b.Key] {
			passthroughItems = append(passthroughItems, listItem{Key: b.Key, Existing: true, Unknown: true})
		}
	}
	if len(passthroughItems) > 0 {
		items = append(items, listItem{Separator: true, Key: ""})
		items = append(items, listItem{Separator: true, Key: "PASSTHROUGH"})
		items = append(items, passthroughItems...)
	}

	return items
}

// SetHeight updates the visible row count and re-clamps the scroll offset.
func (lm listModel) SetHeight(h int) listModel {
	lm.height = h
	return lm.clampScroll()
}

func newListModel(knownKeys []string, existing []document.Block, passthrough map[string]bool, height int) listModel {
	items := buildListItems(knownKeys, existing, passthrough)
	cursor := 0
	for i, it := range items {
		if !it.Separator {
			cursor = i
			break
		}
	}
	return listModel{knownKeys: knownKeys, passthrough: passthrough, items: items, cursor: cursor, height: height}
}

// clampFCursorToFiltered ensures fCursor is within [0, len(filteredItems)-1]
// after a rebuild that changes the filtered item count. No-op when not filtering.
func (lm listModel) clampFCursorToFiltered() listModel {
	if !lm.filtering {
		return lm
	}
	filtered := lm.filteredItems()
	if lm.fCursor < len(filtered) {
		return lm
	}
	if len(filtered) == 0 {
		lm.fCursor = 0
	} else {
		lm.fCursor = len(filtered) - 1
	}
	lm.fOffset = 0
	return lm
}

// Rebuild refreshes the list after blocks change without losing cursor position.
func (lm listModel) Rebuild(existing []document.Block) listModel {
	prevKey := ""
	if lm.cursor < len(lm.items) && !lm.items[lm.cursor].Separator {
		prevKey = lm.items[lm.cursor].Key
	}
	lm.items = buildListItems(lm.knownKeys, existing, lm.passthrough)
	if prevKey != "" {
		for i, it := range lm.items {
			if it.Key == prevKey {
				lm.cursor = i
				return lm.clampScroll().clampFCursorToFiltered()
			}
		}
	}
	lm.cursor = 0
	for i, it := range lm.items {
		if !it.Separator {
			lm.cursor = i
			break
		}
	}
	return lm.clampScroll().clampFCursorToFiltered()
}

// AddedCount returns how many recognised top-level keys are present in the doc.
func (lm listModel) AddedCount() int {
	n := 0
	for _, it := range lm.items {
		if it.Existing && !it.Unknown {
			n++
		}
	}
	return n
}

func (lm listModel) filteredItems() []listItem {
	f := strings.ToLower(lm.filter)
	var out []listItem
	for _, it := range lm.items {
		if it.Separator {
			continue
		}
		if f == "" || strings.Contains(strings.ToLower(it.Key), f) {
			out = append(out, it)
		}
	}
	return out
}

// SelectedItem returns the item under the cursor, or nil when the cursor sits
// on a separator or when the list is empty. In filter mode it returns the item
// under the filter cursor instead of the main cursor.
func (lm listModel) SelectedItem() *listItem {
	if lm.filtering {
		items := lm.filteredItems()
		if lm.fCursor >= len(items) {
			return nil
		}
		it := items[lm.fCursor]
		return &it
	}
	if lm.cursor >= len(lm.items) {
		return nil
	}
	it := lm.items[lm.cursor]
	if it.Separator {
		return nil
	}
	return &it
}

// ItemByKey returns the listItem for the given key, or a zero listItem when
// the key is not in the list. Separator rows are skipped so a block that
// happens to be named like a section label (e.g. "ADDED") cannot match one.
func (lm listModel) ItemByKey(key string) listItem {
	for _, it := range lm.items {
		if !it.Separator && it.Key == key {
			return it
		}
	}
	return listItem{Key: key}
}

// Update handles keyboard input for both normal and filter modes.
func (lm listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return lm, nil
	}
	if lm.filtering {
		return lm.updateFilter(key)
	}
	switch key.String() {
	case "/":
		lm.filtering = true
		lm.filter = ""
		lm.fCursor = 0
		lm.fOffset = 0
	case "up":
		lm = lm.moveCursor(-1)
	case "down":
		lm = lm.moveCursor(1)
	case "enter":
		if it := lm.SelectedItem(); it != nil && !it.Unknown {
			item := *it
			return lm, func() tea.Msg { return openItemMsg{Item: item} }
		}
	case "ctrl+d":
		if it := lm.SelectedItem(); it != nil && it.Existing {
			k := it.Key
			return lm, func() tea.Msg { return deleteItemMsg{Key: k} }
		}
	}
	return lm, nil
}

func (lm listModel) updateFilter(key tea.KeyMsg) (listModel, tea.Cmd) {
	switch key.String() {
	case "esc":
		lm.filtering = false
		lm.filter = ""
		lm.fCursor = 0
		lm.fOffset = 0
	case "enter":
		items := lm.filteredItems()
		var selCmd tea.Cmd
		if lm.fCursor < len(items) {
			sel := items[lm.fCursor].Key
			for i, it := range lm.items {
				if !it.Separator && it.Key == sel {
					lm.cursor = i
					lm = lm.clampScroll()
					break
				}
			}
			// Emit openItemMsg for the selected item, mirroring normal-mode Enter.
			// Guard against separators: filteredItems() skips them, but be
			// defensive in case the list is rebuilt concurrently with a keypress.
			item := items[lm.fCursor]
			if !item.Separator && !item.Unknown {
				selCmd = func() tea.Msg { return openItemMsg{Item: item} }
			}
		}
		lm.filtering = false
		return lm, selCmd
	case "backspace", "ctrl+h":
		if len(lm.filter) > 0 {
			// Drop the last rune, not the last byte - a multibyte character
			// ("ç", "ã") would otherwise leave invalid UTF-8 in the filter.
			_, size := utf8.DecodeLastRuneInString(lm.filter)
			lm.filter = lm.filter[:len(lm.filter)-size]
			lm.fCursor = 0
			lm.fOffset = 0
		}
	// Only the arrow keys navigate while filtering - "j"/"k" must remain
	// typeable so filters like "unknown" or "worker" can be entered.
	case "up":
		lm = lm.moveFCursor(-1)
	case "down":
		lm = lm.moveFCursor(1)
	default:
		if r := key.Runes; len(r) == 1 && r[0] >= 32 {
			lm.filter += string(r)
			lm.fCursor = 0
			lm.fOffset = 0
		}
	}
	return lm, nil
}

func (lm listModel) moveFCursor(delta int) listModel {
	next := lm.fCursor + delta
	if next < 0 || next >= len(lm.filteredItems()) {
		return lm
	}
	lm.fCursor = next
	return lm.clampFScroll()
}

func (lm listModel) clampFScroll() listModel {
	visH := lm.height - 1
	if visH <= 0 {
		return lm
	}
	if lm.fCursor < lm.fOffset {
		lm.fOffset = lm.fCursor
	}
	if lm.fCursor >= lm.fOffset+visH {
		lm.fOffset = lm.fCursor - visH + 1
	}
	return lm
}

func (lm listModel) moveCursor(delta int) listModel {
	// Clamp at the list bounds (no wrap-around), skipping separator rows -
	// matching the tree and viewer panels.
	for i := lm.cursor + delta; i >= 0 && i < len(lm.items); i += delta {
		if !lm.items[i].Separator {
			lm.cursor = i
			break
		}
	}
	return lm.clampScroll()
}

func (lm listModel) clampScroll() listModel {
	if lm.height <= 0 {
		return lm
	}
	lm.offset = theme.ClampScroll(lm.cursor, lm.offset, lm.height)
	// The last visible row is replaced by the "↓ N more" indicator when items
	// overflow below the view. If the cursor lands on that row, advance offset
	// by one so the cursor remains visible.
	if lm.offset+lm.height < len(lm.items) && lm.cursor >= lm.offset+lm.height-1 {
		lm.offset = lm.cursor - lm.height + 2
	}
	return lm
}

func (lm listModel) jumpToLast() listModel {
	for i := len(lm.items) - 1; i >= 0; i-- {
		if !lm.items[i].Separator {
			lm.cursor = i
			return lm.clampScroll()
		}
	}
	return lm
}

func renderListItem(it listItem, selected bool, th resolvedTheme) string {
	if selected {
		mark := "+"
		if it.Unknown {
			mark = "⚠"
		} else if it.Existing {
			mark = "●"
		}
		return th.selectedItem.Render("▶ " + mark + "  " + it.Key)
	}
	if it.Unknown {
		return th.unknownItem.Render("  ⚠  " + it.Key)
	}
	if it.Existing {
		return th.existingItem.Render("  ●  " + it.Key)
	}
	return th.availableItem.Render("  +  " + it.Key)
}

// View renders the scrollable list or the filter prompt, depending on mode.
func (lm listModel) View(th resolvedTheme) string {
	if lm.filtering {
		return lm.viewFilter(th)
	}

	// Reserve last row for a scroll indicator when items overflow below.
	maxVisible := lm.height
	hasMore := lm.offset+lm.height < len(lm.items)
	if hasMore {
		maxVisible = lm.height - 1
	}

	end := lm.offset + maxVisible
	if end > len(lm.items) {
		end = len(lm.items)
	}

	var sb strings.Builder
	for i := lm.offset; i < end; i++ {
		if i > lm.offset {
			sb.WriteByte('\n')
		}
		it := lm.items[i]
		if it.Separator {
			sb.WriteString(th.sectionLabel.Render(it.Key))
		} else {
			sb.WriteString(renderListItem(it, i == lm.cursor, th))
		}
	}

	if hasMore {
		remaining := len(lm.items) - end
		if sb.Len() > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(th.availableItem.Render(fmt.Sprintf("  ↓ %d more", remaining)))
	}

	return sb.String()
}

func (lm listModel) viewFilter(th resolvedTheme) string {
	items := lm.filteredItems()
	visH := lm.height - 1
	end := lm.fOffset + visH
	if end > len(items) {
		end = len(items)
	}

	lines := make([]string, 0, lm.height)
	for i := lm.fOffset; i < end; i++ {
		lines = append(lines, renderListItem(items[i], i == lm.fCursor, th))
	}
	for len(lines) < visH {
		lines = append(lines, "")
	}
	lines = append(lines, th.filterPrompt.Render("/"+lm.filter+"▋"))
	return strings.Join(lines, "\n")
}
