package tui

import (
	"path"
	"strconv"
	"strings"

	"seam/internal/domain/change"
	"seam/internal/textcell"
)

// The parts of a line of the file list: a blank at either edge, and before
// a file's name the gutter "│ " and the status with its blank.
const (
	itemPad    = 1
	gutterCols = 2
	statusCols = 2
)

// minCountWidth is the narrowest column of counts, " +150"; minNameWidth
// is how narrow a name gets before the counts make room for it.
const (
	minCountWidth = 5
	minNameWidth  = 6
)

// footerText is the start of the list's last line, after the number of
// files.
const footerText = " files · [ ] dir"

// writeEntry writes body line i of the file list, its separator included:
// a folder's heading, a file, or on the last line the totals.
func writeEntry(b *strings.Builder, s *state, th *theme, i int) {
	ln := newLine(b, th, s.Cells, s.Layout.List-separatorWidth)
	j := s.List.Scroll + i
	switch {
	case i == s.listLines():
		writeTotals(&ln, &s.List)
	case j >= len(s.List.Items):
		ln.end(plain)
	case s.List.Items[j].heading():
		writeHeading(&ln, s.List.Items[j])
	default:
		f := s.List.Items[j].file
		writeFile(&ln, s.Files[f], s.List.CountWidth, shadeOf(s, f))
	}
	ln = newLine(b, th, s.Cells, separatorWidth)
	ln.text("│", separatorStyle)
	ln.end(separatorStyle)
}

// shadeOf is how file f stands out: selected is the cursor of a focused
// list, or else the file shown, which a focused list only makes bold.
func shadeOf(s *state, f int) listShade {
	switch {
	case s.List.Focus && f == s.List.Cursor, !s.List.Focus && f == s.Current:
		return selectedShade
	case f == s.Current:
		return currentShade
	}
	return plainShade
}

// writeHeading writes " internal/adapter/  3 ", the folder cut at the left
// so its last part stays, its count at the right edge.
func writeHeading(ln *line, it listItem) {
	st := listStyle(listNamePart, plainShade)
	ln.spaces(itemPad, st)
	writeCut(ln, folderText(it.dir), ln.free-1-digits(it.count)-itemPad, listStyle(listDirPart, plainShade), true)
	ln.number(it.count, 1+digits(it.count), listStyle(listDimPart, plainShade))
	ln.end(st)
}

// writeFile writes " │ M name   +3   -1 ", the counts in columns of cw;
// a list too narrow for them drops them.
func writeFile(ln *line, f change.ChangedFile, cw int, sh listShade) {
	st := listStyle(listNamePart, sh)
	ln.spaces(itemPad, st)
	ln.text("│ ", listStyle(listGutterPart, sh))
	ln.text(f.Status.String(), listStyle(statusPart(f.Status), sh))
	ln.spaces(1, st)
	name := ln.free - 2*cw - itemPad
	if name < minNameWidth {
		cw, name = 0, ln.free-itemPad
	}
	writeCut(ln, fileName(f), name, st, false)
	if cw > 0 {
		writeCounts(ln, f.Counts, cw, sh)
	}
	ln.end(st)
}

// writeTotals writes " 25 files · [ ] dir  +1081  -75 " on the list's last
// line.
func writeTotals(ln *line, l *fileList) {
	st := listStyle(listDimPart, plainShade)
	cw := l.CountWidth
	ln.spaces(itemPad, st)
	text := ln.free - 2*cw - itemPad
	if text < minNameWidth {
		cw, text = 0, ln.free-itemPad
	}
	writeCut(ln, totalsText(len(l.At)), text, st, false)
	if cw > 0 {
		writeCounts(ln, l.Total, cw, plainShade)
	}
	ln.end(st)
}

// writeCounts writes "+n" and "-m" right-aligned in two columns of cw;
// blank where git counted nothing.
func writeCounts(ln *line, c change.LineCounts, cw int, sh listShade) {
	if !c.Known {
		ln.spaces(2*cw, listStyle(listNamePart, sh))
		return
	}
	writeCount(ln, "+", c.Added, listAddedPart, cw, sh)
	writeCount(ln, "-", c.Deleted, listDeletedPart, cw, sh)
}

// writeCount writes sign and n right-aligned in cw columns, a zero in grey.
func writeCount(ln *line, sign string, n int, p listPart, cw int, sh listShade) {
	if n == 0 {
		p = listZeroPart
	}
	ln.spaces(cw-len(sign)-digits(n), listStyle(listNamePart, sh))
	ln.text(sign, listStyle(p, sh))
	ln.number(n, 0, listStyle(p, sh))
}

// writeCut writes p in exactly width columns. A longer one loses its end to
// "…", or with atStart its start.
func writeCut(ln *line, p string, width int, st style, atStart bool) {
	rest := ln.narrow(max(width, 0))
	if w := textWidth(p, ln.method); w > ln.free && ln.free > 0 {
		if atStart {
			ln.text("…", st)
			ln.window(p, nil, w-ln.free, cellStyle{fixed: st})
		} else {
			inner := ln.narrow(ln.free - 1)
			ln.text(p, st)
			ln.free += inner
			ln.text("…", st)
		}
	} else {
		ln.text(p, st)
	}
	ln.spaces(ln.free, st)
	ln.free = rest
}

// itemWidth is the columns item it of files takes uncut, with counts in
// columns of cw.
func itemWidth(it listItem, files []change.ChangedFile, cw int, m textcell.Method) int {
	if it.heading() {
		return itemPad + textWidth(folderText(it.dir), m) + 1 + digits(it.count) + itemPad
	}
	return itemPad + gutterCols + statusCols + textWidth(fileName(files[it.file]), m) + 2*cw + itemPad
}

// totalsText starts the list's last line for n files.
func totalsText(n int) string {
	return strconv.Itoa(n) + footerText
}

// totalsWidth is the columns the totals line of n files takes uncut.
func totalsWidth(n, cw int, m textcell.Method) int {
	return itemPad + textWidth(totalsText(n), m) + 2*cw + itemPad
}

// fileName is how the name of f shows, its folder left to the heading.
func fileName(f change.ChangedFile) string {
	_, name := path.Split(f.Path())
	return shownText(name)
}

// folderText is how a folder heads its files; the root is "./".
func folderText(dir string) string {
	if dir == "" {
		return "./"
	}
	return shownText(dir)
}

// statusPart colours the status letter: green for added, red for deleted,
// grey for untracked, blue for any other change.
func statusPart(st change.Status) listPart {
	switch st {
	case change.Added:
		return listAddedPart
	case change.Deleted:
		return listDeletedPart
	case change.Untracked:
		return listDimPart
	case change.Modified, change.Renamed, change.Copied, change.TypeChanged, change.Unmerged:
	}
	return listChangedPart
}
