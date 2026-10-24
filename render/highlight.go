package render

import (
	"strings"
	"sync"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/lucasassuncao/bezel/component/textbox"
)

// palette is the preview's code-block colours by token type, so the YAML
// editor and Preview paint a file alike.
var palette = sync.OnceValue(func() map[chroma.TokenType]lipgloss.Style {
	c := styles.DarkStyleConfig.CodeBlock.Chroma
	p := map[chroma.TokenType]lipgloss.Style{}
	for t, prim := range map[chroma.TokenType]ansi.StylePrimitive{
		chroma.Text: c.Text, chroma.Error: c.Error, chroma.Comment: c.Comment,
		chroma.CommentPreproc: c.CommentPreproc, chroma.Keyword: c.Keyword,
		chroma.KeywordReserved: c.KeywordReserved, chroma.KeywordNamespace: c.KeywordNamespace,
		chroma.KeywordType: c.KeywordType, chroma.Operator: c.Operator,
		chroma.Punctuation: c.Punctuation, chroma.Name: c.Name,
		chroma.NameBuiltin: c.NameBuiltin, chroma.NameTag: c.NameTag,
		chroma.NameAttribute: c.NameAttribute, chroma.NameClass: c.NameClass,
		chroma.NameConstant: c.NameConstant, chroma.NameDecorator: c.NameDecorator,
		chroma.NameFunction: c.NameFunction, chroma.Literal: c.Literal,
		chroma.LiteralNumber: c.LiteralNumber, chroma.LiteralDate: c.LiteralDate,
		chroma.LiteralString: c.LiteralString, chroma.LiteralStringEscape: c.LiteralStringEscape,
	} {
		if s, ok := styleOf(prim); ok {
			p[t] = s
		}
	}
	return p
})

// styleOf is prim as a lipgloss style, false when it sets nothing.
func styleOf(prim ansi.StylePrimitive) (lipgloss.Style, bool) {
	s, set := lipgloss.NewStyle(), false
	if prim.Color != nil {
		s, set = s.Foreground(lipgloss.Color(*prim.Color)), true
	}
	if prim.BackgroundColor != nil {
		s, set = s.Background(lipgloss.Color(*prim.BackgroundColor)), true
	}
	if prim.Bold != nil {
		s, set = s.Bold(*prim.Bold), true
	}
	if prim.Italic != nil {
		s, set = s.Italic(*prim.Italic), true
	}
	if prim.Underline != nil {
		s, set = s.Underline(*prim.Underline), true
	}
	return s, set
}

// tokenStyle is t's colour, else its subcategory's, else its category's.
func tokenStyle(t chroma.TokenType) (lipgloss.Style, bool) {
	p := palette()
	for _, k := range []chroma.TokenType{t, t.SubCategory(), t.Category()} {
		if s, ok := p[k]; ok {
			return s, true
		}
	}
	return lipgloss.Style{}, false
}

// YAMLHighlight is the textbox highlighter for YAML: each line's spans, in
// bytes, coloured as the preview colours them.
func YAMLHighlight(value string) [][]textbox.Span {
	out := make([][]textbox.Span, strings.Count(value, "\n")+1)
	lexer := lexers.Get("yaml")
	if lexer == nil {
		return out
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, value)
	if err != nil {
		return out
	}
	line, col := 0, 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		style, ok := tokenStyle(tok.Type)
		// A token may cross lines, a block scalar say: each line gets its piece.
		for i, piece := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				line, col = line+1, 0
			}
			if line >= len(out) {
				return out
			}
			if ok && strings.TrimSpace(piece) != "" {
				out[line] = append(out[line], textbox.Span{Start: col, End: col + len(piece), Style: style})
			}
			col += len(piece)
		}
	}
	return out
}
