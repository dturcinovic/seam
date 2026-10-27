package tui

import (
	"sort"
	"strings"

	"github.com/dturcinovic/seam/internal/textcell"
)

// fieldRow is a row of the input field: the bytes Start to End of its text,
// End before the line break that ends a line. A row that is not its line's
// last ends where the next starts, so a byte offset there is at the start
// of the next row, not at the end of this one.
type fieldRow struct {
	Start, End int
}

// fieldRows breaks text into rows for a box width columns wide: at each
// line break, and within a line after its last blank, or else between
// graphemes. Text takes all but the last column, which holds the cursor at
// a row's end and a blank that ends a word there, so neither a blank at
// the edge nor the cursor wraps a word that fits. Tab stops count from the
// row's start, as the row is drawn on its own.
func fieldRows(text string, width int, m textcell.Method) []fieldRow {
	width = max(width-1, 1)
	var rows []fieldRow
	start := 0
	for {
		end := len(text)
		if i := strings.IndexByte(text[start:], '\n'); i >= 0 {
			end = start + i
		}
		rows = appendLineRows(rows, text, start, end, width, m)
		if end == len(text) {
			return rows
		}
		start = end + 1
	}
}

// appendLineRows appends the rows of the line from start to end of text,
// its text within width columns.
func appendLineRows(rows []fieldRow, text string, start, end, width int, m textcell.Method) []fieldRow {
	line := text[:end]
	row := start
	// word is where the word after the row's last blank starts.
	word := row
	afterBlank := false
	// used is the row's columns so far, where the cursor at its end stands.
	used := 0
	cells := textcell.Resume(line, row, 0, m)
	for c, ok := cells.Next(); ok; c, ok = cells.Next() {
		used = c.Col + c.Width
		blank := isBlank(line[c.Start:c.End])
		if afterBlank && !blank {
			word = c.Start
		}
		afterBlank = blank
		// A blank may start in the last column; text must end before it.
		over := used > width
		if blank {
			over = c.Col > width
		}
		if !over || c.Start == row {
			continue
		}
		next := c.Start
		if !blank && word > row {
			next = word
		}
		rows = append(rows, fieldRow{row, next})
		row, word, afterBlank = next, next, false
		cells = textcell.Resume(line, row, 0, m)
	}
	rows = append(rows, fieldRow{row, end})
	if row < end && used > width {
		// The cursor at the line's end would stand past the last column.
		rows = append(rows, fieldRow{end, end})
	}
	return rows
}

func isBlank(g string) bool { return g == " " || g == "\t" }

// rowOf is the row byte b of the text stands in: the last that starts at or
// before it.
func rowOf(rows []fieldRow, b int) int {
	return sort.Search(len(rows), func(i int) bool { return rows[i].Start > b }) - 1
}

// cursorCell is the row and column where byte b of text shows, b being a
// grapheme boundary.
func cursorCell(text string, rows []fieldRow, b int, m textcell.Method) (row, col int) {
	row = rowOf(rows, b)
	r := rows[row]
	return row, textWidth(text[r.Start:min(b, r.End)], m)
}

// byteAt is the grapheme boundary in row i of text nearest to column col
// on its left. The end of a row that wraps is the next row's start, so it
// stops before the row's last grapheme there.
func byteAt(text string, rows []fieldRow, i, col int, m textcell.Method) int {
	r := rows[i]
	at := r.End
	cells := seek(text[r.Start:r.End], nil, col, m)
	if c, ok := cells.Next(); ok {
		at = r.Start + c.Start
	}
	if wraps := i+1 < len(rows) && rows[i+1].Start == r.End; wraps && at == r.End && at > r.Start {
		return prevBoundary(text, at, m)
	}
	return at
}
