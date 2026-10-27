package tui

import (
	"math"
	"unicode"
	"unicode/utf8"

	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/view"
	"github.com/dturcinovic/seam/internal/textcell"
)

// caret is the keyboard's stand-in for the mouse, for terminals that send
// no motion: a grapheme of Side, at Byte of that side's line of Row of
// File. A row rather than a line of the view, so it stays put when the
// view changes around it.
type caret struct {
	On   bool
	Side filediff.Which
	File int
	Row  int
	Byte int
}

// spot is a grapheme the caret may stand on: c of the text of line of the
// body, with its column checkpoints.
type spot struct {
	line   int
	text   string
	points []filediff.Checkpoint
	c      textcell.Cell
}

// caretText is the text of line of the body, if the caret may stand on
// it: a line hovers may ask about, with text, not a fold or a header.
func (s *state) caretText(line int) (string, []filediff.Checkpoint, bool) {
	file, l, ok := s.rowLine(line)
	if !ok || l.Fold {
		return "", nil, false
	}
	text, points, ok := s.hoverLine(file, s.lineSide(file, l, s.Caret.Side), l.Rows.Start)
	return text, points, ok && text != ""
}

// caretSide is the side the caret stands on on line, see lineSide.
func (s *state) caretSide(line int) filediff.Which {
	file, l, _ := s.rowLine(line)
	return s.lineSide(file, l, s.Caret.Side)
}

// caretLine is the first line from i on, going by dir and stopping short
// of stop, that the caret may stand on.
func (s *state) caretLine(i, dir, stop int) (int, bool) {
	for ; i != stop && i >= 0 && i < s.lineCount(); i += dir {
		if _, _, ok := s.caretText(i); ok {
			return i, true
		}
	}
	return 0, false
}

// spotAt is the grapheme of line that holds byte b, or its last one; the
// caret must be able to stand on line.
func (s *state) spotAt(line, b int) spot {
	text, points, _ := s.caretText(line)
	b = min(b, len(text)-1)
	cells := seekByte(text, points, b, s.Cells)
	c, _ := cells.Next()
	for c.End <= b {
		c, _ = cells.Next()
	}
	return spot{line: line, text: text, points: points, c: c}
}

// spotAtCol is the grapheme of line at column col, or its last one.
func (s *state) spotAtCol(line, col int) spot {
	text, points, _ := s.caretText(line)
	cells := seek(text, points, col, s.Cells)
	if c, ok := cells.Next(); ok {
		return spot{line: line, text: text, points: points, c: c}
	}
	return s.spotAt(line, len(text))
}

// The classes of grapheme for moving by words.
const (
	blankClass = iota
	wordClass
	otherClass
)

func (p spot) class() int {
	return graphemeClass(p.text[p.c.Start:p.c.End], p.c.Replaced)
}

// graphemeClass is the class of grapheme g, replaced if textcell shows a
// stand-in for it; the caret and the input field split words alike.
func graphemeClass(g string, replaced bool) int {
	r, _ := utf8.DecodeRuneInString(g)
	switch {
	case replaced:
		return otherClass
	case unicode.IsSpace(r):
		return blankClass
	case wordRune(r):
		return wordClass
	}
	return otherClass
}

// caretStep is the grapheme after p, or before it for dir -1, crossed if
// on another line; ok is false past the last one.
func (s *state) caretStep(p spot, dir int) (q spot, crossed, ok bool) {
	if dir > 0 && p.c.End < len(p.text) {
		cells := textcell.Resume(p.text, p.c.End, p.c.Col+p.c.Width, s.Cells)
		p.c, _ = cells.Next()
		return p, false, true
	}
	if dir < 0 && p.c.Start > 0 {
		return s.spotAt(p.line, p.c.Start-1), false, true
	}
	line, ok := s.caretLine(p.line+dir, dir, -1)
	switch {
	case !ok:
		return p, false, false
	case dir < 0:
		return s.spotAt(line, math.MaxInt), true, true
	}
	return s.spotAt(line, 0), true, true
}

// spot is where the caret stands; ok is false if it is on no line shown.
func (s *state) spot() (spot, bool) {
	line, ok := s.shownLine(s.Caret.File, s.Caret.Row)
	if !s.Caret.On || !ok {
		return spot{}, false
	}
	if _, _, ok := s.caretText(line); !ok {
		return spot{}, false
	}
	return s.spotAt(line, s.Caret.Byte), true
}

