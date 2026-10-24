package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/supervise"
)

// The bounds of one server connection; see the plan's write classes.
const (
	mandatoryCap  = 32
	requestCap    = 16
	maxOpenHovers = 8
	initTimeout   = 20 * time.Second
	hoverTimeout  = 5 * time.Second
	// termGrace is how long a server group has between SIGTERM and SIGKILL.
	termGrace = 200 * time.Millisecond
)

var (
	errOverload    = errors.New("server does not read: mandatory queue full")
	errInitTimeout = errors.New("initialize not answered in time")
	errExited      = errors.New("server exited")
)

// sink takes what a client has to report. Neither call may block.
type sink interface {
	status(i lang.Instance, s hover.ServerStatus)
	result(u hover.Update)
}

// desiredDoc is a document as the server is to see it.
type desiredDoc struct {
	hover.Document
	languageID string
}

// frame is a message to write. initialized marks the notification after
// which the server may be told about documents.
type frame struct {
	body        []byte
	initialized bool
}

// client is the connection to one server instance: its process, a reader
// and a writer goroutine and the requests in flight. It owns its supervise
// group; the documents are the instance's, which it is handed.
type client struct {
	inst     lang.Instance
	server   lang.Server
	repoRoot string
	// languages names the language of a code fence in an answer.
	languages lang.Table
	dir       string
	ws        workspace
	docs      *docSet
	starter   starter
	// sink is set by the pool before start.
	sink  sink
	log   *zap.Logger
	group *supervise.Group

	mandatory    chan frame
	requests     chan []byte
	dirty        chan struct{}
	shutdownDone chan struct{}
	exited       chan struct{}
	exitOnce     sync.Once
	stopOnce     sync.Once
	stopped      chan struct{}

	mu sync.Mutex
	// state is what the client reports. stopping is set once stop begins:
	// from then on the end of the connection is expected, not a failure.
	state    hover.ServerStatus
	stopping bool
	conn     *conn
	nextID   int64
	pending  map[int64]*pending
	// hovers are the ids of open hovers, oldest first.
	hovers   []int64
	encoding string
}

type clientConfig struct {
	inst      lang.Instance
	server    lang.Server
	repoRoot  string
	languages lang.Table
	docs      *docSet
	starter   starter
	log       *zap.Logger
	panics    supervise.PanicFunc
}

func newClient(cfg clientConfig) *client {
	dir := filepath.Join(cfg.repoRoot, filepath.FromSlash(cfg.inst.Root))
	c := &client{
		inst: cfg.inst, server: cfg.server, repoRoot: cfg.repoRoot, languages: cfg.languages, dir: dir, docs: cfg.docs, starter: cfg.starter,
		log:          cfg.log.With(zap.String("server", cfg.server.Name), zap.String("root", cfg.inst.Root)),
		ws:           workspace{uri: fileURI(dir), name: filepath.Base(dir), settings: cfg.server.Settings},
		mandatory:    make(chan frame, mandatoryCap),
		requests:     make(chan []byte, requestCap),
		dirty:        make(chan struct{}, 1),
		shutdownDone: make(chan struct{}),
		exited:       make(chan struct{}),
		stopped:      make(chan struct{}),
		pending:      make(map[int64]*pending),
		encoding:     "utf-16",
	}
	c.group = supervise.NewGroup("lsp "+cfg.server.Name+" "+cfg.inst.Root, cfg.panics, c.panicked)
	return c
}

// start launches the server; Starting is reported at once, the rest as it
// happens.
func (c *client) start() {
	c.setState(hover.Starting)
	c.docs.attach(c.dirty)
	if err := c.group.Go("process", c.run); err != nil {
		c.closeExited()
		c.fail(err)
	}
}

func (c *client) status() hover.ServerStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// run starts the process and stays with it until it ends. It never unlocks
// its thread: Pdeathsig ends the server with the thread that started it,
// which is then this goroutine's alone and ends with it.
func (c *client) run(context.Context) {
	runtime.LockOSThread()
	defer c.closeExited()
	cn, err := c.starter.start(c.server, c.dir)
	if err != nil {
		c.fail(err)
		return
	}
	c.log.Info("language server started", zap.Int("pid", cn.pid))
	c.mu.Lock()
	c.conn = cn
	stopping := c.stopping
	c.mu.Unlock()
	switch {
	case stopping:
		// stop came while the process started and found no conn to end.
		cn.kill()
	default:
		if err := c.connect(cn); err != nil {
			c.fail(err)
		}
	}
	err = errExited
	if waitErr := cn.wait(); waitErr != nil {
		err = fmt.Errorf("%w: %w", errExited, waitErr)
	}
	c.fail(err)
	// Whatever the server started may still hold the pipes; ending its
	// group and closing them ends a reader or writer still waiting.
	cn.kill()
	_ = cn.in.Close() // a second close of either end changes nothing
	_ = cn.out.Close()
}

