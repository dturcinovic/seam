package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"seam/internal/domain/view"
)

// sgrPerLine is a guess at the bytes of style switches in a line, so the
// builder grows once per frame.
const sgrPerLine = 256

// View renders the state. It only reads.
func (m model) View() tea.View {
	v := tea.NewView(render(&m.s, m.theme))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	if h := &m.s.Hover; m.s.hoverable() || h.On || h.Popup.Open {
		// Every move, not only with a button held: hovers follow the mouse.
		// Only then, as each move costs a frame; and while one is on, so
		// that moving away closes it.
		v.MouseMode = tea.MouseModeAllMotion
	}
	return v
}

// render writes every line into one builder. A pane reads its line only as
// far as it shows it, so a long line costs no more than a short one.
func render(s *state, th *theme) string {
	var b strings.Builder
	l := s.Layout
	b.Grow((l.Width*4 + sgrPerLine) * (l.Body + chrome))
	writeText(&b, s, th, header(s), headerStyle, l.Width)
	if l.Height == 1 {
		return b.String()
	}
	start, end := thumb(s.lineCount(), s.top(), l.Body)
	p := s.Hover.Popup
	for i := range l.shown() {
		b.WriteByte('\n')
		onThumb := start <= i && i < end
		if !p.contains(p.X, bodyTop+i) {
			writeBodyLine(&b, s, th, i, onThumb)
			continue
		}
		var under strings.Builder
		writeBodyLine(&under, s, th, i, onThumb)
		b.WriteString(overlay(under.String(), p, th, s.Cells, l.Width, bodyTop+i-p.Y))
	}
	b.WriteByte('\n')
	writeFooter(&b, s, th)
	return b.String()
}

// writeText writes text in st on a line of width columns.
func writeText(b *strings.Builder, s *state, th *theme, text string, st style, width int) {
	ln := newLine(b, th, s.Cells, width)
	ln.text(text, st)
	ln.end(st)
}

// writeBodyLine writes body line i: the file list's entry, the row of the
// file and the scrollbar, onThumb if the thumb covers it.
func writeBodyLine(b *strings.Builder, s *state, th *theme, i int, onThumb bool) {
	l := s.Layout
	if l.List > 0 {
		writeEntry(b, s, th, i)
	}
	switch bl, ok := s.bodyAt(s.top() + i); {
	case ok && bl.spot == view.OnRow:
		writeRow(b, s, th, bl.file, bl.line)
	case ok:
		writeStreamLine(b, s, th, bl)
	case i == 0:
		writeText(b, s, th, placeholder(s), plain, l.panes())
	default:
		writeText(b, s, th, "", plain, l.panes())
	}
	m := equalTone
	if i < len(s.Marks) {
		m = s.Marks[i]
	}
	writeScrollbar(b, s, th, m, onThumb)
}

// writeFooter writes the footer's text with the language server's status
// at the right, which the text makes room for.
func writeFooter(b *strings.Builder, s *state, th *theme) {
	text, st := footer(s)
	ln := newLine(b, th, s.Cells, s.Layout.Width)
	status := lspText(s)
	if status == "" {
		ln.text(text, st)
		ln.end(st)
		return
	}
	status = " " + status + " "
	writeCut(&ln, text, ln.free-textWidth(status, s.Cells), st, false)
	ln.text(status, footerStyle)
	ln.end(footerStyle)
}
