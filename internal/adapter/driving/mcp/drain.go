package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// drainWait bounds the wait for a call that hangs; with leaveWait after it,
// seam mcp ends at most 8s after the client's stream does.
const drainWait = 5 * time.Second

// drainTransport holds back the end of the client's stream until every
// call read before it is answered: the SDK (go-sdk v1.7.0, jsonrpc2
// readIncoming) cancels the calls in flight as soon as its reader ends, so
// a client that writes its requests and closes its end at once, as a
// script piping them in does, got no answer. Serve connects it once.
type drainTransport struct {
	sdk.Transport
	*drainState
}

// Connect connects t and holds back the end of what it reads.
func (t drainTransport) Connect(ctx context.Context) (sdk.Connection, error) { //nolint:ireturn // sdk.Transport prescribes it
	conn, err := t.Transport.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	return &drainConn{Connection: conn, drainState: t.drainState}, nil
}

// drainState is what the connection learns of the stream's end.
type drainState struct {
	log *zap.Logger
	// stop ends the wait early: Serve's ctx, as the SDK reads without one
	// that ends.
	stop <-chan struct{}

	mu sync.Mutex
	// open holds the ids of the calls read and not answered. A call whose
	// id is open already the SDK turns down without an answer.
	open        map[jsonrpc.ID]struct{}
	gaveUp      bool
	writeFailed bool
	// settled is closed and replaced once no call is open or a write failed.
	settled chan struct{}
}

func newDrainState(stop <-chan struct{}, log *zap.Logger) *drainState {
	return &drainState{log: log, stop: stop, open: map[jsonrpc.ID]struct{}{}, settled: make(chan struct{})}
}

// leftACall tells whether the wait gave up on a call that hung, while every
// answer could be written: the SDK then reports the orderly end as an error
// of its own, "server is closing: EOF".
func (s *drainState) leftACall() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gaveUp && !s.writeFailed
}

// settle wakes the wait; s.mu is held.
func (s *drainState) settle() {
	close(s.settled)
	s.settled = make(chan struct{})
}

func (s *drainState) drain() {
	timeout := time.NewTimer(drainWait)
	defer timeout.Stop()
	for {
		s.mu.Lock()
		open, done, settled := len(s.open), len(s.open) == 0 || s.writeFailed, s.settled
		s.mu.Unlock()
		if done {
			return
		}
		select {
		case <-settled:
		case <-s.stop:
			return
		case <-timeout.C:
			s.log.Warn("calls left unanswered at the end of the client's stream", zap.Int("calls", open))
			s.mu.Lock()
			s.gaveUp = true
			s.mu.Unlock()
			return
		}
	}
}

// drainConn keeps the calls read until their answers are written.
type drainConn struct {
	sdk.Connection
	*drainState
}

// Read reads the next message; at the end of the stream it first waits for
// the open calls.
func (c *drainConn) Read(ctx context.Context) (jsonrpc.Message, error) { //nolint:ireturn // sdk.Connection prescribes it
	msg, err := c.Connection.Read(ctx)
	if errors.Is(err, io.EOF) {
		c.drain()
	}
	if err != nil {
		return nil, err //nolint:wrapcheck // the SDK tells the end of the stream by io.EOF
	}
	if req, ok := msg.(*jsonrpc.Request); ok && req.IsCall() {
		c.mu.Lock()
		c.open[req.ID] = struct{}{}
		c.mu.Unlock()
	}
	return msg, nil
}

// Write writes msg and lets go of the call it answers.
func (c *drainConn) Write(ctx context.Context, msg jsonrpc.Message) error {
	err := c.Connection.Write(ctx, msg)
	res, answer := msg.(*jsonrpc.Response)
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case err != nil && ctx.Err() == nil && !c.writeFailed:
		// No answer gets through any more; a write whose ctx ended, as the
		// SDK holds too, says nothing of the stream.
		c.writeFailed = true
		c.settle()
	case answer:
		if _, ok := c.open[res.ID]; ok {
			delete(c.open, res.ID)
			if len(c.open) == 0 {
				c.settle()
			}
		}
	}
	return err //nolint:wrapcheck // passed through as the SDK wrote it
}
