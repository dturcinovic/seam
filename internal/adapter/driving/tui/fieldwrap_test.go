package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/textcell"
)

// rowTexts are the texts of the rows of text at width.
func rowTexts(text string, width int, m textcell.Method) []string {
	var ts []string
	for _, r := range fieldRows(text, width, m) {
		ts = append(ts, text[r.Start:r.End])
	}
	return ts
}

func TestTheFieldWrapsItsRows(t *testing.T) {
	tests := map[string]struct {
		text  string
		width int
		rows  []string
	}{
		"empty":                        {"", 5, []string{""}},
		"short":                        {"ab", 5, []string{"ab"}},
		"line breaks":                  {"a\n\nb\n", 5, []string{"a", "", "b", ""}},
		"after the last blank":         {"ab cd ef", 6, []string{"ab cd ", "ef"}},
		"a blank at the edge hangs":    {"aaaa bbbb", 5, []string{"aaaa ", "bbbb"}},
		"within a word too long":       {"abcdefgh", 4, []string{"abc", "def", "gh"}},
		"the last column is the caret": {"abc", 4, []string{"abc"}},
		"a hanging blank at the end":   {"abc ", 4, []string{"abc ", ""}},
		"a wide character moves on":    {"ab日", 4, []string{"ab", "日"}},
		"tab stops from the row":       {"abc\tdefg\th", 7, []string{"abc\t", "defg\t", "h"}},
		"a grapheme wider than a row":  {"日", 2, []string{"日", ""}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.rows, rowTexts(tt.text, tt.width, textcell.Codepoints))
		})
	}
	assert.Equal(t, []string{"👨\u200d👩\u200d👧", "x"}, rowTexts("👨\u200d👩\u200d👧x", 3, textcell.Graphemes))
	assert.Equal(t, []string{"👨\u200d👩\u200d👧", "x"}, rowTexts("👨\u200d👩\u200d👧x", 4, textcell.Codepoints),
		"wcwidth gives the sequence 6 columns, but it stays one grapheme")
}

// caretTheme marks the cursor's cell only, so its column is where the mark
// stands.
func caretTheme() *theme {
	var th theme
	th.open[caretStyle], th.close[caretStyle] = "\x1b[7m", "\x1b[27m"
	return &th
}

// TestTheCursorIsWhereItIsDrawn puts the cursor on every grapheme of text
// that wraps and has controls, combining marks, ZWJ sequences, wide
// characters and tabs, and finds its mark in the box at the cell
// cursorCell gives, under both ways to count columns.
func TestTheCursorIsWhereItIsDrawn(t *testing.T) {
	text := "cafe\u0301 👨\u200d👩\u200d👧 日本語のテキスト\tx\x1by\nzw\u200bq " + strings.Repeat("ab ", 6)
	for _, mt := range methods {
		t.Run(mt.name, func(t *testing.T) {
			s := fieldScreen(t, 12)
			s.Cells = mt.method
			s.openField(noteLimit)
			s.Field.Text = text
			for b := range boundaries(text, mt.method) {
				s.Field.Cursor = b
				s.layoutField()
				f := &s.Field
				row, col := cursorCell(f.Text, f.Rows, b, f.M)

				var box strings.Builder
				ln := newLine(&box, caretTheme(), f.M, f.W)
				s.writeNoteBox(&ln, row-f.Scroll+1+fieldGap)
				line := box.String()

				at := strings.Index(line, "\x1b[7m")
				require.GreaterOrEqual(t, at, 0, "no cursor at %d in %q", b, line)
				assert.Equal(t, 1+fieldPad+col, mt.width(line[:at]), "cursor at byte %d", b)
				assert.Equal(t, f.W, mt.width(line), "the line of the cursor at %d is the box's width", b)
			}
		})
	}
}
