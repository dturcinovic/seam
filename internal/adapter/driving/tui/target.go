package tui

import (
	"unicode"
	"unicode/utf8"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/textcell"
)

// hoverTarget is the grapheme under the mouse: Line of Side and the
// grapheme's first Byte in that line. Word is the bytes of the identifier
// around it, or of the grapheme alone, so moving within one identifier
// asks no new question.
type hoverTarget struct {
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
	if !ok || s.Pos.Scroll+i >= len(s.Lines) {
		return hoverTarget{}, false
	}
	l := s.Lines[s.Pos.Scroll+i]
	if l.Fold {
		return hoverTarget{}, false
	}
	w, ok := s.Layout.sideAt(x)
	if !ok {
		return hoverTarget{}, false
	}
	text, points, ok := s.hoverLine(w, l.Rows.Start)
	if !ok {
		return hoverTarget{}, false
	}
	from, to := s.Layout.text(w)
	skip, end := s.HScroll, s.HScroll+to-from
	cells := seek(text, points, skip+x-from, s.Cells)
	c, ok := cells.Next()
	if !ok || cut(c, skip, end) {
		return hoverTarget{}, false
	}
	return targetOn(w, lineOf(s.Snapshot.Diff.Rows[l.Rows.Start], w), text, c, prev)
}

// hoverLine is the text of side w of row and its column checkpoints, if
// hovers may ask about it: the side has a document, as one from the index
// has not, and the line is no longer than hover.MaxLine.
func (s *state) hoverLine(w filediff.Which, row int) (string, []filediff.Checkpoint, bool) {
	d := s.Snapshot.Diff
	if s.Snapshot.Doc(w).Path == "" || row < 0 || row >= len(d.Rows) {
		return "", nil, false
	}
	i := lineOf(d.Rows[row], w)
	if i < 0 {
		return "", nil, false
	}
	lines, columns := d.New.Lines, s.Snapshot.Columns.New
	if w == filediff.OldSide {
		lines, columns = d.Old.Lines, s.Snapshot.Columns.Old
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

// targetOn is what a hover asks about grapheme c of line of side w, of
// text; nothing on blanks and stand-ins.
func targetOn(w filediff.Which, line int, text string, c textcell.Cell, prev hoverTarget) (hoverTarget, bool) {
	first, _ := utf8.DecodeRuneInString(text[c.Start:])
	if c.Replaced || unicode.IsSpace(first) {
		return hoverTarget{}, false
	}
	word := diff.Span{Start: c.Start, End: c.End}
	switch {
	case prev.Side == w && prev.Line == line && prev.Word.Start <= c.Start && c.Start < prev.Word.End:
		word = prev.Word
	case identifier(first):
		word = identifierAround(text, c.Start, c.End)
	}
	return hoverTarget{Side: w, Line: line, Byte: c.Start, Word: word}, true
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
