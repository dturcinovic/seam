package main

import (
	"bufio"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/notestore"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// agent talks JSON-RPC to seam mcp as a client of the protocol before the
// stateless one does, which is what Claude Code and OpenCode speak.
type agent struct {
	t     *testing.T
	in    *io.PipeWriter
	lines *bufio.Scanner
}

func (a agent) send(msg map[string]any) {
	a.t.Helper()
	msg["jsonrpc"] = "2.0"
	data, err := json.Marshal(msg)
	require.NoError(a.t, err)
	_, err = a.in.Write(append(data, '\n'))
	require.NoError(a.t, err)
}

// answer is the next line of stdout, which must be a JSON-RPC message.
func (a agent) answer() map[string]any {
	a.t.Helper()
	require.True(a.t, a.lines.Scan(), "a line on stdout")
	var msg map[string]any
	require.NoError(a.t, json.Unmarshal(a.lines.Bytes(), &msg), "only the protocol on stdout: %q", a.lines.Text())
	require.Equal(a.t, "2.0", msg["jsonrpc"])
	return msg
}

func (a agent) call(id int, tool string, args map[string]any) map[string]any {
	a.t.Helper()
	a.send(map[string]any{"id": id, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	res := a.answer()
	require.InDelta(a.t, id, res["id"], 0)
	result, ok := res["result"].(map[string]any)
	require.True(a.t, ok, "a result: %v", res)
	require.NotEqual(a.t, true, result["isError"], "%v", result)
	out, ok := result["structuredContent"].(map[string]any)
	require.True(a.t, ok)
	return out
}

// startMCP runs seam mcp in a new repository whose book holds question n1
// at a.go:1, and initializes it as claude-code; it hands back the
// initialize result and what seam mcp returned, once stdin closes.
func startMCP(t *testing.T) (agent, *notestore.Store, map[string]any, <-chan error) {
	t.Helper()
	repo := testsupport.NewGitRepo(t)
	store := notestore.New(notestore.Config{File: filepath.Join(repo.Dir, ".git", "seam", "notes.json")})
	require.NoError(t, store.Change(t.Context(), func(b *note.Book) error {
		a := note.Anchor{Path: "a.go", Start: 1, End: 1, Origin: note.Origin{Kind: change.KindWorktree}}
		return b.Add(note.Draft{Kind: note.Question, Text: "why?", Anchor: a}, "n1", time.Now())
	}))
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	cfg := config.Config{Dir: repo.Dir, LogFile: filepath.Join(t.TempDir(), "seam.log")}
	done := make(chan error, 1)
	go func() {
		done <- mcpSession(t.Context(), cfg, stdinR, stdoutW)
		_ = stdoutW.Close()
	}()
	a := agent{t: t, in: stdinW, lines: bufio.NewScanner(stdoutR)}
	a.send(map[string]any{"id": 0, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "claude-code", "version": "2"},
	}})
	init, ok := a.answer()["result"].(map[string]any)
	require.True(t, ok)
	a.send(map[string]any{"method": "notifications/initialized"})
	return a, store, init, done
}

func TestSeamMCPServesTheNotesOfTheWorktree(t *testing.T) {
	a, store, init, done := startMCP(t)
	assert.Equal(t, "seam", init["serverInfo"].(map[string]any)["name"])

	taken := a.call(1, "seam_notes", map[string]any{})
	notes := taken["notes"].([]any)
	require.Len(t, notes, 1)
	assert.Equal(t, "why?", notes[0].(map[string]any)["text"])
	answer := a.call(2, "seam_reply", map[string]any{"id": "n1", "claim": taken["claim"], "text": "because"})
	assert.Equal(t, false, answer["late"])

	require.NoError(t, a.in.Close())
	require.NoError(t, <-done)
	assert.False(t, a.lines.Scan(), "nothing more on stdout")

	b, err := store.Load(t.Context())
	require.NoError(t, err)
	assert.Equal(t, note.Answered, b.Notes[0].State)
	assert.Equal(t, "claude-code", b.Notes[0].Replies[0].Author)
	assert.Empty(t, b.Sessions, "left once stdin closed")
}

func TestSeamMCPNeedsARepository(t *testing.T) {
	cfg := config.Config{Dir: t.TempDir(), LogFile: filepath.Join(t.TempDir(), "seam.log")}
	err := mcpSession(t.Context(), cfg, nil, io.Discard)
	assert.Error(t, err)
}

func TestSeamMCPTellsItsSessionOfABatchOnce(t *testing.T) {
	a, store, init, done := startMCP(t)
	assert.Contains(t, init["capabilities"].(map[string]any)["experimental"], "claude/channel")
	var me note.ID
	require.Eventually(t, func() bool {
		b, err := store.Load(t.Context())
		if err != nil || len(b.Sessions) != 1 {
			return false
		}
		me = b.Sessions[0].ID
		return true
	}, 5*time.Second, 10*time.Millisecond, "joined once initialized")

	require.NoError(t, store.Change(t.Context(), func(b *note.Book) error {
		_, err := b.Send("b1", me, time.Now(), func(int) bool { return true }, change.Head{})
		return err
	}))
	notice := a.answer()
	require.Equal(t, "notifications/claude/channel", notice["method"])
	meta := notice["params"].(map[string]any)["meta"].(map[string]any)
	assert.Equal(t, map[string]any{"batch": "b1", "note_ids": `["n1"]`}, meta)

	// Another change of the book tells nothing again: the next line is the
	// answer to the call.
	require.NoError(t, store.Change(t.Context(), func(b *note.Book) error {
		return b.Add(note.Draft{Kind: note.Finding, Text: "later", Anchor: note.Anchor{Path: "b.go", Origin: note.Origin{Kind: change.KindWorktree}}}, "n2", time.Now())
	}))
	taken := a.call(1, "seam_notes", map[string]any{"batch": "b1"})
	notes := taken["notes"].([]any)
	require.Len(t, notes, 1)
	assert.Equal(t, "n1", notes[0].(map[string]any)["id"])

	require.NoError(t, a.in.Close())
	require.NoError(t, <-done)
	assert.False(t, a.lines.Scan(), "no second notice: %s", a.lines.Text())
}
