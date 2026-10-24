package diff_test

import (
	"context"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"seam/internal/domain/diff"
)

// pieces cuts content into what lies outside and inside the spans, in turn:
// outside[i] comes before inside[i], and outside has one piece more.
func pieces(content string, spans []diff.Span) (outside, inside []string) {
	pos := 0
	for _, s := range spans {
		outside = append(outside, content[pos:s.Start])
		inside = append(inside, content[s.Start:s.End])
		pos = s.End
	}
	return append(outside, content[pos:]), inside
}

// bracket shows the spans of a line in brackets.
func bracket(line string, spans []diff.Span) string {
	outside, inside := pieces(diff.Content(line), spans)
	var b strings.Builder
	for i, in := range inside {
		b.WriteString(outside[i] + "[" + in + "]")
	}
	b.WriteString(outside[len(inside)])
	return b.String()
}

// inlineOf diffs two one-line files and returns the marked sides of the
// single row.
func inlineOf(t *testing.T, old, new string) (left, right string) {
	d := diff.Compute(t.Context(), diff.Split(old), diff.Split(new))
	require.Len(t, d.Rows, 1)
	r := d.Rows[0]
	return bracket(d.Old.Lines[r.Left], r.LeftInline), bracket(d.New.Lines[r.Right], r.RightInline)
}

func TestInline(t *testing.T) {
	tests := []struct {
		name                string
		old, new            string
		wantLeft, wantRight string
	}{
		{"the screenshot",
			"a finding, an investigation note, and a report the\n",
			"a finding and an investigation note.\n",
			"a finding[,] an investigation note[, and a report the]",
			"a finding [and ]an investigation note[.]"},
		{"a renamed identifier is marked whole",
			"\treturn oldName + 1\n", "\treturn newName + 1\n",
			"\treturn [oldName] + 1", "\treturn [newName] + 1"},
		{"an inserted word", "x := f(a)\n", "x := f(a, b)\n",
			"x := f(a)", "x := f(a[, b])"},
		{"multibyte runes stay whole", "größe := 1\n", "größe := 2\n",
			"größe := [1]", "größe := [2]"},
		{"only the line ending differs", "a b\r\n", "a b\n", "a b", "a b"},
		{"too much changed", "alpha beta\n", "gamma delta\n", "alpha beta", "gamma delta"},
		{"exactly 60 % is still marked", "abc d\n", "efg d\n", "[abc] d", "[efg] d"},
		{"just over 60 % is not", "abcd e\n", "fgh e\n", "abcd e", "fgh e"},
		{"a combining mark stays with its letter",
			"the cafe\u0301 is open\n", "the cafe is open\n",
			"the [cafe\u0301] is open", "the [cafe] is open"},
		{"joined emoji stay whole",
			"family: 👨\u200d👩 here\n", "family: 👨\u200d👧 here\n",
			"family: [👨\u200d👩] here", "family: [👨\u200d👧] here"},
		{"Japanese is cut into characters", "日本語のテキスト\n", "日本語のテクスト\n",
			"日本語のテ[キ]スト", "日本語のテ[ク]スト"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left, right := inlineOf(t, tt.old, tt.new)

			assert.Equal(t, tt.wantLeft, left)
			assert.Equal(t, tt.wantRight, right)
		})
	}
}

func TestInlineSkipsLongLines(t *testing.T) {
	long := strings.Repeat("word ", 1000)
	left, right := inlineOf(t, long[:4096]+"a\n", long[:4096]+"b\n")

	assert.NotContains(t, left, "[")
	assert.NotContains(t, right, "[")
	left, _ = inlineOf(t, long[:4095]+"a\n", long[:4095]+"b\n")
	assert.Contains(t, left, "[a]", "exactly 4 KiB is still marked")
}

func TestInlineStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// No line in common: the rows need no search, only the words would.
	d := diff.Compute(ctx, diff.Split("x y\n"), diff.Split("x z\n"))

	require.Equal(t, diff.Change, d.Rows[0].Kind)
	assert.Nil(t, d.Rows[0].LeftInline)
	assert.Nil(t, d.Rows[0].RightInline)
	assert.True(t, d.Simplified, "unmarked words are reported")
}

func TestInlineLeavesHopelessPairsToTheBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// The lengths alone differ by more than 60 %: nothing is lost by the end
	// of ctx, so nothing is reported.
	d := diff.Compute(ctx, diff.Split("x\n"), diff.Split("a much longer line\n"))

	require.Equal(t, diff.Change, d.Rows[0].Kind)
	assert.False(t, d.Simplified)
}

// whole reports that i does not split a character: it lies on a rune, not
// before a combining mark and not next to a zero width joiner.
func whole(content string, i int) bool {
	if i == 0 || i == len(content) {
		return true
	}
	if !utf8.RuneStart(content[i]) {
		return false
	}
	next, _ := utf8.DecodeRuneInString(content[i:])
	prev, _ := utf8.DecodeLastRuneInString(content[:i])
	return !unicode.Is(unicode.M, next) && next != '\u200d' && prev != '\u200d'
}

func checkSpans(t *rapid.T, content string, spans []diff.Span) {
	end := -1
	for _, s := range spans {
		require.Greater(t, s.Start, end, "spans are ordered and merged")
		require.Less(t, s.Start, s.End)
		require.LessOrEqual(t, s.End, len(content))
		require.True(t, whole(content, s.Start), "span starts on a character")
		require.True(t, whole(content, s.End), "span ends on a character")
		end = s.End
	}
}

func checkInline(t *rapid.T, old, new string, r diff.Row) {
	checkSpans(t, old, r.LeftInline)
	checkSpans(t, new, r.RightInline)
	if r.LeftInline == nil && r.RightInline == nil {
		return
	}
	outLeft, inLeft := pieces(old, r.LeftInline)
	outRight, inRight := pieces(new, r.RightInline)
	require.Equal(t, strings.Join(outLeft, ""), strings.Join(outRight, ""), "outside the spans both sides agree")
	changed := len(strings.Join(inLeft, "")) + len(strings.Join(inRight, ""))
	require.LessOrEqual(t, changed*5, (len(old)+len(new))*3, "at most 60 % marked")
}

func TestInlineSpansAreExactAndWhole(t *testing.T) {
	ctx := t.Context()
	// Invalid bytes, marks and joiners included; lines end differently, so a
	// block pairs lines of either ending.
	piece := rapid.SampledFrom([]string{"a", "b", "_", " ", "\t", ",", "é", "日", "\u0301", "\u200d", "\xff"})
	ending := rapid.SampledFrom([]string{"\n", "\r\n", ""})
	line := rapid.Custom(func(t *rapid.T) string {
		return strings.Join(rapid.SliceOfN(piece, 0, 20).Draw(t, "pieces"), "") + ending.Draw(t, "ending")
	})
	rapid.Check(t, func(t *rapid.T) {
		old := strings.Join(rapid.SliceOfN(line, 0, 4).Draw(t, "old"), "")
		new := strings.Join(rapid.SliceOfN(line, 0, 4).Draw(t, "new"), "")
		d := diff.Compute(ctx, diff.Split(old), diff.Split(new))
		for _, r := range d.Rows {
			if r.Kind == diff.Change {
				checkInline(t, diff.Content(d.Old.Lines[r.Left]), diff.Content(d.New.Lines[r.Right]), r)
			}
		}
	})
}
