package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
)

func TestWatchPaths(t *testing.T) {
	f := newFixture(t)
	f.Write(".gitignore", "*.log\nbin/\n")
	for _, dir := range []string{"bin/deep", "src"} {
		require.NoError(t, os.MkdirAll(filepath.Join(f.Dir, dir), 0o750))
	}
	f.Write("bin/seam", "binary")
	f.Write("bin/deep/x", "x")
	f.Write("src/kept.log", "ignored, but its directory is not")
	f.Write("bin/deep/y.log", "")

	p, err := f.open(t).WatchPaths(t.Context())

	require.NoError(t, err)
	gitDir, err := filepath.EvalSymlinks(filepath.Join(f.Dir, ".git"))
	require.NoError(t, err)
	assert.Equal(t, change.WatchPaths{Root: f.Dir, GitDir: gitDir, Ignored: []string{"bin/"}}, p)
}

func TestAFolderOfIgnoredFilesIsNoIgnoredFolder(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.Dir, "src"), 0o750))
	f.Write("src/kept.log", "")

	p, err := f.open(t).WatchPaths(t.Context())

	require.NoError(t, err)
	assert.Empty(t, p.Ignored)
}

func TestIgnored(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)
	dir := func(name string) string { return filepath.Join(f.Dir, name) }
	require.NoError(t, os.MkdirAll(dir("logs"), 0o750))

	got, err := repo.Ignored(t.Context(), []string{dir("x.log"), dir("logs"), dir("a.txt")})
	require.NoError(t, err)
	assert.Equal(t, []string{dir("x.log")}, got)

	got, err = repo.Ignored(t.Context(), []string{dir("a.txt")})
	require.NoError(t, err)
	assert.Empty(t, got, "none ignored")
	got, err = repo.Ignored(t.Context(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}
