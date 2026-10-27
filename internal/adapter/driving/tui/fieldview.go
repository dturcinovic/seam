package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// fieldHints is the footer while the field is open. Tab is left out: its
// meaning is the owner's.
const fieldHints = "enter done  alt+enter new line  esc discard"

// openField opens the field for a text of at most limit bytes, placed for
// line y and column x as a popup is. The hover gives way to it, and so does
// a definition still to come, whose jump would move the code from under it.
func (m *model) openField(limit, x, y int) {
	m.dropHover()
	if id := m.s.Jump.Inflight; id != 0 {
		m.p.Define.CancelDefinition(id)
	}
	j := &m.s.Jump
	j.Inflight, j.Waiting, j.Target = 0, nil, nil
	m.s.openField(limit, x, y)
}

// openField opens the field; the footer's note, if any, gives way to the
// field's keys.
func (s *state) openField(limit, x, y int) {
	s.Field = field{Open: true, Limit: limit, Goal: -1, AtX: x, AtY: y}
	s.Note = ""
	s.layoutField()
}

// fieldKey gives k to the open field. Discarding closes it; text the limit
// keeps out is named in the footer.
func (s *state) fieldKey(k tea.Key) fieldResult {
	r := s.Field.key(k)
	s.afterField(r)
	return r
}

// fieldPaste puts pasted text in at the field's cursor, whole or not at all.
func (s *state) fieldPaste(text string) {
	s.afterField(s.Field.insert(text))
}

func (s *state) afterField(r fieldResult) {
	s.Note = ""
	switch r { //nolint:exhaustive // done and tab are the owner's
	case fieldDiscard:
		s.Field = field{}
	case fieldRefused:
		s.Note = fmt.Sprintf("text over %s, not taken", sizeText(s.Field.Limit))
	}
	s.layoutField()
}

// layoutField lays the open field out for the screen: as wide as the popup
// may be, so typing does not move the box, as high as its rows up to the
// popup's bound, then scrolled to keep the cursor in view.
func (s *state) layoutField() {
	f := &s.Field
	if !f.Open {
		return
	}
	inner := s.boxMaxInner()
	f.M = s.Cells
	f.Rows = fieldRows(f.Text, inner, f.M)
	f.W, f.H = inner+popupFrame, min(len(f.Rows)+popupFrame, s.boxMaxHeight())
	if inner < 2 || f.H <= popupFrame {
		// No room for text and the column the cursor needs at its end; the
		// field still takes the keys.
		f.W, f.H = 0, 0
	}
	f.X, f.Y = s.boxAt(f.W, f.H, f.AtX, f.AtY)
	f.Row = rowOf(f.Rows, f.Cursor)
	shown := f.shown()
	f.Scroll = min(max(f.Scroll, f.Row-shown+1, 0), f.Row, max(len(f.Rows)-shown, 0))
}

func (f *field) contains(x, y int) bool {
	return f.Open && f.box.contains(x, y)
}

// sizeText is n bytes for the footer, in KiB where they are whole.
func sizeText(n int) string {
	if n >= 1<<10 && n%(1<<10) == 0 {
		return fmt.Sprintf("%d KiB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

// writeRow writes row j with the cursor on its grapheme, or after the
// row's end as a blank.
func (f *field) writeRow(ln *line, j int) {
	if j >= len(f.Rows) {
		return
	}
	r := f.Rows[j]
	st := cellStyle{fixed: popupStyle}
	if at := f.Cursor - r.Start; j == f.Row && f.Cursor < r.End {
		st.runs = []styledRun{{at, popupStyle}, {at + 1, caretStyle}}
	}
	ln.window(f.Text[r.Start:r.End], nil, 0, st)
	if j == f.Row && f.Cursor == r.End {
		ln.spaces(1, caretStyle)
	}
}
