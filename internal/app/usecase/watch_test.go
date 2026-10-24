package usecase_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/out"
	"seam/internal/app/port/out/outfakes"
	"seam/internal/app/usecase"
	"seam/internal/domain/change"
)

type watchFakes struct {
	paths   *outfakes.FakeWatchPathsReader
	ignores *outfakes.FakeIgnoreChecker
	watcher *outfakes.FakeChangeWatcher
	// listener is what the watcher was handed to report to.
	listener chan out.WatchListener
}

func testPaths() change.WatchPaths {
	return change.WatchPaths{Root: "/repo", GitDir: "/repo/.git", Ignored: []string{"bin/"}}
}

// newWatches watches until the test ends; its watcher runs until stopped
// and hands out its changed func.
func newWatches(t *testing.T) (*usecase.Watches, watchFakes) {
	t.Helper()
	f := watchFakes{&outfakes.FakeWatchPathsReader{}, &outfakes.FakeIgnoreChecker{}, &outfakes.FakeChangeWatcher{}, make(chan out.WatchListener, 1)}
	f.paths.WatchPathsReturns(testPaths(), nil)
	f.watcher.WatchCalls(func(ctx context.Context, _ change.WatchPaths, _ bool, l out.WatchListener) error {
		f.listener <- l
		<-ctx.Done()
		return nil
	})
	w := usecase.NewWatches(f.paths, f.ignores, f.watcher, nil)
	t.Cleanup(func() { assert.NoError(t, w.Stop(context.Background())) })
	return w, f
}

func kinds(us []change.WatchUpdate) []change.WatchKind {
	var out []change.WatchKind
	for _, u := range us {
		out = append(out, u.Kind)
	}
	return out
}

// started watches the worktree and returns the watcher's listener.
func started(t *testing.T, w *usecase.Watches, f watchFakes) out.WatchListener { //nolint:ireturn // the use case hands its listener out as the port's type only
	t.Helper()
	w.Watch(comparison(t, change.Commit("HEAD"), change.Worktree()))
	l := <-f.listener
	synctest.Wait()
	require.Equal(t, []change.WatchKind{change.Watching}, kinds(w.Drain()))
	return l
}

func TestABurstReportsOnceItSettles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		l := started(t, w, f)

		for range 3 {
			l.Changed()
			time.Sleep(100 * time.Millisecond)
		}
		time.Sleep(99 * time.Millisecond)
		synctest.Wait()
		assert.Empty(t, w.Drain(), "199 ms after the last change")

		time.Sleep(time.Millisecond)
		synctest.Wait()
		assert.Equal(t, []change.WatchKind{change.Changed}, kinds(w.Drain()))
		select {
		case <-w.Signal():
		default:
			t.Error("no signal")
		}
	})
}

func TestAChangeThatNeverSettlesReportsEverySecond(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		l := started(t, w, f)

		for range 25 {
			l.Changed()
			time.Sleep(100 * time.Millisecond)
		}
		synctest.Wait()

		assert.Equal(t, []change.WatchKind{change.Changed, change.Changed}, kinds(w.Drain()), "at 1 s and 2 s")
	})
}

func TestWhatIsWatched(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		w.Watch(comparison(t, change.Commit("HEAD"), change.Index()))
		<-f.listener
		synctest.Wait()

		require.Equal(t, 1, f.watcher.WatchCallCount())
		_, p, worktree, _ := f.watcher.WatchArgsForCall(0)
		assert.Equal(t, testPaths(), p)
		assert.False(t, worktree, "--staged watches the index and HEAD only")
	})
}

func TestTwoCommitsAreNotWatched(t *testing.T) {
	w, f := newWatches(t)
	w.Watch(comparison(t, change.Commit("a"), change.Commit("b")))

	us := w.Drain()
	require.Len(t, us, 1)
	assert.ErrorIs(t, us[0].Err, change.ErrNotWatchable)
	assert.Zero(t, f.paths.WatchPathsCallCount())
}

func TestAFailingWatcherSaysSo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		f.watcher.WatchReturns(errors.New("inotify watch limit reached"))

		w.Watch(comparison(t, change.Commit("HEAD"), change.Worktree()))
		synctest.Wait()

		us := w.Drain()
		require.Equal(t, []change.WatchKind{change.Watching, change.WatchFailed}, kinds(us))
		assert.EqualError(t, us[1].Err, "watch: inotify watch limit reached")
	})
}

func TestWithoutPathsNothingIsWatched(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		f.paths.WatchPathsReturns(change.WatchPaths{}, errors.New("not a git repository"))

		w.Watch(comparison(t, change.Commit("HEAD"), change.Worktree()))
		synctest.Wait()

		assert.Equal(t, []change.WatchKind{change.WatchFailed}, kinds(w.Drain()))
		assert.Zero(t, f.watcher.WatchCallCount())
	})
}

func TestAPanickingWatcherFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		f.watcher.WatchCalls(func(context.Context, change.WatchPaths, bool, out.WatchListener) error { panic("inotify") })

		w.Watch(comparison(t, change.Commit("HEAD"), change.Worktree()))
		synctest.Wait()

		us := w.Drain()
		require.Equal(t, []change.WatchKind{change.Watching, change.WatchFailed}, kinds(us))
		assert.ErrorIs(t, us[1].Err, change.ErrWatcherFailed)
	})
}

func TestStopEndsTheWatcherQuietly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		started(t, w, f)

		require.NoError(t, w.Stop(context.Background()))

		assert.Empty(t, w.Drain(), "stopping is no failure")
	})
}

func TestTheWatcherAsksGitWhatItIgnores(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w, f := newWatches(t)
		l := started(t, w, f)
		f.ignores.IgnoredReturns([]string{"/repo/node_modules"}, nil)

		assert.Equal(t, []string{"/repo/node_modules"}, l.Ignored([]string{"/repo/node_modules", "/repo/web"}))
		_, asked := f.ignores.IgnoredArgsForCall(0)
		assert.Equal(t, []string{"/repo/node_modules", "/repo/web"}, asked)

		f.ignores.IgnoredReturns(nil, errors.New("git gone"))
		assert.Empty(t, l.Ignored([]string{"/repo/x"}), "without an answer the watcher watches all")
	})
}
