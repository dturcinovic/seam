package tui

import (
	"strings"

	"seam/internal/domain/diff"
	"seam/internal/domain/view"
)

// markGlyph is the stroke the scrollbar shows beside a change.
const markGlyph = "━"

// changeMarks maps the change blocks of d, laid out as lines, onto body
// cells of the scrollbar, each in the tone of its block; nil when d has no
// changes. Line l of n falls into cell l·body/max(n, body), so a file that
// fits marks the cells beside its lines.
func changeMarks(d diff.FileDiff, lines []view.Line, body int) []tone {
	if len(d.Changes) == 0 || len(lines) == 0 {
		return nil
	}
	out := make([]tone, body)
	n := max(len(lines), body)
	cell := func(row int) int { return view.LineForRow(lines, row) * body / n }
	for _, c := range d.Changes {
		t := blockTone(d.Rows[c.Start:c.End])
		for i := cell(c.Start); i <= cell(c.End-1); i++ {
			out[i] = merge(out[i], t)
		}
	}
	return out
}

// blockTone is deleted for a block that only deletes, inserted for one that
// only inserts and changed for any other.
func blockTone(rows []diff.Row) tone {
	t := equalTone
	for _, r := range rows {
		switch r.Kind {
		case diff.Delete:
			t = merge(t, deletedTone)
		case diff.Insert:
			t = merge(t, insertedTone)
		case diff.Change:
			return changedTone
		case diff.Equal:
		}
	}
	return t
}

// merge is the tone of a cell that a and b share; equalTone marks nothing.
func merge(a, b tone) tone {
	if a == equalTone || a == b {
		return b
	}
	return changedTone
}

// thumb is the cells of the scrollbar beside the part of n lines that the
// screen shows from line scroll on; none when all of them fit.
func thumb(n, scroll, body int) (start, end int) {
	if n <= body {
		return 0, 0
	}
	start = scroll * body / n
	end = ((scroll+body)*body + n - 1) / n
	return start, min(max(end, start+1), body)
}

// writeScrollbar writes one cell of the scrollbar in the columns the panes
// leave: the mark of tone t, if any, on the thumb or not.
func writeScrollbar(b *strings.Builder, s *state, th *theme, t tone, onThumb bool) {
	ln := newLine(b, th, s.Cells, s.Layout.Width-s.Layout.panes())
	st := scrollStyle(t, onThumb)
	if t != equalTone {
		ln.text(markGlyph, st)
	}
	ln.end(st)
}
