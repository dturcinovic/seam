package tui

import (
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// noteLine is a line that notes stand at: Line of side Side of File.
type noteLine struct {
	File int
	Side filediff.Which
	Line int
}

// openNotes opens the popup with the notes at l for cell x, y, or closes
// it where l has none.
func (s *state) openNotes(l noteLine, x, y int) {
	spots := s.notesAt(l.File, l.Side, l.Line)
	if len(spots) == 0 {
		s.Hover.Popup = popup{}
		return
	}
	p := s.openPopup(notesContent(spots), x, y)
	if p.Open {
		p.Notes, p.NotesX, p.NotesY = &l, x, y
		p.Title = "notes"
	}
	s.Hover.Popup = p
}

// refreshNotePopup shows the notes popup anew, as notes came, moved or
// were resolved.
func (s *state) refreshNotePopup() {
	p := s.Hover.Popup
	if p.Notes != nil {
		s.openNotes(*p.Notes, p.NotesX, p.NotesY)
	}
}

// notesContent is the popup's text for spots: each note's kind and place,
// then its text as written.
func notesContent(spots []note.Spot) hover.Content {
	var c hover.Content
	for i, sp := range spots {
		if i > 0 {
			c.Blocks = append(c.Blocks, hover.Block{Kind: hover.Rule})
		}
		head := sp.Note.Kind.String()
		if sp.Place.Where != note.Exact {
			head += " · " + sp.Place.Where.String()
		}
		c.Blocks = append(c.Blocks,
			hover.Block{Kind: hover.Heading, Spans: []hover.Span{{Kind: hover.Strong, Text: head}}},
			hover.Block{Kind: hover.Code, Code: sp.Note.Text})
	}
	return c
}

// notesAtCaret opens the notes at the caret's line, once it came there.
func (s *state) notesAtCaret() {
	if l, x, y, ok := s.caretNoteLine(); ok {
		s.openNotes(l, x, y)
	}
}

// caretNoteLine is the line of the side the caret stands on, and the
// caret's screen cell.
func (s *state) caretNoteLine() (l noteLine, x, y int, ok bool) {
	p, ok := s.spot()
	if !ok {
		return noteLine{}, 0, 0, false
	}
	c := s.Caret
	w := s.caretSide(p.line)
	line := lineOf(s.snapOf(c.File).Diff.Rows[c.Row], w)
	x, y, ok = s.caretCell(p)
	return noteLine{File: c.File, Side: w, Line: line}, x, y, ok && line >= 0
}

// noteMarkAt is the line whose gutter mark lies at screen cell x, y.
func (s *state) noteMarkAt(x, y int) (noteLine, bool) {
	file, l, sides, ok := s.gutterAt(x, y)
	if !ok {
		return noteLine{}, false
	}
	for _, w := range sides {
		line := s.lineIn(file, l, w)
		if s.layoutOf(file).onMarks(w, x) && line >= 0 && len(s.notesAt(file, w, line)) > 0 {
			return noteLine{File: file, Side: w, Line: line}, true
		}
	}
	return noteLine{}, false
}

// gutterAt is the line at screen cell x, y and the sides whose marks its
// gutter shows there: side by side the pane's under x; unified the half's,
// or for a whole row either side, the new one first, as writeUnified marks
// it.
func (s *state) gutterAt(x, y int) (file int, l view.Line, sides []filediff.Which, ok bool) {
	i, ok := s.Layout.bodyLine(x, y)
	if !ok {
		return 0, view.Line{}, nil, false
	}
	file, l, ok = s.rowLine(s.top() + i)
	if !ok || l.Fold {
		return 0, view.Line{}, nil, false
	}
	lay := s.layoutOf(file)
	switch {
	case !lay.Unified:
		w, ok := lay.paneAt(x)
		return file, l, []filediff.Which{w}, ok
	case l.Half == view.Whole:
		return file, l, []filediff.Which{filediff.NewSide, filediff.OldSide}, true
	}
	return file, l, []filediff.Which{s.halfSide(file, l)}, true
}

// lineIn is the line of side w on line l of file's view, -1 for none.
func (s *state) lineIn(file int, l view.Line, w filediff.Which) int {
	return lineOf(s.snapOf(file).Diff.Rows[l.Rows.Start], w)
}

// resolveNote resolves the note of the popup, or the first of a line's.
func (m *model) resolveNote() {
	p := m.s.Hover.Popup
	if p.Note != "" {
		m.dropHover()
		m.resolve(p.Note)
		return
	}
	if p.Notes == nil {
		return
	}
	spots := m.s.notesAt(p.Notes.File, p.Notes.Side, p.Notes.Line)
	if len(spots) == 0 {
		return
	}
	m.resolve(spots[0].Note.ID)
}

// resolve marks note id done; the book's change brings the marks and
// lists up to date.
func (m *model) resolve(id note.ID) {
	m.p.NoteWriter.Resolve(id)
	m.s.Note = "note resolved"
}
