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

func TestSeamMCPServesTheNotesOfTheWorktree(t *testing.T) {
	repo := testsupport.NewGitRepo(t)
	file := filepath.Join(repo.Dir, ".git", "seam", "notes.json")
	store := notestore.New(notestore.Config{File: file})
	require.NoError(t, store.Change(t.Context(), func(b *note.Book) error {
		return b.Add(note.Draft{Kind: note.Question, Text: "why?", Anchor: note.Anchor{Path: "a.go", Origin: note.Origin{Kind: change.KindWorktree}}}, "n1", time.Now())
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
	init := a.answer()
	assert.Equal(t, "seam", init["result"].(map[string]any)["serverInfo"].(map[string]any)["name"])
	a.send(map[string]any{"method": "notifications/initialized"})

	taken := a.call(1, "seam_notes", map[string]any{})
	notes := taken["notes"].([]any)
	require.Len(t, notes, 1)
	assert.Equal(t, "why?", notes[0].(map[string]any)["text"])
	answer := a.call(2, "seam_reply", map[string]any{"id": "n1", "claim": taken["claim"], "text": "because"})
	assert.Equal(t, false, answer["late"])

	require.NoError(t, stdinW.Close())
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
