package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// bookNote is an open question of the book at line of path in origin o,
// set at age minutes.
func bookNote(id, path string, o note.Origin, line, age int) note.Note {
	return note.Note{
		ID: note.ID(id), Kind: note.Question, State: note.Open, Text: "text of " + id + "\nmore",
		Created: time.Unix(int64(age)*60, 0),
		Anchor:  note.Anchor{Path: path, Origin: o, Start: line, End: line, Lines: []string{"line of " + id}},
	}
}

func worktreeOrigin() note.Origin { return note.Origin{Kind: change.KindWorktree} }

// overviewOf opens the overview of m and hands it the book of notes.
func overviewOf(t *testing.T, m model, f noteFakes, notes ...note.Note) model {
	t.Helper()
	m, _ = typed(m, "N")
	require.True(t, m.s.Notes.Overview.Open)
	return notesCome(m, f, note.Update{Kind: note.Overview, Notes: notes})
}

func overviewFake(m model) *infakes.FakeNoteOverview {
	return m.p.NoteOverview.(*infakes.FakeNoteOverview)
}

// overviewText is the overview's rows as shown.
func overviewText(m model) string {
	return strings.Join(plainLines(&m.s), "\n")
}

func TestNListsTheWholeBookByFile(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m, _ = typed(m, "N")
	assert.Equal(t, 1, overviewFake(m).OverviewCallCount())
	assert.Contains(t, overviewText(m), "reading notes…")

	head := note.Origin{Kind: change.KindCommit, Commit: objectID(t, "1")}
	gone := bookNote("g", "gone.go", worktreeOrigin(), 1, 1)
	done := bookNote("d", "a.go", worktreeOrigin(), 1, 2)
	done.State = note.Resolved
	m = notesCome(m, f, note.Update{Kind: note.Overview, Notes: []note.Note{gone, bookNote("w", "a.go", worktreeOrigin(), 4, 3), done, bookNote("h", "a.go", head, 2, 4)}})

	o := m.s.Notes.Overview
	assert.Equal(t, []overviewRow{{Note: -1, Path: "a.go"}, {Note: 0}, {Note: 1}, {Note: 2}, {Note: -1, Path: "gone.go"}, {Note: 3}}, o.Rows)
	assert.Equal(t, []note.ID{"h", "d", "w", "g"}, []note.ID{o.Notes[0].ID, o.Notes[1].ID, o.Notes[2].ID, o.Notes[3].ID},
		"by path, then source, then age")
	assert.Equal(t, note.ID("h"), o.Sel, "the first note is chosen")
	text := overviewText(m)
	assert.Contains(t, text, "? open     1111111  not placed yet         text of h")
	assert.Contains(t, text, "✓ resolved worktree")
	assert.Contains(t, text, "? open     worktree not in this comparison text of g")

	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{{Note: m.s.Notes.Overview.Notes[2], Place: note.Placement{Where: note.Ambiguous}}}})
	assert.Contains(t, overviewText(m), "worktree ambiguous", "a placement shows at once")
}

func TestAWorktreeNoteOfAFileGoneFromTheDiffOpensItToRead(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = overviewOf(t, m, f, bookNote("g", "gone.go", worktreeOrigin(), 40, 1))
	fl := m.p.Files.(*infakes.FakeFileDiffLoader)
	loads := fl.LoadCallCount()

	m, _ = typed(m, "enter")
	assert.False(t, m.s.Notes.Overview.Open)
	assert.Equal(t, "gone.go", m.s.VisitPath)
	require.Equal(t, loads+1, fl.LoadCallCount())
	r := fl.LoadArgsForCall(loads)
	assert.Equal(t, "gone.go", r.File.NewPath)

	text := numbered(80)
	fl.DrainReturnsOnCall(fl.DrainCallCount(), []filediff.Update{{Gen: r.Gen, Snapshot: filediff.Snapshot{
		File: r.File, Diff: diff.Compute(t.Context(), diff.Side{}, diff.Split(text)),
		RightDoc:     hover.Document{Path: "gone.go", Version: 1, Text: text},
		WorktreeBlob: change.BlobID(change.SHA1, []byte(text)),
	}}})
	m, _ = send(m, fileDiffsReady{})
	k := note.SnapshotKey{Path: "gone.go", Origin: worktreeOrigin(), Blob: change.BlobID(change.SHA1, []byte(text))}
	asked, _ := f.reader.PlaceArgsForCall(f.reader.PlaceCallCount() - 1)
	require.Equal(t, k, asked, "placed against the file as read")
	assert.Equal(t, 0, m.s.Pos.AnchorRow, "not yet at its place")

	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: k, Spots: []note.Spot{spotAt("g", note.Question, note.Exact, 40, 40)}})
	assert.Equal(t, 39-targetContext, m.s.Pos.AnchorRow, "at its place, with what is above it")
	assert.Empty(t, m.s.Notes.Find, "found")

	m, _ = typed(m, "ctrl+o")
	assert.Empty(t, m.s.VisitPath, "ctrl+o goes back")
}

