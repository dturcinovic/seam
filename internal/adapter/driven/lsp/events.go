package lsp

import (
	"slices"
	"sync"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

// maxResults bounds the hover answers waiting to be drained; the oldest
// fall out, as only the newest is of use to the viewer.
const maxResults = 16

// events holds what the servers reported until it is drained: the last
// status of each instance and the newest answers. Its signal is set after
// each, never blocking.
type events struct {
	signal chan struct{}

	mu      sync.Mutex
	pending []hover.Update
	// current is the client that speaks for each instance. One stopped in
	// the background, after an eviction, must not report Stopped over the
	// client that replaced it.
	current map[lang.Instance]*client
}

func newEvents() *events {
	return &events{signal: make(chan struct{}, 1), current: make(map[lang.Instance]*client)}
}

// of is the sink of c, the client that now speaks for its instance.
func (e *events) of(c *client) clientSink {
	e.mu.Lock()
	e.current[c.inst] = c
	e.mu.Unlock()
	return clientSink{e: e, c: c}
}

type clientSink struct {
	e *events
	c *client
}

// status checks and reports under one lock, so a client replaced meanwhile
// cannot slip a status in after its successor's.
func (s clientSink) status(i lang.Instance, st hover.ServerStatus) {
	s.e.mu.Lock()
	if s.e.current[i] != s.c {
		s.e.mu.Unlock()
		return
	}
	s.e.statusLocked(i, st)
	s.e.mu.Unlock()
	notify(s.e.signal)
}

func (s clientSink) result(u hover.Update) { s.e.result(u) }

func (e *events) status(i lang.Instance, s hover.ServerStatus) {
	e.mu.Lock()
	e.statusLocked(i, s)
	e.mu.Unlock()
	notify(e.signal)
}

// statusLocked replaces a status of i not yet drained with s.
func (e *events) statusLocked(i lang.Instance, s hover.ServerStatus) {
	e.pending = slices.DeleteFunc(e.pending, func(u hover.Update) bool {
		return u.Kind == hover.StatusChanged && u.Instance == i
	})
	e.pending = append(e.pending, hover.Update{Kind: hover.StatusChanged, Instance: i, Status: s})
}

func (e *events) result(u hover.Update) {
	e.mu.Lock()
	e.pending = append(e.pending, u)
	results := 0
	for _, p := range e.pending {
		if answer(p) {
			results++
		}
	}
	if results > maxResults {
		oldest := slices.IndexFunc(e.pending, answer)
		e.pending = slices.Delete(e.pending, oldest, oldest+1)
	}
	e.mu.Unlock()
	notify(e.signal)
}

// answer says u answers a request, rather than reporting a status.
func answer(u hover.Update) bool {
	return u.Kind == hover.HoverDone || u.Kind == hover.DefinitionDone
}

// Signal implements out.ServerEvents.
func (e *events) Signal() <-chan struct{} { return e.signal }

// Drain implements out.ServerEvents.
func (e *events) Drain() []hover.Update {
	e.mu.Lock()
	defer e.mu.Unlock()
	u := e.pending
	e.pending = nil
	return u
}
