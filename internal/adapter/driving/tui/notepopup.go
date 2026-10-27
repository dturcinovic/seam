package tui

import (
	"slices"

	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
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
	p := s.openPopup(notesContent(spots, s.Notes.Sessions), x, y)
	if p.Open {
		p.Notes, p.NotesX, p.NotesY = &l, x, y
		p.Title = "notes"
	}
	s.Hover.Popup = p
}

// refreshNotePopup shows the notes popup anew, as notes came, moved or
// were resolved, or were answered: a line's, or a note's of the overview.
// The text keeps where it was scrolled to, as far as it still reaches.
func (s *state) refreshNotePopup() {
	p := s.Hover.Popup
	switch n, ok := s.overviewNote(p.Note); {
	case p.Notes != nil:
		s.openNotes(*p.Notes, p.NotesX, p.NotesY)
	case ok:
		s.openNotePopup(n, p.NotesX, p.NotesY)
	default:
		return
	}
	s.Hover.Popup.scrollBy(p.Scroll)
}

// overviewNote is note id as the overview last had it.
func (s *state) overviewNote(id note.ID) (note.Note, bool) {
	ns := s.Notes.Overview.Notes
	i := slices.IndexFunc(ns, func(n note.Note) bool { return n.ID == id })
	if id == "" || i < 0 {
		return note.Note{}, false
	}
	return ns[i], true
}

// notesContent is the popup's text for spots: each note's kind, place
// and state, then its text as written and the answers to it.
func notesContent(spots []note.Spot, sessions []note.Session) hover.Content {
	var c hover.Content
	for i, sp := range spots {
		if i > 0 {
			c.Blocks = append(c.Blocks, hover.Block{Kind: hover.Rule})
		}
		head := sp.Note.Kind.String()
		if sp.Place.Where != note.Exact {
			head += " · " + sp.Place.Where.String()
		}
		if sp.Note.State != note.Open {
			head += " · " + stateText(sp.Note, sessions)
		}
		c.Blocks = append(c.Blocks,
			hover.Block{Kind: hover.Heading, Spans: []hover.Span{{Kind: hover.Strong, Text: head}}},
			hover.Block{Kind: hover.Code, Code: sp.Note.Text})
		c.Blocks = append(c.Blocks, repliesBlocks(sp.Note.Replies)...)
	}
	return c
}

// stateText is a note's state as a popup tells it: a taken one names the
// agent that took it and since when.
func stateText(n note.Note, sessions []note.Session) string {
	if n.State != note.Taken || n.Claim == nil {
		return n.State.String()
	}
	by := "an agent"
	if s, ok := note.TakenBy(n, sessions); ok {
		by = s.Client
	}
	return "taken by " + by + " since " + n.Claim.At.Local().Format("Jan 2 15:04")
}

// repliesBlocks are the answers to a note, each under its author and read
// as Markdown, as agents write them. Their fences name no language seam
// colours: the TUI knows no table of languages.
func repliesBlocks(replies []note.Reply) []hover.Block {
	var bs []hover.Block
	for _, r := range replies {
		bs = append(bs,
			hover.Block{Kind: hover.Rule},
			hover.Block{Kind: hover.Heading, Spans: []hover.Span{{Kind: hover.Strong, Text: r.Author}}})
		bs = append(bs, hover.Tidy(hover.Markdown(r.Text, lang.Table{}))...)
	}
	return bs
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

// popupNote is the note a key in the popup acts on: the popup's own note,
// or the first of a line's, that match takes. A note's popup without the
// overview behind it leaves the check to the book.
func (s *state) popupNote(match func(note.Note) bool) (note.ID, bool) {
	p := s.Hover.Popup
	if p.Note != "" {
		n, ok := s.overviewNote(p.Note)
		return p.Note, !ok || match(n)
	}
	if p.Notes == nil {
		return "", false
	}
	for _, sp := range s.notesAt(p.Notes.File, p.Notes.Side, p.Notes.Line) {
		if match(sp.Note) {
			return sp.Note.ID, true
		}
	}
	return "", false
}

// resolveNote resolves the note of the popup, or the first of a line's.
func (m *model) resolveNote() {
	id, ok := m.s.popupNote(func(note.Note) bool { return true })
	if !ok {
		return
	}
	if m.s.Hover.Popup.Note != "" {
		// The popup of one note goes with it.
		m.dropHover()
	}
	m.resolve(id)
}

// resolve marks note id done; the book's change brings the marks and
// lists up to date.
func (m *model) resolve(id note.ID) {
	m.p.NoteWriter.Resolve(id)
	m.s.Note = "note resolved"
}

// releaseNote gives back the note of the popup, or the first taken of a
// line's.
func (m *model) releaseNote() {
	if id, ok := m.s.popupNote(func(n note.Note) bool { return n.State == note.Taken }); ok {
		m.release(id)
		return
	}
	m.s.Note = "no note taken here"
}

// release gives note id back from the agent that took it, for another to
// take.
func (m *model) release(id note.ID) {
	m.p.NoteReleaser.Release(id)
	m.s.Note = "note given back"
}
