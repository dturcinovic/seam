package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
)

func TestEInAPopupRewritesItsNote(t *testing.T) {
	m, f := linePopup(t)

	m, _ = typed(m, "E")
	require.True(t, m.s.Field.Open)
	assert.Equal(t, "text of a", m.s.Field.Text)
	assert.Equal(t, len("text of a"), m.s.Field.Cursor, "the cursor at the end")
	assert.Equal(t, noteDraft{Edit: "a", Kind: note.Question, Lines: noteTarget{File: 0, Side: filediff.NewSide, From: 1, To: 1}}, m.s.Notes.Draft)

	m, _ = typed(m, "!", "tab", "enter")

	require.Equal(t, 1, f.editor.EditCallCount())
	id, k, text := f.editor.EditArgsForCall(0)
	assert.Equal(t, []any{note.ID("a"), note.Finding, "text of a!"}, []any{id, k, text})
	assert.Zero(t, f.writer.AddCallCount(), "no new note")
}

func TestEscKeepsTheNoteAsItWas(t *testing.T) {
	m, f := linePopup(t)

	m, _ = typed(m, "E", "x", "esc")

	assert.False(t, m.s.Field.Open)
	assert.Zero(t, f.editor.EditCallCount())
	assert.Equal(t, noteDraft{}, m.s.Notes.Draft)
}

func TestAnEditThatFailsKeepsTheDraft(t *testing.T) {
	m, f := linePopup(t)
	f.editor.EditReturns(3)
	m, _ = typed(m, "E", "!", "enter")

	m = notesCome(m, f, note.Update{Kind: note.Done, Op: 3, Err: errors.New("locked")})

	assert.True(t, m.s.Field.Open)
	assert.Equal(t, "text of a!", m.s.Field.Text)
	assert.Equal(t, "note not saved: locked", m.s.Note)
}

func TestANoteAnAgentHoldsIsNotEdited(t *testing.T) {
	m, f := placedAt(t, taken(spotAt("a", note.Question, note.Exact, 2, 2)))
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)

	m, _ = typed(m, "E")

	assert.False(t, m.s.Field.Open)
	assert.Equal(t, "an agent holds this note: R gives it back", m.s.Note)
	assert.Zero(t, f.editor.EditCallCount())
}

// placedBoth is placedAt with both sides of a.go placed, as findPlaced
// waits for the old side too.
func placedBoth(t *testing.T, spots ...note.Spot) (model, noteFakes) {
	t.Helper()
	m, f := placedAt(t, spots...)
	k, ok := m.s.noteKey(0, filediff.OldSide)
	require.True(t, ok)
	return notesCome(m, f, note.Update{Kind: note.Placed, Key: k}), f
}

func TestEOnTheOverviewOpensTheFieldAtTheNote(t *testing.T) {
	for _, w := range []note.Where{note.Exact, note.Likely} {
		t.Run(w.String(), func(t *testing.T) {
			n := bookNote("w", "a.go", worktreeOrigin(), 50, 1)
			m, f := placedBoth(t, note.Spot{Note: n, Place: note.Placement{Where: w, Start: 50, End: 50}})
			m = overviewOf(t, m, f, n)

			m, _ = typed(m, "E")

			assert.False(t, m.s.Notes.Overview.Open)
			require.True(t, m.s.Field.Open)
			assert.Equal(t, n.Text, m.s.Field.Text)
			assert.Equal(t, note.ID("w"), m.s.Notes.Draft.Edit)
			assert.Equal(t, noteTarget{File: 0, Side: filediff.NewSide, From: 49, To: 49}, m.s.Notes.Draft.Lines)
			assert.Equal(t, 49-targetContext, m.s.Pos.AnchorRow, "jumped there")
			if w == note.Likely {
				assert.Equal(t, "note probably here", m.s.Note)
			}
		})
	}
}

