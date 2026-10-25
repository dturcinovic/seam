package view_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// layout is how the files of a stream are laid out.
type layout struct {
	mode    view.Mode
	context int
	unified bool
}

func (l layout) lines(d diff.FileDiff) []view.Line {
	lines := view.Lines(d, l.mode, l.context)
	if l.unified {
		lines = view.Unify(d, lines)
	}
	return lines
}

func randomLayout(t *rapid.T, label string) layout {
	return layout{
		mode:    rapid.SampledFrom([]view.Mode{view.Hunks, view.Full}).Draw(t, label+" mode"),
		context: rapid.IntRange(0, 3).Draw(t, label+" context"),
		unified: rapid.Bool().Draw(t, label+" unified"),
	}
}

// randomStream draws up to six files, some laid out in l and the others
// with an estimate, which may be far from their real number of lines.
func randomStream(t *rapid.T, l layout) (*view.Stream, []diff.FileDiff) {
	n := rapid.IntRange(1, 6).Draw(t, "files")
	diffs := make([]diff.FileDiff, n)
	estimates := make([]int, n)
	for i := range n {
		diffs[i] = randomDiff(t)
		estimates[i] = rapid.IntRange(0, 50).Draw(t, "estimate")
	}
	s := view.NewStream(estimates)
	for i, d := range diffs {
		if rapid.Bool().Draw(t, "laid") {
			s.Lay(i, l.lines(d))
		}
	}
	return s, diffs
}

func TestStreamShowsEveryLineOnceUnderItsHeader(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s, _ := randomStream(t, randomLayout(t, "layout"))

		want := view.StreamLine{File: -1}
		for i := range s.Len() {
			got := s.At(i)
			if got.Spot == view.OnHead {
				// The next file starts once the one before showed all it has.
				require.Equal(t, want.File+1, got.File, "line %d", i)
				want = view.StreamLine{File: got.File, Line: -1}
			} else {
				lines, laid := s.Lines(got.File)
				require.Equal(t, want.File, got.File, "line %d", i)
				require.Equal(t, want.Line+1, got.Line, "line %d", i)
				require.Equal(t, laid, got.Spot == view.OnRow, "line %d", i)
				if laid {
					require.Less(t, got.Line, len(lines), "line %d", i)
				}
				want = got
			}
			require.Equal(t, i, s.Index(got), "line %d there and back", i)
		}
		require.Equal(t, s.Files()-1, want.File, "every file has its header")
		for f := range s.Files() {
			if lines, laid := s.Lines(f); laid {
				require.Equal(t, s.Start(f)+len(lines)+1, end(s, f), "file %d shows all its lines", f)
			}
		}
	})
}

// end is the index of the line after file f.
func end(s *view.Stream, f int) int {
	if f+1 < s.Files() {
		return s.Start(f + 1)
	}
	return s.Len()
}

func TestSwitchingBetweenFileAndStreamKeepsTheTopRow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		l := randomLayout(t, "layout")
		s, diffs := randomStream(t, l)
		height := rapid.IntRange(1, 10).Draw(t, "height")
		file := rapid.IntRange(0, s.Files()-1).Draw(t, "file")
		lines := l.lines(diffs[file])
		s.Lay(file, lines)

		pos := view.Navigate(lines, view.Pos{}, rapid.IntRange(0, 150).Draw(t, "scroll"), height)
		sp := s.Switch(view.StreamPosFor(file, pos), height)

		if sp.Scroll < s.Len()-height {
			want := view.StreamLine{File: file, Spot: view.OnRow, Line: pos.Scroll}
			if pos.Scroll == 0 {
				want = view.StreamLine{File: file}
			}
			require.Equal(t, want, s.At(sp.Scroll), "the stream shows the file's top line on top")
		}
		require.Equal(t, pos.Scroll, view.Switch(lines, sp.FilePos(), height).Scroll, "and back")
	})
}

func TestSwitchingTheLayoutOfTheStreamKeepsTheTopLine(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		l, other := randomLayout(t, "layout"), randomLayout(t, "other")
		s, diffs := randomStream(t, l)
		height := rapid.IntRange(1, 10).Draw(t, "height")
		pos := s.Navigate(view.StreamPos{}, rapid.IntRange(0, 400).Draw(t, "scroll"), height)
		top := pos.Scroll

		// Any number of round trips without navigation brings the top line
		// back. Files the first layout left unlaid stay so in both.
		relay := func(l layout) {
			for f, d := range diffs {
				if _, laid := s.Lines(f); laid {
					s.Lay(f, l.lines(d))
				}
			}
			pos = s.Switch(pos, height)
		}
		for range rapid.IntRange(1, 3).Draw(t, "round trips") {
			relay(other)
			relay(l)
		}
		require.Equal(t, top, pos.Scroll)
	})
}

func TestAFileAboveTheTopDoesNotMoveIt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		l := randomLayout(t, "layout")
		s, diffs := randomStream(t, l)
		// A height of one line never clamps: the top line is always there.
		pos := s.Navigate(view.StreamPos{}, rapid.IntRange(0, 400).Draw(t, "scroll"), 1)
		top := s.At(pos.Scroll)
		if pos.File == 0 {
			return
		}

		above := rapid.IntRange(0, pos.File-1).Draw(t, "above")
		switch rapid.IntRange(0, 2).Draw(t, "change") {
		case 0:
			s.Lay(above, l.lines(diffs[above]))
		case 1:
			s.Lay(above, randomLayout(t, "other").lines(diffs[above]))
		case 2:
			s.Forget(above)
		}
		pos = s.Switch(pos, 1)

		require.Equal(t, top, s.At(pos.Scroll))
	})
}
