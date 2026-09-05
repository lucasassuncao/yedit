package report

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/theme"
)

// styles are the report's own lipgloss styles.
//
// Structure (section headings, connectors, paths) follows the configured
// theme, so a validate command looks like the editor it ships with. Severity
// does not: error is red, warning is yellow and success is green in every
// theme, the same rule the alert component follows, because a user must not
// have to learn a palette to know whether something is broken.
type styles struct {
	section   lipgloss.Style
	connector lipgloss.Style
	path      lipgloss.Style
	errorText lipgloss.Style
	warnText  lipgloss.Style
	okText    lipgloss.Style
	count     lipgloss.Style
}

func newStyles(t theme.Theme) styles {
	rt := theme.Resolve(t)
	return styles{
		section:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(rt.Colors.ActiveBorderColor)),
		connector: lipgloss.NewStyle().Foreground(lipgloss.Color(rt.Colors.InactiveBorderColor)),
		path:      lipgloss.NewStyle().Foreground(lipgloss.Color(rt.Colors.SelectionColor)),
		errorText: lipgloss.NewStyle().Foreground(theme.Danger),
		warnText:  lipgloss.NewStyle().Foreground(theme.Warning),
		okText:    lipgloss.NewStyle().Foreground(theme.Success),
		count:     lipgloss.NewStyle().Bold(true),
	}
}

// Pretty writes a themed report: errors grouped by top-level section as a
// tree, warnings listed after them, then a summary line per severity.
func Pretty(w io.Writer, violations []spec.Violation, opts Options) {
	st := newStyles(opts.Theme)
	errs, warnings := Partition(violations)

	if len(errs) == 0 && len(warnings) == 0 {
		fmt.Fprintln(w, st.okText.Render("No errors found - configuration is valid"))
		return
	}

	if len(errs) > 0 {
		sections, bySection := GroupBySection(errs)
		if !opts.SummaryOnly {
			for _, section := range sections {
				fmt.Fprintln(w)
				fmt.Fprintln(w, "  "+st.section.Render(section))
				writeTree(w, st, bySection[section], true)
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, summaryLine(st, sections, bySection))
	}

	if len(warnings) > 0 {
		if !opts.SummaryOnly {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "  "+st.warnText.Render("warnings"))
			writeTree(w, st, warnings, false)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, warningSummary(st, len(warnings), opts.WarningNote))
	}
}

// writeTree renders one bucket of violations as a connector tree. stripSection
// drops the leading section from each path, which only makes sense when the
// bucket is a section and the heading already showed it.
func writeTree(w io.Writer, st styles, vs []spec.Violation, stripSection bool) {
	for i, v := range vs {
		p := v.Path
		if stripSection {
			p = SubPath(p)
		}
		fmt.Fprintf(w, "  %s %s %s\n",
			st.connector.Render(connector(i, len(vs))),
			st.path.Render(fmt.Sprintf("%-44s", p)),
			v.Message)
	}
}

// summaryLine builds the "N error(s) - X in a, Y in b" line shared by Pretty
// and Table.
func summaryLine(st styles, sections []string, bySection map[string][]spec.Violation) string {
	parts := make([]string, 0, len(sections))
	total := 0
	for _, s := range sections {
		n := len(bySection[s])
		total += n
		parts = append(parts, fmt.Sprintf("%d in %s", n, s))
	}
	return fmt.Sprintf("%s error(s) - %s",
		st.errorText.Render(st.count.Render(fmt.Sprintf("%d", total))),
		strings.Join(parts, ", "))
}

func warningSummary(st styles, n int, note string) string {
	line := fmt.Sprintf("%s warning(s)", st.warnText.Render(st.count.Render(fmt.Sprintf("%d", n))))
	if note != "" {
		line += " - " + note
	}
	return line
}
