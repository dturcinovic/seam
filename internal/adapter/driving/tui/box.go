package tui

import (
	"strings"

	"github.com/dturcinovic/seam/internal/textcell"
)

// box is a framed box over the body, as the hover's popup and the input
// field are: at X, Y of W by H cells, frame included, its rows scrolled by
// Scroll, with Title in its top edge and Foot in its bottom one.
type box struct {
	X, Y, W, H  int
	Scroll      int
	Title, Foot string
}

func (b box) contains(x, y int) bool {
	return x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H
}

// shown is how many rows fit in the frame.
func (b box) shown() int { return max(b.H-popupFrame, 0) }

// boxMaxInner is the most columns a box's rows may take.
func (s *state) boxMaxInner() int {
	return min(popupMaxWidth, s.Layout.Width) - popupFrame
}

// boxMaxHeight is the most lines a box may take, frame included.
func (s *state) boxMaxHeight() int {
	l := s.Layout
	return min(max(l.Height*popupMaxPercent/100, popupFrame+1), l.shown())
}

// boxAt places a box of w by h cells for line y and column x: below the
// line, or above if there is no room below, from the column, or further
// left if the box would reach past the edge.
func (s *state) boxAt(w, h, x, y int) (bx, by int) {
	top, bottom := bodyTop, bodyTop+s.Layout.shown()
	switch {
	case y+1+h <= bottom:
		by = y + 1
	case y-h >= top:
		by = y - h
	default:
		by = bottom - h
	}
	return max(min(x, s.Layout.Width-w), 0), by
}

// rowWriter writes row j of a box's content into the frame, if it has one.
type rowWriter func(ln *line, j int)

// writeBoxLine writes line i of bx, counted from its top: the frame or a
// row of its content.
func writeBoxLine(b *strings.Builder, bx box, row rowWriter, th *theme, m textcell.Method, i int) {
	ln := newLine(b, th, m, bx.W)
	switch i {
	case 0:
		writeTitled(&ln, bx.W, bx.Title, "┌", "┐")
		return
	case bx.H - 1:
		writeTitled(&ln, bx.W, bx.Foot, "└", "┘")
		return
	}
	ln.text("│", popupFrameStyle)
	rest := ln.narrow(bx.W - popupFrame)
	row(&ln, bx.Scroll+i-1)
	ln.spaces(ln.free, popupStyle)
	ln.free = rest
	ln.text("│", popupFrameStyle)
	ln.end(popupFrameStyle)
}

// writeEdge writes the top or the bottom of the frame of a box w columns
// wide, between its corners left and right.
func writeEdge(ln *line, w int, left, right string) {
	ln.text(left+strings.Repeat("─", max(w-popupFrame, 0))+right, popupFrameStyle)
	ln.end(popupFrameStyle)
}

// writeTitled writes the top or the bottom of the frame between its
// corners left and right, with title after the left one where it fits
// whole.
func writeTitled(ln *line, w int, title, left, right string) {
	if title == "" || textWidth(title, ln.method)+4 > w {
		writeEdge(ln, w, left, right)
		return
	}
	ln.text(left+" ", popupFrameStyle)
	ln.text(title, popupFrameStyle)
	writeEdge(ln, w-textWidth(title, ln.method)-2, " ", right)
}

// overlay is screen line under with line i of bx over it.
func overlay(under string, bx box, row rowWriter, th *theme, m textcell.Method, width, i int) string {
	var over strings.Builder
	writeBoxLine(&over, bx, row, th, m, i)
	return textcell.Overlay(under, bx.X, over.String(), width, m)
}
