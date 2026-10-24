package lsp

import (
	"bufio"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

// peer is the server side of an in-process connection. A synctest bubble
// counts goroutines waiting on an io.Pipe as blocked, so its clock runs,
// which a real pipe to a process would stop.
type peer struct {
	in   *bufio.Reader
	out  io.Writer
	done chan struct{}
}

// next reads the next message, or reports false once the connection ends.
func (p *peer) next() (message, bool) {
	body, err := readFrame(p.in)
	if err != nil {
		return message{}, false
	}
	var m message
	if json.Unmarshal(body, &m) != nil {
		return message{}, false
	}
	return m, true
}

func (p *peer) send(m message) {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err == nil {
		_ = writeFrame(p.out, body) // the client may be gone; the test sees that elsewhere
	}
}

func (p *peer) reply(id jsontext.Value, result string) {
	p.send(message{ID: id, Result: jsontext.Value(result)})
}

// serveUntil answers initialize and shutdown and ends on exit, handing
// every other message to handle; handle reports false to stop serving.
func (p *peer) serveUntil(handle func(m message) bool) {
	for {
		m, ok := p.next()
		if !ok {
			return
		}
		switch m.Method {
		case "initialize":
			p.reply(m.ID, `{"capabilities":{}}`)
		case "shutdown":
			p.reply(m.ID, "null")
		case "exit":
			return
		default:
			if !handle(m) {
				return
			}
		}
	}
}

// pipes starts peers instead of processes, each running serve.
type pipes struct {
	serve func(p *peer)
	// gate, if set, holds every start until it closes.
	gate chan struct{}

	mu      sync.Mutex
	live    int
	maxLive int
	started int
}

func (f *pipes) find(command string) (string, error) {
	if command == "missing" {
		return "", errMissing
	}
	return command, nil
}

func (f *pipes) start(lang.Server, string) (*conn, error) {
	if f.gate != nil {
		<-f.gate
	}
	fromClient, toServer := io.Pipe()
	fromServer, toClient := io.Pipe()
	p := &peer{in: bufio.NewReader(fromClient), out: toClient, done: make(chan struct{})}
	var once sync.Once
	end := func() {
		once.Do(func() {
			_ = fromClient.Close()
			_ = toClient.Close()
			close(p.done)
		})
	}
	f.mu.Lock()
	f.live++
	f.started++
	f.maxLive = max(f.maxLive, f.live)
	f.mu.Unlock()
	go func() {
		defer func() {
			f.mu.Lock()
			f.live--
			f.mu.Unlock()
		}()
		defer end()
		f.serve(p)
	}()
	return &conn{
		in: toServer, out: fromServer, pid: 1,
		terminate: end, kill: end,
		wait: func() error { <-p.done; return nil },
	}, nil
}

func (f *pipes) counts() (live, maxLive, started int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live, f.maxLive, f.started
}

// served answers initialize and shutdown and nothing else.
func served(p *peer) { p.serveUntil(func(message) bool { return true }) }

// goTable maps .go files to one server with root marker go.mod.
func goTable(t *testing.T, s lang.Server) lang.Table {
	t.Helper()
	if s.Name == "" {
		s.Name = "fake"
	}
	if s.Command == "" {
		s.Command = "fake"
	}
	table, err := lang.NewTable([]lang.Language{{Lang: lang.Go, Extensions: []string{".go"}, Server: s.Name}}, []lang.Server{s})
	require.NoError(t, err)
	return table
}

func testPool(t *testing.T, table lang.Table, s starter) *Pool {
	t.Helper()
	return newPool(Config{Root: t.TempDir(), LogDir: t.TempDir()}, table, s, zap.NewNop(), nil)
}

func doc(path string, v hover.DocVersion, text string) hover.Document {
	return hover.Document{Path: path, Lang: lang.Go, Version: v, Text: text}
}

func hasStatus(s hover.ServerStatus) func([]hover.Update) bool {
	return func(all []hover.Update) bool { return countStatus(all, s) > 0 }
}

func countStatus(all []hover.Update, s hover.ServerStatus) int {
	n := 0
	for _, u := range all {
		if u.Kind == hover.StatusChanged && u.Status == s {
			n++
		}
	}
	return n
}

func hasResult(all []hover.Update) bool {
	for _, u := range all {
		if u.Kind == hover.HoverDone {
			return true
		}
	}
	return false
}

func results(all []hover.Update) []hover.Update {
	var r []hover.Update
	for _, u := range all {
		if u.Kind == hover.HoverDone {
			r = append(r, u)
		}
	}
	return r
}
