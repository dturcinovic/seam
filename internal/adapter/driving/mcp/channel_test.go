package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// noticeTransport is a client's transport that hands on the channel's
// notices it reads, as the SDK's client drops them.
type noticeTransport struct {
	sdk.Transport
	notices chan<- *jsonrpc.Request
}

func (t *noticeTransport) Connect(ctx context.Context) (sdk.Connection, error) { //nolint:ireturn // sdk.Transport prescribes it
	conn, err := t.Transport.Connect(ctx)
	return noticeConn{Connection: conn, notices: t.notices}, err
}

type noticeConn struct {
	sdk.Connection
	notices chan<- *jsonrpc.Request
}

func (c noticeConn) Read(ctx context.Context) (jsonrpc.Message, error) { //nolint:ireturn // sdk.Connection prescribes it
	msg, err := c.Connection.Read(ctx)
	if r, ok := msg.(*jsonrpc.Request); ok && r.Method == "notifications/claude/channel" {
		c.notices <- r
	}
	return msg, err
}

// listener is what the server followed the batches with, before it
// served the client.
func (s *served) listener(t *testing.T) in.BatchListener { //nolint:ireturn // the server hands itself out as the port's type only
	t.Helper()
	require.Equal(t, 1, s.batches.FollowCallCount())
	return s.batches.FollowArgsForCall(0)
}

func TestTheServerClaimsTheChannel(t *testing.T) {
	s := serve(t)

	assert.Equal(t, "2025-11-25", s.client.InitializeResult().ProtocolVersion, "a protocol with a stream the notices go over")

	caps := s.client.InitializeResult().Capabilities
	require.NotNil(t, caps)
	assert.Contains(t, caps.Experimental, "claude/channel")
	assert.Contains(t, s.client.InitializeResult().Instructions, "seam_notes with that batch")
}

func TestABatchSentToTheSessionBecomesANotice(t *testing.T) {
	s := serve(t)
	l := s.listener(t)
	id, ok := l.Session()
	require.True(t, ok)
	assert.Equal(t, note.ID("s1"), id)

	n := aNote()
	n.Anchor.End = 5
	l.Sent(t.Context(), note.Sending{Batch: note.Batch{ID: "b1"}, Notes: []note.Note{n, {ID: "n2", Kind: note.Finding, Anchor: note.Anchor{Path: "b.go", Start: 7, End: 7}}}})

	var got struct {
		Content string            `json:"content"`
		Meta    map[string]string `json:"meta"`
	}
	select {
	case r := <-s.notices:
		require.NoError(t, json.Unmarshal(r.Params, &got))
	case <-t.Context().Done():
		t.Fatal("no notice")
	}
	assert.Equal(t, map[string]string{"batch": "b1", "note_ids": `["n1","n2"]`}, got.Meta)
	assert.Contains(t, got.Content, `batch "b1"`)
	assert.Contains(t, got.Content, "- question \"a.go\":3-5\n- finding \"b.go\":7\n")
	assert.NotContains(t, got.Content, "why?", "the notes' text comes as data, with seam_notes")
}

func TestSeamNotesTakesABatch(t *testing.T) {
	s := serve(t)
	s.taker.TakeBatchReturns(note.Taking{Claim: note.Claim{ID: "c1"}, Refused: []note.Refusal{{ID: "n2", Reason: note.RefusedRebatched}}}, nil)

	var got struct {
		Claim   string                        `json:"claim"`
		Refused []struct{ ID, Reason string } `json:"refused"`
	}
	s.call(t, "seam_notes", map[string]any{"batch": "b1"}, &got)

	require.Equal(t, 1, s.taker.TakeBatchCallCount())
	_, session, batch := s.taker.TakeBatchArgsForCall(0)
	assert.Equal(t, []note.ID{"s1", "b1"}, []note.ID{session, batch})
	assert.Equal(t, "c1", got.Claim)
	assert.Equal(t, []struct{ ID, Reason string }{{"n2", "rebatched"}}, got.Refused)
	assert.Zero(t, s.answerer.TakeCallCount())

	res := s.call(t, "seam_notes", map[string]any{"batch": "b1", "ids": []string{"n1"}}, nil)
	assert.True(t, res.IsError, "ids and batch at once")
}

func TestABatchOfASessionLostIsTakenAsWaitingNotes(t *testing.T) {
	s := serve(t)
	s.taker.TakeBatchReturns(note.Taking{}, note.ErrNoSession)
	s.answerer.TakeReturns(note.Taking{Claim: note.Claim{ID: "c"}, Notes: []note.Note{aNote()}}, nil)

	var got struct {
		Notes []struct{ ID string } `json:"notes"`
	}
	s.call(t, "seam_notes", map[string]any{"batch": "b1"}, &got)

	assert.Equal(t, 2, s.sessions.JoinCallCount(), "joined anew")
	require.Equal(t, 1, s.answerer.TakeCallCount())
	_, _, ids := s.answerer.TakeArgsForCall(0)
	assert.Empty(t, ids)
	require.Len(t, got.Notes, 1)
}
