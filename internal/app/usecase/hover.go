package usecase

import (
	"context"
	"errors"
	"sync"

	"seam/internal/app/port/out"
	"seam/internal/domain/change"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
	"seam/internal/supervise"
)

// The bounds of the documents kept open with the servers: as many as a
// user goes back and forth between, and no more text than the servers
// should have to hold for them.
const (
	// Both sides of a file count, so twice the files a user goes back and
	// forth between.
	openDocs      = 32
	openDocsBytes = 128 << 20
	// waitingDocs bounds the documents of commits not checked out yet: a
	// few files back, as a hover only checks out the commit of one shown.
	waitingDocs = 4
)

// Hovers implements in.Hoverer, in.DocumentTracker and in.LSPUpdates over
// the language servers. It passes the servers' updates on, less the
// answers no one waits for any more, and colours the code in answers
// first, in the background. Documents of a commit wait for a checkout of
// it, made the first time a hover asks in one.
//
// Two goroutines in groups of their own do that: one takes the updates
// from the servers and passes them on, the other colours. Status changes
// thus arrive while a highlighter hangs, and after the colourer failed;
// its answers then come uncoloured. Once the first has failed, nothing
// arrives any more, and every hover is answered Failed at once, so that
// none is waited for in vain.
type Hovers struct {
	docs        out.DocumentSync
	requester   out.HoverRequester
	events      out.ServerEvents
	highlighter out.Highlighter
	checkouts   out.CommitCheckouts
	roots       out.CheckoutRoots
	languages   lang.Table

	forward *supervise.Group
	// colours colours one answer at a time and publishes all updates.
	colours     *worker[hover.Update, hover.Update]
	checkingOut *supervise.Group

	mu sync.Mutex
	// open are the documents shown last, their texts counting as their
	// size. waiting are those of commits not checked out yet, kept apart
	// so they take no room from the documents the servers have.
	open    *lru[hover.DocKey, struct{}]
	waiting *lru[hover.DocKey, hover.Document]
	// trees is how far the checkout of each commit asked in has come:
	// Starting, Ready or ServerFailed.
	trees  map[change.ObjectID]hover.ServerStatus
	latest hover.ReqID
	// forwardFailed says the servers' updates no longer arrive.
	forwardFailed bool
}

// HoverPorts are the outgoing ports of Hovers.
type HoverPorts struct {
	Docs        out.DocumentSync
	Requester   out.HoverRequester
	Events      out.ServerEvents
	Highlighter out.Highlighter
	Checkouts   out.CommitCheckouts
	Roots       out.CheckoutRoots
}

// NewHovers returns the use case over its ports; Start starts its
// goroutines. languages tells which documents a server may hover over.
func NewHovers(p HoverPorts, languages lang.Table, panics supervise.PanicFunc) *Hovers {
	h := &Hovers{
		docs: p.Docs, requester: p.Requester, events: p.Events, highlighter: p.Highlighter,
		checkouts: p.Checkouts, roots: p.Roots, languages: languages,
		checkingOut: supervise.NewGroup("hover checkouts", panics, nil),
		open:        newLRU[hover.DocKey, struct{}](openDocs, openDocsBytes),
		waiting:     newLRU[hover.DocKey, hover.Document](waitingDocs, openDocsBytes),
		trees:       make(map[change.ObjectID]hover.ServerStatus),
	}
	h.forward = supervise.NewGroup("hovers", panics, func(supervise.Failure) {
		h.mu.Lock()
		h.forwardFailed = true
		h.mu.Unlock()
	})
	h.colours = newWorker("hover colours", hover.ErrColourerFailed, panics, h.colour,
		// A newer answer is the one waited for, see Hover.
		func(hover.Update, hover.Update) bool { return true },
		// The answer is still of use without colours.
		func(u hover.Update, _ error) hover.Update { return u })
	return h
}

// Start starts the goroutines.
func (h *Hovers) Start() error {
	if err := h.colours.Start(); err != nil {
		return err
	}
	return h.forward.Go("forward", h.forwardLoop)
}

// Stop ends the goroutines, waiting for them until ctx ends.
func (h *Hovers) Stop(ctx context.Context) error {
	return errors.Join(h.forward.Stop(ctx), h.checkingOut.Stop(ctx), h.colours.Stop(ctx))
}

