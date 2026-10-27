package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/domain/note"
)

// channelMethod is the notice Claude Code takes into a session started
// with the channel; other clients, and Claude Code without it, drop it
// without a word, so nothing tells seam that it arrived.
const channelMethod = "notifications/claude/channel"

// errNotConnected is a notice before the client connected.
var errNotConnected = errors.New("no client connected")

// channelTransport is a transport that keeps its connection, so the
// server can write a notice of its own into the same stream: the SDK sends
// only the protocol's notifications. The connection serializes writes, so
// the notice never splits an answer.
type channelTransport struct {
	sdk.Transport

	mu   sync.Mutex
	conn sdk.Connection
}

// Connect connects t and keeps the connection.
func (t *channelTransport) Connect(ctx context.Context) (sdk.Connection, error) { //nolint:ireturn // sdk.Transport prescribes it
	conn, err := t.Transport.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	t.mu.Lock()
	t.conn = conn
	t.mu.Unlock()
	return conn, nil
}

// statefulOnly turns down server/discover, as a server from before the
// stateless protocol does, so the client initializes instead. Claude Code
// delivers a channel's notices only over the stream of an initialized
// session; over the stateless protocol it shows "no channel delivery
// path" and drops them.
func statefulOnly(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if method == "server/discover" {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "seam mcp speaks the protocol before server/discover"}
		}
		return next(ctx, method, req)
	}
}

// close lets go of the connection once the server is done with it; it
// waits for a notice being written.
func (t *channelTransport) close() {
	t.mu.Lock()
	t.conn = nil
	t.mu.Unlock()
}

// notify writes n as the channel's notice, a request without ID. It holds
// the lock while writing, so close waits for it.
func (t *channelTransport) notify(ctx context.Context, n channelNotice) error {
	raw, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("encode notice: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return errNotConnected
	}
	if err := t.conn.Write(ctx, &jsonrpc.Request{Method: channelMethod, Params: raw}); err != nil {
		return fmt.Errorf("write notice: %w", err)
	}
	return nil
}

// channelNotice is the notice's params. Claude Code shows meta as
// attributes of the message, which take strings alone, so the ids go as
// JSON in a string.
type channelNotice struct {
	Content string            `json:"content"`
	Meta    map[string]string `json:"meta"`
}

// Session implements in.BatchListener: the client's session, once joined.
func (s *server) Session() (note.ID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.joined, s.joined != ""
}

// Sent implements in.BatchListener: it tells the client of the batch. A
// notice that fails is logged and not tried again; sending anew in seam
// makes a new batch.
func (s *server) Sent(ctx context.Context, sending note.Sending) {
	ids := make([]string, len(sending.Notes))
	for i, n := range sending.Notes {
		ids[i] = string(n.ID)
	}
	idsJSON, _ := json.Marshal(ids) // strings always encode
	notice := channelNotice{
		Content: noticeText(sending),
		Meta:    map[string]string{"batch": string(sending.Batch.ID), "note_ids": string(idsJSON)},
	}
	if err := s.channel.notify(ctx, notice); err != nil {
		s.log.Warn("tell the agent of a batch", zap.String("batch", string(sending.Batch.ID)), zap.Error(err))
		return
	}
	s.log.Info("told the agent of a batch", zap.String("batch", string(sending.Batch.ID)), zap.Int("notes", len(ids)))
}

// Unread implements in.BatchListener: the process has no screen, so the
// log says why no batch was told.
func (s *server) Unread(err error) {
	s.log.Warn("read the notes for batches", zap.Error(err))
}

// noticeText says what to do and lists the notes by kind and place only:
// their text comes with seam_notes, as data.
func noticeText(s note.Sending) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The user sent %d review notes from seam. Take them with seam_notes and batch %q, then answer each with seam_reply.\n", len(s.Notes), s.Batch.ID)
	for _, n := range s.Notes {
		a := n.Anchor
		// %q: a path is text of the repository, which may hold a line
		// break and words that read like an instruction.
		fmt.Fprintf(&b, "- %s %q:%d", n.Kind, a.Path, a.Start)
		if a.End > a.Start {
			fmt.Fprintf(&b, "-%d", a.End)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
