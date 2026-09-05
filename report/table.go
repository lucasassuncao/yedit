package report

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
	prettytable "github.com/jedib0t/go-pretty/v6/table"

	"github.com/lucasassuncao/yedit/spec"
)

// Column budget for Table: the path column is capped, the message column takes
// what is left, and never drops below minMessageCol however narrow the
// terminal claims to be.
const (
	pathColMax     = 40
	minMessageCol  = 30
	tableBorders   = 7 // "│ " + " │ " + " │"
	fallbackWidth  = 120
	minUsefulWidth = 60
)

// Table writes errors as one bordered table per top-level section, then the
// same summary lines Pretty produces. Warnings are listed as a tree rather
// than a table: they are rarely numerous enough to need columns.
//
// This is the only reporter that costs a dependency (go-pretty). Pretty covers
// the same ground without one.
func Table(w io.Writer, violations []spec.Violation, opts Options) {
	st := newStyles(opts.Theme)
	errs, warnings := Partition(violations)

	if len(errs) == 0 && len(warnings) == 0 {
		fmt.Fprintln(w, st.okText.Render("No errors found - configuration is valid"))
		return
	}

	if len(errs) > 0 {
		sections, bySection := GroupBySection(errs)
		if !opts.SummaryOnly {
			width := opts.Width
			if width == 0 {
				width = detectWidth()
			}
			for _, section := range sections {
				vs := bySection[section]
				fmt.Fprintln(w)
				fmt.Fprintf(w, "%s  %s\n",
					st.section.Render(section),
					st.connector.Render(fmt.Sprintf("(%d errors)", len(vs))))
				writeSectionTable(w, vs, width)
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

// detectWidth reads the terminal width from stdout, falling back to a readable
// default when stdout is not a terminal or reports something unusably narrow.
func detectWidth() int {
	width, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || width < minUsefulWidth {
		return fallbackWidth
	}
	return width
}

func writeSectionTable(w io.Writer, vs []spec.Violation, width int) {
	messageCol := width - pathColMax - tableBorders
	if messageCol < minMessageCol {
		messageCol = minMessageCol
	}

	t := prettytable.NewWriter()
	t.SetOutputMirror(w)
	t.SetStyle(prettytable.StyleRounded)
	t.SetColumnConfigs([]prettytable.ColumnConfig{
		{Number: 1, WidthMax: pathColMax},
		{Number: 2, WidthMax: messageCol},
	})
	t.AppendHeader(prettytable.Row{"PATH", "ERROR"})
	for _, v := range vs {
		t.AppendRow(prettytable.Row{SubPath(v.Path), v.Message})
	}
	t.Render()
}
