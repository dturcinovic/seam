package lsp

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// JSON-RPC's answers to a method seam does not offer and to params it
// cannot read.
const (
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// message is any JSON-RPC message; which fields are set tells its kind.
type message struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      jsontext.Value `json:"id,omitzero"`
	Method  string         `json:"method,omitzero"`
	Params  jsontext.Value `json:"params,omitzero"`
	Result  jsontext.Value `json:"result,omitzero"`
	Error   *rpcError      `json:"error,omitzero"`
}

type rpcError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

func (m message) isRequest() bool      { return m.Method != "" && len(m.ID) > 0 }
func (m message) isNotification() bool { return m.Method != "" && len(m.ID) == 0 }

func request(id int64, method string, params any) ([]byte, error) {
	return outgoing(message{ID: jsontext.Value(fmt.Sprint(id)), Method: method}, params)
}

func notification(method string, params any) ([]byte, error) {
	return outgoing(message{Method: method}, params)
}

// call is a request or notification as sent. Params stay a value, so a
// document's text is encoded once, not once more as a raw message.
type call struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      jsontext.Value `json:"id,omitzero"`
	Method  string         `json:"method"`
	// Nil is left out, as shutdown and exit take no params.
	Params any `json:"params,omitzero"`
}

func outgoing(m message, params any) ([]byte, error) {
	body, err := json.Marshal(call{JSONRPC: "2.0", ID: m.ID, Method: m.Method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Method, err)
	}
	return body, nil
}

func response(id jsontext.Value, result any, e *rpcError) ([]byte, error) {
	m := message{JSONRPC: "2.0", ID: id, Error: e}
	if e == nil {
		r, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("result: %w", err)
		}
		m.Result = r
	}
	return json.Marshal(m)
}

// folder is a workspace folder as the protocol writes it.
type folder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// workspace is what a server may ask the client about: its folder and its
// settings.
type workspace struct {
	uri, name string
	settings  map[string]any
}

// answer is the client's side of a server request, after the plan's table
// of methods: only what seam does, and an honest refusal of the rest.
func (w workspace) answer(method string, params jsontext.Value) (any, *rpcError) {
	switch method {
	case "workspace/configuration":
		var p struct {
			Items []struct {
				Section string `json:"section"`
			} `json:"items"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: err.Error()}
		}
		out := make([]any, len(p.Items))
		for i, item := range p.Items {
			out[i] = section(w.settings, item.Section)
		}
		return out, nil
	case "workspace/workspaceFolders":
		return []folder{{URI: w.uri, Name: w.name}}, nil
	case "window/workDoneProgress/create", "window/showMessageRequest":
		// A void answer, which JSON-RPC still requires as a null result.
		return nil, nil
	case "workspace/applyEdit":
		return struct {
			Applied       bool   `json:"applied"`
			FailureReason string `json:"failureReason"`
		}{false, "seam is read-only"}, nil
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not supported: " + method}
}

// section looks up a dotted section in settings; the whole settings for an
// empty one, nil where nothing is set, which tells the server to use its
// default.
func section(settings map[string]any, name string) any {
	if name == "" {
		if settings == nil {
			return nil
		}
		return settings
	}
	var v any = settings
	for key := range strings.SplitSeq(name, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		if v, ok = m[key]; !ok {
			return nil
		}
	}
	return v
}

func initializeParams(pid int, w workspace, options map[string]any) map[string]any {
	p := map[string]any{
		"processId":        pid,
		"clientInfo":       map[string]string{"name": "seam"},
		"rootUri":          w.uri,
		"workspaceFolders": []folder{{URI: w.uri, Name: w.name}},
		// Only what seam implements; see the plan's LSP protocol.
		"capabilities": map[string]any{
			"general": map[string]any{"positionEncodings": []string{"utf-8", "utf-16"}},
			"textDocument": map[string]any{
				"hover":           map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
				"definition":      map[string]any{"linkSupport": true},
				"synchronization": map[string]any{"didSave": false, "willSave": false, "dynamicRegistration": false},
			},
			"workspace": map[string]any{"configuration": true, "workspaceFolders": true},
			"window":    map[string]any{"workDoneProgress": true},
		},
	}
	if options != nil {
		p["initializationOptions"] = options
	}
	return p
}

// positionEncoding is the one the server chose; utf-16 unless it says.
func positionEncoding(result jsontext.Value) string {
	var r struct {
		Capabilities struct {
			PositionEncoding string `json:"positionEncoding"`
		} `json:"capabilities"`
	}
	if json.Unmarshal(result, &r) == nil && r.Capabilities.PositionEncoding == "utf-8" {
		return "utf-8"
	}
	return "utf-16"
}

// character is the offset of byte b into line n of text, in the encoding,
// clamped to the line and to a whole character.
func character(text string, n, b int, encoding string) (int, bool) {
	line, ok := lineOf(text, n)
	if !ok {
		return 0, false
	}
	b = runeStart(line, min(max(b, 0), len(line)))
	if encoding == "utf-8" {
		return b, true
	}
	units := 0
	for _, r := range line[:b] {
		units += utf16.RuneLen(r)
	}
	return units, true
}

// runeStart is b, or before it the start of the character b is within.
func runeStart(s string, b int) int {
	for b > 0 && b < len(s) && !utf8.RuneStart(s[b]) {
		b--
	}
	return b
}

// lineOf returns line n of text without its line break.
func lineOf(text string, n int) (string, bool) {
	if n < 0 {
		return "", false
	}
	for range n {
		var ok bool
		if _, text, ok = strings.Cut(text, "\n"); !ok {
			return "", false
		}
	}
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSuffix(line, "\r"), true
}
