package textcell_test

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"seam/internal/textcell"
)

// cell is a Cell with what it shows, for readable expectations.
type cell struct {
	Shown      string
	Col, Width int
	Replaced   bool
}

func walk(text string, m textcell.Method) []cell {
	var got []cell
	for _, c := range cells(text, m) {
		got = append(got, cell{string(c.Append(nil, text)), c.Col, c.Width, c.Replaced})
	}
	return got
}

func TestCells(t *testing.T) {
	tests := map[string]struct {
		text string
		want []cell
	}{
		"ascii":               {"ab", []cell{{"a", 0, 1, false}, {"b", 1, 1, false}}},
		"combining é":         {"e\u0301x", []cell{{"e\u0301", 0, 1, false}, {"x", 1, 1, false}}},
		"cjk":                 {"日本", []cell{{"日", 0, 2, false}, {"本", 2, 2, false}}},
		"tab to stop":         {"ab\tc", []cell{{"a", 0, 1, false}, {"b", 1, 1, false}, {"  ", 2, 2, false}, {"c", 4, 1, false}}},
		"tab after wide":      {"日\tx", []cell{{"日", 0, 2, false}, {"  ", 2, 2, false}, {"x", 4, 1, false}}},
		"tab on stop":         {"abcd\t", []cell{{"a", 0, 1, false}, {"b", 1, 1, false}, {"c", 2, 1, false}, {"d", 3, 1, false}, {"    ", 4, 4, false}}},
		"esc":                 {"\x1b[", []cell{{"␛", 0, 1, true}, {"[", 1, 1, false}}},
		"bell":                {"\a", []cell{{"␇", 0, 1, true}}},
		"newline":             {"\r\n", []cell{{"␍", 0, 1, true}, {"␊", 1, 1, true}}},
		"del":                 {"\x7f", []cell{{"␡", 0, 1, true}}},
		"c1":                  {"\u009b", []cell{{`\x9B`, 0, 4, true}}},
		"invalid":             {"a\xff", []cell{{"a", 0, 1, false}, {`\xFF`, 1, 4, true}}},
		"invalid in grapheme": {"é\xcc", []cell{{"é", 0, 1, false}, {`\xCC`, 1, 4, true}}},
		"bidi":                {"\u202ex", []cell{{"<U+202E>", 0, 8, true}, {"x", 8, 1, false}}},
		"isolate":             {"\u2066", []cell{{"<U+2066>", 0, 8, true}}},
		"lone combining":      {"\u0301a", []cell{{"<U+0301>", 0, 8, true}, {"a", 8, 1, false}}},
		"zero width":          {"\u200b", []cell{{"<U+200B>", 0, 8, true}}},
		"bom":                 {"\ufeffx", []cell{{"<U+FEFF>", 0, 8, true}, {"x", 8, 1, false}}},
		"empty":               {"", nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			for _, m := range []textcell.Method{textcell.Codepoints, textcell.Graphemes} {
				assert.Equal(t, tt.want, walk(tt.text, m), "method %d", m)
			}
		})
	}
}

// TestMethodsDifferInWidthOnly: a ZWJ sequence is one grapheme either way,
// as wide as one glyph or as all its code points.
func TestMethodsDifferInWidthOnly(t *testing.T) {
	family := "👨\u200d👩\u200d👧"

	assert.Equal(t, []cell{{family, 0, 6, false}, {"x", 6, 1, false}}, walk(family+"x", textcell.Codepoints))
	assert.Equal(t, []cell{{family, 0, 2, false}, {"x", 2, 1, false}}, walk(family+"x", textcell.Graphemes))
}

func TestResumeContinuesTheWalk(t *testing.T) {
	text := "a\t日e\u0301\tb"
	all := cells(text, textcell.Codepoints)

	for i, from := range all {
		it := textcell.Resume(text, from.Start, from.Col, textcell.Codepoints)
		for _, want := range all[i:] {
			got, ok := it.Next()
			require.True(t, ok)
			assert.Equal(t, want, got)
		}
		_, ok := it.Next()
		assert.False(t, ok)
	}
}

func TestSkipStopsBeforeTheCellAcrossTheColumn(t *testing.T) {
	text := "ab日\tc"
	for col, want := range map[int]int{0: 0, 1: 1, 2: 2, 3: 2, 4: 3, 7: 3, 8: 4, 9: 5, 99: 5} {
		it := textcell.New(text, textcell.Codepoints)
		it.Skip(col)
		c, ok := it.Next()
		if want == 5 {
			assert.False(t, ok, "col %d", col)
			continue
		}
		assert.Equal(t, cells(text, textcell.Codepoints)[want], c, "col %d", col)
	}
}

// TestAGraphemeIsBounded: the marks past maxGrapheme bytes are cells of
// their own, so one letter with a megabyte of accents costs no more per
// cell than any other.
func TestAGraphemeIsBounded(t *testing.T) {
	zalgo := "e" + strings.Repeat("\u0301", 1000)

	all := cells(zalgo, textcell.Graphemes)

	require.Greater(t, len(all), 1)
	assert.LessOrEqual(t, all[0].End, 128)
	assert.Equal(t, 1, all[0].Width)
	assert.True(t, all[1].Replaced, "the rest has no base to join")
	assert.True(t, strings.HasPrefix(string(all[1].Append(nil, zalgo)), "<U+0301><U+0301>"))
}

