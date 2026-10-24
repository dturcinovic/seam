package supervise

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"
)

// ErrFailed is returned by Go and AfterFunc once a task of the group has
// panicked: the owner has been told through onFail and is winding down.
var ErrFailed = errors.New("supervise: group failed")

// ErrStopped is returned by Go and AfterFunc after Stop.
var ErrStopped = errors.New("supervise: group stopped")

// Failure describes a panic caught in one of the group's tasks.
type Failure struct {
	Task  string
	Value any
	Stack []byte
}

// PanicFunc records a caught panic. It runs before onFail.
type PanicFunc func(owner string, f Failure)

// StopTimeout is returned by Stop when its context ends before every task
// has returned.
type StopTimeout struct {
	Tasks []string
}

func (e *StopTimeout) Error() string {
	return fmt.Sprintf("supervise: stop timed out with tasks still running: %s", strings.Join(e.Tasks, ", "))
}

// Group runs the background work of one owner. Create it with NewGroup.
type Group struct {
	owner  string
	log    PanicFunc
	onFail func(Failure)

	ctx    context.Context
	cancel context.CancelFunc

	mu sync.Mutex
	// closed is ErrFailed or ErrStopped once the group takes no more work.
	closed  error
	nextID  uint64
	running map[uint64]string
	// idle is closed when the last running task returns; nil while none run.
	idle   chan struct{}
	timers map[*Timer]struct{}
}

// NewGroup returns a group for owner. log and onFail may be nil. Neither may
// block: they run on the goroutine that panicked, before the group's context
// is cancelled.
func NewGroup(owner string, log PanicFunc, onFail func(Failure)) *Group {
	ctx, cancel := context.WithCancel(context.Background())
	return &Group{
		owner:   owner,
		log:     log,
		onFail:  onFail,
		ctx:     ctx,
		cancel:  cancel,
		running: make(map[uint64]string),
		timers:  make(map[*Timer]struct{}),
	}
}

// Go runs fn on a goroutine of its own. fn must return soon after ctx ends.
func (g *Group) Go(task string, fn func(ctx context.Context)) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed != nil {
		return g.closed
	}
	id := g.enterLocked(task)
	go g.run(id, task, func() { fn(g.ctx) })
	return nil
}

// AfterFunc runs fn after d, as a task of the group. A timer that has not
// fired by the time the group stops or fails never runs.
func (g *Group) AfterFunc(d time.Duration, task string, fn func()) (*Timer, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed != nil {
		return nil, g.closed
	}
	t := &Timer{group: g}
	t.timer = time.AfterFunc(d, func() { g.fire(t, task, fn) })
	g.timers[t] = struct{}{}
	return t, nil
}

// Stop cancels the tasks' context, stops pending timers and waits for
// running tasks until ctx ends. It then returns a *StopTimeout naming the
// tasks still running. It cannot end them: a task that ignores its context
// runs on until it returns by itself.
func (g *Group) Stop(ctx context.Context) error {
	g.mu.Lock()
	g.closeLocked(ErrStopped)
	idle := g.idle
	g.mu.Unlock()
	g.cancel()

	if idle == nil {
		return nil
	}
	select {
	case <-idle:
		return nil
	case <-ctx.Done():
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	// The last task may have returned while ctx ended.
	if len(g.running) == 0 {
		return nil
	}
	return &StopTimeout{Tasks: slices.Sorted(maps.Values(g.running))}
}

// closeLocked refuses further work with reason and drops pending timers. A
// failure overrides a stop, never the other way round, so a panic during
// shutdown still reaches the owner.
func (g *Group) closeLocked(reason error) {
	if !errors.Is(g.closed, ErrFailed) {
		g.closed = reason
	}
	for t := range g.timers {
		t.timer.Stop()
	}
	clear(g.timers)
}

func (g *Group) enterLocked(task string) uint64 {
	g.nextID++
	g.running[g.nextID] = task
	if g.idle == nil {
		g.idle = make(chan struct{})
	}
	return g.nextID
}

func (g *Group) leave(id uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.running, id)
	if len(g.running) == 0 {
		close(g.idle)
		g.idle = nil
	}
}

func (g *Group) fire(t *Timer, task string, fn func()) {
	g.mu.Lock()
	// time.Timer.Stop cannot recall a callback that has already started;
	// whether the timer is still registered decides. Closing the group
	// unregisters all of them.
	if _, pending := g.timers[t]; !pending {
		g.mu.Unlock()
		return
	}
	delete(g.timers, t)
	id := g.enterLocked(task)
	g.mu.Unlock()
	g.run(id, task, fn)
}

func (g *Group) run(id uint64, task string, fn func()) {
	// leave runs after the panic is handled, so Stop also waits for onFail.
	defer g.leave(id)
	defer func() {
		if v := recover(); v != nil {
			g.fail(Failure{Task: task, Value: v, Stack: debug.Stack()})
		}
	}()
	fn()
}

func (g *Group) fail(f Failure) {
	if g.log != nil {
		g.log(g.owner, f)
	}
	g.mu.Lock()
	first := !errors.Is(g.closed, ErrFailed)
	g.closeLocked(ErrFailed)
	g.mu.Unlock()
	if !first {
		return
	}
	if g.onFail != nil {
		g.onFail(f)
	}
	g.cancel()
}

// Timer is a pending AfterFunc of a group.
type Timer struct {
	group *Group
	timer *time.Timer
}

// Stop prevents the callback from running. It reports false if the callback
// has already started, the timer was stopped before, or the group has
// stopped or failed.
func (t *Timer) Stop() bool {
	g := t.group
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, pending := g.timers[t]; !pending {
		return false
	}
	delete(g.timers, t)
	t.timer.Stop()
	return true
}