// connect starts the reader and writer and sends initialize.
func (c *client) connect(cn *conn) error {
	if err := c.group.Go("read", func(context.Context) { c.readLoop(cn.out) }); err != nil {
		return err
	}
	if err := c.group.Go("write", func(ctx context.Context) { c.writeLoop(ctx, cn.in) }); err != nil {
		return err
	}
	options := c.server.InitializationOptions
	if err := c.call(pendingInit, "initialize", initializeParams(os.Getpid(), c.ws, options)); err != nil {
		return err
	}
	_, err := c.group.AfterFunc(initTimeout, "initialize timeout", func() {
		if c.status() == hover.Starting {
			c.fail(errInitTimeout)
		}
	})
	return err
}

func (c *client) closeExited() { c.exitOnce.Do(func() { close(c.exited) }) }

// transition moves from to to, unless something else, as a failure, has
// moved the client meanwhile.
func (c *client) transition(from, to hover.ServerStatus) {
	c.mu.Lock()
	if c.state != from || c.stopping {
		c.mu.Unlock()
		return
	}
	c.state = to
	c.mu.Unlock()
	c.sink.status(c.inst, to)
}

func (c *client) setState(s hover.ServerStatus) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
	c.sink.status(c.inst, s)
}

// fail moves the client to Failed once: open hovers end with Error, and the
// server group is ended. While stopping, the connection ending is what was
// asked for, so nothing fails.
func (c *client) fail(err error) {
	c.mu.Lock()
	if c.stopping || c.state == hover.ServerFailed {
		c.mu.Unlock()
		return
	}
	c.state = hover.ServerFailed
	open := c.takeHoversLocked()
	cn := c.conn
	c.mu.Unlock()

	c.log.Warn("language server failed", zap.Error(err))
	c.sink.status(c.inst, hover.ServerFailed)
	c.endHovers(open, hover.Error)
	if cn == nil {
		return
	}
	cn.terminate()
	if _, err := c.group.AfterFunc(termGrace, "kill", cn.kill); err != nil {
		cn.kill()
	}
}

// panicked is the group's onFail. It must not block, and a closed group
// runs no more timers, so the server goes at once, even when it had failed
// before and the kill fail scheduled then will never run.
func (c *client) panicked(f supervise.Failure) {
	c.fail(fmt.Errorf("panic in %s: %v", f.Task, f.Value))
	c.mu.Lock()
	cn := c.conn
	c.mu.Unlock()
	if cn != nil {
		cn.kill()
	}
}

// stop ends the server in the plan's order: shutdown, answered within
// shutdownIn; exit, the process gone within exitIn; then the group by
// signal. It returns once the group's goroutines have, or ctx ends. A
// second call, as the pool's own Stop during an eviction, does not start
// over: it ends the server at once and waits for the first.
func (c *client) stop(ctx context.Context, shutdownIn, exitIn time.Duration) error {
	first := false
	c.stopOnce.Do(func() { first = true })
	if !first {
		c.mu.Lock()
		cn := c.conn
		c.mu.Unlock()
		if cn != nil {
			cn.kill()
		}
		waitFor(ctx, 0, c.stopped)
		return c.group.Stop(ctx)
	}
	defer close(c.stopped)

	c.mu.Lock()
	ready := c.state == hover.Ready
	c.stopping = true
	cn := c.conn
	open := c.takeHoversLocked()
	c.mu.Unlock()
	c.endHovers(open, hover.Error)

	if ready && c.call(pendingShutdown, "shutdown", nil) == nil {
		waitFor(ctx, shutdownIn, c.shutdownDone)
	}
	if cn != nil {
		if body, err := notification("exit", nil); err == nil {
			// A full queue changes nothing here: the signals follow anyway.
			select {
			case c.mandatory <- frame{body: body}:
			default:
			}
		}
		waitFor(ctx, exitIn, c.exited)
		cn.terminate()
		waitFor(ctx, termGrace, c.exited)
		cn.kill()
	}
	// Without a conn the process may still be starting; run ends it once it
	// has, as stopping is set.
	waitFor(ctx, 0, c.exited)
	c.mu.Lock()
	failed := c.state == hover.ServerFailed
	if !failed {
		c.state = hover.Stopped
	}
	c.mu.Unlock()
	if !failed {
		c.sink.status(c.inst, hover.Stopped)
	}
	return c.group.Stop(ctx)
}

// waitFor waits for done for at most d, or until ctx ends for a zero d.
func waitFor(ctx context.Context, d time.Duration, done <-chan struct{}) {
	if d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// push queues a mandatory frame. A full queue means the server reads no
// more; the connection fails rather than lose the frame.
func (c *client) push(f frame) bool {
	select {
	case c.mandatory <- f:
		return true
	default:
		c.fail(errOverload)
		return false
	}
}

// notify sets a signal of capacity 1 without blocking: one already set
// covers this event too.
func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
