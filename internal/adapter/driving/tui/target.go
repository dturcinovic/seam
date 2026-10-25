package tui

import (
	"unicode"
	"unicode/utf8"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/view"
	"seam/internal/textcell"
)

// hoverTarget is the grapheme under the mouse: Line of Side of File and
// the grapheme's first Byte in that line. Word is the bytes of the identifier
// around it, or of the grapheme alone, so moving within one identifier
// asks no new question.
type hoverTarget struct {
	File       int
	Side       filediff.Which
	Line, Byte int
	Word       diff.Span
}

// wordReach bounds how far an identifier is looked for either side of the
// grapheme: a minified line may be one identifier of 64 KiB, and the mouse
// asks on every move.
const wordReach = 1 << 10

// targetAt is what a hover at screen cell x, y asks about; within prev's
// identifier it keeps prev's Word rather than look for it again. ok is false
// where there is nothing to ask: outside the panes' text, on a fold, past a
// line's end, on a grapheme cut by an edge, and where targetOn or
// hoverLine find nothing.
func (s *state) targetAt(x, y int, prev hoverTarget) (hoverTarget, bool) {
	i, ok := s.Layout.bodyLine(x, y)
	if !ok {
		return hoverTarget{}, false
	}
	file, l, ok := s.rowLine(s.top() + i)
	if !ok || l.Fold {
		return hoverTarget{}, false
	}
	lay := s.layoutOf(file)
	w, ok := lay.sideAt(x)
	if !ok {
		return hoverTarget{}, false
	}
	w = s.lineSide(file, l, w)
	text, points, ok := s.hoverLine(file, w, l.Rows.Start)
	if !ok {
		return hoverTarget{}, false
	}
	from, to := lay.text(w)
	skip, end := s.HScroll, s.HScroll+to-from
	cells := seek(text, points, skip+x-from, s.Cells)
	c, ok := cells.Next()
	if !ok || cut(c, skip, end) {
		return hoverTarget{}, false
	}
	return targetOn(file, w, lineOf(s.snapOf(file).Diff.Rows[l.Rows.Start], w), text, c, prev)
}

// lineSide is the side of line l that w points at: w itself side by side,
// but in the unified view, where both sides share the columns, the line's.
func (s *state) lineSide(file int, l view.Line, w filediff.Which) filediff.Which {
	if s.Layout.Unified {
		return s.halfSide(file, l)
	}
	return w
}

// halfSide is the side line l of the unified view shows: its half's, or for
// a whole row the new side, unless only the old one has a document.
func (s *state) halfSide(file int, l view.Line) filediff.Which {
	switch l.Half {
	case view.OldHalf:
		return filediff.OldSide
	case view.NewHalf:
		return filediff.NewSide
	case view.Whole:
	}
	if s.snapOf(file).RightDoc.Path == "" {
		return filediff.OldSide
	}
	return filediff.NewSide
}

// hoverLine is the text of side w of row of file and its column checkpoints, if
// hovers may ask about it: the side has a document, as one from the index
// has not, and the line is no longer than hover.MaxLine.
func (s *state) hoverLine(file int, w filediff.Which, row int) (string, []filediff.Checkpoint, bool) {
	snap := s.snapOf(file)
	d := snap.Diff
	if snap.Doc(w).Path == "" || row < 0 || row >= len(d.Rows) {
		return "", nil, false
	}
	i := lineOf(d.Rows[row], w)
	if i < 0 {
		return "", nil, false
	}
	lines, columns := d.New.Lines, snap.Columns.New
	if w == filediff.OldSide {
		lines, columns = d.Old.Lines, snap.Columns.Old
	}
	text := diff.Content(lines[i])
	if len(text) > hover.MaxLine {
		return "", nil, false
	}
	return text, columns.At(i), true
}

// lineOf is the line of side w in row r, -1 for none.
func lineOf(r diff.Row, w filediff.Which) int {
	if w == filediff.OldSide {
		return r.Left
	}
	return r.Right
}

// targetOn is what a hover asks about grapheme c of line of side w of
// file, of text; nothing on blanks and stand-ins.
func targetOn(file int, w filediff.Which, line int, text string, c textcell.Cell, prev hoverTarget) (hoverTarget, bool) {
	first, _ := utf8.DecodeRuneInString(text[c.Start:])
	if c.Replaced || unicode.IsSpace(first) {
		return hoverTarget{}, false
	}
	word := diff.Span{Start: c.Start, End: c.End}
	switch {
	case prev.File == file && prev.Side == w && prev.Line == line && prev.Word.Start <= c.Start && c.Start < prev.Word.End:
		word = prev.Word
	case identifier(first):
		word = identifierAround(text, c.Start, c.End)
	}
	return hoverTarget{File: file, Side: w, Line: line, Byte: c.Start, Word: word}, true
}

func identifier(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

// wordRune says r belongs to an identifier: marks join it, as the accent
// of a decomposed é does.
func wordRune(r rune) bool { return identifier(r) || unicode.Is(unicode.M, r) }

// identifierAround widens start, end to the identifier holding it, by at
// most wordReach either side.
func identifierAround(text string, start, end int) diff.Span {
	first, last := max(start-wordReach, 0), min(end+wordReach, len(text))
	for start > first {
		r, n := utf8.DecodeLastRuneInString(text[:start])
		if !wordRune(r) {
			break
		}
		start -= n
	}
	for end < last {
		r, n := utf8.DecodeRuneInString(text[end:])
		if !wordRune(r) {
			break
		}
		end += n
	}
	return diff.Span{Start: start, End: end}
}
