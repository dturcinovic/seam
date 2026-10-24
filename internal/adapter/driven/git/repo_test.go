package git_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/adapter/driven/git"
	"seam/internal/domain/change"
	"seam/internal/domain/limits"
)

// load resolves the comparison's commits and lists what git diffs. Adding
// the untracked files is the use case's business, tested there.
func load(t *testing.T, repo *git.Repo, left, right change.Source) []change.ChangedFile {
	t.Helper()
	resolve := func(s change.Source) change.Source {
		if s.Kind() != change.KindCommit {
			return s
		}
		id, err := repo.ResolveCommit(t.Context(), s.Rev())
		require.NoError(t, err)
		resolved, err := s.Resolved(id)
		require.NoError(t, err)
		return resolved
	}
	c, err := change.NewComparison(resolve(left), resolve(right))
	require.NoError(t, err)
	files, err := repo.ListChanges(t.Context(), c)
	require.NoError(t, err)
	return files
}

// sides reads both sides of f the way the right source dictates.
func sides(t *testing.T, repo *git.Repo, f change.ChangedFile, right change.SourceKind) (old, new string) {
	t.Helper()
	if !f.OldID.IsZero() {
		data, err := repo.ReadBlob(t.Context(), f.OldID, limits.DefaultFileSize)
		require.NoError(t, err)
		old = string(data)
	}
	switch {
	case f.NewPath == "":
	case right == change.KindWorktree:
		data, _, err := repo.ReadWorktree(t.Context(), f.NewPath, limits.DefaultFileSize)
		require.NoError(t, err)
		new = string(data)
	default:
		data, err := repo.ReadBlob(t.Context(), f.NewID, limits.DefaultFileSize)
		require.NoError(t, err)
		new = string(data)
	}
	return old, new
}

func TestModes(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)

	type want struct {
		status   change.Status
		old, new string
	}
	staged := map[string]want{
		"a.txt":        {change.Modified, "HEAD\n", "INDEX\n"},
		"added.txt":    {change.Added, "", "added\n"},
		"gone.txt":     {change.Deleted, "bye\n", ""},
		"new-name.txt": {change.Renamed, renamedContent, renamedContent},
		nlName:         {change.Modified, "nl HEAD\n", "nl index\n"},
	}
	worktree := map[string]want{
		"a.txt":        {change.Modified, "HEAD\n", "WORK\n"},
		"added.txt":    {change.Added, "", "added\n"},
		"gone.txt":     {change.Deleted, "bye\n", ""},
		"new-name.txt": {change.Renamed, renamedContent, renamedContent},
		nlName:         {change.Modified, "nl HEAD\n", "nl work\n"},
	}
	fromFirst := map[string]want{
		"a.txt":        {change.Modified, "one\n", "WORK\n"},
		"added.txt":    {change.Added, "", "added\n"},
		"bin.dat":      {change.Modified, "\x00\x01", "\x00\x02"},
		"gone.txt":     {change.Deleted, "bye\n", ""},
		"new-name.txt": {change.Renamed, renamedContent, renamedContent},
		nlName:         {change.Modified, "nl one\n", "nl work\n"},
	}
	commits := map[string]want{
		"a.txt":   {change.Modified, "one\n", "HEAD\n"},
		"bin.dat": {change.Modified, "\x00\x01", "\x00\x02"},
		nlName:    {change.Modified, "nl one\n", "nl HEAD\n"},
	}

	tests := []struct {
		name        string
		left, right change.Source
		want        map[string]want
	}{
		{"seam", change.Commit("HEAD"), change.Worktree(), worktree},
		{"seam <rev>", change.Commit(f.first), change.Worktree(), fromFirst},
		{"seam <a> <b>", change.Commit(f.first), change.Commit("HEAD"), commits},
		{"seam --staged", change.Commit("HEAD"), change.Index(), staged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]want{}
			for _, file := range load(t, repo, tt.left, tt.right) {
				old, new := sides(t, repo, file, tt.right.Kind())
				got[file.Path()] = want{file.Status, old, new}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenameKeepsBothPaths(t *testing.T) {
	repo := newFixture(t).open(t)

	files := load(t, repo, change.Commit("HEAD"), change.Index())
	i := slices.IndexFunc(files, func(f change.ChangedFile) bool { return f.Status == change.Renamed })
	require.NotEqual(t, -1, i, "no rename listed")
	assert.Equal(t, "old-name.txt", files[i].OldPath)
	assert.Equal(t, "new-name.txt", files[i].NewPath)
}

func TestListUntracked(t *testing.T) {
	repo := newFixture(t).open(t)

	untracked, err := repo.ListUntracked(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"untracked.txt"}, untracked, "neither tracked nor ignored files")
}

func TestBinaryContentIsReadAsIs(t *testing.T) {
	repo := newFixture(t).open(t)

	data, _, err := repo.ReadWorktree(t.Context(), "bin.dat", limits.DefaultFileSize)
	require.NoError(t, err)
	assert.True(t, change.IsBinary(data))
}

func TestNotARepository(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())

	_, err := git.FindRoot(t.Context(), t.TempDir())

	assert.ErrorIs(t, err, change.ErrNoRepository)
}

func TestFindRootFromASubdirectory(t *testing.T) {
	f := newFixture(t)
	sub := filepath.Join(f.Dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o700))

	root, err := git.FindRoot(t.Context(), sub)
	require.NoError(t, err)
	assert.Equal(t, f.Dir, root)
}

func TestFindRootPassesOtherFailuresOn(t *testing.T) {
	_, err := git.FindRoot(t.Context(), filepath.Join(t.TempDir(), "missing"))

	require.Error(t, err)
	assert.NotErrorIs(t, err, change.ErrNoRepository, "a missing directory is no question of git")
}

func TestUnknownRevision(t *testing.T) {
	repo := newFixture(t).open(t)

	for _, rev := range []string{"no-such-branch", "--output=x", "HEAD:a.txt"} {
		_, err := repo.ResolveCommit(t.Context(), rev)
		assert.ErrorIs(t, err, change.ErrUnknownRevision, rev)
		assert.EqualError(t, err, `unknown revision "`+rev+`"`)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := repo.ResolveCommit(ctx, "HEAD")
	assert.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, change.ErrUnknownRevision, "a cancelled call is no unknown revision")
}

func TestListChangesNeedsResolvedCommits(t *testing.T) {
	repo := newFixture(t).open(t)
	c, err := change.NewComparison(change.Commit("HEAD"), change.Worktree())
	require.NoError(t, err)

	_, err = repo.ListChanges(t.Context(), c)
	assert.ErrorContains(t, err, "not resolved")
}
