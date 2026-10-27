package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// waitingHead is an open note of a.go in the worktree.
func waitingHead() note.Head {
	return note.Head{ID: "a", Path: "a.go", Origin: note.Origin{Kind: change.KindWorktree}, Kind: note.Question, State: note.Open}
}

func TestSWithoutAgentSessionSaysSo(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: []note.Head{waitingHead()}, Sessions: []note.Session{agentSession()}, Live: true})

	m, _ = typed(m, "S")

	assert.Zero(t, f.sender.SendCallCount(), "a session whose process died is no target")
	assert.Equal(t, "no agent session in this worktree", m.s.Note)
}

func TestSWithoutNotesSaysSo(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Targets: []note.Session{agentSession()}, Live: true})

	m, _ = typed(m, "S")

	assert.Zero(t, f.sender.SendCallCount())
	assert.Equal(t, "no notes to send", m.s.Note)
}

func TestSSendsToTheOneSessionAndSaysSentNotArrived(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: []note.Head{waitingHead()}, Targets: []note.Session{agentSession()}, Live: true})
	f.sender.SendReturns(7)

	m, _ = typed(m, "S")
	require.Equal(t, 1, f.sender.SendCallCount())
	assert.Equal(t, note.ID("s"), f.sender.SendArgsForCall(0))

	m = notesCome(m, f, note.Update{Kind: note.Done, Op: 7})
	assert.Equal(t, "notes sent to claude-code, not taken yet", m.s.Note)

	m, _ = typed(m, "S")
	m = notesCome(m, f, note.Update{Kind: note.Done, Op: 7, Err: errors.New("locked")})
	assert.Equal(t, "notes not sent: locked", m.s.Note)

	m, _ = typed(m, "S")
	m = notesCome(m, f, note.Update{Kind: note.Done, Op: 7, Err: fmt.Errorf("%w: s", note.ErrNoSession)})
	assert.Equal(t, "notes not sent: the agent session ended", m.s.Note, "the process died since the status came")
}

func TestSWithTwoSessionsAsksWhichAndSendsToTheChosen(t *testing.T) {
	m, f := noteModel(t, headToWorktree(t))
	other := note.Session{ID: "o", Client: "opencode", Started: time.Date(2026, 10, 27, 9, 30, 0, 0, time.Local)}
	m = notesCome(m, f, note.Update{Kind: note.StoreStatus, Heads: []note.Head{waitingHead()}, Targets: []note.Session{agentSession(), other}, Live: true})

	m, _ = typed(m, "S")
	require.True(t, m.s.Notes.Choice.Open)
	assert.Zero(t, f.sender.SendCallCount())
	screen := strings.Join(plainLines(&m.s), "\n")
	assert.Contains(t, screen, "send notes to")
	assert.Contains(t, screen, "2 opencode")
	assert.Contains(t, screen, "since Oct 27 09:30")
	assert.Contains(t, m.View().Content, "1–9/enter send  j/k move  esc cancel")

	m, _ = typed(m, "x", "j", "enter")
	assert.False(t, m.s.Notes.Choice.Open)
	require.Equal(t, 1, f.sender.SendCallCount(), "x does nothing under the choice")
	assert.Equal(t, note.ID("o"), f.sender.SendArgsForCall(0))

	m, _ = typed(m, "S", "1")
	require.Equal(t, 2, f.sender.SendCallCount())
	assert.Equal(t, note.ID("s"), f.sender.SendArgsForCall(1), "by its number")

	m, _ = typed(m, "S", "esc")
	assert.False(t, m.s.Notes.Choice.Open)
	assert.Equal(t, 2, f.sender.SendCallCount(), "esc sends nothing")
}

func TestASentNoteIsNeverSaidToHaveArrived(t *testing.T) {
	sp := spotAt("a", note.Question, note.Exact, 2, 2)
	sp.Note.State = note.Sent
	m, f := noteModel(t, headToWorktree(t))
	m = notesCome(m, f, note.Update{Kind: note.Placed, Key: worktreeSide(), Spots: []note.Spot{sp}})
	m, _ = typed(m, "i", "j")
	require.True(t, m.s.Hover.Popup.Open)

	assert.Contains(t, strings.Join(texts(m.s.Hover.Popup), "\n"), "question · sent, not taken")
}
