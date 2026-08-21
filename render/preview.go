package render

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"

	"github.com/lucasassuncao/yedit/theme"
)

// NewPreviewRenderer builds a glamour renderer that word-wraps to wrap columns.
// It starts from the dark style and trims glamour's default chrome: the
// document and code-block left margins
// stack to ~4 columns and the block prefix/suffix add blank lines, all wasteful
// inside a panel that already has its own border. No margin is kept - the
// gutter rendered alongside the content (PreviewGutter, numberPreviewLines,
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

// ClampLines truncates s to at most maxLines newline-separated lines so that
// content passed to RenderTitledPanel never overflows its allocated height.
// lipgloss.Height() is a minimum, not a cap - without this, a tall hint or
// preview would push the right column taller than the left.
func ClampLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}

// PreviewGutterWidth is the fixed real-width, in cells, of PreviewGutter's
// output ("%4d │ "). The viewport's own gutter-width probe calls the
// GutterFunc with a zero-value GutterContext, so the format must not depend
// on ctx.TotalLines or the probed width would mismatch the width used when
// actually rendering lines.
const PreviewGutterWidth = 7

// PreviewGutter renders a fixed-width line-number column for the YAML
// preview, styled to match the rest of the muted/secondary UI text.
func PreviewGutter(rt theme.Resolved) viewport.GutterFunc {
	return func(ctx viewport.GutterContext) string {
		if ctx.Index >= ctx.TotalLines {
			return rt.HintDim.Render("     │ ")
		}
		return rt.HintDim.Render(fmt.Sprintf("%4d │ ", ctx.Index+1))
	}
}
