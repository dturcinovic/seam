package tui

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// streamFiles are n files f000.go on, each changing rows 5 and 15 of 20,
// counted as git counts them.
func streamFiles(n int) []change.ChangedFile {
	fs := make([]change.ChangedFile, n)
	for i := range fs {
		p := fmt.Sprintf("f%03d.go", i)
		fs[i] = change.ChangedFile{Status: change.Modified, OldPath: p, NewPath: p, Counts: change.LineCounts{Added: 2, Deleted: 2, Known: true}}
	}
	return fs
}

// twentyRows is the diff of every file of streamFiles.
func twentyRows(t *testing.T) diff.FileDiff {
	t.Helper()
	return diff.Compute(t.Context(), diff.Split(numbered(20)), diff.Split(numbered(20, 5, 15)))
}

// streamed is the stream over fs, side by side, on a screen of width by
// height, nothing loaded yet.
func streamed(t *testing.T, fs []change.ChangedFile, width, height int) *state {
	t.Helper()
	set := splitSettings()
	set.View = ui.StreamView
	s := newState(change.Changeset{Files: fs}, set)
	s.resize(width, height)
	require.True(t, s.Stream.On)
	return &s
}

// feed answers what the stream asks for with d, until it asks nothing
// more.
func feed(s *state, d diff.FileDiff) {
	for {
		rs, ok := s.streamWant()
		if !ok || len(rs) == 0 {
			return
		}
		us := make([]filediff.Update, len(rs))
		for i, r := range rs {
			us[i] = filediff.Update{Gen: r.Gen, File: r.File, Snapshot: filediff.Snapshot{File: r.File, Diff: d}}
		}
		s.streamApply(us)
	}
}

// topLine is what the top line of the stream shows.
func topLine(s *state) view.StreamLine {
	return s.Stream.View.At(s.top())
}

func TestTheStreamShowsEveryFileUnderItsHeader(t *testing.T) {
	s := streamed(t, streamFiles(3), 100, 40)
	d := twentyRows(t)

	feed(s, d)

	shown := strings.Join(plainLines(s), "\n")
	for _, p := range []string{"f000.go", "f001.go", "f002.go"} {
		assert.Contains(t, shown, " M "+p+"  +2 -2")
	}
	assert.Less(t, strings.Index(shown, "f000.go  +2"), strings.Index(shown, "f001.go  +2"))
	assert.Contains(t, plainLines(s)[0], "1/3  M f000.go  hunks  stream")
}

func TestTheStreamAsksForTheVisibleFilesFirst(t *testing.T) {
	s := streamed(t, streamFiles(30), 100, 24)
	s.streamScrollTo(s.Stream.View.Start(10))

	rs, ok := s.streamWant()

	require.True(t, ok)
	require.LessOrEqual(t, len(rs), streamWindow)
	assert.Equal(t, "f010.go", rs[0].File.Path(), "the file at the top first")
	paths := make([]string, len(rs))
	for i, r := range rs {
		paths[i] = r.File.Path()
	}
	assert.Contains(t, paths, "f009.go", "and one above")
	_, ok = s.streamWant()
	assert.False(t, ok, "nothing new to ask")
}

func TestScrollingThroughManyFilesHoldsOnlyTheWindow(t *testing.T) {
	s := streamed(t, streamFiles(500), 100, 24)
	d := twentyRows(t)
	var heap runtime.MemStats

	for i := 0; s.top() < s.lineCount()-s.bodyHeight(); i++ {
		feed(s, d)
		require.LessOrEqual(t, len(s.Stream.Held), max(streamWindow, s.bodyHeight()))
		if i == 50 {
			runtime.GC()
			runtime.ReadMemStats(&heap)
		}
		s.scrollBy(s.bodyHeight())
	}
	held := heap.HeapAlloc
	runtime.GC()
	runtime.ReadMemStats(&heap)

	laid := 0
	for f := range s.Stream.View.Files() {
		if _, ok := s.Stream.View.Lines(f); ok {
			laid++
		}
	}
	assert.LessOrEqual(t, laid, max(streamWindow, s.bodyHeight()), "only the window is laid out")
	assert.Less(t, int64(heap.HeapAlloc)-int64(held), int64(1<<20), "the heap does not grow with the files passed")
}

