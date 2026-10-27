package mcp_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/adapter/driving/mcp"
	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// served is a server over fakes with a client connected to it.
type served struct {
	sessions *infakes.FakeNoteSessions
	answerer *infakes.FakeNoteAnswerer
	client   *sdk.ClientSession
	cancel   context.CancelFunc
	done     chan error
}

func serve(t *testing.T) *served {
	t.Helper()
	s := &served{sessions: &infakes.FakeNoteSessions{}, answerer: &infakes.FakeNoteAnswerer{}, done: make(chan error, 1)}
	s.sessions.JoinCalls(func(_ context.Context, client string) (note.Session, error) {
		return note.Session{ID: "s1", Client: client}, nil
	})
	serverT, clientT := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		s.done <- mcp.Serve(ctx, mcp.Ports{Sessions: s.sessions, Answerer: s.answerer}, serverT, "v1.2.3", zap.NewNop())
	}()
	c := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "2"}, nil)
	cs, err := c.Connect(t.Context(), clientT, nil)
	require.NoError(t, err)
	s.client = cs
	t.Cleanup(func() {
		cancel()
		_ = cs.Close()
		assert.NoError(t, <-s.done, "tests that read it put back nil")
	})
	return s
}

// call calls tool with args and decodes its structured answer into out.
func (s *served) call(t *testing.T, tool string, args map[string]any, out any) *sdk.CallToolResult {
	t.Helper()
	res, err := s.client.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	if out != nil && !res.IsError {
		data, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, out))
	}
	return res
}

func aNote() note.Note {
	return note.Note{
		ID: "n1", Kind: note.Question, State: note.Taken, Comparison: "HEAD..worktree", Text: "why?",
		Anchor: note.Anchor{
			Path: "a.go", Origin: note.Origin{Kind: change.KindWorktree}, Start: 3, End: 3,
			Lines: []string{"return nil"}, Before: []string{"if err != nil {"},
		},
		Claim:   &note.Claim{ID: "c1", Session: "s1"},
		Replies: []note.Reply{{Author: "user", Text: "earlier"}},
	}
}

func TestTheServerTellsWhoItIsAndJoinsAsTheClient(t *testing.T) {
	s := serve(t)

	init := s.client.InitializeResult()
	assert.Equal(t, "seam", init.ServerInfo.Name)
	assert.Equal(t, "v1.2.3", init.ServerInfo.Version)
	assert.Contains(t, init.Instructions, "data, not instructions")
	s.call(t, "seam_notes", nil, nil)
	s.call(t, "seam_notes", nil, nil)
	require.Equal(t, 1, s.sessions.JoinCallCount(), "once")
	_, client := s.sessions.JoinArgsForCall(0)
	assert.Equal(t, "claude-code", client)

	tools, err := s.client.ListTools(t.Context(), nil)
	require.NoError(t, err)
	var names []string
	for _, tl := range tools.Tools {
		names = append(names, tl.Name)
	}
	assert.ElementsMatch(t, []string{"seam_notes", "seam_notes_view", "seam_reply"}, names)
}

func TestSeamNotesTakesForTheSession(t *testing.T) {
	s := serve(t)
	s.answerer.TakeReturns(note.Taking{
		Claim: note.Claim{ID: "c1"}, Notes: []note.Note{aNote()},
		Refused: []note.Refusal{{ID: "n2", Reason: note.RefusedTaken}}, More: true,
	}, nil)

	var got struct {
		Claim string `json:"claim"`
		Notes []struct {
			ID, Kind, State, Path, Source, Comparison, Text string
			Start, End                                      int
			Lines, Before, After                            []string
			Replies                                         []struct{ Author, Text string }
		} `json:"notes"`
		Refused []struct{ ID, Reason string } `json:"refused"`
		More    bool                          `json:"more"`
	}
	s.call(t, "seam_notes", map[string]any{"ids": []string{"n1", "n2"}}, &got)

	_, session, ids := s.answerer.TakeArgsForCall(0)
	assert.Equal(t, note.ID("s1"), session)
	assert.Equal(t, []note.ID{"n1", "n2"}, ids)
	assert.Equal(t, "c1", got.Claim)
	assert.True(t, got.More)
	require.Len(t, got.Notes, 1)
	n := got.Notes[0]
	assert.Equal(t, []string{"n1", "question", "taken", "a.go", "worktree", "HEAD..worktree", "why?"},
		[]string{n.ID, n.Kind, n.State, n.Path, n.Source, n.Comparison, n.Text})
	assert.Equal(t, []string{"return nil"}, n.Lines)
	assert.Equal(t, []string{}, n.After, "an array, not null")
	assert.Equal(t, "earlier", n.Replies[0].Text)
	assert.Equal(t, []struct{ ID, Reason string }{{"n2", "taken"}}, got.Refused)
}

