package tui

import (
	"strings"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/syntax"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// sideLine is what one pane shows of a row: the line's index in its side,
// -1 for none, its text and how it is marked and coloured.
type sideLine struct {
	index  int
	text   string
	tone   tone
	mark   string
	spans  []diff.Span
	runs   []syntax.Run
	points []filediff.Checkpoint
}

// writeRow writes one line of file's view: both panes, the unified one, or
// a fold across them.
func writeRow(b *strings.Builder, s *state, th *theme, file int, l view.Line) {
	lay := s.layoutOf(file)
	ln := newLine(b, th, s.Cells, lay.panes())
	if l.Fold {
		ln.spaces(lay.gutter(), foldStyle)
		ln.text("⋯ ", foldStyle)
		ln.number(l.Rows.End-l.Rows.Start, 0, foldStyle)
		ln.text(" unchanged lines", foldStyle)
		ln.end(foldStyle)
		return
	}
	if lay.Unified {
		writeUnified(&ln, s, lay, file, l)
		return
	}
	left, right := sides(s.snapOf(file), l.Rows.Start)
	writePane(&ln, lay.Left, left, s.HScroll, s.caretOn(file, l.Rows.Start, filediff.OldSide))
	ln.text("│", separatorStyle)
	writePane(&ln, lay.Right, right, s.HScroll, s.caretOn(file, l.Rows.Start, filediff.NewSide))
	ln.end(plain)
}

// writeUnified writes a line of the unified view: the old line number,
// then a pane of the line's side with the new one. A changed row's halves
// take the tones of a deletion and an insertion, as their words are marked.
func writeUnified(ln *line, s *state, lay layout, file int, l view.Line) {
	left, right := sides(s.snapOf(file), l.Rows.Start)
	sl, old := right, left.index
	// A whole row shows the caret from either side: it keeps the side it
	// had side by side.
	w := s.Caret.Side
	switch l.Half {
	case view.OldHalf:
		w, sl = filediff.OldSide, left
		sl.tone, sl.mark, sl.index = deletedTone, "-", -1
	case view.NewHalf:
		w, sl, old = filediff.NewSide, right, -1
		sl.tone, sl.mark = insertedTone, "+"
		if s.visiting() {
			// Read only: the file as it is, not as an insertion.
			sl.tone, sl.mark = equalTone, " "
		}
	case view.Whole:
		if s.halfSide(file, l) == filediff.OldSide {
			sl.text, sl.points, sl.runs = left.text, left.points, left.runs
		}
	}
	gutter := paneStyle(sl.tone, gutterRole)
	ln.spaces(1, gutter)
	writeNumber(ln, old, lay.Left.Digits, gutter)
	writePane(ln, pane{Width: ln.free, Digits: lay.Right.Digits}, sl, s.HScroll, s.caretOn(file, l.Rows.Start, w))
	ln.end(plain)
}

// writeNumber writes line index+1 in digits columns, blank for no line.
func writeNumber(ln *line, index, digits int, st style) {
	if index < 0 {
		ln.spaces(digits, st)
		return
	}
	ln.number(index+1, digits, st)
}

// caretOn is the caret if it is on side w of row of file, else none.
func (s *state) caretOn(file, row int, w filediff.Which) caret {
	c := s.Caret
	c.On = c.On && c.File == file && c.Row == row && c.Side == w
	return c
}

// sides is how row i of snap shows on either side.
func sides(snap *filediff.Snapshot, i int) (left, right sideLine) {
	d := snap.Diff
	row := d.Rows[i]
	left, right = sideLine{index: row.Left, mark: " "}, sideLine{index: row.Right, mark: " "}
	switch row.Kind {
	case diff.Equal:
		left.tone, right.tone = equalTone, equalTone
	case diff.Delete:
		left.tone, left.mark, right.tone = deletedTone, "-", absentTone
	case diff.Insert:
		left.tone, right.tone, right.mark = absentTone, insertedTone, "+"
	case diff.Change:
		left.tone, left.mark, right.tone, right.mark = changedTone, "~", changedTone, "~"
		left.spans, right.spans = d.InlineAt(i)
	}
	if row.Left >= 0 {
		left.text, left.points = diff.Content(d.Old.Lines[row.Left]), snap.Columns.Old.At(row.Left)
		left.runs = snap.Highlights.Old.Line(row.Left)
	}
	if row.Right >= 0 {
		right.text, right.points = diff.Content(d.New.Lines[row.Right]), snap.Columns.New.At(row.Right)
		right.runs = snap.Highlights.New.Line(row.Right)
	}
	return left, right
}

// writePane writes one side of a row into its pane: the gutter with line
// number and mark, then the text from column skip on, with the caret if
// it is on this line. Nothing spills over into the next pane, however
// narrow.
func writePane(ln *line, p pane, sl sideLine, skip int, c caret) {
	rest := ln.narrow(p.Width)
	gutter := paneStyle(sl.tone, gutterRole)
	ln.spaces(1, gutter)
	writeNumber(ln, sl.index, p.Digits, gutter)
	ln.spaces(1, gutter)
	ln.text(sl.mark, gutter)
	ln.spaces(1, gutter)
	ln.window(sl.text, sl.points, skip, cellStyle{pane: true, tone: sl.tone, spans: sl.spans, syntax: sl.runs, caret: c})
	ln.spaces(ln.free, textStyle(sl.tone, syntax.Plain, false))
	ln.free = rest
}
