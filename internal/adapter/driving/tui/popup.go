package tui

import (
	"strings"

	"seam/internal/textcell"
)

// The bounds of the popup, frame included: no wider than a line of text
// reads well, no higher than leaves the code around it in view.
const (
	popupMaxWidth   = 80
	popupMaxPercent = 40
)

// popupFrame is the frame's columns, or lines, on both sides together.
const popupFrame = 2

// popupTab is how many blanks a tab takes: the popup's lines start
// anywhere, so tab stops would not line up.
const popupTab = "    "

// popup is the answer of a hover on screen: its text wrapped to the box at
// X, Y of W by H cells, frame included, scrolled by Scroll lines.
type popup struct {
	Open       bool
	Lines      []string
	X, Y, W, H int
	Scroll     int
}

func (p popup) contains(x, y int) bool {
	return p.Open && x >= p.X && x < p.X+p.W && y >= p.Y && y < p.Y+p.H
}

// shown is how many lines of text fit in the frame.
func (p popup) shown() int { return max(p.H-popupFrame, 0) }

// scrollBy scrolls the text by n lines, within its length.
func (p *popup) scrollBy(n int) {
	p.Scroll = min(max(p.Scroll+n, 0), max(len(p.Lines)-p.shown(), 0))
}

// openPopup lays text out for the mouse at x, y: below its line, or above
// if there is no room below, from its column, or further left if the box
// would reach past the edge. Without text or room it stays closed.
func (s *state) openPopup(text string, x, y int) popup {
	l := s.Layout
	inner := min(popupMaxWidth, l.Width) - popupFrame
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines, widest := wrapText(strings.TrimRight(text, "\n"), inner, s.Cells)
	top, bottom := bodyTop, bodyTop+l.shown()
	if inner < 1 || widest == 0 {
		return popup{}
	}
	h := min(len(lines)+popupFrame, max(l.Height*popupMaxPercent/100, popupFrame+1), bottom-top)
	if h <= popupFrame {
		return popup{}
	}
	p := popup{Open: true, Lines: lines, W: widest + popupFrame, H: h}
	switch {
	case y+1+h <= bottom:
		p.Y = y + 1
	case y-h >= top:
		p.Y = y - h
	default:
		p.Y = bottom - h
	}
	p.X = max(min(x, l.Width-p.W), 0)
	return p
}

// wrapText breaks text into lines of at most width columns, at grapheme
// boundaries, and returns them with the widest's columns.
func wrapText(text string, width int, m textcell.Method) (lines []string, widest int) {
	for line := range strings.SplitSeq(strings.ReplaceAll(text, "\t", popupTab), "\n") {
		start, startCol := 0, 0
		cells := textcell.New(line, m)
		for c, ok := cells.Next(); ok; c, ok = cells.Next() {
			if c.Col+c.Width-startCol > width && c.Start > start {
				lines = append(lines, line[start:c.Start])
				start, startCol = c.Start, c.Col
			}
			widest = max(widest, min(c.Col+c.Width-startCol, width))
		}
		lines = append(lines, line[start:])
	}
	return lines, widest
}

// writePopupLine writes line i of p's box, counted from its top: the frame
// or a line of text.
func writePopupLine(b *strings.Builder, p popup, th *theme, m textcell.Method, i int) {
	ln := newLine(b, th, m, p.W)
	edge := func(left, fill, right string) {
		ln.text(left+strings.Repeat(fill, max(p.W-popupFrame, 0))+right, popupFrameStyle)
		ln.end(popupFrameStyle)
	}
	switch i {
	case 0:
		edge("┌", "─", "┐")
		return
	case p.H - 1:
		edge("└", "─", "┘")
		return
	}
	ln.text("│", popupFrameStyle)
	text := ""
	if j := p.Scroll + i - 1; j < len(p.Lines) {
		text = p.Lines[j]
	}
	writeCut(&ln, text, p.W-popupFrame, popupStyle, false)
	ln.text("│", popupFrameStyle)
	ln.end(popupFrameStyle)
}

// overlay is screen line under with line i of the popup over it.
func overlay(under string, p popup, th *theme, m textcell.Method, width, i int) string {
	var over strings.Builder
	writePopupLine(&over, p, th, m, i)
	return textcell.Overlay(under, p.X, over.String(), width, m)
}
