package tui

import (
	"slices"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// draftLost is why the field marks no lines; the zero value is that it
// marks them.
type draftLost uint8

const (
	linesFound draftLost = iota
	linesNotFound
	linesAmbiguous
	sideNotShown
)

// String is the footer's reason, short to leave room for the keys.
func (l draftLost) String() string {
	switch l {
	case linesNotFound:
		return "lines noted not found"
	case linesAmbiguous:
		return "lines noted found more than once"
	case sideNotShown:
		return "lines noted not shown"
	case linesFound:
	}
	return ""
}

// linesChanged is the footer's word on lines found changed within.
const linesChanged = "the lines noted changed"

// followDraft keeps the field at its lines as their side is shown anew:
// at its index among the files, which a reload changes, and in a new
// version found again by its anchor as the notes saved are. Where the
// anchor is not found, the field stays near where its lines were and marks
// none; where no side on screen is the one noted, it shows nowhere. The
// anchor itself never changes, so the note still goes to the lines meant.
func (s *state) followDraft() {
	if !s.Field.Open {
		return
	}
	d := &s.Notes.Draft
	ref, k, ok := s.draftSide()
	if !ok {
		s.loseDraft(sideNotShown)
		d.Lines.File = -1
		return
	}
	back := d.Lost == sideNotShown
	d.Lines.File, d.Lines.Side = ref.File, ref.W
	if k.Blob == d.Blob && !back {
		// A reload lays the file out with its folds anew, also in the same
		// version, and the field's columns are its file's.
		s.unfoldDraft()
		s.layoutField()
		return
	}
	lines := sideText(s.snapOf(ref.File), ref.W)
	p := note.Locate(d.Anchor, k.Blob, lines)
	d.Blob = k.Blob
	if s.Note == probablyHere || s.Note == linesChanged {
		s.Note = ""
	}
	switch p.Where {
	case note.Exact, note.Likely, note.Changed:
		d.Lines.From, d.Lines.To, d.Lost = p.Start-1, p.End-1, linesFound
		if w := placeWord(p.Where); w != "" && s.Note == "" {
			s.Note = w
		}
	case note.Ambiguous, note.Orphaned:
		d.Lines.To = min(d.Lines.To, len(lines)-1)
		d.Lines.From = d.Lines.To
		why := linesNotFound
		if p.Where == note.Ambiguous {
			why = linesAmbiguous
		}
		s.loseDraft(why)
	}
	s.unfoldDraft()
	s.layoutField()
	if !d.Away {
		s.showGap()
	}
}

// loseDraft says why the field marks no lines. Lines lost in a new
// version go ahead of what the footer said before, which was of the lines
// as they were; a side the wheel scrolled out of the stream's window is no
// such news.
func (s *state) loseDraft(why draftLost) {
	d := &s.Notes.Draft
	if why != sideNotShown {
		s.Note = ""
	}
	d.Lost = why
}

// draftSide is the side on screen with the field's lines: its anchor's
// path and origin, which a visit keeps; a file renamed is another.
func (s *state) draftSide() (sideRef, note.SnapshotKey, bool) {
	a := s.Notes.Draft.Anchor
	for k, ref := range s.noteSides() {
		if k.Path == a.Path && k.Origin == a.Origin {
			return ref, k, true
		}
	}
	return sideRef{}, note.SnapshotKey{}, false
}

// placeWord is the footer's word on lines found only so surely, empty for
// none.
func placeWord(w note.Where) string {
	switch w {
	case note.Likely:
		return probablyHere
	case note.Changed:
		return linesChanged
	case note.Exact, note.Ambiguous, note.Orphaned:
	}
	return ""
}

// unfoldDraft opens a fold that hides the field's last line.
func (s *state) unfoldDraft() {
	t := s.Notes.Draft.Lines
	lines := s.linesOf(t.File)
	row := slices.IndexFunc(s.snapOf(t.File).Diff.Rows, func(r diff.Row) bool { return lineOf(r, t.Side) == t.To })
	if row < 0 || len(lines) == 0 {
		return
	}
	if i := view.LineForRow(lines, row); lines[i].Fold {
		s.unfoldLine(s.bodyIndex(t.File, i))
	}
}