func TestANoteWithoutAPlaceOpensInAPopupOverTheList(t *testing.T) {
	cases := map[string]struct {
		note   note.Note
		placed note.Where
	}{
		"orphaned":  {bookNote("o", "a.go", worktreeOrigin(), 4, 1), note.Orphaned},
		"ambiguous": {bookNote("o", "a.go", worktreeOrigin(), 4, 1), note.Ambiguous},
		"a commit no side shows, though the worktree has its path": {
			bookNote("o", "a.go", note.Origin{Kind: change.KindCommit, Commit: objectID(t, "9")}, 4, 1), 0,
		},
		"an index note without --staged": {bookNote("o", "a.go", note.Origin{Kind: change.KindIndex}, 4, 1), 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			m, f := noteModel(t, headToWorktree(t))
			m = overviewOf(t, m, f, c.note)
			if c.placed != 0 {
				m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{{Note: c.note, Place: note.Placement{Where: c.placed}}}})
			}
			loads := m.p.Files.(*infakes.FakeFileDiffLoader).LoadCallCount()

			m, _ = typed(m, "enter")
			require.True(t, m.s.Hover.Popup.Open)
			assert.Equal(t, "note", m.s.Hover.Popup.Title)
			assert.True(t, m.s.Notes.Overview.Open, "the list stays under the popup")
			assert.Empty(t, m.s.VisitPath)
			assert.Equal(t, loads, m.p.Files.(*infakes.FakeFileDiffLoader).LoadCallCount(), "no file opens")
			text := overviewText(m)
			assert.Contains(t, text, "line of o", "the lines it was set at")
			assert.Contains(t, text, "text of o")

			m, _ = typed(m, "esc")
			assert.False(t, m.s.Hover.Popup.Open)
			assert.True(t, m.s.Notes.Overview.Open, "esc closes the popup first")
		})
	}
}

func TestANotePlacedInTheComparisonIsJumpedTo(t *testing.T) {
	for _, w := range []note.Where{note.Exact, note.Likely, note.Changed} {
		t.Run(w.String(), func(t *testing.T) {
			m, f := noteModel(t, headToWorktree(t))
			n := bookNote("w", "a.go", worktreeOrigin(), 50, 1)
			m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{{Note: n, Place: note.Placement{Where: w, Start: 50, End: 50}}}})
			m = overviewOf(t, m, f, n)

			m, _ = typed(m, "enter")
			assert.False(t, m.s.Notes.Overview.Open)
			assert.Equal(t, 49-targetContext, m.s.Pos.AnchorRow)
			if w == note.Likely {
				assert.Equal(t, "note probably here", m.s.Note)
			}
		})
	}
}

func TestANoteOfAFileNotLoadedWaitsForItsPlace(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = overviewOf(t, m, f, bookNote("b", "b.go", worktreeOrigin(), 1, 1))
	fl := m.p.Files.(*infakes.FakeFileDiffLoader)
	loads := fl.LoadCallCount()

	m, _ = typed(m, "enter")
	require.Equal(t, loads+1, fl.LoadCallCount())
	assert.Equal(t, 1, m.s.Current, "b.go opens")
	assert.Equal(t, noteFind{Note: m.s.Notes.Find.Note, File: 1}, m.s.Notes.Find)
}

func TestXResolvesTheNoteChosen(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	// Notes no side shows, so that enter opens their popup.
	index := note.Origin{Kind: change.KindIndex}
	m = overviewOf(t, m, f, bookNote("a", "a.go", index, 1, 1), bookNote("b", "a.go", index, 2, 2))
	m, _ = typed(m, "j", "x")
	require.Equal(t, 1, f.writer.ResolveCallCount())
	assert.Equal(t, note.ID("b"), f.writer.ResolveArgsForCall(0))
	assert.Equal(t, "note resolved", m.s.Note)

	m, _ = typed(m, "enter", "x")
	assert.Equal(t, 2, f.writer.ResolveCallCount(), "also from the popup of the note")
	assert.False(t, m.s.Hover.Popup.Open)
}