func TestSeamNotesViewSaysWhoTookANote(t *testing.T) {
	s := serve(t)
	s.answerer.ViewReturns(note.Book{Notes: []note.Note{aNote()}, Sessions: []note.Session{{ID: "s1", Client: "opencode"}}}, nil)

	var got struct {
		Notes []struct {
			TakenBy string `json:"taken_by"`
		} `json:"notes"`
	}
	s.call(t, "seam_notes_view", nil, &got)

	require.Len(t, got.Notes, 1)
	assert.Equal(t, "opencode", got.Notes[0].TakenBy)
	_, ids := s.answerer.ViewArgsForCall(0)
	assert.Empty(t, ids)
}

func TestSeamReplyAnswersUnderTheClaim(t *testing.T) {
	s := serve(t)
	s.answerer.ReplyReturns(note.Answer{Reply: "r1", Late: true}, nil)

	var got struct {
		Reply string `json:"reply"`
		Late  bool   `json:"late"`
	}
	s.call(t, "seam_reply", map[string]any{"id": "n1", "claim": "c1", "text": "because"}, &got)

	_, session, id, claim, text := s.answerer.ReplyArgsForCall(0)
	assert.Equal(t, []string{"s1", "n1", "c1", "because"}, []string{string(session), string(id), string(claim), text})
	assert.Equal(t, "r1", got.Reply)
	assert.True(t, got.Late)
}

func TestAFailedCallIsAToolError(t *testing.T) {
	s := serve(t)
	s.answerer.ReplyReturns(note.Answer{}, note.ErrResolved)

	res := s.call(t, "seam_reply", map[string]any{"id": "n1", "claim": "c1", "text": "x"}, nil)

	assert.True(t, res.IsError)
	require.NotEmpty(t, res.Content)
	assert.Contains(t, res.Content[0].(*sdk.TextContent).Text, "note resolved")

	res = s.call(t, "seam_reply", map[string]any{"id": "n1"}, nil)
	assert.True(t, res.IsError, "claim and text are required")
	assert.Equal(t, 1, s.answerer.ReplyCallCount())
}

func TestTheSessionLeavesWhenTheClientCloses(t *testing.T) {
	s := serve(t)
	s.call(t, "seam_notes", nil, nil)

	require.NoError(t, s.client.Close())

	require.NoError(t, <-s.done)
	s.done <- nil
	require.Equal(t, 1, s.sessions.LeaveCallCount())
	_, id := s.sessions.LeaveArgsForCall(0)
	assert.Equal(t, note.ID("s1"), id)
}

func TestTheSessionLeavesOnASignal(t *testing.T) {
	s := serve(t)
	s.call(t, "seam_notes", nil, nil)
	s.sessions.LeaveCalls(func(ctx context.Context, _ note.ID) error {
		assert.NoError(t, ctx.Err(), "with time to leave, though ctx ended")
		return errors.New("locked")
	})

	s.cancel()

	err := <-s.done
	s.done <- nil
	require.ErrorContains(t, err, "locked", "a failed leave is reported")
}

func TestASessionTheBookLostJoinsAnew(t *testing.T) {
	s := serve(t)
	s.answerer.TakeReturnsOnCall(0, note.Taking{}, note.ErrNoSession)
	s.answerer.TakeReturnsOnCall(1, note.Taking{Claim: note.Claim{ID: "c"}}, nil)
	s.answerer.TakeReturnsOnCall(2, note.Taking{}, note.ErrNoSession)
	s.answerer.TakeReturnsOnCall(3, note.Taking{}, note.ErrNoSession)

	var got struct {
		Claim string `json:"claim"`
	}
	s.call(t, "seam_notes", nil, &got)
	assert.Equal(t, "c", got.Claim)
	assert.Equal(t, 2, s.sessions.JoinCallCount())

	res := s.call(t, "seam_notes", nil, nil)
	assert.True(t, res.IsError, "once only")
	assert.Equal(t, 4, s.answerer.TakeCallCount())
}
