package chroma_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"seam/internal/domain/diff"
	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
)

func TestAMultiLineCommentStaysAComment(t *testing.T) {
	text := "a := 1 /* open\nstill in it\n*/ b := 2\n"
	tok := highlight(t, lang.Go, text)

	assert.Equal(t, syntax.Comment, kindOf(t, tok, text, 0, "/* open"))
	assert.Equal(t, syntax.Comment, kindOf(t, tok, text, 1, "still in it"))
	assert.Equal(t, syntax.Comment, kindOf(t, tok, text, 2, "*/"))
	assert.Equal(t, syntax.Number, kindOf(t, tok, text, 2, "2"))
}

// TestALongLineKeepsTheState: a line of 10 KiB opens a block comment that
// a later short line closes. The long line itself keeps no runs, but the
// lexer read it, so the lines between are comment and those after code.
func TestALongLineKeepsTheState(t *testing.T) {
	long := "x := 1 /* " + strings.Repeat("y", 10<<10)
	text := "package a\n" + long + "\nstill a comment\n*/\nvar z = 3\n"
	tok := highlight(t, lang.Go, text)

	require.Equal(t, 5, tok.Lines())
	assert.Empty(t, tok.Line(1), "over MaxRunsLine: no runs")
	assert.Equal(t, syntax.Comment, kindOf(t, tok, text, 2, "still a comment"))
	assert.Equal(t, syntax.Comment, kindOf(t, tok, text, 3, "*/"))
	assert.Equal(t, syntax.Keyword, kindOf(t, tok, text, 4, "var"))
	assert.Equal(t, syntax.Number, kindOf(t, tok, text, 4, "3"))
}

// TestAVeryLongLineEndsTheColours: cut as the use case cuts it, from a line
// over 64 KiB on, the side has no lines covered, so no later line can have a
// kind from a wrong state.
func TestAVeryLongLineEndsTheColours(t *testing.T) {
	text := "package a\n/* " + strings.Repeat("y", 100<<10) + "\n*/\nvar z = 3\n"
	tok := highlight(t, lang.Go, text)

	assert.Equal(t, 1, tok.Lines())
	assert.Equal(t, syntax.Keyword, kindOf(t, tok, text, 0, "package"))
}

func TestTheLimitsCountTheContentOnly(t *testing.T) {
	runs := "var " + strings.Repeat("a", syntax.MaxRunsLine-4)
	tokenised := strings.Repeat("b", syntax.MaxLine)
	text := runs + "\r\n" + tokenised + "\r\n"
	tok := highlight(t, lang.Go, text)

	assert.Equal(t, 2, tok.Lines(), "a line of exactly MaxLine is read")
	assert.NotEmpty(t, tok.Line(0), "a line of exactly MaxRunsLine keeps its runs")
	assert.Empty(t, tok.Line(1))
}

func TestOffsetsHoldWithCRLFAndInvalidUTF8(t *testing.T) {
	tests := map[string]string{
		"crlf":             "var a = 1\r\nvar b = \"x\"\r\nvar c = 3\r\n",
		"invalid":          "var a = \"\xff\xfe\"\nvar b = 2 // \xc3\nvar c = 3\n",
		"no final newline": "var a = 1\nvar b = 2\nvar c = 3",
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			tok := highlight(t, lang.Go, text)

			require.Equal(t, 3, tok.Lines())
			for i := range 3 {
				assert.Equal(t, syntax.Keyword, kindOf(t, tok, text, i, "var"), "line %d", i)
			}
			assert.Equal(t, syntax.Number, kindOf(t, tok, text, 2, "3"))
		})
	}
}

func TestNoLexerSaysSo(t *testing.T) {
	h := newHighlighter(t)
	for _, l := range []lang.Lang{"", "cobol-in-space"} {
		tok, ok := h.Highlight(t.Context(), l, "text\n")
		assert.False(t, ok, l)
		assert.Zero(t, tok.Lines())
	}
	tok, ok := h.Highlight(t.Context(), lang.Go, "")
	assert.True(t, ok)
	assert.Zero(t, tok.Lines())
}

// countdown is a context that ends after its Err has been asked n times,
// so a test can stop the highlighter between two given tokens.
type countdown struct {
	context.Context
	n atomic.Int64
}

func (c *countdown) Err() error {
	if c.n.Add(-1) < 0 {
		return context.Canceled
	}
	return nil
}

// TestAnEndedContextStopsBetweenTokens: whenever ctx ends, the lines
// covered are those of the whole text, up to the line it stopped in.
func TestAnEndedContextStopsBetweenTokens(t *testing.T) {
	_, text := fixture(t, "sample.go")
	h := newHighlighter(t)
	full, _ := h.Highlight(t.Context(), lang.Go, text)
	lines := diff.Split(text).Lines

	for n := range int64(200) {
		ctx := &countdown{Context: t.Context()}
		ctx.n.Store(n)

		got, _ := h.Highlight(ctx, lang.Go, text)

		require.LessOrEqual(t, got.Lines(), full.Lines())
		for i := range got.Lines() {
			require.Equal(t, full.Line(i), got.Line(i), "after %d tokens, line %d", n, i)
		}
		if got.Lines() == len(lines) {
			return
		}
	}
	t.Fatal("never got through the file")
}

// TestRunsStayWithinTheirLines: whatever the text, in any language, every
// line is covered and no run reaches past its line.
func TestRunsStayWithinTheirLines(t *testing.T) {
	h := newHighlighter(t)
	pieces := []string{"a", " ", "\n", "\r\n", "\t", "\"", "'", "`", "/*", "*/", "//", "#", "<", ">", "<?php", "?>", "{", "}", "é", "👨‍👩", "\xff", "\xc3", "1.5", "\\"}
	rapid.Check(t, func(t *rapid.T) {
		l := rapid.SampledFrom(lang.Known()).Draw(t, "lang")
		text := strings.Join(rapid.SliceOf(rapid.SampledFrom(pieces)).Draw(t, "pieces"), "")

		tok, ok := h.Highlight(context.Background(), l, text)
		require.True(t, ok)

		lines := diff.Split(text).Lines
		require.Equal(t, len(lines), tok.Lines())
		for i, line := range lines {
			runs := tok.Line(i)
			if len(runs) > 0 {
				require.LessOrEqual(t, int(runs[len(runs)-1].End), len(line), "line %d %q", i, line)
			}
		}
	})
}
