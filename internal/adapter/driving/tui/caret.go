package tui

import (
	"math"
	"unicode"
	"unicode/utf8"

	"seam/internal/domain/filediff"
	"seam/internal/domain/view"
	"seam/internal/textcell"
)

// caret is the keyboard's stand-in for the mouse, for terminals that send
// no motion: a grapheme of Side, at Byte of that side's line of Row. A row
// rather than a line of the view, so it stays put when the view changes
// around it.
type caret struct {
	On   bool
	Side filediff.Which
	Row  int
	Byte int
}

// spot is a grapheme the caret may stand on: c of the text of line of the
// view, with its column checkpoints.
type spot struct {
	line   int
	text   string
	points []filediff.Checkpoint
	c      textcell.Cell
}

// caretText is the text of line of the view, if the caret may stand on
// it: a line hovers may ask about, with text, not a fold.
func (s *state) caretText(line int) (string, []filediff.Checkpoint, bool) {
	if line < 0 || line >= len(s.Lines) || s.Lines[line].Fold {
		return "", nil, false
	}
	text, points, ok := s.hoverLine(s.Caret.Side, s.Lines[line].Rows.Start)
	return text, points, ok && text != ""
}

// caretLine is the first line from i on, going by dir and stopping short
// of stop, that the caret may stand on.
func (s *state) caretLine(i, dir, stop int) (int, bool) {
	for ; i != stop && i >= 0 && i < len(s.Lines); i += dir {
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
	r, _ := utf8.DecodeRuneInString(p.text[p.c.Start:])
	switch {
	case p.c.Replaced:
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
	line, ok := s.shownLine(s.Caret.Row)
	if !s.Caret.On || !ok {
		return spot{}, false
	}
	if _, _, ok := s.caretText(line); !ok {
		return spot{}, false
	}
	return s.spotAt(line, s.Caret.Byte), true
}

// shownLine is the line of the view that shows row on its own; ok is false
// for a row folded away.
func (s *state) shownLine(row int) (int, bool) {
	i := view.LineForRow(s.Lines, row)
	if i >= len(s.Lines) || s.Lines[i].Fold || s.Lines[i].Rows.Start != row {
		return 0, false
	}
	return i, true
}

// onScreen says line of the view is on the screen.
func (s *state) onScreen(line int) bool {
	return line >= s.Pos.Scroll && line < s.Pos.Scroll+s.Layout.shown()
}

// startCaret puts the caret on the first text of the screen, from the
// first column shown: on the right side, unless only the left one has a
// document.
func (s *state) startCaret() {
	s.Caret.Side = filediff.NewSide
	if s.Snapshot.RightDoc.Path == "" {
		s.Caret.Side = filediff.OldSide
	}
	line, ok := s.caretLine(s.Pos.Scroll, 1, s.Pos.Scroll+s.Layout.shown())
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

// moveCaret puts the caret on p and brings it into view.
func (s *state) moveCaret(p spot) {
	s.Caret = caret{On: true, Side: s.Caret.Side, Row: s.Lines[p.line].Rows.Start, Byte: p.c.Start}
	if h := s.Layout.shown(); p.line < s.Pos.Scroll {
		s.scrollTo(p.line)
	} else if p.line >= s.Pos.Scroll+h {
		s.scrollTo(p.line - h + 1)
	}
	from, to := s.Layout.text(s.Caret.Side)
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
		s.Caret.Byte = s.keepColumn(p).c.Start
		return
	}
	top, bottom := s.Pos.Scroll, s.Pos.Scroll+s.Layout.shown()
	if bottom <= top {
		return
	}
	col, line := p.c.Col, view.LineForRow(s.Lines, s.Caret.Row)
	from := min(max(line, top), bottom-1)
	l, found := s.caretLine(from, 1, bottom)
	if !found {
		l, found = s.caretLine(from, -1, top-1)
	}
	switch {
	case found:
		q := s.keepColumn(s.spotAtCol(l, col))
		s.Caret.Row, s.Caret.Byte = s.Lines[l].Rows.Start, q.c.Start
	case ok:
		s.Caret.Byte = p.c.Start
	default:
		s.Caret = caret{}
	}
}

// keepColumn is the grapheme of p's line nearest p within the columns of
// its pane's text, or p if none of them shows.
func (s *state) keepColumn(p spot) spot {
	from, to := s.Layout.text(s.Caret.Side)
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
	from, _ := s.Layout.text(s.Caret.Side)
	return from + p.c.Col - s.HScroll, bodyTop + p.line - s.Pos.Scroll, true
}
