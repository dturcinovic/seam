package tui

import (
	"fmt"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/textcell"
)

// noteKeys are the field's keys, at the bottom of its box, or in the
// footer while the box is not on the screen.
const noteKeys = "⏎ save  ⌥⏎ newline  ⇥ kind  esc discard"

// noteHook joins the box to the lines noted, below their marks.
const noteHook = "└"

// anchorMark marks the lines the field notes.
const anchorMark = "●"

// kindLook is how a note of a kind shows while it is written: the frame of
// its box, its filled name, and the role of its marks in the gutter.
type kindLook struct {
	frame, chip style
	mark        role
}

func lookOf(k note.Kind) kindLook {
	if k == note.Finding {
		return kindLook{findingFrameStyle, findingChipStyle, findingRole}
	}
	return kindLook{questionFrameStyle, questionChipStyle, questionRole}
}

// keysShown reports whether the field's box is on the screen and names
// its keys; else the footer does.
func (s *state) keysShown() bool {
	return s.gapOnScreen(s.gap()) && keysFit(s.Field.W, s.Cells)
}

// keysFit reports whether the keys fit the bottom edge of a box w wide.
func keysFit(w int, m textcell.Method) bool {
	return textWidth(" "+noteKeys+" ", m)+4 <= w
}

// gapOnScreen reports whether the field's box at g is on the screen down
// to its bottom edge, which names the keys.
func (s *state) gapOnScreen(g gapSpot) bool {
	t := s.top()
	return g.OK && g.At >= t && g.At-t+g.H < s.Layout.shown()
}

// writeGapLine writes row j of the field's box across the panes: the box
// in the text of the side noted, the hook below the marks on its top edge,
// blanks elsewhere.
func writeGapLine(b *strings.Builder, s *state, th *theme, j int) {
	t := s.Notes.Draft.Lines
	lay := s.layoutOf(t.File)
	width := lay.panes()
	ln := newLine(b, th, s.Cells, width)
	to := func(col int) { ln.spaces(col-(width-ln.free), plain) }
	start, marks := lay.paneStart(t.Side)-lay.List, lay.Left.Marks
	right := !lay.Unified && t.Side == filediff.NewSide
	if right {
		marks = lay.Right.Marks
		to(lay.Left.Width)
		ln.text("│", separatorStyle)
	}
	to(start)
	if j == 0 {
		ln.spaces((marks-1)/2, plain)
		ln.text(noteHook, paneStyle(equalTone, lookOf(s.Notes.Draft.Kind).mark))
	}
	to(s.Field.X - lay.List)
	s.writeNoteBox(&ln, j)
	if !lay.Unified && !right {
		to(lay.Left.Width)
		ln.text("│", separatorStyle)
	}
	ln.end(plain)
}

// writeNoteBox writes row j of the field's box: its top edge with the
// kinds and the lines, a row of the text, or its bottom edge with the
// keys.
func (s *state) writeNoteBox(ln *line, j int) {
	f := &s.Field
	frame := lookOf(s.Notes.Draft.Kind).frame
	rest := ln.narrow(f.W)
	defer func() { ln.free = rest }()
	switch j {
	case 0:
		s.writeNoteTop(ln, frame)
		return
	case f.H - 1:
		writeNoteFoot(ln, f.W, frame)
		return
	}
	ln.text("│", frame)
	ln.spaces(fieldPad, popupStyle)
	inner := ln.narrow(f.inner())
	switch row := j - 1 - fieldGap; {
	case row < 0:
	case f.Text == "" && row == 0:
		ln.spaces(1, caretStyle)
		ln.text(s.notePlaceholder(), popupDimStyle)
	default:
		f.writeRow(ln, f.Scroll+row)
	}
	ln.spaces(ln.free, popupStyle)
	ln.free = inner
	ln.spaces(fieldPad, popupStyle)
	ln.text("│", frame)
}

// writeNoteTop writes the top edge: both kinds with the one being written
// filled, and the lines noted at the right. A narrow box keeps the kind
// being written, then drops the lines, so its corner stays.
func (s *state) writeNoteTop(ln *line, frame style) {
	d := &s.Notes.Draft
	kinds, lines := []note.Kind{note.Question, note.Finding}, " "+linesText(d.Lines)+" "
	fits := func() bool {
		n := 0
		for _, k := range kinds {
			n += len(k.String()) + 2
		}
		return n+1+textWidth(lines, ln.method)+2 <= ln.free
	}
	if !fits() {
		kinds = []note.Kind{d.Kind}
	}
	if !fits() {
		lines = ""
	}
	ln.text("┌", frame)
	for _, k := range kinds {
		st := popupDimStyle
		if k == d.Kind {
			st = lookOf(d.Kind).chip
		}
		ln.text(" ", frame)
		ln.text(k.String(), st)
		ln.text(" ", frame)
	}
	ln.text(strings.Repeat("─", max(ln.free-textWidth(lines, ln.method)-2, 0)), frame)
	ln.text(lines, popupDimStyle)
	ln.text("─┐", frame)
}

// writeNoteFoot writes the bottom edge of a box w wide, with the keys at
// its right where they fit.
func writeNoteFoot(ln *line, w int, frame style) {
	keys := " " + noteKeys + " "
	n := textWidth(keys, ln.method)
	if !keysFit(w, ln.method) {
		writeEdge(ln, w, "└", "┘", frame)
		return
	}
	ln.text("└"+strings.Repeat("─", w-n-3), frame)
	ln.text(keys, popupDimStyle)
	ln.text("─┘", frame)
}

// notePlaceholder is the empty field's word on what to write.
func (s *state) notePlaceholder() string {
	d := &s.Notes.Draft
	if d.Kind == note.Finding {
		return "Describe the finding on " + lineWord(d.Lines) + "…"
	}
	return "Ask about " + lineWord(d.Lines) + "…"
}

// linesText names the lines of t, counted from 1, as the top edge does.
func linesText(t noteTarget) string {
	if t.From == t.To {
		return fmt.Sprintf("L%d", t.To+1)
	}
	return fmt.Sprintf("L%d–%d", t.From+1, t.To+1)
}

// lineWord names the lines of t in words.
func lineWord(t noteTarget) string {
	if t.From == t.To {
		return fmt.Sprintf("line %d", t.To+1)
	}
	return fmt.Sprintf("lines %d–%d", t.From+1, t.To+1)
}
