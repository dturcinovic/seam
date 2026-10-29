package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// The overview's columns before a note's text, see writeOverviewNote.
const (
	overviewStateCols  = 9
	overviewOriginCols = 9
	overviewWhereCols  = 23
	// overviewCommitCols is how much of a commit's ID names it.
	overviewCommitCols = 7
)

// sideHead is side W of a file of the changeset as the book counts its
// notes; a side that takes none is left out.
type sideHead struct {
	W   filediff.Which
	Key headKey
}

// headsOf are the sides of file i of the changeset that take notes, the
// first n of hs. An array, not a slice, as the file list and the footer
// ask for every file in every frame.
func (s *state) headsOf(i int) (hs [2]sideHead, n int) {
	f := s.Files[i]
	if left, ok := note.OriginOf(s.Comparison.Left); ok && f.OldPath != "" {
		hs[n], n = sideHead{filediff.OldSide, headKey{left, f.OldPath}}, n+1
	}
	right, ok := note.OriginOf(s.Comparison.Right)
	path := f.NewPath
	if s.Comparison.Right.Kind() == change.KindPath {
		// A file on disk takes notes where it lies in the worktree.
		right, ok, path = note.Origin{Kind: change.KindWorktree}, f.WorktreePath != "", f.WorktreePath
	}
	if ok && path != "" {
		hs[n], n = sideHead{filediff.NewSide, headKey{right, path}}, n+1
	}
	return hs, n
}

// indexSides maps every side of the changeset that takes notes to where
// it is, for the open overview: its rows ask in every frame, and a
// changeset may have tens of thousands of files.
func (s *state) indexSides() {
	o := &s.Notes.Overview
	if !o.Open {
		return
	}
	o.Sides = make(map[headKey]sideRef)
	for i := range s.Files {
		hs, n := s.headsOf(i)
		for _, h := range hs[:n] {
			o.Sides[h.Key] = sideRef{File: i, W: h.W}
		}
	}
}

// sideFor is the side that shows the source and path of n: a side of a
// file of the changeset, or the file visited; none for a note of another
// branch, which shows on no side.
func (s *state) sideFor(n note.Note) (sideRef, bool) {
	if s.Notes.Away[n.ID] {
		return sideRef{}, false
	}
	k := headKey{n.Anchor.Origin, n.Anchor.Path}
	if s.visiting() && k == (headKey{note.Origin{Kind: change.KindWorktree}, s.VisitPath}) {
		return sideRef{File: s.Current, W: filediff.NewSide}, true
	}
	ref, ok := s.Notes.Overview.Sides[k]
	return ref, ok
}

// whereText is where note n stands as the overview names it, empty for a
// note resolved.
func (s *state) whereText(n note.Note) string {
	if n.State == note.Resolved {
		return ""
	}
	if s.Notes.Away[n.ID] {
		return awayText(n.On, s.Notes.Overview.Gone)
	}
	ref, ok := s.sideFor(n)
	if !ok {
		return "not in this comparison"
	}
	sp, ok := s.spotOf(n.ID, ref)
	if !ok {
		return "not placed yet"
	}
	return sp.Place.Where.String()
}

// awayText names the branch or detached commit a note of another branch
// belongs to, or says its branch is gone.
func awayText(on change.Head, gone []string) string {
	switch {
	case slices.Contains(gone, on.Branch):
		return "branch gone"
	case on.Branch != "":
		return "on " + on.Branch
	}
	return "on " + shortCommit(on.Commit)
}

// originText names o in a column of the overview.
func originText(o note.Origin) string {
	if o.Kind == change.KindCommit {
		return shortCommit(o.Commit)
	}
	return o.String()
}

// shortCommit names commit id in a column of the overview.
func shortCommit(id change.ObjectID) string {
	s := id.String()
	return s[:min(len(s), overviewCommitCols)]
}