// shownLine is the line of the body that shows row of file, on the caret's
// side if the unified view splits it; ok is false for a row folded away
// and a file not laid out.
func (s *state) shownLine(file, row int) (int, bool) {
	h := view.NewHalf
	if s.Caret.Side == filediff.OldSide {
		h = view.OldHalf
	}
	lines := s.linesOf(file)
	i := view.LineFor(lines, row, h)
	if i >= len(lines) || lines[i].Fold || lines[i].Rows.Start != row {
		return 0, false
	}
	return s.bodyIndex(file, i), true
}

// onScreen says line of the body is on the screen.
func (s *state) onScreen(line int) bool {
	return line >= s.top() && line < s.top()+s.Layout.shown()
}

// startCaret puts the caret on the first text of the screen, from the
// first column shown: on the right side, unless only the left one has a
// document.
func (s *state) startCaret() {
	s.Caret.Side = filediff.NewSide
	if s.snapOf(s.Current).RightDoc.Path == "" {
		s.Caret.Side = filediff.OldSide
	}
	line, ok := s.caretLine(s.top(), 1, s.top()+s.Layout.shown())
	if !ok {
		return
	}
	p := s.spotAtCol(line, s.HScroll)
	for p.class() == blankClass {
		q, crossed, ok := s.caretStep(p, 1)
		if !ok || crossed {
			break
		}
		p = q
	}
	s.moveCaret(p)
}

// otherSide moves the caret to the other side, at the same column of the
// nearest line there it may stand on, below first.
func (s *state) otherSide() {
	p, ok := s.spot()
	if !ok {
		return
	}
	if s.Layout.Unified {
		s.otherHalf(p)
		return
	}
	was := s.Caret
	s.Caret.Side = filediff.OldSide
	if was.Side == filediff.OldSide {
		s.Caret.Side = filediff.NewSide
	}
	line, ok := s.caretLine(p.line, 1, -1)
	if !ok {
		line, ok = s.caretLine(p.line, -1, -1)
	}
	if !ok {
		s.Caret = was
		return
	}
	s.moveCaret(s.spotAtCol(line, p.c.Col))
}

// otherHalf is otherSide in the unified view: from one half of a changed
// row to its other, at the same column. Whole rows and rows of one half
// have no other side to go to.
func (s *state) otherHalf(p spot) {
	file, l, _ := s.rowLine(p.line)
	h := view.OldHalf
	switch l.Half {
	case view.Whole:
		return
	case view.OldHalf:
		h = view.NewHalf
	case view.NewHalf:
	}
	lines := s.linesOf(file)
	i := view.LineFor(lines, l.Rows.Start, h)
	if lines[i].Half != h || lines[i].Rows.Start != l.Rows.Start {
		return
	}
	i = s.bodyIndex(file, i)
	if _, _, ok := s.caretText(i); ok {
		s.moveCaret(s.spotAtCol(i, p.c.Col))
	}
}

// moveCaret puts the caret on p and brings it into view.
func (s *state) moveCaret(p spot) {
	file, l, _ := s.rowLine(p.line)
	s.Caret = caret{On: true, Side: s.lineSide(file, l, s.Caret.Side), File: file, Row: l.Rows.Start, Byte: p.c.Start}
	if h := s.Layout.shown(); p.line < s.top() {
		s.scrollTo(p.line)
	} else if p.line >= s.top()+h {
		s.scrollTo(p.line - h + 1)
	}
	from, to := s.layoutOf(file).text(s.Caret.Side)
	switch w := to - from; {
	case w <= 0:
	case p.c.Col < s.HScroll:
		s.HScroll = p.c.Col
	case p.c.Col+p.c.Width > s.HScroll+w:
		s.HScroll = p.c.Col + p.c.Width - w
	}
}

// caretUpDown moves the caret to the next line it may stand on, by dir,
// at the same column or the line's end.
func (s *state) caretUpDown(dir int) {
	p, ok := s.spot()
	if !ok {
		return
	}
	if line, ok := s.caretLine(p.line+dir, dir, -1); ok {
		s.moveCaret(s.spotAtCol(line, p.c.Col))
	}
}

