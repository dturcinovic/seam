package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// openField opens the field for a text of at most limit bytes. The hover
// gives way to it, and so does a definition still to come, whose jump would
// move the code from under it.
func (m *model) openField(limit int) {
	m.dropHover()
	if id := m.s.Jump.Inflight; id != 0 {
		m.p.Define.CancelDefinition(id)
	}
	j := &m.s.Jump
	j.Inflight, j.Waiting, j.Target = 0, nil, nil
	m.s.openField(limit)
}

// openField opens the field; the footer's note, if any, gives way to the
// field's keys.
func (s *state) openField(limit int) {
	s.Field = field{Open: true, Limit: limit, Goal: -1}
	s.Note = ""
	s.layoutField()
	s.showGap()
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
		s.closeField()
	case fieldRefused:
		s.Note = fmt.Sprintf("text over %s, not taken", sizeText(s.Field.Limit))
	}
	s.layoutField()
	s.showGap()
}

// fieldPad keeps the text, and the cursor at either end of a row, off the
// frame, fieldGap off the kinds in the top edge; fieldMinRows gives the
// text room before it is written.
const (
	fieldPad     = 1
	fieldGap     = 1
	fieldMinRows = 2
)

// closeField closes the field; lines its box made room for past the end
// of the body go back to the page.
func (s *state) closeField() {
	s.Field = field{}
	s.scrollTo(s.top())
}

// layoutField lays the open field out for the screen: across the text of
// the side it notes, as high as its rows up to the popup's bound, then
// scrolled to keep the cursor in view.
func (s *state) layoutField() {
	f := &s.Field
	if !f.Open {
		return
	}
	from, to := s.gapColumns()
	f.X, f.W = from, to-from
	inner := f.inner()
	f.M = s.Cells
	f.Rows = fieldRows(f.Text, inner, f.M)
	rows := max(len(f.Rows), fieldMinRows)
	f.H = min(rows+popupFrame+fieldGap, s.boxMaxHeight())
	if inner < 2 || f.H <= popupFrame+fieldGap {
		// No room for text and the column the cursor needs at its end; the
		// field still takes the keys.
		f.W, f.H = 0, 0
	}
	f.Row = rowOf(f.Rows, f.Cursor)
	shown := f.shown() - fieldGap
	f.Scroll = min(max(f.Scroll, f.Row-shown+1, 0), f.Row, max(len(f.Rows)-shown, 0))
}

// inner is how many columns of text the field's box holds.
func (f *field) inner() int { return f.W - popupFrame - 2*fieldPad }

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
