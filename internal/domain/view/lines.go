package view

import (
	"sort"

	"github.com/dturcinovic/seam/internal/domain/diff"
)

// Mode says which rows a view shows.
type Mode int

// The modes of a view. The zero value is not set and shows every row, as
// hiding nothing is the safe choice.
const (
	_ Mode = iota
	// Hunks shows the changed blocks with rows of context and folds the rest.
	Hunks
	// Full shows every row.
	Full
)

// Line is one line of a view: a single row, or with Fold a range of
// unchanged rows shown as one line. Half says which side of its row the
// line shows; only Unify splits a row.
type Line struct {
	Rows diff.RowRange
	Fold bool
	Half Half
	// reach is how far the rows of the old halves before a new half reach,
	// see LineForRow.
	reach int
}

// Contains reports whether the line shows or folds row.
func (l Line) Contains(row int) bool {
	return l.Rows.Start <= row && row < l.Rows.End
}

// Lines lays out d in mode m, in Hunks with context rows around each change.
// The lines cover every row once and in order.
func Lines(d diff.FileDiff, m Mode, context int) []Line {
	n := len(d.Rows)
	if m != Hunks {
		return rowLines(make([]Line, 0, n), 0, n)
	}
	context = max(context, 0)
	var lines []Line
	next := 0
	for _, c := range d.Changes {
		// Contexts that overlap the previous one merge with it.
		start, end := max(c.Start-context, next), min(c.End+context, n)
		lines = gap(lines, next, start)
		lines = rowLines(lines, start, end)
		next = end
	}
	return gap(lines, next, n)
}

// gap folds the rows from start to end. A single row stays a row: its fold
// line would take the same room and hide nothing.
func gap(lines []Line, start, end int) []Line {
	if end-start < 2 {
		return rowLines(lines, start, end)
	}
	return append(lines, Line{Rows: diff.RowRange{Start: start, End: end}, Fold: true})
}

func rowLines(lines []Line, start, end int) []Line {
	for r := start; r < end; r++ {
		lines = append(lines, Line{Rows: diff.RowRange{Start: r, End: r + 1}})
	}
	return lines
}

// LineForRow is the index of the first line that shows or folds row. A row
// past the end gives the last line, and no lines give 0. The new halves of
// a unified block repeat rows of its old halves before them, so they count
// as reaching as far as those: that keeps the search's key from falling.
func LineForRow(lines []Line, row int) int {
	i := sort.Search(len(lines), func(i int) bool { return max(lines[i].Rows.End, lines[i].reach) > row })
	return max(min(i, len(lines)-1), 0)
}

// RowAt is the row of line i, the first one for a fold; i must index lines.
func RowAt(lines []Line, i int) int {
	return lines[i].Rows.Start
}
