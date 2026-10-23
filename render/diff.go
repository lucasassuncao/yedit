package render

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// diffContext is how many unchanged lines a hunk keeps around each change.
const diffContext = 2

// DiffStyles colours the rows Diff draws; the zero value draws plain text.
// Added and Removed paint the +/- markers, AddedRow and RemovedRow the rows.
type DiffStyles struct {
	Header, Gutter, Text, Added, Removed, AddedRow, RemovedRow lipgloss.Style
}

// DiffResult is a drawn diff and how many lines it adds and removes.
type DiffResult struct {
	Rows           []string
	Added, Removed int
}

// edit is one line of a line diff: ' ' kept, '-' removed, '+' added. old and
// new count the lines of each side that come before it.
type edit struct {
	op       byte
	text     string
	old, new int
}

// Diff is before against after as unified diff hunks: one row per line with
// its old and new numbers, no rows when they match. Rows share one width, so
// a row style paints them edge to edge.
func Diff(before, after []byte, st DiffStyles) DiffResult {
	a, b := splitLines(before), splitLines(after)
	nw := len(strconv.Itoa(max(len(a), len(b))))
	type row struct {
		op           byte // 0 for a hunk header
		gutter, text string
	}
	var rows []row
	var res DiffResult
	for _, h := range hunks(lineEdits(a, b)) {
		rows = append(rows, row{text: hunkHeader(h)})
		for _, e := range h {
			oldN, newN := "", ""
			if e.op != '+' {
				oldN = strconv.Itoa(e.old + 1)
			}
			if e.op != '-' {
				newN = strconv.Itoa(e.new + 1)
			}
			rows = append(rows, row{op: e.op, gutter: fmt.Sprintf("%*s %*s ", nw, oldN, nw, newN), text: e.text})
			switch e.op {
			case '+':
				res.Added++
			case '-':
				res.Removed++
			}
		}
	}
	width := 0
	for _, r := range rows {
		if r.op != 0 {
			width = max(width, lipgloss.Width(r.gutter)+2+lipgloss.Width(r.text))
		}
	}
	for _, r := range rows {
		if r.op == 0 {
			res.Rows = append(res.Rows, st.Header.Render(r.text))
			continue
		}
		paint, marker := lipgloss.NewStyle(), st.Text
		switch r.op {
		case '+':
			paint, marker = st.AddedRow, st.Added
		case '-':
			paint, marker = st.RemovedRow, st.Removed
		}
		pad := strings.Repeat(" ", width-lipgloss.Width(r.gutter)-2-lipgloss.Width(r.text))
		res.Rows = append(res.Rows, st.Gutter.Inherit(paint).Render(r.gutter)+marker.Inherit(paint).Render(string(r.op)+" ")+
			st.Text.Inherit(paint).Render(r.text)+paint.Render(pad))
	}
	return res
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// lineEdits keeps the lines a and b share at both ends and diffs only what
// lies between, so the table below is the size of the change, not the file.
func lineEdits(a, b []string) []edit {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	out := make([]edit, 0, len(a)+len(b)-pre-suf)
	for i := range pre {
		out = append(out, edit{' ', a[i], i, i})
	}
	for _, e := range lcsEdits(a[pre:len(a)-suf], b[pre:len(b)-suf]) {
		out = append(out, edit{e.op, e.text, e.old + pre, e.new + pre})
	}
	for k := suf; k > 0; k-- {
		i, j := len(a)-k, len(b)-k
		out = append(out, edit{' ', a[i], i, j})
	}
	return out
}

// lcsEdits walks a longest common subsequence of a and b, removals first.
func lcsEdits(a, b []string) []edit {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []edit
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			out = append(out, edit{' ', a[i], i, j})
			i, j = i+1, j+1
		case j < len(b) && (i == len(a) || lcs[i][j+1] > lcs[i+1][j]):
			out = append(out, edit{'+', b[j], i, j})
			j++
		default:
			out = append(out, edit{'-', a[i], i, j})
			i++
		}
	}
	return out
}

// hunks cuts edits around each change, joining changes whose context touches.
func hunks(edits []edit) [][]edit {
	var out [][]edit
	start, end := -1, -1
	for i, e := range edits {
		if e.op == ' ' {
			continue
		}
		lo, hi := max(0, i-diffContext), min(len(edits), i+diffContext+1)
		if start >= 0 && lo <= end {
			end = hi
			continue
		}
		if start >= 0 {
			out = append(out, edits[start:end])
		}
		start, end = lo, hi
	}
	if start >= 0 {
		out = append(out, edits[start:end])
	}
	return out
}

func hunkHeader(h []edit) string {
	oldN, newN := 0, 0
	for _, e := range h {
		if e.op != '+' {
			oldN++
		}
		if e.op != '-' {
			newN++
		}
	}
	return fmt.Sprintf("@@ -%s +%s @@", span(h[0].old, oldN), span(h[0].new, newN))
}

// span is a hunk side as "start,count"; an empty side names the line before it.
func span(before, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", before)
	}
	return fmt.Sprintf("%d,%d", before+1, count)
}
