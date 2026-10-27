package tui

import (
	"strings"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// The bounds of the popup, frame included: no wider than a line of text
// reads well, no higher than leaves the code around it in view.
const (
	popupMaxWidth   = 80
	popupMaxPercent = 40
)

// popupFrame is the frame's columns, or lines, on both sides together.
const popupFrame = 2

// popupTab is how many blanks a tab takes, see lineBuilder.
const popupTab = "    "

// popup is the answer of a hover on screen, the notes at a line, or one
// note: its text wrapped to its box. Notes is that line, nil otherwise,
// opened at NotesX, NotesY; Note the one note, for x.
type popup struct {
	Open           bool
	Lines          []popupLine
	Notes          *noteLine
	NotesX, NotesY int
	Note           note.ID
	box
}

func (p popup) contains(x, y int) bool {
	return p.Open && p.box.contains(x, y)
}

// scrollBy scrolls the text by n lines, within its length.
func (p *popup) scrollBy(n int) {
	p.Scroll = min(max(p.Scroll+n, 0), max(len(p.Lines)-p.shown(), 0))
}

// openPopup lays c out for the mouse at x, y, placed as boxAt does.
// Without text or room it stays closed.
func (s *state) openPopup(c hover.Content, x, y int) popup {
	inner := s.boxMaxInner()
	if inner < 1 {
		return popup{}
	}
	lines, widest := popupText(c, inner, s.Cells)
	if widest == 0 {
		return popup{}
	}
	h := min(len(lines)+popupFrame, s.boxMaxHeight())
	if h <= popupFrame {
		return popup{}
	}
	p := popup{Open: true, Lines: lines, W: widest + popupFrame, H: h}
	p.X, p.Y = s.boxAt(p.W, p.H, x, y)
	return p
}

// writeRow writes line j of the text, if there is one.
func (p *popup) writeRow(ln *line, j int) {
	if j >= len(p.Lines) {
		return
	}
	text := p.Lines[j]
	if text.Rule {
		ln.text(strings.Repeat("─", ln.free), popupFrameStyle)
	}
	ln.window(text.Text, nil, 0, cellStyle{fixed: popupStyle, runs: text.Runs})
}
