package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/yedit/spec"
)

type groupEntry struct{ path, msg string }

// rulesLines renders GroupRules entries as a tree: sections with ├/└ connectors.
func rulesLines(entries []groupEntry) []string {
	type sectionItem struct{ label, msg string }
	sections := make(map[string][]sectionItem)
	var sectionOrder []string
	sectionSeen := make(map[string]bool)

	for _, entry := range entries {
		sec, label := splitRulesPath(entry.path)
		if !sectionSeen[sec] {
			sectionSeen[sec] = true
			sectionOrder = append(sectionOrder, sec)
		}
		sections[sec] = append(sections[sec], sectionItem{label, entry.msg})
	}

	var lines []string
	for _, sec := range sectionOrder {
		items := sections[sec]
		lines = append(lines, "  - "+sec)
		// Measure in terminal cells, not bytes, so labels with multibyte runes
		// do not skew the message column.
		maxW := 0
		for _, it := range items {
			if w := lipgloss.Width(it.label); w > maxW {
				maxW = w
			}
		}
		for i, it := range items {
			conn := "├"
			if i == len(items)-1 {
				conn = "└"
			}
			if it.label == "" {
				lines = append(lines, fmt.Sprintf("    %s %s", conn, it.msg))
			} else {
				pad := strings.Repeat(" ", maxW-lipgloss.Width(it.label))
				lines = append(lines, fmt.Sprintf("    %s %s%s  %s", conn, it.label, pad, it.msg))
			}
		}
	}
	return lines
}

// splitRulesPath splits a dotted/bracketed path into (section, fieldLabel).
// For unsplit paths the section is the path itself and fieldLabel is empty.
func splitRulesPath(path string) (sec, label string) {
	dot := strings.IndexByte(path, '.')
	bracket := strings.IndexByte(path, '[')
	split := dot
	if bracket >= 0 && (split < 0 || bracket < split) {
		split = bracket
	}
	switch {
	case split < 0:
		return path, ""
	case path[split] == '[':
		return path[:split], path[split:]
	default:
		return path[:split], path[split+1:]
	}
}

// Violations renders errs as a grouped list. Every violation must carry a
// Group (collectErrors guarantees this). GroupRules uses tree-style rendering
// (sections with ├/└ connectors); all other groups use a bullet list.
// maxLines caps the total output lines; excess is replaced with a summary line.
func Violations(errs []spec.Violation, maxLines int) string {
	entries := make(map[spec.Group][]groupEntry)
	var groupOrder []spec.Group
	groupSeen := make(map[spec.Group]bool)

	for _, e := range errs {
		if !groupSeen[e.Group] {
			groupSeen[e.Group] = true
			groupOrder = append(groupOrder, e.Group)
		}
		entries[e.Group] = append(entries[e.Group], groupEntry{e.Path, e.Message})
	}

	var lines []string
	for i, grp := range groupOrder {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "• "+string(grp)+":")
		if grp == spec.GroupRules {
			lines = append(lines, rulesLines(entries[grp])...)
		} else {
			for _, entry := range entries[grp] {
				loc := entry.path
				switch {
				case entry.msg == "":
					lines = append(lines, "  - "+loc)
				case loc == "":
					lines = append(lines, "  - "+entry.msg)
				default:
					lines = append(lines, "  - "+loc+": "+entry.msg)
				}
			}
		}
	}

	if maxLines > 0 && len(lines) > maxLines {
		// The tail mixes group headers and blank spacers, so count what is
		// actually cut: lines, not errors.
		remaining := len(lines) - maxLines
		lines = append(lines[:maxLines], fmt.Sprintf("... and %d more line(s).", remaining))
	}
	return strings.Join(lines, "\n")
}
