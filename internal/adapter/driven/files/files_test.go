package files_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/files"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/limits"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// TestListPathsAsGitDifftoolHandsThem builds the two directories as git
// difftool -d does: old content on the left, links into the worktree on
// the right.
func TestListPathsAsGitDifftoolHandsThem(t *testing.T) {
	worktree, left, right := t.TempDir(), t.TempDir(), t.TempDir()
	for name, content := range map[string]string{"same.go": "a\n", "changed.go": "new\n", "sub/added.go": "x\n", "long.txt": strings.Repeat("b", 100<<10)} {
		write(t, filepath.Join(worktree, name), content)
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(right, name)), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(worktree, name), filepath.Join(right, name)))
	}
	write(t, filepath.Join(left, "same.go"), "a\n")
	write(t, filepath.Join(left, "changed.go"), "old\n")
	write(t, filepath.Join(left, "gone/deleted.go"), "y\n")
	write(t, filepath.Join(left, "long.txt"), strings.Repeat("b", 100<<10-1)+"c")
	require.NoError(t, syscall.Mkfifo(filepath.Join(right, "fifo"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(worktree, "missing"), filepath.Join(right, "dangling")))

	got, err := files.New(files.Config{}).ListPaths(t.Context(), left, right)
	require.NoError(t, err)

	assert.Equal(t, []change.ChangedFile{
		{Status: change.Modified, OldPath: "changed.go", NewPath: "changed.go"},
		{Status: change.Deleted, OldPath: "gone/deleted.go"},
		{Status: change.Modified, OldPath: "long.txt", NewPath: "long.txt"},
		{Status: change.Added, NewPath: "sub/added.go"},
	}, got, "same.go is the same through its link; the FIFO and the dangling link are left out")
}

func TestListPathsFollowsALinkedDirectory(t *testing.T) {
	real, right, links := t.TempDir(), t.TempDir(), t.TempDir()
	write(t, filepath.Join(real, "a.go"), "a\n")
	write(t, filepath.Join(right, "a.go"), "b\n")
	left := filepath.Join(links, "left")
	require.NoError(t, os.Symlink(real, left))

	got, err := files.New(files.Config{}).ListPaths(t.Context(), left, right)
	require.NoError(t, err)

	assert.Equal(t, []change.ChangedFile{{Status: change.Modified, OldPath: "a.go", NewPath: "a.go"}}, got)
}

func TestListPathsOfAMissingDirectoryFails(t *testing.T) {
	_, err := files.New(files.Config{}).ListPaths(t.Context(), filepath.Join(t.TempDir(), "missing"), t.TempDir())
	require.Error(t, err)
}

func TestReadPath(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.go"), "package a\n")
	require.NoError(t, os.Symlink(filepath.Join(dir, "a.go"), filepath.Join(dir, "link")))
	write(t, filepath.Join(dir, "big"), "0123456789")
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644))
	disk := files.New(files.Config{})

	for _, path := range []string{"a.go", "link"} {
		data, stamp, err := disk.ReadPath(t.Context(), filepath.Join(dir, path), limits.DefaultFileSize)
		require.NoError(t, err, path)
		assert.Equal(t, "package a\n", string(data), "%s: a link reads as its target", path)
		statted, err := disk.StatPath(t.Context(), filepath.Join(dir, path))
		require.NoError(t, err)
		assert.True(t, stamp.Equal(statted), "%s: the stamp is the target's", path)
	}
	_, err := disk.StatPath(t.Context(), filepath.Join(dir, "missing"))
	require.ErrorIs(t, err, change.ErrNotExist)
	_, _, err = disk.ReadPath(t.Context(), filepath.Join(dir, "missing"), limits.DefaultFileSize)
	require.ErrorIs(t, err, change.ErrNotExist)
	_, _, err = disk.ReadPath(t.Context(), filepath.Join(dir, "fifo"), limits.DefaultFileSize)
	require.ErrorIs(t, err, change.ErrNotRegular, "a FIFO is never opened")
	_, _, err = disk.ReadPath(t.Context(), dir, limits.DefaultFileSize)
	require.ErrorIs(t, err, change.ErrNotRegular)

	max, err := limits.ParseFileSize("9")
	require.NoError(t, err)
	_, _, err = disk.ReadPath(t.Context(), filepath.Join(dir, "big"), max)
	tooLarge, ok := errors.AsType[*change.TooLargeError](err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, int64(10), tooLarge.Size)
}

func TestLocateInWorktree(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	write(t, filepath.Join(root, "pkg/a.go"), "package pkg\n")
	write(t, filepath.Join(root, ".git/config"), "")
	write(t, filepath.Join(elsewhere, "b.go"), "package b\n")
	require.NoError(t, os.Symlink(filepath.Join(root, "pkg/a.go"), filepath.Join(elsewhere, "link.go")))
	disk := files.New(files.Config{Root: root})

	tests := []struct {
		name, path, rel string
		ok              bool
	}{
		{"in the worktree", filepath.Join(root, "pkg/a.go"), "pkg/a.go", true},
		{"a link into the worktree", filepath.Join(elsewhere, "link.go"), "pkg/a.go", true},
		{"outside", filepath.Join(elsewhere, "b.go"), "", false},
		{"git's own directory", filepath.Join(root, ".git/config"), "", false},
		{"missing", filepath.Join(root, "pkg/missing.go"), "", false},
	}
	for _, tt := range tests {
		rel, ok := disk.LocateInWorktree(tt.path)
		assert.Equal(t, tt.ok, ok, tt.name)
		assert.Equal(t, tt.rel, rel, tt.name)
	}

	t.Chdir(root)
	rel, ok := disk.LocateInWorktree("pkg/a.go")
	assert.True(t, ok, "a path relative to the working directory, as git difftool passes it")
	assert.Equal(t, "pkg/a.go", rel)

	_, ok = files.New(files.Config{}).LocateInWorktree(filepath.Join(root, "pkg/a.go"))
	assert.False(t, ok, "outside a repository nothing is in a worktree")
}
