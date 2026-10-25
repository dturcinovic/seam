package tui

import (
	"fmt"
	"strings"

	"seam/internal/domain/view"
)

// writeStreamLine writes a line of the stream that shows no rows: a file's
// header, or a stand-in for the lines of one not loaded yet.
func writeStreamLine(b *strings.Builder, s *state, th *theme, bl bodyLine) {
	width := s.Layout.panes()
	if bl.spot == view.OnHead {
		writeText(b, s, th, fileHead(s, bl.file), headerStyle, width)
		return
	}
	text := ""
	if bl.guess == 0 {
		text = "  loading…"
	}
	writeText(b, s, th, text, foldStyle, width)
}

// fileHead is the header of a file of the stream: its status, path and
// counts, and once loaded what it leaves out.
func fileHead(s *state, file int) string {
	f := s.Files[file]
	var b strings.Builder
	fmt.Fprintf(&b, " %s %s", f.Status, shownText(f.Path()))
	if f.Counts.Known {
		fmt.Fprintf(&b, "  +%d -%d", f.Counts.Added, f.Counts.Deleted)
	}
	if h, held := s.Stream.Held[file]; held && h.err != nil {
		b.WriteString("  failed: " + h.err.Error())
	} else if held {
		b.WriteString(noticesText(h.snap.Notices))
	}
	return b.String()
}
