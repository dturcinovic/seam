package usecase

import (
	"sync"

	"seam/internal/app/port/out"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
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
// answers no one waits for any more; it has no goroutine of its own.
type Hovers struct {
	docs      out.DocumentSync
	requester out.HoverRequester
	events    out.ServerEvents

	mu sync.Mutex
	// open are the documents shown last, by path, their texts counting as
	// their size.
	open   *lru[string, struct{}]
	latest hover.ReqID
}

// NewHovers returns the use case over its ports.
func NewHovers(docs out.DocumentSync, requester out.HoverRequester, events out.ServerEvents) *Hovers {
	return &Hovers{
		docs: docs, requester: requester, events: events,
		open: newLRU[string, struct{}](openDocs, openDocsBytes),
	}
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

// Hover asks at p; an answer to an earlier request is dropped from then on.
func (h *Hovers) Hover(p hover.Position) hover.Submit {
	h.mu.Lock()
	// Before the request: its answer may be drained before Request returns.
	h.latest = p.Req
	h.mu.Unlock()
	return h.requester.Request(p)
}

// CancelHover drops the request id.
func (h *Hovers) CancelHover(id hover.ReqID) { h.requester.Cancel(id) }

// Signal is set after each new update.
func (h *Hovers) Signal() <-chan struct{} { return h.events.Signal() }

// Drain hands over the updates since the last call, oldest first, without
// the answers to requests since replaced.
func (h *Hovers) Drain() []hover.Update {
	us := h.events.Drain()
	h.mu.Lock()
	defer h.mu.Unlock()
	kept := us[:0]
	for _, u := range us {
		if u.Kind != hover.HoverDone || u.Req == h.latest {
			kept = append(kept, u)
		}
	}
	return kept
}
