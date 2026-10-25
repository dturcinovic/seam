package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/out/outfakes"
	"github.com/dturcinovic/seam/internal/app/usecase"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/testsupport"
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
	return usecase.NewChangesets(f.revisions, f.changes, f.untracked, nil), f
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

// reloadOne reloads as generation 1 and waits for its update.
func reloadOne(t *testing.T, uc *usecase.Changesets) change.Update {
	t.Helper()
	const gen change.Gen = 1
	uc.Reload(gen)
	all := testsupport.Collect(t, uc, func(u []change.Update) bool { return len(u) > 0 && u[len(u)-1].Gen == gen })
	return all[len(all)-1]
}

func TestReloadResolvesTheRevisionsAnew(t *testing.T) {
	uc, f := newChangesets()
	start(t, uc)
	_, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Worktree()))
	require.NoError(t, err)
	moved := mustID(t, strings.Repeat("c", 40))
	f.revisions.ResolveCommitReturns(moved, nil)
	f.changes.ListChangesReturns([]change.ChangedFile{{Status: change.Modified, OldPath: "b.go", NewPath: "b.go"}}, nil)

	u := reloadOne(t, uc)

	require.NoError(t, u.Err)
	assert.Equal(t, change.Gen(1), u.Gen)
	assert.Equal(t, moved, u.Changeset.Comparison.Left.ID(), "a commit moved HEAD")
	assert.Equal(t, "a", u.Changeset.Comparison.Left.Rev())
	assert.Equal(t, []string{"b.go"}, paths(u.Changeset.Files))
	_, rev := f.revisions.ResolveCommitArgsForCall(1)
	assert.Equal(t, "a", rev)
}

func TestReloadErrors(t *testing.T) {
	t.Run("before the first load", func(t *testing.T) {
		uc, _ := newChangesets()
		start(t, uc)

		require.ErrorIs(t, reloadOne(t, uc).Err, change.ErrNotLoaded)
	})
	t.Run("from listing", func(t *testing.T) {
		uc, f := newChangesets()
		start(t, uc)
		_, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Worktree()))
		require.NoError(t, err)
		boom := errors.New("boom")
		f.changes.ListChangesReturns(nil, boom)

		require.ErrorIs(t, reloadOne(t, uc).Err, boom)
	})
	t.Run("from resolving", func(t *testing.T) {
		uc, f := newChangesets()
		start(t, uc)
		_, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Worktree()))
		require.NoError(t, err)
		gone := errors.New("unknown revision")
		f.revisions.ResolveCommitReturns(change.ObjectID{}, gone)

		require.ErrorIs(t, reloadOne(t, uc).Err, gone)
	})
}

func TestANewerReloadCancelsTheRunningOne(t *testing.T) {
	uc, f := newChangesets()
	start(t, uc)
	_, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Index()))
	require.NoError(t, err)
	started := make(chan struct{})
	f.changes.ListChangesCalls(func(ctx context.Context, _ change.Comparison) ([]change.ChangedFile, error) {
		if f.changes.ListChangesCallCount() == 2 {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return nil, nil
	})

	uc.Reload(1)
	testsupport.Until(t, started)
	uc.Reload(2)

	all := testsupport.Collect(t, uc, func(u []change.Update) bool { return len(u) > 0 })
	require.Len(t, all, 1, "the cancelled reload publishes nothing")
	assert.Equal(t, change.Gen(2), all[0].Gen)
	require.NoError(t, all[0].Err)
}

func TestReloadNeverBlocks(t *testing.T) {
	uc, f := newChangesets()
	start(t, uc)
	_, err := uc.LoadChangeset(t.Context(), comparison(t, change.Commit("a"), change.Index()))
	require.NoError(t, err)
	release := make(chan struct{})
	defer close(release)
	f.changes.ListChangesCalls(func(context.Context, change.Comparison) ([]change.ChangedFile, error) {
		<-release
		return nil, nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			uc.Reload(change.Gen(i + 1))
		}
		uc.Drain()
	}()
	testsupport.Until(t, done)
}
