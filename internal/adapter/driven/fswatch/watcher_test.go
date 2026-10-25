package fswatch_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/adapter/driven/fswatch"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/testsupport"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// quiet is how long no change must come to count as none: inotify reports
// within milliseconds.
const quiet = 300 * time.Millisecond

// listener counts changes and says git ignores every directory named
// node_modules.
type listener struct {
	changes chan struct{}
	asked   chan []string
}

func newListener() listener {
	return listener{changes: make(chan struct{}, 1024), asked: make(chan []string, 1024)}
}

func (l listener) Changed() {
	select {
	case l.changes <- struct{}{}:
	default:
	}
}

func (l listener) Ignored(dirs []string) []string {
	l.asked <- dirs
	var ignored []string
	for _, d := range dirs {
		if filepath.Base(d) == "node_modules" {
			ignored = append(ignored, d)
		}
	}
	return ignored
}

type watched struct {
	repo *testsupport.GitRepo
	listener
}

// watch watches a repository with a commit, bin/ ignored and holding a
// file, until the test ends; it returns once the watches are in place.
func watch(t *testing.T, worktree bool) watched {
	t.Helper()
	repo := testsupport.NewGitRepo(t)
	repo.Write(".gitignore", "bin/\n")
	repo.Write("a.txt", "one\n")
	require.NoError(t, os.Mkdir(filepath.Join(repo.Dir, "bin"), 0o750))
	repo.Write("bin/seam", "binary\n")
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	w := watched{repo: repo, listener: newListener()}
	p := change.WatchPaths{Root: repo.Dir, GitDir: filepath.Join(repo.Dir, ".git"), Ignored: []string{"bin/"}}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- fswatch.New(zap.NewNop()).Watch(ctx, p, worktree, w.listener)
	}()
	t.Cleanup(func() {
		cancel()
		assert.NoError(t, <-done)
	})
	// The watches are in place once a rewrite of HEAD comes through.
	require.Eventually(t, func() bool {
		repo.Git("symbolic-ref", "HEAD", "refs/heads/main")
		return w.drained()
	}, testsupport.Guard, 20*time.Millisecond)
	w.settle()
	return w
}

// drained empties the changes and says whether there were any.
func (w watched) drained() bool {
	some := false
	for {
		select {
		case <-w.changes:
			some = true
		default:
			return some
		}
	}
}

// settle waits until no change has come for quiet.
func (w watched) settle() {
	for {
		select {
		case <-w.changes:
		case <-time.After(quiet):
			return
		}
	}
}

// changesOn says whether a change comes within quiet after do.
func (w watched) changesOn(t *testing.T, do func()) bool {
	t.Helper()
	w.settle()
	do()
	select {
	case <-w.changes:
		w.settle()
		return true
	case <-time.After(quiet):
		return false
	}
}

func TestTheWorktreeAndGitAreWatched(t *testing.T) {
	w := watch(t, true)
	r := w.repo

	assert.True(t, w.changesOn(t, func() { r.Write("a.txt", "two\n") }), "a file written")
	assert.True(t, w.changesOn(t, func() {
		require.NoError(t, os.Mkdir(filepath.Join(r.Dir, "new"), 0o750))
	}), "a directory made")
	assert.True(t, w.changesOn(t, func() { r.Write("new/b.txt", "b\n") }), "a file in the new directory")
	assert.True(t, w.changesOn(t, func() { r.Git("add", ".") }), "git add")
	assert.True(t, w.changesOn(t, func() { r.Git("commit", "-q", "-m", "second") }), "git commit")
	assert.True(t, w.changesOn(t, func() { r.Git("checkout", "-q", "-b", "other") }), "git checkout")
	assert.False(t, w.changesOn(t, func() { r.Write("bin/seam", "rebuilt\n") }), "an ignored directory")
	assert.True(t, w.changesOn(t, func() { require.NoError(t, os.Remove(filepath.Join(r.Dir, "a.txt"))) }), "a file removed")
}

func TestADirectoryIgnoredAfterTheStartIsLeftOut(t *testing.T) {
	w := watch(t, true)
	r := w.repo
	for len(w.asked) > 0 {
		<-w.asked
	}

	assert.False(t, w.changesOn(t, func() {
		require.NoError(t, os.MkdirAll(filepath.Join(r.Dir, "node_modules/left-pad"), 0o750))
	}), "an ignored directory made")
	assert.Equal(t, []string{filepath.Join(r.Dir, "node_modules")}, <-w.asked, "asked about the tree alone")
	assert.False(t, w.changesOn(t, func() { r.Write("node_modules/left-pad/index.js", "x\n") }), "a file in it")

	assert.True(t, w.changesOn(t, func() {
		require.NoError(t, os.MkdirAll(filepath.Join(r.Dir, "web/node_modules/x"), 0o750))
	}), "a new directory holding an ignored one")
	assert.False(t, w.changesOn(t, func() { r.Write("web/node_modules/x/y.js", "y\n") }), "a file in the ignored one")
	assert.True(t, w.changesOn(t, func() { r.Write("web/page.html", "p\n") }), "a file beside it")
}

func TestABranchThatMovesAloneIsSeen(t *testing.T) {
	w := watch(t, false)
	r := w.repo
	r.Git("commit", "-q", "--allow-empty", "-m", "second")
	w.settle()

	assert.True(t, w.changesOn(t, func() { r.Git("update-ref", "-m", "back", "refs/heads/main", "HEAD~1") }), "only the branch moved")
}

func TestTheIndexAloneWatchesGitOnly(t *testing.T) {
	w := watch(t, false)
	r := w.repo

	assert.False(t, w.changesOn(t, func() { r.Write("a.txt", "two\n") }), "the worktree")
	assert.True(t, w.changesOn(t, func() { r.Git("add", "a.txt") }), "git add")
	assert.True(t, w.changesOn(t, func() { r.Git("commit", "-q", "-m", "second") }), "git commit")
}

func TestAMissingGitDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	p := change.WatchPaths{Root: dir, GitDir: filepath.Join(dir, "missing")}

	err := fswatch.New(zap.NewNop()).Watch(t.Context(), p, true, newListener())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "watch "+p.GitDir)
}

func TestTheWatcherReportsOnceItIsSetUp(t *testing.T) {
	repo := testsupport.NewGitRepo(t)
	p := change.WatchPaths{Root: repo.Dir, GitDir: filepath.Join(repo.Dir, ".git")}
	ctx, cancel := context.WithCancel(context.Background())
	l, done := newListener(), make(chan error, 1)
	go func() { done <- fswatch.New(zap.NewNop()).Watch(ctx, p, true, l) }()

	select {
	case <-l.changes:
	case <-time.After(testsupport.Guard):
		t.Error("no change reported after the setup")
	}
	cancel()
	assert.NoError(t, <-done)
}
