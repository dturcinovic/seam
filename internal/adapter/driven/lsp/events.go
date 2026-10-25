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

func (s clientSink) status(i lang.Instance, st hover.ServerStatus) {
	s.report(i, func() bool { s.e.statusLocked(i, st); return true })
}

func (s clientSink) result(u hover.Update) { s.e.result(u) }

// progress reports only while the client is ready: its reader goes on
// reading while it stops, and work after Stopped would stay on screen.
func (s clientSink) progress(i lang.Instance, p hover.Progress) {
	s.report(i, func() bool {
		if s.c.status() != hover.Ready {
			return false
		}
		s.e.progressLocked(i, p)
		return true
	})
}

// report checks and reports under one lock, so a client replaced meanwhile
// cannot slip an update in after its successor's.
func (s clientSink) report(i lang.Instance, add func() bool) {
	s.e.mu.Lock()
	added := s.e.current[i] == s.c && add()
	s.e.mu.Unlock()
	if added {
		notify(s.e.signal)
	}
}

func (e *events) status(i lang.Instance, s hover.ServerStatus) {
	e.mu.Lock()
	e.statusLocked(i, s)
	e.mu.Unlock()
	notify(e.signal)
}

// statusLocked replaces a status of i not yet drained with s. Its work
// ends with the connection; said here, as Stopped may be replaced by a
// restart's Ready before the viewer drains it.
func (e *events) statusLocked(i lang.Instance, s hover.ServerStatus) {
	e.pending = slices.DeleteFunc(e.pending, func(u hover.Update) bool {
		return u.Kind == hover.StatusChanged && u.Instance == i
	})
	e.pending = append(e.pending, hover.Update{Kind: hover.StatusChanged, Instance: i, Status: s})
	if s == hover.Stopped || s == hover.ServerFailed {
		e.progressLocked(i, hover.Progress{})
	}
}

// progressLocked adds p for i. It replaces a report of work just before
// it, so a viewer that drains late finds no pile of percentages; the end
// of work is kept, as a hover waiting for it asks again then, and nothing
// moves past an answer.
func (e *events) progressLocked(i lang.Instance, p hover.Progress) {
	if n := len(e.pending); n > 0 {
		last := e.pending[n-1]
		if last.Kind == hover.ProgressChanged && last.Instance == i && last.Progress != (hover.Progress{}) {
			e.pending = e.pending[:n-1]
		}
	}
	e.pending = append(e.pending, hover.Update{Kind: hover.ProgressChanged, Instance: i, Progress: p})
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
