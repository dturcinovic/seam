package git_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/limits"
	"seam/internal/testsupport"
)

func size(t *testing.T, n int64) limits.FileSize {
	t.Helper()
	s, err := limits.NewFileSize(n)
	require.NoError(t, err)
	return s
}

func blobID(t *testing.T, f *testsupport.GitRepo, spec string) change.ObjectID {
	t.Helper()
	id, err := change.ParseObjectID(strings.TrimSpace(f.Git("rev-parse", spec)))
	require.NoError(t, err)
	return id
}

func TestBlobOverTheLimitIsNotTransferred(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)
	big := blobID(t, f.GitRepo, "HEAD:old-name.txt")

	_, err := repo.ReadBlob(t.Context(), big, size(t, 10))

	tooLarge, ok := errors.AsType[*change.TooLargeError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, int64(len(renamedContent)), tooLarge.Size)

	// Had cat-file sent the content, it would still sit in the pipe and
	// garble the next answer.
	data, err := repo.ReadBlob(t.Context(), blobID(t, f.GitRepo, "HEAD:a.txt"), limits.DefaultFileSize)
	require.NoError(t, err)
	assert.Equal(t, "HEAD\n", string(data))
}

func TestBlobAtTheLimitIsRead(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)

	data, err := repo.ReadBlob(t.Context(), blobID(t, f.GitRepo, "HEAD:a.txt"), size(t, 5))
	require.NoError(t, err)
	assert.Equal(t, "HEAD\n", string(data))
}

func TestObjectsThatAreNoBlob(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)

	_, err := repo.ReadBlob(t.Context(), blobID(t, f.GitRepo, "HEAD^{tree}"), limits.DefaultFileSize)
	assert.ErrorIs(t, err, change.ErrNotRegular)

	missing, err := change.ParseObjectID(strings.Repeat("1", 40))
	require.NoError(t, err)
	_, err = repo.ReadBlob(t.Context(), missing, limits.DefaultFileSize)
	assert.ErrorIs(t, err, change.ErrNotExist)

	_, err = repo.ReadBlob(t.Context(), change.ObjectID{}, limits.DefaultFileSize)
	assert.ErrorIs(t, err, change.ErrNotExist, "the missing side of an added file")
	data, err := repo.ReadBlob(t.Context(), blobID(t, f.GitRepo, "HEAD:a.txt"), limits.DefaultFileSize)
	require.NoError(t, err)
	assert.Equal(t, "HEAD\n", string(data), "and cat-file is still in step")
}

func TestWorktreeFileOverTheLimit(t *testing.T) {
	repo := newFixture(t).open(t)

	_, _, err := repo.ReadWorktree(t.Context(), "old-name.txt", size(t, 10))
	assert.ErrorIs(t, err, change.ErrNotExist, "renamed in the index and gone from the worktree")

	_, _, err = repo.ReadWorktree(t.Context(), "new-name.txt", size(t, 10))
	tooLarge, ok := errors.AsType[*change.TooLargeError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, int64(len(renamedContent)), tooLarge.Size)
}

// withTimeout fails the test instead of hanging when read blocks.
func withTimeout(t *testing.T, read func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- read() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("read blocked")
		return nil
	}
}

func TestSpecialFilesAreNotRead(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(f.Dir, "fifo"), 0o600))
	// A socket path is limited to about 100 bytes, which a temp dir may
	// exceed; the listener sits in the worktree under a short name.
	t.Chdir(f.Dir)
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", "sock")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	require.NoError(t, os.Mkdir(filepath.Join(f.Dir, "dir"), 0o700))

	for _, name := range []string{"fifo", "sock", "dir"} {
		err := withTimeout(t, func() error {
			_, _, err := repo.ReadWorktree(t.Context(), name, limits.DefaultFileSize)
			return err
		})
		assert.ErrorIs(t, err, change.ErrNotRegular, name)
	}
}

func TestSymlinkIsReadAsItsTarget(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)
	require.NoError(t, os.Symlink("a.txt", filepath.Join(f.Dir, "link")))

	data, stamp, err := repo.ReadWorktree(t.Context(), "link", limits.DefaultFileSize)
	require.NoError(t, err)
	assert.Equal(t, "a.txt", string(data))
	assert.Equal(t, int64(len("a.txt")), stamp.Size)

	_, _, err = repo.ReadWorktree(t.Context(), "link", size(t, 2))
	assert.ErrorAs(t, err, new(*change.TooLargeError))
}

func TestPathsCannotLeaveTheWorktree(t *testing.T) {
	repo := newFixture(t).open(t)

	_, _, err := repo.ReadWorktree(t.Context(), "../outside", limits.DefaultFileSize)
	assert.Error(t, err)
}

func TestStampsFollowTheFile(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)

	_, read, err := repo.ReadWorktree(t.Context(), "a.txt", limits.DefaultFileSize)
	require.NoError(t, err)
	stat, err := repo.StatWorktree(t.Context(), "a.txt")
	require.NoError(t, err)
	assert.Equal(t, read, stat, "reading and stat-ing agree")
	assert.NotZero(t, stat.Inode)

	f.Write("a.txt", "changed and longer\n")
	changed, err := repo.StatWorktree(t.Context(), "a.txt")
	require.NoError(t, err)
	assert.NotEqual(t, stat, changed)

	_, err = repo.StatWorktree(t.Context(), "missing")
	assert.ErrorIs(t, err, change.ErrNotExist)
}

func TestCancelledContext(t *testing.T) {
	f := newFixture(t)
	repo := f.open(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := repo.ReadBlob(ctx, blobID(t, f.GitRepo, "HEAD:a.txt"), limits.DefaultFileSize)
	assert.ErrorIs(t, err, ctx.Err())
	_, _, err = repo.ReadWorktree(ctx, "a.txt", limits.DefaultFileSize)
	assert.ErrorIs(t, err, ctx.Err())
	_, err = repo.StatWorktree(ctx, "a.txt")
	assert.ErrorIs(t, err, ctx.Err())
}
