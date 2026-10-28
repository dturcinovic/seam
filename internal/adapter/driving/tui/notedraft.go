package tui

import (
	"fmt"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// noteTarget is the lines a new note takes: From to To, 0-based and To
// included, of side Side of File.
type noteTarget struct {
	File     int
	Side     filediff.Which
	From, To int
}

// addNote opens the field for a note at the caret's line, at the lines
// marked with v up to it, or without the caret at the first change on
// screen; the footer says why where no note can go.
func (m *model) addNote() {
	t, ok := m.s.noteTarget()
	if !ok {
		m.s.Note = "no lines here to note"
		return
	}
	if m.openNote(t) {
		// The lines marked are taken; a click on a plus leaves them marked.
		m.s.Notes.Mark = lineMark{}
	}
}

// openNote opens the field for a note at the lines of t; ok is false
// where none can go, as the footer then says.
func (m *model) openNote(t noteTarget) (ok bool) {
	k, ok := m.s.noteKey(t.File, t.Side)
	if !ok {
		m.s.Note = "no notes on this side: it is no commit, index or worktree file"
		return false
	}
	a, err := note.NewAnchor(k.Path, k.Origin, k.Blob, sideText(m.s.snapOf(t.File), t.Side), t.From+1, t.To+1)
	if err != nil {
		m.s.Note = "no note: " + err.Error()
		return false
	}
	// The field lays itself out at the lines of the draft.
	m.s.Notes.Draft = noteDraft{Kind: note.Question, Anchor: a, Comparison: comparisonText(&m.s), Lines: t}
	m.openField(note.MaxText)
	return true
}

// comparisonText names the comparison for whoever reads the note.
func comparisonText(s *state) string {
	if s.visiting() {
		return "worktree"
	}
	return s.Comparison.Left.String() + ".." + s.Comparison.Right.String()
}

// noteTarget is where c sets a note: see addNote.
func (s *state) noteTarget() (noteTarget, bool) {
	if !s.Caret.On {
		return s.changeTarget()
	}
	l, _, _, ok := s.caretNoteLine()
	if !ok {
		return noteTarget{}, false
	}
	c, w, line := s.Caret, l.Side, l.Line
	rows := s.snapOf(c.File).Diff.Rows
	t := noteTarget{File: c.File, Side: w, From: line, To: line}
	if mk := s.Notes.Mark; mk.On && mk.File == c.File && mk.Side == w && mk.Row < len(rows) {
		if from := lineOf(rows[mk.Row], w); from >= 0 {
			t.From, t.To = min(from, line), max(from, line)
		}
	}
	return t, true
}

// changeTarget is the first change on screen: the lines of its block on
// the new side, or on the old one where it has none there.
func (s *state) changeTarget() (noteTarget, bool) {
	// No field is open, so no gap moves the lines.
	for i := range s.Layout.shown() {
		file, l, ok := s.rowLine(s.top() + i)
		if !ok || l.Fold {
			continue
		}
		rows := s.snapOf(file).Diff.Rows
		if rows[l.Rows.Start].Kind == diff.Equal {
			continue
		}
		end := l.Rows.Start
		for end < len(rows) && rows[end].Kind != diff.Equal {
			end++
		}
		for _, w := range []filediff.Which{filediff.NewSide, filediff.OldSide} {
			if t, ok := blockLines(rows[l.Rows.Start:end], w); ok {
				t.File = file
				return t, true
			}
		}
	}
	return noteTarget{}, false
}

// blockLines are the first and last lines of side w in rows.
func blockLines(rows []diff.Row, w filediff.Which) (noteTarget, bool) {
	t := noteTarget{Side: w, From: -1}
	for _, r := range rows {
		if i := lineOf(r, w); i >= 0 {
			if t.From < 0 {
				t.From = i
			}
			t.To = i
		}
	}
	return t, t.From >= 0
}

// markLines marks the caret's line with v, or drops the mark.
func (s *state) markLines() {
	if s.Notes.Mark.On {
		s.Notes.Mark = lineMark{}
		return
	}
	p, ok := s.spot()
	if !ok {
		return
	}
	c := s.Caret
	s.Notes.Mark = lineMark{On: true, File: c.File, Side: s.caretSide(p.line), Row: c.Row}
}

// markText is the footer's word on the lines marked, empty for none.
func (s *state) markText() string {
	mk := s.Notes.Mark
	if !mk.On || mk.Row >= len(s.snapOf(mk.File).Diff.Rows) {
		return ""
	}
	line := lineOf(s.snapOf(mk.File).Diff.Rows[mk.Row], mk.Side)
	return fmt.Sprintf("from line %d, c notes up to the caret  ", line+1)
}

// noteFieldDone takes enter in the note's field: the text goes to the
// book, and the field stays open with it until the book took it.
func (m *model) noteFieldDone() {
	d := &m.s.Notes.Draft
	if d.Op != 0 {
		return
	}
	if strings.TrimSpace(m.s.Field.Text) == "" {
		m.s.Note = "a note needs a text"
		return
	}
	if d.Edit != "" {
		d.Op = m.p.NoteEditor.Edit(d.Edit, d.Kind, m.s.Field.Text)
	} else {
		d.Op = m.p.NoteWriter.Add(note.Draft{Kind: d.Kind, Anchor: d.Anchor, Comparison: d.Comparison, Text: m.s.Field.Text})
	}
	m.s.Note = "saving the note…"
}

// noteFieldTab switches the note's kind between question and finding.
func (s *state) noteFieldTab() {
	d := &s.Notes.Draft
	if d.Kind == note.Question {
		d.Kind = note.Finding
	} else {
		d.Kind = note.Question
	}
}

// noteDone takes the outcome of a change of the book: the note written
// closes its field, a failure keeps the draft and says why.
func (s *state) noteDone(u note.Update) {
	d, n := &s.Notes.Draft, &s.Notes
	switch {
	case u.Op != 0 && u.Op == n.Discarded:
		n.Discarded = 0
		s.Note = "note saved before esc came"
		if u.Err != nil {
			s.Note = "note discarded, not saved: " + u.Err.Error()
		}
	case u.Op == 0 || u.Op != d.Op:
		if u.Err != nil {
			s.Note = "notes: " + u.Err.Error()
		}
	case u.Err != nil:
		d.Op = 0
		s.Note = "note not saved: " + u.Err.Error()
	default:
		s.Note = ""
		s.closeField()
		*d = noteDraft{}
	}
}
