package chroma

import (
	"context"
	"errors"
	"testing"

	chromalib "github.com/alecthomas/chroma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"seam/internal/domain/lang"
)

func newInternal(t *testing.T, log *zap.Logger) *Highlighter {
	t.Helper()
	h, err := New(log)
	require.NoError(t, err)
	return h
}

func TestEachLanguageHasItsLexer(t *testing.T) {
	want := map[lang.Lang]string{
		lang.Go: "Go", lang.Rust: "Rust", lang.JavaScript: "JavaScript", lang.TypeScript: "TypeScript",
		lang.JSX: "react", lang.TSX: "TypeScript", lang.HTML: "HTML", lang.PHP: "PHTML", lang.Python: "Python",
	}
	h := newInternal(t, zap.NewNop())
	require.Len(t, want, len(lang.Defaults().Languages()))
	for l, name := range want {
		lx, ok := h.lexerFor(l)
		require.True(t, ok, l)
		assert.Equal(t, name, lx.name, l)
	}
}

// failing is a lexer whose iterator yields some tokens and then fails the
// way chroma's do, or fails before it starts.
func failing(before bool) lexer {
	return lexer{name: "failing", tokenise: func(context.Context, *chromalib.TokeniseOptions, string) (chromalib.Iterator, error) {
		return failingTokens(before)
	}}
}

func failingTokens(before bool) (chromalib.Iterator, error) {
	if before {
		return nil, errors.New("no rules")
	}
	tokens := []chromalib.Token{{Type: chromalib.Keyword, Value: "var"}, {Type: chromalib.Text, Value: " a\n"}, {Type: chromalib.Keyword, Value: "var"}}
	return func() chromalib.Token {
		if len(tokens) == 0 {
			panic("boom") // chroma reports an error in an iterator so
		}
		t := tokens[0]
		tokens = tokens[1:]
		return t
	}, nil
}

func TestAFailingLexerKeepsTheLinesBeforeAndIsLogged(t *testing.T) {
	tests := []struct {
		name      string
		lexer     lexer
		wantLines int
	}{
		{"panic in the iterator", failing(false), 1},
		{"error before", failing(true), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := walker{text: "var a\nvar b\n", valid: true}

			err := w.run(t.Context(), tt.lexer)

			require.Error(t, err)
			assert.Equal(t, tt.wantLines, w.b.Tokens().Lines())
		})
	}
}

func TestTheHighlighterLogsWhyItStopped(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	h := newInternal(t, zap.New(core))
	h.own["failing"] = failing(false)

	tok, ok := h.Highlight(t.Context(), "failing", "var a\nvar b\n")

	assert.True(t, ok)
	assert.Equal(t, 1, tok.Lines())
	require.Equal(t, 1, logs.Len())
	entry := logs.All()[0]
	assert.Equal(t, "failing", entry.ContextMap()["lexer"])
	assert.EqualValues(t, 2, entry.ContextMap()["line"])
	assert.Contains(t, entry.ContextMap()["error"], "boom")
}

// TestATokenThatIsNotTheTextStops: offsets would shift after a token whose
// text differs from the source, so the colours stop there instead.
func TestATokenThatIsNotTheTextStops(t *testing.T) {
	lx := lexer{name: "dropping", tokenise: func(context.Context, *chromalib.TokeniseOptions, string) (chromalib.Iterator, error) {
		return chromalib.Literator(
			chromalib.Token{Type: chromalib.Keyword, Value: "var a\n"},
			chromalib.Token{Type: chromalib.Keyword, Value: "b\n"}, // "var " dropped
		), nil
	}}
	w := walker{text: "var a\nvar b\n", valid: true}

	err := w.run(t.Context(), lx)

	require.Error(t, err)
	assert.Equal(t, 1, w.b.Lines())
}
