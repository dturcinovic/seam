package lsp

import (
	"context"
	"io"
	"strings"

	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/domain/hover"
)

// writer is the state of the writer goroutine, which alone writes to the
// server: a full pipe blocks it and nothing else.
type writer struct {
	c  *client
	in io.Writer
	// open is set once initialized has gone out; documents wait for it.
	open bool
	// stale is set when the documents may have changed since the last
	// reconcile, so a request or reply alone costs no comparison.
	stale bool
	// sent is the version of each document as the server has it.
	sent map[string]hover.DocVersion
}

// writeLoop writes in the plan's order: mandatory frames first, then the
// documents the server is to have, then requests, each request only after
// the documents, so a hover never goes out before the text it is about.
func (c *client) writeLoop(ctx context.Context, in io.Writer) {
	w := &writer{c: c, in: in, sent: make(map[string]hover.DocVersion), stale: true}
	for {
		if !w.flush() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case f := <-c.mandatory:
			if !w.write(f) {
				return
			}
		case <-c.dirty:
			w.stale = true
		case body := <-c.requests:
			if !w.flush() || !w.write(frame{body: body}) {
				return
			}
		}
	}
}

// flush writes every mandatory frame waiting, then brings the documents
// up to date. It reports false once the connection is lost.
func (w *writer) flush() bool {
	for drained := false; !drained; {
		select {
		case f := <-w.c.mandatory:
			if !w.write(f) {
				return false
			}
		default:
			drained = true
		}
	}
	select {
	case <-w.c.dirty:
		w.stale = true
	default:
	}
	if !w.open || !w.stale {
		return true
	}
	w.stale = false
	return w.reconcile()
}

func (w *writer) write(f frame) bool {
	if err := writeFrame(w.in, f.body); err != nil {
		w.c.fail(err)
		return false
	}
	if f.initialized {
		w.open = true
		notify(w.c.dirty)
	}
	return true
}

// reconcile sends what moves the server's documents to the desired ones:
// closes first, then opens and changes, in path order. A version that was
// replaced before it went out is never sent. Each message is encoded just
// before it is written, so a reopen of many large documents holds one of
// them encoded at a time.
func (w *writer) reconcile() bool {
	closed, changed := w.c.docs.since(w.sent)
	for _, path := range closed {
		body, err := notification("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": w.c.uri(path)}})
		if err == nil && !w.write(frame{body: body}) {
			return false
		}
		delete(w.sent, path)
	}
	for _, d := range changed {
		body, err := w.didOpenOrChange(d)
		if err != nil {
			// Marked sent all the same, or it would fail again on every
			// round; hovers on it end with the server's error.
			w.c.log.Error("document not sent", zap.String("path", d.Path), zap.Error(err))
		} else if !w.write(frame{body: body}) {
			return false
		}
		w.sent[d.Path] = d.Version
	}
	return true
}

func (w *writer) didOpenOrChange(d desiredDoc) ([]byte, error) {
	uri := w.c.uri(d.Path)
	// The protocol carries UTF-8 only, and the encoder refuses anything
	// else; a file in another encoding goes with replacement characters.
	text := strings.ToValidUTF8(d.Text, "\uFFFD")
	if _, open := w.sent[d.Path]; open {
		// The whole text without a range, which the protocol takes as the
		// full content whatever sync kind the server asked for.
		return notification("textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": d.Version},
			"contentChanges": []map[string]string{{"text": text}},
		})
	}
	return notification("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": d.languageID, "version": d.Version, "text": text},
	})
}
