package tui

import (
	"slices"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/domain/view"
	"github.com/dturcinovic/seam/internal/textcell"
)

// hStep is how far h and l, or a notch of the wheel sideways, move the
// panes, in columns.
const hStep = 8

// wheelLines is how far a notch of the wheel scrolls.
const wheelLines = 3

// The actions change the state and return what the ports should do; they do
// no I/O themselves, so they are tested without a terminal or ports.

// open makes file i current and returns the request for it; ok is false if
// i names no file. Another file starts at the top; the same file, read
// afresh, keeps its place and stays shown until the new one arrives.
func (s *state) open(i int, fresh bool) (r filediff.Request, ok bool) {
	if i < 0 || i >= len(s.Files) {
		return filediff.Request{}, false
	}
	if i != s.Current {
		s.unload()
		s.Current, s.VisitPath = i, ""
		s.List.follow(i, s.listLines())
	}
	s.LoadGen++
	return filediff.Request{Gen: s.LoadGen, Comparison: s.Comparison, File: s.Files[i], Fresh: fresh}, true
}

// unload shows no file until the next one arrives.
func (s *state) unload() {
	s.Snapshot, s.Loaded = filediff.Snapshot{}, false
	s.LSP.Left, s.LSP.Right = lang.Instance{}, lang.Instance{}
	s.Pos, s.HScroll = view.Pos{}, 0
	s.Caret = caret{}
	s.place(s.Layout.Width, s.Layout.Height)
	s.show(nil)
}

// step opens the file n places away, staying at either end of the list.
func (s *state) step(n int) (filediff.Request, bool) {
	if s.Stream.On {
		s.streamStep(n)
		return filediff.Request{}, false
	}
	i := min(max(s.Current+n, 0), len(s.Files)-1)
	if i == s.Current {
		return filediff.Request{}, false
	}
	return s.open(i, false)
}

// apply shows u if it answers the last request; a late answer for a file
// left since is dropped. shown reports a new snapshot on screen.
func (s *state) apply(u filediff.Update) (shown bool) {
	if u.Gen != s.LoadGen {
		return false
	}
	if u.Err != nil {
		s.Err = u.Err
		return false
	}
	s.Snapshot, s.Loaded, s.Err, s.ShownGen = u.Snapshot, true, nil, u.Gen
	s.relayout()
	s.bringTarget()
	return true
}

// reload returns the generation of a new reload of the changeset.
func (s *state) reload() change.Gen {
	s.ReloadGen++
	s.Reloading = true
	return s.ReloadGen
}

// watchReload is reload for a change the watcher saw; ok is false while a
// reload runs, which a new one would cancel: on a repository that takes
// longer to list than a burst lasts, a busy writer would leave every reload
// unfinished. The change waits for the running reload's answer instead.
func (s *state) watchReload() (gen change.Gen, ok bool) {
	if s.Reloading {
		s.WatchAgain = true
		return 0, false
	}
	return s.reload(), true
}

// reloaded takes the new file list if u answers the last reload, and returns
// the fresh request for the file shown, or for the one that took its place
// if it is gone.
func (s *state) reloaded(u change.Update) (filediff.Request, bool) {
	if u.Gen != s.ReloadGen {
		return filediff.Request{}, false
	}
	s.Reloading = false
	if u.Err != nil {
		s.Err = u.Err
		return filediff.Request{}, false
	}
	before := s.Files
	shown, had := s.file()
	// Their file indexes are of the files before.
	s.Jump.Target, s.Jump.Waiting = nil, nil
	old := s.Current
	// A commit or a checkout may have moved the commits, and the files'
	// loads must ask for the new ones.
	s.Comparison = u.Changeset.Comparison
	s.setFiles(u.Changeset.Files)
	if s.Stream.On {
		// Before place, which may lay the stream out anew: its files must
		// already have their new indexes.
		s.streamReloaded(before)
	}
	// open places the screen anew only for another file, and the list's
	// width and whether it shows depend on the files.
	s.place(s.Layout.Width, s.Layout.Height)
	s.List.Cursor = min(s.List.Cursor, max(len(s.Files)-1, 0))
	if s.Stream.On {
		s.layStream()
		s.List.keep(s.Current, s.listLines())
		return filediff.Request{}, false
	}
	s.Current = slices.IndexFunc(s.Files, func(f change.ChangedFile) bool { return had && f.Path() == shown.Path() })
	if s.Current >= 0 {
		// The same file stays current, so open does not bring it into
		// view, though files before it may have come or gone; nor does it
		// drop the caret, which goes with its file. A file visited that is
		// in the changeset now is one of it.
		s.Caret.File, s.VisitPath = s.Current, ""
		r, ok := s.open(s.Current, true)
		s.List.keep(s.Current, s.listLines())
		return r, ok
	}
	if s.visiting() {
		// Still outside the changeset: read afresh, at its place.
		return s.visitRequest(true), true
	}
	if len(s.Files) == 0 {
		s.unload()
		// A load still running is for a file that is gone.
		s.LoadGen++
		return filediff.Request{}, false
	}
	return s.open(min(max(old, 0), len(s.Files)-1), true)
}

// resize takes the new terminal size; the top line stays where it is.
func (s *state) resize(width, height int) {
	body := s.Layout.Body
	laid := s.place(width, height)
	s.List.keep(s.Current, s.listLines())
	if laid {
		return
	}
	if s.Stream.On {
		s.switchStream()
		return
	}
	if s.Layout.Body != body {
		s.mark()
	}
	s.Pos = view.Switch(s.Lines, s.Pos, s.bodyHeight())
}

