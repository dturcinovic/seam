package tui

import (
	"cmp"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// noteOverview is the list of every note of the book, N opens it over the
// body. Notes are by path, then source and age, and Rows their lines with
// a heading per path; Sel is the note chosen, kept by its ID as the book
// changes. Known says the book came once, Err why it did not.
type noteOverview struct {
	Open  bool
	Known bool
	Err   error
	Notes []note.Note
	Rows  []overviewRow
	Sel   note.ID
	// Sides are the sides of the changeset by source and path, see
	// indexSides.
	Sides map[headKey]sideRef
	box
}

// overviewRow is a line of the overview: the heading of Path when Note is
// -1, else note Note of Notes.
type overviewRow struct {
	Note int
	Path string
}

// noteFind is a note enter went to whose place in File is not known yet:
// once the file is loaded and its sides placed it shows there, or in a
// popup.
type noteFind struct {
	Note note.Note
	File int
}

// openOverview opens the overview and asks for the book; it stays empty
// until the book comes.
func (m *model) openOverview() {
	m.dropHover()
	m.s.Notes.Overview = noteOverview{Open: true}
	m.s.indexSides()
	m.s.layoutOverview()
	m.p.NoteOverview.Overview()
}

// takeOverview shows the book of u in the open overview. The note chosen
// stays chosen; if it is gone, the one now in its line.
func (s *state) takeOverview(u note.Update) {
	o := &s.Notes.Overview
	if !o.Open {
		return
	}
	at := max(o.selRow(), 0)
	o.Known, o.Err = true, u.Err
	// The update is this model's alone, Drain handed it over.
	o.Notes = u.Notes
	slices.SortStableFunc(o.Notes, func(a, b note.Note) int {
		return cmp.Or(cmp.Compare(a.Anchor.Path, b.Anchor.Path), cmp.Compare(a.Anchor.Origin.String(), b.Anchor.Origin.String()),
			a.Created.Compare(b.Created))
	})
	o.Rows = o.Rows[:0]
	for i, n := range o.Notes {
		if i == 0 || n.Anchor.Path != o.Notes[i-1].Anchor.Path {
			o.Rows = append(o.Rows, overviewRow{Note: -1, Path: n.Anchor.Path})
		}
		o.Rows = append(o.Rows, overviewRow{Note: i})
	}
	if o.selRow() < 0 {
		o.Sel = ""
		o.moveSel(at, 1)
	}
	s.layoutOverview()
}

// selRow is the row of the note chosen, -1 for none.
func (o *noteOverview) selRow() int {
	return slices.IndexFunc(o.Rows, func(r overviewRow) bool { return r.Note >= 0 && o.Notes[r.Note].ID == o.Sel })
}

// selected is the note chosen.
func (o *noteOverview) selected() (note.Note, bool) {
	if i := o.selRow(); i >= 0 {
		return o.Notes[o.Rows[i].Note], true
	}
	return note.Note{}, false
}

// moveSel chooses the note at row, or the next one in direction dir, or
// else the nearest the other way, as headings take no choice.
func (o *noteOverview) moveSel(row, dir int) {
	row = min(max(row, 0), len(o.Rows)-1)
	for _, d := range []int{dir, -dir} {
		for i := row; i >= 0 && i < len(o.Rows); i += d {
			if r := o.Rows[i]; r.Note >= 0 {
				o.Sel = o.Notes[r.Note].ID
				return
			}
		}
	}
}

// onOverviewAction does an action of the open overview: the file list's
// motions, enter and esc, and x of the popups. It reports whether the
// overview took a; it takes every action but quitting, so that no key
// changes the view under it.
func (m *model) onOverviewAction(a ui.Action) bool {
	s := &m.s
	o := &s.Notes.Overview
	if a == ui.Quit {
		return false
	}
	// A note's popup is of the note chosen, and goes with any choice.
	m.dropHover()
	if mo, ok := motionOf(a); ok {
		steps := mo.steps(len(o.Rows), o.shown())
		o.moveSel(max(o.selRow(), 0)+steps, cmp.Or(cmp.Compare(steps, 0), 1))
		s.layoutOverview()
		return true
	}
	switch a { //nolint:exhaustive // the overview's keys only
	case ui.Back, ui.Notes:
		*o = noteOverview{}
	case ui.Open:
		m.load(s.enterNote())
	case ui.ResolveNote:
		if n, ok := o.selected(); ok && n.State != note.Resolved {
			m.resolve(n.ID)
		}
	case ui.ReleaseNote:
		if n, ok := o.selected(); ok && n.State == note.Taken {
			m.release(n.ID)
		}
	}
	return true
}

// enterNote goes to the note chosen. It jumps to its place only where a
// side of the comparison shows its source and path; a worktree note of a
// file outside the changeset opens that file to read. Either waits for
// the note's place in the file, if not known yet. Every other note opens
// in a popup: one resolved, one without a place, and a commit or index
// note no side shows, even where the worktree has the same path.
func (s *state) enterNote() (filediff.Request, bool) {
	o := &s.Notes.Overview
	n, ok := o.selected()
	if !ok {
		return filediff.Request{}, false
	}
	ref, onSide := s.sideFor(n)
	sp, placed := s.spotOf(n.ID, ref)
	visits := !onSide && n.Anchor.Origin.Kind == change.KindWorktree && s.canVisit()
	if n.State == note.Resolved || onSide && placed && !sp.Place.Where.Shown() || !onSide && !visits {
		s.openNotePopup(n, o.X+1, o.Y+1+o.selRow()-o.Scroll)
		return filediff.Request{}, false
	}
	*o = noteOverview{}
	s.remember()
	switch {
	case visits:
		s.Mode = view.Full
		s.Notes.Find = noteFind{Note: n, File: -1}
		return s.visit(n.Anchor.Path)
	case placed:
		return s.jumpToSpot(ref, sp)
	}
	s.Notes.Find = noteFind{Note: n, File: ref.File}
	return s.showSide(ref.File)
}

// jumpToSpot brings the note at sp in side ref up; the caller remembered
// the place left.
func (s *state) jumpToSpot(ref sideRef, sp note.Spot) (filediff.Request, bool) {
	s.targetSpot(ref, sp)
	return s.showTarget()
}

// targetSpot makes the note at sp in side ref the row a jump brings up.
func (s *state) targetSpot(ref sideRef, sp note.Spot) {
	s.Jump.Target = &target{File: ref.File, Side: ref.W, Line: sp.Place.Start - 1}
	if sp.Place.Where == note.Likely {
		s.Note = "note probably here"
	}
}

// findPlaced shows the note enter went to, once its file is loaded and
// the file's sides are placed: at its place if it has one, else in a
// popup, also where its side takes no notes, as an empty file. It jumps
// itself and leaves the popup of n to the caller, if show.
func (s *state) findPlaced() (n note.Note, show bool) {
	f := s.Notes.Find
	if f.Note.ID == "" {
		return note.Note{}, false
	}
	file := f.File
	if _, loaded := s.loadedOf(file); !loaded || s.awaitsPlaces(file) {
		return note.Note{}, false
	}
	s.Notes.Find = noteFind{}
	for _, w := range []filediff.Which{filediff.OldSide, filediff.NewSide} {
		ref := sideRef{File: file, W: w}
		if sp, ok := s.spotOf(f.Note.ID, ref); ok && sp.Place.Where.Shown() {
			s.targetSpot(ref, sp)
			s.bringTarget()
			return note.Note{}, false
		}
	}
	return f.Note, true
}

// spotOf is note id among the placed notes of side ref; ok is false while
// the side is not placed, or has no such note open.
func (s *state) spotOf(id note.ID, ref sideRef) (note.Spot, bool) {
	if k, ok := s.noteKey(ref.File, ref.W); !ok || s.Notes.Asked[k] {
		return note.Spot{}, false
	}
	spots := s.spotsOf(ref.File, ref.W)
	i := slices.IndexFunc(spots, func(sp note.Spot) bool { return sp.Note.ID == id })
	if i < 0 {
		return note.Spot{}, false
	}
	return spots[i], true
}

// overviewKey gives key to the open overview, which has no layer of its
// own: it takes the file list's actions, x of the popups and N of the
// view. A layer of its own would only repeat the list's keys in the
// configuration. It reports whether the key is done with.
func (m *model) overviewKey(key string) bool {
	k := m.s.Keys
	for _, l := range []ui.Layer{ui.ListLayer, ui.PopupLayer, ui.ViewLayer} {
		if a, ok := k.Action(l, key); ok && (l != ui.PopupLayer || a.OnNote()) {
			return m.onOverviewAction(a)
		}
	}
	return true
}

// boxRests lets the mouse rest while the overview or the choice of a
// session is open, as it does for the field: the hovers would cover them. The wheel still scrolls a
// note's popup opened from it.
func (s *state) boxRests(msg tea.Msg) bool {
	if w, ok := msg.(tea.MouseWheelMsg); ok && s.Hover.Popup.contains(w.X, w.Y) && len(s.Hover.Popup.Lines) > s.Hover.Popup.shown() {
		return false
	}
	return mouseOrHover(msg)
}