// layoutOverview lays the open overview out over the body: as wide as a
// popup may be, as high as its rows or the body, with the note chosen in
// view.
func (s *state) layoutOverview() {
	o := &s.Notes.Overview
	if !o.Open {
		return
	}
	o.Title = "notes"
	if !s.centerBox(&o.box, s.boxMaxInner()+popupFrame, len(o.Rows)) {
		return
	}
	shown := o.shown()
	if row := o.selRow(); row >= 0 {
		o.Scroll = min(max(o.Scroll, row-shown+1), row)
		if row > 0 && o.Rows[row-1].Note < 0 && row-1 < o.Scroll && shown > 1 {
			// The heading of the first note shown stays with it.
			o.Scroll = row - 1
		}
	}
	o.Scroll = min(max(o.Scroll, 0), max(len(o.Rows)-shown, 0))
}

// overviewRows writes the rows of the overview: a word while there are no
// notes, else the heading of a path or a note.
func (s *state) overviewRows(ln *line, j int) {
	o := &s.Notes.Overview
	switch {
	case len(o.Rows) == 0:
		ln.text(" "+overviewEmpty(o), popupDimStyle)
	case o.Rows[j].Note < 0:
		ln.text(" "+shownText(o.Rows[j].Path), popupStrongStyle)
	default:
		n := o.Notes[o.Rows[j].Note]
		s.writeOverviewNote(ln, n, overviewLook(n.ID == o.Sel))
	}
}

// rowLook is the styles of a row: its text, its columns and its mark.
type rowLook struct{ text, dim, mark style }

// overviewLook is how a note's row shows: as the popup's text, or chosen
// as the file list's cursor, since the theme has no style to spare.
func overviewLook(chosen bool) rowLook {
	if chosen {
		return rowLook{listStyle(listNamePart, selectedShade), listStyle(listDimPart, selectedShade), listStyle(listDirPart, selectedShade)}
	}
	return rowLook{popupStyle, popupDimStyle, popupLinkStyle}
}

// overviewEmpty says why the overview has no rows.
func overviewEmpty(o *noteOverview) string {
	switch {
	case o.Err != nil:
		return o.Err.Error()
	case !o.Known:
		return "reading notes…"
	}
	return "no notes"
}

// writeOverviewNote writes a note's row: its mark, state, source and
// place in columns, then the first line of its text.
func (s *state) writeOverviewNote(ln *line, n note.Note, lk rowLook) {
	ln.spaces(itemPad+1, lk.text)
	ln.text(noteSign(n), lk.mark)
	ln.spaces(1, lk.text)
	writeCut(ln, n.State.String(), overviewStateCols, lk.dim, false)
	writeCut(ln, originText(n.Anchor.Origin), overviewOriginCols, lk.dim, false)
	writeCut(ln, s.whereText(n), overviewWhereCols, lk.dim, false)
	first, _, _ := strings.Cut(n.Text, "\n")
	writeCut(ln, first, ln.free-itemPad, lk.text, false)
	ln.spaces(ln.free, lk.text)
}

// openNotePopup shows n in a popup for cell x, y: what it is and where it
// was set, the lines it was set at, its text and the answers to it.
func (s *state) openNotePopup(n note.Note, x, y int) {
	p := s.openPopup(noteContent(n, s.whereText(n), s.Notes.Sessions), x, y)
	if p.Open {
		p.Title = "note"
		p.NotesX, p.NotesY = x, y
		if n.State != note.Resolved {
			p.Note = n.ID
		}
	}
	s.Hover.Popup = p
}

func noteContent(n note.Note, where string, sessions []note.Session) hover.Content {
	a := n.Anchor
	head := fmt.Sprintf("%s · %s · %s · %s:%d", n.Kind, stateText(n, sessions), originText(a.Origin), shownText(a.Path), a.Start)
	if a.End > a.Start {
		head += fmt.Sprintf("–%d", a.End)
	}
	if where != "" {
		head += " · " + where
	}
	c := hover.Content{Blocks: []hover.Block{
		{Kind: hover.Heading, Spans: []hover.Span{{Kind: hover.Strong, Text: head}}},
		{Kind: hover.Code, Code: strings.Join(a.Lines, "\n")},
		{Kind: hover.Rule},
		{Kind: hover.Code, Code: n.Text},
	}}
	c.Blocks = append(c.Blocks, repliesBlocks(n.Replies)...)
	return c
}
