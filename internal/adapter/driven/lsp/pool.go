package lsp

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/supervise"
)

// The bounds of the pool; see the plan's section on limiting instances.
const (
	maxInstances = 4
	idleTimeout  = 60 * time.Second
	// stopReserve is what the stop hook leaves of the deadline to the hooks
	// after it.
	stopReserve = 500 * time.Millisecond
	// The budget of stopping one instance outside of shutdown, when it idles
	// or is evicted.
	evictShutdown = time.Second
	evictExit     = 500 * time.Millisecond
	// stopBudget applies when a stop context has no deadline.
	stopBudget = 2500 * time.Millisecond
)

// Config is where the pool runs servers: the repository root, which
// document paths are relative to, and the directory for the servers'
// stderr files.
type Config struct {
	Root   string
	LogDir string
}

// Pool runs the language servers of a session: one client per instance,
// at most maxInstances at a time. Each instance keeps its documents, so a
// server stopped and started anew gets them again. It implements
// out.DocumentSync, out.HoverRequester and out.ServerEvents; none of its
// calls blocks.
type Pool struct {
	*events

	root    string
	table   lang.Table
	starter starter
	log     *zap.Logger
	panics  supervise.PanicFunc
	// group owns the idle timers and the stops of idle and evicted
	// instances.
	group *supervise.Group

	mu        sync.Mutex
	stopped   bool
	instances map[lang.Instance]*instance
	paths     map[string]lang.Instance
	// clients are all clients whose stop has not returned, evicted ones
	// included, so the pool's own Stop waits for them too.
	clients map[*client]struct{}
	tick    uint64
	markers *markers
	// programs are the resolved commands by server name, "" for one that
	// is not installed: a missing server stays so for the session.
	programs map[string]string
}

type instance struct {
	server lang.Server
	docs   *docSet
	client *client
	used   uint64
	idle   *supervise.Timer
	// missing is set once Missing has been reported for the instance.
	missing bool
}

func newPool(cfg Config, table lang.Table, s starter, log *zap.Logger, panics supervise.PanicFunc) *Pool {
	return &Pool{
		events: newEvents(), root: cfg.Root, table: table, starter: s, log: log, panics: panics,
		group:     supervise.NewGroup("lsp pool", panics, nil),
		instances: make(map[lang.Instance]*instance),
		paths:     make(map[string]lang.Instance),
		clients:   make(map[*client]struct{}),
		markers:   newMarkers(cfg.Root),
		programs:  make(map[string]string),
	}
}

// Sync implements out.DocumentSync.
func (p *Pool) Sync(d hover.Document) (lang.Instance, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return lang.Instance{}, false
	}
	inst, ok := p.table.Instance(d.Lang, d.Path, p.markers.has)
	if old, had := p.paths[d.Path]; had && (!ok || old != inst) {
		p.closeLocked(d.Path)
	}
	if !ok {
		return lang.Instance{}, false
	}
	server, languageID, _ := p.table.Server(d.Lang)
	in := p.instances[inst]
	if in == nil {
		in = &instance{server: server, docs: newDocSet()}
		p.instances[inst] = in
	}
	in.docs.put(desiredDoc{Document: d, languageID: languageID})
	p.paths[d.Path] = inst
	if in.idle != nil {
		in.idle.Stop()
		in.idle = nil
	}
	p.clientLocked(inst, in)
	return inst, true
}

// Close implements out.DocumentSync.
func (p *Pool) Close(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked(path)
}

// Request implements out.HoverRequester.
func (p *Pool) Request(pos hover.Position) hover.Submit {
	p.mu.Lock()
	defer p.mu.Unlock()
	inst, ok := p.paths[pos.Path]
	if !ok || p.stopped {
		return hover.Unsupported
	}
	c := p.clientLocked(inst, p.instances[inst])
	if c == nil {
		return hover.Unsupported
	}
	return c.request(pos)
}

// Cancel implements out.HoverRequester.
func (p *Pool) Cancel(id hover.ReqID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for c := range p.clients {
		c.cancel(id)
	}
}

