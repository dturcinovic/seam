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
	Body int
	// List is the columns of the file list and its separator at the left
	// edge, none while it is hidden.
	List        int
	Left, Right pane
}

// panes is the width of both panes and the separator between them; the
// file list and the scrollbar take the rest.
func (l layout) panes() int {
	return max(l.Width-l.List-scrollbarWidth, 0)
}

// scrollbar is the columns the scrollbar takes, none on a screen too narrow.
func (l layout) scrollbar() int {
	return max(l.Width-l.List, 0) - l.panes()
}

// bodyLine is which of the lines on the screen screen cell x, y lies in,
// counted from the top one; ok is false outside the panes' lines.
func (l layout) bodyLine(x, y int) (i int, ok bool) {
	return l.within(x-l.List, y, l.panes())
}

// inList says whether screen cell x, y lies in the file list's lines, its
// separator included.
func (l layout) inList(x, y int) bool {
	_, ok := l.within(x, y, l.List)
	return ok
}

// footerMinLines is the height from which the file list spends its last
// line on the totals.
const footerMinLines = 3

// listEntries is how many entries the file list shows: all its lines but
// the last, which holds the totals once there are enough.
func (l layout) listEntries() int {
	if shown := l.shown(); shown < footerMinLines {
		return shown
	}
	return l.shown() - 1
}

// listLine is which entry of the file list screen cell x, y lies in,
// counted from the top one; ok is false outside its entries.
func (l layout) listLine(x, y int) (i int, ok bool) {
	i, ok = l.within(x, y, l.List-separatorWidth)
	return i, ok && i < l.listEntries()
}

// within is the body line of cell x, y of a column width columns wide.
func (l layout) within(x, y, width int) (i int, ok bool) {
	if x < 0 || x >= width || y < bodyTop || y >= bodyTop+l.shown() {
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

// computeLayout splits width between a file list of list columns, both
// panes of d and the scrollbar; the right pane takes the odd column.
func computeLayout(width, height, list int, d diff.FileDiff) layout {
	l := layout{Width: width, Height: height, Body: max(height-chrome, 1), List: list}
	left := max((l.panes()-separatorWidth)/2, 0)
	l.Left = pane{Width: left, Digits: digits(len(d.Old.Lines))}
	l.Right = pane{Width: max(l.panes()-separatorWidth-left, 0), Digits: digits(len(d.New.Lines))}
	return l
}

func digits(lines int) int {
	return len(strconv.Itoa(max(lines, 1)))
}
