package report

import (
	"fmt"
	"io"

	"github.com/lucasassuncao/yedit/spec"
)

// Plain writes an uncolored, one-line-per-violation report: the path padded to
// a fixed column, then the message, then the counts. It is the format to pipe
// into grep, and the one to use when the output is not a terminal.
//
// A run with no violations at all prints "ok".
func Plain(w io.Writer, violations []spec.Violation, opts Options) {
	errs, warnings := Partition(violations)

	if len(errs) == 0 && len(warnings) == 0 {
		fmt.Fprintln(w, "ok")
		return
	}

	if !opts.SummaryOnly {
		for _, v := range errs {
			fmt.Fprintf(w, "%-48s %s\n", v.Path, v.Message)
		}
		for _, v := range warnings {
			fmt.Fprintf(w, "%-48s %s\n", v.Path, v.Message)
		}
	}

	if len(errs) > 0 {
		fmt.Fprintf(w, "%d error(s)\n", len(errs))
	}
	if len(warnings) > 0 {
		fmt.Fprintf(w, "%d warning(s)\n", len(warnings))
	}
}
