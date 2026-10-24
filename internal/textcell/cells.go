package textcell

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// TabWidth is the distance between tab stops, in columns.
const TabWidth = 4

// Method is how a terminal counts the columns of a grapheme. The graphemes
// are the same under both; only their widths differ, as for a ZWJ emoji
// sequence, which is one glyph but several code points.
type Method uint8

const (
	// Codepoints adds up the widths of a grapheme's code points, as
	// terminals do by default (wcwidth).
	Codepoints Method = iota
	// Graphemes counts a grapheme as one glyph, as terminals do in Unicode
	// core mode (mode 2027).
	Graphemes
)

// Cell is one grapheme of a text as the terminal shows it. Start and End are
// its bytes in the text, Col its first column counted from the start of the
// line, and Width the columns it takes. A Replaced grapheme must not reach
// the terminal as it is: Append writes a visible stand-in instead, which
// should look different from text.
type Cell struct {
	Start, End int
	Col, Width int
	Replaced   bool
}

// Cells walks the cells of a text. Its zero value walks nothing.
type Cells struct {
	text   string
	pos    int
	col    int
	method Method
}

// New walks text from its start.
func New(text string, m Method) Cells {
	return Cells{text: text, method: m}
}

// Resume walks text from byte pos, which must be a grapheme boundary at
// column col, as a walk from the start reaches it. Tab stops count from the
// start of the line, so col must be exact.
func Resume(text string, pos, col int, m Method) Cells {
	return Cells{text: text, pos: pos, col: col, method: m}
}

// Next returns the next cell; ok is false at the end of the text.
func (c *Cells) Next() (cell Cell, ok bool) {
	if c.pos >= len(c.text) {
		return Cell{}, false
	}
	n, width, replaced := measure(c.text[c.pos:], c.col, c.method)
	cell = Cell{Start: c.pos, End: c.pos + n, Col: c.col, Width: width, Replaced: replaced}
	c.pos += n
	c.col += width
	return cell, true
}

// Skip moves past every cell that ends at or before column col, without
// building them: scrolled far right, a line is mostly skipped.
func (c *Cells) Skip(col int) {
	for c.pos < len(c.text) && c.col < col {
		if plainASCII(c.text[c.pos:]) {
			c.pos++
			c.col++
			continue
		}
		n, width, _ := measure(c.text[c.pos:], c.col, c.method)
		if c.col+width > col {
			return
		}
		c.pos += n
		c.col += width
	}
}

// maxGrapheme bounds a grapheme in bytes; the marks beyond it become cells
// of their own. Without it, one letter with megabytes of combining marks
// would be segmented and written whole in every frame.
const maxGrapheme = 128

// kind is what cut found at the start of a text.
type kind uint8

const (
	asciiKind kind = iota
	tabKind
	standInKind
	clusterKind
)

// cut returns the length of the grapheme at the start of s, what it is, and
// for a cluster its width under m, segmenting it once. Controls and invalid
// bytes are cut off as single characters before segmenting: a control
// stands alone as a grapheme anyway, and an invalid byte must not ride along
// in one.
func cut(s string, m Method) (n int, k kind, width int) {
	if plainASCII(s) {
		return 1, asciiKind, 1
	}
	r, size := utf8.DecodeRuneInString(s)
	switch {
	case r == '\t':
		return size, tabKind, 0
	case r == utf8.RuneError && size == 1, isControl(r):
		return size, standInKind, 0
	}
	am := ansi.WcWidth
	if m == Graphemes {
		am = ansi.GraphemeWidth
	}
	cluster, width := ansi.FirstGraphemeCluster(s[:min(len(s), maxGrapheme)], am)
	if at := invalidAt(cluster); at > 0 {
		cluster = cluster[:at]
		width = am.StringWidth(cluster)
	}
	return len(cluster), clusterKind, width
}

