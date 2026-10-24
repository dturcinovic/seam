package tui

import (
	"unicode"
	"unicode/utf8"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/textcell"
)

// hoverTarget is the grapheme under the mouse in the right pane: Line of
// the right side and the grapheme's first Byte in that line. Word is the
// bytes of the identifier around it, or of the grapheme alone, so moving
// within one identifier asks no new question.
type hoverTarget struct {
	Line, Byte int
	Word       diff.Span
}

// wordReach bounds how far an identifier is looked for either side of the
// grapheme: a minified line may be one identifier of 64 KiB, and the mouse
// asks on every move.
const wordReach = 1 << 10

// targetAt is what a hover at screen cell x, y asks about; within prev's
// identifier it keeps prev's Word rather than look for it again. ok is false
// where there is nothing to ask: outside the right pane's text, on a fold,
// past a line's end, on a grapheme cut by an edge, and where targetOn
// or rightLine find nothing.
func (s *state) targetAt(x, y int, prev hoverTarget) (hoverTarget, bool) {
	i, ok := s.Layout.bodyLine(x, y)
	if !ok || s.Pos.Scroll+i >= len(s.Lines) {
		return hoverTarget{}, false
	}
	l := s.Lines[s.Pos.Scroll+i]
	if l.Fold {
		return hoverTarget{}, false
	}
	text, points, ok := s.rightLine(l.Rows.Start)
	from, to := s.Layout.rightText()
	if !ok || x < from || x >= to {
		return hoverTarget{}, false
	}
	skip, end := s.HScroll, s.HScroll+to-from
	cells := seek(text, points, skip+x-from, s.Cells)
	c, ok := cells.Next()
	if !ok || cut(c, skip, end) {
		return hoverTarget{}, false
	}
	return targetOn(s.Snapshot.Diff.Rows[l.Rows.Start].Right, text, c, prev)
}

// rightLine is the text of row's right side and its column checkpoints,
// if hovers may ask about it: the right side is the worktree (G0.2) and
// the line no longer than hover.MaxLine.
func (s *state) rightLine(row int) (string, []filediff.Checkpoint, bool) {
	d := s.Snapshot.Diff
	if s.Snapshot.RightDoc.Path == "" || row < 0 || row >= len(d.Rows) || d.Rows[row].Right < 0 {
		return "", nil, false
	}
	right := d.Rows[row].Right
	text := diff.Content(d.New.Lines[right])
	if len(text) > hover.MaxLine {
		return "", nil, false
	}
	return text, s.Snapshot.Columns.New.At(right), true
}

// targetOn is what a hover asks about grapheme c of line of the right
// side, of text; nothing on blanks and stand-ins.
func targetOn(line int, text string, c textcell.Cell, prev hoverTarget) (hoverTarget, bool) {
	first, _ := utf8.DecodeRuneInString(text[c.Start:])
	if c.Replaced || unicode.IsSpace(first) {
		return hoverTarget{}, false
	}
	word := diff.Span{Start: c.Start, End: c.End}
	switch {
	case prev.Line == line && prev.Word.Start <= c.Start && c.Start < prev.Word.End:
		word = prev.Word
	case identifier(first):
		word = identifierAround(text, c.Start, c.End)
	}
	return hoverTarget{Line: line, Byte: c.Start, Word: word}, true
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
