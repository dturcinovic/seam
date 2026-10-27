package view_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/domain/view"
)

// noted shows lines as render does, a note line as "n" and its Ref.
func noted(lines []view.Line) []string {
	out := render(lines)
	for i, l := range lines {
		if ref, ok := l.Note(); ok {
			out[i] = fmt.Sprintf("n%d", ref)
		}
	}
	return out
}

func TestANoteLineStandsBelowTheLineOfItsRow(t *testing.T) {
	d := fileDiff(12, block(5, 6))
	lines := view.Lines(d, view.Hunks, 1)
	require.Equal(t, []string{"f0:4", "4", "5", "6", "f7:12"}, render(lines))

	got := view.Annotate(lines, []view.NoteAt{
		{Row: 5, Ref: 0}, {Row: 5, Ref: 1}, {Row: 2, Ref: 2}, {Row: 6, Ref: 3},
	})

	assert.Equal(t, []string{"f0:4", "n2", "4", "5", "n0", "n1", "6", "n3", "f7:12"}, noted(got),
		"in the order given, and below the fold that hides its row")
	assert.Equal(t, 2, view.NotesBelow(got, 3))
	assert.Equal(t, render(lines), noted(view.Annotate(got, nil)), "no notes take the note lines away")
}

func TestANoteLineFollowsTheHalfOfItsSide(t *testing.T) {
	d := fileDiff(4, block(1, 3))
	lines := view.Unify(d, view.Lines(d, view.Full, 0))

	got := view.Annotate(lines, []view.NoteAt{{Row: 1, Half: view.OldHalf, Ref: 0}, {Row: 1, Half: view.NewHalf, Ref: 1}})

	assert.Equal(t, []string{"0", "1", "n0", "2", "1", "n1", "2", "3"}, noted(got))
	assert.Equal(t, 4, view.LineFor(got, 1, view.NewHalf), "the new half is found past the note line")
	assert.Equal(t, 1, view.LineFor(got, 1, view.OldHalf))
}

func TestUnfoldLeavesANoteLineAlone(t *testing.T) {
	d := fileDiff(12, block(5, 6))
	lines := view.Annotate(view.Lines(d, view.Hunks, 1), []view.NoteAt{{Row: 2}})

	got, _ := view.Unfold(lines, view.Pos{}, 1)

	assert.Equal(t, lines, got)
}

func TestTheTopStaysOnANoteLine(t *testing.T) {
	d := fileDiff(40)
	plain := view.Lines(d, view.Full, 0)
	lines := view.Annotate(plain, []view.NoteAt{{Row: 10}, {Row: 10, Ref: 1}})

	pos := view.Navigate(lines, view.Switch(lines, view.Pos{}, 5), 12, 5)
	require.Equal(t, view.Pos{AnchorRow: 10, AnchorBelow: 2, Scroll: 12}, pos)

	more := view.Annotate(plain, []view.NoteAt{{Row: 3}, {Row: 10}, {Row: 10, Ref: 1}})
	assert.Equal(t, 13, view.Switch(more, pos, 5).Scroll, "a note above the top moves it down with its line")
	assert.Equal(t, 10, view.Switch(plain, pos, 5).Scroll, "without its notes the top is their line")
}

func TestTheStreamKeepsATopNoteLine(t *testing.T) {
	d := fileDiff(40)
	plain := view.Lines(d, view.Full, 0)
	s := view.NewStream([]int{40, 40})
	s.Lay(0, view.Annotate(plain, []view.NoteAt{{Row: 10}}))
	s.Lay(1, plain)

	pos := s.Navigate(view.StreamPos{}, 12, 5)
	require.Equal(t, view.StreamPos{Spot: view.OnRow, AnchorRow: 10, AnchorBelow: 1, Scroll: 12}, pos)
	assert.Equal(t, view.Pos{AnchorRow: 10, AnchorBelow: 1}, pos.FilePos())

	s.Lay(0, view.Annotate(plain, []view.NoteAt{{Row: 0}, {Row: 10}}))
	assert.Equal(t, 13, s.Switch(pos, 5).Scroll)
	s.Lay(0, plain)
	assert.Equal(t, 11, s.Switch(pos, 5).Scroll)
}

func TestNoteLinesNeverHideARow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		d := randomDiff(t)
		lines := view.Lines(d, rapid.SampledFrom([]view.Mode{view.Hunks, view.Full}).Draw(t, "mode"), 2)
		if rapid.Bool().Draw(t, "unified") {
			lines = view.Unify(d, lines)
		}
		var notes []view.NoteAt
		if n := len(d.Rows); n > 0 {
			for i := range rapid.IntRange(0, 6).Draw(t, "notes") {
				half := rapid.SampledFrom([]view.Half{view.Whole, view.OldHalf, view.NewHalf}).Draw(t, "half")
				notes = append(notes, view.NoteAt{Row: rapid.IntRange(0, n-1).Draw(t, "row"), Half: half, Ref: i})
			}
		}

		got := view.Annotate(lines, notes)

		require.Len(t, got, len(lines)+len(notes))
		require.Equal(t, lines, view.Annotate(got, nil))
		for row := range d.Rows {
			for _, h := range []view.Half{view.Whole, view.OldHalf, view.NewHalf} {
				i := view.LineFor(got, row, h)
				_, isNote := got[i].Note()
				require.False(t, isNote, "row %d half %d", row, h)
				require.Equal(t, lines[view.LineFor(lines, row, h)], got[i])
			}
		}
	})
}

func TestAFoldOnTopTakesTheTopFromItsNoteLine(t *testing.T) {
	d := fileDiff(30, block(20, 21))
	lines := view.Annotate(view.Lines(d, view.Hunks, 1), []view.NoteAt{{Row: 5}})
	require.Equal(t, []string{"f0:19", "n0", "19", "20", "21", "f22:30"}, noted(lines))

	pos := view.Navigate(lines, view.Pos{Scroll: 2}, 1, 3)
	require.Equal(t, 1, pos.AnchorBelow)
	pos = view.Navigate(lines, pos, 0, 3)

	assert.Zero(t, pos.AnchorBelow)
	assert.Zero(t, view.Switch(lines, pos, 3).Scroll, "the fold stays on top")

	s := view.NewStream([]int{6})
	s.Lay(0, lines)
	sp := s.Navigate(view.StreamPos{}, 2, 3)
	require.Equal(t, 1, sp.AnchorBelow)
	sp = s.Navigate(sp, 1, 3)
	assert.Zero(t, sp.AnchorBelow)
	assert.Equal(t, 1, s.Switch(sp, 3).Scroll)
}
