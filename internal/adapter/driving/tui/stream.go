package tui

import (
	"slices"

	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/lang"
	"seam/internal/domain/view"
)

// streamWindow bounds the files the stream holds loaded beside those on
// the screen: a screen's height above and below fills up to this many.
// The files on the screen are never left out, however many.
const streamWindow = 8

// streamState is the stream: every file below the one before, each with a
// header. It holds only the files of its window; what leaves it is dropped,
// as the use case's cache keeps it for a while. Current follows the file
// at its top; Lines, Pos, Snapshot and LSP's servers are the single view's.
type streamState struct {
	On   bool
	View *view.Stream
	Pos  view.StreamPos
	// Held are the loaded files by index in Files.
	Held map[int]*heldFile
	// Gen is the generation of the files asked for; it changes with the
	// files, so an answer for an index of the list before is dropped.
	Gen filediff.Gen
	// Wanted is what was asked for last and asked the window it was asked
	// at, so that neither the same is asked again nor the window worked
	// out anew with every mouse move.
	Wanted []filediff.Request
	asked  windowKey
}

// heldFile is a loaded file of the stream, with the servers of its sides,
// or the error its load failed with, which its header shows: the footer's
// error is the screen's, not of one file among many. Stale is from before
// a reload, shown until its fresh answer. The digits of its line numbers
// are worked out once rather than with every row drawn.
type heldFile struct {
	snap        filediff.Snapshot
	err         error
	servers     servers
	stale       bool
	left, right int
}

func newHeld(snap filediff.Snapshot, sv servers) *heldFile {
	return &heldFile{snap: snap, servers: sv, left: digits(len(snap.Diff.Old.Lines)), right: digits(len(snap.Diff.New.Lines))}
}

// windowKey is what the window depends on.
type windowKey struct {
	gen               filediff.Gen
	scroll, body, len int
}

// servers are the instances serving the sides of a file.
type servers struct {
	Left, Right lang.Instance
}

// shown is the instance that serves side w.
func (v servers) shown(w filediff.Which) lang.Instance {
	if w == filediff.OldSide {
		return v.Left
	}
	return v.Right
}

// toggleStream turns the stream on or off at the same top line, and
// returns the request for the file the single view is to show, if it was
// not loaded.
func (s *state) toggleStream() (filediff.Request, bool) {
	if s.Stream.On {
		return s.leaveStream()
	}
	s.enterStream()
	return filediff.Request{}, false
}

// enterStream shows every file, with the file shown so far at its top line
// and loaded as it is.
func (s *state) enterStream() {
	st := &s.Stream
	*st = streamState{On: true, Gen: st.Gen + 1, Held: map[int]*heldFile{}, View: view.NewStream(s.estimates())}
	if s.Loaded {
		h := newHeld(s.Snapshot, s.LSP.servers)
		// An answer still to come, as for a reload, is the stream's to ask
		// for now: the single view drops it.
		h.stale = s.ShownGen != s.LoadGen
		st.Held[s.Current] = h
		st.View.Lay(s.Current, s.Lines)
	}
	s.LoadGen++
	st.Pos = view.StreamPosFor(max(s.Current, 0), s.Pos)
	s.Pos, s.Lines = view.Pos{}, nil
	s.switchStream()
}

// leaveStream shows the file at the top on its own, from its top row, as
// open does: shown at once if it is loaded, else asked for.
func (s *state) leaveStream() (filediff.Request, bool) {
	st := &s.Stream
	f, pos := st.Pos.File, st.Pos.FilePos()
	h := st.Held[f]
	*st = streamState{Gen: st.Gen}
	s.Current = -1
	stale := h != nil && h.stale
	r, ok := s.open(f, stale)
	s.Pos = pos
	if !ok || h == nil || h.err != nil {
		return r, ok
	}
	s.apply(filediff.Update{Gen: r.Gen, Snapshot: h.snap})
	s.LSP.servers = h.servers
	if !stale {
		return filediff.Request{}, false
	}
	// Shown until its fresh answer, which r asks for.
	s.ShownGen = r.Gen - 1
	return r, ok
}

// estimates are the guessed heights of the files in the current view.
func (s *state) estimates() []int {
	out := make([]int, len(s.Files))
	for i, f := range s.Files {
		out[i] = view.Estimate(f.Counts, s.Mode, s.Context)
	}
	return out
}

// linesFor lays out snap as the current view does.
func (s *state) linesFor(snap *filediff.Snapshot) []view.Line {
	lines := view.Lines(snap.Diff, s.Mode, s.Context)
	if s.Layout.Unified {
		lines = view.Unify(snap.Diff, lines)
	}
	return lines
}

// layStream lays out the files held anew, after the mode or the sides
// changed, and the others by their estimates in it.
func (s *state) layStream() {
	st := &s.Stream
	st.View = view.NewStream(s.estimates())
	for f, h := range st.Held {
		st.View.Lay(f, s.linesFor(&h.snap))
	}
	s.switchStream()
}

// switchStream places the stream after its lines changed, keeping the top
// line.
func (s *state) switchStream() {
	s.Stream.Pos = s.Stream.View.Switch(s.Stream.Pos, s.bodyHeight())
	s.mark()
	s.followTop()
}

// followTop makes the file at the top the current one, which the header,
// the list and the footer name.
func (s *state) followTop() {
	f := -1
	if len(s.Files) > 0 {
		f = s.Stream.Pos.File
	}
	if f != s.Current {
		s.Current = f
		s.List.follow(f, s.listLines())
	}
}

