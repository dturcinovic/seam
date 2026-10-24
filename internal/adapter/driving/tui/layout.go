package tui

import (
	"strconv"

	"seam/internal/domain/diff"
)

// chrome is the lines the view spends outside the panes: header and footer.
const chrome = 2

// separatorWidth is the column between the panes.
const separatorWidth = 1

// layout is where things are on the screen. The actions compute it whenever
// the size or the file changes; the view only reads it.
type layout struct {
	Width, Height int
	// Body is how many lines of the file fit between header and footer, at
	// least one to scroll by even when none shows.
	Body        int
	Left, Right pane
}

// pane is one side on the screen: its Width in columns, the gutter
// included, and Digits for the largest line number of its side.
type pane struct {
	Width, Digits int
}

// gutter is the width of " 123 ~ ": a line number and the mark of the row.
func (p pane) gutter() int { return p.Digits + 4 }

// computeLayout splits width between both panes of d; the right one takes
// the odd column.
func computeLayout(width, height int, d diff.FileDiff) layout {
	left := max((width-separatorWidth)/2, 0)
	right := max(width-separatorWidth-left, 0)
	return layout{
		Width: width, Height: height, Body: max(height-chrome, 1),
		Left:  pane{Width: left, Digits: digits(len(d.Old.Lines))},
		Right: pane{Width: right, Digits: digits(len(d.New.Lines))},
	}
}

func digits(lines int) int {
	return len(strconv.Itoa(max(lines, 1)))
}
