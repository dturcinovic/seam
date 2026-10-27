package tui

import (
	"fmt"

	"github.com/dturcinovic/seam/internal/domain/filediff"
)

// plusAt is the line whose gutter offers the mouse at screen cell x, y a
// new note: the line of the side under it, if that side takes notes and
// the gutter there shows no mark. It shows on the whole line, not only on
// the gutter, so that the plus is seen before it is aimed at.
func (s *state) plusAt(x, y int) (noteLine, bool) {
	file, l, sides, ok := s.gutterAt(x, y)
	if !ok {
		return noteLine{}, false
	}
	for _, w := range sides {
		if line := s.lineIn(file, l, w); line >= 0 && len(s.notesAt(file, w, line)) > 0 {
			return noteLine{}, false
		}
	}
	for _, w := range sides {
		if _, takes := s.noteKey(file, w); takes {
			if line := s.lineIn(file, l, w); line >= 0 {
				return noteLine{File: file, Side: w, Line: line}, true
			}
		}
	}
	return noteLine{}, false
}

// plusText is the footer's word on the plus, empty while none shows.
func (s *state) plusText() string {
	h := s.Hover
	// A note that came since the mouse moved takes the line, as markSide
	// draws it.
	if !h.PlusOn {
		return ""
	}
	if mark, _ := s.noteMark(h.Plus.File, h.Plus.Side, h.Plus.Line); mark != "" {
		return ""
	}
	return fmt.Sprintf("click %s to note line %d  ", plusMark, h.Plus.Line+1)
}

// plusOn reports whether line i of side w of file shows the plus.
func (s *state) plusOn(file int, w filediff.Which, i int) bool {
	h := &s.Hover
	return h.PlusOn && h.Plus == noteLine{File: file, Side: w, Line: i}
}

// clickPlus opens the field for a note at the line whose plus lies at
// screen cell x, y; it reports whether there was one. Only a plus shown
// counts: a click in the gutter is no note where the screen offered none.
func (m *model) clickPlus(x, y int) bool {
	h := m.s.Hover
	l, ok := m.s.plusAt(x, y)
	if !ok || !h.PlusOn || l != h.Plus || !m.s.layoutOf(l.File).onMarks(l.Side, x) {
		return false
	}
	m.s.List.blur()
	m.openNote(noteTarget{File: l.File, Side: l.Side, From: l.Line, To: l.Line, X: x, Y: y})
	return true
}
