package note_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// agentBook is a book with session "s" and the open notes ids.
func agentBook(t *testing.T, ids ...note.ID) *note.Book {
	t.Helper()
	b := &note.Book{}
	require.NoError(t, b.Join(note.Session{ID: "s", Client: "claude-code", PID: 1}))
	for _, id := range ids {
		require.NoError(t, b.Add(note.Draft{Kind: note.Question, Text: "why " + string(id), Anchor: worktreeAnchor()}, id, time.Time{}))
	}
	return b
}

func states(b *note.Book) []note.State {
	var out []note.State
	for _, n := range b.Notes {
		out = append(out, n.State)
	}
	return out
}

func TestTakingWithoutIDsTakesTheOpenNotes(t *testing.T) {
	b := agentBook(t, "a", "b", "c")
	require.NoError(t, b.Resolve("b"))

	got, err := b.Take(nil, note.Claim{ID: "c1", Session: "s"}, change.Head{})
	require.NoError(t, err)

	assert.Equal(t, []note.ID{"a", "c"}, []note.ID{got.Notes[0].ID, got.Notes[1].ID})
	assert.Empty(t, got.Refused)
	assert.False(t, got.More)
	assert.Equal(t, []note.State{note.Taken, note.Resolved, note.Taken}, states(b))
	assert.Equal(t, note.ID("c1"), b.Notes[0].Claim.ID)

	again, err := b.Take(nil, note.Claim{ID: "c2", Session: "s"}, change.Head{})
	require.NoError(t, err)
	assert.Empty(t, again.Notes, "a second session finds nothing left to take")
}

func TestTakingIDsRefusesWhatIsNotOpen(t *testing.T) {
	b := agentBook(t, "open", "taken", "answered", "resolved")
	_, err := b.Take([]note.ID{"taken", "answered"}, note.Claim{ID: "c0", Session: "s"}, change.Head{})
	require.NoError(t, err)
	_, err = b.Reply("answered", note.Reply{ID: "r", Claim: "c0", Text: "because"})
	require.NoError(t, err)
	require.NoError(t, b.Resolve("resolved"))

	got, err := b.Take([]note.ID{"open", "taken", "answered", "resolved", "gone", "open"}, note.Claim{ID: "c1", Session: "s"}, change.Head{})
	require.NoError(t, err)

	require.Len(t, got.Notes, 1)
	assert.Equal(t, note.ID("open"), got.Notes[0].ID)
	assert.Equal(t, []note.Refusal{
		{ID: "taken", Reason: note.RefusedTaken}, {ID: "answered", Reason: note.RefusedAnswered},
		{ID: "resolved", Reason: note.RefusedResolved}, {ID: "gone", Reason: note.RefusedUnknown},
	}, got.Refused)
}

func TestTakingStopsAtMaxTake(t *testing.T) {
	var ids []note.ID
	for i := range note.MaxTake + 1 {
		ids = append(ids, note.ID(fmt.Sprint(i)))
	}
	b := agentBook(t, ids...)

	got, err := b.Take(nil, note.Claim{ID: "c", Session: "s"}, change.Head{})
	require.NoError(t, err)
	assert.Len(t, got.Notes, note.MaxTake)
	assert.True(t, got.More)

	rest, err := b.Take(nil, note.Claim{ID: "d", Session: "s"}, change.Head{})
	require.NoError(t, err)
	assert.Len(t, rest.Notes, 1)
	assert.False(t, rest.More)
}

func TestTakingIDsCountsOnlyTheNotesTaken(t *testing.T) {
	var ids []note.ID
	for i := range note.MaxTake + 5 {
		ids = append(ids, note.ID(fmt.Sprint(i)))
	}
	b := agentBook(t, ids...)

	first, err := b.Take(ids, note.Claim{ID: "c", Session: "s"}, change.Head{})
	require.NoError(t, err)
	assert.Len(t, first.Notes, note.MaxTake)
	assert.True(t, first.More)
	assert.Empty(t, first.Refused, "the rest is left, not refused")

	again, err := b.Take(ids, note.Claim{ID: "d", Session: "s"}, change.Head{})
	require.NoError(t, err)
	assert.Len(t, again.Notes, 5, "the same ids again reach the rest")
	assert.Len(t, again.Refused, note.MaxTake)
	assert.False(t, again.More)
}

func TestTakingNeedsASession(t *testing.T) {
	b := agentBook(t, "a")
	_, err := b.Take(nil, note.Claim{ID: "c", Session: "other"}, change.Head{})
	require.ErrorIs(t, err, note.ErrNoSession)
	assert.Equal(t, note.Open, b.Notes[0].State)
	require.ErrorIs(t, b.Join(note.Session{ID: "s"}), note.ErrDuplicateID)
}

func TestAReplyAnswersTheNote(t *testing.T) {
	b := agentBook(t, "a", "b")
	_, err := b.Take(nil, note.Claim{ID: "c", Session: "s"}, change.Head{})
	require.NoError(t, err)

	got, err := b.Reply("a", note.Reply{ID: "r1", Author: "claude-code", Claim: "c", Text: "because"})
	require.NoError(t, err)
	assert.Equal(t, note.Answer{Reply: "r1"}, got)
	assert.Equal(t, note.Answered, b.Notes[0].State)
	assert.Nil(t, b.Notes[0].Claim)

	again, err := b.Reply("a", note.Reply{ID: "r2", Claim: "c", Text: "because"})
	require.NoError(t, err)
	assert.Equal(t, note.Answer{Reply: "r1"}, again, "the same reply again is the one there")
	assert.Len(t, b.Notes[0].Replies, 1)

	_, err = b.Reply("b", note.Reply{ID: "r3", Text: "without a claim"})
	require.NoError(t, err)
	assert.Equal(t, note.Taken, b.Notes[1].State, "came late: the note's claim is another")
}

