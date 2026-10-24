// Command fakelsp is a language server for seam's tests that does what its
// script says, misbehaviour included. It logs every message it reads as
// its JSON body, one line each, so a test can see what reached it and in
// which order.
//
// Its framing is its own, not the adapter's: a fault shared by both sides
// would pass unseen.
//
// It reads its script from fakelsp.script and logs to fakelsp.log, both in
// its working directory, which is the root of its workspace: so instances
// of one server configuration can each be told something else.
//
// Commands, one per line:
//
//	silent               answer nothing, initialize included
//	encoding utf-8       choose utf-8 positions
//	hover <text>         answer hovers with text (default: an empty result);
//	                     a text in double quotes is a Go string literal
//	hover-delay <dur>    wait that long before each hover answer
//	no-shutdown          leave shutdown unanswered and ignore exit
//	stop-reading         after initialize, read nothing more
//	broken-frame         after initialize, write a frame without a length
//	crash                after initialize, exit with status 3
//	child                after initialize, start a child that holds the pipes
//	ask <method> <json>  after initialized, send this request to the client
package main

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type script struct {
	silent, noShutdown, stopReading, brokenFrame, crash, child bool
	encoding, hover                                            string
	hoverDelay                                                 time.Duration
	asks                                                       []ask
}

type ask struct {
	method string
	params jsontext.Value
}

type message struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      jsontext.Value `json:"id,omitzero"`
	Method  string         `json:"method,omitzero"`
	Params  jsontext.Value `json:"params,omitzero"`
	Result  jsontext.Value `json:"result,omitzero"`
	Error   jsontext.Value `json:"error,omitzero"`
}

func main() {
	text, err := os.ReadFile("fakelsp.script")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "fakelsp:", err)
		os.Exit(2)
	}
	s, err := parse(string(text))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakelsp:", err)
		os.Exit(2)
	}
	log, err := os.OpenFile("fakelsp.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakelsp:", err)
		os.Exit(2)
	}
	os.Exit(serve(s, bufio.NewReader(os.Stdin), os.Stdout, log))
}

func parse(text string) (script, error) {
	var s script
	for line := range strings.Lines(text) {
		cmd, arg, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch cmd {
		case "":
		case "silent":
			s.silent = true
		case "encoding":
			s.encoding = arg
		case "hover":
			s.hover = arg
			if strings.HasPrefix(arg, `"`) {
				var err error
				if s.hover, err = strconv.Unquote(arg); err != nil {
					return s, err
				}
			}
		case "hover-delay":
			d, err := time.ParseDuration(arg)
			if err != nil {
				return s, err
			}
			s.hoverDelay = d
		case "no-shutdown":
			s.noShutdown = true
		case "stop-reading":
			s.stopReading = true
		case "broken-frame":
			s.brokenFrame = true
		case "crash":
			s.crash = true
		case "child":
			s.child = true
		case "ask":
			method, params, _ := strings.Cut(arg, " ")
			s.asks = append(s.asks, ask{method: method, params: jsontext.Value(params)})
		default:
			return s, fmt.Errorf("unknown command %q", cmd)
		}
	}
	return s, nil
}

func serve(s script, in *bufio.Reader, out io.Writer, log io.Writer) int {
	next := 1000
	for {
		m, body, err := read(in)
		if err != nil {
			logf(log, "eof")
			return 0
		}
		logf(log, "%s", body)
		if s.silent {
			continue
		}
		switch m.Method {
		case "initialize":
			reply(out, m.ID, initializeResult(s.encoding))
			if code, done := afterInitialize(s, out); done {
				return code
			}
		case "initialized":
			for _, a := range s.asks {
				next++
				write(out, message{JSONRPC: "2.0", ID: jsontext.Value(strconv.Itoa(next)), Method: a.method, Params: a.params})
			}
		case "textDocument/hover":
			time.Sleep(s.hoverDelay)
			reply(out, m.ID, hoverResult(s.hover))
		case "shutdown":
			if !s.noShutdown {
				reply(out, m.ID, nil)
			}
		case "exit":
			if !s.noShutdown {
				return 0
			}
		}
	}
}

// afterInitialize does the misbehaviour the script asks for once the
// client thinks the server is up.
func afterInitialize(s script, out io.Writer) (code int, done bool) {
	switch {
	case s.crash:
		return 3, true
	case s.brokenFrame:
		say(out, "Content-Type: x\r\n\r\n{}")
	case s.child:
		// The child inherits stdin and stdout and outlives this process
		// unless its group is ended.
		cmd := exec.CommandContext(context.Background(), "sleep", "3600")
		cmd.Stdin, cmd.Stdout = os.Stdin, os.Stdout
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "fakelsp: child:", err)
		}
	}
	if s.stopReading {
		// Not select{}: with no other goroutine the runtime would call that
		// a deadlock and exit, which is not a server that stopped reading.
		for {
			time.Sleep(time.Hour)
		}
	}
	return 0, false
}

func initializeResult(encoding string) any {
	caps := map[string]any{"hoverProvider": true, "textDocumentSync": 2}
	if encoding != "" {
		caps["positionEncoding"] = encoding
	}
	return map[string]any{"capabilities": caps, "serverInfo": map[string]string{"name": "fakelsp"}}
}

func hoverResult(text string) any {
	if text == "" {
		return nil
	}
	return map[string]any{"contents": map[string]string{"kind": "markdown", "value": text}}
}

func read(in *bufio.Reader) (message, []byte, error) {
	var length int
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return message{}, nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
			length, _ = strconv.Atoi(v) // a bad length reads as an empty body, which fails below
		}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(in, body); err != nil {
		return message{}, nil, err
	}
	var m message
	return m, body, json.Unmarshal(body, &m)
}

func reply(out io.Writer, id jsontext.Value, result any) {
	r, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakelsp:", err)
		return
	}
	write(out, message{JSONRPC: "2.0", ID: id, Result: r})
}

func write(out io.Writer, m message) {
	body, err := json.Marshal(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakelsp:", err)
		return
	}
	say(out, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

// logf and say write to the log and the client. A failed write means the
// client is gone, which the next read finds out; there is nothing else to
// do about it.
func logf(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format+"\n", args...) }

func say(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
