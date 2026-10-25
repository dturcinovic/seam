package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/supervise"
)

// A burst of changes, such as a checkout or a formatter writing many files,
// reloads once it has been quiet for watchQuiet, but no later than
// watchMaxWait after its first change, so a writer that never stops cannot
// hold the view back.
const (
	watchQuiet   = 200 * time.Millisecond
	watchMaxWait = time.Second
)

// Watches implements in.WatchStarter and in.WatchUpdates.
type Watches struct {
	group   *supervise.Group
	paths   out.WatchPathsReader
	ignores out.IgnoreChecker
	watcher out.ChangeWatcher

	// changed carries the watcher's changes to the debouncer; one waiting
	// covers any number.
	changed chan struct{}
	signal  chan struct{}

	mu      sync.Mutex
	updates []change.WatchUpdate
}

// NewWatches returns the use case over its ports. It watches nothing until
// Watch.
func NewWatches(p out.WatchPathsReader, i out.IgnoreChecker, w out.ChangeWatcher, panics supervise.PanicFunc) *Watches {
	ws := &Watches{paths: p, ignores: i, watcher: w, changed: make(chan struct{}, 1), signal: make(chan struct{}, 1)}
	ws.group = supervise.NewGroup("watch", panics, func(f supervise.Failure) {
		ws.publish(change.WatchUpdate{Kind: change.WatchFailed, Err: fmt.Errorf("%w: %v", change.ErrWatcherFailed, f.Value)})
	})
	return ws
}

// Watch starts watching the right side of c; it is called once.
func (w *Watches) Watch(c change.Comparison) {
	worktree, err := change.WatchesWorktree(c)
	if err == nil {
		err = w.group.Go("debounce", w.debounce)
	}
	if err == nil {
		err = w.group.Go("watch", func(ctx context.Context) { w.watch(ctx, worktree) })
	}
	if err != nil {
		w.publish(change.WatchUpdate{Kind: change.WatchFailed, Err: err})
	}
}

// Stop ends watching and waits for it until ctx ends.
func (w *Watches) Stop(ctx context.Context) error {
	return w.group.Stop(ctx)
}

// Signal is set after each new update.
func (w *Watches) Signal() <-chan struct{} {
	return w.signal
}

// Drain hands over the updates since the last call, oldest first.
func (w *Watches) Drain() []change.WatchUpdate {
	w.mu.Lock()
	defer w.mu.Unlock()
	u := w.updates
	w.updates = nil
	return u
}

// watch runs the watcher until ctx ends or it fails.
func (w *Watches) watch(ctx context.Context, worktree bool) {
	p, err := w.paths.WatchPaths(ctx)
	if err == nil {
		w.publish(change.WatchUpdate{Kind: change.Watching})
		err = w.watcher.Watch(ctx, p, worktree, listener{ctx: ctx, w: w})
	}
	if err != nil && ctx.Err() == nil {
		w.publish(change.WatchUpdate{Kind: change.WatchFailed, Err: fmt.Errorf("watch: %w", err)})
	}
}

// debounce turns the watcher's changes into one Changed per burst. Its
// timers are nil between bursts, and a nil channel never fires.
func (w *Watches) debounce(ctx context.Context) {
	var quiet, limit <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.changed:
			quiet = time.After(watchQuiet)
			if limit == nil {
				limit = time.After(watchMaxWait)
			}
			continue
		case <-quiet:
		case <-limit:
		}
		quiet, limit = nil, nil
		w.publish(change.WatchUpdate{Kind: change.Changed})
	}
}

// listener is what the watcher tells, within the watch's ctx.
type listener struct {
	ctx context.Context //nolint:containedctx // the port's calls carry none, and they end with the watch
	w   *Watches
}

func (l listener) Changed() { notify(l.w.changed) }

// Ignored asks git; if git cannot say, the watcher watches them all, which
// costs at most reloads nobody needed.
func (l listener) Ignored(dirs []string) []string {
	ignored, err := l.w.ignores.Ignored(l.ctx, dirs)
	if err != nil {
		return nil
	}
	return ignored
}

func (w *Watches) publish(u change.WatchUpdate) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.updates = append(w.updates, u)
	notify(w.signal)
}