// Track opens d with its server and keeps the documents shown last open:
// the oldest beyond the bounds is closed. A document without a path gets
// no server; one of a commit not checked out yet waits for the checkout,
// if its language has a server, and has none before.
func (h *Hovers) Track(d hover.Document) (lang.Instance, bool) {
	if d.Path == "" {
		return lang.Instance{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if st := h.trees[d.Commit]; !d.Commit.IsZero() && st != hover.Ready {
		if _, _, ok := h.languages.Server(d.Lang); ok && st != hover.ServerFailed {
			h.waiting.put(d.Key(), d, int64(len(d.Text)))
		}
		return lang.Instance{}, false
	}
	return h.syncLocked(d)
}

// syncLocked opens d with its server and keeps it among the documents
// shown last, closing the oldest beyond the bounds.
func (h *Hovers) syncLocked(d hover.Document) (lang.Instance, bool) {
	inst, ok := h.docs.Sync(d)
	if !ok {
		h.open.remove(d.Key())
		return lang.Instance{}, false
	}
	for _, k := range h.open.put(d.Key(), struct{}{}, int64(len(d.Text))) {
		h.docs.Close(k)
	}
	return inst, true
}

// Untrack closes the document k.
func (h *Hovers) Untrack(k hover.DocKey) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.open.remove(k)
	h.waiting.remove(k)
	h.docs.Close(k)
}

// Hover asks at p; an answer to an earlier request is dropped from then
// on, and its colouring given up. In a commit it is first checked out.
func (h *Hovers) Hover(p hover.Position) hover.Submit {
	h.mu.Lock()
	// Before the request: its answer may be drained before Request returns.
	h.latest = p.Req
	failed := h.forwardFailed
	checkout := hover.Ready
	if !p.Commit.IsZero() {
		checkout = h.checkoutLocked(p.Key())
	}
	h.mu.Unlock()
	switch {
	case failed:
		return hover.Failed
	case checkout == hover.Starting:
		return hover.NotReady
	case checkout != hover.Ready:
		return hover.Unsupported
	}
	h.colours.drop(func(u hover.Update) bool { return u.Req != p.Req })
	return h.requester.Request(p)
}

// CancelHover drops the request id.
func (h *Hovers) CancelHover(id hover.ReqID) {
	h.colours.drop(func(u hover.Update) bool { return u.Req == id })
	h.requester.Cancel(id)
}

// Signal is set after each new update.
func (h *Hovers) Signal() <-chan struct{} { return h.colours.Signal() }

// Drain hands over the updates since the last call, oldest first, without
// the answers to requests since replaced.
func (h *Hovers) Drain() []hover.Update {
	us := h.colours.Drain()
	h.mu.Lock()
	latest := h.latest
	h.mu.Unlock()
	kept := us[:0]
	for _, u := range us {
		if u.Kind != hover.HoverDone || u.Req == latest {
			kept = append(kept, u)
		}
	}
	return kept
}

// waitedFor says that the answer to req is the one waited for.
func (h *Hovers) waitedFor(req hover.ReqID) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return req == h.latest
}

// forwardLoop passes the servers' updates on as they come. An answer with
// code in a known language goes to the colourer, unless it is no longer
// waited for.
func (h *Hovers) forwardLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.events.Signal():
		}
		for _, u := range h.events.Drain() {
			if u.Kind != hover.HoverDone || !u.Content.HasCode() {
				h.colours.publish(u)
				continue
			}
			if h.waitedFor(u.Req) {
				h.colours.submit(u)
			}
		}
	}
}

// colour finds the tokens of u's code blocks within one budget; what it
// leaves stays plain. A request given up meanwhile ends ctx, which the
// highlighter notices after its next token.
func (h *Hovers) colour(ctx context.Context, u hover.Update) hover.Update {
	budget, cancel := context.WithTimeout(ctx, highlightBudget)
	defer cancel()
	// The blocks are this job's alone: the adapter hands each update over once.
	for i, b := range u.Content.Blocks {
		if b.Kind != hover.Code || b.Lang == "" || budget.Err() != nil {
			continue
		}
		// No lexer for the language leaves the block plain, as in a file.
		u.Content.Blocks[i].Tokens, _ = h.highlighter.Highlight(budget, b.Lang, syntax.Lexable(b.Code))
	}
	return u
}
