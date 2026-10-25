package view_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/view"
)

func TestEstimate(t *testing.T) {
	counts := change.LineCounts{Added: 7, Deleted: 3, Known: true}
	tests := []struct {
		name    string
		counts  change.LineCounts
		mode    view.Mode
		context int
		want    int
	}{
		{"full", counts, view.Full, 3, 10},
		{"hunks add a context above and below", counts, view.Hunks, 3, 16},
		{"a negative context adds none", counts, view.Hunks, -1, 10},
		{"nothing counted gives one line", change.LineCounts{}, view.Hunks, 3, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, view.Estimate(tt.counts, tt.mode, tt.context))
		})
	}
}

// threeFiles is a stream of a laid out file of five rows, a file of four
// estimated lines and a laid out file of three rows: lines 0 to 5, 6 to 10
// and 11 to 14.
func threeFiles() *view.Stream {
	s := view.NewStream([]int{0, 4, 0})
	s.Lay(0, view.Lines(fileDiff(5), view.Full, 0))
	s.Lay(2, view.Lines(fileDiff(3), view.Full, 0))
	return s
}

func TestStreamAt(t *testing.T) {
	s := threeFiles()

	require.Equal(t, 15, s.Len())
	assert.Equal(t, []int{0, 6, 11}, []int{s.Start(0), s.Start(1), s.Start(2)})
	assert.Equal(t, []int{6, 11, 15}, []int{s.End(0), s.End(1), s.End(2)})
	tests := []struct {
		line int
		want view.StreamLine
	}{
		{0, view.StreamLine{File: 0}},
		{1, view.StreamLine{File: 0, Spot: view.OnRow, Line: 0}},
		{6, view.StreamLine{File: 1}},
		{8, view.StreamLine{File: 1, Spot: view.OnGuess, Line: 1}},
		{11, view.StreamLine{File: 2}},
		{14, view.StreamLine{File: 2, Spot: view.OnRow, Line: 2}},
		{-3, view.StreamLine{File: 0}},
		{99, view.StreamLine{File: 2, Spot: view.OnRow, Line: 2}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, s.At(tt.line), "line %d", tt.line)
	}
}

func TestAGuessBecomesARowOnceTheFileIsLaidOut(t *testing.T) {
	s := threeFiles()
	pos := s.Navigate(view.StreamPos{}, 9, 1)
	require.Equal(t, view.StreamPos{File: 1, Spot: view.OnGuess, AnchorRow: 2, Scroll: 9}, pos)

	// The hunk view of 100 rows with one change at row 50 shows rows 47 to
	// 53 below a fold; the third line of the estimate is row 48.
	s.Lay(1, view.Lines(fileDiff(100, block(50, 51)), view.Hunks, 3))
	pos = s.Switch(pos, 1)

	assert.Equal(t, view.StreamPos{File: 1, Spot: view.OnRow, AnchorRow: 48, Scroll: 9}, pos)
}

func TestAGuessPastTheLinesOfItsFileStandsOnTheLastOne(t *testing.T) {
	s := threeFiles()
	pos := s.Navigate(view.StreamPos{}, 10, 1)

	s.Lay(1, view.Lines(fileDiff(2), view.Full, 0))
	pos = s.Switch(pos, 1)

	assert.Equal(t, view.StreamPos{File: 1, Spot: view.OnRow, AnchorRow: 1, Scroll: 8}, pos)
}

func TestAFileWithoutLinesShowsItsHeader(t *testing.T) {
	s := threeFiles()
	pos := s.Navigate(view.StreamPos{}, 9, 1)

	s.Lay(1, nil)
	pos = s.Switch(pos, 1)

	assert.Equal(t, 6, pos.Scroll)
	assert.Equal(t, view.StreamLine{File: 1}, s.At(pos.Scroll))
	assert.Equal(t, view.StreamLine{File: 2}, s.At(7))
}

func TestForgettingAFileKeepsTheStreamsLength(t *testing.T) {
	s := threeFiles()

	s.Forget(0)

	assert.Equal(t, 15, s.Len())
	_, laid := s.Lines(0)
	assert.False(t, laid)
	assert.Equal(t, view.StreamLine{File: 0, Spot: view.OnGuess, Line: 4}, s.At(5))
}

func TestARowOfAFileNotLaidOutStandsOnItsGuess(t *testing.T) {
	s := threeFiles()

	pos := s.Switch(view.StreamPos{File: 1, Spot: view.OnRow, AnchorRow: 2}, 1)

	assert.Equal(t, 9, pos.Scroll)
	assert.Equal(t, view.OnRow, pos.Spot, "a row stays a row until its file is laid out")
}

func TestStreamSwitchClamps(t *testing.T) {
	s := threeFiles()

	assert.Equal(t, 10, s.Switch(view.StreamPos{File: 7, Spot: view.OnRow, AnchorRow: 1}, 5).Scroll,
		"a file past the end gives the last one, and the last page stays full")
}

func TestAFoldAtTheTopOfTheStreamKeepsAnAnchorInside(t *testing.T) {
	s := view.NewStream([]int{0})
	s.Lay(0, view.Lines(fileDiff(100, block(50, 51)), view.Hunks, 3))
	pos := view.StreamPos{Spot: view.OnRow, AnchorRow: 20, Scroll: 2}

	assert.Equal(t, view.StreamPos{Spot: view.OnRow, AnchorRow: 20, Scroll: 1}, s.Navigate(pos, 1, 1))

	pos = s.Navigate(pos, 0, 1)
	pos = s.Navigate(pos, 1, 1)
	assert.Equal(t, view.StreamPos{Spot: view.OnRow, AnchorRow: 0, Scroll: 1}, pos,
		"the header took the anchor, so the fold hands it its first row")
}

func TestStreamPosForAndBack(t *testing.T) {
	assert.Equal(t, view.StreamPos{File: 2}, view.StreamPosFor(2, view.Pos{AnchorRow: 0}),
		"the top of a file shows its header")
	pos := view.Pos{AnchorRow: 40, AnchorHalf: view.NewHalf, Scroll: 12}
	sp := view.StreamPosFor(2, pos)
	assert.Equal(t, view.StreamPos{File: 2, Spot: view.OnRow, AnchorRow: 40, AnchorHalf: view.NewHalf}, sp)
	assert.Equal(t, view.Pos{AnchorRow: 40, AnchorHalf: view.NewHalf}, sp.FilePos())
	assert.Equal(t, view.Pos{}, view.StreamPos{File: 2, AnchorRow: 9}.FilePos(), "a header stands for the first row")
}

func TestEmptyStream(t *testing.T) {
	var s view.Stream
	pos := view.StreamPos{File: 3, Spot: view.OnRow, AnchorRow: 4, Scroll: 9}

	assert.Equal(t, 0, s.Len())
	assert.Equal(t, view.StreamPos{}, s.Switch(pos, 5))
	assert.Equal(t, view.StreamPos{File: 3, Spot: view.OnRow, AnchorRow: 4}, s.Navigate(pos, 2, 5))
	s.Lay(0, nil)
	s.Forget(0)
	assert.Equal(t, 0, s.Files())
}
