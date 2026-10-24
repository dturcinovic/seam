package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"seam/internal/app/port/in"
	"seam/internal/config"
	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/limits"
	"seam/internal/observability"
	"seam/internal/testsupport"
)

// loadNew loads file through the port and returns the text of its new side.
func loadNew(t *testing.T, files in.FileDiffLoader, r filediff.Request) string {
	t.Helper()
	files.Load(r)
	all := testsupport.Collect(t, files, func(u []filediff.Update) bool { return len(u) > 0 && u[len(u)-1].Gen == r.Gen })
	u := all[len(all)-1]
	require.NoError(t, u.Err)
	return diff.Content(u.Snapshot.Diff.New.Lines[0])
}

// TestFreshReadsAChangeTheStampHides is what r relies on: an edit that
// keeps size and mtime is invisible to the cache, and a fresh load sees it.
func TestFreshReadsAChangeTheStampHides(t *testing.T) {
	repo := testsupport.NewGitRepo(t)
	repo.Write("a.txt", "one\n")
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	repo.Write("a.txt", "two\n")
	path := filepath.Join(repo.Dir, "a.txt")
	info, err := os.Stat(path)
	require.NoError(t, err)

	cfg := config.Config{Dir: repo.Dir, LogFile: filepath.Join(t.TempDir(), "seam.log"), MaxFileSize: limits.DefaultFileSize}
	logger, err := observability.NewLogger(cfg)
	require.NoError(t, err)
	defer func() { assert.NoError(t, logger.Close()) }()
	var (
		loader in.ChangesetLoader
		files  in.FileDiffLoader
	)
	app := fx.New(append(options(cfg, logger, repo.Dir), fx.Populate(&loader, &files))...)
	require.NoError(t, app.Start(t.Context()))
	defer func() { assert.NoError(t, stopGraph(app)) }()
	c, err := change.NewComparison(change.Commit("HEAD"), change.Worktree())
	require.NoError(t, err)
	cs, err := loader.LoadChangeset(t.Context(), c)
	require.NoError(t, err)
	require.Len(t, cs.Files, 1)
	r := filediff.Request{Gen: 1, Comparison: cs.Comparison, File: cs.Files[0]}
	require.Equal(t, "two", loadNew(t, files, r))

	require.NoError(t, os.WriteFile(path, []byte("six\n"), 0o644))
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))

	r.Gen = 2
	assert.Equal(t, "two", loadNew(t, files, r), "the same stamp is trusted")
	r.Gen, r.Fresh = 3, true
	assert.Equal(t, "six", loadNew(t, files, r), "r reads it anyway")
}
