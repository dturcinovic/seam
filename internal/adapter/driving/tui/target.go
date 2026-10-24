package tui

import (
	"unicode"
	"unicode/utf8"

	"seam/internal/domain/diff"
	"seam/internal/domain/hover"
)

// hoverTarget is the grapheme under the mouse in the right pane: Line of
// the right side and the grapheme's first Byte in that line. Word is the
// bytes of the identifier around it, or of the grapheme alone, so moving
// within one identifier asks no new question.
type hoverTarget struct {
	Line, Byte int
	Word       diff.Span
}

// targetAt is what a hover at screen cell x, y asks about. ok is false
// where there is nothing to ask: outside the right pane's text, as hovers
// see only the worktree (G0.2), on a fold, past a line's end, on blanks
// and stand-ins, on a grapheme cut by an edge, and in a line past
// hover.MaxLine.
func (s *state) targetAt(x, y int) (hoverTarget, bool) {
	i, ok := s.Layout.bodyLine(x, y)
	if !ok || s.Snapshot.RightDoc.Path == "" || s.Pos.Scroll+i >= len(s.Lines) {
		return hoverTarget{}, false
	}
	l := s.Lines[s.Pos.Scroll+i]
	if l.Fold {
		return hoverTarget{}, false
	}
	row := s.Snapshot.Diff.Rows[l.Rows.Start]
	if row.Right < 0 {
		return hoverTarget{}, false
	}
	from, to := s.Layout.rightText()
	if x < from || x >= to {
		return hoverTarget{}, false
	}
	text := diff.Content(s.Snapshot.Diff.New.Lines[row.Right])
	if len(text) > hover.MaxLine {
		return hoverTarget{}, false
	}
	skip, end := s.HScroll, s.HScroll+to-from
	cells := seek(text, s.Snapshot.Columns.New.At(row.Right), skip+x-from, s.Cells)
	c, ok := cells.Next()
	if !ok || cut(c, skip, end) || c.Replaced {
		return hoverTarget{}, false
	}
	first, _ := utf8.DecodeRuneInString(text[c.Start:])
	if unicode.IsSpace(first) {
		return hoverTarget{}, false
	}
	word := diff.Span{Start: c.Start, End: c.End}
	if identifier(first) {
		word = identifierAround(text, c.Start, c.End)
	}
	return hoverTarget{Line: row.Right, Byte: c.Start, Word: word}, true
}

func identifier(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
}

// identifierAround widens start, end to the identifier holding it; marks
// join it, as the accent of a decomposed é does.
func identifierAround(text string, start, end int) diff.Span {
	part := func(r rune) bool { return identifier(r) || unicode.Is(unicode.M, r) }
	for start > 0 {
		r, n := utf8.DecodeLastRuneInString(text[:start])
		if !part(r) {
			break
		}
		start -= n
	}
	for end < len(text) {
		r, n := utf8.DecodeRuneInString(text[end:])
		if !part(r) {
			break
		}
		end += n
	}
	return diff.Span{Start: start, End: end}
}
