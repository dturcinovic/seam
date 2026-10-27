package tui

import (
	"slices"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// The field of a note stands between the lines of the body, below the last
// line it notes, and the code below it moves down while it is open. It is
// a gap in the drawing only: the body's lines keep their indices, so the
// jumps and the stream know nothing of it, and none of them can move while
// the field takes every key. The mouse rests but for the wheel, which
// scrolls with no line to find, so only drawing goes through bodyRowAt.

// gapSpot is where the field stands: below line At of the body, H lines
// high; OK is false while it shows nowhere: closed, without room, or at
// lines that are not laid out. Finding the line reads the file's rows, so
// a frame finds it once.
type gapSpot struct {
	At, H int
	OK    bool
}

func (s *state) gap() gapSpot {
	f := &s.Field
	if !f.Open || f.H == 0 {
		return gapSpot{}
	}
	t := s.Notes.Draft.Lines
	at, ok := s.lineIndex(t.File, t.Side, t.To)
	return gapSpot{At: at, H: f.H, OK: ok}
}

// lineIndex is the line of the body that shows line i of side w of file;
// ok is false while the file is not laid out or folds the line.
func (s *state) lineIndex(file int, w filediff.Which, i int) (int, bool) {
	lines := s.linesOf(file)
	rows := s.snapOf(file).Diff.Rows
	row := slices.IndexFunc(rows, func(r diff.Row) bool { return lineOf(r, w) == i })
	if len(lines) == 0 || row < 0 {
		return 0, false
	}
	h := view.NewHalf
	if w == filediff.OldSide {
		h = view.OldHalf
	}
	j := view.LineFor(lines, row, h)
	if j >= len(lines) || lines[j].Fold || !lines[j].Contains(row) {
		return 0, false
	}
	return s.bodyIndex(file, j), true
}

// bodyRowAt is what line i of the body's screen shows with the field at
// g: line of the body, or with inGap row boxRow of the field's box. A gap
// whose line went above the top goes with it.
func (s *state) bodyRowAt(g gapSpot, i int) (line, boxRow int, inGap bool) {
	t, at, h := s.top(), g.At, g.H
	switch {
	case !g.OK || at < t || i <= at-t:
		return t + i, 0, false
	case i <= at-t+h:
		return 0, i - (at - t) - 1, true
	}
	return t + i - h, 0, false
}

// scrollHeight is how many lines of the body the screen shows besides the
// field's box: scrolling by it keeps the last lines in reach below the box.
func (s *state) scrollHeight() int {
	h := s.bodyHeight()
	if g := s.gap(); g.OK {
		h -= g.H
	}
	return max(h, 1)
}

// showGap scrolls the lines noted and the box below them into view, where
// they fit.
func (s *state) showGap() {
	g := s.gap()
	if !g.OK {
		return
	}
	at, h, t, shown := g.At, g.H, s.top(), s.Layout.shown()
	switch {
	case at < t:
		s.scrollTo(at)
	case at-t+h >= shown:
		s.scrollTo(at + h + 1 - shown)
	}
}

// gapColumns are the screen columns of the text of the side noted, where
// the box stands.
func (s *state) gapColumns() (from, to int) {
	t := s.Notes.Draft.Lines
	return s.layoutOf(t.File).text(t.Side)
}
