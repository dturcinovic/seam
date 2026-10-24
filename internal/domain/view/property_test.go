package view_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"seam/internal/domain/diff"
	"seam/internal/domain/view"
)

// randomDiff draws up to 120 rows, each changed with some chance, so that
// blocks, gaps and contexts of every length occur.
func randomDiff(t *rapid.T) diff.FileDiff {
	odds := rapid.Map(rapid.IntRange(0, 9), func(n int) bool { return n == 0 })
	changed := rapid.SliceOfN(odds, 0, 120).Draw(t, "changed")
	var changes []diff.RowRange
	for i := 0; i < len(changed); i++ {
		if !changed[i] {
			continue
		}
		start := i
		for i < len(changed) && changed[i] {
			i++
		}
		changes = append(changes, diff.RowRange{Start: start, End: i})
	}
	return fileDiff(len(changed), changes...)
}

// near reports whether row lies within context rows of a change.
func near(d diff.FileDiff, context, row int) bool {
	for _, c := range d.Changes {
		if row >= c.Start-context && row < c.End+context {
			return true
		}
	}
	return false
}

func TestLinesCoverEveryRowOnce(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomDiff(t)
		mode := rapid.SampledFrom([]view.Mode{view.Hunks, view.Full}).Draw(t, "mode")
		context := rapid.IntRange(0, 5).Draw(t, "context")

		lines := view.Lines(d, mode, context)

		next := 0
		for i, l := range lines {
			require.Equal(t, next, l.Rows.Start, "line %d starts where the one before ended", i)
			require.Less(t, l.Rows.Start, l.Rows.End, "line %d holds a row", i)
			next = l.Rows.End
		}
		require.Equal(t, len(d.Rows), next)
		for row := range d.Rows {
			require.True(t, lines[view.LineForRow(lines, row)].Contains(row), "row %d", row)
		}
	})
}

func TestHunksFoldExactlyTheRowsFarFromAChange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomDiff(t)
		context := rapid.IntRange(0, 5).Draw(t, "context")

		lines := view.Lines(d, view.Hunks, context)

		for i, l := range lines {
			if l.Fold {
				require.GreaterOrEqual(t, l.Rows.End-l.Rows.Start, 2, "fold %d hides at least two rows", i)
				for row := l.Rows.Start; row < l.Rows.End; row++ {
					require.False(t, near(d, context, row), "fold %d hides row %d next to a change", i, row)
				}
				require.False(t, i > 0 && lines[i-1].Fold, "folds %d and %d are not merged", i-1, i)
				continue
			}
			row := l.Rows.Start
			if !near(d, context, row) {
				// Shown although far from a change: only as a gap of one row.
				require.True(t, row == 0 || near(d, context, row-1), "row %d", row)
				require.True(t, row == len(d.Rows)-1 || near(d, context, row+1), "row %d", row)
			}
		}
	})
}

func TestFullHidesNothing(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomDiff(t)

		for _, l := range view.Lines(d, view.Full, 3) {
			require.False(t, l.Fold)
		}
	})
}

func TestTheAnchorSurvivesAnyDetour(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomDiff(t)
		context := rapid.IntRange(0, 5).Draw(t, "context")
		height := rapid.IntRange(1, 30).Draw(t, "height")
		full, hunks := view.Lines(d, view.Full, 0), view.Lines(d, view.Hunks, context)

		pos := view.Navigate(full, view.Pos{}, rapid.IntRange(-5, 150).Draw(t, "scroll"), height)
		top := pos.Scroll

		// Any number of switches without navigation brings the top row back.
		for range rapid.IntRange(1, 3).Draw(t, "round trips") {
			pos = view.Switch(hunks, pos, height)
			pos = view.Switch(full, pos, height)
		}
		require.Equal(t, top, pos.Scroll)

		// Navigation in the hunk view anchors at its top row, unless nothing
		// moved; the full view then shows the anchor on top unless the last
		// page holds it.
		pos = view.Switch(hunks, pos, height)
		before := pos
		pos = view.Navigate(hunks, pos, rapid.IntRange(-5, 150).Draw(t, "hunk scroll"), height)
		if pos.Scroll == before.Scroll {
			require.Equal(t, before.AnchorRow, pos.AnchorRow, "nothing moved")
		} else {
			require.True(t, hunks[pos.Scroll].Contains(pos.AnchorRow))
		}
		pos = view.Switch(full, pos, height)
		require.Equal(t, max(min(pos.AnchorRow, len(full)-height), 0), pos.Scroll)
	})
}