// Stop ends every server in parallel, within the deadline of ctx less
// stopReserve: shutdown within half the time, exit within the next
// quarter, then by signal. It logs the tasks still running when the time
// is up rather than fail the stop, which must go on to the hooks after it.
func (p *Pool) Stop(ctx context.Context) error {
	p.mu.Lock()
	p.stopped = true
	clients := slices.Collect(maps.Keys(p.clients))
	p.mu.Unlock()

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, stopBudget)
		defer cancel()
	}
	deadline, _ := ctx.Deadline()
	end := deadline.Add(-stopReserve)
	ctx, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	left := time.Until(end)

	stops := supervise.NewGroup("lsp stop", p.panics, nil)
	var mu sync.Mutex
	var errs []error
	for _, c := range clients {
		err := stops.Go("stop "+c.server.Name, func(context.Context) {
			if err := c.stop(ctx, left/2, left/4); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		})
		if err != nil {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}
	}
	stopErr := stops.Stop(ctx)
	mu.Lock()
	errs = append(errs, stopErr, p.group.Stop(ctx))
	mu.Unlock()
	if err := errors.Join(errs...); err != nil {
		p.log.Warn("language servers did not stop in time", zap.Error(err))
	}
	return nil
}

// clientLocked returns the client of inst, starting one if it has none or
// it was stopped. A server that is not installed gets no client: nil, with
// Missing reported once for the instance.
func (p *Pool) clientLocked(inst lang.Instance, in *instance) *client {
	p.tick++
	in.used = p.tick
	if in.client != nil && in.client.status() != hover.Stopped {
		return in.client
	}
	program, known := p.programs[in.server.Name]
	if !known {
		var err error
		if program, err = p.starter.find(in.server.Command); err != nil {
			p.log.Info("language server missing", zap.String("server", in.server.Name), zap.Error(err))
		}
		p.programs[in.server.Name] = program
	}
	if program == "" {
		if !in.missing {
			in.missing = true
			p.status(inst, hover.Missing)
		}
		return nil
	}
	p.evictLocked(inst)
	s := in.server
	s.Command = program
	c := newClient(clientConfig{inst: inst, server: s, repoRoot: p.root, languages: p.table, docs: in.docs, starter: p.starter, log: p.log, panics: p.panics})
	c.sink = p.of(c)
	in.client = c
	p.clients[c] = struct{}{}
	c.start()
	return c
}

// evictLocked makes room for one more instance: with maxInstances running,
// the one used longest ago goes, one without documents first. Its
// documents stay, for when it is needed again.
func (p *Pool) evictLocked(except lang.Instance) {
	var running []lang.Instance
	for inst, in := range p.instances {
		if inst != except && in.client != nil && isRunning(in.client.status()) {
			running = append(running, inst)
		}
	}
	if len(running) < maxInstances {
		return
	}
	victim := slices.MinFunc(running, func(a, b lang.Instance) int {
		ia, ib := p.instances[a], p.instances[b]
		if emptyA, emptyB := ia.docs.len() == 0, ib.docs.len() == 0; emptyA != emptyB {
			if emptyA {
				return -1
			}
			return 1
		}
		return cmp.Compare(ia.used, ib.used)
	})
	p.stopInstanceLocked(p.instances[victim])
}

func isRunning(s hover.ServerStatus) bool { return s == hover.Starting || s == hover.Ready }

// stopInstanceLocked stops in's client in the background.
func (p *Pool) stopInstanceLocked(in *instance) {
	c := in.client
	in.client = nil
	err := p.group.Go("stop "+c.server.Name, func(ctx context.Context) {
		if err := c.stop(ctx, evictShutdown, evictExit); err != nil {
			p.log.Warn("language server did not stop in time", zap.Error(err))
		}
		p.mu.Lock()
		delete(p.clients, c)
		p.mu.Unlock()
	})
	if err != nil {
		// The pool is stopping; its Stop takes the client along.
		p.log.Debug("background stop refused", zap.Error(err))
	}
}

func (p *Pool) closeLocked(path string) {
	inst, ok := p.paths[path]
	if !ok {
		return
	}
	delete(p.paths, path)
	in := p.instances[inst]
	if in.docs.remove(path) > 0 {
		return
	}
	switch {
	case in.client == nil || in.client.status() == hover.Stopped:
		delete(p.instances, inst)
		return
	case in.client.status() == hover.ServerFailed:
		// Kept, so a failed server stays failed for the session rather
		// than start again whenever one of its files is shown.
		return
	}
	t, err := p.group.AfterFunc(idleTimeout, "idle "+inst.Server, func() { p.idleOut(inst) })
	if err == nil {
		in.idle = t
	}
}

// idleOut stops an instance that has had no documents for idleTimeout.
func (p *Pool) idleOut(inst lang.Instance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	in := p.instances[inst]
	if in == nil || in.docs.len() > 0 {
		return
	}
	delete(p.instances, inst)
	if in.client != nil && isRunning(in.client.status()) {
		p.stopInstanceLocked(in)
	}
}
