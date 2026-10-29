package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// awayHeads are the heads of notes, each away.
func awayHeads(notes ...note.Note) []note.Head {
	var hs []note.Head
	for _, n := range notes {
		hs = append(hs, note.Head{ID: n.ID, Path: n.Anchor.Path, Origin: n.Anchor.Origin, Kind: n.Kind, State: n.State, Away: true})
	}
	return hs
}

func TestTheOverviewNamesTheBranchOfANoteAway(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m, _ = typed(m, "N")
	other := bookNote("o", "a.go", worktreeOrigin(), 1, 1)
	other.On = change.Head{Branch: "feature/x"}
	deleted := bookNote("d", "a.go", worktreeOrigin(), 1, 2)
	deleted.On = change.Head{Branch: "deleted"}
	detached := bookNote("c", "a.go", worktreeOrigin(), 1, 3)
	detached.On = change.Head{Commit: objectID(t, "2")}
	m = notesCome(m, f,
		note.Update{Kind: note.StoreStatus, Heads: awayHeads(other, deleted, detached), Live: true},
		note.Update{Kind: note.Overview, Notes: []note.Note{other, deleted, detached}, Gone: []string{"deleted"}})

	text := overviewText(m)
	assert.Contains(t, text, "worktree on feature/x")
	assert.Contains(t, text, "worktree branch gone")
	assert.Contains(t, text, "worktree on 2222222")
}

func TestANoteAwayOpensInAPopupThoughItsFileIsInTheWorktree(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	away := bookNote("o", "gone.go", worktreeOrigin(), 4, 1)
	away.On = change.Head{Branch: "feature/x"}
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: awayHeads(away), Live: true})
	m = overviewOf(t, m, f, away)
	loads := m.p.Files.(*infakes.FakeFileDiffLoader).LoadCallCount()

	m, _ = typed(m, "enter")
	require.True(t, m.s.Hover.Popup.Open)
	assert.Empty(t, m.s.VisitPath, "the file of this branch is not the one the note was set on")
	assert.Equal(t, loads, m.p.Files.(*infakes.FakeFileDiffLoader).LoadCallCount())
	assert.Contains(t, overviewText(m), "on feature/x")
}

func TestNotesAwayCountNowhereAndAreNotSent(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	away := waitingHead()
	away.Away = true
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: []note.Head{away}, Targets: []note.Session{agentSession()}, Live: true})
	assert.Empty(t, m.s.notesText())

	m, _ = typed(m, "S")
	assert.Zero(t, f.sender.SendCallCount())
	assert.Equal(t, "no notes to send", m.s.Note)
}

func TestTheFooterSaysWhyHEADCannotBeRead(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: []note.Head{waitingHead()}, HeadErr: errors.New("read HEAD: exit 128"), Live: true})
	assert.Contains(t, m.s.notesText(), "notes read HEAD: exit 128, 1 open")
	assert.NoError(t, m.s.Notes.Err, "the book itself reads")
}