// caretSideways moves the caret by one grapheme within its line.
func (s *state) caretSideways(dir int) {
	if p, ok := s.spot(); ok {
		if q, crossed, ok := s.caretStep(p, dir); ok && !crossed {
			s.moveCaret(q)
		}
	}
}

// wordForward moves to the start of the next word, as vim's w: past the
// rest of this one and the blanks after it, a line's end counting as one.
func (s *state) wordForward() {
	if p, ok := s.spot(); ok {
		if q, ok := s.nextWord(s.wordEdge(p, 1), 1); ok {
			s.moveCaret(q)
		}
	}
}

// wordEnd moves to the end of this word, or of the next one if already at
// it, as vim's e.
func (s *state) wordEnd() {
	if p, ok := s.spot(); ok {
		if q, ok := s.nextWord(p, 1); ok {
			s.moveCaret(s.wordEdge(q, 1))
		}
	}
}

// wordBack moves to the start of this word, or of the one before if
// already at it, as vim's b.
func (s *state) wordBack() {
	if p, ok := s.spot(); ok {
		if q, ok := s.nextWord(p, -1); ok {
			s.moveCaret(s.wordEdge(q, -1))
		}
	}
}

// nextWord is the first grapheme after p by dir that is no blank.
func (s *state) nextWord(p spot, dir int) (spot, bool) {
	for {
		var ok bool
		if p, _, ok = s.caretStep(p, dir); !ok {
			return spot{}, false
		}
		if p.class() != blankClass {
			return p, true
		}
	}
}

// wordEdge is the last grapheme of p's word by dir, within its line.
func (s *state) wordEdge(p spot, dir int) spot {
	for {
		q, crossed, ok := s.caretStep(p, dir)
		if !ok || crossed || q.class() != p.class() {
			return p
		}
		p = q
	}
}

// keepCaret keeps the caret on a grapheme it may stand on within the
// screen, after the view or the text changed under it: on the nearest
// such line, at the nearest column shown. With no such line on the screen
// it stays, unseen, if its own line is still one; else it goes.
func (s *state) keepCaret() {
	if !s.Caret.On {
		return
	}
	p, ok := s.spot()
	if ok && s.onScreen(p.line) {
		// The unified view may have found the row's other half, after a
		// reload made it a row of one side.
		s.Caret.Side, s.Caret.Byte = s.caretSide(p.line), s.keepColumn(p).c.Start
		return
	}
	top, bottom := s.top(), s.top()+s.Layout.shown()
	if bottom <= top {
		return
	}
	col, line := p.c.Col, s.rowIndex(s.Caret.File, s.Caret.Row)
	from := min(max(line, top), bottom-1)
	l, found := s.caretLine(from, 1, bottom)
	if !found {
		l, found = s.caretLine(from, -1, top-1)
	}
	switch {
	case found:
		q := s.keepColumn(s.spotAtCol(l, col))
		file, row, _ := s.rowLine(l)
		s.Caret.Side, s.Caret.File, s.Caret.Row, s.Caret.Byte = s.lineSide(file, row, s.Caret.Side), file, row.Rows.Start, q.c.Start
	case ok:
		s.Caret.Byte = p.c.Start
	default:
		s.Caret = caret{}
	}
}

// keepColumn is the grapheme of p's line nearest p within the columns of
// its pane's text, or p if none of them shows.
func (s *state) keepColumn(p spot) spot {
	file, _, _ := s.rowLine(p.line)
	from, to := s.layoutOf(file).text(s.Caret.Side)
	left, right := s.HScroll, s.HScroll+to-from
	if right <= left || !cut(p.c, left, right) {
		return p
	}
	q := s.spotAtCol(p.line, min(max(p.c.Col, left), right-1))
	for cut(q.c, left, right) {
		var ok bool
		if q, _, ok = s.caretStep(q, 1); !ok || q.line != p.line {
			return p
		}
	}
	return q
}

// caretCell is the screen cell of the caret at p; ok is false off the
// screen.
func (s *state) caretCell(p spot) (x, y int, ok bool) {
	if !s.onScreen(p.line) {
		return 0, 0, false
	}
	from, _ := s.layoutOf(s.Caret.File).text(s.Caret.Side)
	return from + p.c.Col - s.HScroll, bodyTop + p.line - s.top(), true
}
