package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// noteFakes are the note ports of a model, with the updates the next
// notesUpdated drains.
type noteFakes struct {
	writer  *infakes.FakeNoteWriter
	reader  *infakes.FakeNoteReader
	updates *infakes.FakeNoteUpdates
}

func objectID(t *testing.T, c string) change.ObjectID {
	t.Helper()
	id, err := change.ParseObjectID(strings.Repeat(c, 40))
	require.NoError(t, err)
	return id
}

// headToWorktree is HEAD, resolved, against the worktree.
func headToWorktree(t *testing.T) change.Comparison {
	t.Helper()
	head, err := change.Commit("HEAD").Resolved(objectID(t, "1"))
	require.NoError(t, err)
	return change.Comparison{Left: head, Right: change.Worktree()}
}

// noteText is the sides of the notes' file: eighty numbered lines, line 3
// changed.
func noteText() (old, new string) { return numbered(80), numbered(80, 3) }

// noteModel shows a.go of HEAD against the worktree, 80 by 24, with the
// note ports as fakes.
func noteModel(t *testing.T, c change.Comparison) (model, noteFakes) {
	t.Helper()
	return noteModelWith(t, c, splitSettings())
}

// noteModelWith is noteModel with the view's settings set.
func noteModelWith(t *testing.T, c change.Comparison, set ui.Settings) (model, noteFakes) {
	t.Helper()
	fs := files("a.go", "b.go")
	fs[0].OldID, fs[0].NewID = objectID(t, "2"), objectID(t, "3")
	p := fakePorts()
	f := noteFakes{p.NoteWriter.(*infakes.FakeNoteWriter), p.NoteReader.(*infakes.FakeNoteReader), p.NoteUpdates.(*infakes.FakeNoteUpdates)}
	m := newModel(change.Changeset{Comparison: c, Files: fs}, p, set)
	m, _ = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	fl := p.Files.(*infakes.FakeFileDiffLoader)
	r := fl.LoadArgsForCall(0)
	old, new := noteText()
	fl.DrainReturns([]filediff.Update{{Gen: r.Gen, Snapshot: filediff.Snapshot{
		File: r.File, Diff: diff.Compute(t.Context(), diff.Split(old), diff.Split(new)),
		RightDoc:     hover.Document{Path: "a.go", Version: 1, Text: new},
		WorktreeBlob: change.BlobID(change.SHA1, []byte(new)),
	}}})
	m, _ = send(m, fileDiffsReady{})
	m.s.toggleMode()
	return m, f
}

// worktreeSide is the key of a.go's right side in noteModel.
func worktreeSide() note.SnapshotKey {
	_, new := noteText()
	return note.SnapshotKey{Path: "a.go", Origin: note.Origin{Kind: change.KindWorktree}, Blob: change.BlobID(change.SHA1, []byte(new))}
}

// notesCome hands the model us as the notes' news.
func notesCome(m model, f noteFakes, us ...note.Update) model {
	f.updates.DrainReturnsOnCall(f.updates.DrainCallCount(), us)
	m, _ = send(m, notesUpdated{})
	return m
}

// spotAt is a note of kind k placed at the 1-based lines from to to of
// the worktree side.
func spotAt(id note.ID, k note.Kind, w note.Where, from, to int) note.Spot {
	return note.Spot{
		Note:  note.Note{ID: id, Kind: k, Text: "text of " + string(id), State: note.Open},
		Place: note.Placement{Where: w, Start: from, End: to},
	}
}

func TestTheNotesOfBothSidesAreAskedFor(t *testing.T) {
	_, f := noteModel(t, headToWorktree(t))

	require.Equal(t, 2, f.reader.PlaceCallCount())
	keys := map[note.SnapshotKey][]string{}
	for i := range 2 {
		k, lines := f.reader.PlaceArgsForCall(i)
		keys[k] = lines
	}
	head := note.Origin{Kind: change.KindCommit, Commit: objectID(t, "1")}
	assert.Contains(t, keys, note.SnapshotKey{Path: "a.go", Origin: head, Blob: objectID(t, "2")})
	require.Contains(t, keys, worktreeSide())
	assert.Equal(t, "changed 3", keys[worktreeSide()][3], "lines without their ends")
}

func TestEachSourceKeepsItsNotes(t *testing.T) {
	staged := headToWorktree(t)
	staged.Right = change.Index()
	m, f := noteModel(t, staged)

	k, ok := m.s.noteKey(0, filediff.NewSide)
	require.True(t, ok)
	assert.Equal(t, note.SnapshotKey{Path: "a.go", Origin: note.Origin{Kind: change.KindIndex}, Blob: objectID(t, "3")}, k,
		"the index side is the index's blob, not the worktree's")
	assert.Equal(t, 2, f.reader.PlaceCallCount())
}

