package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/dturcinovic/seam/internal/textcell"
)

const noteLimit = 8 << 10

// keyOf is the key press that Bubble Tea reports as name: modifiers, then a
// named key or one character.
func keyOf(name string) tea.Key {
	named := map[string]rune{
		"left": tea.KeyLeft, "right": tea.KeyRight, "up": tea.KeyUp, "down": tea.KeyDown,
		"home": tea.KeyHome, "end": tea.KeyEnd, "backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "space": tea.KeySpace, "pgdown": tea.KeyPgDown,
	}
	var k tea.Key
	for {
		switch {
		case strings.HasPrefix(name, "ctrl+") && len(name) > len("ctrl+"):
			k.Mod |= tea.ModCtrl
			name = name[len("ctrl+"):]
			continue
		case strings.HasPrefix(name, "alt+") && len(name) > len("alt+"):
			k.Mod |= tea.ModAlt
			name = name[len("alt+"):]
			continue
		}
		break
	}
	if code, ok := named[name]; ok {
		k.Code = code
		if code == tea.KeySpace && k.Mod == 0 {
			k.Text = " "
		}
		return k
	}
	k.Code, _ = utf8.DecodeRuneInString(name)
	if k.Mod == 0 {
		k.Text = name
	}
	return k
}

// fieldOn is a screen with the field open on it, 28 columns of text wide, holding
// text with the cursor at the bar in it.
func fieldOn(t *testing.T, marked string) *state {
	t.Helper()
	s := fieldScreen(t, 28)
	s.openField(noteLimit)
	at := strings.IndexByte(marked, '|')
	require.GreaterOrEqual(t, at, 0, "no cursor in %q", marked)
	s.Field.Text, s.Field.Cursor = marked[:at]+marked[at+1:], at
	s.layoutField()
	return s
}

// fieldScreen is a screen whose field has cols columns of text.
func fieldScreen(t *testing.T, cols int) *state {
	t.Helper()
	w := cols
	for from, to := screen(t, w).gapColumns(); to-from < cols+popupFrame+2*fieldPad; from, to = screen(t, w).gapColumns() {
		w++
	}
	return screen(t, w)
}

// marked is the field's text with a bar at its cursor.
func marked(f field) string {
	return f.Text[:f.Cursor] + "|" + f.Text[f.Cursor:]
}

func press(s *state, keys ...string) {
	for _, k := range keys {
		s.fieldKey(keyOf(k))
	}
}

func TestTheFieldEdits(t *testing.T) {
	family := "👨\u200d👩\u200d👧"
	tests := map[string]struct {
		before string
		keys   []string
		after  string
	}{
		"typing":                       {"|", []string{"a", "space", "b"}, "a b|"},
		"left over a combined letter":  {"cafe\u0301|", []string{"left"}, "caf|e\u0301"},
		"left over a ZWJ sequence":     {"x" + family + "|", []string{"left"}, "x|" + family},
		"right over a wide character":  {"|日本", []string{"right"}, "日|本"},
		"right over a line break":      {"a|\nb", []string{"right"}, "a\n|b"},
		"left over a line break":       {"a\n|b", []string{"left"}, "a|\nb"},
		"backspace takes the grapheme": {"x" + family + "|", []string{"backspace"}, "x|"},
		"delete takes the grapheme":    {"|e\u0301x", []string{"delete"}, "|x"},
		"backspace at the start":       {"|ab", []string{"backspace"}, "|ab"},
		"delete at the end":            {"ab|", []string{"delete"}, "ab|"},
		"alt+left to the word's start": {"foo bar|  baz", []string{"alt+left"}, "foo |bar  baz"},
		"ctrl+left past blanks":        {"foo bar  |baz", []string{"ctrl+left"}, "foo |bar  baz"},
		"alt+right to the word's end":  {"foo| bar.x", []string{"alt+right"}, "foo bar|.x"},
		"ctrl+right over punctuation":  {"foo|..bar", []string{"ctrl+right"}, "foo..|bar"},
		"word jumps cross lines":       {"foo\n|bar", []string{"alt+left"}, "|foo\nbar"},
		"alt+backspace takes a word":   {"foo bar|", []string{"alt+backspace"}, "foo |"},
		"ctrl+w takes a word":          {"foo bar |", []string{"ctrl+w"}, "foo |"},
		"alt+enter breaks the line":    {"a|b", []string{"alt+enter"}, "a\n|b"},
		"home and end of the line":     {"ab|c\nd", []string{"home", "x", "end", "y"}, "xabcy|\nd"},
		"down keeps the column":        {"abcd|ef\nab\nabcdef", []string{"down", "down"}, "abcdef\nab\nabcd|ef"},
		"up stays in the first row":    {"ab|c", []string{"up"}, "ab|c"},
		"down stays in the last row":   {"ab|c", []string{"down"}, "ab|c"},
		"ctrl keys are no text":        {"a|", []string{"ctrl+x", "alt+x"}, "a|"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := fieldOn(t, tt.before)

			press(s, tt.keys...)

			assert.Equal(t, tt.after, marked(s.Field))
		})
	}
}

