package textcell

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// leadingEscapes is the escape sequences s starts with.
func leadingEscapes(s string) string {
	rest := s
	for strings.HasPrefix(rest, "\x1b[") {
		end := strings.IndexFunc(rest[2:], func(r rune) bool { return r >= 0x40 && r <= 0x7e })
		if end < 0 {
			break
		}
		rest = rest[2+end+1:]
	}
	return s[:len(s)-len(rest)]
}

// Overlay is the line under, lineWidth columns wide and written for the
// terminal with its escape sequences, with over in its columns from at on.
// A wide grapheme cut by over's edges gives way to blanks, so what follows
// stays in its columns. over starts from no style, whatever under had on.
func Overlay(under string, at int, over string, lineWidth int, m Method) string {
	cut, cutLeft, measure := ansi.Truncate, ansi.TruncateLeft, ansi.StringWidth
	if m == Codepoints {
		cut, cutLeft, measure = ansi.TruncateWc, ansi.TruncateLeftWc, ansi.StringWidthWc
	}
	var b strings.Builder
	left := cut(under, at, "")
	b.WriteString(left)
	b.WriteString(strings.Repeat(" ", max(at-measure(left), 0)))
	if strings.Contains(left, "\x1b") {
		b.WriteString(ansi.ResetStyle)
	}
	b.WriteString(over)
	from := at + measure(over)
	right, after := cutLeft(under, from, ""), max(measure(under)-from, 0)
	// A grapheme reaching back into over is kept by TruncateLeft; the cut
	// one column on drops it.
	for extra := 1; measure(right) > after; extra++ {
		right = cutLeft(under, from+extra, "")
	}
	// The blank for a cut grapheme takes the line's style there.
	lead := leadingEscapes(right)
	b.WriteString(lead)
	b.WriteString(strings.Repeat(" ", max(lineWidth-from-measure(right), 0)))
	b.WriteString(right[len(lead):])
	return b.String()
}
