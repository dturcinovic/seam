package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/ui"
	"seam/internal/domain/view"
)

// numbered is n lines "0".."n-1", with the lines in changed replaced.
func numbered(n int, changed ...int) string {
	var b strings.Builder
	for i := range n {
		if slices.Contains(changed, i) {
			fmt.Fprintf(&b, "changed %d\n", i)
		} else {
			fmt.Fprintf(&b, "%d\n", i)
		}
	}
	return b.String()
}

func files(paths ...string) []change.ChangedFile {
	var fs []change.ChangedFile
	for _, p := range paths {
		fs = append(fs, change.ChangedFile{Status: change.Modified, OldPath: p, NewPath: p})
	}
	return fs
}

// shown is a state of the given size showing snap as the first of fs.
func shown(t *testing.T, fs []change.ChangedFile, width, height int, snap filediff.Snapshot) *state {
	t.Helper()
	s := newState(change.Changeset{Files: fs}, ui.DefaultSettings())
	s.resize(width, height)
	r, ok := s.open(0, false)
	require.True(t, ok)
	snap.File = r.File
	s.apply(filediff.Update{Gen: r.Gen, Snapshot: snap})
	require.True(t, s.Loaded)
	return &s
}

// loaded is a state of 10 lines' height showing a 60-line file whose rows
// 10 and 40 changed, in the hunk view.
func loaded(t *testing.T) *state {
	t.Helper()
	d := diff.Compute(t.Context(), diff.Split(numbered(60)), diff.Split(numbered(60, 10, 40)))
	return shown(t, files("a.go", "b.go", "c.go"), 80, 10+chrome, filediff.Snapshot{Diff: d})
}

func TestOpenAndStep(t *testing.T) {
	s := loaded(t)
	s.scrollBy(3)
	s.shiftH(1)

	r, ok := s.step(1)
	require.True(t, ok)
	assert.Equal(t, "b.go", r.File.Path())
	assert.Equal(t, filediff.Gen(2), r.Gen)
	assert.False(t, s.Loaded, "the new file is not shown before it arrives")
	assert.Equal(t, view.Pos{}, s.Pos, "another file starts at the top")
	assert.Zero(t, s.HScroll)

	s.step(1)
	_, ok = s.step(1)
	assert.False(t, ok, "the last file stays")
	assert.Equal(t, 2, s.Current)
	s.open(0, false)
	_, ok = s.step(-1)
	assert.False(t, ok, "so does the first")
}

func TestOpeningTheSameFileFreshKeepsItsPlace(t *testing.T) {
	s := loaded(t)
	s.scrollBy(3)
	pos := s.Pos

	r, ok := s.open(0, true)

	require.True(t, ok)
	assert.True(t, r.Fresh)
	assert.True(t, s.Loaded, "still shown until the new one arrives")
	assert.Equal(t, pos, s.Pos)
}

func TestApply(t *testing.T) {
	s := loaded(t)
	r, _ := s.step(1)
	assert.Empty(t, s.Snapshot.Diff.Rows, "the file left is not shown while the next one loads")
	boom := errors.New("boom")

	s.apply(filediff.Update{Gen: r.Gen - 1, Snapshot: filediff.Snapshot{File: files("late.go")[0]}})
	assert.False(t, s.Loaded, "a late answer is dropped")
	assert.Empty(t, s.Snapshot.File.Path())

	s.apply(filediff.Update{Gen: r.Gen, Err: boom})
	assert.Equal(t, boom, s.Err)
	assert.False(t, s.Loaded)

	s.apply(filediff.Update{Gen: r.Gen, Snapshot: filediff.Snapshot{File: r.File}})
	assert.True(t, s.Loaded)
	assert.NoError(t, s.Err, "a success clears the error")
}