// streamApply holds the files of us that answer the files asked for, and
// places the stream once for all of them; files are those to tell the
// servers of.
func (s *state) streamApply(us []filediff.Update) (files []int) {
	st := &s.Stream
	for _, u := range us {
		if !st.On || u.Gen != st.Gen {
			continue
		}
		f := slices.Index(s.Files, u.File)
		if f < 0 {
			continue
		}
		if u.Err != nil {
			st.Held[f] = &heldFile{err: u.Err}
			st.View.Lay(f, nil)
			continue
		}
		st.Held[f] = newHeld(u.Snapshot, servers{})
		st.View.Lay(f, s.linesFor(&st.Held[f].snap))
		files = append(files, f)
	}
	if len(us) > 0 && st.On {
		s.switchStream()
	}
	return files
}

// streamWant drops the files that left the window and returns the
// requests for those in it that are missing or stale, the visible ones
// first; ok is false when that is what was asked for last.
func (s *state) streamWant() ([]filediff.Request, bool) {
	st := &s.Stream
	key := windowKey{st.Gen, st.Pos.Scroll, s.bodyHeight(), st.View.Len()}
	if key == st.asked {
		return nil, false
	}
	st.asked = key
	window := s.window()
	for f := range st.Held {
		if !slices.Contains(window, f) {
			delete(st.Held, f)
			st.View.Forget(f)
		}
	}
	var rs []filediff.Request
	for _, f := range window {
		h := st.Held[f]
		if h != nil && !h.stale {
			continue
		}
		rs = append(rs, filediff.Request{Gen: st.Gen, Comparison: s.Comparison, File: s.Files[f], Fresh: h != nil})
	}
	if slices.Equal(rs, st.Wanted) {
		return nil, false
	}
	st.Wanted = rs
	return rs, true
}

// window is the files to hold: those on the screen, then those of a
// screen's height below and above it, up to streamWindow.
func (s *state) window() []int {
	st := &s.Stream
	if st.View.Len() == 0 {
		return nil
	}
	h := s.bodyHeight()
	top := st.Pos.Scroll
	first, last := st.View.At(top).File, st.View.At(top+h-1).File
	out := make([]int, 0, max(streamWindow, last-first+1))
	for f := first; f <= last; f++ {
		out = append(out, f)
	}
	below, above := st.View.At(top+2*h-1).File, st.View.At(top-h).File
	for f := last + 1; f <= below && len(out) < streamWindow; f++ {
		out = append(out, f)
	}
	for f := first - 1; f >= above && len(out) < streamWindow; f-- {
		out = append(out, f)
	}
	return out
}

// streamReloaded takes the new files of a reload by their paths: the file
// at the top stays at the top with its row, or the one that took its place
// does, and so does the caret's file. Files kept are shown as they were
// until their fresh answer, once layStream lays them out.
func (s *state) streamReloaded(before []change.ChangedFile) {
	st := &s.Stream
	byPath := make(map[string]int, len(s.Files))
	for i, f := range s.Files {
		byPath[f.Path()] = i
	}
	held := st.Held
	st.Held, st.Wanted, st.asked = map[int]*heldFile{}, nil, windowKey{}
	st.Gen++
	for old, h := range held {
		if f, ok := byPath[before[old].Path()]; ok && h.err == nil {
			h.stale = true
			st.Held[f] = h
		}
	}
	if c := &s.Caret; c.On {
		f, kept := byPath[before[c.File].Path()]
		c.File, c.On = f, kept
	}
	top := st.Pos.File
	f, kept := 0, false
	if top < len(before) {
		f, kept = byPath[before[top].Path()]
	}
	if kept {
		st.Pos.File = f
	} else {
		// The file that took its place: the next one, or the last.
		st.Pos = view.StreamPos{File: min(top, max(len(s.Files)-1, 0))}
	}
	st.View = view.NewStream(s.estimates())
}

// streamScrollTo moves the top line to line i of the stream.
func (s *state) streamScrollTo(i int) {
	s.Stream.Pos = s.Stream.View.Navigate(s.Stream.Pos, i, s.bodyHeight())
	s.followTop()
}

// streamStep brings the header of the file n places away to the top; back
// by one goes to the top file's own header first, unless it is on top.
func (s *state) streamStep(n int) {
	st := &s.Stream
	f := st.Pos.File + n
	if n < 0 && s.top() > st.View.Start(st.Pos.File) {
		f++
	}
	s.streamJump(f)
}

// streamJump brings the header of file to the top, as the file list does
// in the stream.
func (s *state) streamJump(file int) {
	if file < 0 || file >= len(s.Files) {
		return
	}
	s.streamScrollTo(s.Stream.View.Start(file))
}

// streamUnfold shows the rows of the fold at line i of the stream one by
// one. A fold that held the anchor hands it to its first row, as Unfold
// does for a single file.
func (s *state) streamUnfold(i int) {
	st := &s.Stream
	f, l, ok := s.rowLine(i)
	if !ok || !l.Fold {
		return
	}
	lines, _ := st.View.Lines(f)
	out, _ := view.Unfold(lines, view.Pos{}, i-st.View.Start(f)-1)
	if st.Pos.File == f && st.Pos.Spot == view.OnRow && l.Contains(st.Pos.AnchorRow) {
		st.Pos.AnchorRow, st.Pos.AnchorHalf = l.Rows.Start, view.Whole
	}
	st.View.Lay(f, out)
	s.switchStream()
}
