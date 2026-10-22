package editor

import (
	"fmt"
	"image/color"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/lucasassuncao/bezel/layout"
	"github.com/lucasassuncao/bezel/legend"
	"github.com/lucasassuncao/bezel/overlay"
	"github.com/lucasassuncao/bezel/theme"
	"github.com/lucasassuncao/yedit/keys"
	"github.com/lucasassuncao/yedit/render"
)

// reviewChrome is the rows the dialog spends around the diff: border and
// padding, the title, the path and the footer, and the gaps between them.
const reviewChrome = 2 + 2 + 2 + 2 + 3

var (
	reviewPageUp   = key.NewBinding(key.WithKeys("pgup"))
	reviewPageDown = key.NewBinding(key.WithKeys("pgdown"))
)

// saveReview is the "Save changes?" dialog: the diff of what the save writes,
// scrolled with the arrows. Enter saves; esc closes it, as on every dialog.
type saveReview struct {
	th     theme.Resolved
	path   string
	diff   render.DiffResult
	onSave tea.Cmd
	offset int
	fit    int // diff rows shown at the last Resize
}

func newSaveReview(th theme.Resolved, path string, before, after []byte, onSave tea.Cmd) saveReview {
	added, removed := lipgloss.Color(th.Colors.Success), lipgloss.Color(th.Colors.Danger)
	st := render.DiffStyles{
		Header: th.Info, Gutter: th.Dim,
		Added: th.Success.Bold(true), Removed: th.Danger.Bold(true),
		AddedRow:   lipgloss.NewStyle().Background(rowTint(added, th)),
		RemovedRow: lipgloss.NewStyle().Background(rowTint(removed, th)),
	}
	d := render.Diff(before, after, st)
	return saveReview{th: th, path: path, diff: d, onSave: onSave, fit: len(d.Rows)}
}

// rowTint is c faded toward the terminal's background, which light body text
// says is dark.
func rowTint(c color.Color, th theme.Resolved) color.Color {
	r, g, b, _ := lipgloss.Color(th.Colors.Text).RGBA()
	if r+g+b > 3*0x7fff {
		return lipgloss.Darken(c, 0.7)
	}
	return lipgloss.Lighten(c, 0.75)
}

// Resize makes the diff as many rows as fit in body.
func (r saveReview) Resize(body layout.Rect) overlay.Overlay {
	r.fit = max(1, min(len(r.diff.Rows), body.H-reviewChrome))
	return r
}

func (r saveReview) Update(msg tea.Msg) (overlay.Overlay, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return r, nil
	}
	switch {
	case key.Matches(k, keys.Enter):
		return r, tea.Batch(r.onSave, overlay.Close())
	case key.Matches(k, keys.Up):
		r = r.scroll(-1)
	case key.Matches(k, keys.Down):
		r = r.scroll(1)
	case key.Matches(k, reviewPageUp):
		r = r.scroll(-r.fit)
	case key.Matches(k, reviewPageDown):
		r = r.scroll(r.fit)
	}
	return r, nil
}

func (r saveReview) scroll(by int) saveReview {
	r.offset = min(max(0, r.offset+by), max(0, len(r.diff.Rows)-r.fit))
	return r
}

func (r saveReview) Legend() []legend.Entry {
	return []legend.Entry{legend.New("enter", "save"), legend.New("esc", "keep editing"), legend.New("↑/↓", "scroll")}
}

func (r saveReview) View(body layout.Rect) string {
	accent := lipgloss.NewStyle().Foreground(r.th.Modal.GetBorderTopForeground()).Bold(true)
	lines := []string{accent.Render("Save changes?"), "", "Save to " + r.path, ""}
	summary := r.th.Success.Render("✓") + " validation passes · " +
		r.th.Success.Render(fmt.Sprintf("+%d", r.diff.Added)) + " " + r.th.Danger.Render(fmt.Sprintf("-%d", r.diff.Removed))
	if len(r.diff.Rows) == 0 {
		lines = append(lines, r.th.Dim.Render("No changes."))
	} else {
		end := min(len(r.diff.Rows), r.offset+r.fit)
		lines = append(lines, r.diff.Rows[r.offset:end]...)
		if r.fit < len(r.diff.Rows) {
			summary += r.th.Dim.Render(fmt.Sprintf(" · lines %d-%d of %d", r.offset+1, end, len(r.diff.Rows)))
		}
	}
	lines = append(lines, "", summary, legend.HintLine(r.Legend(), r.th.Legend))
	return overlay.Box(lines, body, r.th.Modal.Padding(1, 3))
}
