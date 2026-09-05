package report

import (
	"encoding/json"
	"io"

	"github.com/lucasassuncao/yedit/spec"
)

// JSONOutput is the machine-readable shape written by JSON.
//
// It is the contract a CI job parses, so it is documented and stable: fields
// may be added, but the meaning of an existing one will not change. Valid is
// derived from the error count alone, so a warning never fails a build.
type JSONOutput struct {
	Valid        bool            `json:"valid"`
	ErrorCount   int             `json:"error_count"`
	WarningCount int             `json:"warning_count"`
	Errors       []JSONViolation `json:"errors,omitempty"`
	Warnings     []JSONViolation `json:"warnings,omitempty"`
	Summary      map[string]int  `json:"summary"`
}

// JSONViolation is one violation in a JSONOutput.
type JSONViolation struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// JSON writes the report as indented JSON. Summary counts errors per top-level
// section and is always present, so a consumer reading only the summary does
// not have to handle a missing key.
func JSON(w io.Writer, violations []spec.Violation, opts Options) error {
	errs, warnings := Partition(violations)
	_, bySection := GroupBySection(errs)

	summary := make(map[string]int, len(bySection))
	for s, vs := range bySection {
		summary[s] = len(vs)
	}

	out := JSONOutput{
		Valid:        len(errs) == 0,
		ErrorCount:   len(errs),
		WarningCount: len(warnings),
		Summary:      summary,
	}

	if !opts.SummaryOnly {
		out.Errors = toJSONViolations(errs)
		out.Warnings = toJSONViolations(warnings)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func toJSONViolations(vs []spec.Violation) []JSONViolation {
	if len(vs) == 0 {
		return nil
	}
	out := make([]JSONViolation, len(vs))
	for i, v := range vs {
		out[i] = JSONViolation{Path: v.Path, Message: v.Message}
	}
	return out
}