func TestAFileOnDiskTakesNoNotesOutsideTheWorktree(t *testing.T) {
	m, _ := noteModel(t, change.Comparison{Left: change.File("/tmp/a"), Right: change.File("/tmp/b")})
	_, ok := m.s.noteKey(0, filediff.OldSide)
	assert.False(t, ok, "git difftool's $LOCAL")
	_, ok = m.s.noteKey(0, filediff.NewSide)
	assert.False(t, ok, "not in the worktree")

	m, _ = typed(m, "i", "c")
	assert.False(t, m.s.Field.Open)
	assert.Contains(t, footerLine(m), "no notes on this side")
}

func TestCAtTheCaretWritesANote(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	f.writer.AddReturns(5)
	m, _ = typed(m, "i", "j", "j", "j", "c")
	require.True(t, m.s.Field.Open)
	assert.Equal(t, note.Question, m.s.Notes.Draft.Kind)

	m, _ = typed(m, "w", "h", "y", "tab")
	assert.Equal(t, note.Finding, m.s.Notes.Draft.Kind)
	m, _ = typed(m, "enter")

	require.Equal(t, 1, f.writer.AddCallCount())
	d := f.writer.AddArgsForCall(0)
	assert.Equal(t, note.Finding, d.Kind)
	assert.Equal(t, "why", d.Text)
	assert.Equal(t, "HEAD..worktree", d.Comparison)
	assert.Equal(t, worktreeSide().Origin, d.Anchor.Origin)
	assert.Equal(t, []string{"changed 3"}, d.Anchor.Lines)
	assert.Equal(t, 4, d.Anchor.Start)
	assert.True(t, m.s.Field.Open, "the draft stays until the book took it")
	m, _ = typed(m, "enter")
	assert.Equal(t, 1, f.writer.AddCallCount(), "not twice")

	m = notesCome(m, f, note.Update{Kind: note.Done, Op: m.s.Notes.Draft.Op})
	assert.False(t, m.s.Field.Open)
	assert.Zero(t, m.s.Notes.Draft)
}

func TestANoteNotSavedKeepsItsDraft(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	f.writer.AddReturns(7)
	m, _ = typed(m, "i", "c", "x", "enter")

	m = notesCome(m, f, note.Update{Kind: note.Done, Op: 7, Err: fmt.Errorf("lock notes: %w", errors.New("held"))})

	assert.True(t, m.s.Field.Open)
	assert.Equal(t, "x", m.s.Field.Text)
	assert.Contains(t, footerLine(m), "note not saved: lock notes: held")
	m, _ = typed(m, "enter")
	assert.Equal(t, 2, f.writer.AddCallCount(), "enter tries again")
}

func TestAnEmptyNoteIsNotSent(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m, _ = typed(m, "i", "c", "space", "enter")
	assert.Zero(t, f.writer.AddCallCount())
	assert.Contains(t, footerLine(m), "a note needs a text")
}

func TestVMarksTheLinesUpToTheCaret(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m, _ = typed(m, "i", "j", "v", "j", "j")
	assert.Contains(t, footerLine(m), "from line 2, c notes up to the caret")
	_, _ = typed(m, "c", "r", "enter")

	d := f.writer.AddArgsForCall(0)
	assert.Equal(t, []string{"1", "2", "changed 3"}, d.Anchor.Lines)
	assert.Equal(t, 2, d.Anchor.Start)
	assert.Equal(t, 4, d.Anchor.End)
	assert.Equal(t, []string{"0"}, d.Anchor.Before)
}

func TestWithoutTheCaretCTakesTheFirstChangeOnScreen(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	_, _ = typed(m, "c", "q", "enter")

	require.Equal(t, 1, f.writer.AddCallCount(), "q is text in the field")
	d := f.writer.AddArgsForCall(0)
	assert.Equal(t, []string{"changed 3"}, d.Anchor.Lines)
	assert.Equal(t, "q", d.Text)
}

func TestPlacedNotesShowInTheGutter(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
		spotAt("q", note.Question, note.Exact, 2, 3),
		spotAt("f", note.Finding, note.Likely, 6, 6),
		spotAt("o", note.Question, note.Orphaned, 0, 0),
	}})

	lines := plainLines(&m.s)
	// The mark stands in the middle of its columns.
	right := m.s.Layout.Left.Width + separatorWidth + 1
	mark := func(line int) string { return string([]rune(lines[bodyTop+line])[right]) }
	assert.Equal(t, "◆", mark(1))
	assert.Equal(t, "╎", mark(2), "the note's further lines")
	assert.Contains(t, lines[bodyTop+3], "?   text of q", "its line below its last one")
	assert.Equal(t, "◆", mark(6))
	assert.Contains(t, lines[bodyTop+7], "!   text of f")
	assert.Equal(t, " ", mark(0))
	assert.Contains(t, footerLine(m), "notes 1 not placed")
}