func TestTheStreamKeepsTheTopRowAcrossTheViews(t *testing.T) {
	d := twentyRows(t)
	s := shown(t, streamFiles(3), 100, 12, filediff.Snapshot{Diff: d})
	s.toggleMode()
	s.scrollTo(7)
	before := s.Pos

	_, asked := s.toggleStream()

	assert.False(t, asked, "the file shown is loaded")
	assert.Equal(t, view.StreamLine{File: 0, Spot: view.OnRow, Line: 7}, topLine(s))
	_, asked = s.toggleStream()
	assert.False(t, asked)
	assert.Equal(t, before, s.Pos)
}

func TestLeavingTheStreamOnAFileNotLoadedAsksForIt(t *testing.T) {
	s := streamed(t, streamFiles(30), 100, 12)
	s.streamJump(20)

	r, asked := s.toggleStream()

	require.True(t, asked)
	assert.Equal(t, "f020.go", r.File.Path())
	assert.Equal(t, 20, s.Current)
	assert.False(t, s.Stream.On)
	s.apply(filediff.Update{Gen: r.Gen, File: r.File, Snapshot: filediff.Snapshot{File: r.File, Diff: twentyRows(t)}})
	assert.Equal(t, 0, s.Pos.Scroll, "from the header, the file's top")
}

func TestChangesAndFilesAreFoundAcrossFiles(t *testing.T) {
	s := streamed(t, streamFiles(3), 100, 12)
	d := twentyRows(t)
	feed(s, d)
	lines, _ := s.Stream.View.Lines(1)

	s.streamJump(1)
	assert.Equal(t, view.StreamLine{File: 1}, topLine(s), "tab brings a header to the top")
	s.nextChange()
	assert.Equal(t, s.bodyIndex(1, view.LineForRow(lines, 5)), s.top(), "n: the first change of the file")
	s.nextChange()
	s.nextChange()
	assert.Equal(t, 2, topLine(s).File, "n: on into the next file")
	s.prevChange()
	assert.Equal(t, s.bodyIndex(1, view.LineForRow(lines, 15)), s.top(), "p: back to the last change of the one before")

	s.step(-1)
	assert.Equal(t, view.StreamLine{File: 1}, topLine(s), "shift+tab: the top file's own header first")
	s.step(-1)
	assert.Equal(t, view.StreamLine{File: 0}, topLine(s), "then the one before")
	s.step(1)
	assert.Equal(t, view.StreamLine{File: 1}, topLine(s))
}

func TestAFileNotLoadedCountsAsAChangeAtItsHeader(t *testing.T) {
	s := streamed(t, streamFiles(30), 100, 12)

	s.nextChange()

	assert.Equal(t, view.StreamLine{File: 1}, topLine(s))
}

func TestTheFileListJumpsToTheHeader(t *testing.T) {
	s := streamed(t, streamFiles(30), 140, 12)
	require.Positive(t, s.Layout.List)
	s.toggleList()
	s.moveCursor(4)

	_, asked := s.openCursor()

	assert.False(t, asked, "no file is opened")
	assert.Equal(t, view.StreamLine{File: 4}, topLine(s))
	assert.Equal(t, 4, s.Current, "the list marks the file at the top")
}

func TestAFileAboveThatLoadsKeepsTheTopLine(t *testing.T) {
	s := streamed(t, streamFiles(30), 100, 6)
	// The full view of 20 rows: far more than the estimate of 2+2.
	s.toggleMode()
	s.streamScrollTo(s.Stream.View.Start(3) + 2)
	top := topLine(s)
	f1 := s.Files[1]
	s.streamApply([]filediff.Update{{Gen: s.Stream.Gen, File: f1, Snapshot: filediff.Snapshot{File: f1, Diff: twentyRows(t)}}})

	assert.Equal(t, top, topLine(s))
}

func TestAReloadKeepsTheTopFileByItsPath(t *testing.T) {
	s := streamed(t, streamFiles(3), 100, 12)
	feed(s, twentyRows(t))
	s.streamJump(1)
	s.scrollBy(2)
	top := topLine(s)
	fs := append([]change.ChangedFile{{Status: change.Added, NewPath: "a.go"}}, streamFiles(3)...)

	s.reloaded(change.Update{Gen: s.reload(), Changeset: change.Changeset{Files: fs}})

	assert.Equal(t, "f001.go", s.Files[topLine(s).File].Path())
	assert.Equal(t, top.Line, topLine(s).Line, "with its row, shown as it was")
	rs, ok := s.streamWant()
	require.True(t, ok)
	fresh := 0
	for _, r := range rs {
		if r.Fresh {
			fresh++
		}
	}
	assert.Positive(t, fresh, "the files shown are asked for afresh")
}

