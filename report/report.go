// Package report renders []spec.Violation for humans and for machines.
//
// It exists so that every yedit-based tool can offer the same "validate my
// config" command without rewriting the grouping and formatting each time, and
// so that the JSON produced by those commands has one shape that a single CI
// parser can consume across all of them.
//
// The validate package stays free of any rendering dependency: importing
// report is opt-in, and a headless validation run that only inspects the
// returned violations never links it.
//
// # Severity
//
// Reporters split violations by spec.Severity. Errors and warnings are counted
// and rendered separately, and JSON reports "valid" from the error count alone,
// so a warning never fails a build.
package report

import (
	"regexp"
	"sort"
	"strings"

	"github.com/lucasassuncao/yedit/spec"
	"github.com/lucasassuncao/yedit/theme"
)

// Options tunes the reporters. The zero value is valid: full detail, default
// theme, auto-detected width.
type Options struct {
	// SummaryOnly prints only the counts, omitting individual violations.
	SummaryOnly bool
	// Theme styles the structural parts of the output (section headings,
	// connectors, paths). Severity colors are deliberately not themed: an
	// error is red and a warning is yellow in every theme, the same rule the
	// alert component follows.
	Theme theme.Theme
	// WarningNote is appended to the warning summary line when non-empty, for
	// applications that want to say what their warnings mean
	// (e.g. "valid configuration that can lose files").
	WarningNote string
	// Width is the terminal width used by Table. Zero detects it from stdout
	// and falls back to 120.
	Width int
}

// Partition splits violations by severity, preserving order within each group.
func Partition(violations []spec.Violation) (errs, warnings []spec.Violation) {
	for _, v := range violations {
		if v.Severity == spec.SeverityWarning {
			warnings = append(warnings, v)
		} else {
			errs = append(errs, v)
		}
	}
	return errs, warnings
}

var topSectionRe = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9_-]*)`)

// Section returns the top-level section a violation path belongs to, or
// "(general)" for a document-wide violation with no leading key.
func Section(path string) string {
	if m := topSectionRe.FindString(path); m != "" {
		return m
	}
	return "(general)"
}

// SubPath strips the top-level section from a violation path, leaving the part
// that is worth showing under a section heading. A path with no separator is
// returned unchanged.
func SubPath(path string) string {
	if i := strings.IndexAny(path, ".["); i >= 0 {
		return strings.TrimPrefix(path[i:], ".")
	}
	return path
}

// GroupBySection buckets violations by Section. sections is sorted, so the
// output order is stable across runs.
func GroupBySection(violations []spec.Violation) (sections []string, bySection map[string][]spec.Violation) {
	bySection = make(map[string][]spec.Violation)
	for _, v := range violations {
		s := Section(v.Path)
		bySection[s] = append(bySection[s], v)
	}
	sections = make([]string, 0, len(bySection))
	for s := range bySection {
		sections = append(sections, s)
	}
	sort.Strings(sections)
	return sections, bySection
}

// connector returns the tree glyph for item i of n.
func connector(i, n int) string {
	if i == n-1 {
		return "└─"
	}
	return "├─"
}
