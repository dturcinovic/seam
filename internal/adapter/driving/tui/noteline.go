package tui

import (
	"slices"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// A note placed in a side shows a line of its own below its last line: its
// kind and the first line of its text. The lines are lines of the view,
// laid in by view.Annotate whenever the lines or the notes change; a note
// line's Ref names the note as its index in spotsOf times two plus its
// side, so it needs no state of its own.

// noteRef is the Ref of the spot at index i of side w.
func noteRef(i int, w filediff.Which) int {
	if w == filediff.NewSide {
		return 2*i + 1
	}
	return 2 * i
}

// refSpot is the spot a note line's ref names in file; ok is false where
// the spots changed under it.
func (s *state) refSpot(file, ref int) (note.Spot, filediff.Which, bool) {
	w := filediff.OldSide
	if ref%2 == 1 {
		w = filediff.NewSide
	}
	spots := s.spotsOf(file, w)
	if i := ref / 2; i < len(spots) {
		return spots[i], w, true
	}
	return note.Spot{}, w, false
}

// noteAts are the note lines of file as snap lays it out: one below the
// last line of each note placed, none while they are hidden.
func (s *state) noteAts(file int, snap *filediff.Snapshot) []view.NoteAt {
	if s.Notes.Hidden {
		return nil
	}
	var out []view.NoteAt
	for _, w := range []filediff.Which{filediff.OldSide, filediff.NewSide} {
		spots := s.spotsOf(file, w)
		if len(spots) == 0 {
			continue
		}
		rows := rowsOfLines(snap.Diff.Rows, w)
		for i, sp := range spots {
			if last := sp.Place.End - 1; sp.Place.Where.Shown() && last >= 0 && last < len(rows) {
				out = append(out, view.NoteAt{Row: rows[last], Half: halfOf(w), Ref: noteRef(i, w)})
			}
		}
	}
	return out
}

// halfOf is the half of a unified row that shows side w.
func halfOf(w filediff.Which) view.Half {
	if w == filediff.OldSide {
		return view.OldHalf
	}
	return view.NewHalf
}

// rowsOfLines is the row of each line of side w of rows.
func rowsOfLines(rows []diff.Row, w filediff.Which) []int {
	var out []int
	for r, row := range rows {
		if lineOf(row, w) >= 0 {
			out = append(out, r)
		}
	}
	return out
}

// annotateFile lays the note lines of file in anew after its notes
// changed; the top line stays.
func (s *state) annotateFile(file int) {
	if s.relayNotes(file) {
		s.placeTop()
	}
}

// relayNotes lays the note lines of file in anew, and reports whether
// they changed: only then do the lines of the body move.
func (s *state) relayNotes(file int) bool {
	lines := s.linesOf(file)
	if len(lines) == 0 {
		return false
	}
	if s.Stream.On {
		if h, ok := s.Stream.Held[file]; !ok || h.err != nil {
			return false
		}
	} else if !s.Loaded {
		return false
	}
	out := view.Annotate(lines, s.noteAts(file, s.snapOf(file)))
	if slices.Equal(out, lines) {
		return false
	}
	s.layNotes(file, lines)
	return true
}

// layNotes gives file lines with its note lines laid in last, as Annotate
// must come after the lines are laid out or a fold opened.
func (s *state) layNotes(file int, lines []view.Line) {
	out := view.Annotate(lines, s.noteAts(file, s.snapOf(file)))
	s.Notes.Laid++
	if s.Stream.On {
		s.Stream.View.Lay(file, out)
		return
	}
	s.show(out)
}

// placeTop places the body after its lines changed, keeping the top line.
func (s *state) placeTop() {
	if s.Stream.On {
		s.switchStream()
		return
	}
	s.Pos = view.Switch(s.Lines, s.Pos, s.scrollHeight())
}

// toggleNoteLines hides the note lines, or shows them again.
func (s *state) toggleNoteLines() {
	s.Notes.Hidden = !s.Notes.Hidden
	first, end := s.bodyFiles()
	moved := false
	for f := first; f < end; f++ {
		moved = s.relayNotes(f) || moved
	}
	if moved {
		s.placeTop()
	}
}

// writeNoteLine writes the line of the note ref names in file: its kind in
// the number column and the first line of its text, cut to the text of
// its side. A note gone from under the line leaves it blank.
func writeNoteLine(b *strings.Builder, s *state, th *theme, file, ref int) {
	sp, w, ok := s.refSpot(file, ref)
	if !ok {
		writeSideLine(b, s, th, file, w, func(*line, layout, func(int)) {})
		return
	}
	look, dim := lookOf(sp.Note.Kind), sp.Place.Where == note.Likely
	writeSideLine(b, s, th, file, w, func(ln *line, lay layout, to func(int)) {
		p := lay.sidePane(w)
		to(lay.paneStart(w) - lay.List + p.Marks + p.Digits - 1)
		ln.text(noteSign(sp.Note), paneStyle(equalTone, look.marks(dim)))
		from, end := lay.text(w)
		to(from - lay.List)
		st := look.text
		if dim {
			st = look.dimText
		}
		text, _, _ := strings.Cut(sp.Note.Text, "\n")
		writeCut(ln, text, end-from, st, false)
	})
}
