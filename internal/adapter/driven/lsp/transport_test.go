package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func frames(bodies ...string) string {
	var b strings.Builder
	for _, body := range bodies {
		fmt.Fprintf(&b, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	return b.String()
}

func readAll(t *testing.T, r io.Reader) ([]string, error) {
	t.Helper()
	br := bufio.NewReader(r)
	var got []string
	for {
		body, err := readFrame(br)
		if err != nil {
			return got, err
		}
		got = append(got, string(body))
	}
}

func TestReadFrame(t *testing.T) {
	stream := frames(`{"a":1}`, `{"b":"ü"}`, ``)
	tests := map[string]io.Reader{
		"several frames in one read": strings.NewReader(stream),
		"split frames":               iotest.OneByteReader(strings.NewReader(stream)),
		"half reads":                 iotest.HalfReader(strings.NewReader(stream)),
	}
	for name, r := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := readAll(t, r)
			assert.ErrorIs(t, err, io.EOF)
			assert.Equal(t, []string{`{"a":1}`, `{"b":"ü"}`, ``}, got)
		})
	}
}

func TestReadFrameWithOtherHeaders(t *testing.T) {
	stream := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\ncontent-length: 2\r\n\r\n{}"
	got, err := readAll(t, strings.NewReader(stream))
	assert.ErrorIs(t, err, io.EOF)
	assert.Equal(t, []string{"{}"}, got)
}

func TestReadFrameRejects(t *testing.T) {
	tests := map[string]string{
		"a header without a length":   "Content-Type: x\r\n\r\n{}",
		"a length that is no number":  "Content-Length: two\r\n\r\n{}",
		"a negative length":           "Content-Length: -1\r\n\r\n",
		"a length over 64 MiB":        fmt.Sprintf("Content-Length: %d\r\n\r\n", maxFrame+1),
		"a body cut short":            "Content-Length: 10\r\n\r\n{}",
		"headers cut short":           "Content-Length: 2\r\n",
		"a header line without colon": "nonsense\r\n\r\n{}",
	}
	for name, stream := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := readFrame(bufio.NewReader(strings.NewReader(stream)))
			assert.ErrorIs(t, err, errFrame)
		})
	}
}

func TestALengthOver64MiBReadsNoBody(t *testing.T) {
	r := &countingReader{r: strings.NewReader(fmt.Sprintf("Content-Length: %d\r\n\r\n", maxFrame+1) + strings.Repeat("x", 1<<20))}
	_, err := readFrame(bufio.NewReaderSize(r, 4096))
	require.ErrorIs(t, err, errFrame)
	assert.Less(t, r.n, 8192, "the body is never read")
}

func TestWriteFrameRoundTrips(t *testing.T) {
	var b bytes.Buffer
	require.NoError(t, writeFrame(&b, []byte(`{"jsonrpc":"2.0"}`)))
	require.NoError(t, writeFrame(&b, nil))
	assert.Equal(t, frames(`{"jsonrpc":"2.0"}`, ``), b.String())
}

type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}
