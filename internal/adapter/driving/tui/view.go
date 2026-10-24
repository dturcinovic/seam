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
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// render writes every line into one builder. A pane reads its line only as
// far as it shows it, so a long line costs no more than a short one.
func render(s *state, th *theme) string {
	var b strings.Builder
	l := s.Layout
	b.Grow((l.Width*4 + sgrPerLine) * (l.Body + chrome))
	textLine := func(text string, st style, width int) {
		ln := newLine(&b, th, s.Cells, width)
		ln.text(text, st)
		ln.end(st)
	}
	textLine(header(s), headerStyle, l.Width)
	if l.Height == 1 {
		return b.String()
	}
	start, end := thumb(len(s.Lines), s.Pos.Scroll, l.Body)
	for i := range l.shown() {
		b.WriteByte('\n')
		switch j := s.Pos.Scroll + i; {
		case j < len(s.Lines):
			writeRow(&b, s, th, s.Lines[j])
		case i == 0:
			textLine(placeholder(s), plain, l.panes())
		default:
			textLine("", plain, l.panes())
		}
		m := equalTone
		if i < len(s.Marks) {
			m = s.Marks[i]
		}
		writeScrollbar(&b, s, th, m, start <= i && i < end)
	}
	b.WriteByte('\n')
	text, st := footer(s)
	textLine(text, st, l.Width)
	return b.String()
}
