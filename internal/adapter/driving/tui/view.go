package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// sgrPerLine is a guess at the bytes of style switches in a line, so the
// builder grows once per frame.
const sgrPerLine = 256

// View renders the state. It only reads.
func (m model) View() tea.View {
	v := tea.NewView(render(&m.s, m.theme))
	v.AltScreen = true
	return v
}

// render writes every line into one builder. A pane reads its line only as
// far as it shows it, so a long line costs no more than a short one.
func render(s *state, th *theme) string {
	var b strings.Builder
	l := s.Layout
	b.Grow((l.Width*4 + sgrPerLine) * (l.Body + chrome))
	textLine := func(text string, st style) {
		ln := newLine(&b, th, s.Cells, l.Width)
		ln.text(text, st)
		ln.end(st)
	}
	textLine(header(s), headerStyle)
	if l.Height == 1 {
		return b.String()
	}
	// Below three lines no line of the file fits; the screen gets no more
	// lines than it has.
	for i := range min(l.Body, max(l.Height-chrome, 0)) {
		b.WriteByte('\n')
		switch j := s.Pos.Scroll + i; {
		case j < len(s.Lines):
			writeRow(&b, s, th, s.Lines[j])
		case i == 0:
			textLine(placeholder(s), plain)
		default:
			textLine("", plain)
		}
	}
	b.WriteByte('\n')
	textLine(footer(s))
	return b.String()
}
