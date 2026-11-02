package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/adapter/driving/mcp"
	"github.com/dturcinovic/seam/internal/domain/note"
)

const (
	initialize  = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"script","version":"1"}}}`
	initialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	listTools   = `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	reply       = `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"seam_reply","arguments":{"id":"n1","claim":"c1","text":"because"}}}`
)

// stdout is the server's stdout, read once it is done; with broken set,
// every write fails as into a pipe closed at the other end.
type stdout struct {
	bytes.Buffer
	broken bool
}

func (o *stdout) Write(p []byte) (int, error) {
	if o.broken {
		return 0, syscall.EPIPE
	}
	return o.Buffer.Write(p) //nolint:wrapcheck // a buffer does not fail
}

func (*stdout) Close() error { return nil }

// piped runs ServeStdio over the messages given at once, as a script piping
// them in does: stdin ends right after the last one. It returns the
// answers by id and Serve's error.
func piped(t *testing.T, p mcp.Ports, out *stdout, msgs ...string) (map[int]json.RawMessage, error) {
	t.Helper()
	in := io.NopCloser(strings.NewReader(strings.Join(msgs, "\n") + "\n"))
	err := mcp.ServeStdio(context.Background(), p, in, out, "v1.2.3", zap.NewNop())
	answers := map[int]json.RawMessage{}
	for line := range strings.Lines(out.String()) {
		var m struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		if m.ID != nil {
			answers[*m.ID] = m.Result
		}
	}
	return answers, err
}

func TestRequestsPipedInAreAnsweredBeforeTheEnd(t *testing.T) {
	answers, err := piped(t, newFakes().ports(), &stdout{}, initialize, initialized, listTools)

	require.NoError(t, err)
	require.Contains(t, answers, 1)
	assert.Contains(t, string(answers[1]), `"name":"seam"`)
	require.Contains(t, answers, 2)
	assert.Contains(t, string(answers[2]), `"seam_reply"`)
}

func TestAReplyPipedInIsMadeBeforeTheSessionLeaves(t *testing.T) {
	f := newFakes()
	f.answerer.ReplyReturns(note.Answer{Reply: "r1"}, nil)
	f.sessions.LeaveCalls(func(context.Context, note.ID) error {
		assert.Equal(t, 1, f.answerer.ReplyCallCount(), "the reply comes first")
		return nil
	})

	answers, err := piped(t, f.ports(), &stdout{}, initialize, initialized, reply)

	require.NoError(t, err)
	require.Contains(t, answers, 3)
	assert.Contains(t, string(answers[3]), `"r1"`)
	assert.Equal(t, 1, f.sessions.LeaveCallCount())
}

func TestTheEndWaitsNoLongerThanTheAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakes()
		f.answerer.ReplyReturns(note.Answer{Reply: "r1"}, nil)
		start := time.Now()

		answers, err := piped(t, f.ports(), &stdout{}, initialize, initialized, listTools, reply)

		require.NoError(t, err)
		assert.Len(t, answers, 3)
		assert.Zero(t, time.Since(start))
	})
}

func TestACallThatHangsHoldsTheEndOnlyForAWhile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFakes()
		f.answerer.ReplyCalls(func(ctx context.Context, _, _, _ note.ID, _ string) (note.Answer, error) {
			<-ctx.Done()
			return note.Answer{}, ctx.Err()
		})
		start := time.Now()

		answers, err := piped(t, f.ports(), &stdout{}, initialize, initialized, reply)

		require.NoError(t, err)
		assert.NotContains(t, answers, 3)
		assert.Equal(t, mcp.DrainWait, time.Since(start))
		assert.Equal(t, 1, f.sessions.LeaveCallCount())
	})
}

func TestABrokenStdoutIsAnErrorAndEndsAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()

		_, err := piped(t, newFakes().ports(), &stdout{broken: true}, initialize, initialized, listTools)

		require.Error(t, err)
		assert.True(t, errors.Is(err, syscall.EPIPE) || strings.Contains(err.Error(), "broken pipe"), err.Error())
		assert.Zero(t, time.Since(start))
	})
}