func TestALateAnswerOfAnotherGenerationIsDropped(t *testing.T) {
	s := streamed(t, streamFiles(3), 100, 12)
	rs, _ := s.streamWant()

	s.reloaded(change.Update{Gen: s.reload(), Changeset: change.Changeset{Files: streamFiles(3)}})
	files := s.streamApply([]filediff.Update{{Gen: rs[0].Gen, File: rs[0].File, Snapshot: filediff.Snapshot{File: rs[0].File, Diff: twentyRows(t)}}})

	assert.Empty(t, files)
	assert.Empty(t, s.Stream.Held)
}

func TestFoldingInTheStream(t *testing.T) {
	s := streamed(t, streamFiles(2), 100, 30)
	feed(s, twentyRows(t))
	before := s.lineCount()
	fold := -1
	for i := range s.lineCount() {
		if _, l, ok := s.rowLine(i); ok && l.Fold {
			fold = i
			break
		}
	}
	require.GreaterOrEqual(t, fold, 0)

	s.streamUnfold(fold)

	assert.Greater(t, s.lineCount(), before)
	_, l, _ := s.rowLine(fold)
	assert.False(t, l.Fold)
}

func TestLeavingOnAStaleFileShowsItAndAsksAfresh(t *testing.T) {
	s := streamed(t, streamFiles(3), 100, 12)
	feed(s, twentyRows(t))
	s.reloaded(change.Update{Gen: s.reload(), Changeset: change.Changeset{Files: streamFiles(3)}})

	r, asked := s.toggleStream()

	require.True(t, asked)
	assert.True(t, r.Fresh)
	assert.True(t, s.Loaded, "shown as it was meanwhile")
	assert.NotEqual(t, s.LoadGen, s.ShownGen, "and still waiting")
}

func TestEnteringWhileAReloadIsOutAsksAfresh(t *testing.T) {
	s := shown(t, streamFiles(3), 100, 12, filediff.Snapshot{Diff: twentyRows(t)})
	_, ok := s.open(0, true)
	require.True(t, ok)

	s.toggleStream()
	rs, _ := s.streamWant()

	require.NotEmpty(t, rs)
	assert.Equal(t, "f000.go", rs[0].File.Path())
	assert.True(t, rs[0].Fresh)
}

func TestAFailedFileSaysSoInItsHeader(t *testing.T) {
	s := streamed(t, streamFiles(3), 100, 30)
	rs, _ := s.streamWant()

	s.streamApply([]filediff.Update{{Gen: rs[1].Gen, File: rs[1].File, Err: errors.New("boom")}})

	assert.NoError(t, s.Err, "the footer's error is not one file's")
	assert.Contains(t, strings.Join(plainLines(s), "\n"), rs[1].File.Path()+"  +2 -2  failed: boom")
	again, _ := s.streamWant()
	for _, r := range again {
		assert.NotEqual(t, rs[1].File, r.File, "not asked again and again")
	}
}

func TestAReloadKeepsTheCaretWithItsFile(t *testing.T) {
	d := twentyRows(t)
	s := shown(t, files("b.go"), 100, 12, worktreeDoc(filediff.Snapshot{Diff: d}))
	s.startCaret()
	for range 4 {
		s.caretUpDown(1)
	}
	require.True(t, s.Caret.On)
	row := s.Caret.Row
	require.Positive(t, row)
	fs := append(files("a.go"), files("b.go")...)

	r, _ := s.reloaded(change.Update{Gen: s.reload(), Changeset: change.Changeset{Files: fs}})
	s.apply(filediff.Update{Gen: r.Gen, Snapshot: worktreeDoc(filediff.Snapshot{File: r.File, Diff: d})})
	s.keepCaret()

	assert.Equal(t, 1, s.Caret.File)
	assert.Equal(t, row, s.Caret.Row)
}
