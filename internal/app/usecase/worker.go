package usecase

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/dturcinovic/seam/internal/supervise"
)

// worker runs the jobs of one use case on a single goroutine. A request
// replaces those still waiting, so a burst of requests leaves one job
// running and at most what the last one asked for waiting: one request
// from submit, or the list from want, run in order. Updates wait until drained; the signal is set
// after each one is published, never blocking. A use case embeds it for
// Start, Stop, Signal and Drain.
type worker[R, U any] struct {
	group *supervise.Group
	// failedErr is the domain error that answers every request after a panic:
	// the group refuses further work, so nothing else would answer them.
	failedErr error
	do        func(ctx context.Context, r R) U
	// supersedes reports that next makes the running job pointless, so it
	// is cancelled rather than finished.
	supersedes   func(running, next R) bool
	failedUpdate func(r R, err error) U

	wake   chan struct{}
	signal chan struct{}

	mu      sync.Mutex
	waiting []R
	running *R
	cancel  context.CancelFunc
	updates []U
	err     error
}

func newWorker[R, U any](owner string, failedErr error, panics supervise.PanicFunc, do func(context.Context, R) U,
	supersedes func(running, next R) bool, failedUpdate func(R, error) U,
) *worker[R, U] {
	w := &worker[R, U]{
		failedErr:    failedErr,
		do:           do,
		supersedes:   supersedes,
		failedUpdate: failedUpdate,
		wake:         make(chan struct{}, 1),
		signal:       make(chan struct{}, 1),
	}
	w.group = supervise.NewGroup(owner, panics, w.fail)
	return w
}

// Start starts the goroutine.
func (w *worker[R, U]) Start() error {
	return w.group.Go("worker", w.loop)
}

// Stop cancels the running job and waits for it until ctx ends.
func (w *worker[R, U]) Stop(ctx context.Context) error {
	return w.group.Stop(ctx)
}

// Signal is set after each new update.
func (w *worker[R, U]) Signal() <-chan struct{} {
	return w.signal
}

// Drain hands over the updates since the last call, oldest first.
func (w *worker[R, U]) Drain() []U {
	w.mu.Lock()
	defer w.mu.Unlock()
	u := w.updates
	w.updates = nil
	return u
}

func (w *worker[R, U]) submit(r R) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		w.publishLocked(w.failedUpdate(r, w.err))
		return
	}
	w.waiting = []R{r}
	if w.running != nil && w.supersedes(*w.running, r) {
		w.cancel()
	}
	notify(w.wake)
}

// want replaces the waiting requests with rs, to run in order. The running
// job goes on if rs holds a request same as it, which is then not run
// again; otherwise it is cancelled.
func (w *worker[R, U]) want(rs []R, same func(running, r R) bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		for _, r := range rs {
			w.publishLocked(w.failedUpdate(r, w.err))
		}
		return
	}
	// A copy, as the caller may reuse rs.
	w.waiting = slices.Clone(rs)
	if w.running != nil {
		running := *w.running
		w.waiting = slices.DeleteFunc(w.waiting, func(r R) bool { return same(running, r) })
		if len(w.waiting) == len(rs) {
			w.cancel()
		}
	}
	notify(w.wake)
}

// drop forgets the waiting request and cancels the running job where
// match says so; neither publishes anything.
func (w *worker[R, U]) drop(match func(R) bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.waiting = slices.DeleteFunc(w.waiting, match)
	if w.running != nil && match(*w.running) {
		w.cancel()
	}
}

// publish hands on an update that needs no job.
func (w *worker[R, U]) publish(u U) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.publishLocked(u)
}

func (w *worker[R, U]) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		}
		for w.runNext(ctx) {
		}
	}
}

// runNext runs the waiting job, if any. A job cancelled meanwhile publishes
// nothing: it was superseded, or the worker is stopping.
func (w *worker[R, U]) runNext(ctx context.Context) bool {
	w.mu.Lock()
	if len(w.waiting) == 0 {
		w.mu.Unlock()
		return false
	}
	r := w.waiting[0]
	jobCtx, cancel := context.WithCancel(ctx)
	w.waiting, w.running, w.cancel = w.waiting[1:], &r, cancel
	w.mu.Unlock()

	u := w.do(jobCtx, r)

	w.mu.Lock()
	defer w.mu.Unlock()
	if jobCtx.Err() == nil {
		w.publishLocked(u)
	}
	cancel()
	w.running, w.cancel = nil, nil
	return true
}

// fail answers the running and the waiting requests with the failure, and
// every later one, as the group runs no more jobs.
func (w *worker[R, U]) fail(f supervise.Failure) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = fmt.Errorf("%w: %v", w.failedErr, f.Value)
	if w.running != nil {
		w.publishLocked(w.failedUpdate(*w.running, w.err))
	}
	for _, r := range w.waiting {
		w.publishLocked(w.failedUpdate(r, w.err))
	}
	w.running, w.waiting = nil, nil
}

func (w *worker[R, U]) publishLocked(u U) {
	w.updates = append(w.updates, u)
	notify(w.signal)
}

// notify sets a signal of capacity 1 without blocking: one already set
// covers this event too, as the receiver drains everything.
func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
