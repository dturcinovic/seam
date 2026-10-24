package view

import "seam/internal/domain/diff"

// Half is which side of its row a line shows.
type Half int

// The halves of a row. A row of the side-by-side view, and an unchanged row
// of the unified one, shows Whole.
const (
	Whole Half = iota
	OldHalf
	NewHalf
)

// Unify lays out lines as the unified view does, as git does: each changed
// block shows the old halves of its rows, then the new ones, so a Change
// row shows twice. Unchanged rows and folds stay as they are.
func Unify(d diff.FileDiff, lines []Line) []Line {
	out := make([]Line, 0, len(lines))
	for i := 0; i < len(lines); {
		if !changed(d, lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && changed(d, lines[j]) {
			j++
		}
		out = halves(out, d, lines[i:j])
		i = j
	}
	return out
}

// changed reports whether l shows a row of a changed block. The views
// never fold one, so l shows a single row.
func changed(d diff.FileDiff, l Line) bool {
	return !l.Fold && d.Rows[l.Rows.Start].Kind != diff.Equal
}

// halves appends the old halves of a block's rows, then the new ones. A
// block pairs its rows first, so the rows only its new halves show lie past
// every old one.
func halves(out []Line, d diff.FileDiff, block []Line) []Line {
	reach := 0
	for _, l := range block {
		if d.Rows[l.Rows.Start].Left >= 0 {
			out = append(out, Line{Rows: l.Rows, Half: OldHalf})
			reach = l.Rows.End
		}
	}
	for _, l := range block {
		if d.Rows[l.Rows.Start].Right >= 0 {
			out = append(out, Line{Rows: l.Rows, Half: NewHalf, reach: reach})
		}
	}
	return out
}

// LineFor is the index of the line that shows half h of row; for a row
// that shows whole or is folded, and for a half the row lacks, it is
// LineForRow.
func LineFor(lines []Line, row int, h Half) int {
	i := LineForRow(lines, row)
	if h != NewHalf || i >= len(lines) || lines[i].Half != OldHalf {
		return i
	}
	// The new half, if any, follows the block's old halves.
	for j := i + 1; j < len(lines) && lines[j].Half != Whole; j++ {
		if lines[j].Half == NewHalf && lines[j].Rows.Start == row {
			return j
		}
	}
	return i
}
