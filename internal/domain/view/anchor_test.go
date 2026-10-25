package view_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/view"
)

// height is smaller than the 17 lines of the hunk view, so that it scrolls.
const height = 5

// A diff of 300 rows whose rows 53 to 246 are folded in the hunk view with
// three rows of context: an unchanged block of 200 rows between two changes.
func anchorViews() (full, hunks []view.Line) {
	d := fileDiff(300, block(49, 50), block(250, 251))
	return view.Lines(d, view.Full, 0), view.Lines(d, view.Hunks, 3)
}

func TestSwitchingBackKeepsTheTopRow(t *testing.T) {
	full, hunks := anchorViews()

	pos := view.Navigate(full, view.Pos{}, 120, height)
	assert.Equal(t, view.Pos{AnchorRow: 120, Scroll: 120}, pos, "in the middle of the unchanged block")

	pos = view.Switch(hunks, pos, height)
	assert.True(t, hunks[pos.Scroll].Fold, "the hunk view stands on the fold that hides row 120")
	assert.Equal(t, 120, pos.AnchorRow)

	pos = view.Switch(full, pos, height)
	assert.Equal(t, view.Pos{AnchorRow: 120, Scroll: 120}, pos)
}

func TestSwitchingBackAtTheEndKeepsTheTopRow(t *testing.T) {
	full, hunks := anchorViews()

	pos := view.Navigate(full, view.Pos{}, 1000, height)
	assert.Equal(t, view.Pos{AnchorRow: 295, Scroll: 295}, pos, "clamped to the last page")

	pos = view.Switch(hunks, pos, height)
	assert.Equal(t, len(hunks)-height, pos.Scroll, "the shorter view clamps its scroll")
	assert.Equal(t, 295, pos.AnchorRow, "but not the anchor")

	pos = view.Switch(full, pos, height)
	assert.Equal(t, view.Pos{AnchorRow: 295, Scroll: 295}, pos)
}

func TestNavigatingInTheHunkViewSetsTheAnchor(t *testing.T) {
	full, hunks := anchorViews()
	pos := view.Switch(hunks, view.Navigate(full, view.Pos{}, 120, height), height)

	pos = view.Navigate(hunks, pos, pos.Scroll+2, height)
	row := view.RowAt(hunks, pos.Scroll)
	assert.Equal(t, row, pos.AnchorRow)
	assert.Equal(t, 248, row, "the second line after the fold")

	pos = view.Switch(full, pos, height)
	assert.Equal(t, view.Pos{AnchorRow: 248, Scroll: 248}, pos)
}

func TestAFoldAtTheTopKeepsAnAnchorInside(t *testing.T) {
	full, hunks := anchorViews()
	pos := view.Switch(hunks, view.Navigate(full, view.Pos{}, 120, height), height)
	fold := pos.Scroll

	pos = view.Navigate(hunks, pos, fold, height)
	assert.Equal(t, 120, pos.AnchorRow, "resting on the fold")

	pos = view.Navigate(hunks, pos, 0, height)
	pos = view.Navigate(hunks, pos, fold, height)
	assert.Equal(t, 53, pos.AnchorRow, "arriving at the fold from elsewhere gives its first row")
}

func TestNavigatingWithoutMovingKeepsTheAnchor(t *testing.T) {
	full, hunks := anchorViews()
	pos := view.Switch(hunks, view.Navigate(full, view.Pos{}, 1000, height), height)
	require.Equal(t, view.Pos{AnchorRow: 295, Scroll: len(hunks) - height}, pos)

	pos = view.Navigate(hunks, pos, pos.Scroll+1, height)
	assert.Equal(t, 295, pos.AnchorRow, "down at the end moves nothing")

	pos = view.Switch(full, pos, height)
	assert.Equal(t, view.Pos{AnchorRow: 295, Scroll: 295}, pos)
}

func TestNoLines(t *testing.T) {
	assert.Equal(t, view.Pos{AnchorRow: 4}, view.Navigate(nil, view.Pos{AnchorRow: 4, Scroll: 2}, 3, height))
	assert.Equal(t, view.Pos{AnchorRow: 4}, view.Switch(nil, view.Pos{AnchorRow: 4, Scroll: 2}, height))
}