func TestALateReplyLeavesTheNewerTakingStanding(t *testing.T) {
	b := agentBook(t, "a")
	_, err := b.Take(nil, note.Claim{ID: "old", Session: "s"}, change.Head{})
	require.NoError(t, err)
	require.NoError(t, b.Release("a"))
	_, err = b.Take(nil, note.Claim{ID: "new", Session: "s"}, change.Head{})
	require.NoError(t, err)

	got, err := b.Reply("a", note.Reply{ID: "r", Claim: "old", Text: "late"})
	require.NoError(t, err)

	assert.True(t, got.Late)
	assert.Equal(t, note.Taken, b.Notes[0].State)
	assert.Equal(t, note.ID("new"), b.Notes[0].Claim.ID)
	require.Len(t, b.Notes[0].Replies, 1, "kept all the same")

	got, err = b.Reply("a", note.Reply{ID: "r2", Claim: "new", Text: "on time"})
	require.NoError(t, err)
	assert.False(t, got.Late)
	assert.Equal(t, note.Answered, b.Notes[0].State)
}

func TestARefusedReply(t *testing.T) {
	b := agentBook(t, "a")
	for _, c := range []struct {
		name string
		id   note.ID
		text string
		want error
	}{
		{"empty", "a", "", note.ErrEmptyText},
		{"too long", "a", strings.Repeat("x", note.MaxReply+1), note.ErrReplyTooLong},
		{"no such note", "b", "x", note.ErrNoNote},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := b.Reply(c.id, note.Reply{Text: c.text})
			require.ErrorIs(t, err, c.want)
		})
	}
	require.NoError(t, b.Resolve("a"))
	_, err := b.Reply("a", note.Reply{Text: "x"})
	require.ErrorIs(t, err, note.ErrResolved)
	assert.Empty(t, b.Notes[0].Replies)
}

func TestReleaseAndLeaveGiveNotesBack(t *testing.T) {
	b := agentBook(t, "a", "b", "c")
	require.NoError(t, b.Join(note.Session{ID: "t", PID: 2}))
	_, err := b.Take([]note.ID{"a", "b"}, note.Claim{ID: "c1", Session: "s"}, change.Head{})
	require.NoError(t, err)
	_, err = b.Take([]note.ID{"c"}, note.Claim{ID: "c2", Session: "t"}, change.Head{})
	require.NoError(t, err)

	require.NoError(t, b.Release("a"))
	assert.Nil(t, b.Notes[0].Claim)
	require.ErrorIs(t, b.Release("a"), note.ErrNotTaken)
	require.ErrorIs(t, b.Release("x"), note.ErrNoNote)

	b.Leave("s")
	b.Leave("s")
	assert.Equal(t, []note.State{note.Open, note.Open, note.Taken}, states(b), "only its own")
	_, ok := b.Session("s")
	assert.False(t, ok)
}

func TestPruneKeepsSessionsAliveOrHoldingAClaim(t *testing.T) {
	b := agentBook(t, "a")
	require.NoError(t, b.Join(note.Session{ID: "dead", PID: 2}))
	require.NoError(t, b.Join(note.Session{ID: "holds", PID: 3}))
	_, err := b.Take(nil, note.Claim{ID: "c", Session: "holds"}, change.Head{})
	require.NoError(t, err)

	b.Prune(func(pid int) bool { return pid == 1 })

	var left []note.ID
	for _, s := range b.Sessions {
		left = append(left, s.ID)
	}
	assert.Equal(t, []note.ID{"s", "holds"}, left)
}

func TestResolvingATakenNoteDropsItsClaim(t *testing.T) {
	b := agentBook(t, "a")
	_, err := b.Take(nil, note.Claim{ID: "c", Session: "s"}, change.Head{})
	require.NoError(t, err)
	require.NoError(t, b.Resolve("a"))
	assert.Nil(t, b.Notes[0].Claim)
}

func TestTakenByNamesTheSessionHoldingTheClaim(t *testing.T) {
	sessions := []note.Session{{ID: "s", Client: "opencode"}}
	n := note.Note{Claim: &note.Claim{Session: "s"}}

	got, ok := note.TakenBy(n, sessions)
	assert.True(t, ok)
	assert.Equal(t, "opencode", got.Client)
	_, ok = note.TakenBy(note.Note{}, sessions)
	assert.False(t, ok, "no claim")
	_, ok = note.TakenBy(n, nil)
	assert.False(t, ok, "its session gone")
}

func TestTargetsAreTheSessionsWhoseProcessRuns(t *testing.T) {
	b := agentBook(t)
	require.NoError(t, b.Join(note.Session{ID: "dead", PID: 3}))

	got := b.Targets(func(pid int) bool { return pid == 1 })

	require.Len(t, got, 1)
	assert.Equal(t, note.ID("s"), got[0].ID)
}
