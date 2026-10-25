package view_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/domain/view"
)

func TestUnfold(t *testing.T) {
	d := fileDiff(14, block(6, 8))
	lines := view.Lines(d, view.Hunks, 1)
	require.Equal(t, []string{"f0:5", "5", "6", "7", "8", "f9:14"}, render(lines))

	tests := []struct {
		name  string
		pos   view.Pos
		i     int
		lines []string
		want  view.Pos
	}{
		{"a fold below the top", view.Pos{AnchorRow: 5, Scroll: 1}, 5,
			join([]string{"f0:5", "5", "6", "7", "8"}, rows(9, 14)), view.Pos{AnchorRow: 5, Scroll: 1}},
		{"the fold on top", view.Pos{AnchorRow: 3, Scroll: 0}, 0,
			join(rows(0, 5), []string{"5", "6", "7", "8", "f9:14"}), view.Pos{AnchorRow: 0, Scroll: 0}},
		{"a row", view.Pos{Scroll: 1, AnchorRow: 5}, 2, render(lines), view.Pos{Scroll: 1, AnchorRow: 5}},
		{"a fold above the top", view.Pos{AnchorRow: 7, Scroll: 3}, 0,
			join(rows(0, 5), []string{"5", "6", "7", "8", "f9:14"}), view.Pos{AnchorRow: 7, Scroll: 7}},
		{"a fold below the top that holds the anchor", view.Pos{AnchorRow: 12, Scroll: 1}, 5,
			join([]string{"f0:5", "5", "6", "7", "8"}, rows(9, 14)), view.Pos{AnchorRow: 5, Scroll: 1}},
		{"before the first line", view.Pos{}, -1, render(lines), view.Pos{}},
		{"past the last line", view.Pos{}, len(lines), render(lines), view.Pos{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, pos := view.Unfold(lines, tt.pos, tt.i)
			assert.Equal(t, tt.lines, render(got))
			assert.Equal(t, tt.want, pos)
		})
	}
	assert.Equal(t, []string{"f0:5", "5", "6", "7", "8", "f9:14"}, render(lines), "the lines given stay as they were")
}

// TestUnfoldingAtTheEndKeepsTheView: at the end of the file the clamped
// hunk view holds the anchor in the last fold below its top line; once that
// fold is open, switching the view in place must not jump.
func TestUnfoldingAtTheEndKeepsTheView(t *testing.T) {
	full, hunks := anchorViews()
	pos := view.Switch(hunks, view.Navigate(full, view.Pos{}, 1000, height), height)
	last := len(hunks) - 1
	require.True(t, hunks[last].Fold)
	require.True(t, hunks[last].Contains(pos.AnchorRow))

	lines, after := view.Unfold(hunks, pos, last)

	assert.Equal(t, after, view.Switch(lines, after, height))
}

func TestUnfoldingKeepsTheLinesAboveAndCoversEveryRow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomDiff(t)
		lines := view.Lines(d, view.Hunks, rapid.IntRange(0, 3).Draw(t, "context"))
		if len(lines) == 0 {
			return
		}
		i := rapid.IntRange(0, len(lines)-1).Draw(t, "line")
		// Any anchor, as Switch leaves one the clamped scroll does not show.
		pos := view.Pos{
			AnchorRow: rapid.IntRange(0, len(d.Rows)-1).Draw(t, "anchor"),
			Scroll:    rapid.IntRange(0, len(lines)-1).Draw(t, "scroll"),
		}

		got, after := view.Unfold(lines, pos, i)

		require.Equal(t, lines[:i], got[:i], "the lines above")
		require.Equal(t, view.RowAt(lines, pos.Scroll), view.RowAt(got, after.Scroll), "the top line keeps its row")
		if lines[i].Fold && lines[i].Contains(pos.AnchorRow) {
			require.Equal(t, view.RowAt(got, after.Scroll), after.AnchorRow, "the fold hands its anchor to the top line")
		} else {
			require.Equal(t, pos.AnchorRow, after.AnchorRow)
		}
		next := 0
		for j, l := range got {
			require.Equal(t, next, l.Rows.Start, "line %d", j)
			next = l.Rows.End
		}
		require.Equal(t, len(d.Rows), next)
		if lines[i].Fold {
			require.Equal(t, len(lines)-1+lines[i].Rows.End-lines[i].Rows.Start, len(got))
			for r := lines[i].Rows.Start; r < lines[i].Rows.End; r++ {
				require.False(t, got[view.LineForRow(got, r)].Fold, "row %d shows", r)
			}
		}
	})
}
