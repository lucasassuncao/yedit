package editor

import (
	"fmt"
	"strings"

	"github.com/lucasassuncao/bezel/browser"
	"github.com/lucasassuncao/yedit/presets"
)

// presetItems lists a field's presets for the browser, each previewing its
// YAML on demand. Nil when the source has none, so the picker does not open.
func presetItems(source presets.Source, field string) []browser.Item {
	if source == nil {
		return nil
	}
	names := source.ListPresets(field)
	items := make([]browser.Item, 0, len(names))
	for _, name := range names {
		items = append(items, browser.Item{Label: name, Detail: func() string {
			y, err := source.PresetYAML(field, name)
			if err != nil {
				return fmt.Sprintf("# error: %v", err)
			}
			// Applying normalizes too; a CR left here sends the cursor to column 0.
			return strings.ReplaceAll(y, "\r\n", "\n")
		}})
	}
	return items
}
