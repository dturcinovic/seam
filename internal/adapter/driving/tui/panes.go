package tui

import (
	"strings"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/syntax"
	"seam/internal/domain/view"
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

// writeRow writes one line of the view: both panes, or a fold across them.
func writeRow(b *strings.Builder, s *state, th *theme, l view.Line) {
	ln := newLine(b, th, s.Cells, s.Layout.panes())
	if l.Fold {
		ln.spaces(s.Layout.Left.gutter(), foldStyle)
		ln.text("⋯ ", foldStyle)
		ln.number(l.Rows.End-l.Rows.Start, 0, foldStyle)
		ln.text(" unchanged lines", foldStyle)
		ln.end(foldStyle)
		return
	}
	left, right := sides(s.Snapshot, l.Rows.Start)
	writePane(&ln, s.Layout.Left, left, s.HScroll, caret{})
	ln.text("│", separatorStyle)
	c := s.Caret
	c.On = c.On && c.Row == l.Rows.Start
	writePane(&ln, s.Layout.Right, right, s.HScroll, c)
	ln.end(plain)
}

// sides is how row i of snap shows on either side.
func sides(snap filediff.Snapshot, i int) (left, right sideLine) {
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
	if sl.index >= 0 {
		ln.number(sl.index+1, p.Digits, gutter)
	} else {
		ln.spaces(p.Digits, gutter)
	}
	ln.spaces(1, gutter)
	ln.text(sl.mark, gutter)
	ln.spaces(1, gutter)
	ln.window(sl.text, sl.points, skip, cellStyle{pane: true, tone: sl.tone, spans: sl.spans, syntax: sl.runs, caret: c})
	ln.spaces(ln.free, textStyle(sl.tone, syntax.Plain, false))
	ln.free = rest
}
