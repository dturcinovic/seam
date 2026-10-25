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

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/limits"
	"github.com/dturcinovic/seam/internal/supervise"
)

// The bounds of the pool; see the plan's section on limiting instances.
const (
	idleTimeout = 60 * time.Second
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
// stderr files. MaxInstances bounds the servers running at once; zero is
// the default.
type Config struct {
	Root         string
	LogDir       string
	MaxInstances limits.Instances
}

// Pool runs the language servers of a session: one client per instance,
// at most maxInstances at a time. Each instance keeps its documents, so a
// server stopped and started anew gets them again. Servers run in the
// worktree, or in the checkout of the commit whose documents they serve.
// It implements out.DocumentSync, out.HoverRequester, out.ServerEvents and
// out.CheckoutRoots; none of its calls blocks.
type Pool struct {
	*events

	table        lang.Table
	maxInstances int
	starter      starter
	log          *zap.Logger
	panics       supervise.PanicFunc
	// group owns the idle timers and the stops of idle and evicted
	// instances.
	group *supervise.Group

	mu        sync.Mutex
	stopped   bool
	instances map[lang.Instance]*instance
	paths     map[hover.DocKey]lang.Instance
	// trees are where the documents of each commit lie, the worktree's
	// under the zero commit.
	trees map[change.ObjectID]*tree
	// clients are all clients whose stop has not returned, evicted ones
	// included, so the pool's own Stop waits for them too.
	clients map[*client]struct{}
	tick    uint64
	// programs are the resolved commands by server name, "" for one that
	// is not installed: a missing server stays so for the session.
	programs map[string]string
}

// tree is the worktree or the checkout of a commit, with the root markers
// found in it.
type tree struct {
	dir     string
	markers *markers
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
		events: newEvents(), table: table, maxInstances: int(cfg.MaxInstances.OrDefault()), starter: s, log: log, panics: panics,
		group:     supervise.NewGroup("lsp pool", panics, nil),
		instances: make(map[lang.Instance]*instance),
		paths:     make(map[hover.DocKey]lang.Instance),
		trees:     map[change.ObjectID]*tree{{}: {dir: cfg.Root, markers: newMarkers(cfg.Root)}},
		clients:   make(map[*client]struct{}),
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
	var inst lang.Instance
	t, ok := p.trees[d.Commit]
	if ok {
		inst, ok = p.table.Instance(d.Lang, d.Path, t.markers.has)
		inst.Commit = d.Commit
	}
	if old, had := p.paths[d.Key()]; had && (!ok || old != inst) {
		p.closeLocked(d.Key())
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
	p.paths[d.Key()] = inst
	if in.idle != nil {
		in.idle.Stop()
		in.idle = nil
	}
	p.clientLocked(inst, in)
	return inst, true
}

// Close implements out.DocumentSync.
func (p *Pool) Close(k hover.DocKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked(k)
}

// Mount implements out.CheckoutRoots. A commit's tree never moves: a
// second mount changes nothing.
func (p *Pool) Mount(id change.ObjectID, dir string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.trees[id]; !ok {
		p.trees[id] = &tree{dir: dir, markers: newMarkers(dir)}
	}
}

// Request implements out.HoverRequester.
func (p *Pool) Request(pos hover.Position) hover.Submit { return p.ask(pos, pendingHover) }

// RequestDefinition implements out.DefinitionRequester.
func (p *Pool) RequestDefinition(pos hover.Position) hover.Submit {
	return p.ask(pos, pendingDefinition)
}

// ask hands a request of kind at pos to the client of pos's instance.
func (p *Pool) ask(pos hover.Position, kind pendingKind) hover.Submit {
	p.mu.Lock()
	defer p.mu.Unlock()
	inst, ok := p.paths[pos.Key()]
	if !ok || p.stopped {
		return hover.Unsupported
	}
	c := p.clientLocked(inst, p.instances[inst])
	if c == nil {
		return hover.Unsupported
	}
	return c.ask(pos, kind)
}

// CancelDefinition implements out.DefinitionRequester: hovers and
// definitions share their ReqIDs.
func (p *Pool) CancelDefinition(id hover.ReqID) { p.Cancel(id) }

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
	c := newClient(clientConfig{inst: inst, server: s, repoRoot: p.trees[inst.Commit].dir, languages: p.table, docs: in.docs, starter: p.starter, log: p.log, panics: p.panics})
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
	if len(running) < p.maxInstances {
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

func (p *Pool) closeLocked(k hover.DocKey) {
	inst, ok := p.paths[k]
	if !ok {
		return
	}
	delete(p.paths, k)
	in := p.instances[inst]
	if in.docs.remove(k.Path) > 0 {
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
