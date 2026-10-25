package chroma

import (
	"context"
	"strings"
	"testing"

	chromalib "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/domain/syntax"
)

// coalesced merges adjacent tokens of one type and drops empty ones, so
// two lexings that cut the same text differently compare equal.
func coalesced(it chromalib.Iterator) []chromalib.Token {
	var out []chromalib.Token
	for t := it(); t != chromalib.EOF; t = it() {
		switch {
		case t.Value == "":
		case len(out) > 0 && out[len(out)-1].Type == t.Type:
			out[len(out)-1].Value += t.Value
		default:
			out = append(out, t)
		}
	}
	return out
}

// TestTheDelegationLexesAsChromaDoes: for any mix of HTML and PHP without
// interpolation, the lazy delegation yields what chroma's own PHTML lexer
// does; with it, chroma's goes wrong, see TestAnInterpolationStaysPHP.
func TestTheDelegationLexesAsChromaDoes(t *testing.T) {
	php, err := phpInHTML()
	require.NoError(t, err)
	theirs := lexers.Get("phtml")
	pieces := []string{"<?php ", "<?= ", " ?>", "<p class=\"a\">", "</p>", "<script>", "</script>", "$x", " echo ", "\"s\"", "/* c */", "// c", "\n", "<", ">", "text", "é", "\xff", "42;"}
	options := &chromalib.TokeniseOptions{State: "root"}
	rapid.Check(t, func(t *rapid.T) {
		text := strings.Join(rapid.SliceOf(rapid.SampledFrom(pieces)).Draw(t, "pieces"), "")
		want, err := theirs.Tokenise(options, text)
		require.NoError(t, err)

		got, err := php.tokenise(context.Background(), options, text)

		require.NoError(t, err)
		require.Equal(t, coalesced(want), coalesced(got))
	})
}

// TestAnEndedContextStopsThePHPPass: the inner language is lexed in full
// before the first token, so ctx is checked there.
func TestAnEndedContextStopsThePHPPass(t *testing.T) {
	h := newInternal(t, zap.NewNop())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	text := strings.Repeat("<?php echo $x; ?>\n<p>hi</p>\n", 100)

	cut, _ := h.Highlight(ctx, "php", text)
	full, _ := h.Highlight(t.Context(), "php", text)

	assert.Zero(t, cut.Lines())
	assert.Equal(t, 200, full.Lines())
}

// TestAnInterpolationStaysPHP: "{$...}" in a PHP string is lexed by PHP's
// own rules, not as HTML, which chroma's PHTML does and which would turn a
// "<!--" in it into a comment over all the HTML after.
func TestAnInterpolationStaysPHP(t *testing.T) {
	h := newInternal(t, zap.NewNop())
	text := "<?php $s = \"Hi {$name} {$a['<!--']}\"; ?>\n<p class=\"c\">x</p>\n"

	tok, ok := h.Highlight(t.Context(), "php", text)

	require.True(t, ok)
	require.Equal(t, 2, tok.Lines())
	at := strings.Index(text, "$name")
	assert.Equal(t, syntax.Variable, kindAt(tok.Line(0), at))
	assert.Equal(t, syntax.Tag, kindAt(tok.Line(1), strings.Index(text[strings.Index(text, "\n")+1:], "p")))
	assert.Equal(t, syntax.Attribute, kindAt(tok.Line(1), strings.Index(text[strings.Index(text, "\n")+1:], "class")))
}

func kindAt(runs []syntax.Run, at int) syntax.Kind {
	for _, r := range runs {
		if at < int(r.End) {
			return r.Kind
		}
	}
	return syntax.Plain
}