func TestTheFieldMovesThroughWrappedRows(t *testing.T) {
	// 28 columns a row: the second word goes to the next row whole.
	s := fieldOn(t, "|"+strings.Repeat("a", 20)+" "+strings.Repeat("b", 10))
	require.Len(t, s.Field.Rows, 2)

	press(s, "end")
	assert.Equal(t, 20, s.Field.Cursor, "the end of a wrapped row stops before its last grapheme")
	press(s, "down")
	assert.Equal(t, 21+10, s.Field.Cursor, "the goal column lies past the next row's end")
	press(s, "home")
	assert.Equal(t, 21, s.Field.Cursor)
	press(s, "up")
	assert.Equal(t, 0, s.Field.Cursor)
}

func TestTheFieldAsksItsOwner(t *testing.T) {
	tests := map[string]fieldResult{"enter": fieldDone, "tab": fieldTab, "esc": fieldDiscard, "ctrl+c": fieldDiscard}
	for k, want := range tests {
		t.Run(k, func(t *testing.T) {
			s := fieldOn(t, "draft|")

			got := s.fieldKey(keyOf(k))

			assert.Equal(t, want, got)
			assert.Equal(t, want != fieldDiscard, s.Field.Open)
			if want != fieldDiscard {
				assert.Equal(t, "draft", s.Field.Text, "only the owner ends the draft")
			}
		})
	}
}

func TestPastingKeepsControlsAndBreaksLines(t *testing.T) {
	s := fieldOn(t, "|")

	s.fieldPaste("a\x1bb\x07c\r\nd\re")

	assert.Equal(t, "a\x1bb\x07c\nd\ne|", marked(s.Field))
}

func TestTextOverTheLimitIsNotTaken(t *testing.T) {
	s := fieldOn(t, "|")
	s.Field.Limit = 5

	s.fieldPaste("123456")
	assert.Empty(t, s.Field.Text, "a paste comes whole or not at all")
	assert.Equal(t, "text over 5 bytes, not taken", s.Note)

	s.Note = ""
	press(s, "1", "2", "3", "4", "5", "6")
	assert.Equal(t, "12345", s.Field.Text)
	assert.Equal(t, "text over 5 bytes, not taken", s.Note)
}

func TestTheLimitIsNamedInKiB(t *testing.T) {
	s := fieldOn(t, strings.Repeat("a", noteLimit)+"|")

	press(s, "b")

	assert.Equal(t, "text over 8 KiB, not taken", s.Note)
}

// TestTheFieldKeepsItsInvariants edits at random: the cursor stays on a
// grapheme boundary and in view, the text within its limit, and the rows
// cover the text in order, their words short of the box's last column
// unless one grapheme is wider.
func TestTheFieldKeepsItsInvariants(t *testing.T) {
	pieces := []string{"a", " ", "日", "e\u0301", "👨\u200d👩\u200d👧", "\t", "\x1b", "\n", "word ", "\xff"}
	keys := []string{"left", "right", "up", "down", "home", "end", "backspace", "delete",
		"alt+left", "alt+right", "alt+backspace", "alt+enter"}
	rapid.Check(t, func(rt *rapid.T) {
		s := screen(t, rapid.IntRange(4, 50).Draw(rt, "width"))
		s.Cells = rapid.SampledFrom([]textcell.Method{textcell.Codepoints, textcell.Graphemes}).Draw(rt, "method")
		s.openField(rapid.IntRange(0, 64).Draw(rt, "limit"))
		for range rapid.IntRange(0, 60).Draw(rt, "steps") {
			if rapid.Bool().Draw(rt, "paste") {
				s.fieldPaste(rapid.SampledFrom(pieces).Draw(rt, "piece"))
			} else {
				s.fieldKey(keyOf(rapid.SampledFrom(keys).Draw(rt, "key")))
			}
			checkField(rt, &s.Field)
		}
	})
}

func checkField(t *rapid.T, f *field) {
	if len(f.Text) > f.Limit {
		t.Fatalf("%d bytes over the limit of %d", len(f.Text), f.Limit)
	}
	if !boundaries(f.Text, f.M)[f.Cursor] {
		t.Fatalf("cursor %d in %q is no grapheme boundary", f.Cursor, f.Text)
	}
	if row := rowOf(f.Rows, f.Cursor); f.H > 0 && (row < f.Scroll || row >= f.Scroll+f.shown()) {
		t.Fatalf("cursor row %d out of view from %d for %d rows", row, f.Scroll, f.shown())
	}
	at := 0
	for i, r := range f.Rows {
		if r.Start < at || r.End < r.Start {
			t.Fatalf("row %d %v out of order after %d", i, r, at)
		}
		// Text leaves the last column free; only a blank may stand there.
		words := strings.TrimRight(f.Text[r.Start:r.End], " \t")
		if w := textWidth(words, f.M); f.W > 0 && w > f.W-popupFrame-1 && countCells(words, f.M) > 1 {
			t.Fatalf("row %d %q is %d columns wide in %d", i, words, w, f.W-popupFrame-1)
		}
		at = r.End
	}
	if at != len(f.Text) {
		t.Fatalf("rows end at %d of %d", at, len(f.Text))
	}
}

// boundaries are the grapheme boundaries of text, its end included.
func boundaries(text string, m textcell.Method) map[int]bool {
	b := map[int]bool{len(text): true}
	cells := textcell.New(text, m)
	for c, ok := cells.Next(); ok; c, ok = cells.Next() {
		b[c.Start] = true
	}
	return b
}

func countCells(s string, m textcell.Method) int {
	n := 0
	cells := textcell.New(s, m)
	for _, ok := cells.Next(); ok; _, ok = cells.Next() {
		n++
	}
	return n
}
