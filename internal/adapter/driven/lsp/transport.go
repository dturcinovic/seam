package lsp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
)

// maxFrame bounds the body of one message. A server that announces more
// is broken or hostile; reading it would hold that much in memory.
const maxFrame = 64 << 20

// errFrame is a frame that breaks the base protocol; the connection is
// lost after it.
var errFrame = errors.New("invalid frame")

// readFrame reads one message body: headers up to an empty line, of which
// Content-Length is required, then that many bytes.
func readFrame(r *bufio.Reader) ([]byte, error) {
	header, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		if errors.Is(err, io.EOF) && len(header) == 0 {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("%w: header: %w", errFrame, err)
	}
	length := strings.TrimSpace(header.Get("Content-Length"))
	if length == "" {
		return nil, fmt.Errorf("%w: no Content-Length", errFrame)
	}
	n, err := strconv.ParseInt(length, 10, 64)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("%w: Content-Length %q", errFrame, length)
	}
	if n > maxFrame {
		return nil, fmt.Errorf("%w: Content-Length %d over %d", errFrame, n, maxFrame)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("%w: body: %w", errFrame, err)
	}
	return body, nil
}

// writeFrame writes body as one message. Header and body go in two writes
// rather than one copy of the body: only the writer goroutine writes to a
// server, so nothing can come between them.
func writeFrame(w io.Writer, body []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}
