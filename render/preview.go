package render

import (
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
)

// NewPreviewRenderer builds a glamour renderer that word-wraps to wrap columns.
// It starts from the dark style and trims glamour's default chrome: the
// document and code-block left margins
// stack to ~4 columns and the block prefix/suffix add blank lines, all wasteful
// inside a panel that already has its own border. No margin is kept - the
// gutter rendered alongside the content (draw.ViewportGutter, draw.NumberLines,
// or the YAML editor's own line-number prompt) already ends in a space, so an
// extra glamour margin would double that gap and misalign Preview against the
// YAML editor. Returns nil on error, in which case PreviewYAML falls
// back to plain text.
func NewPreviewRenderer(wrap int) *glamour.TermRenderer {
	cfg := styles.DarkStyleConfig
	zero := uint(0)
	cfg.Document.Margin = &zero
	cfg.Document.BlockPrefix = ""
	cfg.Document.BlockSuffix = ""
	cfg.CodeBlock.Margin = &zero

	r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg), glamour.WithWordWrap(wrap))
	if err != nil {
		return nil
	}
	return r
}

// PreviewYAML renders raw YAML through r (wrapped in a markdown code fence)
// for syntax-highlighted display. Falls back to the plain text when r is nil or
// rendering fails.
func PreviewYAML(raw string, r *glamour.TermRenderer) string {
	raw = strings.TrimSuffix(raw, "\n")
	if r == nil || raw == "" {
		return raw
	}
	out := YAMLFence(raw, r)
	if out == raw {
		return raw // rendering failed - YAMLFence returned the input unchanged
	}
	return trimBlankLines(out)
}

var ansiEscapeRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// trimBlankLines drops leading and trailing whitespace-only lines - glamour
// emits a padded blank line around the code block - while leaving any interior
// blank lines intact. It is ANSI-aware so colored padding still reads as blank.
func trimBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	blank := func(l string) bool {
		return strings.TrimSpace(ansiEscapeRE.ReplaceAllString(l, "")) == ""
	}
	start, end := 0, len(lines)
	for start < end && blank(lines[start]) {
		start++
	}
	for end > start && blank(lines[end-1]) {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}