func TestEOnTheOverviewWaitsForTheNotesPlace(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	n := bookNote("w", "a.go", worktreeOrigin(), 50, 1)
	m = overviewOf(t, m, f, n)
	m.s.Notes.Asked = map[note.SnapshotKey]bool{worktreeSide(): true}

	m, _ = typed(m, "E")
	require.False(t, m.s.Field.Open, "not placed yet")
	assert.True(t, m.s.Notes.Find.Edit)

	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{{Note: n, Place: note.Placement{Where: note.Exact, Start: 50, End: 50}}}})

	require.True(t, m.s.Field.Open)
	assert.Equal(t, note.ID("w"), m.s.Notes.Draft.Edit)
}

func TestEOnTheOverviewRefusesWhatItCannotEdit(t *testing.T) {
	resolved := bookNote("r", "a.go", worktreeOrigin(), 1, 1)
	resolved.State = note.Resolved
	tests := map[string]struct {
		n    note.Note
		want string
	}{
		"resolved":            {resolved, "a resolved note is not edited"},
		"in no side":          {bookNote("i", "a.go", note.Origin{Kind: change.KindIndex}, 1, 1), noPlaceToEdit},
		"placed not for sure": {bookNote("m", "a.go", worktreeOrigin(), 1, 1), noPlaceToEdit},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, f := placedAt(t, note.Spot{Note: tt.n, Place: note.Placement{Where: note.Ambiguous}})
			m = overviewOf(t, m, f, tt.n)

			m, _ = typed(m, "E")

			assert.False(t, m.s.Field.Open)
			assert.Equal(t, tt.want, m.s.Note)
		})
	}
}

func TestEInALinePopupSkipsANoteAnAgentHolds(t *testing.T) {
	m, f := placedAt(t, taken(spotAt("a", note.Question, note.Exact, 2, 2)), spotAt("b", note.Finding, note.Exact, 2, 2))
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)

	m, _ = typed(m, "E")

	require.True(t, m.s.Field.Open)
	assert.Equal(t, note.ID("b"), m.s.Notes.Draft.Edit)
	assert.Zero(t, f.editor.EditCallCount())
}

func TestEWhileTheNotesArePlacedAgainSaysSo(t *testing.T) {
	m, _ := linePopup(t)
	m.s.Notes.Asked = map[note.SnapshotKey]bool{worktreeSide(): true}

	m, _ = typed(m, "E")

	assert.False(t, m.s.Field.Open)
	assert.Equal(t, "the notes are being placed again: E once they are", m.s.Note)
}

func TestAWaitingEditOpensTheNoteAsItStandsOnceFound(t *testing.T) {
	n := bookNote("w", "a.go", worktreeOrigin(), 50, 1)
	at := note.Placement{Where: note.Exact, Start: 50, End: 50}
	waiting := func(t *testing.T) (model, noteFakes) {
		t.Helper()
		m, f := noteModel(t, headToWorktree(t))
		m = overviewOf(t, m, f, n)
		m.s.Notes.Asked = map[note.SnapshotKey]bool{worktreeSide(): true}
		m, _ = typed(m, "E")
		require.True(t, m.s.Notes.Find.Edit)
		return m, f
	}

	t.Run("newer text", func(t *testing.T) {
		m, f := waiting(t)
		now := n
		now.Text = "edited elsewhere"
		m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{{Note: now, Place: at}}})
		require.True(t, m.s.Field.Open)
		assert.Equal(t, "edited elsewhere", m.s.Field.Text)
	})
	t.Run("taken since", func(t *testing.T) {
		m, f := waiting(t)
		m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{taken(note.Spot{Note: n, Place: at})}})
		assert.False(t, m.s.Field.Open)
		assert.Equal(t, "an agent holds this note: R gives it back", m.s.Note)
	})
	t.Run("a field opened meanwhile", func(t *testing.T) {
		m, f := waiting(t)
		m, _ = typed(m, "c", "m", "y")
		require.True(t, m.s.Field.Open)
		m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{{Note: n, Place: at}}})
		assert.Equal(t, "my", m.s.Field.Text, "a key dropped the edit waiting")
		assert.Empty(t, m.s.Notes.Draft.Edit)
	})
}