func TestPlacementsOfSidesNoLongerShownAreDropped(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	other := worktreeSide()
	other.Blob = objectID(t, "9")
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: other, Spots: []note.Spot{spotAt("q", note.Question, note.Exact, 1, 1)}})
	assert.Empty(t, m.s.Notes.Spots, "a late answer for a text since replaced")
}

func TestANoteFromElsewhereKeepsTheDraft(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m, _ = typed(m, "i", "c", "d", "r", "a", "f", "t")
	asked := f.reader.PlaceCallCount()

	m = notesCome(m, f,
		note.Update{Kind: note.StoreStatus, Live: true, Heads: []note.Head{{ID: "x", Path: "a.go", Origin: worktreeSide().Origin, Kind: note.Question, State: note.Open}}},
		note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{spotAt("x", note.Question, note.Exact, 1, 1)}})

	assert.Equal(t, asked+2, f.reader.PlaceCallCount(), "the sides are placed anew")
	assert.Equal(t, "draft", m.s.Field.Text)
	assert.True(t, m.s.Field.Open)
	m, _ = typed(m, "esc")
	assert.Contains(t, footerLine(m), "notes 1 open")
}

func TestTheFileListCountsOpenNotes(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	heads := []note.Head{{ID: "resolved", Path: "a.go", Origin: worktreeSide().Origin, State: note.Resolved}}
	for i := range 3 {
		heads = append(heads, note.Head{ID: note.ID(rune('a' + i)), Path: "a.go", Origin: worktreeSide().Origin, State: note.Open})
	}
	heads = append(heads, note.Head{ID: "index", Path: "b.go", Origin: note.Origin{Kind: change.KindIndex}, State: note.Open})
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Live: true, Heads: heads})

	assert.Equal(t, 3, m.s.openNotesIn(0))
	assert.Zero(t, m.s.openNotesIn(1), "an index note is not shown against the worktree")
}

func TestADamagedBookIsAnError(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Live: true, Err: fmt.Errorf("read notes: %w", note.ErrNotesDamaged)})
	assert.True(t, strings.HasPrefix(footerLine(m), "notes read notes: notes file damaged · q quit"), "the keys stay: %s", footerLine(m))
}

func TestNotesThatDoNotShowLiveSaySo(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus})
	assert.Contains(t, footerLine(m), "notes not live")
}

func TestTheCaretOnANoteOpensItAndXResolvesIt(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
		spotAt("a", note.Question, note.Exact, 2, 2), spotAt("b", note.Finding, note.Changed, 2, 2),
	}})
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)
	text := strings.Join(texts(m.s.Hover.Popup), "\n")
	assert.Contains(t, text, "question\n\ntext of a")
	assert.Contains(t, text, "finding · placed, changed\n\ntext of b")

	m, _ = typed(m, "x")
	require.Equal(t, 1, f.writer.ResolveCallCount())
	assert.Equal(t, note.ID("a"), f.writer.ResolveArgsForCall(0))

	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{spotAt("b", note.Finding, note.Changed, 2, 2)}})
	assert.NotContains(t, strings.Join(texts(m.s.Hover.Popup), "\n"), "text of a", "the popup follows the notes")
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide()})
	assert.False(t, m.s.Hover.Popup.Open, "and closes with the last")
}

func TestTheMouseOnAMarkOpensItsNotes(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{spotAt("a", note.Question, note.Exact, 2, 2)}})
	x := m.s.Layout.Left.Width + separatorWidth

	m, cmd := send(m, tea.MouseMotionMsg{X: x, Y: bodyTop + 1})

	assert.Nil(t, cmd, "no hover to wait for")
	require.True(t, m.s.Hover.Popup.Open)
	assert.Contains(t, strings.Join(texts(m.s.Hover.Popup), "\n"), "text of a")
	m, _ = send(m, tea.MouseMotionMsg{X: x, Y: bodyTop + 1})
	assert.True(t, m.s.Hover.Popup.Open, "resting on it keeps it")
}

func TestMGoesFromNoteToNote(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
		spotAt("a", note.Question, note.Exact, 20, 20), spotAt("b", note.Question, note.Exact, 30, 31),
	}})
	m, _ = typed(m, "m")
	assert.Equal(t, 19, m.s.top())
	m, _ = typed(m, "m")
	assert.Equal(t, 30, m.s.top(), "past the line of the note before")
	m, _ = typed(m, "M")
	assert.Equal(t, 19, m.s.top())
}

