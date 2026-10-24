package tui

import (
	"strconv"

	"seam/internal/domain/diff"
)

// chrome is the lines the view spends outside the panes: header and footer.
const chrome = 2

// separatorWidth is the column between the panes.
const separatorWidth = 1

// scrollbarWidth is the column at the right edge that shows where the
// changes are and which part of the file is on the screen.
const scrollbarWidth = 1

// bodyTop is the screen line of the first line of the file, below the
// header.
const bodyTop = 1

// layout is where things are on the screen. The actions compute it whenever
// the size or the file changes; the view only reads it.
type layout struct {
	Width, Height int
	// Body is how many lines of the file fit between header and footer, at
	// least one to scroll by even when none shows.
	Body        int
	Left, Right pane
}

// panes is the width of both panes and the separator; the scrollbar takes
// the rest.
func (l layout) panes() int {
	return max(l.Width-scrollbarWidth, 0)
}

// bodyLine is which of the lines on the screen screen cell x, y lies in,
// counted from the top one; ok is false outside the panes' lines.
func (l layout) bodyLine(x, y int) (i int, ok bool) {
	if x < 0 || x >= l.panes() || y < bodyTop || y >= bodyTop+l.shown() {
		return 0, false
	}
	return y - bodyTop, true
}

// shown is how many lines of the file the screen shows: below three lines
// none fit, and the screen gets no more lines than it has.
func (l layout) shown() int {
	return min(l.Body, max(l.Height-chrome, 0))
}

// pane is one side on the screen: its Width in columns, the gutter
// included, and Digits for the largest line number of its side.
type pane struct {
	Width, Digits int
}

// gutter is the width of " 123 ~ ": a line number and the mark of the row.
func (p pane) gutter() int { return p.Digits + 4 }

// computeLayout splits width between both panes of d and the scrollbar;
// the right pane takes the odd column.
func computeLayout(width, height int, d diff.FileDiff) layout {
	l := layout{Width: width, Height: height, Body: max(height-chrome, 1)}
	left := max((l.panes()-separatorWidth)/2, 0)
	l.Left = pane{Width: left, Digits: digits(len(d.Old.Lines))}
	l.Right = pane{Width: max(l.panes()-separatorWidth-left, 0), Digits: digits(len(d.New.Lines))}
	return l
}

func digits(lines int) int {
	return len(strconv.Itoa(max(lines, 1)))
}
