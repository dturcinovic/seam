package note_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

func noteIDsOf(ns []note.Note) []note.ID {
	var out []note.ID
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func TestSendingBatchesTheWaitingNotesOfThatMoment(t *testing.T) {
	b := agentBook(t, "open", "taken", "resolved")
	_, err := b.Take([]note.ID{"taken"}, note.Claim{ID: "c0", Session: "s"}, change.Head{})
	require.NoError(t, err)
	require.NoError(t, b.Resolve("resolved"))

	bt, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	require.NoError(t, b.Add(note.Draft{Kind: note.Question, Text: "later", Anchor: worktreeAnchor()}, "later", time.Time{}))

	assert.Equal(t, []note.ID{"open"}, bt.Notes)
	assert.Equal(t, []note.Batch{bt}, b.Batches)
	assert.Equal(t, []note.State{note.Sent, note.Taken, note.Resolved, note.Open}, states(b))
	assert.Equal(t, note.ID("b1"), b.Notes[0].Batch)
}

func TestSendingRefusesWhatCannotBeSent(t *testing.T) {
	b := agentBook(t)
	_, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.ErrorIs(t, err, note.ErrNothingToSend)

	require.NoError(t, b.Add(note.Draft{Kind: note.Question, Text: "why", Anchor: worktreeAnchor()}, "a", time.Time{}))
	_, err = b.Send("b1", "gone", time.Time{}, anyPID, change.Head{})
	require.ErrorIs(t, err, note.ErrNoSession)
	assert.Equal(t, []note.State{note.Open}, states(b), "a refused send changes nothing")

	_, err = b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	_, err = b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.ErrorIs(t, err, note.ErrDuplicateID)
}

func TestTakingABatchTakesItsNotesForItsSession(t *testing.T) {
	b := agentBook(t, "a", "b")
	require.NoError(t, b.Join(note.Session{ID: "other", PID: 2}))
	_, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)

	other, err := b.TakeBatch("b1", note.Claim{ID: "c0", Session: "other"})
	require.NoError(t, err)
	assert.Empty(t, other.Notes)
	assert.Equal(t, []note.Refusal{{ID: "a", Reason: note.RefusedOtherSession}, {ID: "b", Reason: note.RefusedOtherSession}}, other.Refused)

	got, err := b.TakeBatch("b1", note.Claim{ID: "c1", Session: "s"})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"a", "b"}, noteIDsOf(got.Notes))
	assert.Empty(t, got.Refused)
	assert.Equal(t, []note.State{note.Taken, note.Taken}, states(b))

	again, err := b.TakeBatch("b1", note.Claim{ID: "c2", Session: "s"})
	require.NoError(t, err)
	assert.Empty(t, again.Notes)
	assert.Empty(t, again.Refused, "what the session took is no refusal")

	require.NoError(t, b.Join(note.Session{ID: "late", PID: 3}))
	b.Batches[0].Session = "late"
	_, err = b.TakeBatch("b1", note.Claim{ID: "c3", Session: "late"})
	require.NoError(t, err)

	_, err = b.TakeBatch("nope", note.Claim{ID: "c3", Session: "s"})
	require.ErrorIs(t, err, note.ErrNoBatch)
}

func TestSendingAgainMovesTheNotesNotTakenToTheNewBatch(t *testing.T) {
	b := agentBook(t, "a", "b")
	_, err := b.Send("old", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	_, err = b.Take([]note.ID{"a"}, note.Claim{ID: "c0", Session: "s"}, change.Head{})
	require.NoError(t, err)

	bt, err := b.Send("new", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"b"}, bt.Notes)

	got, err := b.TakeBatch("old", note.Claim{ID: "c1", Session: "s"})
	require.NoError(t, err)
	assert.Empty(t, got.Notes)
	assert.Equal(t, []note.Refusal{{ID: "b", Reason: note.RefusedRebatched}}, got.Refused, "a is this session's already")

	got, err = b.TakeBatch("new", note.Claim{ID: "c2", Session: "s"})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"b"}, noteIDsOf(got.Notes))
}

func TestSentNotesAreTakenWithoutBatchByAnySession(t *testing.T) {
	b := agentBook(t, "a", "b")
	require.NoError(t, b.Join(note.Session{ID: "other", PID: 2}))
	_, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)

	got, err := b.Take(nil, note.Claim{ID: "c", Session: "other"}, change.Head{})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"a", "b"}, noteIDsOf(got.Notes))
}

