package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/out/outfakes"
	"seam/internal/app/usecase"
	"seam/internal/domain/change"
)

type fakes struct {
	revisions *outfakes.FakeRevisionResolver
	changes   *outfakes.FakeChangeLister
	untracked *outfakes.FakeUntrackedLister
}

func newChangesets() (*usecase.Changesets, fakes) {
	f := fakes{
		revisions: &outfakes.FakeRevisionResolver{},
		changes:   &outfakes.FakeChangeLister{},
		untracked: &outfakes.FakeUntrackedLister{},
	}
	f.revisions.ResolveCommitCalls(func(_ context.Context, rev string) (change.ObjectID, error) {
		return change.ParseObjectID(strings.Repeat(rev[:1], 40))
	})
	return usecase.NewChangesets(f.revisions, f.changes, f.untracked), f
}

func comparison(t *testing.T, left, right change.Source) change.Comparison {
	t.Helper()
	c, err := change.NewComparison(left, right)
	require.NoError(t, err)
	return c
}

func paths(files []change.ChangedFile) []string {
	var p []string
	for _, f := range files {
		p = append(p, f.Path())
	}
	return p
}

func TestUntrackedFilesOnlyWithTheWorktreeOnTheRight(t *testing.T) {
	tracked := change.ChangedFile{Status: change.Modified, OldPath: "b.go", NewPath: "b.go"}
	untracked := change.ChangedFile{Status: change.Untracked, NewPath: "new.go"}
	tests := []struct {
		name      string
		right     change.Source
		want      []change.ChangedFile
		wantCalls int
	}{
		{"worktree", change.Worktree(), []change.ChangedFile{tracked, untracked}, 1},
		{"index", change.Index(), []change.ChangedFile{tracked}, 0},
		{"commit", change.Commit("b"), []change.ChangedFile{tracked}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, f := newChangesets()
			f.changes.ListChangesReturns([]change.ChangedFile{tracked}, nil)
			f.untracked.ListUntrackedReturns([]string{"new.go"}, nil)

			cs, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), tt.right))
			require.NoError(t, err)

			assert.Equal(t, tt.want, cs.Files)
			assert.Equal(t, tt.wantCalls, f.untracked.ListUntrackedCallCount())
		})
	}
}

func TestCommitsAreResolvedBeforeListing(t *testing.T) {
	uc, f := newChangesets()

	cs, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Commit("b")))
	require.NoError(t, err)

	require.Equal(t, 1, f.changes.ListChangesCallCount())
	_, listed := f.changes.ListChangesArgsForCall(0)
	assert.Equal(t, strings.Repeat("a", 40), listed.Left.ID().String())
	assert.Equal(t, strings.Repeat("b", 40), listed.Right.ID().String())
	assert.Equal(t, listed, cs.Comparison, "the changeset keeps the resolved commits")
	assert.Equal(t, "a", cs.Comparison.Left.Rev(), "and the names the user gave")
}

func TestOnlyCommitsAreResolved(t *testing.T) {
	uc, f := newChangesets()

	_, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Index()))
	require.NoError(t, err)

	assert.Equal(t, 1, f.revisions.ResolveCommitCallCount())
}

func TestFilesAreOrderedByPath(t *testing.T) {
	uc, f := newChangesets()
	f.changes.ListChangesReturns([]change.ChangedFile{
		{Status: change.Modified, OldPath: "z.go", NewPath: "z.go"},
		{Status: change.Deleted, OldPath: "m.go"},
	}, nil)
	f.untracked.ListUntrackedReturns([]string{"a.go"}, nil)

	cs, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Worktree()))
	require.NoError(t, err)

	assert.Equal(t, []string{"a.go", "m.go", "z.go"}, paths(cs.Files))
}

func TestErrorsArePassedOn(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name  string
		right change.Source
		setup func(*testing.T, fakes)
		want  string
	}{
		{"resolve left", change.Worktree(), func(_ *testing.T, f fakes) {
			f.revisions.ResolveCommitReturnsOnCall(0, change.ObjectID{}, boom)
		}, "boom"},
		{"resolve right", change.Commit("b"), func(t *testing.T, f fakes) {
			f.revisions.ResolveCommitCalls(nil)
			f.revisions.ResolveCommitReturnsOnCall(0, mustID(t, strings.Repeat("a", 40)), nil)
			f.revisions.ResolveCommitReturnsOnCall(1, change.ObjectID{}, boom)
		}, "boom"},
		{"list changes", change.Worktree(), func(_ *testing.T, f fakes) { f.changes.ListChangesReturns(nil, boom) }, "list changes: boom"},
		{"list untracked", change.Worktree(), func(_ *testing.T, f fakes) { f.untracked.ListUntrackedReturns(nil, boom) }, "list untracked files: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, f := newChangesets()
			tt.setup(t, f)

			_, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), tt.right))

			require.ErrorIs(t, err, boom)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func mustID(t *testing.T, s string) change.ObjectID {
	t.Helper()
	id, err := change.ParseObjectID(s)
	require.NoError(t, err)
	return id
}
