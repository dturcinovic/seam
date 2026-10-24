package supervise_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"seam/internal/supervise"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// recorder collects what the group reports to its owner, in order.
type recorder struct {
	mu     sync.Mutex
	events []string
	failed []supervise.Failure
}

func (r *recorder) log(_ string, f supervise.Failure) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "log "+f.Task)
}

func (r *recorder) onFail(f supervise.Failure) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, "fail "+f.Task)
	r.failed = append(r.failed, f)
}

func (r *recorder) failures() []supervise.Failure {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.failed)
}

func (r *recorder) history() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

func TestPanicInTaskFailsTheGroup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := &recorder{}
		g := supervise.NewGroup("owner", rec.log, rec.onFail)

		cancelled := make(chan struct{})
		require.NoError(t, g.Go("waiter", func(ctx context.Context) {
			<-ctx.Done()
			close(cancelled)
		}))
		require.NoError(t, g.Go("boom", func(context.Context) { panic("kaputt") }))
		synctest.Wait()

		<-cancelled
		failures := rec.failures()
		require.Len(t, failures, 1)
		assert.Equal(t, "boom", failures[0].Task)
		assert.Equal(t, "kaputt", failures[0].Value)
		assert.Contains(t, string(failures[0].Stack), "group_test.go")
		assert.Equal(t, []string{"log boom", "fail boom"}, rec.history())

		assert.ErrorIs(t, g.Go("late", func(context.Context) {}), supervise.ErrFailed)
		_, err := g.AfterFunc(time.Second, "late", func() {})
		assert.ErrorIs(t, err, supervise.ErrFailed)
		assert.NoError(t, g.Stop(t.Context()))
	})
}

func TestOnlyTheFirstPanicReachesTheOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := &recorder{}
		g := supervise.NewGroup("owner", rec.log, rec.onFail)

		for _, name := range []string{"a", "b"} {
			require.NoError(t, g.Go(name, func(context.Context) { panic(name) }))
		}
		synctest.Wait()

		assert.Len(t, rec.failures(), 1)
		assert.Len(t, rec.history(), 3, "every panic is logged, onFail runs once")
		assert.NoError(t, g.Stop(t.Context()))
	})
}

func TestPanicInTimerCallbackFailsTheGroup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := &recorder{}
		g := supervise.NewGroup("owner", rec.log, rec.onFail)

		laterRan := false
		_, err := g.AfterFunc(time.Second, "boom", func() { panic("kaputt") })
		require.NoError(t, err)
		_, err = g.AfterFunc(2*time.Second, "later", func() { laterRan = true })
		require.NoError(t, err)

		time.Sleep(3 * time.Second)
		synctest.Wait()

		failures := rec.failures()
		require.Len(t, failures, 1)
		assert.Equal(t, "boom", failures[0].Task)
		assert.False(t, laterRan, "a failed group starts no further timer")
		assert.ErrorIs(t, g.Go("late", func(context.Context) {}), supervise.ErrFailed)
		assert.NoError(t, g.Stop(t.Context()))
	})
}

func TestStopPreventsPendingTimers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := supervise.NewGroup("owner", nil, nil)
		ran := false
		_, err := g.AfterFunc(time.Second, "pending", func() { ran = true })
		require.NoError(t, err)

		require.NoError(t, g.Stop(t.Context()))
		time.Sleep(2 * time.Second)
		synctest.Wait()

		assert.False(t, ran)
		_, err = g.AfterFunc(time.Second, "late", func() {})
		assert.ErrorIs(t, err, supervise.ErrStopped)
	})
}

func TestStopWaitsForRunningCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := supervise.NewGroup("owner", nil, nil)
		done := false
		_, err := g.AfterFunc(time.Second, "slow", func() {
			time.Sleep(time.Second)
			done = true
		})
		require.NoError(t, err)

		time.Sleep(1500 * time.Millisecond)
		require.NoError(t, g.Stop(t.Context()))
		assert.True(t, done)
	})
}

func TestTimerStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := supervise.NewGroup("owner", nil, nil)
		ran := false
		timer, err := g.AfterFunc(time.Second, "pending", func() { ran = true })
		require.NoError(t, err)

		assert.True(t, timer.Stop())
		assert.False(t, timer.Stop(), "a second Stop finds nothing to stop")
		time.Sleep(2 * time.Second)
		synctest.Wait()

		assert.False(t, ran)
		assert.NoError(t, g.Stop(t.Context()))
	})
}

func TestTimerStopAfterFiring(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := supervise.NewGroup("owner", nil, nil)
		timer, err := g.AfterFunc(time.Second, "quick", func() {})
		require.NoError(t, err)

		time.Sleep(2 * time.Second)
		synctest.Wait()

		assert.False(t, timer.Stop())
		assert.NoError(t, g.Stop(t.Context()))
	})
}

func TestStopEndsCooperativeTasksBeforeTheDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := supervise.NewGroup("owner", nil, nil)
		for _, name := range []string{"a", "b"} {
			require.NoError(t, g.Go(name, func(ctx context.Context) { <-ctx.Done() }))
		}

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		start := time.Now()
		require.NoError(t, g.Stop(ctx))
		assert.Zero(t, time.Since(start), "cooperative tasks return without waiting for the deadline")

		assert.ErrorIs(t, g.Go("late", func(context.Context) {}), supervise.ErrStopped)
	})
}

func TestStopReturnsOnTimeWithUncooperativeTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := supervise.NewGroup("owner", nil, nil)
		release := make(chan struct{})
		require.NoError(t, g.Go("stuck", func(context.Context) { <-release }))
		require.NoError(t, g.Go("polite", func(ctx context.Context) { <-ctx.Done() }))

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		start := time.Now()
		err := g.Stop(ctx)

		assert.Equal(t, time.Second, time.Since(start))
		timeout, ok := errors.AsType[*supervise.StopTimeout](err)
		require.True(t, ok, "got %v", err)
		assert.Equal(t, []string{"stuck"}, timeout.Tasks)
		assert.Contains(t, err.Error(), "stuck")

		// Stop does not claim to have ended the task; only releasing it does.
		close(release)
		synctest.Wait()
	})
}

func TestStopWithoutTasks(t *testing.T) {
	g := supervise.NewGroup("owner", nil, nil)
	require.NoError(t, g.Stop(t.Context()))
	require.NoError(t, g.Stop(t.Context()), "Stop may be called again")
}

func TestPanicDuringStopStillReachesTheOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rec := &recorder{}
		g := supervise.NewGroup("owner", rec.log, rec.onFail)
		require.NoError(t, g.Go("shutdown", func(ctx context.Context) {
			<-ctx.Done()
			panic("while stopping")
		}))

		require.NoError(t, g.Stop(t.Context()))

		assert.Len(t, rec.failures(), 1)
		assert.ErrorIs(t, g.Go("late", func(context.Context) {}), supervise.ErrFailed)
	})
}
