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
	i, ok := s.Layout.bodyLine(x, y)
	if !ok {
		return noteLine{}, false
	}
	file, l, ok := s.rowLine(s.top() + i)
	if !ok || l.Fold {
		return noteLine{}, false
	}
	lay := s.layoutOf(file)
	sides := []filediff.Which{filediff.OldSide, filediff.NewSide}
	switch {
	case !lay.Unified:
	case l.Half == view.Whole:
		// As writeUnified marks a whole row with either side's notes.
		sides = []filediff.Which{filediff.NewSide, filediff.OldSide}
	default:
		sides = []filediff.Which{s.halfSide(file, l)}
	}
	for _, w := range sides {
		line := lineOf(s.snapOf(file).Diff.Rows[l.Rows.Start], w)
		if x == lay.paneStart(w) && line >= 0 && len(s.notesAt(file, w, line)) > 0 {
			return noteLine{File: file, Side: w, Line: line}, true
		}
	}
	return noteLine{}, false
}

// resolveNote resolves the first note of the notes popup.
func (m *model) resolveNote() {
	p := m.s.Hover.Popup
	if p.Notes == nil {
		return
	}
	spots := m.s.notesAt(p.Notes.File, p.Notes.Side, p.Notes.Line)
	if len(spots) == 0 {
		return
	}
	m.p.NoteWriter.Resolve(spots[0].Note.ID)
	m.s.Note = "note resolved"
}