func TestTheZeroValueWalksNothing(t *testing.T) {
	var it textcell.Cells
	_, ok := it.Next()

	assert.False(t, ok)
}

// TestWalkingAllocatesNothing walks text of every kind, stand-ins included.
func TestWalkingAllocatesNothing(t *testing.T) {
	text := strings.Repeat("ab\té日👨\u200d👩\x1b\u009b\xff\u202e\u200b", 50)
	buf := make([]byte, 0, 64)

	for _, m := range []textcell.Method{textcell.Codepoints, textcell.Graphemes} {
		allocs := testing.AllocsPerRun(10, func() {
			it := textcell.New(text, m)
			for c, ok := it.Next(); ok; c, ok = it.Next() {
				buf = c.Append(buf[:0], text)
			}
		})
		assert.Zero(t, allocs, "method %d", m)
	}
}

// alphabet mixes what segmenting, widths and stand-ins have to handle.
var alphabet = []string{
	"a", " ", "\t", "é", "e", "\u0301", "日", "👨", "\u200d", "👩", "\ufe0f",
	"🇩", "🇪", "\x1b", "\a", "\x7f", "\u009b", "\xff", "\xcc", "\u202e", "\u200b", "\ufeff", "\r", "\n",
}

func TestPropertiesOfTheWalk(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := strings.Join(rapid.SliceOf(rapid.SampledFrom(alphabet)).Draw(t, "pieces"), "")

		codepoints, graphemes := cells(text, textcell.Codepoints), cells(text, textcell.Graphemes)

		require.Len(t, graphemes, len(codepoints), "same graphemes under both methods")
		for i := range codepoints {
			require.Equal(t, codepoints[i].Start, graphemes[i].Start)
			require.Equal(t, codepoints[i].End, graphemes[i].End)
		}
		check(t, text, codepoints, ansi.StringWidthWc)
		check(t, text, graphemes, ansi.StringWidth)
		checkpoints(t, text, codepoints, graphemes)
		skips(t, text, codepoints, textcell.Codepoints)
		skips(t, text, graphemes, textcell.Graphemes)
	})
}

// checkpoints: each lies on a cell boundary at both columns the walks
// reach it with, the first one at or past each multiple of every.
func checkpoints(t *rapid.T, text string, codepoints, graphemes []textcell.Cell) {
	const every = 8
	var want []textcell.Checkpoint
	next := every
	for i, c := range codepoints {
		if c.Start >= next {
			want = append(want, textcell.Checkpoint{Byte: c.Start, Col: c.Col, GraphemeCol: graphemes[i].Col})
			next = (c.Start/every + 1) * every
		}
	}
	got, err := textcell.Checkpoints(context.Background(), text, every)
	require.NoError(t, err)
	if len(want) == 0 {
		require.Empty(t, got)
		return
	}
	require.Equal(t, want, got)
}

// skips: after Skip(col) the next cell is the first one that ends past col.
func skips(t *rapid.T, text string, all []textcell.Cell, m textcell.Method) {
	col := rapid.IntRange(0, 40).Draw(t, "col")
	it := textcell.New(text, m)
	it.Skip(col)
	got, ok := it.Next()
	for _, c := range all {
		if c.Col+c.Width > col {
			require.True(t, ok)
			require.Equal(t, c, got)
			return
		}
	}
	require.False(t, ok)
}

func cells(text string, m textcell.Method) []textcell.Cell {
	var all []textcell.Cell
	it := textcell.New(text, m)
	for c, ok := it.Next(); ok; c, ok = it.Next() {
		all = append(all, c)
	}
	return all
}

// check: the cells cover text in order, each is as wide as the terminal
// counts what it writes, and nothing written is a control or invalid.
func check(t *rapid.T, text string, all []textcell.Cell, width func(string) int) {
	pos, col := 0, 0
	for _, c := range all {
		require.Equal(t, pos, c.Start)
		require.Equal(t, col, c.Col)
		require.Positive(t, c.Width)
		shown := string(c.Append(nil, text))
		require.True(t, utf8.ValidString(shown), "%q", shown)
		for _, b := range []byte(shown) {
			require.False(t, b < 0x20 || b == 0x7f, "control %#x in %q", b, shown)
		}
		require.Equal(t, c.Width, width(shown), "width of %q", shown)
		pos, col = c.End, col+c.Width
	}
	require.Equal(t, len(text), pos)
}

func TestCheckpointsStopWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := textcell.Checkpoints(ctx, strings.Repeat("a", 100), 10)
	require.ErrorIs(t, err, context.Canceled)
	points, err := textcell.Checkpoints(t.Context(), "abc", 0)
	require.NoError(t, err)
	assert.Empty(t, points, "no step, no checkpoints")
}