func TestMOpensTheNextFileWithNotes(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Live: true, Heads: []note.Head{
		{ID: "b", Path: "b.go", Origin: worktreeSide().Origin, State: note.Open},
	}})
	m, _ = typed(m, "m")
	assert.Equal(t, 1, m.s.Current)
	assert.Equal(t, 1, m.s.Notes.Seek)
	m, _ = typed(m, "m")
	assert.Contains(t, footerLine(m), "no further notes")
}

func TestMReachesNotesOnTheLastPage(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
		spotAt("a", note.Question, note.Exact, 70, 70), spotAt("b", note.Question, note.Exact, 75, 75),
	}})
	m, _ = typed(m, "m")
	top := m.s.top()
	require.Less(t, top, 69, "the last page cannot scroll the note to the top")
	assert.Equal(t, 69, m.s.Notes.At)
	m, _ = typed(m, "m")
	assert.Equal(t, 75, m.s.Notes.At, "the next m goes on from the note, not from the top")
	m, _ = typed(m, "m")
	assert.Contains(t, footerLine(m), "no further notes")
	m, _ = typed(m, "M")
	assert.Equal(t, 69, m.s.Notes.At)
}

// headSide is the key of a.go's left side in noteModel.
func headSide(t *testing.T) note.SnapshotKey {
	t.Helper()
	return note.SnapshotKey{Path: "a.go", Origin: note.Origin{Kind: change.KindCommit, Commit: objectID(t, "1")}, Blob: objectID(t, "2")}
}

func TestASeekWaitsForBothSidesAndEnds(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m.s.Notes.Seek = 1
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: headSide(t)})
	assert.Equal(t, 1, m.s.Notes.Seek, "the new side is still to come")
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{spotAt("a", note.Question, note.Exact, 40, 40)}})
	assert.Zero(t, m.s.Notes.Seek)
	assert.Equal(t, 39, m.s.top())

	m.s.Notes.Seek = 1
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Live: true})
	m = notesCome(m, f,
		note.Update{Kind: note.Placed, Key: headSide(t)},
		note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{spotAt("o", note.Question, note.Orphaned, 0, 0)}})
	assert.Zero(t, m.s.Notes.Seek, "it does not wait for another file")
	assert.Contains(t, footerLine(m), "the notes of this file have no place")
}

func TestTheNoteMWentToCountsOnlyInItsLayout(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
		spotAt("a", note.Question, note.Exact, 5, 5), spotAt("b", note.Question, note.Exact, 10, 10),
	}})
	m, _ = typed(m, "m")
	require.Equal(t, 4, m.s.Notes.At)
	m.s.toggleMode()
	m.s.toggleMode()
	m.s.Notes.At = 9
	m.s.Notes.AtLaid.mode = view.Hunks
	m, _ = typed(m, "m")
	assert.Equal(t, 10, m.s.Notes.At, "from the top, as a line of another layout is no place to go on from")
	assert.NotContains(t, footerLine(m), "no further notes")
}

func TestEscAfterEnterSaysTheNoteWasSaved(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	f.writer.AddReturns(3)
	m, _ = typed(m, "i", "c", "x", "enter", "esc")
	assert.False(t, m.s.Field.Open)
	m, _ = typed(m, "c")
	require.True(t, m.s.Field.Open, "a new note while the other is still saved")
	m = notesCome(m, f, note.Update{Kind: note.Done, Op: 3})
	assert.Contains(t, footerLine(m), "note saved before esc came")
}

func TestXInAHoverPopupIsNoResolve(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m.s.Hover.Popup = m.s.openPopup(code("func f()"), 10, bodyTop)
	m, _ = typed(m, "x")
	assert.Zero(t, f.writer.ResolveCallCount())
	assert.True(t, m.s.Hover.Popup.Open, "x is not a key of the hover's popup")
}

func TestAPlacementAskedIsNotAskedAgainUntilItComes(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	asked := f.reader.PlaceCallCount()
	m, _ = send(m, fileDiffsReady{})
	assert.Equal(t, asked, f.reader.PlaceCallCount())
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide()})
	m, _ = send(m, fileDiffsReady{})
	assert.Equal(t, asked, f.reader.PlaceCallCount(), "placed already")
	_ = notesCome(m, f, note.Update{Kind: note.StoreStatus, Live: true})
	assert.Equal(t, asked+2, f.reader.PlaceCallCount(), "a change of the book asks anew")
}
