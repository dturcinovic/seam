package usecase_test

// The load job of FileDiffs: one at a time, late results dropped, never
// blocking the caller, stopping within its deadline.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/limits"
	"seam/internal/supervise"
)

// blockOn makes the worktree reader wait for release on path, ignoring ctx
// as a reader that does not cooperate would. started is closed on entry.
func blockOn(f fileFakes, path string) (started, release chan struct{}) {
	started, release = make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.worktree.ReadWorktreeCalls(func(_ context.Context, p string, _ limits.FileSize) ([]byte, change.Stamp, error) {
		if p == path {
			once.Do(func() { close(started) })
			<-release
		}
		return f.repo.readWorktree(p)
	})
	return started, release
}

func TestALateResultIsDropped(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	a, b := f.modified(t, "a.go", "x\n", "y\n"), f.modified(t, "b.go", "x\n", "z\n")
	started, release := blockOn(f, "a.go")
	c := worktreeComparison(t)

	uc.Load(filediff.Request{Gen: 1, Comparison: c, File: a})
	until(t, started)
	uc.Load(filediff.Request{Gen: 2, Comparison: c, File: b})
	close(release)

	all := collect(t, uc, func(u []filediff.Update) bool { return len(u) > 0 })
	require.Len(t, all, 1)
	assert.Equal(t, filediff.Gen(2), all[0].Gen)
	assert.Equal(t, b, all[0].Snapshot.File)
}

func TestFreshCancelsTheRunningLoadOfTheSameFile(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	started := make(chan struct{})
	f.worktree.ReadWorktreeCalls(func(ctx context.Context, path string, _ limits.FileSize) ([]byte, change.Stamp, error) {
		if f.worktree.ReadWorktreeCallCount() == 1 {
			close(started)
			<-ctx.Done()
			return nil, change.Stamp{}, ctx.Err()
		}
		return f.repo.readWorktree(path)
	})
	c := worktreeComparison(t)

	uc.Load(filediff.Request{Gen: 1, Comparison: c, File: file})
	until(t, started)
	uc.Load(filediff.Request{Gen: 2, Comparison: c, File: file, Fresh: true})

	all := collect(t, uc, func(u []filediff.Update) bool { return len(u) > 0 })
	require.Len(t, all, 1)
	assert.Equal(t, filediff.Gen(2), all[0].Gen)
}

func TestABurstRunsOneLoadAtATime(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	var running atomic.Int32
	var overlap atomic.Bool
	f.worktree.ReadWorktreeCalls(func(_ context.Context, _ string, _ limits.FileSize) ([]byte, change.Stamp, error) {
		if running.Add(1) > 1 {
			overlap.Store(true)
		}
		defer running.Add(-1)
		time.Sleep(100 * time.Microsecond)
		return []byte("y\n"), change.Stamp{}, nil
	})
	c := worktreeComparison(t)
	var last change.ChangedFile
	for i := range 100 {
		last = f.modified(t, fmt.Sprintf("f%03d.go", i), "x\n", "y\n")
		uc.Load(filediff.Request{Gen: filediff.Gen(i + 1), Comparison: c, File: last})
	}

	all := collect(t, uc, func(u []filediff.Update) bool { return len(u) > 0 && u[len(u)-1].Gen == 100 })

	assert.False(t, overlap.Load(), "never two loads at once")
	assert.Equal(t, last, all[len(all)-1].Snapshot.File)
}

func TestLoadNeverBlocks(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	started, release := blockOn(f, "a.go")
	f.stats.StatWorktreeCalls(func(context.Context, string) (change.Stamp, error) {
		<-release
		return change.Stamp{}, nil
	})
	defer close(release)
	c := worktreeComparison(t)

	uc.Load(filediff.Request{Gen: 1, Comparison: c, File: file})
	until(t, started)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			uc.Load(filediff.Request{Gen: filediff.Gen(i + 2), Comparison: c, File: file})
		}
		uc.Drain()
	}()
	until(t, done)
}

func TestManyResultsSetTheSignalOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		uc, f := newFileDiffs(t)
		require.NoError(t, uc.Start())
		c := worktreeComparison(t)
		for i := range 10 {
			uc.Load(filediff.Request{Gen: filediff.Gen(i + 1), Comparison: c, File: f.modified(t, fmt.Sprintf("f%d.go", i), "x\n", "y\n")})
			synctest.Wait()
		}

		assert.Len(t, uc.Signal(), 1, "set, and once")
		<-uc.Signal()
		assert.Len(t, uc.Drain(), 10, "no update lost")
		assert.Empty(t, uc.Signal())
		require.NoError(t, uc.Stop(t.Context()))
	})
}

func TestAPanicFailsThisAndLaterLoads(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "x\n", "y\n")
	f.worktree.ReadWorktreeCalls(func(context.Context, string, limits.FileSize) ([]byte, change.Stamp, error) {
		panic("boom")
	})
	c := worktreeComparison(t)

	u := loadOne(t, uc, c, 1, file, false)
	require.ErrorIs(t, u.Err, filediff.ErrLoaderFailed)
	assert.ErrorContains(t, u.Err, "boom")

	u = loadOne(t, uc, c, 2, file, false)
	require.ErrorIs(t, u.Err, filediff.ErrLoaderFailed)
}

func TestStop(t *testing.T) {
	t.Run("a cooperating load ends with its context", func(t *testing.T) {
		uc, f := newFileDiffs(t)
		require.NoError(t, uc.Start())
		file := f.modified(t, "a.go", "x\n", "y\n")
		started := make(chan struct{})
		f.worktree.ReadWorktreeCalls(func(ctx context.Context, _ string, _ limits.FileSize) ([]byte, change.Stamp, error) {
			close(started)
			<-ctx.Done()
			return nil, change.Stamp{}, ctx.Err()
		})
		uc.Load(filediff.Request{Gen: 1, Comparison: worktreeComparison(t), File: file})
		until(t, started)

		ctx, cancel := context.WithTimeout(t.Context(), guard)
		defer cancel()
		require.NoError(t, uc.Stop(ctx))
		assert.Empty(t, uc.Drain(), "a cancelled load publishes nothing")
	})
	t.Run("a stuck load does not hold up the stop", func(t *testing.T) {
		uc, f := newFileDiffs(t)
		require.NoError(t, uc.Start())
		file := f.modified(t, "a.go", "x\n", "y\n")
		started, release := blockOn(f, "a.go")
		uc.Load(filediff.Request{Gen: 1, Comparison: worktreeComparison(t), File: file})
		until(t, started)

		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		var timeout *supervise.StopTimeout
		require.ErrorAs(t, uc.Stop(ctx), &timeout)
		assert.Equal(t, []string{"worker"}, timeout.Tasks)
		// goleak in TestMain checks that the worker ends once released.
		close(release)
	})
}
