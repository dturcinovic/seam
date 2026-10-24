package tui

import (
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/syntax"
	"seam/internal/textcell"
)

// unicodeCore is the terminal mode in which a grapheme counts as one glyph.
// Bubble Tea asks for the DEC mode of that number. It also tells it from an
// ANSI mode of the same number, which this package cannot without x/ansi;
// no terminal reports that one unasked.
const unicodeCore = 2027

// reportsGraphemes tells whether Bubble Tea's renderer counts graphemes
// after report r: it switches on exactly these values.
func reportsGraphemes(r tea.ModeReportMsg) bool {
	if r.Mode == nil || r.Mode.Mode() != unicodeCore {
		return false
	}
	v := r.Value
	return v.IsSet() || (v.IsReset() && !v.IsPermanentlyReset())
}

// line writes one screen line of exactly its width into b, switching
// styles only where they change. Foreign text goes through textcell, so no
// byte of it reaches the terminal unreplaced.
type line struct {
	b      *strings.Builder
	th     *theme
	method textcell.Method
	free   int
	cur    style
	buf    [64]byte
}

func newLine(b *strings.Builder, th *theme, m textcell.Method, width int) line {
	return line{b: b, th: th, method: m, free: max(width, 0)}
}

// use switches to st. Styles that look the same need no switch, such as the
// separator and the gutter of an equal line.
func (l *line) use(st style) {
	if st == l.cur {
		return
	}
	if l.th.open[st] != l.th.open[l.cur] {
		l.b.WriteString(l.th.close[l.cur])
		l.b.WriteString(l.th.open[st])
	}
	l.cur = st
}

// text writes s from its first column on, as far as it fits.
func (l *line) text(s string, st style) {
	l.window(s, nil, 0, cellStyle{fixed: st})
}

// spaces writes n blank columns, as far as they fit.
func (l *line) spaces(n int, st style) {
	l.use(st)
	n = max(min(n, l.free), 0)
	for range n {
		l.b.WriteByte(' ')
	}
	l.free -= n
}

// number writes n right-aligned in width columns, as far as it fits.
func (l *line) number(n, width int, st style) {
	digits := strconv.AppendInt(l.buf[:0], int64(n), 10)
	l.spaces(width-len(digits), st)
	digits = digits[:min(len(digits), l.free)]
	l.b.Write(digits)
	l.free -= len(digits)
}

// narrow keeps the next writes within width columns and returns the free
// columns to restore after them.
func (l *line) narrow(width int) (rest int) {
	width = min(width, l.free)
	rest = l.free - width
	l.free = width
	return rest
}

// end fills the rest of the line in st and switches the style off.
func (l *line) end(st style) {
	l.spaces(l.free, st)
	l.use(plain)
}

// window writes s from column skip on as far as the line has room, styled
// per cell by st, and reads s no further. points let it start at a
// checkpoint rather than at the start of a long line. A grapheme cut by
// either edge shows as blank columns in its style.
func (l *line) window(s string, points []filediff.Checkpoint, skip int, st cellStyle) {
	end := skip + l.free
	cells := seek(s, points, skip, l.method)
	for l.free > 0 {
		c, ok := cells.Next()
		if !ok {
			return
		}
		l.use(st.of(c))
		if cut(c, skip, end) {
			l.spaces(min(c.Col+c.Width, end)-max(c.Col, skip), l.cur)
			continue
		}
		if g, ok := c.Verbatim(s); ok {
			l.b.WriteString(g)
		} else {
			l.b.Write(c.Append(l.buf[:0], s))
		}
		l.free -= c.Width
	}
}

// cut says that c reaches past column skip or end on the screen, so only
// its columns between them show, as blanks.
func cut(c textcell.Cell, skip, end int) bool {
	return c.Col < skip || c.Col+c.Width > end
}

// seek walks s from the first cell that reaches past column skip, starting
// at the last checkpoint at or before it. It is the one way to find a column
// in a line, so whatever looks one up agrees with what is drawn there.
func seek(s string, points []filediff.Checkpoint, skip int, m textcell.Method) textcell.Cells {
	col := func(p filediff.Checkpoint) int {
		if m == textcell.Graphemes {
			return p.GraphemeCol
		}
		return p.Col
	}
	cells := textcell.New(s, m)
	if i := sort.Search(len(points), func(i int) bool { return col(points[i]) > skip }); i > 0 {
		cells = textcell.Resume(s, points[i-1].Byte, col(points[i-1]), m)
	}
	cells.Skip(skip)
	return cells
}

// cellStyle picks the style of each cell: fixed, or in a pane's tone with
// the inline spans stronger and coloured by the syntax runs. Spans and runs
// are ordered and walked along with the cells, so it is used for one walk
// only.
type cellStyle struct {
	fixed style
	pane  bool
	tone  tone
	spans []diff.Span
	runs  []syntax.Run
}

func (cs *cellStyle) of(c textcell.Cell) style {
	if !cs.pane {
		if c.Replaced {
			return cs.fixed.standIn()
		}
		return cs.fixed
	}
	for len(cs.spans) > 0 && cs.spans[0].End <= c.Start {
		cs.spans = cs.spans[1:]
	}
	// A span may start inside a grapheme, as the word diff splits a skin
	// tone or flag off its emoji; the whole grapheme then counts as changed.
	inSpan := len(cs.spans) > 0 && cs.spans[0].Start < c.End
	switch {
	case inSpan && c.Replaced:
		return paneStyle(cs.tone, inlineStandInRole)
	case c.Replaced:
		return paneStyle(cs.tone, standInRole)
	}
	return textStyle(cs.tone, cs.kind(c), inSpan)
}

// kind is the syntax kind of c. The runs follow each other from the line's
// start, so the first one left holds the cell.
func (cs *cellStyle) kind(c textcell.Cell) syntax.Kind {
	for len(cs.runs) > 0 && int(cs.runs[0].End) <= c.Start {
		cs.runs = cs.runs[1:]
	}
	if len(cs.runs) == 0 {
		return syntax.Plain
	}
	return cs.runs[0].Kind
}

// textWidth is the columns s takes on one line, as line draws it.
func textWidth(s string, m textcell.Method) int {
	cells := textcell.New(s, m)
	w := 0
	for c, ok := cells.Next(); ok; c, ok = cells.Next() {
		w = c.Col + c.Width
	}
	return w
}
