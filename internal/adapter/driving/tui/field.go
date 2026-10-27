package tui

import (
	"iter"
	"math"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dturcinovic/seam/internal/textcell"
)

// field is the multi-line input field, opened by its owner for a text of at
// most Limit bytes.
type field struct {
	Open bool
	// Text keeps controls as they came; textcell replaces them only where
	// they are drawn.
	Text  string
	Limit int
	// Cursor is a grapheme boundary of Text, in row Row.
	Cursor, Row int
	// Goal is the column up and down keep to, or -1 for the cursor's own.
	Goal int
	// Rows and the box are Text laid out by layoutField, as M counts
	// columns.
	Rows []fieldRow
	M    textcell.Method
	box
}

// fieldResult is what a key in the field asks of its owner.
type fieldResult uint8

const (
	// fieldEdited is a key the field took itself.
	fieldEdited fieldResult = iota
	// fieldRefused is text the limit kept out.
	fieldRefused
	// fieldDone is enter: the text is finished.
	fieldDone
	// fieldTab is tab, which the owner gives a meaning.
	fieldTab
	// fieldDiscard is esc or ctrl+c: close the field, without the text.
	fieldDiscard
)

// key takes a key press; every key that is no command is text. The keys
// are fixed rather than set in [keys]: while the field is open, all of
// them are the text's.
func (f *field) key(k tea.Key) fieldResult {
	switch k.String() {
	case "esc", "ctrl+c":
		return fieldDiscard
	case "enter":
		return fieldDone
	case "tab":
		return fieldTab
	case "alt+enter":
		return f.insert("\n")
	}
	if f.edit(k.String()) {
		return fieldEdited
	}
	// Bubble Tea sets Text for printable characters only, also where a
	// terminal reports AltGr as ctrl+alt.
	if k.Text == "" {
		return fieldEdited
	}
	return f.insert(k.Text)
}

// edit moves the cursor or deletes for key; ok is false for any other key.
func (f *field) edit(key string) (ok bool) {
	t, b, m := f.Text, f.Cursor, f.M
	switch key {
	case "up":
		f.upDown(-1)
	case "down":
		f.upDown(1)
	case "left":
		f.moveTo(prevBoundary(t, b, m))
	case "right":
		f.moveTo(nextBoundary(t, b, m))
	case "alt+left", "ctrl+left":
		f.moveTo(wordBefore(t, b, m))
	case "alt+right", "ctrl+right":
		f.moveTo(wordAfter(t, b, m))
	case "home":
		f.moveTo(f.Rows[f.Row].Start)
	case "end":
		f.moveTo(byteAt(t, f.Rows, f.Row, math.MaxInt, m))
	case "backspace":
		f.cut(prevBoundary(t, b, m), b)
	case "delete":
		f.cut(b, nextBoundary(t, b, m))
	case "alt+backspace", "ctrl+w":
		f.cut(wordBefore(t, b, m), b)
	default:
		return false
	}
	return true
}

func (f *field) moveTo(b int) { f.Cursor, f.Goal = b, -1 }

// cut takes the bytes from to of the text out, the cursor at from.
func (f *field) cut(from, to int) {
	f.Text = f.Text[:from] + f.Text[to:]
	f.moveTo(from)
}

// insert puts s in at the cursor, with every CRLF or CR a line break, if
// the text stays within the limit; otherwise the text stays as it was.
func (f *field) insert(s string) fieldResult {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if len(f.Text)+len(s) > f.Limit {
		return fieldRefused
	}
	f.Text = f.Text[:f.Cursor] + s + f.Text[f.Cursor:]
	f.moveTo(f.Cursor + len(s))
	return fieldEdited
}

// upDown moves the cursor dir rows, as near to its goal column as the row
// has a grapheme boundary; past the first or last row it stays.
func (f *field) upDown(dir int) {
	row, col := cursorCell(f.Text, f.Rows, f.Cursor, f.M)
	if f.Goal < 0 {
		f.Goal = col
	}
	if to := row + dir; to >= 0 && to < len(f.Rows) {
		f.Cursor = byteAt(f.Text, f.Rows, to, f.Goal, f.M)
	}
}

// nextBoundary is the grapheme boundary after b, or b at the end.
func nextBoundary(text string, b int, m textcell.Method) int {
	cells := textcell.Resume(text, b, 0, m)
	if c, ok := cells.Next(); ok {
		return c.End
	}
	return b
}

// prevBoundary is the grapheme boundary before b, or b at the start. It
// walks from the start of b's line, as a line break is always a boundary.
func prevBoundary(text string, b int, m textcell.Method) int {
	start := strings.LastIndexByte(text[:b], '\n') + 1
	if start == b {
		return max(b-1, 0)
	}
	cells := textcell.Resume(text[:b], start, 0, m)
	at := start
	for c, ok := cells.Next(); ok && c.End < b; c, ok = cells.Next() {
		at = c.End
	}
	return at
}

// wordGrapheme is a grapheme a word jump passes: its word class, and the
// boundary past it in the jump's direction.
type wordGrapheme struct {
	class, to int
}

// wordAfter is the end of the word after b, past any blanks before it.
func wordAfter(text string, b int, m textcell.Method) int {
	return wordEdge(b, func(yield func(wordGrapheme) bool) {
		cells := textcell.Resume(text, b, 0, m)
		for c, ok := cells.Next(); ok; c, ok = cells.Next() {
			if !yield(wordGrapheme{classOf(text, c), c.End}) {
				return
			}
		}
	})
}

// wordBefore is the start of the word before b, past any blanks after it.
// It walks each line forward once and passes its graphemes back, as
// graphemes cannot be found walking backwards.
func wordBefore(text string, b int, m textcell.Method) int {
	return wordEdge(b, func(yield func(wordGrapheme) bool) {
		var line []wordGrapheme
		for end := b; end > 0; {
			start := strings.LastIndexByte(text[:end], '\n') + 1
			if start == end {
				if !yield(wordGrapheme{blankClass, end - 1}) {
					return
				}
				end--
				continue
			}
			line = line[:0]
			cells := textcell.Resume(text[:end], start, 0, m)
			for c, ok := cells.Next(); ok; c, ok = cells.Next() {
				line = append(line, wordGrapheme{classOf(text, c), c.Start})
			}
			for _, l := range slices.Backward(line) {
				if !yield(l) {
					return
				}
			}
			end = start
		}
	})
}

// wordEdge is where a word jump from b stops: past the blanks first, then
// past the graphemes of the class of the first one after them.
func wordEdge(b int, graphemes iter.Seq[wordGrapheme]) int {
	class := -1
	for g := range graphemes {
		if class < 0 && g.class != blankClass {
			class = g.class
		}
		if class >= 0 && g.class != class {
			break
		}
		b = g.to
	}
	return b
}

// classOf is the word class of cell c of text; a line break, which
// textcell replaces, is blank like any space.
func classOf(text string, c textcell.Cell) int {
	g := text[c.Start:c.End]
	return graphemeClass(g, c.Replaced && g != "\n")
}
