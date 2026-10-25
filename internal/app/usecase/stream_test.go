package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/usecase"
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/limits"
	"seam/internal/testsupport"
)

// newStreams runs a stream loader over its own FileDiffs, whose worker
// stays idle unless a test loads through it.
func newStreams(t *testing.T) (*usecase.Streams, *usecase.FileDiffs, fileFakes) {
	t.Helper()
	files, f := newFileDiffs(t)
	start(t, files)
	s := usecase.NewStreams(files, nil)
	start(t, s)
	return s, files, f
}

func requests(gen filediff.Gen, c change.Comparison, files ...change.ChangedFile) []filediff.Request {
	rs := make([]filediff.Request, len(files))
	for i, file := range files {
		rs[i] = filediff.Request{Gen: gen, Comparison: c, File: file}
	}
	return rs
}

func loaded(all []filediff.Update) []string {
	var paths []string
	for _, u := range all {
		paths = append(paths, u.Snapshot.File.Path())
	}
	return paths
}

// readsOf counts the worktree reads of path.
func readsOf(f fileFakes, path string) int {
	n := 0
	for i := range f.worktree.ReadWorktreeCallCount() {
		if _, p, _ := f.worktree.ReadWorktreeArgsForCall(i); p == path {
			n++
		}
	}
	return n
}

func TestTheStreamLoadsInTheOrderAskedFor(t *testing.T) {
	s, _, f := newStreams(t)
	c := worktreeComparison(t)
	a, b, d := f.modified(t, "a.go", "x\n", "y\n"), f.modified(t, "b.go", "x\n", "y\n"), f.modified(t, "d.go", "x\n", "y\n")

	s.Want(requests(1, c, d, a, b))

	all := testsupport.Collect(t, s, func(u []filediff.Update) bool { return len(u) == 3 })
	assert.Equal(t, []string{"d.go", "a.go", "b.go"}, loaded(all))
}

func TestANewWantKeepsLoadingAFileStillWanted(t *testing.T) {
	s, _, f := newStreams(t)
	c := worktreeComparison(t)
	a, b, d := f.modified(t, "a.go", "x\n", "y\n"), f.modified(t, "b.go", "x\n", "y\n"), f.modified(t, "d.go", "x\n", "y\n")
	started, release := blockOn(f, "a.go")

	s.Want(requests(1, c, a, b))
	testsupport.Until(t, started)
	s.Want(requests(1, c, d, a))
	close(release)

	all := testsupport.Collect(t, s, func(u []filediff.Update) bool { return len(u) == 2 })
	assert.Equal(t, []string{"a.go", "d.go"}, loaded(all), "a goes on, b is no longer wanted")
	assert.Equal(t, 1, readsOf(f, "a.go"), "a is read once")
}

func TestANewWantCancelsAFileNoLongerWanted(t *testing.T) {
	s, _, f := newStreams(t)
	c := worktreeComparison(t)
	a, b := f.modified(t, "a.go", "x\n", "y\n"), f.modified(t, "b.go", "x\n", "y\n")
	started := make(chan struct{})
	f.worktree.ReadWorktreeCalls(func(ctx context.Context, path string, _ limits.FileSize) ([]byte, change.Stamp, error) {
		if path == "a.go" {
			close(started)
			<-ctx.Done()
			return nil, change.Stamp{}, ctx.Err()
		}
		return f.repo.readWorktree(path)
	})

	s.Want(requests(1, c, a))
	testsupport.Until(t, started)
	s.Want(requests(1, c, b))

	all := testsupport.Collect(t, s, func(u []filediff.Update) bool { return len(u) > 0 })
	assert.Equal(t, []string{"b.go"}, loaded(all))
}

func TestANewGenerationReadsAWantedFileAgain(t *testing.T) {
	s, _, f := newStreams(t)
	c := worktreeComparison(t)
	a := f.modified(t, "g.go", "x\n", "y\n")
	started, release := blockOn(f, "g.go")

	s.Want(requests(1, c, a))
	testsupport.Until(t, started)
	fresh := requests(2, c, a)
	fresh[0].Fresh = true
	s.Want(fresh)
	close(release)

	all := testsupport.Collect(t, s, func(u []filediff.Update) bool { return len(u) > 0 && u[len(u)-1].Gen == 2 })
	assert.Equal(t, filediff.Gen(2), all[len(all)-1].Gen)
	assert.Equal(t, 2, readsOf(f, "g.go"))
}

func TestTheStreamSharesTheCacheOfTheSingleFile(t *testing.T) {
	s, files, f := newStreams(t)
	c := worktreeComparison(t)
	a := f.modified(t, "a.go", "x\n", "y\n")
	loadOne(t, files, c, 1, a, false)

	s.Want(requests(2, c, a))

	all := testsupport.Collect(t, s, func(u []filediff.Update) bool { return len(u) > 0 })
	require.NoError(t, all[0].Err)
	assert.Equal(t, 1, readsOf(f, "a.go"), "read once, by the single file")
}

func TestAPanicFailsEveryWantedFile(t *testing.T) {
	s, _, f := newStreams(t)
	c := worktreeComparison(t)
	a, b := f.modified(t, "a.go", "x\n", "y\n"), f.modified(t, "b.go", "x\n", "y\n")
	f.worktree.ReadWorktreeCalls(func(context.Context, string, limits.FileSize) ([]byte, change.Stamp, error) {
		panic("boom")
	})

	s.Want(requests(1, c, a, b))
	all := testsupport.Collect(t, s, func(u []filediff.Update) bool { return len(u) == 2 })
	for _, u := range all {
		require.ErrorIs(t, u.Err, filediff.ErrLoaderFailed)
	}

	s.Want(requests(2, c, a))
	all = testsupport.Collect(t, s, func(u []filediff.Update) bool { return len(u) > 0 })
	require.ErrorIs(t, all[0].Err, filediff.ErrLoaderFailed, "and every later one")
}