func TestANoteOfABatchGivenBackIsSentAgain(t *testing.T) {
	b := agentBook(t, "a", "b", "c")
	_, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	_, err = b.TakeBatch("b1", note.Claim{ID: "c1", Session: "s"})
	require.NoError(t, err)

	require.NoError(t, b.Release("a"))
	b.Leave("s")

	assert.Equal(t, []note.State{note.Sent, note.Sent, note.Sent}, states(b))
	assert.Equal(t, note.ID("b1"), b.Notes[1].Batch)
}

func TestSendingDropsBatchesWithNothingLeft(t *testing.T) {
	b := agentBook(t, "a")
	_, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	_, err = b.Send("b2", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	got, err := b.TakeBatch("b2", note.Claim{ID: "c", Session: "s"})
	require.NoError(t, err)
	_, err = b.Reply("a", note.Reply{ID: "r", Claim: got.Claim.ID, Text: "because"})
	require.NoError(t, err)

	require.NoError(t, b.Add(note.Draft{Kind: note.Question, Text: "next", Anchor: worktreeAnchor()}, "next", time.Time{}))
	_, err = b.Send("b3", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)

	require.Len(t, b.Batches, 1)
	assert.Equal(t, note.ID("b3"), b.Batches[0].ID)
}

func TestTheBookKeepsAtMostMaxBatches(t *testing.T) {
	b := agentBook(t, "a")
	for i := range note.MaxBatches + 1 {
		_, err := b.Send(note.ID(fmt.Sprint(i)), "s", time.Time{}, anyPID, change.Head{})
		require.NoError(t, err)
	}
	require.Len(t, b.Batches, note.MaxBatches)
	assert.Equal(t, note.ID("1"), b.Batches[0].ID)
}

func TestSendingsAreTheBatchesOfASessionWithNotesNotTaken(t *testing.T) {
	b := agentBook(t, "a", "b")
	require.NoError(t, b.Join(note.Session{ID: "other", PID: 2}))
	_, err := b.Send("b1", "other", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	assert.Empty(t, b.Sendings("s"))

	_, err = b.Send("b2", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	_, err = b.Take([]note.ID{"a"}, note.Claim{ID: "c", Session: "s"}, change.Head{})
	require.NoError(t, err)

	got := b.Sendings("s")
	require.Len(t, got, 1)
	assert.Equal(t, note.ID("b2"), got[0].Batch.ID)
	assert.Equal(t, []note.ID{"b"}, noteIDsOf(got[0].Notes))
	assert.Empty(t, b.Sendings("other"), "b1's notes moved to b2")
}

// anyPID takes every process for alive.
func anyPID(int) bool { return true }

func TestABatchGoesOnlyToASessionWhoseProcessRuns(t *testing.T) {
	b := agentBook(t, "a")
	_, err := b.Send("b1", "s", time.Time{}, func(int) bool { return false }, change.Head{})
	require.ErrorIs(t, err, note.ErrNoSession)
	assert.Equal(t, []note.State{note.Open}, states(b))
}

func TestANoteOfABatchDroppedSinceIsGivenBackOpen(t *testing.T) {
	b := agentBook(t, "a")
	_, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	_, err = b.TakeBatch("b1", note.Claim{ID: "c", Session: "s"})
	require.NoError(t, err)
	b.Batches = nil

	require.NoError(t, b.Release("a"))

	assert.Equal(t, []note.State{note.Open}, states(b), "no batch left to take it by")
}

func TestTakingTheRestOfABatchLeavesOutWhatTheSessionTook(t *testing.T) {
	var ids []note.ID
	for i := range note.MaxTake + 2 {
		ids = append(ids, note.ID(fmt.Sprint(i)))
	}
	b := agentBook(t, ids...)
	_, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	first, err := b.TakeBatch("b1", note.Claim{ID: "c1", Session: "s"})
	require.NoError(t, err)
	require.True(t, first.More)

	rest, err := b.TakeBatch("b1", note.Claim{ID: "c2", Session: "s"})
	require.NoError(t, err)
	assert.Len(t, rest.Notes, 2)
	assert.Empty(t, rest.Refused)
}

func TestDeletingTakesANoteOutInAnyState(t *testing.T) {
	b := agentBook(t, "sent", "taken", "answered", "resolved")
	_, err := b.Take([]note.ID{"taken", "answered"}, note.Claim{ID: "c0", Session: "s"}, change.Head{})
	require.NoError(t, err)
	_, err = b.Reply("answered", note.Reply{ID: "r1", Claim: "c0", Text: "because"})
	require.NoError(t, err)
	require.NoError(t, b.Resolve("resolved"))
	_, err = b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	require.NoError(t, b.Add(note.Draft{Kind: note.Question, Text: "later", Anchor: worktreeAnchor()}, "open", time.Time{}))
	require.Equal(t, []note.State{note.Sent, note.Taken, note.Answered, note.Resolved, note.Open}, states(b))

	for _, id := range []note.ID{"sent", "taken", "answered", "resolved", "open"} {
		require.NoError(t, b.Delete(id), id)
	}

	assert.Empty(t, b.Notes)
	require.ErrorIs(t, b.Delete("open"), note.ErrNoNote)
	_, err = b.Reply("taken", note.Reply{ID: "r2", Claim: "c0", Text: "late"})
	require.ErrorIs(t, err, note.ErrNoNote, "the agent's answer to a deleted note")
	took, err := b.TakeBatch("b1", note.Claim{ID: "c1", Session: "s"})
	require.NoError(t, err)
	assert.Empty(t, took.Notes)
	assert.Equal(t, []note.Refusal{{ID: "sent", Reason: note.RefusedUnknown}}, took.Refused, "the batch of a deleted note")
}

func TestEditingRewritesANoteAndLetsItWaitAgain(t *testing.T) {
	b := agentBook(t, "sent", "answered")
	_, err := b.Take([]note.ID{"answered"}, note.Claim{ID: "c0", Session: "s"}, change.Head{})
	require.NoError(t, err)
	_, err = b.Reply("answered", note.Reply{ID: "r1", Claim: "c0", Text: "because"})
	require.NoError(t, err)
	_, err = b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	require.NoError(t, b.Add(note.Draft{Kind: note.Question, Text: "later", Anchor: worktreeAnchor()}, "open", time.Time{}))
	require.Equal(t, []note.State{note.Sent, note.Answered, note.Open}, states(b))
	anchor := b.Notes[1].Anchor

	for _, id := range []note.ID{"sent", "answered", "open"} {
		require.NoError(t, b.Edit(id, note.Finding, "now "+string(id)), id)
	}

	assert.Equal(t, []note.State{note.Open, note.Open, note.Open}, states(b))
	for _, n := range b.Notes {
		assert.Equal(t, note.Finding, n.Kind)
		assert.Equal(t, "now "+string(n.ID), n.Text)
		assert.Empty(t, n.Batch, "out of its batch")
	}
	assert.Equal(t, anchor, b.Notes[1].Anchor)
	assert.Len(t, b.Notes[1].Replies, 1, "the answers stay")
	took, err := b.TakeBatch("b1", note.Claim{ID: "c1", Session: "s"})
	require.NoError(t, err)
	assert.Empty(t, took.Notes, "the batch takes no note edited since")
}

func TestEditingRefusesWhatItMustNotChange(t *testing.T) {
	b := agentBook(t, "taken", "resolved")
	_, err := b.Take([]note.ID{"taken"}, note.Claim{ID: "c0", Session: "s"}, change.Head{})
	require.NoError(t, err)
	require.NoError(t, b.Resolve("resolved"))
	before := slices.Clone(b.Notes)

	tests := []struct {
		id   note.ID
		k    note.Kind
		text string
		want error
	}{
		{"taken", note.Question, "x", note.ErrTaken},
		{"resolved", note.Question, "x", note.ErrResolved},
		{"gone", note.Question, "x", note.ErrNoNote},
		{"taken", note.Question, "", note.ErrEmptyText},
		{"taken", note.Question, strings.Repeat("x", note.MaxText+1), note.ErrTextTooLong},
		{"taken", 0, "x", note.ErrInvalidKind},
	}
	for _, tt := range tests {
		require.ErrorIs(t, b.Edit(tt.id, tt.k, tt.text), tt.want, "%s %q", tt.id, tt.text)
	}
	assert.Equal(t, before, b.Notes, "nothing changed")
}

func TestEditingWithoutAChangeKeepsTheNoteWhereItStands(t *testing.T) {
	b := agentBook(t, "sent")
	bt, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{})
	require.NoError(t, err)
	n := b.Notes[0]

	require.NoError(t, b.Edit("sent", n.Kind, n.Text))

	assert.Equal(t, n, b.Notes[0], "still sent in its batch")
	took, err := b.TakeBatch(bt.ID, note.Claim{ID: "c0", Session: "s"})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"sent"}, noteIDsOf(took.Notes))
}
