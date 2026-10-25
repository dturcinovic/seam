package git_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/adapter/driven/git"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// checkoutRepo is a repository with two commits of a.txt, and the
// directory that holds its checkouts.
type checkoutRepo struct {
	*testsupport.GitRepo
	first, second change.ObjectID
	store         string
}

func newCheckoutRepo(t *testing.T) checkoutRepo {
	t.Helper()
	r := checkoutRepo{GitRepo: testsupport.NewGitRepo(t), store: t.TempDir()}
	r.Write(".gitignore", "bin/\n")
	r.Write("a.txt", "one\n")
	r.Git("add", "-A")
	r.Git("commit", "-q", "-m", "first")
	r.first = blobID(t, r.GitRepo, "HEAD")
	r.Write("a.txt", "two\n")
	r.Git("commit", "-q", "-a", "-m", "second")
	r.second = blobID(t, r.GitRepo, "HEAD")
	return r
}

func (r checkoutRepo) checkouts() *git.Checkouts {
	return git.NewCheckouts(r.Dir, r.store, zap.NewNop(), nil)
}

// worktrees are the checkouts git knows, the repository's own first.
func (r checkoutRepo) worktrees() []string {
	var dirs []string
	for l := range strings.Lines(r.Git("worktree", "list", "--porcelain")) {
		if dir, ok := strings.CutPrefix(strings.TrimSuffix(l, "\n"), "worktree "); ok {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// files are all paths under the checkout store, relative to it.
func (r checkoutRepo) files(t *testing.T) []string {
	t.Helper()
	var paths []string
	require.NoError(t, filepath.WalkDir(r.store, func(p string, _ os.DirEntry, err error) error {
		if p != r.store {
			rel, _ := filepath.Rel(r.store, p)
			paths = append(paths, rel)
		}
		return err
	}))
	return paths
}

func TestACheckoutHasTheCommitsFilesAndGoesWithClose(t *testing.T) {
	r := newCheckoutRepo(t)
	c := r.checkouts()

	first, err := c.Checkout(t.Context(), r.first)
	require.NoError(t, err)
	second, err := c.Checkout(t.Context(), r.second)
	require.NoError(t, err)
	again, err := c.Checkout(t.Context(), r.first)
	require.NoError(t, err)

	assert.Equal(t, first, again, "a commit is checked out once")
	assert.FileExists(t, filepath.Join(filepath.Dir(first), ".seam-owner"))
	for dir, want := range map[string]string{first: "one\n", second: "two\n"} {
		got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	}
	assert.Len(t, r.worktrees(), 3)

	require.NoError(t, os.MkdirAll(filepath.Join(first, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(first, "bin", "generated"), []byte("x"), 0o600))
	require.NoError(t, c.Close(t.Context()))

	assert.Equal(t, []string{r.Dir}, r.worktrees())
	assert.Empty(t, r.files(t), "not even the repository's directory is left")
	_, err = c.Checkout(t.Context(), r.first)
	assert.ErrorIs(t, err, git.ErrClosed)
}

func TestAFailedCheckoutLeavesNothing(t *testing.T) {
	r := newCheckoutRepo(t)
	c := r.checkouts()
	missing, err := change.ParseObjectID(strings.Repeat("1", 40))
	require.NoError(t, err)

	_, err = c.Checkout(t.Context(), missing)

	require.Error(t, err)
	require.NoError(t, c.Close(t.Context()))
	assert.Equal(t, []string{r.Dir}, r.worktrees())
	assert.Empty(t, r.files(t))
}

// With these set, TestCheckoutCrashHelper is a seam that checks out a
// commit, writes into the checkout and is killed: a crash that leaves all
// behind.
const (
	crashRoot   = "SEAM_CRASH_ROOT"
	crashStore  = "SEAM_CRASH_STORE"
	crashCommit = "SEAM_CRASH_COMMIT"
)

func TestCheckoutCrashHelper(t *testing.T) {
	root, store := os.Getenv(crashRoot), os.Getenv(crashStore)
	if root == "" {
		t.Skip("only run as the crashing session of TestCleanRemovesTheCheckoutsOfACrashedSession")
	}
	id, err := change.ParseObjectID(os.Getenv(crashCommit))
	require.NoError(t, err)
	dir, err := git.NewCheckouts(root, store, zap.NewNop(), nil).Checkout(t.Context(), id)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin", "generated"), []byte("x"), 0o600))
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGKILL))
}

func TestCleanRemovesTheCheckoutsOfACrashedSession(t *testing.T) {
	r := newCheckoutRepo(t)
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestCheckoutCrashHelper$")
	cmd.Env = append(os.Environ(), crashRoot+"="+r.Dir, crashStore+"="+r.store, crashCommit+"="+r.first.String())
	out, err := cmd.CombinedOutput()
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	require.True(t, ok, "the helper is killed, not done: %v %s", err, out)
	require.Equal(t, syscall.SIGKILL, exitErr.Sys().(syscall.WaitStatus).Signal(), "%s", out)
	require.Len(t, r.worktrees(), 2, "the crash leaves its checkout")
	require.Contains(t, strings.Join(r.files(t), " "), filepath.Join("bin", "generated"))

	c := r.checkouts()
	require.NoError(t, c.Clean(t.Context()))
	require.NoError(t, c.Close(t.Context()))

	assert.Equal(t, []string{r.Dir}, r.worktrees())
	assert.Empty(t, r.files(t))
}

func TestCleanLeavesRunningSessionsAndForeignDirectories(t *testing.T) {
	r := newCheckoutRepo(t)
	running := r.checkouts()
	dir, err := running.Checkout(t.Context(), r.first)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, running.Close(t.Context())) })
	// A directory of the same shape without the marker, and the user's own
	// checkout of the repository.
	foreign := filepath.Join(filepath.Dir(filepath.Dir(dir)), "0123456789abcdef")
	require.NoError(t, os.MkdirAll(filepath.Join(foreign, r.second.String()), 0o700))
	own := filepath.Join(t.TempDir(), "own")
	r.Git("worktree", "add", "-q", "--detach", own, r.second.String())

	c := r.checkouts()
	require.NoError(t, c.Clean(t.Context()))
	require.NoError(t, c.Close(t.Context()))

	assert.FileExists(t, filepath.Join(dir, "a.txt"))
	assert.FileExists(t, filepath.Dir(dir)+".lock")
	assert.DirExists(t, filepath.Join(foreign, r.second.String()))
	assert.ElementsMatch(t, []string{r.Dir, dir, own}, r.worktrees())
}

