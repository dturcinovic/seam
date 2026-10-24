package usecase

import (
	"context"
	"errors"
	"sync"

	"seam/internal/app/port/out"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
	"seam/internal/supervise"
)

// The bounds of the documents kept open with the servers: as many as a
// user goes back and forth between, and no more text than the servers
// should have to hold for them.
const (
	openDocs      = 16
	openDocsBytes = 128 << 20
)

// Hovers implements in.Hoverer, in.DocumentTracker and in.LSPUpdates over
// the language servers. It passes the servers' updates on, less the
// answers no one waits for any more, and colours the code in answers
// first, in the background.
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

	forward *supervise.Group
	// colours colours one answer at a time and publishes all updates.
	colours *worker[hover.Update, hover.Update]

	mu sync.Mutex
	// open are the documents shown last, by path, their texts counting as
	// their size.
	open   *lru[string, struct{}]
	latest hover.ReqID
	// forwardFailed says the servers' updates no longer arrive.
	forwardFailed bool
}

// NewHovers returns the use case over its ports; Start starts its
// goroutines.
func NewHovers(docs out.DocumentSync, requester out.HoverRequester, events out.ServerEvents, highlighter out.Highlighter, panics supervise.PanicFunc) *Hovers {
	h := &Hovers{
		docs: docs, requester: requester, events: events, highlighter: highlighter,
		open: newLRU[string, struct{}](openDocs, openDocsBytes),
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
	return errors.Join(h.forward.Stop(ctx), h.colours.Stop(ctx))
}

// Track opens d with its server and keeps the documents shown last open:
// the oldest beyond the bounds is closed. A document without a path gets
// no server.
func (h *Hovers) Track(d hover.Document) (lang.Instance, bool) {
	if d.Path == "" {
		return lang.Instance{}, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	inst, ok := h.docs.Sync(d)
	if !ok {
		h.open.remove(d.Path)
		return lang.Instance{}, false
	}
	for _, path := range h.open.put(d.Path, struct{}{}, int64(len(d.Text))) {
		h.docs.Close(path)
	}
	return inst, true
}

// Untrack closes the document at path.
func (h *Hovers) Untrack(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.open.remove(path)
	h.docs.Close(path)
}

// Hover asks at p; an answer to an earlier request is dropped from then
// on, and its colouring given up.
func (h *Hovers) Hover(p hover.Position) hover.Submit {
	h.mu.Lock()
	// Before the request: its answer may be drained before Request returns.
	h.latest = p.Req
	failed := h.forwardFailed
	h.mu.Unlock()
	if failed {
		return hover.Failed
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