// place lays out the screen for the current file, and its lines anew when
// the sides change between side by side and unified; laid says it did.
func (s *state) place(width, height int) (laid bool) {
	was := s.Layout.Unified
	s.Layout = computeLayout(width, height, s.List.columns(width), s.Snapshot.Diff, s.unified)
	if s.Layout.Unified != was {
		s.layLines()
		laid = true
	}
	// Only a list on the screen can have the focus, and a page that
	// shrank keeps its last line full.
	if s.Layout.List == 0 {
		s.List.blur()
	}
	s.scrollList(0)
	return laid
}

// setFiles is the one way to replace the files: in the order the list
// shows them, so that Tab and the list agree, and with the list built anew.
func (s *state) setFiles(files []change.ChangedFile) {
	s.Files = sortByFolder(files)
	s.List.index(s.Files, s.Cells)
}

// show is the one way to change Lines, so the scrollbar's marks follow.
func (s *state) show(lines []view.Line) {
	s.Lines = lines
	s.mark()
}

// mark derives the scrollbar's marks from Lines at the body's height.
func (s *state) mark() {
	if s.visiting() {
		// Every row is new to a diff against nothing; none is a change.
		s.Marks = nil
		return
	}
	if s.Stream.On {
		s.Marks = streamMarks(s, s.Layout.Body)
		return
	}
	s.Marks = changeMarks(s.Snapshot.Diff, s.Lines, s.Layout.Body)
}

// unified says whether a diff of width columns shows unified.
func (s *state) unified(width int) bool {
	if s.visiting() {
		// It has one side only.
		return true
	}
	switch s.LayoutSetting {
	case ui.SplitLayout:
		return false
	case ui.UnifiedLayout:
		return true
	case ui.AutoLayout:
	}
	return width < s.SplitMinWidth
}

// toggleLayout turns the sides the other way, for good: a resize after it
// leaves them so.
func (s *state) toggleLayout() {
	s.LayoutSetting = ui.UnifiedLayout
	if s.Layout.Unified {
		s.LayoutSetting = ui.SplitLayout
	}
	s.place(s.Layout.Width, s.Layout.Height)
}

// toggleMode switches between the hunk and the full view at the same row.
func (s *state) toggleMode() {
	if s.Mode == view.Full {
		s.Mode = view.Hunks
	} else {
		s.Mode = view.Full
	}
	s.relayout()
}

// scrollBy moves the top line by n lines.
func (s *state) scrollBy(n int) {
	s.scrollTo(s.top() + n)
}

// useCells takes the way the terminal counts columns, which the list's
// width depends on.
func (s *state) useCells(m textcell.Method) {
	s.Cells = m
	s.List.index(s.Files, m)
	s.place(s.Layout.Width, s.Layout.Height)
}

// pageLines is how far a page of height lines moves, or half a page.
func pageLines(height int, half bool) int {
	if half {
		return max(height/2, 1)
	}
	return height
}

// nextChange brings the first change below the top line to the top, in
// this file or, in the stream, a later one.
func (s *state) nextChange() {
	first, end := s.bodyFiles()
	for f := max(s.Current, first); f < end; f++ {
		for _, line := range s.changeLines(f) {
			if line > s.top() {
				s.scrollTo(line)
				return
			}
		}
	}
}

// prevChange brings the last change above the top line to the top, in
// this file or, in the stream, an earlier one.
func (s *state) prevChange() {
	first, _ := s.bodyFiles()
	for f := s.Current; f >= first; f-- {
		for _, line := range slices.Backward(s.changeLines(f)) {
			if line < s.top() {
				s.scrollTo(line)
				return
			}
		}
	}
}

// changeLines are the lines of the body where the changes of file start.
// In the stream a file not laid out, or one without changes, counts by
// its header, so that n stops at every file.
func (s *state) changeLines(file int) []int {
	lines, snap := s.linesOf(file), s.snapOf(file)
	if len(lines) == 0 || len(snap.Diff.Changes) == 0 {
		if s.Stream.On {
			return []int{s.Stream.View.Start(file)}
		}
		return nil
	}
	out := make([]int, len(snap.Diff.Changes))
	for i, c := range snap.Diff.Changes {
		out[i] = s.bodyIndex(file, view.LineForRow(lines, c.Start))
	}
	return out
}

// shiftH moves the panes sideways by n steps, never left of the start.
func (s *state) shiftH(n int) {
	s.HScroll = max(s.HScroll+n*hStep, 0)
}

// unfoldAt shows the rows of the fold at screen cell x, y one by one; a
// click anywhere else does nothing.
func (s *state) unfoldAt(x, y int) {
	if i, ok := s.Layout.bodyLine(x, y); ok {
		s.unfoldLine(s.top() + i)
	}
}

// unfoldLine shows the rows of the fold at line i of the body one by one.
func (s *state) unfoldLine(i int) {
	if s.Stream.On {
		s.streamUnfold(i)
		return
	}
	lines, pos := view.Unfold(s.Lines, s.Pos, i)
	// Unfold hands back the same lines when there was no fold, and then
	// the marks stand.
	if len(lines) != len(s.Lines) {
		s.Pos = pos
		s.show(lines)
	}
}

func (s *state) scrollTo(line int) {
	if s.Stream.On {
		s.streamScrollTo(line)
		return
	}
	s.Pos = view.Navigate(s.Lines, s.Pos, line, s.bodyHeight())
}

// relayout lays out the diff again after it or the mode changed, keeping
// the anchor row.
func (s *state) relayout() {
	if !s.place(s.Layout.Width, s.Layout.Height) {
		s.layLines()
	}
}

// layLines lays out the diff in Mode, unified if Layout is, and keeps the
// anchor row on the screen.
func (s *state) layLines() {
	if s.Stream.On {
		s.layStream()
		return
	}
	s.show(s.linesFor(&s.Snapshot))
	s.Pos = view.Switch(s.Lines, s.Pos, s.bodyHeight())
}
