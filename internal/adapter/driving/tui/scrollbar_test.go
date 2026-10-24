package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"seam/internal/domain/diff"
	"seam/internal/domain/view"
)

// rowsOf is a diff whose rows have the given kinds, with a change block for
// each run of rows that are not equal.
func rowsOf(kinds ...diff.RowKind) diff.FileDiff {
	var d diff.FileDiff
	for i, k := range kinds {
		d.Rows = append(d.Rows, diff.Row{Kind: k, Left: i, Right: i})
		switch {
		case k == diff.Equal:
		case len(d.Changes) > 0 && d.Changes[len(d.Changes)-1].End == i:
			d.Changes[len(d.Changes)-1].End++
		default:
			d.Changes = append(d.Changes, diff.RowRange{Start: i, End: i + 1})
		}
	}
	return d
}

func TestChangeMarks(t *testing.T) {
	const (
		e = diff.Equal
		d = diff.Delete
		i = diff.Insert
		c = diff.Change
	)
	tests := []struct {
		name  string
		diff  diff.FileDiff
		body  int
		marks []tone
	}{
		{"no changes", rowsOf(e, e), 4, nil},
		{"a file that fits is marked beside its lines", rowsOf(e, d, d, e, i), 6,
			[]tone{equalTone, deletedTone, deletedTone, equalTone, insertedTone, equalTone}},
		{"a block of deletes and inserts is blue", rowsOf(d, i, e), 3, []tone{changedTone, changedTone, equalTone}},
		{"so is one with a changed row", rowsOf(i, c), 2, []tone{changedTone, changedTone}},
		{"a long file shares the cells", rowsOf(d, e, e, e, i, e, e, e), 2, []tone{deletedTone, insertedTone}},
		{"blocks of two colours in a cell make it blue", rowsOf(d, e, i, e, e, e, e, e), 2, []tone{changedTone, equalTone}},
		{"the last line gets the last cell", rowsOf(e, e, e, e, e, e, e, i), 4, []tone{equalTone, equalTone, equalTone, insertedTone}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := view.Lines(tt.diff, view.Full, 0)

			assert.Equal(t, tt.marks, changeMarks(tt.diff, lines, tt.body))
		})
	}
}

func TestThumb(t *testing.T) {
	tests := []struct {
		name               string
		n, scroll, body    int
		wantStart, wantEnd int
	}{
		{"all lines fit", 10, 0, 10, 0, 0},
		{"at the top", 100, 0, 10, 0, 1},
		{"in the middle", 100, 45, 10, 4, 6},
		{"at the bottom", 100, 90, 10, 9, 10},
		{"half the file", 20, 10, 10, 5, 10},
		{"a huge file keeps one cell", 1 << 20, 1 << 19, 10, 5, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := thumb(tt.n, tt.scroll, tt.body)

			assert.Equal(t, [2]int{tt.wantStart, tt.wantEnd}, [2]int{start, end})
		})
	}
}

func TestTheMarksFollowTheBodysHeight(t *testing.T) {
	s := loaded(t)
	tall := s.Marks

	s.resize(60, s.Layout.Height)
	assert.Equal(t, tall, s.Marks, "a narrower window keeps them")
	s.resize(60, 5+chrome)
	assert.Len(t, s.Marks, 5)
	assert.Equal(t, changeMarks(s.Snapshot.Diff, s.Lines, 5), s.Marks)
}
