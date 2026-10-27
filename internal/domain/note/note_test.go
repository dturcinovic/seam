package note_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

func TestTheBookAddsAndResolves(t *testing.T) {
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	var b note.Book
	d := note.Draft{Kind: note.Question, Text: "why?", Anchor: worktreeAnchor()}
	require.NoError(t, b.Add(d, "1", at))
	require.ErrorIs(t, b.Add(d, "1", at), note.ErrDuplicateID)
	require.Len(t, b.Notes, 1)
	assert.Equal(t, note.Open, b.Notes[0].State)
	assert.Equal(t, at, b.Notes[0].Created)

	require.NoError(t, b.Resolve("1"))
	require.NoError(t, b.Resolve("1"), "resolving twice is no error")
	assert.Equal(t, note.Resolved, b.Notes[0].State)
	require.ErrorIs(t, b.Resolve("2"), note.ErrNoNote)
}

// worktreeAnchor is the least a note needs to be anchored.
func worktreeAnchor() note.Anchor { return note.Anchor{Origin: note.Origin{Kind: change.KindWorktree}} }

func TestTheBookRefusesWhatIsTooMuch(t *testing.T) {
	var b note.Book
	draft := func(k note.Kind, text string) note.Draft {
		return note.Draft{Kind: k, Text: text, Anchor: worktreeAnchor()}
	}
	require.ErrorIs(t, b.Add(draft(note.Finding, ""), "e", time.Time{}), note.ErrEmptyText)
	require.ErrorIs(t, b.Add(draft(note.Finding, strings.Repeat("x", note.MaxText+1)), "l", time.Time{}), note.ErrTextTooLong)
	require.ErrorIs(t, b.Add(draft(0, "x"), "k", time.Time{}), note.ErrInvalidKind)
	require.ErrorIs(t, b.Add(note.Draft{Kind: note.Finding, Text: "x"}, "o", time.Time{}), note.ErrNoOrigin)
	require.ErrorIs(t, b.Add(note.Draft{Kind: note.Finding, Text: "x", Anchor: note.Anchor{Origin: note.Origin{Kind: change.KindCommit}}}, "o", time.Time{}),
		note.ErrNoOrigin, "a commit without id")
	require.NoError(t, b.Add(draft(note.Finding, strings.Repeat("x", note.MaxText)), "m", time.Time{}))

	b.Notes = make([]note.Note, note.MaxNotes)
	for i := range b.Notes {
		b.Notes[i].State = note.Open
	}
	require.ErrorIs(t, b.Add(draft(note.Question, "x"), "n", time.Time{}), note.ErrTooManyNotes)
	b.Notes[0].State = note.Resolved
	require.NoError(t, b.Add(draft(note.Question, "x"), "n", time.Time{}), "resolved notes do not count")
}

func TestOnlyGitSidesTakeNotes(t *testing.T) {
	head, err := change.ParseObjectID(strings.Repeat("1", 40))
	require.NoError(t, err)
	c, err := change.Commit("HEAD").Resolved(head)
	require.NoError(t, err)

	o, ok := note.OriginOf(c)
	assert.True(t, ok)
	assert.Equal(t, note.Origin{Kind: change.KindCommit, Commit: head}, o)
	_, ok = note.OriginOf(change.Commit("HEAD"))
	assert.False(t, ok, "an unresolved commit names no version")
	o, ok = note.OriginOf(change.Index())
	assert.True(t, ok)
	assert.Equal(t, "index", o.String())
	_, ok = note.OriginOf(change.File("/tmp/a"))
	assert.False(t, ok)
}

func TestNamesOfKindsStatesAndOutcomes(t *testing.T) {
	assert.Equal(t, []string{"unset", "question", "finding"},
		[]string{note.Kind(0).String(), note.Question.String(), note.Finding.String()})
	assert.Equal(t, []string{"unset", "open", "resolved", "taken", "answered"},
		[]string{note.State(0).String(), note.Open.String(), note.Resolved.String(), note.Taken.String(), note.Answered.String()})
	assert.Equal(t, []string{"unset", "placed", "likely placed", "placed, changed", "ambiguous", "orphaned"},
		[]string{note.Where(0).String(), note.Exact.String(), note.Likely.String(), note.Changed.String(), note.Ambiguous.String(), note.Orphaned.String()})
	assert.True(t, note.Changed.Shown())
	assert.False(t, note.Ambiguous.Shown())
	assert.Equal(t, "worktree", note.Origin{Kind: change.KindWorktree}.String())
	assert.Equal(t, "unset", note.Origin{}.String())
}