func TestScrollStaysWithinTheFile(t *testing.T) {
	s := loaded(t)
	last := len(s.Lines) - s.bodyHeight()
	require.Positive(t, last)

	s.scrollBy(-5)
	assert.Zero(t, s.Pos.Scroll)
	s.scrollTo(len(s.Lines))
	assert.Equal(t, last, s.Pos.Scroll)
	s.scrollBy(5)
	assert.Equal(t, last, s.Pos.Scroll)
	s.scrollTo(0)
	s.scrollBy(pageLines(s.bodyHeight(), true))
	assert.Equal(t, 5, s.Pos.Scroll)
	s.scrollBy(-pageLines(s.bodyHeight(), false))
	assert.Zero(t, s.Pos.Scroll)
}

func TestNextAndPrevChangeCrossFolds(t *testing.T) {
	s := loaded(t)
	s.toggleMode()
	require.Equal(t, view.Full, s.Mode)
	s.resize(80, 5+chrome)

	s.nextChange()
	assert.Equal(t, 10, s.Pos.Scroll, "the first change")
	s.nextChange()
	assert.Equal(t, 40, s.Pos.Scroll, "the second")
	s.nextChange()
	assert.Equal(t, 40, s.Pos.Scroll, "no third")
	s.prevChange()
	assert.Equal(t, 10, s.Pos.Scroll)

	s.toggleMode()
	require.Equal(t, view.Hunks, s.Mode)
	s.scrollTo(0)
	s.nextChange()
	assert.Equal(t, view.LineForRow(s.Lines, 10), s.Pos.Scroll)
	s.nextChange()
	assert.Equal(t, view.LineForRow(s.Lines, 40), s.Pos.Scroll, "over the fold between them")
}

func TestToggleModeKeepsTheAnchor(t *testing.T) {
	s := loaded(t)
	s.toggleMode()
	s.scrollBy(25)
	top := view.RowAt(s.Lines, s.Pos.Scroll)

	s.toggleMode()
	s.toggleMode()

	assert.Equal(t, top, view.RowAt(s.Lines, s.Pos.Scroll), "a row the hunk view folds is kept")
}

func TestReloaded(t *testing.T) {
	t.Run("the shown file stays shown where it moved", func(t *testing.T) {
		s := loaded(t)
		s.scrollBy(2)
		pos := s.Pos
		gen := s.reload()

		r, ok := s.reloaded(change.Update{Gen: gen, Changeset: change.Changeset{Files: files("0.go", "a.go")}})

		require.True(t, ok)
		assert.Equal(t, "a.go", r.File.Path())
		assert.True(t, r.Fresh)
		assert.Equal(t, 1, s.Current)
		assert.Equal(t, pos, s.Pos)
	})
	t.Run("a file gone gives way to the next", func(t *testing.T) {
		s := loaded(t)
		s.step(1)
		gen := s.reload()

		r, ok := s.reloaded(change.Update{Gen: gen, Changeset: change.Changeset{Files: files("a.go", "c.go")}})

		require.True(t, ok)
		assert.Equal(t, "c.go", r.File.Path())
		assert.Equal(t, view.Pos{}, s.Pos)
	})
	t.Run("no files left", func(t *testing.T) {
		s := loaded(t)
		running, _ := s.open(0, true)
		gen := s.reload()

		_, ok := s.reloaded(change.Update{Gen: gen})
		s.apply(filediff.Update{Gen: running.Gen, Snapshot: filediff.Snapshot{File: running.File}})

		assert.False(t, ok)
		assert.False(t, s.Loaded, "a load still running for a file gone is dropped")
		_, has := s.file()
		assert.False(t, has)
	})
	t.Run("late or failed", func(t *testing.T) {
		s := loaded(t)
		gen := s.reload()
		s.reload()

		_, ok := s.reloaded(change.Update{Gen: gen, Changeset: change.Changeset{Files: files("x.go")}})
		assert.False(t, ok, "a late reload is dropped")
		assert.Len(t, s.Files, 3)

		boom := errors.New("boom")
		_, ok = s.reloaded(change.Update{Gen: gen + 1, Err: boom})
		assert.False(t, ok)
		assert.Equal(t, boom, s.Err)
	})
}

func TestShiftH(t *testing.T) {
	s := loaded(t)
	s.shiftH(-1)
	assert.Zero(t, s.HScroll)
	s.shiftH(2)
	assert.Equal(t, 2*hStep, s.HScroll)
}