// measure returns the length of the cell at the start of s, at column col,
// its width under m and whether it is replaced.
func measure(s string, col int, m Method) (n, width int, replaced bool) {
	n, k, width := cut(s, m)
	width, replaced = widthOf(s[:n], k, width, col)
	return n, width, replaced
}

// widthOf is the width of the grapheme g that cut found, at column col, and
// whether it is replaced; a cluster's width comes from cut.
func widthOf(g string, k kind, clusterWidth, col int) (width int, replaced bool) {
	switch k {
	case asciiKind:
		return 1, false
	case tabKind:
		return TabWidth - col%TabWidth, false
	case standInKind:
		return standInWidth(g), true
	case clusterKind:
	}
	if clusterWidth == 0 {
		// It would join the cell before it, which may be the gutter.
		return standInWidth(g), true
	}
	return clusterWidth, false
}

// plainASCII reports printable ASCII at the start of s that nothing after it
// can join.
func plainASCII(s string) bool {
	b := s[0]
	return b >= 0x20 && b < 0x7f && (len(s) == 1 || s[1] < 0x80)
}

// isControl reports what "Steuerzeichen im Terminal" in the plan replaces:
// C0 controls, DEL, C1 controls and the bidi controls, which can reorder
// what the terminal shows.
func isControl(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f) ||
		(r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

// invalidAt is the offset of the first invalid byte in s, or 0 if there is
// none; s starts with a valid character.
func invalidAt(s string) int {
	for i, r := range s {
		if r == utf8.RuneError {
			if _, n := utf8.DecodeRuneInString(s[i:]); n == 1 {
				return i
			}
		}
	}
	return 0
}

// Verbatim returns the grapheme of cell c in text when the terminal gets it
// as it is, which is all but tabs and replaced ones.
func (c Cell) Verbatim(text string) (string, bool) {
	s := text[c.Start:c.End]
	return s, !c.Replaced && s != "\t"
}

// Append writes cell c of text to dst as the terminal should get it: the
// grapheme itself, a tab as spaces up to its stop, or the stand-in of a
// replaced one.
func (c Cell) Append(dst []byte, text string) []byte {
	s, verbatim := c.Verbatim(text)
	switch {
	case verbatim:
		return append(dst, s...)
	case c.Replaced:
		return appendStandIn(dst, s)
	}
	for range c.Width {
		dst = append(dst, ' ')
	}
	return dst
}

// standInWidth is the width of the stand-in for s: one column per rune, as
// stand-ins are ASCII or control pictures.
func standInWidth(s string) int {
	var buf [16]byte
	return utf8.RuneCount(appendStandIn(buf[:0], s))
}

// appendStandIn writes how the replaced grapheme s shows: a C0 control as
// its control picture, DEL as ␡, an invalid byte or a C1 control as \xNN,
// anything else by each of its code points as <U+NNNN>, so two different
// invisible graphemes never look the same.
func appendStandIn(dst []byte, s string) []byte {
	r, size := utf8.DecodeRuneInString(s)
	switch {
	case r == utf8.RuneError && size == 1:
		return appendHex(append(dst, `\x`...), uint32(s[0]), 2)
	case r < 0x20:
		return utf8.AppendRune(dst, 0x2400+r)
	case r == 0x7f:
		return utf8.AppendRune(dst, '␡')
	case r <= 0x9f:
		return appendHex(append(dst, `\x`...), uint32(r), 2)
	}
	for _, r := range s {
		dst = append(appendHex(append(dst, "<U+"...), uint32(r), 4), '>')
	}
	return dst
}

// appendHex writes v in upper case hex with at least digits digits; fmt
// would box v and allocate for every stand-in.
func appendHex(dst []byte, v uint32, digits int) []byte {
	const hex = "0123456789ABCDEF"
	for v>>(4*digits) != 0 {
		digits++
	}
	for i := digits - 1; i >= 0; i-- {
		dst = append(dst, hex[v>>(4*i)&0xf])
	}
	return dst
}
