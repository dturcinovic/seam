package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// agentSession is the session of an agent that takes notes.
func agentSession() note.Session { return note.Session{ID: "s", Client: "claude-code"} }

// taken is spot sp taken by agentSession at 14:05.
func taken(sp note.Spot) note.Spot {
	sp.Note.State = note.Taken
	sp.Note.Claim = &note.Claim{ID: "c", Session: "s", At: time.Date(2026, 10, 27, 14, 5, 0, 0, time.Local)}
	return sp
}

// answered is spot sp answered by agentSession.
func answered(sp note.Spot, text string) note.Spot {
	sp.Note.State = note.Answered
	sp.Note.Replies = []note.Reply{{ID: "r", Author: "claude-code", Text: text}}
	return sp
}

func TestRGivesBackTheNoteAnAgentTook(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f,
		note.Update{Kind: note.StoreStatus, Sessions: []note.Session{agentSession()}, Live: true},
		note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
			spotAt("a", note.Question, note.Exact, 2, 2), taken(spotAt("b", note.Finding, note.Exact, 2, 2)),
		}})
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)
	assert.Contains(t, strings.Join(texts(m.s.Hover.Popup), "\n"), "finding · taken by claude-code since Oct 27 14:05")

	m, _ = typed(m, "R")

	require.Equal(t, 1, f.release.ReleaseCallCount())
	assert.Equal(t, note.ID("b"), f.release.ReleaseArgsForCall(0), "the taken one of the line")
	assert.Equal(t, "note given back", m.s.Note)
	assert.True(t, m.s.Hover.Popup.Open, "the popup stays and follows the book")

}

func TestRWhereNoNoteIsTakenSaysSo(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{spotAt("a", note.Question, note.Exact, 2, 2)}})
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)

	m, _ = typed(m, "R")

	assert.Zero(t, f.release.ReleaseCallCount())
	assert.Equal(t, "no note taken here", m.s.Note)
}

func TestAnAnswerShowsAsMarkdownUnderItsAuthor(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{
		answered(spotAt("a", note.Question, note.Exact, 2, 2), "It is **nil** when\r\nthe file is `empty`."),
	}})
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)

	text := strings.Join(texts(m.s.Hover.Popup), "\n")
	assert.Contains(t, text, "question · answered\n\ntext of a")
	assert.Contains(t, text, "claude-code\n\nIt is nil when the file is empty.", "read as Markdown")
}

func TestAnAnsweredNoteShowsATickInItsLine(t *testing.T) {
	m, _ := placedAt(t, answered(spotAt("a", note.Question, note.Exact, 3, 3), "yes"))
	assert.Contains(t, strings.Join(plainLines(&m.s), "\n"), "✓   text of a")
}

func TestTheFooterCountsAnsweredNotesApart(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	heads := []note.Head{
		{ID: "o", Path: "a.go", Origin: worktreeOrigin(), State: note.Open},
		{ID: "t", Path: "a.go", Origin: worktreeOrigin(), State: note.Taken},
		{ID: "a", Path: "a.go", Origin: worktreeOrigin(), State: note.Answered},
	}
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: heads, Live: true})
	assert.Contains(t, m.s.notesText(), "notes 2 open, 1 answered")
}

func TestROnTheOverviewGivesBackOnlyATakenNote(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	open := bookNote("a", "a.go", worktreeOrigin(), 1, 1)
	took := bookNote("b", "a.go", worktreeOrigin(), 2, 2)
	took.State, took.Claim = note.Taken, &note.Claim{ID: "c", Session: "s"}
	m = overviewOf(t, m, f, open, took)

	m, _ = typed(m, "R")
	assert.Zero(t, f.release.ReleaseCallCount(), "an open note is no one's")
	_, _ = typed(m, "j", "R")
	require.Equal(t, 1, f.release.ReleaseCallCount())
	assert.Equal(t, note.ID("b"), f.release.ReleaseArgsForCall(0))
}

func TestAnAnswerArrivingChangesTheOverviewAndKeepsTheChoice(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	a, b := bookNote("a", "a.go", worktreeOrigin(), 1, 1), bookNote("b", "a.go", worktreeOrigin(), 2, 2)
	m = overviewOf(t, m, f, a, b)
	m, _ = typed(m, "j")

	b.State, b.Replies = note.Answered, []note.Reply{{Author: "claude-code", Text: "because"}}
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Live: true}, note.Update{Kind: note.Overview, Notes: []note.Note{a, b}})

	assert.Equal(t, note.ID("b"), m.s.Notes.Overview.Sel)
	assert.Contains(t, overviewText(m), "✓ answered worktree")
}

func TestTheOverviewsPopupOfANoteShowsItsAnswerOnceItComes(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	// An index note no side shows, so that enter opens its popup.
	took := bookNote("a", "a.go", note.Origin{Kind: change.KindIndex}, 1, 1)
	took.State, took.Claim = note.Taken, &note.Claim{ID: "c", Session: "s"}
	m = overviewOf(t, m, f, took)
	m, _ = typed(m, "enter")
	require.True(t, m.s.Hover.Popup.Open)

	took.State, took.Claim, took.Replies = note.Answered, nil, []note.Reply{{Author: "opencode", Text: "because"}}
	m = notesCome(m, f, note.Update{Kind: note.Overview, Notes: []note.Note{took}})

	assert.Contains(t, strings.Join(texts(m.s.Hover.Popup), "\n"), "opencode\n\nbecause")
	m, _ = typed(m, "R")
	assert.Zero(t, f.release.ReleaseCallCount(), "answered, so taken no more")
	assert.Equal(t, "no note taken here", m.s.Note)
}
