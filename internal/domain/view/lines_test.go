package view_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// fileDiff is a diff of n rows with the given changed blocks; the views only
// look at the number of rows and the blocks.
func fileDiff(n int, changes ...diff.RowRange) diff.FileDiff {
	rows := make([]diff.Row, n)
	for i := range rows {
		rows[i] = diff.Row{Kind: diff.Equal, Left: i, Right: i}
	}
	for _, c := range changes {
		for i := c.Start; i < c.End; i++ {
			rows[i].Kind = diff.Change
		}
	}
	return diff.FileDiff{Rows: rows, Changes: changes}
}

func block(start, end int) diff.RowRange {
	return diff.RowRange{Start: start, End: end}
}

// render shows lines compactly: "7" a row, "f0:7" a fold of rows 0 to 6.
func render(lines []view.Line) []string {
	var out []string
	for _, l := range lines {
		if l.Fold {
			out = append(out, fmt.Sprintf("f%d:%d", l.Rows.Start, l.Rows.End))
		} else {
			out = append(out, fmt.Sprint(l.Rows.Start))
		}
	}
	return out
}

func rows(start, end int) []string {
	var out []string
	for r := start; r < end; r++ {
		out = append(out, fmt.Sprint(r))
	}
	return out
}

func join(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestLines(t *testing.T) {
	tests := []struct {
		name    string
		d       diff.FileDiff
		mode    view.Mode
		context int
		want    []string
	}{
		{"full shows every row", fileDiff(3, block(1, 2)), view.Full, 3, rows(0, 3)},
		{"unset mode shows every row", fileDiff(3, block(1, 2)), 0, 3, rows(0, 3)},
		{"empty file", fileDiff(0), view.Hunks, 3, nil},
		{"no change is one fold", fileDiff(10), view.Hunks, 3, []string{"f0:10"}},
		{"no change in one row", fileDiff(1), view.Hunks, 3, []string{"0"}},
		{"folds before and after", fileDiff(20, block(10, 11)), view.Hunks, 3,
			join([]string{"f0:7"}, rows(7, 14), []string{"f14:20"})},
		{"change at the start", fileDiff(10, block(0, 1)), view.Hunks, 2,
			join(rows(0, 3), []string{"f3:10"})},
		{"change at the end", fileDiff(10, block(9, 10)), view.Hunks, 2,
			join([]string{"f0:7"}, rows(7, 10))},
		{"overlapping contexts merge", fileDiff(30, block(5, 6), block(10, 11)), view.Hunks, 3,
			join([]string{"f0:2"}, rows(2, 14), []string{"f14:30"})},
		{"a gap of one row is no fold", fileDiff(20, block(5, 6), block(13, 14)), view.Hunks, 3,
			join([]string{"f0:2"}, rows(2, 17), []string{"f17:20"})},
		{"a gap of two rows folds", fileDiff(20, block(5, 6), block(14, 15)), view.Hunks, 3,
			join([]string{"f0:2"}, rows(2, 9), []string{"f9:11"}, rows(11, 18), []string{"f18:20"})},
		{"a fold of two rows at the start", fileDiff(10, block(5, 10)), view.Hunks, 3,
			join([]string{"f0:2"}, rows(2, 10))},
		{"negative context is none", fileDiff(5, block(2, 3)), view.Hunks, -1,
			[]string{"f0:2", "2", "f3:5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, render(view.Lines(tt.d, tt.mode, tt.context)))
		})
	}
}

func TestLineForRow(t *testing.T) {
	// f0:7, rows 7 to 13, f14:20
	lines := view.Lines(fileDiff(20, block(10, 11)), view.Hunks, 3)

	assert.Equal(t, 0, view.LineForRow(lines, 0))
	assert.Equal(t, 0, view.LineForRow(lines, 6), "a row inside a fold")
	assert.Equal(t, 1, view.LineForRow(lines, 7))
	assert.Equal(t, 7, view.LineForRow(lines, 13))
	assert.Equal(t, 8, view.LineForRow(lines, 14))
	assert.Equal(t, 8, view.LineForRow(lines, 19), "the last row inside the last fold")
	assert.Equal(t, 8, view.LineForRow(lines, 25), "past the end")
	assert.Equal(t, 0, view.LineForRow(nil, 3), "no lines")
}

func TestRowAt(t *testing.T) {
	lines := view.Lines(fileDiff(20, block(10, 11)), view.Hunks, 3)

	assert.Equal(t, 0, view.RowAt(lines, 0), "a fold gives its first row")
	assert.Equal(t, 7, view.RowAt(lines, 1))
	assert.Equal(t, 14, view.RowAt(lines, 8))
}
