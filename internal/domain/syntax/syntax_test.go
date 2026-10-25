package syntax_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/domain/syntax"
)

func TestBuilder(t *testing.T) {
	var b syntax.Builder
	require.NoError(t, b.Add(3, syntax.Keyword))
	require.NoError(t, b.Add(3, syntax.String)) // empty, ignored
	require.NoError(t, b.Add(4, syntax.Plain))
	require.NoError(t, b.Add(6, syntax.Keyword))
	require.NoError(t, b.Add(8, syntax.Keyword)) // merges
	require.NoError(t, b.Add(9, syntax.Plain))   // trailing plain, dropped
	b.EndLine(0)
	b.EndLine(0) // a line without runs
	require.NoError(t, b.Add(2, syntax.Comment))
	b.EndLine(0)
	require.NoError(t, b.Add(5, syntax.Comment)) // not ended: not covered

	got := b.Tokens()

	assert.Equal(t, 3, got.Lines())
	assert.Equal(t, []syntax.Run{{3, syntax.Keyword}, {4, syntax.Plain}, {8, syntax.Keyword}}, got.Line(0))
	assert.Empty(t, got.Line(1))
	assert.Equal(t, []syntax.Run{{2, syntax.Comment}}, got.Line(2), "a kind does not merge across lines")
	assert.Nil(t, got.Line(3))
	assert.Nil(t, got.Line(-1))
	assert.Positive(t, got.ApproxBytes())
}

func TestBuilderRejectsBadRuns(t *testing.T) {
	var b syntax.Builder
	require.NoError(t, b.Add(4, syntax.Keyword))
	assert.Error(t, b.Add(3, syntax.Keyword), "backwards")
	assert.Error(t, b.Add(5, syntax.Kinds), "no kind")
}

func TestZeroTokensCoverNothing(t *testing.T) {
	var tok syntax.Tokens
	assert.Equal(t, 0, tok.Lines())
	assert.Nil(t, tok.Line(0))
	assert.Zero(t, tok.ApproxBytes())
}

// TestLineDoesNotShareCapacity: appending to one line's runs must not
// overwrite the next line's.
func TestLineDoesNotShareCapacity(t *testing.T) {
	var b syntax.Builder
	require.NoError(t, b.Add(1, syntax.Keyword))
	b.EndLine(0)
	require.NoError(t, b.Add(1, syntax.String))
	b.EndLine(0)
	tok := b.Tokens()

	_ = append(tok.Line(0), syntax.Run{End: 9, Kind: syntax.Number})

	assert.Equal(t, []syntax.Run{{1, syntax.String}}, tok.Line(1))
}

// TestBuilderKeepsEveryByteOfItsKind: whatever the runs added, each byte of
// a line reads back with the kind it was added with, past the runs as plain.
func TestBuilderKeepsEveryByteOfItsKind(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lines := rapid.SliceOfN(rapid.SliceOfN(rapid.Uint8Range(0, uint8(syntax.Kinds-1)), 0, 30), 0, 10).Draw(t, "lines")
		var b syntax.Builder
		for _, kinds := range lines {
			for i, k := range kinds {
				require.NoError(t, b.Add(i+1, syntax.Kind(k)))
			}
			b.EndLine(0)
		}
		tok := b.Tokens()
		require.Equal(t, len(lines), tok.Lines())
		for i, kinds := range lines {
			runs := tok.Line(i)
			for j := range len(kinds) + 3 {
				want := syntax.Plain
				if j < len(kinds) {
					want = syntax.Kind(kinds[j])
				}
				require.Equal(t, want, kindAt(runs, j), "line %d byte %d", i, j)
			}
			for j := 1; j < len(runs); j++ {
				require.NotEqual(t, runs[j-1].Kind, runs[j].Kind, "adjacent runs differ")
			}
			if len(runs) > 0 {
				require.NotEqual(t, syntax.Plain, runs[len(runs)-1].Kind, "no trailing plain")
			}
		}
	})
}

func kindAt(runs []syntax.Run, at int) syntax.Kind {
	for _, r := range runs {
		if at < int(r.End) {
			return r.Kind
		}
	}
	return syntax.Plain
}

func TestALineOverMaxRunsLineKeepsNoRuns(t *testing.T) {
	var b syntax.Builder
	require.NoError(t, b.Add(3, syntax.Keyword))
	b.EndLine(syntax.MaxRunsLine + 1)
	require.NoError(t, b.Add(3, syntax.Keyword))
	b.EndLine(syntax.MaxRunsLine)
	tok := b.Tokens()

	assert.Equal(t, 2, tok.Lines())
	assert.Empty(t, tok.Line(0))
	assert.Equal(t, []syntax.Run{{3, syntax.Keyword}}, tok.Line(1))
	assert.Error(t, b.Add(syntax.MaxLine+3, syntax.Keyword), "no line is that long")
}

func TestLexableStopsBeforeTheFirstVeryLongLine(t *testing.T) {
	fits := strings.Repeat("a", syntax.MaxLine)
	tests := map[string]struct{ text, want string }{
		"short":             {"a\nb\n", "a\nb\n"},
		"exactly the limit": {fits + "\r\nb\n", fits + "\r\nb\n"},
		"one over":          {"a\n" + fits + "b\nc\n", "a\n"},
		"the last line":     {"a\n" + fits + "b", "a\n"},
	}
	for name, tt := range tests {
		assert.Equal(t, tt.want, syntax.Lexable(tt.text), name)
	}
}
