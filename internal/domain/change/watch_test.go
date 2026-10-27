package change_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
)

func TestWatchesWorktree(t *testing.T) {
	worktree, err := change.NewComparison(change.Commit("HEAD"), change.Worktree())
	require.NoError(t, err)
	index, err := change.NewComparison(change.Commit("HEAD"), change.Index())
	require.NoError(t, err)
	commits, err := change.NewComparison(change.Commit("a"), change.Commit("b"))
	require.NoError(t, err)

	got, err := change.WatchesWorktree(worktree)
	require.NoError(t, err)
	assert.True(t, got)
	got, err = change.WatchesWorktree(index)
	require.NoError(t, err)
	assert.False(t, got, "the index and HEAD only")
	_, err = change.WatchesWorktree(commits)
	require.ErrorIs(t, err, change.ErrNotWatchable)
	assert.Contains(t, err.Error(), "a and b are commits")

	files, err := change.NewComparison(change.File("a"), change.File("b"))
	require.NoError(t, err)
	_, err = change.WatchesWorktree(files)
	require.ErrorIs(t, err, change.ErrNotWatchable)
	assert.Contains(t, err.Error(), "files on disk")
}