func TestTheListFollowsTheBookAndKeepsItsChoice(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	a, b, c := bookNote("a", "a.go", worktreeOrigin(), 1, 1), bookNote("b", "a.go", worktreeOrigin(), 2, 2), bookNote("c", "a.go", worktreeOrigin(), 3, 3)
	m = overviewOf(t, m, f, a, b, c)
	m, _ = typed(m, "j")
	require.Equal(t, note.ID("b"), m.s.Notes.Overview.Sel)

	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Live: true})
	assert.Equal(t, 2, overviewFake(m).OverviewCallCount(), "a change of the book asks again")
	m = notesCome(m, f, note.Update{Kind: note.Overview, Notes: []note.Note{bookNote("new", "a.go", worktreeOrigin(), 9, 0), a, b, c}})
	assert.Equal(t, note.ID("b"), m.s.Notes.Overview.Sel, "kept by its ID, not its line")

	m = notesCome(m, f, note.Update{Kind: note.Overview, Notes: []note.Note{bookNote("new", "a.go", worktreeOrigin(), 9, 0), a, c}})
	assert.Equal(t, note.ID("c"), m.s.Notes.Overview.Sel, "gone: the note now in its line")
}

func TestTheListTakesTheKeysAndTheMouseRests(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = overviewOf(t, m, f, bookNote("a", "a.go", worktreeOrigin(), 1, 1))
	pos := m.s.Pos
	m, _ = typed(m, "n", "j", "G")
	assert.Equal(t, pos, m.s.Pos, "the view under it stays")
	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 10, Y: 5})
	assert.Equal(t, pos, m.s.Pos)

	m, _ = typed(m, "N")
	assert.False(t, m.s.Notes.Overview.Open, "N closes it")
	m = overviewOf(t, m, f)
	assert.Contains(t, overviewText(m), "no notes")
	m, _ = typed(m, "esc")
	assert.False(t, m.s.Notes.Overview.Open)
	_, cmd := typed(m, "esc")
	assert.NotNil(t, cmd, "then esc quits")
}

func TestTopAndBottomChooseTheFirstAndLastNote(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = overviewOf(t, m, f, bookNote("a", "a.go", worktreeOrigin(), 1, 1), bookNote("z", "z.go", worktreeOrigin(), 1, 2))
	m, _ = typed(m, "G")
	assert.Equal(t, note.ID("z"), m.s.Notes.Overview.Sel)
	m, _ = typed(m, "g")
	assert.Equal(t, note.ID("a"), m.s.Notes.Overview.Sel, "past the heading at the top")
}

func TestANotePopupLiesOverTheListAndGoesWithTheChoice(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	index := note.Origin{Kind: change.KindIndex}
	m = overviewOf(t, m, f, bookNote("a", "a.go", index, 1, 1), bookNote("b", "a.go", index, 2, 2))
	m, _ = typed(m, "enter")
	p := m.s.Hover.Popup
	require.True(t, p.Open)
	o := m.s.Notes.Overview
	line := plainLines(&m.s)[p.Y+1]
	assert.Contains(t, "│└", string([]rune(line)[o.X]), "the list's frame beside the popup")
	assert.Contains(t, "│┘", string([]rune(line)[o.X+o.W-1]))

	m, _ = typed(m, "j")
	assert.False(t, m.s.Hover.Popup.Open, "the popup was of the note chosen before")
}

func TestTheWheelScrollsALongNotePopup(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	long := bookNote("a", "a.go", note.Origin{Kind: change.KindIndex}, 1, 1)
	long.Text = strings.Repeat("line\n", 40)
	m = overviewOf(t, m, f, long)
	m, _ = typed(m, "enter")
	p := m.s.Hover.Popup
	require.Greater(t, len(p.Lines), p.shown())
	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: p.X + 1, Y: p.Y + 1})
	assert.Positive(t, m.s.Hover.Popup.Scroll)
}

func TestANoteWhoseSideTakesNoneOpensInAPopupOnceLoaded(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	n := bookNote("b", "b.go", worktreeOrigin(), 1, 1)
	m = overviewOf(t, m, f, n)
	m, _ = typed(m, "enter")
	fl := m.p.Files.(*infakes.FakeFileDiffLoader)
	r := fl.LoadArgsForCall(fl.LoadCallCount() - 1)
	fl.DrainReturnsOnCall(fl.DrainCallCount(), []filediff.Update{{Gen: r.Gen, Snapshot: filediff.Snapshot{File: r.File}}})
	m, _ = send(m, fileDiffsReady{})

	require.True(t, m.s.Hover.Popup.Open, "an empty file: no placement comes")
	assert.Equal(t, note.ID("b"), m.s.Hover.Popup.Note)
	m, _ = typed(m, "x")
	require.Equal(t, 1, f.writer.ResolveCallCount(), "x in the popup without the list")
	assert.Equal(t, note.ID("b"), f.writer.ResolveArgsForCall(0))
}

func TestXOnAResolvedNoteDoesNothing(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	done := bookNote("d", "a.go", worktreeOrigin(), 1, 1)
	done.State = note.Resolved
	m = overviewOf(t, m, f, done)
	m, _ = typed(m, "enter")
	require.True(t, m.s.Hover.Popup.Open)
	m, _ = typed(m, "x")
	assert.Zero(t, f.writer.ResolveCallCount())
}
