package lsp

import (
	"net/url"
	"path/filepath"
	"slices"

	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/supervise"
)

type pendingKind int

const (
	pendingInit pendingKind = iota + 1
	pendingHover
	pendingDefinition
	pendingShutdown
)

// asks says that a request of kind k asks about a position for the
// viewer: it counts among the open ones and is answered by an update.
func (k pendingKind) asks() bool { return k == pendingHover || k == pendingDefinition }

// method is the protocol's name of a request of kind k, which asks.
func (k pendingKind) method() string {
	if k == pendingDefinition {
		return "textDocument/definition"
	}
	return "textDocument/hover"
}

// pending is a request of the client's that awaits its answer. The
// JSON-RPC id stays inside the adapter; outside, a hover or a definition
// is its ReqID.
type pending struct {
	kind    pendingKind
	req     hover.ReqID
	version hover.DocVersion
	timer   *supervise.Timer
}

// call sends a request of the client's own through the mandatory queue.
func (c *client) call(kind pendingKind, method string, params any) error {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.pending[id] = &pending{kind: kind}
	c.mu.Unlock()
	body, err := request(id, method, params)
	if err != nil {
		return err
	}
	if !c.push(frame{body: body}) {
		return errOverload
	}
	return nil
}

// ask sends a request of kind at p, after the text of p's version, which
// the writer sends first if it has not yet. The oldest open request beyond
// maxOpenHovers ends with Error, as every Sent gets an answer.
func (c *client) ask(p hover.Position, kind pendingKind) hover.Submit {
	var dropped *pending
	// Runs after the unlock below; the sink must not be called under c.mu.
	defer func() {
		if dropped != nil {
			c.endHovers([]*pending{dropped}, hover.Error)
		}
	}()
	c.mu.Lock()
	defer c.mu.Unlock()
	d, refused := c.admitLocked(p)
	if refused != 0 {
		return refused
	}
	char, ok := character(d.Text, p.Line, p.Byte, c.encoding)
	if !ok {
		return hover.Unsupported
	}
	c.nextID++
	id := c.nextID
	method := kind.method()
	body, err := request(id, method, map[string]any{
		"textDocument": map[string]string{"uri": c.uri(p.Path)},
		"position":     map[string]int{"line": p.Line, "character": char},
	})
	if err != nil {
		c.log.Error("request", zap.String("method", method), zap.Error(err))
		return hover.Failed
	}
	select {
	case c.requests <- body:
	default:
		return hover.Busy
	}
	entry := &pending{kind: kind, req: p.Req, version: p.Version}
	c.pending[id] = entry
	c.hovers = append(c.hovers, id)
	if len(c.hovers) > maxOpenHovers {
		dropped = c.dropLocked(c.hovers[0])
	}
	// The answer cannot overtake this: the reader takes c.mu to find it.
	if t, err := c.group.AfterFunc(hoverTimeout, "hover timeout", func() { c.timeout(id) }); err == nil {
		entry.timer = t
	}
	return hover.Sent
}

// admitLocked returns the text a hover at p is about, or why the client
// cannot take it now.
func (c *client) admitLocked(p hover.Position) (desiredDoc, hover.Submit) {
	switch c.state {
	case hover.Starting, hover.Stopped:
		return desiredDoc{}, hover.NotReady
	case hover.Missing:
		// The pool starts no client for a missing server.
		return desiredDoc{}, hover.Unsupported
	case hover.ServerFailed:
		return desiredDoc{}, hover.Failed
	case hover.Ready:
	}
	if c.stopping {
		return desiredDoc{}, hover.NotReady
	}
	d, ok := c.docs.get(p.Path)
	if !ok {
		return desiredDoc{}, hover.Unsupported
	}
	if d.Version != p.Version {
		return desiredDoc{}, hover.Stale
	}
	return d, 0
}

// cancel drops the hover req, if it is still open.
func (c *client) cancel(req hover.ReqID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range c.hovers {
		if c.pending[id].req == req {
			c.dropLocked(id)
			return
		}
	}
}

func (c *client) timeout(id int64) {
	c.mu.Lock()
	p, ok := c.pending[id]
	if ok {
		c.dropLocked(id)
	}
	c.mu.Unlock()
	if ok {
		c.sink.result(p.ended(hover.TimedOut))
	}
}

// dropLocked forgets the open hover id and tells the server. Cancelling is
// optional in the protocol, so with the request queue full only the notice
// is lost: a late answer finds no request and is dropped.
func (c *client) dropLocked(id int64) *pending {
	p, _ := c.takeLocked(id)
	body, err := notification("$/cancelRequest", map[string]int64{"id": id})
	if err != nil {
		return p
	}
	select {
	case c.requests <- body:
	default:
		c.log.Info("cancel not sent: request queue full", zap.Int64("id", id))
	}
	return p
}

// takeLocked removes the pending request id and stops its timer.
func (c *client) takeLocked(id int64) (*pending, bool) {
	p, ok := c.pending[id]
	if !ok {
		return nil, false
	}
	delete(c.pending, id)
	if p.timer != nil {
		p.timer.Stop()
	}
	if p.kind.asks() {
		c.hovers = slices.DeleteFunc(c.hovers, func(h int64) bool { return h == id })
	}
	return p, true
}

// takeHoversLocked removes every open hover.
func (c *client) takeHoversLocked() []*pending {
	var open []*pending
	for _, id := range slices.Clone(c.hovers) {
		if p, ok := c.takeLocked(id); ok {
			open = append(open, p)
		}
	}
	return open
}

func (c *client) endHovers(open []*pending, o hover.Outcome) {
	for _, p := range open {
		c.sink.result(p.ended(o))
	}
}

// ended is the answer to p when it ends without the server's, with o.
func (p *pending) ended(o hover.Outcome) hover.Update {
	kind := hover.HoverDone
	if p.kind == pendingDefinition {
		kind = hover.DefinitionDone
	}
	return hover.Update{Kind: kind, Req: p.req, Version: p.version, Outcome: o}
}

func (c *client) uri(path string) string {
	return fileURI(filepath.Join(c.repoRoot, filepath.FromSlash(path)))
}

func fileURI(abs string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}