func TestTheRepositorysDirectoryLeftEmptyIsRemoved(t *testing.T) {
	tests := map[string]func(*git.Checkouts, context.Context) error{
		"by Clean": (*git.Checkouts).Clean,
		// One whose session failed to open may have made it.
		"by Close without a session": (*git.Checkouts).Close,
	}
	for name, remove := range tests {
		t.Run(name, func(t *testing.T) {
			r := newCheckoutRepo(t)
			c := r.checkouts()
			dir, err := c.Checkout(t.Context(), r.first)
			require.NoError(t, err)
			require.NoError(t, c.Close(t.Context()))
			// As an older seam left it.
			require.NoError(t, os.Mkdir(filepath.Dir(filepath.Dir(dir)), 0o700))

			require.NoError(t, remove(r.checkouts(), t.Context()))

			assert.Empty(t, r.files(t))
		})
	}
}

func TestCheckoutsLeftByCloseKeepTheirMarkerForTheNextClean(t *testing.T) {
	r := newCheckoutRepo(t)
	c := r.checkouts()
	dir, err := c.Checkout(t.Context(), r.first)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_ = c.Close(ctx) // Out of time: what it could not remove is the point.

	require.DirExists(t, dir)
	require.FileExists(t, filepath.Join(filepath.Dir(dir), ".seam-owner"))
	next := r.checkouts()
	require.NoError(t, next.Clean(t.Context()))
	require.NoError(t, next.Close(t.Context()))
	assert.Equal(t, []string{r.Dir}, r.worktrees())
	assert.Empty(t, r.files(t))
}

func TestACheckoutRunsNoHooks(t *testing.T) {
	r := newCheckoutRepo(t)
	hook := filepath.Join(r.Dir, ".git", "hooks", "post-checkout")
	ran := filepath.Join(t.TempDir(), "ran")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+ran+"\n"), 0o700))
	c := r.checkouts()

	_, err := c.Checkout(t.Context(), r.first)

	require.NoError(t, err)
	require.NoError(t, c.Close(t.Context()))
	assert.NoFileExists(t, ran)
}

// TestACheckoutFromAGitHook: a pre-commit hook runs with GIT_INDEX_FILE
// set relative to the repository, which a new checkout must not take.
func TestACheckoutFromAGitHook(t *testing.T) {
	r := newCheckoutRepo(t)
	t.Setenv("GIT_INDEX_FILE", ".git/index")
	c := r.checkouts()

	_, err := c.Checkout(t.Context(), r.first)

	require.NoError(t, err)
	require.NoError(t, c.Close(t.Context()))
}
