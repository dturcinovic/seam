package lsp

import (
	"bufio"
	"encoding/json/v2"
	"fmt"
	"io"
	"strconv"

	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
)

// readLoop reads until the connection ends. It returns on EOF or when the
// pipe is closed, which ending the server does.
func (c *client) readLoop(out io.Reader) {
	r := bufio.NewReader(out)
	for {
		body, err := readFrame(r)
		if err != nil {
			c.fail(err)
			return
		}
		var m message
		if err := json.Unmarshal(body, &m); err != nil {
			c.fail(fmt.Errorf("%w: %w", errFrame, err))
			return
		}
		c.dispatch(m)
	}
}

func (c *client) dispatch(m message) {
	switch {
	case m.isRequest():
		result, rpcErr := c.ws.answer(m.Method, m.Params)
		body, err := response(m.ID, result, rpcErr)
		if err != nil {
			c.fail(err)
			return
		}
		c.push(frame{body: body})
	case m.isNotification():
		switch m.Method {
		case "window/logMessage", "window/showMessage":
			c.log.Debug("server message", zap.String("method", m.Method), zap.ByteString("params", m.Params))
		case "$/progress":
			if p, changed := c.works.take(m.Params); changed {
				c.sink.progress(c.inst, p)
			}
		}
	default:
		c.answered(m)
	}
}

// answered handles the response to a request of the client's. One that
// matches none was cancelled or timed out; its answer is of no use.
func (c *client) answered(m message) {
	id, err := strconv.ParseInt(string(m.ID), 10, 64)
	if err != nil {
		return
	}
	c.mu.Lock()
	p, ok := c.takeLocked(id)
	c.mu.Unlock()
	if !ok {
		return
	}
	switch p.kind {
	case pendingInit:
		c.initialized(m)
	case pendingHover:
		c.sink.result(hoverResult(p, m, c.languages))
	case pendingDefinition:
		c.sink.result(c.definitionResult(p, m))
	case pendingShutdown:
		close(c.shutdownDone)
	}
}

func (c *client) initialized(m message) {
	if m.Error != nil {
		c.fail(fmt.Errorf("initialize: %s", m.Error.Message))
		return
	}
	body, err := notification("initialized", map[string]any{})
	if err != nil {
		c.fail(err)
		return
	}
	c.mu.Lock()
	c.encoding = positionEncoding(m.Result)
	c.mu.Unlock()
	if c.push(frame{body: body, initialized: true}) {
		c.transition(hover.Starting, hover.Ready)
	}
}

func hoverResult(p *pending, m message, languages lang.Table) hover.Update {
	u := hover.Update{Kind: hover.HoverDone, Req: p.req, Version: p.version, Outcome: hover.Error}
	if m.Error != nil {
		return u
	}
	c, err := hoverContent(m.Result, languages)
	switch {
	case err != nil:
	case len(c.Blocks) == 0:
		u.Outcome = hover.Empty
	default:
		u.Outcome, u.Content = hover.Text, c
	}
	return u
}
