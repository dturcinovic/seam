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

func commitID(t *testing.T, digit string) change.ObjectID {
	t.Helper()
	id, err := change.ParseObjectID(strings.Repeat(digit, 40))
	require.NoError(t, err)
	return id
}

func TestAwayTellsNotesOfAnotherBranch(t *testing.T) {
	main, feature := change.Head{Branch: "main"}, change.Head{Branch: "feature"}
	detached := change.Head{Commit: commitID(t, "a")}
	tests := map[string]struct {
		on, cur change.Head
		away    bool
	}{
		"same branch":             {main, main, false},
		"other branch":            {main, feature, true},
		"same detached commit":    {detached, detached, false},
		"other detached commit":   {detached, change.Head{Commit: commitID(t, "b")}, true},
		"detached, now a branch":  {detached, main, true},
		"set before seam kept it": {change.Head{}, feature, false},
		"head not known":          {main, change.Head{}, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.away, note.Away(note.Note{On: tt.on}, tt.cur))
		})
	}
}

func TestTheBookKeepsTheBranchOnlyForTheWorktreeAndIndex(t *testing.T) {
	main := change.Head{Branch: "main"}
	tests := map[string]struct {
		origin note.Origin
		want   change.Head
	}{
		"worktree": {note.Origin{Kind: change.KindWorktree}, main},
		"index":    {note.Origin{Kind: change.KindIndex}, main},
		"commit":   {note.Origin{Kind: change.KindCommit, Commit: commitID(t, "c")}, change.Head{}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var b note.Book
			d := note.Draft{Kind: note.Question, Text: "why?", Anchor: note.Anchor{Origin: tt.origin}, On: main}
			require.NoError(t, b.Add(d, "1", time.Time{}))
			assert.Equal(t, tt.want, b.Notes[0].On)
		})
	}
}

// branchBook is a book with session "s", note "here" on main and note
// "there" on feature.
func branchBook(t *testing.T) *note.Book {
	t.Helper()
	b := &note.Book{}
	require.NoError(t, b.Join(note.Session{ID: "s", Client: "claude-code", PID: 1}))
	for id, branch := range map[note.ID]string{"here": "main", "there": "feature"} {
		d := note.Draft{Kind: note.Question, Text: "why?", Anchor: worktreeAnchor(), On: change.Head{Branch: branch}}
		require.NoError(t, b.Add(d, id, time.Time{}))
	}
	return b
}

func TestTakingWithoutIDsLeavesTheNotesOfOtherBranches(t *testing.T) {
	b := branchBook(t)
	got, err := b.Take(nil, note.Claim{ID: "c", Session: "s"}, change.Head{Branch: "main"})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"here"}, noteIDsOf(got.Notes))
	assert.False(t, got.More)

	named, err := b.Take([]note.ID{"there"}, note.Claim{ID: "d", Session: "s"}, change.Head{Branch: "main"})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"there"}, noteIDsOf(named.Notes), "a note named by id is taken wherever it belongs")
}

func TestSendingLeavesTheNotesOfOtherBranches(t *testing.T) {
	b := branchBook(t)
	bt, err := b.Send("b1", "s", time.Time{}, anyPID, change.Head{Branch: "main"})
	require.NoError(t, err)
	assert.Equal(t, []note.ID{"here"}, bt.Notes)

	_, err = b.Send("b2", "s", time.Time{}, anyPID, change.Head{Branch: "other"})
	require.ErrorIs(t, err, note.ErrNothingToSend, "only the notes of main wait, and they are sent")
}
