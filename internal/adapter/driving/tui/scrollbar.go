package tui

import (
	"strings"

	"seam/internal/domain/change"
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
	m := newMarks(len(lines), body)
	m.blocks(d, lines, 0)
	return m.out
}

// streamMarks are changeMarks over every file of the stream: a file laid
// out marks its blocks, one that is not its stand-in lines in the tone
// its counts give.
func streamMarks(s *state, body int) []tone {
	st := s.Stream.View
	if st.Len() == 0 {
		return nil
	}
	m := newMarks(st.Len(), body)
	for f := range st.Files() {
		first, last := st.Start(f)+1, st.End(f)-1
		if lines, laid := st.Lines(f); laid {
			if len(lines) > 0 {
				m.blocks(s.snapOf(f).Diff, lines, first)
			}
			continue
		}
		if t := countsTone(s.Files[f].Counts); t != equalTone && first <= last {
			m.span(first, last, t)
		}
	}
	return m.out
}

// marks are the cells of a scrollbar of body cells beside n lines.
type marks struct {
	out     []tone
	n, body int
}

func newMarks(n, body int) marks {
	return marks{out: make([]tone, body), n: max(n, body), body: body}
}

// blocks marks the change blocks of d, laid out as lines from line start.
func (m marks) blocks(d diff.FileDiff, lines []view.Line, start int) {
	for _, c := range d.Changes {
		// The block ends before the line of the row after it: the unified
		// view shows new halves after its last row's old one.
		last := len(lines) - 1
		if c.End < len(d.Rows) {
			last = view.LineForRow(lines, c.End) - 1
		}
		m.span(start+view.LineForRow(lines, c.Start), start+last, blockTone(d.Rows[c.Start:c.End]))
	}
}

// span marks the cells beside lines first to last in tone t.
func (m marks) span(first, last int, t tone) {
	for i := first * m.body / m.n; i <= last*m.body/m.n; i++ {
		m.out[i] = merge(m.out[i], t)
	}
}

// countsTone is the tone of a file git counted: inserted or deleted if it
// only gains or loses lines, else changed.
func countsTone(c change.LineCounts) tone {
	switch {
	case !c.Known || c.Added == 0 && c.Deleted == 0:
		return equalTone
	case c.Deleted == 0:
		return insertedTone
	case c.Added == 0:
		return deletedTone
	}
	return changedTone
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
	ln := newLine(b, th, s.Cells, s.Layout.scrollbar())
	st := scrollStyle(t, onThumb)
	if t != equalTone {
		ln.text(markGlyph, st)
	}
	ln.end(st)
}
