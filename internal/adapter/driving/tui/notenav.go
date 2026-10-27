package tui

import (
	"slices"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// onNoteAction does an action of the view on notes.
func (m *model) onNoteAction(a ui.Action) {
	switch a { //nolint:exhaustive // the view's actions on notes only
	case ui.AddNote:
		m.addNote()
	case ui.NextNote:
		m.load(m.s.seekNote(1))
	case ui.PrevNote:
		m.load(m.s.seekNote(-1))
	case ui.Notes:
		m.openOverview()
	}
}

// noteField does what the note's owner makes of a key in the field.
func (m *model) noteField(r fieldResult) {
	switch r {
	case fieldDone:
		m.noteFieldDone()
	case fieldTab:
		m.s.noteFieldTab()
	case fieldDiscard:
		// A note already sent is the book's; its outcome still comes.
		if op := m.s.Notes.Draft.Op; op != 0 {
			m.s.Notes.Discarded = op
		}
		m.s.Notes.Draft = noteDraft{}
	case fieldEdited, fieldRefused:
	}
}

// noteLines are the lines of the body where the notes placed in file
// start, in order.
func (s *state) noteLines(file int) []int {
	rows := s.snapOf(file).Diff.Rows
	var out []int
	for _, w := range []filediff.Which{filediff.OldSide, filediff.NewSide} {
		for _, sp := range s.spotsOf(file, w) {
			if !sp.Place.Where.Shown() {
				continue
			}
			row := slices.IndexFunc(rows, func(r diff.Row) bool { return lineOf(r, w) >= sp.Place.Start-1 })
			if row >= 0 {
				out = append(out, s.rowIndex(file, row))
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// seekNote brings the next note below the line it last went to, or the
// top line, to the top; by dir -1 the last one above. A note on the last
// page cannot come to the top, so the line it went to counts while it is
// on screen. Past the files on screen it opens the next file in that
// direction with open notes, and goes to its first or last note once they
// are placed.
func (s *state) seekNote(dir int) (filediff.Request, bool) {
	from := s.top()
	if at := s.Notes.At; s.Notes.AtLaid == s.laid() && at >= from && at < from+s.Layout.shown() {
		from = at
	}
	first, end := s.bodyFiles()
	start := first
	if dir < 0 {
		start = end - 1
	}
	for f := start; f >= first && f < end; f += dir {
		lines := s.noteLines(f)
		if dir < 0 {
			slices.Reverse(lines)
		}
		for _, line := range lines {
			if (dir > 0 && line > from) || (dir < 0 && line < from) {
				s.goToNote(line)
				return filediff.Request{}, false
			}
		}
	}
	if s.Stream.On {
		s.Note = "no further notes"
		return filediff.Request{}, false
	}
	for i := s.Current + dir; i >= 0 && i < len(s.Files); i += dir {
		if s.openNotesIn(i) > 0 {
			r, ok := s.open(i, false)
			s.Notes.Seek = dir
			return r, ok
		}
	}
	s.Note = "no further notes"
	return filediff.Request{}, false
}

func (s *state) goToNote(line int) {
	s.scrollTo(line)
	s.Notes.At, s.Notes.AtLaid = line, s.laid()
}

// laidKey is what the lines of the body depend on: the file and its
// load, the stream's files, and the mode.
type laidKey struct {
	file           int
	gen, streamGen filediff.Gen
	mode           view.Mode
	stream, visit  bool
}

func (s *state) laid() laidKey {
	return laidKey{s.Current, s.ShownGen, s.Stream.Gen, s.Mode, s.Stream.On, s.visiting()}
}

// seekPlaced goes to the first or last note of the file opened by
// seekNote, once its notes are placed; where none has a place, it stays
// at the top.
func (s *state) seekPlaced() {
	dir := s.Notes.Seek
	if dir == 0 || !s.Loaded {
		return
	}
	s.Notes.Seek = 0
	lines := s.noteLines(s.Current)
	if len(lines) == 0 {
		s.Note = "the notes of this file have no place"
		return
	}
	line := lines[0]
	if dir < 0 {
		line = lines[len(lines)-1]
	}
	s.goToNote(line)
}

// openNotesIn is how many open notes the book has on a side of file i of
// the comparison.
func (s *state) openNotesIn(i int) int {
	n := 0
	hs, k := s.headsOf(i)
	for _, h := range hs[:k] {
		n += s.Notes.Open[h.Key]
	}
	return n
}
