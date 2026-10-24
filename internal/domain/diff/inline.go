package diff

import (
	"context"
	"slices"
	"unicode"
	"unicode/utf8"
)

// Span is the bytes of a line's Content from Start up to but not including
// End.
type Span struct {
	Start, End int
}

// maxInlineLine is the longest content, in bytes, that gets marked within:
// beyond it the token search costs more than the marks are worth.
const maxInlineLine = 4 << 10

// markInline marks the changed words of the Change rows, in row order;
// Delete and Insert rows are marked whole anyway. complete is false if ctx
// ended first and left the rest unmarked.
func markInline(ctx context.Context, rows []Row, old, new Side) (marks []InlineSpans, complete bool) {
	complete = true
	for i, r := range rows {
		if r.Kind != Change {
			continue
		}
		left, right, ok := inline(ctx, old.Lines[r.Left], new.Lines[r.Right])
		if !ok {
			complete = false
			break
		}
		if left != nil || right != nil {
			marks = append(marks, InlineSpans{Row: i, Left: left, Right: right})
		}
	}
	// Exact, as the diff keeps them: append leaves up to half again spare.
	return slices.Clone(marks), complete
}

// inline returns the spans of the tokens that differ between two lines. It
// marks nothing if the contents are equal, a line is too long, or more than
// 60 % of the bytes changed, as marking nearly everything is noise. ok is
// false if ctx ended first.
func inline(ctx context.Context, old, new string) (left, right []Span, ok bool) {
	old, new = Content(old), Content(new)
	if old == new || len(old) > maxInlineLine || len(new) > maxInlineLine {
		return nil, nil, true
	}
	// At least the difference in length changed: this rejects pairs that only
	// share a position in a rewritten block before they cost any search.
	if tooMuch(max(len(old), len(new))-min(len(old), len(new)), len(old)+len(new)) {
		return nil, nil, true
	}
	if ctx.Err() != nil {
		return nil, nil, false
	}
	ta, tb := tokens(old), tokens(new)
	a, b := intern(ta, tb)
	matches, complete := commonSubsequence(ctx, a, b)
	if !complete {
		return nil, nil, false
	}
	kept := make([]bool, len(ta)+len(tb))
	keptA, keptB := kept[:len(ta)], kept[len(ta):]
	common := 0
	for _, m := range matches {
		keptA[m.old], keptB[m.new] = true, true
		common += len(ta[m.old])
	}
	if tooMuch(len(old)+len(new)-2*common, len(old)+len(new)) {
		return nil, nil, true
	}
	return spans(ta, keptA), spans(tb, keptB), true
}

// tooMuch reports that more than 60 % of the bytes changed.
func tooMuch(changed, total int) bool {
	return changed*5 > total*3
}

// spans merges runs of tokens that were not kept.
func spans(toks []string, kept []bool) []Span {
	var out []Span
	pos := 0
	for i, tok := range toks {
		end := pos + len(tok)
		if !kept[i] {
			if n := len(out); n > 0 && out[n-1].End == pos {
				out[n-1].End = end
			} else {
				out = append(out, Span{Start: pos, End: end})
			}
		}
		pos = end
	}
	return out
}

// tokens cuts s into runs of word characters, runs of space and single other
// characters, so that a changed word is marked whole. It counts them first:
// growing the slice cost more allocations than the rest of a line's marking.
func tokens(s string) []string {
	n := 0
	for rest := s; rest != ""; rest = rest[token(rest):] {
		n++
	}
	out := make([]string, 0, n)
	for s != "" {
		n := token(s)
		out = append(out, s[:n])
		s = s[n:]
	}
	return out
}

// token is the length in bytes of the token s starts with.
func token(s string) int {
	r, n := utf8.DecodeRuneInString(s)
	in := runOf(r)
	for prev := r; n < len(s); {
		next, size := utf8.DecodeRuneInString(s[n:])
		if !extends(prev, next) && (in == nil || !in(next)) {
			break
		}
		prev, n = next, n+size
	}
	return n
}

// zwj joins the runes on both sides into one character, as in emoji.
const zwj = '\u200d'

// extends reports that next belongs to the character before it, so that no
// span splits a character a terminal shows as one: a combining mark, or what
// a zero width joiner joins. No mark is ASCII, and code is nearly all ASCII:
// looking up the table for every rune made marking half again as slow.
func extends(prev, next rune) bool {
	if prev == zwj || next == zwj {
		return true
	}
	return next >= utf8.RuneSelf && unicode.Is(unicode.M, next)
}

// runOf is the class of characters a token starting with r runs on, or nil
// if r stands alone.
func runOf(r rune) func(rune) bool {
	switch {
	case isWord(r):
		return isWord
	case unicode.IsSpace(r):
		return unicode.IsSpace
	default:
		return nil
	}
}

// isWord leaves out Han, Hiragana and Katakana: these scripts do not space
// their words, so a run would be a whole sentence.
func isWord(r rune) bool {
	if r >= utf8.RuneSelf && unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
		return false
	}
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
