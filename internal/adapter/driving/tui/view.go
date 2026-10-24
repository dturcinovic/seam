package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/view"
)

// This view is provisional: plain text without styles, counting one cell per
// rune, so that seam can be used before S07 renders the panes properly with
// textcell. Until then it also replaces control characters itself.

// separator divides the panes; sepWidth is its width in cells.
const (
	separator = " │ "
	sepWidth  = 3
)

// tab stands in for tab stops until textcell computes them.
const tab = "    "

// View renders the state. It only reads.
func (m model) View() tea.View {
	v := tea.NewView(render(&m.s))
	v.AltScreen = true
	return v
}

// render writes every line into one builder; a pane is cut to what is
// visible before anything is replaced, so a long line costs no more than a
// short one.
func render(s *state) string {
	var b strings.Builder
	b.Grow((s.Width + 1) * s.Height * 2)
	writeText(&b, header(s), s.Width)
	b.WriteByte('\n')
	for i := range s.bodyHeight() {
		switch j := s.Pos.Scroll + i; {
		case j < len(s.Lines):
			writeLine(&b, s, s.Lines[j])
		case i == 0:
			writeText(&b, placeholder(s), s.Width)
		default:
			pad(&b, 0, s.Width)
		}
		b.WriteByte('\n')
	}
	writeText(&b, footer(s), s.Width)
	return b.String()
}

func header(s *state) string {
	f, ok := s.file()
	if !ok {
		return "no changes"
	}
	// writeText replaces all a path may hold but line breaks, which names
	// show as ␤.
	path := strings.ReplaceAll(f.Path(), "\n", "\u2424")
	h := fmt.Sprintf("%d/%d  %s %s  %s", s.Current+1, len(s.Files), f.Status, path, modeName(s.Mode))
	switch {
	case !s.Loaded && s.Err != nil:
		return h + "  failed"
	case !s.Loaded:
		return h + "  loading…"
	}
	h += fmt.Sprintf("  %d differences", len(s.Snapshot.Diff.Changes))
	for _, n := range s.Snapshot.Notices {
		h += "  " + noticeText(n)
	}
	return h
}

func placeholder(s *state) string {
	switch {
	case len(s.Files) == 0:
		return "nothing to compare"
	case !s.Loaded:
		return ""
	case len(s.Snapshot.Notices) > 0 && len(s.Snapshot.Diff.Rows) == 0:
		return "not shown as text"
	}
	return "no lines"
}

func footer(s *state) string {
	if s.Err != nil {
		return "error: " + s.Err.Error()
	}
	return "q quit  r reload  f full/hunks  n/p change  tab/shift+tab file  j/k scroll  h/l sideways"
}

func modeName(m view.Mode) string {
	if m == view.Full {
		return "full file"
	}
	return "hunks"
}

func noticeText(n filediff.Notice) string {
	side := ""
	switch n.Side {
	case filediff.OldSide:
		side = "old side "
	case filediff.NewSide:
		side = "new side "
	}
	switch n.Kind {
	case filediff.TooLarge:
		return fmt.Sprintf("%stoo large (%d bytes)", side, n.Size)
	case filediff.Binary:
		return side + "binary"
	case filediff.Simplified:
		return "diff simplified"
	}
	return ""
}

// writeLine writes one line of the view: both panes, or a fold across them.
func writeLine(b *strings.Builder, s *state, l view.Line) {
	if l.Fold {
		writeText(b, fmt.Sprintf("  ⋯ %d unchanged lines", l.Rows.End-l.Rows.Start), s.Width)
		return
	}
	d := s.Snapshot.Diff
	r := d.Rows[l.Rows.Start]
	left, right := ' ', ' '
	switch r.Kind {
	case diff.Delete:
		left = '-'
	case diff.Insert:
		right = '+'
	case diff.Change:
		left, right = '~', '~'
	case diff.Equal:
	}
	// The right pane takes the odd column, so a row fills the screen.
	leftWidth := max((s.Width-sepWidth)/2, 1)
	writePane(b, left, side(d.Old, r.Left), s.HScroll, leftWidth)
	b.WriteString(separator)
	writePane(b, right, side(d.New, r.Right), s.HScroll, max(s.Width-sepWidth-leftWidth, 1))
}

func side(sd diff.Side, i int) string {
	if i < 0 {
		return ""
	}
	return diff.Content(sd.Lines[i])
}

// writePane writes one side of a row: its mark, then its text from column
// skip on, padded or cut to width.
func writePane(b *strings.Builder, mark rune, text string, skip, width int) {
	start := b.Len()
	b.WriteRune(mark)
	b.WriteByte(' ')
	writeCells(b, text, skip, width-2)
	pad(b, start, width)
}

// writeText writes text made printable and fitted to width.
func writeText(b *strings.Builder, text string, width int) {
	start := b.Len()
	writeCells(b, text, 0, width)
	pad(b, start, width)
}

// writeCells writes the printable form of text from output column skip on,
// at most width columns, and stops reading text there.
func writeCells(b *strings.Builder, text string, skip, width int) {
	var buf [16]byte
	col, end := 0, skip+width
	for i := 0; i < len(text) && col < end; {
		g, n := glyph(buf[:0], text[i:])
		i += n
		for _, r := range string(g) {
			if col >= skip && col < end {
				b.WriteRune(r)
			}
			col++
		}
	}
}

// pad fills what was written since start up to width runes.
func pad(b *strings.Builder, start, width int) {
	if n := utf8.RuneCountInString(b.String()[start:]); n < width {
		b.WriteString(strings.Repeat(" ", width-n))
	}
}

// glyph appends how the first character of s is shown and says how many
// bytes it took. It replaces what must not reach the terminal as it is,
// following "Steuerzeichen im Terminal" in the plan: C0 controls but tab as
// control pictures, DEL as ␡, C1 controls and invalid UTF-8 as \xNN, bidi
// controls as <U+NNNN>. A tab becomes spaces.
func glyph(dst []byte, s string) ([]byte, int) {
	r, n := utf8.DecodeRuneInString(s)
	switch {
	case r == utf8.RuneError && n == 1:
		return fmt.Appendf(dst, `\x%02X`, s[0]), n
	case r == '\t':
		return append(dst, tab...), n
	case r < 0x20:
		return utf8.AppendRune(dst, 0x2400+r), n
	case r == 0x7f:
		return utf8.AppendRune(dst, '␡'), n
	case r >= 0x80 && r <= 0x9f:
		return fmt.Appendf(dst, `\x%02X`, r), n
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return fmt.Appendf(dst, "<U+%04X>", r), n
	}
	return append(dst, s[:n]...), n
}
